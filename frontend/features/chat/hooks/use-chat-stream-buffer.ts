"use client";

import * as React from "react";

import { clearLiveUpstreamThinkTrace, upsertLiveUpstreamThinkTrace } from "@/features/chat/model/upstream-think-store";
import type { PendingExchangeMap } from "@/features/chat/types/chat-runtime";
import type { StreamMessageEvent } from "@/shared/api/conversation.types";

const STREAM_TEXT_FLUSH_INTERVAL_MS = 50;
const STREAM_THINK_FLUSH_INTERVAL_MS = 40;

type UpstreamThinkDeltaEvent = Extract<StreamMessageEvent, { type: "upstream_think_delta" }>;

type StreamBuffer = {
  runID: string | null;
  pendingText: string;
  textFrame: number | null;
  textTimeout: number | null;
  lastTextFlushAt: number;
  pendingThinkDelta: string;
  pendingThinkEvent: UpstreamThinkDeltaEvent | null;
  thinkFrame: number | null;
  thinkTimeout: number | null;
  lastThinkFlushAt: number;
};

function createStreamBuffer(runID?: string): StreamBuffer {
  return {
    runID: runID?.trim() || null,
    pendingText: "",
    textFrame: null,
    textTimeout: null,
    lastTextFlushAt: 0,
    pendingThinkDelta: "",
    pendingThinkEvent: null,
    thinkFrame: null,
    thinkTimeout: null,
    lastThinkFlushAt: 0,
  };
}

function reasoningEventTarget(event: UpstreamThinkDeltaEvent) {
  return `${event.kind ?? ""}:${event.summaryIndex ?? ""}:${event.contentIndex ?? ""}`;
}

function flushPendingThinkEvent(buffer: StreamBuffer) {
  if (!buffer.runID || !buffer.pendingThinkEvent) {
    return;
  }
  upsertLiveUpstreamThinkTrace(buffer.runID, {
    ...buffer.pendingThinkEvent,
    delta: buffer.pendingThinkDelta,
    contentMarkdown: buffer.pendingThinkDelta ? undefined : buffer.pendingThinkEvent.contentMarkdown,
  });
  buffer.pendingThinkDelta = "";
  buffer.pendingThinkEvent = null;
}

function cancelBufferTimers(buffer: StreamBuffer) {
  if (buffer.textFrame !== null) {
    window.cancelAnimationFrame(buffer.textFrame);
  }
  if (buffer.textTimeout !== null) {
    window.clearTimeout(buffer.textTimeout);
  }
  if (buffer.thinkFrame !== null) {
    window.cancelAnimationFrame(buffer.thinkFrame);
  }
  if (buffer.thinkTimeout !== null) {
    window.clearTimeout(buffer.thinkTimeout);
  }
}

