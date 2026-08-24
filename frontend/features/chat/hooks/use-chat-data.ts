"use client";

import { useTranslations } from "next-intl";
import * as React from "react";
import { applyAgentExecutionEvent } from "@/features/chat/model/agent-run-store";
import {
  shouldRetryConversationStream,
  shouldRefreshMessagesAfterHistory,
  shouldSurfaceConversationLoadError,
} from "@/features/chat/model/conversation-load-policy";
import { buildMediaImagePreviewMarkdown } from "@/features/chat/model/media-image-preview";
import { upsertLiveUpstreamThinkTrace } from "@/features/chat/model/upstream-think-store";
import {
  cancelMessageGeneration,
  ensureConversationHistory,
  getConversationHistory,
  listMessagesPage,
  resumeMessageGenerationStream,
} from "@/shared/api/conversation";
import type { MessageDTO } from "@/shared/api/conversation.types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

const MESSAGE_PAGE_SIZE = 30;
const HISTORY_POLL_INTERVAL_MS = 500;
const HISTORY_POLL_ATTEMPTS = 250;
const RESUME_TEXT_FLUSH_INTERVAL_MS = 100;
const RESUME_RETRY_INITIAL_DELAY_MS = 500;
const RESUME_RETRY_MAX_DELAY_MS = 5_000;

type ChatDataState = {
  loading: boolean;
  loadingOlder: boolean;
  errorMsg: string;
  messages: MessageDTO[];
  total: number;
  hasOlder: boolean;
};

type ActiveResumeStream = {
  controller: AbortController;
  runID: string;
  accessToken: string | null;
};

type ResumeTextReplayState = {
  baseContent: string;
  replayedContent: string;
  visibleContent: string;
};

function appendResumedTextDelta(state: ResumeTextReplayState, delta: string): string {
  if (!delta) {
    return state.visibleContent;
  }

  state.replayedContent += delta;
  const { baseContent, replayedContent } = state;
  if (!baseContent) {
    state.visibleContent = replayedContent;
    return state.visibleContent;
  }

  if (
    replayedContent === baseContent ||
    baseContent.startsWith(replayedContent) ||
    baseContent.includes(replayedContent)
  ) {
    state.visibleContent = baseContent;
    return state.visibleContent;
  }

  if (replayedContent.startsWith(baseContent)) {
    state.visibleContent = replayedContent;
    return state.visibleContent;
  }

  const maxOverlapLength = Math.min(baseContent.length, replayedContent.length);
  for (let length = maxOverlapLength; length > 0; length -= 1) {
    if (baseContent.endsWith(replayedContent.slice(0, length))) {
      state.visibleContent = `${baseContent}${replayedContent.slice(length)}`;
      return state.visibleContent;
    }
  }

  state.visibleContent = `${state.visibleContent}${delta}`;
  return state.visibleContent;
}

