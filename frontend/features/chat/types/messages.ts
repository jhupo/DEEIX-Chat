import type { UpstreamDebugInfo } from "@/shared/api/conversation.types";

export type MessageAttachment = {
  fileID: string;
  fileName: string;
  mimeType: string;
  detectedMime?: string;
  fileCategory?: string;
  sizeBytes: number;
  durationSeconds?: number;
  kind: "file" | "image";
  previewURL?: string;
  processingStatus?: string;
  processingReady?: boolean;
  processingErrorCode?: string;
  processingErrorMessage?: string;
  extractStatus?: string;
  embedStatus?: string;
  ragReady?: boolean;
  ragReason?: string;
  ocrUsed?: boolean;
};

export type ChatMessageBranchNavigator = {
  parentPublicID: string | null;
  index: number;
  total: number;
  canPrevious: boolean;
  canNext: boolean;
};

export type ChatInlineAlert = {
  title: string;
  message: string;
  details?: UpstreamDebugInfo;
};

export type ImageLoadingAspectRatio = "wide" | "portrait" | "square";

export type ChatAreaMessage = {
  key: string;
  publicID: string;
  parentPublicID: string | null;
  sourcePublicID: string | null;
  role: "user" | "assistant" | "system";
  contentType?: string;
  content: string;
  branchReason: "default" | "retry" | "edit";
  status?: string;
  runID?: string;
  platformModelName?: string;
  serverMessageID?: number;
  createdAt?: string;
  updatedAt?: string;
  editedAt?: string | null;
  isPending?: boolean;
  isStreaming?: boolean;
  isFileProc?: boolean; // Active file_proc stream stage.
  activityLabel?: string;
  imageAspectRatio?: ImageLoadingAspectRatio;
  myFeedback?: "up" | "down" | null;
  thumbsUpCount?: number;
  thumbsDownCount?: number;
  branchNavigator?: ChatMessageBranchNavigator;
  attachments?: MessageAttachment[];
  // Token usage for assistant messages.
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  latencyMS?: number;
  inlineAlert?: ChatInlineAlert;
  compactDone?: { method: string; freed_tokens: number; summary_preview: string };
};
