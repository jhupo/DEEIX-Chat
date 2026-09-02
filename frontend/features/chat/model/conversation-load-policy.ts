import type { AgentRunStatus } from "@/features/chat/model/agent-run-store";

export function shouldSurfaceConversationLoadError(messageCount: number): boolean {
  return messageCount <= 0;
}

export function isConversationAgentRunActive(
  messageActive: boolean,
  messageStatus: string | null | undefined,
  agentRunStatus: AgentRunStatus,
): boolean {
  const status = messageStatus?.trim().toLowerCase();
  return messageActive || status === "pending" ||
    agentRunStatus === "running" || agentRunStatus === "waiting_interaction";
}

export function shouldRefreshMessagesAfterHistory(messageCount: number, initialHistoryStatus: string): boolean {
  return messageCount <= 0 || initialHistoryStatus !== "loaded";
}

export function shouldReloadMessagesForExecutionBoundary(
  kind: string,
  assistantStatus: string | null | undefined,
): boolean {
  const status = assistantStatus?.trim().toLowerCase();
  if (kind === "turn/completed") {
    return status === undefined || status === "pending";
  }
  return false;
}

export function isConversationStreamDisconnect(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  const name = "name" in error && typeof error.name === "string" ? error.name : "";
  return name === "ConversationStreamDisconnectError";
}

export function shouldRetryConversationStream(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  if (isConversationStreamDisconnect(error)) {
    return true;
  }
  const name = "name" in error && typeof error.name === "string" ? error.name : "";
  if (name === "ApiNetworkError" || name === "NetworkError") {
    return true;
  }
  const status = "status" in error && typeof error.status === "number" ? error.status : 0;
  const errorCode = "errorCode" in error && typeof error.errorCode === "string" ? error.errorCode : "";
  return [409, 429, 502, 503, 504].includes(status) || errorCode === "conversation_run.stream_interrupted";
}
