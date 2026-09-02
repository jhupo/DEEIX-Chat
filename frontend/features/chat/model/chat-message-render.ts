import type {
  ChatAreaMessage,
  ChatInlineAlert,
  ChatMessageBranchNavigator,
  MessageAttachment,
} from "@/features/chat/types/messages";

function areBranchNavigatorsEqual(
  previous: ChatMessageBranchNavigator | undefined,
  next: ChatMessageBranchNavigator | undefined,
) {
  if (previous === next) return true;
  if (!previous || !next) return false;
  return (
    previous.parentPublicID === next.parentPublicID &&
    previous.index === next.index &&
    previous.total === next.total &&
    previous.canPrevious === next.canPrevious &&
    previous.canNext === next.canNext
  );
}

function areInlineAlertsEqual(
  previous: ChatInlineAlert | undefined,
  next: ChatInlineAlert | undefined,
) {
  if (previous === next) return true;
  if (!previous || !next) return false;
  return previous.title === next.title && previous.message === next.message;
}

function areAttachmentsEqual(
  previous: MessageAttachment[] | undefined,
  next: MessageAttachment[] | undefined,
) {
  if (previous === next) return true;
  if (!previous || !next || previous.length !== next.length) return false;

  return previous.every((item, index) => {
    const nextItem = next[index];
    return (
      item.fileID === nextItem.fileID &&
      item.fileName === nextItem.fileName &&
      item.mimeType === nextItem.mimeType &&
      item.detectedMime === nextItem.detectedMime &&
      item.fileCategory === nextItem.fileCategory &&
      item.sizeBytes === nextItem.sizeBytes &&
      item.durationSeconds === nextItem.durationSeconds &&
      item.kind === nextItem.kind &&
      item.previewURL === nextItem.previewURL &&
      item.processingStatus === nextItem.processingStatus &&
      item.processingReady === nextItem.processingReady &&
      item.processingErrorCode === nextItem.processingErrorCode &&
      item.processingErrorMessage === nextItem.processingErrorMessage &&
      item.extractStatus === nextItem.extractStatus &&
      item.embedStatus === nextItem.embedStatus &&
      item.ragReady === nextItem.ragReady &&
      item.ragReason === nextItem.ragReason &&
      item.ocrUsed === nextItem.ocrUsed
    );
  });
}

function areCompactDoneEqual(
  previous: ChatAreaMessage["compactDone"],
  next: ChatAreaMessage["compactDone"],
) {
  if (previous === next) return true;
  if (!previous || !next) return false;
  return (
    previous.method === next.method &&
    previous.freed_tokens === next.freed_tokens &&
    previous.summary_preview === next.summary_preview
  );
}

export function areChatAreaMessagesRenderEqual(
  previous: ChatAreaMessage,
  next: ChatAreaMessage,
) {
  return (
    previous.key === next.key &&
    previous.publicID === next.publicID &&
    previous.parentPublicID === next.parentPublicID &&
    previous.sourcePublicID === next.sourcePublicID &&
    previous.role === next.role &&
    previous.contentType === next.contentType &&
    previous.content === next.content &&
    previous.branchReason === next.branchReason &&
    previous.status === next.status &&
    previous.runID === next.runID &&
    previous.platformModelName === next.platformModelName &&
    previous.serverMessageID === next.serverMessageID &&
    previous.createdAt === next.createdAt &&
    previous.updatedAt === next.updatedAt &&
    previous.editedAt === next.editedAt &&
    previous.isPending === next.isPending &&
    previous.isStreaming === next.isStreaming &&
    previous.isFileProc === next.isFileProc &&
    previous.activityLabel === next.activityLabel &&
    previous.imageAspectRatio === next.imageAspectRatio &&
    previous.myFeedback === next.myFeedback &&
    previous.thumbsUpCount === next.thumbsUpCount &&
    previous.thumbsDownCount === next.thumbsDownCount &&
    previous.inputTokens === next.inputTokens &&
    previous.outputTokens === next.outputTokens &&
    previous.cacheReadTokens === next.cacheReadTokens &&
    previous.cacheWriteTokens === next.cacheWriteTokens &&
    previous.reasoningTokens === next.reasoningTokens &&
    previous.latencyMS === next.latencyMS &&
    areBranchNavigatorsEqual(previous.branchNavigator, next.branchNavigator) &&
    areAttachmentsEqual(previous.attachments, next.attachments) &&
    areInlineAlertsEqual(previous.inlineAlert, next.inlineAlert) &&
    areCompactDoneEqual(previous.compactDone, next.compactDone)
  );
}
