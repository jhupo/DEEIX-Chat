import type { ChatAreaMessage, MessageAttachment } from "@/features/chat/types/messages";
import type { MessageDTO, UpstreamDebugInfo } from "@/shared/api/conversation.types";

function parseAttachmentDurationSeconds(value: unknown): number | undefined {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return undefined;
  }
  return Math.ceil(parsed);
}

export function parseAttachments(raw: string): MessageAttachment[] {
  if (!raw) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return (parsed as Record<string, unknown>[])
      .map((item) => ({
        fileID: String(item.file_id ?? ""),
        fileName: String(item.file_name ?? ""),
        mimeType: String(item.mime_type ?? ""),
        detectedMime: String(item.detected_mime ?? ""),
        fileCategory: String(item.file_category ?? ""),
        sizeBytes: Number(item.file_size ?? 0),
        durationSeconds: parseAttachmentDurationSeconds(item.duration_seconds),
        kind: item.kind === "image" ? ("image" as const) : ("file" as const),
        processingStatus: String(item.processing_status ?? ""),
        processingReady: Boolean(item.processing_ready),
        processingErrorCode: String(item.processing_error_code ?? ""),
        processingErrorMessage: String(item.processing_error_message ?? ""),
      }))
      .filter((item) => item.fileID && item.fileName);
  } catch {
    return [];
  }
}

const ROOT_BRANCH_KEY = "__root__";

type MessageLabels = {
  generationInterrupted: string;
  streamInterrupted?: string;
  imageRunning?: string;
  resolveErrorMessage?: (errorCode: string, fallback: string, details?: UpstreamDebugInfo) => string;
};

function resolveAssistantErrorMessage(item: MessageDTO, labels: MessageLabels): string {
  const fallback = item.errorMessage.trim();
  if (item.errorCode === "stream_interrupted" || item.errorCode === "conversation_run.stream_interrupted") {
    return labels.streamInterrupted || fallback;
  }
  const errorCode = item.errorCode.trim();
  if (errorCode && labels.resolveErrorMessage) {
    return labels.resolveErrorMessage(errorCode, fallback);
  }
  return fallback;
}

export function mapServerMessage(
  item: MessageDTO,
  labels: MessageLabels = {
    generationInterrupted: "Generation interrupted",
  },
  options: { liveRunIDs?: ReadonlySet<string> } = {},
): ChatAreaMessage {
  const publicID = item.publicID.trim();
  const msg: ChatAreaMessage = {
    key: `server-${publicID}`,
    publicID,
    parentPublicID: item.parentPublicID?.trim() || null,
    sourcePublicID: item.sourcePublicID?.trim() || null,
    role: item.role === "assistant" ? "assistant" : item.role === "system" ? "system" : "user",
    contentType: item.contentType,
    content: item.content,
    branchReason: item.branchReason || "default",
    status: item.status || "success",
    runID: item.runID || undefined,
    platformModelName: item.platformModelName?.trim() || undefined,
    serverMessageID: item.id,
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
    editedAt: item.editedAt ?? null,
    myFeedback: item.myFeedback || null,
    thumbsUpCount: item.thumbsUpCount ?? 0,
    thumbsDownCount: item.thumbsDownCount ?? 0,
  };
  const parsedAttachments = parseAttachments(item.attachments);
  if (parsedAttachments.length > 0) {
    msg.attachments = parsedAttachments;
  }
  if (item.role === "user") {
    msg.inputTokens = item.inputTokens ?? 0;
    msg.cacheReadTokens = item.cacheReadTokens ?? 0;
    msg.cacheWriteTokens = item.cacheWriteTokens ?? 0;
  }
  if (item.role === "assistant") {
    msg.inputTokens = item.inputTokens ?? 0;
    msg.outputTokens = item.outputTokens ?? 0;
    msg.cacheReadTokens = item.cacheReadTokens ?? 0;
    msg.cacheWriteTokens = item.cacheWriteTokens ?? 0;
    msg.reasoningTokens = item.reasoningTokens ?? 0;
    msg.latencyMS = item.latencyMS ?? 0;
    if ((item.status === "error" || item.status === "interrupted") && item.errorMessage?.trim()) {
      msg.inlineAlert = {
        title: labels.generationInterrupted,
        message: resolveAssistantErrorMessage(item, labels),
      };
    }
    if (item.status === "pending") {
      const liveRunID = item.runID?.trim() || "";
      const live = Boolean(liveRunID && options.liveRunIDs?.has(liveRunID));
      msg.isPending = live;
      msg.isStreaming = live;
      msg.activityLabel = live && item.contentType === "image" ? labels.imageRunning : undefined;
    }
  }
  return msg;
}

export function toBranchKey(publicID?: string | null): string {
  return publicID?.trim() || ROOT_BRANCH_KEY;
}

