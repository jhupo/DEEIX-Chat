"use client";

import * as React from "react";
import {
  applyAgentExecutionEvents,
  getAgentExecutionRecoverySnapshot,
  replaceActiveAgentInteractions,
  setAgentRunContext,
  useAgentExecutionRecoverySnapshot,
} from "@/features/chat/model/agent-run-store";
import {
  listConversationExecutionEvents,
  listConversationInteractions,
} from "@/shared/api/conversation";
import type { ConversationExecutionEventDTO } from "@/shared/api/conversation.types";
import { streamAgentEvents } from "@/shared/api/agent-gateway";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

const EXECUTION_EVENT_RETRY_MAX_MS = 15_000;
const INTERACTION_RETRY_MAX_MS = 15_000;
const EVENT_STREAM_RECONNECT_MAX_MS = 15_000;
const EVENT_SYNC_DEBOUNCE_MS = 250;

type AgentRunHydrationScope = {
  conversationID: string | null;
  deviceID?: string;
  profileID?: string;
  workspaceID?: string;
  onExecutionBoundary?: (event: ConversationExecutionEventDTO) => void;
};

export function useAgentRunHydration({
  conversationID,
  deviceID = "",
  profileID = "",
  workspaceID = "",
  onExecutionBoundary,
}: AgentRunHydrationScope) {
  const recovery = useAgentExecutionRecoverySnapshot();
  const normalizedConversationID = conversationID?.trim() || "";
  const contextKey = JSON.stringify([
    normalizedConversationID,
    deviceID.trim(),
    profileID.trim(),
    workspaceID.trim(),
  ]);
  const requestExecutionSyncRef = React.useRef<(() => void) | null>(null);
  const onExecutionBoundaryRef = React.useRef(onExecutionBoundary);
  React.useLayoutEffect(() => {
    onExecutionBoundaryRef.current = onExecutionBoundary;
  }, [onExecutionBoundary]);

  React.useEffect(() => {
    setAgentRunContext(contextKey, normalizedConversationID);
    if (!normalizedConversationID) return;

    let cancelled = false;
    let accessToken = "";
    let eventSync: Promise<void> | null = null;
    let eventSyncRequested = false;
    let interactionSync: Promise<void> | null = null;
    let interactionSyncRequested = false;
    let eventRetryTimer: number | null = null;
    let eventRetryDelay = 1_000;
    let interactionRetryTimer: number | null = null;
    let interactionRetryDelay = 1_000;
    let streamController: AbortController | null = null;
    let streamReconnectTimer: number | null = null;
    let streamReconnectDelay = 1_000;
    let syncDebounceTimer: number | null = null;

    const syncEvents = () => {
      eventSyncRequested = true;
      if (eventSync) return eventSync;
      eventSync = (async () => {
        while (eventSyncRequested && !cancelled) {
          eventSyncRequested = false;
          let cursor = getAgentExecutionRecoverySnapshot().contiguousSeq;
          while (!cancelled) {
            const events = await listConversationExecutionEvents(
              accessToken,
              normalizedConversationID,
              cursor,
            );
            if (cancelled || events.length === 0) break;
            let nextCursor = cursor;
            const sortedEvents = events.slice().sort((left, right) => left.seq - right.seq);
            applyAgentExecutionEvents(sortedEvents, normalizedConversationID);
            for (const event of sortedEvents) {
              nextCursor = Math.max(nextCursor, event.seq);
              if (event.kind === "turn/started" || event.kind === "turn/completed") {
                onExecutionBoundaryRef.current?.(event);
              }
            }
            if (nextCursor <= cursor) break;
            cursor = nextCursor;
          }
        }
      })().finally(() => {
        eventSync = null;
      });
      return eventSync;
    };

    const requestExecutionSync = () => {
      if (!accessToken || cancelled) return;
      void syncEvents().then(
        () => {
          eventRetryDelay = 1_000;
          if (eventRetryTimer !== null) {
            window.clearTimeout(eventRetryTimer);
            eventRetryTimer = null;
          }
        },
        () => {
          if (cancelled || eventRetryTimer !== null) return;
          eventRetryTimer = window.setTimeout(() => {
            eventRetryTimer = null;
            requestExecutionSync();
          }, eventRetryDelay);
          eventRetryDelay = Math.min(eventRetryDelay * 2, EXECUTION_EVENT_RETRY_MAX_MS);
        },
      );
    };
    requestExecutionSyncRef.current = requestExecutionSync;

    const syncInteractions = () => {
      interactionSyncRequested = true;
      if (interactionSync) return interactionSync;
      interactionSync = (async () => {
        while (interactionSyncRequested && !cancelled) {
          interactionSyncRequested = false;
          const [pending, responding, failed] = await Promise.all([
            listConversationInteractions(accessToken, normalizedConversationID, "pending"),
            listConversationInteractions(accessToken, normalizedConversationID, "responding"),
            listConversationInteractions(accessToken, normalizedConversationID, "failed"),
          ]);
          if (!cancelled) replaceActiveAgentInteractions([...pending, ...responding, ...failed]);
        }
      })().finally(() => {
        interactionSync = null;
      });
      return interactionSync;
    };

    const requestInteractionSync = () => {
      if (!accessToken || cancelled) return;
      void syncInteractions().then(
        () => {
          interactionRetryDelay = 1_000;
          if (interactionRetryTimer !== null) {
            window.clearTimeout(interactionRetryTimer);
            interactionRetryTimer = null;
          }
        },
        () => {
          if (cancelled || interactionRetryTimer !== null) return;
          interactionRetryTimer = window.setTimeout(() => {
            interactionRetryTimer = null;
            requestInteractionSync();
          }, interactionRetryDelay);
          interactionRetryDelay = Math.min(
            interactionRetryDelay * 2,
            INTERACTION_RETRY_MAX_MS,
          );
        },
      );
    };

    const syncLatest = () => {
      requestExecutionSync();
      requestInteractionSync();
    };

    const scheduleLatestSync = () => {
      if (cancelled || syncDebounceTimer !== null) return;
      syncDebounceTimer = window.setTimeout(() => {
        syncDebounceTimer = null;
        syncLatest();
      }, EVENT_SYNC_DEBOUNCE_MS);
    };

    function scheduleStreamReconnect() {
      if (cancelled || streamReconnectTimer !== null) return;
      streamReconnectTimer = window.setTimeout(() => {
        streamReconnectTimer = null;
        connectEventStream();
      }, streamReconnectDelay);
      streamReconnectDelay = Math.min(streamReconnectDelay * 2, EVENT_STREAM_RECONNECT_MAX_MS);
    }

    function connectEventStream() {
      if (!accessToken || cancelled || streamController) return;
      const controller = new AbortController();
      streamController = controller;
      void streamAgentEvents(accessToken, controller.signal, (type) => {
        streamReconnectDelay = 1_000;
        if (document.visibilityState === "hidden") return;
        if (type === "ready") syncLatest();
        else scheduleLatestSync();
      }).catch(() => undefined).finally(() => {
        if (streamController === controller) streamController = null;
        scheduleStreamReconnect();
      });
    }

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") syncLatest();
    };

    async function start() {
      accessToken = (await resolveAccessToken()) ?? "";
      if (!accessToken || cancelled) return;
      connectEventStream();
      document.addEventListener("visibilitychange", onVisibilityChange);
    }

    void start();
    return () => {
      cancelled = true;
      requestExecutionSyncRef.current = null;
      document.removeEventListener("visibilitychange", onVisibilityChange);
      streamController?.abort();
      if (streamReconnectTimer !== null) window.clearTimeout(streamReconnectTimer);
      if (syncDebounceTimer !== null) window.clearTimeout(syncDebounceTimer);
      if (eventRetryTimer !== null) window.clearTimeout(eventRetryTimer);
      if (interactionRetryTimer !== null) window.clearTimeout(interactionRetryTimer);
    };
  }, [contextKey, normalizedConversationID]);

  React.useEffect(() => {
    if (recovery.hasGap) requestExecutionSyncRef.current?.();
  }, [contextKey, recovery.hasGap, recovery.highestSeq]);
}
