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
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

const INTERACTION_POLL_INTERVAL_MS = 1_500;
const EXECUTION_EVENT_POLL_INTERVAL_MS = 3_000;
const EXECUTION_EVENT_RETRY_MAX_MS = 15_000;

type AgentRunHydrationScope = {
  conversationID: string | null;
  deviceID?: string;
  profileID?: string;
  workspaceID?: string;
};

export function useAgentRunHydration({
  conversationID,
  deviceID = "",
  profileID = "",
  workspaceID = "",
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

  React.useEffect(() => {
    setAgentRunContext(contextKey, normalizedConversationID);
    if (!normalizedConversationID) return;

    let cancelled = false;
    let accessToken = "";
    let eventSync: Promise<void> | null = null;
    let interactionTimer: number | null = null;
    let eventTimer: number | null = null;
    let eventRetryTimer: number | null = null;
    let eventRetryDelay = 1_000;

    const syncEvents = () => {
      if (eventSync) return eventSync;
      eventSync = (async () => {
        let cursor = getAgentExecutionRecoverySnapshot().contiguousSeq;
        while (!cancelled) {
          const events = await listConversationExecutionEvents(
            accessToken,
            normalizedConversationID,
            cursor,
          );
          if (cancelled || events.length === 0) return;
          let nextCursor = cursor;
          const sortedEvents = events.slice().sort((left, right) => left.seq - right.seq);
          applyAgentExecutionEvents(sortedEvents, normalizedConversationID);
          for (const event of sortedEvents) {
            nextCursor = Math.max(nextCursor, event.seq);
          }
          if (nextCursor <= cursor) return;
          cursor = nextCursor;
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

    const syncInteractions = async () => {
      const [pending, responding, failed] = await Promise.all([
        listConversationInteractions(accessToken, normalizedConversationID, "pending"),
        listConversationInteractions(accessToken, normalizedConversationID, "responding"),
        listConversationInteractions(accessToken, normalizedConversationID, "failed"),
      ]);
      if (!cancelled) replaceActiveAgentInteractions([...pending, ...responding, ...failed]);
    };

    async function start() {
      accessToken = (await resolveAccessToken()) ?? "";
      if (!accessToken || cancelled) return;
      requestExecutionSync();
      void syncInteractions().catch(() => undefined);
      eventTimer = window.setInterval(requestExecutionSync, EXECUTION_EVENT_POLL_INTERVAL_MS);
      interactionTimer = window.setInterval(() => {
        void syncInteractions().catch(() => undefined);
      }, INTERACTION_POLL_INTERVAL_MS);
    }

    void start();
    return () => {
      cancelled = true;
      requestExecutionSyncRef.current = null;
      if (interactionTimer !== null) window.clearInterval(interactionTimer);
      if (eventTimer !== null) window.clearInterval(eventTimer);
      if (eventRetryTimer !== null) window.clearTimeout(eventRetryTimer);
    };
  }, [contextKey, normalizedConversationID]);

  React.useEffect(() => {
    if (recovery.hasGap) requestExecutionSyncRef.current?.();
  }, [contextKey, recovery.hasGap, recovery.highestSeq]);
}
