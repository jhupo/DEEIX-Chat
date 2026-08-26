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
import type { AgentStreamEvent } from "@/shared/api/agent-gateway";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

const EXECUTION_EVENT_RETRY_MAX_MS = 15_000;
const INTERACTION_RETRY_MAX_MS = 15_000;
const SESSION_SNAPSHOT_EVENT_KIND = "workspace/sessions/updated";

type AgentRunHydrationScope = {
  conversationID: string | null;
  deviceID?: string;
  profileID?: string;
  workspaceID?: string;
  agentEvent?: AgentStreamEvent | null;
  onExecutionBoundary?: (event: ConversationExecutionEventDTO) => void;
  onConversationInvalidated?: () => void;
};

export function useAgentRunHydration({
  conversationID,
  deviceID = "",
  profileID = "",
  workspaceID = "",
  agentEvent = null,
  onExecutionBoundary,
  onConversationInvalidated,
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
  const requestLatestSyncRef = React.useRef<(() => void) | null>(null);
  const onExecutionBoundaryRef = React.useRef(onExecutionBoundary);
  const onConversationInvalidatedRef = React.useRef(onConversationInvalidated);
  React.useLayoutEffect(() => {
    onExecutionBoundaryRef.current = onExecutionBoundary;
    onConversationInvalidatedRef.current = onConversationInvalidated;
  }, [onConversationInvalidated, onExecutionBoundary]);

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
    requestLatestSyncRef.current = syncLatest;

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
    };
  }, [contextKey, normalizedConversationID]);

  React.useEffect(() => {
    if (!agentEvent || !normalizedConversationID) return;
    requestLatestSyncRef.current?.();
    if (
      agentEvent.type === "change" &&
      agentEvent.kind === SESSION_SNAPSHOT_EVENT_KIND &&
      agentEvent.deviceID === deviceID.trim() &&
      agentEvent.conversationIDs.includes(normalizedConversationID)
    ) {
      onConversationInvalidatedRef.current?.();
    }
  }, [agentEvent, deviceID, normalizedConversationID]);

  React.useEffect(() => {
    if (recovery.hasGap) requestExecutionSyncRef.current?.();
  }, [contextKey, recovery.hasGap, recovery.highestSeq]);
}
