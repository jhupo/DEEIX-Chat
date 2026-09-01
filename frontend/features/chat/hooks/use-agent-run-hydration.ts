"use client";

import * as React from "react";
import {
  applyAgentExecutionEvents,
  replaceActiveAgentInteractions,
  setAgentRunContext,
} from "@/features/chat/model/agent-run-store";
import {
  listConversationExecutionEvents,
  listConversationInteractions,
} from "@/shared/api/conversation";
import type { ConversationExecutionEventDTO } from "@/shared/api/conversation.types";
import type { AgentStreamEvent } from "@/shared/api/agent-gateway";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

const EXECUTION_EVENT_RETRY_MAX_MS = 15_000;
const INTERACTION_RETRY_MAX_MS = 15_000;
const LATEST_SYNC_DEBOUNCE_MS = 100;
const HISTORY_INVALIDATION_DEBOUNCE_MS = 100;
const THREAD_HISTORY_EVENT_KIND = "thread/history/updated";

type AgentRunHydrationScope = {
  conversationID: string | null;
  deviceID?: string;
  profileID?: string;
  workspaceID?: string;
  runIDs?: string[];
  agentEvent?: AgentStreamEvent | null;
  onExecutionBoundary?: (event: ConversationExecutionEventDTO) => void;
  onConversationInvalidated?: () => void;
};

export function useAgentRunHydration({
  conversationID,
  deviceID = "",
  profileID = "",
  workspaceID = "",
  runIDs = [],
  agentEvent = null,
  onExecutionBoundary,
  onConversationInvalidated,
}: AgentRunHydrationScope) {
  const normalizedConversationID = conversationID?.trim() || "";
  const runIDKey = [...new Set(runIDs.map((runID) => runID.trim()).filter(Boolean))].slice(-64).join(",");
  const contextKey = JSON.stringify([
    normalizedConversationID,
    deviceID.trim(),
    profileID.trim(),
    workspaceID.trim(),
  ]);
  const requestExecutionSyncRef = React.useRef<(() => void) | null>(null);
  const requestLatestSyncRef = React.useRef<(() => void) | null>(null);
  const onExecutionBoundaryRef = React.useRef(onExecutionBoundary);
  const onConversationInvalidatedRef = React.useRef(onConversationInvalidated);
  const invalidationTimerRef = React.useRef<number | null>(null);
  React.useLayoutEffect(() => {
    onExecutionBoundaryRef.current = onExecutionBoundary;
    onConversationInvalidatedRef.current = onConversationInvalidated;
  }, [onConversationInvalidated, onExecutionBoundary]);

  React.useEffect(() => {
    setAgentRunContext(contextKey, normalizedConversationID);
    if (!normalizedConversationID) return;
    const hydratedRunIDs = runIDKey ? runIDKey.split(",") : [];

    let cancelled = false;
    let accessToken = "";
    let eventSync: Promise<void> | null = null;
    let eventSyncRequested = false;
    let interactionSync: Promise<void> | null = null;
    let interactionSyncRequested = false;
    let eventRetryTimer: number | null = null;
    let eventRetryDelay = 1_000;
    let executionCursor = 0;
    let interactionRetryTimer: number | null = null;
    let interactionRetryDelay = 1_000;
    let latestSyncTimer: number | null = null;

    const syncEvents = () => {
      eventSyncRequested = true;
      if (eventSync) return eventSync;
      eventSync = (async () => {
        while (eventSyncRequested && !cancelled) {
          eventSyncRequested = false;
          while (!cancelled) {
            const historical = executionCursor === 0;
            const page = await listConversationExecutionEvents(
              accessToken,
              normalizedConversationID,
              executionCursor,
              historical ? hydratedRunIDs : [],
            );
            if (cancelled) break;
            const sortedEvents = page.events.slice().sort((left, right) => left.seq - right.seq);
            applyAgentExecutionEvents(sortedEvents, normalizedConversationID);
            for (const event of sortedEvents) {
              if (!historical && (event.kind === "turn/started" || event.kind === "turn/completed")) {
                onExecutionBoundaryRef.current?.(event);
              }
            }
            executionCursor = Math.max(executionCursor, page.cursor);
            if (!page.hasMore) break;
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
    const requestLatestSync = () => {
      if (latestSyncTimer !== null) return;
      latestSyncTimer = window.setTimeout(() => {
        latestSyncTimer = null;
        syncLatest();
      }, LATEST_SYNC_DEBOUNCE_MS);
    };
    requestLatestSyncRef.current = requestLatestSync;

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") syncLatest();
    };

    async function start() {
      accessToken = (await resolveAccessToken()) ?? "";
      if (!accessToken || cancelled) return;
      syncLatest();
      document.addEventListener("visibilitychange", onVisibilityChange);
    }

    void start();
    return () => {
      cancelled = true;
      requestExecutionSyncRef.current = null;
      requestLatestSyncRef.current = null;
      document.removeEventListener("visibilitychange", onVisibilityChange);
      if (eventRetryTimer !== null) window.clearTimeout(eventRetryTimer);
      if (interactionRetryTimer !== null) window.clearTimeout(interactionRetryTimer);
      if (latestSyncTimer !== null) window.clearTimeout(latestSyncTimer);
    };
  }, [contextKey, normalizedConversationID, runIDKey]);

  React.useEffect(() => () => {
    if (invalidationTimerRef.current !== null) {
      window.clearTimeout(invalidationTimerRef.current);
      invalidationTimerRef.current = null;
    }
  }, [normalizedConversationID]);

  React.useEffect(() => {
    if (!agentEvent || !normalizedConversationID) return;
    const normalizedDeviceID = deviceID.trim();
    const targetsCurrentConversation = agentEvent.type === "change" &&
      agentEvent.conversationIDs.includes(normalizedConversationID);
    if (agentEvent.type !== "ready" && !targetsCurrentConversation) {
      return;
    }
    requestLatestSyncRef.current?.();
    const shouldRestoreConversationSnapshot = agentEvent.type === "ready" || (
      agentEvent.kind === THREAD_HISTORY_EVENT_KIND &&
      (!normalizedDeviceID || agentEvent.deviceID === normalizedDeviceID) &&
      targetsCurrentConversation
    );
    if (shouldRestoreConversationSnapshot && invalidationTimerRef.current === null) {
      invalidationTimerRef.current = window.setTimeout(() => {
        invalidationTimerRef.current = null;
        onConversationInvalidatedRef.current?.();
      }, HISTORY_INVALIDATION_DEBOUNCE_MS);
    }
  }, [agentEvent, deviceID, normalizedConversationID]);
}
