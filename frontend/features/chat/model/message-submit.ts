import type { ChatAreaMessage, MessageAttachment } from "@/features/chat/types/messages";
import type { PendingAttachment } from "@/features/chat/types/chat-runtime";

export function toPendingAttachments(message: ChatAreaMessage | null | undefined): PendingAttachment[] {
  if (!message?.attachments || message.attachments.length === 0) {
    return [];
  }
  return message.attachments.map(toPendingAttachment);
}

export function toPendingAttachment(item: MessageAttachment): PendingAttachment {
  return {
    fileID: item.fileID,
    fileName: item.fileName,
    mimeType: item.mimeType,
    detectedMime: item.detectedMime,
    fileCategory: item.fileCategory,
    sizeBytes: item.sizeBytes,
    previewURL: item.previewURL,
    processingStatus: item.processingStatus,
    processingReady: item.processingReady,
    processingErrorCode: item.processingErrorCode,
    processingErrorMessage: item.processingErrorMessage,
    extractStatus: item.extractStatus,
    embedStatus: item.embedStatus,
    ragReady: item.ragReady,
    ragReason: item.ragReason,
    ocrUsed: item.ocrUsed,
  };
}

export function resolvePersistedPublicID(value: string | null | undefined): string | null {
  const normalized = value?.trim() || "";
  if (!normalized || normalized.startsWith("local-exchange-")) {
    return null;
  }
  return normalized;
}

export function resolveAssistantInputSideUsageValue(
  assistantOwnsUsage: boolean,
  assistantValue: number | null | undefined,
  userValue: number | null | undefined,
  liveValue: number | null | undefined,
): number {
  if (assistantOwnsUsage) {
    return typeof assistantValue === "number" && Number.isFinite(assistantValue) && assistantValue >= 0
      ? assistantValue
      : 0;
  }
  for (const value of [assistantValue, userValue, liveValue]) {
    if (typeof value === "number" && Number.isFinite(value) && value > 0) {
      return value;
    }
  }
  return 0;
}

function isSuccessfulContextMessage(message: ChatAreaMessage): boolean {
  const status = message.status?.trim().toLowerCase() || "success";
  return (
    (status === "success" || (message.role === "assistant" && status === "interrupted")) &&
    !message.isPending &&
    !message.isStreaming &&
    resolvePersistedPublicID(message.publicID) !== null
  );
}

export function resolveDefaultSubmissionParentMessage(messages: ChatAreaMessage[]): ChatAreaMessage | null {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    if (message.role === "assistant" && isSuccessfulContextMessage(message)) {
      return message;
    }
  }
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    if (message.role === "user" && isSuccessfulContextMessage(message)) {
      return message;
    }
  }
  return null;
}