export function useChatStreamBuffer({
  setPendingExchanges,
}: {
  setPendingExchanges: React.Dispatch<React.SetStateAction<PendingExchangeMap>>;
}) {
  const buffersRef = React.useRef(new Map<string, StreamBuffer>());
  const scheduleThinkFlushRef = React.useRef<(exchangeKey: string) => void>(() => undefined);

  const flushStreamText = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer) {
      return;
    }
    buffer.textFrame = null;
    buffer.lastTextFlushAt = performance.now();
    const pendingText = buffer.pendingText;
    if (!pendingText) {
      return;
    }
    buffer.pendingText = "";

    setPendingExchanges((current) => {
      const exchange = current[exchangeKey];
      if (!exchange) {
        return current;
      }
      return {
        ...current,
        [exchangeKey]: {
          ...exchange,
          assistantPending: false,
          assistantStreaming: true,
          assistantText: exchange.assistantText + pendingText,
        },
      };
    });
  }, [setPendingExchanges]);

  const flushUpstreamThink = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer) {
      return;
    }
    buffer.thinkFrame = null;
    buffer.lastThinkFlushAt = performance.now();
    if (!buffer.runID || !buffer.pendingThinkEvent) {
      return;
    }

    const delta = buffer.pendingThinkDelta;
    buffer.pendingThinkDelta = "";
    const event: UpstreamThinkDeltaEvent = {
      ...buffer.pendingThinkEvent,
      delta,
      contentMarkdown: delta ? undefined : buffer.pendingThinkEvent.contentMarkdown,
    };
    buffer.pendingThinkEvent = null;
    upsertLiveUpstreamThinkTrace(buffer.runID, event);
  }, []);

  const scheduleStreamFlush = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer || buffer.textFrame !== null || buffer.textTimeout !== null) {
      return;
    }
    const elapsed = performance.now() - buffer.lastTextFlushAt;
    if (elapsed >= STREAM_TEXT_FLUSH_INTERVAL_MS) {
      buffer.textFrame = window.requestAnimationFrame(() => flushStreamText(exchangeKey));
      return;
    }
    buffer.textTimeout = window.setTimeout(() => {
      buffer.textTimeout = null;
      buffer.textFrame = window.requestAnimationFrame(() => flushStreamText(exchangeKey));
    }, STREAM_TEXT_FLUSH_INTERVAL_MS - elapsed);
  }, [flushStreamText]);

  const scheduleUpstreamThinkFlush = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer || buffer.thinkFrame !== null || buffer.thinkTimeout !== null) {
      return;
    }
    const elapsed = performance.now() - buffer.lastThinkFlushAt;
    if (elapsed >= STREAM_THINK_FLUSH_INTERVAL_MS) {
      buffer.thinkFrame = window.requestAnimationFrame(() => flushUpstreamThink(exchangeKey));
      return;
    }
    buffer.thinkTimeout = window.setTimeout(() => {
      buffer.thinkTimeout = null;
      buffer.thinkFrame = window.requestAnimationFrame(() => flushUpstreamThink(exchangeKey));
    }, STREAM_THINK_FLUSH_INTERVAL_MS - elapsed);
  }, [flushUpstreamThink]);

  React.useEffect(() => {
    scheduleThinkFlushRef.current = scheduleUpstreamThinkFlush;
  }, [scheduleUpstreamThinkFlush]);

  const enqueueStreamText = React.useCallback((exchangeKey: string, delta: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer || !delta) {
      return;
    }
    buffer.pendingText += delta;
    scheduleStreamFlush(exchangeKey);
  }, [scheduleStreamFlush]);

  const enqueueUpstreamThinkDelta = React.useCallback((exchangeKey: string, event: UpstreamThinkDeltaEvent) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer) {
      return;
    }
    if (event.trace?.enabled || typeof event.contentMarkdown === "string") {
      buffer.pendingThinkDelta = "";
      buffer.pendingThinkEvent = null;
    } else if (
      buffer.pendingThinkEvent &&
      (
        buffer.pendingThinkEvent.trace?.enabled ||
        typeof buffer.pendingThinkEvent.contentMarkdown === "string" ||
        reasoningEventTarget(buffer.pendingThinkEvent) !== reasoningEventTarget(event)
      )
    ) {
      flushPendingThinkEvent(buffer);
    }
    if (event.trace?.enabled || typeof event.contentMarkdown === "string") {
      buffer.pendingThinkDelta = "";
      buffer.pendingThinkEvent = event;
      scheduleUpstreamThinkFlush(exchangeKey);
      return;
    }
    if (event.delta) {
      buffer.pendingThinkDelta += event.delta;
    }
    buffer.pendingThinkEvent = { ...event, delta: "" };
    scheduleUpstreamThinkFlush(exchangeKey);
  }, [scheduleUpstreamThinkFlush]);

  const startStream = React.useCallback((exchangeKey: string, runID?: string) => {
    const existing = buffersRef.current.get(exchangeKey);
    if (existing) {
      cancelBufferTimers(existing);
    }
    const buffer = createStreamBuffer(runID);
    buffersRef.current.set(exchangeKey, buffer);
    clearLiveUpstreamThinkTrace(buffer.runID);
  }, []);

  const flushStreamTextNow = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer) {
      return;
    }
    if (buffer.textFrame !== null) {
      window.cancelAnimationFrame(buffer.textFrame);
      buffer.textFrame = null;
    }
    if (buffer.textTimeout !== null) {
      window.clearTimeout(buffer.textTimeout);
      buffer.textTimeout = null;
    }
    flushStreamText(exchangeKey);
  }, [flushStreamText]);

  const flushUpstreamThinkNow = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer) {
      return;
    }
    if (buffer.thinkFrame !== null) {
      window.cancelAnimationFrame(buffer.thinkFrame);
      buffer.thinkFrame = null;
    }
    if (buffer.thinkTimeout !== null) {
      window.clearTimeout(buffer.thinkTimeout);
      buffer.thinkTimeout = null;
    }
    if (!buffer.runID || !buffer.pendingThinkEvent) {
      return;
    }
    flushPendingThinkEvent(buffer);
  }, []);

  const resetStreamBuffer = React.useCallback((exchangeKey?: string) => {
    if (exchangeKey) {
      const buffer = buffersRef.current.get(exchangeKey);
      if (!buffer) {
        return;
      }
      cancelBufferTimers(buffer);
      buffersRef.current.delete(exchangeKey);
      return;
    }
    for (const buffer of buffersRef.current.values()) {
      cancelBufferTimers(buffer);
    }
    buffersRef.current.clear();
  }, []);

  React.useEffect(() => () => resetStreamBuffer(), [resetStreamBuffer]);

  return {
    enqueueUpstreamThinkDelta,
    enqueueStreamText,
    flushStreamTextNow,
    flushUpstreamThinkNow,
    resetStreamBuffer,
    startStream,
  };
}