export function useChatData(
  conversationID: string | null,
  {
    activeGenerationRunsRef,
    failedGenerationRunsRef,
    generationSeqByRunRef,
  }: {
    activeGenerationRunsRef?: React.RefObject<Set<string>>;
    failedGenerationRunsRef?: React.RefObject<Set<string>>;
    generationSeqByRunRef?: React.MutableRefObject<Record<string, number>>;
  } = {},
) {
  const t = useTranslations("chat.data");
  const tSubmit = useTranslations("chat.submit");
  const [state, setState] = React.useState<ChatDataState>({
    loading: Boolean(conversationID),
    loadingOlder: false,
    errorMsg: "",
    messages: [],
    total: 0,
    hasOlder: false,
  });
  const [reloadToken, setReloadToken] = React.useState(0);
  const [resumingRunID, setResumingRunID] = React.useState("");
  const stateRef = React.useRef(state);
  stateRef.current = state;
  const previousConversationIDRef = React.useRef<string | null>(conversationID);
  const resumeSeqByRunRef = React.useRef<Record<string, number>>({});
  const pendingAssistantContentRef = React.useRef("");
  const resumeTextReplayByRunRef = React.useRef<Record<string, ResumeTextReplayState>>({});
  const activeResumeStreamRef = React.useRef<ActiveResumeStream | null>(null);
  const refreshedPendingRunsRef = React.useRef(new Set<string>());
  // 恢复游标只在对应的可见内容仍被保留时有效，两者必须同步清理。
  const clearResumeCheckpoint = React.useCallback((runID: string) => {
    const normalizedRunID = runID.trim();
    if (!normalizedRunID) {
      return;
    }
    delete resumeSeqByRunRef.current[normalizedRunID];
    delete resumeTextReplayByRunRef.current[normalizedRunID];
    delete generationSeqByRunRef?.current[normalizedRunID];
  }, [generationSeqByRunRef]);

  React.useEffect(() => {
    let cancelled = false;

    async function load() {
      if (!conversationID) {
        setState({
          loading: false,
          loadingOlder: false,
          errorMsg: "",
          messages: [],
          total: 0,
          hasOlder: false,
        });
        return;
      }

      const isConversationSwitch = previousConversationIDRef.current !== conversationID;
      previousConversationIDRef.current = conversationID;
      setState((prev) => ({
        loading: isConversationSwitch || prev.messages.length === 0,
        loadingOlder: false,
        errorMsg: "",
        messages: isConversationSwitch ? [] : prev.messages,
        total: isConversationSwitch ? 0 : prev.total,
        hasOlder: isConversationSwitch ? false : prev.hasOlder,
      }));
      try {
        const token = await resolveAccessToken();
        if (!token) {
          if (!cancelled) {
            setState({
              loading: false,
              loadingOlder: false,
              errorMsg: t("signInRequired"),
              messages: [],
              total: 0,
              hasOlder: false,
            });
          }
          return;
        }

        const initialData = await listMessagesPage(token, conversationID, {
          page: 1,
          pageSize: MESSAGE_PAGE_SIZE,
          tail: true,
        });
        if (cancelled) {
          return;
        }
        setState((prev) => {
          const firstTailMessageID = initialData.results[0]?.id ?? 0;
          const loadedOlderMessages =
            isConversationSwitch || firstTailMessageID <= 0 || prev.messages.length <= MESSAGE_PAGE_SIZE
              ? []
              : prev.messages.filter((message) => message.id < firstTailMessageID);
          const messages = [...loadedOlderMessages, ...initialData.results];
          return {
            loading: initialData.results.length === 0,
            loadingOlder: false,
            errorMsg: "",
            messages,
            total: initialData.total,
            hasOlder: messages.length < initialData.total,
          };
        });

        let history = await ensureConversationHistory(token, conversationID);
        const historyWasLoaded = history.status === "loaded";
        for (let attempt = 0; history.status !== "loaded" && attempt < HISTORY_POLL_ATTEMPTS; attempt += 1) {
          if (history.status === "error") {
            throw new Error(history.error || t("loadFailed"));
          }
          await new Promise((resolve) => window.setTimeout(resolve, HISTORY_POLL_INTERVAL_MS));
          if (cancelled) return;
          history = await getConversationHistory(token, conversationID);
        }
        if (history.status !== "loaded") {
          throw new Error(history.error || t("loadFailed"));
        }

        if (!shouldRefreshMessagesAfterHistory(initialData.results.length, historyWasLoaded)) {
          return;
        }
        const data = await listMessagesPage(token, conversationID, {
          page: 1,
          pageSize: MESSAGE_PAGE_SIZE,
          tail: true,
        });
        if (cancelled) {
          return;
        }

        setState((prev) => {
          const firstTailMessageID = data.results[0]?.id ?? 0;
          // 只有已加载过额外历史页时才保留旧区间，避免普通 reload 无限累积 tail 消息。
          const loadedOlderMessages =
            isConversationSwitch ||
            firstTailMessageID <= 0 ||
            prev.messages.length <= MESSAGE_PAGE_SIZE
              ? []
              : prev.messages.filter((message) => message.id < firstTailMessageID);
          const messages = [...loadedOlderMessages, ...data.results];
          return {
            loading: false,
            loadingOlder: false,
            errorMsg: "",
            messages,
            total: data.total,
            hasOlder: messages.length < data.total,
          };
        });
      } catch (error) {
        if (!cancelled) {
          setState((prev) => ({
            ...prev,
            loading: false,
            loadingOlder: false,
            errorMsg: shouldSurfaceConversationLoadError(prev.messages.length)
              ? error instanceof Error && error.message
                ? error.message
                : t("loadFailed")
              : "",
          }));
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [conversationID, reloadToken, t]);

  const reload = React.useCallback(() => {
    setReloadToken((prev) => prev + 1);
  }, []);

  const replaceMessage = React.useCallback((nextMessage: MessageDTO) => {
    setState((prev) => ({
      ...prev,
      messages: prev.messages.map((message) =>
        message.publicID === nextMessage.publicID ? nextMessage : message,
      ),
    }));
  }, []);

  const loadOlderMessages = React.useCallback(async () => {
    const current = stateRef.current;
    if (!conversationID || current.loading || current.loadingOlder || !current.hasOlder || current.messages.length === 0) {
      return false;
    }

    const beforeID = current.messages[0]?.id ?? 0;
    if (beforeID <= 0) {
      setState((prev) => {
        const next = { ...prev, hasOlder: false };
        stateRef.current = next;
        return next;
      });
      return false;
    }

    setState((prev) => {
      const next = { ...prev, loadingOlder: true };
      stateRef.current = next;
      return next;
    });
    try {
      const token = await resolveAccessToken();
      if (!token) {
        setState((prev) => {
          const next = { ...prev, loadingOlder: false, hasOlder: false };
          stateRef.current = next;
          return next;
        });
        return false;
      }

      const data = await listMessagesPage(token, conversationID, {
        pageSize: MESSAGE_PAGE_SIZE,
        beforeID,
      });
      if (previousConversationIDRef.current !== conversationID) {
        return false;
      }
      let loaded = false;
      setState((prev) => {
        const existingPublicIDs = new Set(prev.messages.map((message) => message.publicID));
        const olderMessages = data.results.filter((message) => !existingPublicIDs.has(message.publicID));
        const messages = [...olderMessages, ...prev.messages];
        loaded = olderMessages.length > 0;
        const next = {
          ...prev,
          loadingOlder: false,
          messages,
          total: data.total,
          hasOlder: loaded && messages.length < data.total,
        };
        stateRef.current = next;
        return next;
      });
      return loaded;
    } catch {
      setState((prev) => {
        const next = { ...prev, loadingOlder: false };
        stateRef.current = next;
        return next;
      });
      return false;
    }
  }, [conversationID]);

  const loadAllOlderMessages = React.useCallback(async ({ maxPages = 50 }: { maxPages?: number } = {}) => {
    for (let iteration = 0; iteration < maxPages; iteration += 1) {
      if (!stateRef.current.hasOlder) {
        return true;
      }
      const loaded = await loadOlderMessages();
      if (!loaded) {
        return !stateRef.current.hasOlder;
      }
    }
    return !stateRef.current.hasOlder;
  }, [loadOlderMessages]);

  const cancelResumedGeneration = React.useCallback(async () => {
    const active = activeResumeStreamRef.current;
    if (!active) {
      return false;
    }

    active.controller.abort();
    clearResumeCheckpoint(active.runID);
    setResumingRunID("");
    setState((prev) => {
      const next = {
        ...prev,
        messages: prev.messages.map((message) =>
          message.runID === active.runID && message.role === "assistant" && message.status === "pending"
            ? { ...message, status: "interrupted", activityLabel: "" }
            : message,
        ),
      };
      stateRef.current = next;
      return next;
    });

    const token = active.accessToken ?? (await resolveAccessToken());
    if (!token) {
      return false;
    }

    const result = await cancelMessageGeneration(token, active.runID).catch(() => null);
    reload();
    return Boolean(result?.canceled);
  }, [clearResumeCheckpoint, reload]);

  const pendingAssistant = React.useMemo(() => {
    for (let index = state.messages.length - 1; index >= 0; index -= 1) {
      const message = state.messages[index];
      if (message.role === "assistant" && message.status === "pending") {
        return message;
      }
    }
    return null;
  }, [state.messages]);

  const pendingRunID = pendingAssistant?.runID?.trim() || "";

  React.useEffect(() => {
    pendingAssistantContentRef.current = pendingAssistant?.content ?? "";
  }, [pendingAssistant?.content]);

  React.useEffect(() => {
    if (
      !conversationID ||
      !pendingRunID ||
      activeGenerationRunsRef?.current.has(pendingRunID) ||
      failedGenerationRunsRef?.current.has(pendingRunID)
    ) {
      setResumingRunID("");
      return;
    }

    const controller = new AbortController();
    let closed = false;
    let pendingTextDelta = "";
    let textFlushTimer: number | null = null;
    const afterSeq = Math.max(
      resumeSeqByRunRef.current[pendingRunID] ?? 0,
      generationSeqByRunRef?.current[pendingRunID] ?? 0,
    );
    const baseContent = pendingAssistantContentRef.current;
    const resumeTextReplayByRun = resumeTextReplayByRunRef.current;
    const clearResumeTextReplay = () => {
      delete resumeTextReplayByRun[pendingRunID];
    };
    const isResumeInactive = () => closed || controller.signal.aborted;
    const updateResumeState = (update: (current: ChatDataState) => ChatDataState) => {
      setState((current) => isResumeInactive() ? current : update(current));
    };
    const flushPendingText = () => {
      textFlushTimer = null;
      const delta = pendingTextDelta;
      pendingTextDelta = "";
      if (!delta || isResumeInactive()) return;
      let replayState = resumeTextReplayByRun[pendingRunID];
      if (!replayState) {
        replayState = {
          baseContent: "",
          replayedContent: "",
          visibleContent: "",
        };
        resumeTextReplayByRun[pendingRunID] = replayState;
      }
      const nextContent = appendResumedTextDelta(replayState, delta);
      updateResumeState((prev) => ({
        ...prev,
        messages: prev.messages.map((message) =>
          message.runID === pendingRunID && message.role === "assistant" && message.status === "pending"
            ? { ...message, content: nextContent }
            : message,
        ),
      }));
    };
    const scheduleTextFlush = () => {
      if (textFlushTimer !== null) return;
      textFlushTimer = window.setTimeout(flushPendingText, RESUME_TEXT_FLUSH_INTERVAL_MS);
    };
    resumeTextReplayByRun[pendingRunID] = {
      baseContent,
      replayedContent: afterSeq > 0 ? baseContent : "",
      visibleContent: baseContent,
    };
    activeResumeStreamRef.current = {
      controller,
      runID: pendingRunID,
      accessToken: null,
    };
    setResumingRunID(pendingRunID);

    const reconcileTerminalRun = async (token: string) => {
      try {
        const latest = await listMessagesPage(token, conversationID, {
          page: 1,
          pageSize: MESSAGE_PAGE_SIZE,
          tail: true,
        });
        const assistant = latest.results.find(
          (message) => message.role === "assistant" && message.runID === pendingRunID,
        );
        if (!assistant || assistant.status.trim().toLowerCase() === "pending") {
          return false;
        }
        refreshedPendingRunsRef.current.add(pendingRunID);
        clearResumeCheckpoint(pendingRunID);
        reload();
        return true;
      } catch {
        return false;
      }
    };
    let resumeRetryDelay = RESUME_RETRY_INITIAL_DELAY_MS;

    async function resume() {
      let retry = false;
      let token = "";
      try {
        token = (await resolveAccessToken()) ?? "";
        if (!token || controller.signal.aborted) {
          return;
        }
        if (activeResumeStreamRef.current?.controller === controller) {
          activeResumeStreamRef.current.accessToken = token;
        }
        const completed = await resumeMessageGenerationStream(token, pendingRunID, {
          signal: controller.signal,
          afterSeq: Math.max(
            resumeSeqByRunRef.current[pendingRunID] ?? afterSeq,
            generationSeqByRunRef?.current[pendingRunID] ?? afterSeq,
          ),
          onExecutionEvent: (event) => applyAgentExecutionEvent(event, conversationID),
          onEventSeq: (seq) => {
            if (isResumeInactive()) {
              return;
            }
            resumeSeqByRunRef.current[pendingRunID] = Math.max(resumeSeqByRunRef.current[pendingRunID] ?? 0, seq);
            if (generationSeqByRunRef) {
              generationSeqByRunRef.current[pendingRunID] = Math.max(
                generationSeqByRunRef.current[pendingRunID] ?? 0,
                seq,
              );
            }
          },
          onMediaStatus: (event) => {
            const status = event.status.trim();
            const contentType = event.content_type === "video" ? "video" : "image";
            const activityLabel =
              status === "queued"
                ? tSubmit(contentType === "video" ? "mediaStatus.videoQueued" : "mediaStatus.queued")
                : status === "running"
                  ? tSubmit(contentType === "video" ? "mediaStatus.videoRunning" : "mediaStatus.running")
                  : status === "saving_artifact"
                    ? tSubmit(contentType === "video" ? "mediaStatus.videoSavingArtifact" : "mediaStatus.savingArtifact")
                    : event.message.trim() || status;
            updateResumeState((prev) => ({
              ...prev,
              messages: prev.messages.map((message) =>
                message.runID === pendingRunID && message.role === "assistant" && message.status === "pending"
                  ? { ...message, activityLabel, contentType }
                  : message,
              ),
            }));
          },
          onMediaImageDelta: (event) => {
            if (isResumeInactive()) {
              return;
            }
            clearResumeTextReplay();
            pendingTextDelta = "";
            if (textFlushTimer !== null) {
              window.clearTimeout(textFlushTimer);
              textFlushTimer = null;
            }
            const previewMarkdown = buildMediaImagePreviewMarkdown(event, tSubmit("imagePreviewAlt"));
            if (!previewMarkdown) {
              return;
            }
            updateResumeState((prev) => ({
              ...prev,
              messages: prev.messages.map((message) =>
                message.runID === pendingRunID && message.role === "assistant" && message.status === "pending"
                  ? { ...message, content: previewMarkdown, contentType: "image", activityLabel: "" }
                  : message,
              ),
            }));
          },
          onDelta: (delta) => {
            if (isResumeInactive()) {
              return;
            }
            pendingTextDelta += delta;
            scheduleTextFlush();
          },
          onProcessUpdate: (event) => {
            updateResumeState((prev) => ({
              ...prev,
              messages: prev.messages.map((message) =>
                message.runID === pendingRunID && message.role === "assistant" && message.status === "pending"
                  ? { ...message, processTrace: event.trace }
                  : message,
              ),
            }));
          },
          onUpstreamThinkDelta: (event) => {
            if (isResumeInactive()) {
              return;
            }
            upsertLiveUpstreamThinkTrace(pendingRunID, event);
          },
          onUsage: (event) => {
            updateResumeState((prev) => ({
              ...prev,
              messages: prev.messages.map((message) =>
                message.runID === pendingRunID && message.role === "assistant" && message.status === "pending"
                  ? {
                      ...message,
                      inputTokens: event.input_tokens > 0 ? event.input_tokens : message.inputTokens,
                      outputTokens: event.output_tokens > 0 ? event.output_tokens : message.outputTokens,
                      cacheReadTokens:
                        event.cache_read_tokens > 0 ? event.cache_read_tokens : message.cacheReadTokens,
                      cacheWriteTokens:
                        event.cache_write_tokens > 0 ? event.cache_write_tokens : message.cacheWriteTokens,
                      reasoningTokens:
                        event.reasoning_tokens > 0 ? event.reasoning_tokens : message.reasoningTokens,
                    }
                  : message,
              ),
            }));
          },
        });
        flushPendingText();
        if (!isResumeInactive() && completed) {
          refreshedPendingRunsRef.current.add(pendingRunID);
          clearResumeCheckpoint(pendingRunID);
          reload();
          return;
        }
        if (await reconcileTerminalRun(token)) {
          return;
        }
        retry = true;
      } catch (error) {
        if (isResumeInactive() || (error instanceof Error && error.name === "AbortError")) {
          return;
        }
        if (!shouldRetryConversationStream(error)) {
          clearResumeCheckpoint(pendingRunID);
          return;
        }
        if (token && (await reconcileTerminalRun(token))) {
          return;
        }
        retry = true;
      } finally {
        if (!retry) {
          if (activeResumeStreamRef.current?.controller === controller) {
            activeResumeStreamRef.current = null;
          }
          if (!controller.signal.aborted && !closed) {
            setResumingRunID("");
          }
        }
      }

      await new Promise((resolve) => window.setTimeout(resolve, resumeRetryDelay));
      resumeRetryDelay = Math.min(resumeRetryDelay * 2, RESUME_RETRY_MAX_DELAY_MS);
      if (!isResumeInactive()) {
        void resume();
      }
    }

    void resume();
    return () => {
      closed = true;
      if (textFlushTimer !== null) {
        window.clearTimeout(textFlushTimer);
      }
      controller.abort();
      clearResumeCheckpoint(pendingRunID);
      if (activeResumeStreamRef.current?.controller === controller) {
        activeResumeStreamRef.current = null;
      }
    };
  }, [
    activeGenerationRunsRef,
    clearResumeCheckpoint,
    conversationID,
    failedGenerationRunsRef,
    generationSeqByRunRef,
    pendingRunID,
    reload,
    tSubmit,
  ]);

  React.useEffect(() => {
    if (
      !conversationID ||
      !pendingAssistant ||
      activeGenerationRunsRef?.current.has(pendingRunID) ||
      failedGenerationRunsRef?.current.has(pendingRunID) ||
      (pendingRunID && pendingRunID === resumingRunID)
    ) {
      return;
    }
    const pendingKey = pendingRunID || pendingAssistant.publicID;
    if (refreshedPendingRunsRef.current.has(pendingKey)) {
      return;
    }
    const timer = window.setTimeout(() => {
      refreshedPendingRunsRef.current.add(pendingKey);
      reload();
    }, 1500);
    return () => {
      window.clearTimeout(timer);
    };
  }, [activeGenerationRunsRef, conversationID, failedGenerationRunsRef, pendingAssistant, pendingRunID, reload, resumingRunID]);

  return {
    ...state,
    cancelResumedGeneration,
    loadOlderMessages,
    loadAllOlderMessages,
    reload,
    replaceMessage,
    resumingRunID,
  };
}