export function buildChildrenIndex(messages: ChatAreaMessage[]) {
  const children = new Map<string, ChatAreaMessage[]>();
  for (const item of messages) {
    const parentKey = toBranchKey(item.parentPublicID);
    const siblings = children.get(parentKey) ?? [];
    siblings.push(item);
    children.set(parentKey, siblings);
  }
  return children;
}

export function reconcileBranchSelections(messages: ChatAreaMessage[], previous: Record<string, string>) {
  const next: Record<string, string> = {};
  const children = buildChildrenIndex(messages);
  const messagesByPublicID = new Map<string, ChatAreaMessage>();
  let latestPublicID = "";

  for (const item of messages) {
    const publicID = item.publicID.trim();
    if (publicID) {
      messagesByPublicID.set(publicID, item);
      latestPublicID = publicID;
    }
  }

  const visited = new Set<string>();
  let current = latestPublicID ? messagesByPublicID.get(latestPublicID) ?? null : null;

  while (current) {
    const publicID = current.publicID.trim();
    if (!publicID || visited.has(publicID)) {
      break;
    }
    visited.add(publicID);
    next[toBranchKey(current.parentPublicID)] = publicID;

    const parentPublicID = current.parentPublicID?.trim() || "";
    current = parentPublicID ? messagesByPublicID.get(parentPublicID) ?? null : null;
  }

  for (const [parentKey, siblings] of children.entries()) {
    const existing = previous[parentKey];
    if (existing && siblings.some((item) => item.publicID === existing)) {
      next[parentKey] = existing;
      continue;
    }
    if (next[parentKey]) {
      continue;
    }
    const latest = siblings[siblings.length - 1];
    if (latest) {
      next[parentKey] = latest.publicID;
    }
  }
  return next;
}

export function buildVisibleMessages(
  messages: ChatAreaMessage[],
  selections: Record<string, string>,
): ChatAreaMessage[] {
  const children = buildChildrenIndex(messages);
  const reconciledSelections = reconcileBranchSelections(messages, selections);
  let visible: ChatAreaMessage[] = [];
  const visited = new Set<string>();
  let parentKey = ROOT_BRANCH_KEY;

  while (true) {
    const siblings = children.get(parentKey);
    if (!siblings || siblings.length === 0) {
      break;
    }

    const selectedPublicID = reconciledSelections[parentKey] || siblings[siblings.length - 1]?.publicID;
    const selected = siblings.find((item) => item.publicID === selectedPublicID) ?? siblings[siblings.length - 1];
    if (!selected || visited.has(selected.publicID)) {
      break;
    }

    visited.add(selected.publicID);
    visible.push(selected);
    parentKey = selected.publicID;
  }

  if (visible.length === 0 && messages.length > 0) {
    visible = buildTailVisibleMessages(messages);
  }

  const withBranchNavigators = visible.map((item) => {
    if (item.role !== "user" && item.role !== "assistant") {
      return item;
    }
    const siblings = children.get(toBranchKey(item.parentPublicID)) ?? [];
    if (siblings.length <= 1) {
      return item;
    }
    const currentIndex = siblings.findIndex((candidate) => candidate.publicID === item.publicID);
    if (currentIndex < 0) {
      return item;
    }
    return {
      ...item,
      branchNavigator: {
        parentPublicID: item.parentPublicID,
        index: currentIndex + 1,
        total: siblings.length,
        canPrevious: currentIndex > 0,
        canNext: currentIndex < siblings.length - 1,
      },
    };
  });

  return withBranchNavigators.map((item, index) => {
    if (item.role !== "assistant") {
      return item;
    }
    // Assistant-only retries reuse the original user message, but own the
    // prompt-side usage for their generation. A zero value is authoritative
    // and must not fall back to the reused user's first-run usage.
    if (item.branchReason === "retry" && item.sourcePublicID?.trim()) {
      return item;
    }
    const previous = index > 0 ? withBranchNavigators[index - 1] : null;
    if (!previous || previous.role !== "user") {
      return item;
    }
    return {
      ...item,
      inputTokens: item.inputTokens && item.inputTokens > 0 ? item.inputTokens : previous.inputTokens,
      cacheReadTokens: item.cacheReadTokens && item.cacheReadTokens > 0 ? item.cacheReadTokens : previous.cacheReadTokens,
      cacheWriteTokens: item.cacheWriteTokens && item.cacheWriteTokens > 0 ? item.cacheWriteTokens : previous.cacheWriteTokens,
    };
  });
}

function buildTailVisibleMessages(messages: ChatAreaMessage[]): ChatAreaMessage[] {
  const byPublicID = new Map(messages.map((item) => [item.publicID, item]));
  const visible: ChatAreaMessage[] = [];
  const visited = new Set<string>();
  let current = messages.at(-1) ?? null;

  while (current && !visited.has(current.publicID)) {
    visited.add(current.publicID);
    visible.push(current);
    const parentPublicID = current.parentPublicID?.trim() || "";
    current = parentPublicID ? byPublicID.get(parentPublicID) ?? null : null;
  }

  return visible.reverse();
}
