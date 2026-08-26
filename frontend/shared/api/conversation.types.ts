import type {
  BatchSetConversationProjectResponse,
  ContextArtifactResponse,
  BatchSetConversationProjectRequest as ContractBatchSetConversationProjectRequest,
  CreateConversationProjectRequest as ContractCreateConversationProjectRequest,
  CreateConversationRequest as ContractCreateConversationRequest,
  CreateConversationShareRequest as ContractCreateConversationShareRequest,
  InteractionResponse as ContractInteractionResponse,
  RenameConversationRequest as ContractRenameConversationRequest,
  ReorderConversationProjectsRequest as ContractReorderConversationProjectsRequest,
  RevokeConversationSharesRequest as ContractRevokeConversationSharesRequest,
  SendMessageRequest as ContractSendMessageRequest,
  SetConversationArchiveRequest as ContractSetConversationArchiveRequest,
  SetConversationProjectRequest as ContractSetConversationProjectRequest,
  SetConversationStarRequest as ContractSetConversationStarRequest,
  SetMessageFeedbackRequest as ContractSetMessageFeedbackRequest,
  UpdateConversationLabelsRequest as ContractUpdateConversationLabelsRequest,
  UpdateConversationProjectRequest as ContractUpdateConversationProjectRequest,
  UpdateMessageRequest as ContractUpdateMessageRequest,
  ConversationDefaultModelCandidateResponse,
  ConversationDeleteResponse,
  ConversationExportResponse,
  ConversationInputResourceCatalogResponse,
  ConversationInputResourceResponse,
  ConversationPreviewMessageResponse,
  ConversationProjectResponse,
  ConversationResponse,
  ConversationSearchPageResponse,
  ConversationSearchResultResponse,
  ConversationShareResponse,
  MessageFeedbackResponse,
  MessageResponse,
  ModelProbeDebugResponse,
  PublicSharedConversationResponse,
  PublicSharedMessageResponse,
  RevokeConversationSharesResponse,
  RunResponse,
  SendMessageResponse,
} from "@deeix/api-contract";
import type { UserStorageQuotaDTO } from "@/shared/api/file.types";

export type ConversationDTO = ConversationResponse;

export type ConversationInteractionKind =
  | "command_approval"
  | "file_approval"
  | "user_input"
  | "permission"
  | "mcp_elicitation"
  | "dynamic_tool";

export type ConversationInteractionStatus = "pending" | "responding" | "resolved" | "failed";

export type AgentInteractionQuestionOptionDTO = {
  label: string;
  description?: string;
};

export type AgentInteractionQuestionDTO = {
  questionRef: string;
  header?: string;
  label?: string;
  question?: string;
  prompt?: string;
  required?: boolean;
  allowFreeform?: boolean;
  secret?: boolean;
  options?: AgentInteractionQuestionOptionDTO[];
};

export type AgentInteractionSchemaFieldDTO = {
  type?: "string" | "number" | "integer" | "boolean";
  title?: string;
  description?: string;
  enum?: Array<string | number>;
};

type ConversationInteractionBaseDTO<K extends ConversationInteractionKind, R> = Omit<
  ContractInteractionResponse,
  "kind" | "request" | "status"
> & {
  kind: K;
  status: ConversationInteractionStatus;
  request: R;
};

export type ConversationInteractionDTO =
  | ConversationInteractionBaseDTO<"command_approval", {
      command?: string;
      reason?: string;
      risk?: string;
    }>
  | ConversationInteractionBaseDTO<"file_approval", {
      reason?: string;
      changes?: Array<{ path?: string; change?: string }>;
    }>
  | ConversationInteractionBaseDTO<"user_input", {
      questions?: AgentInteractionQuestionDTO[];
    }>
  | ConversationInteractionBaseDTO<"permission", {
      title?: string;
      description?: string;
      permissions?: string[] | { names?: string[] };
      allowedScopes?: Array<"turn" | "session">;
      highImpact?: boolean;
    }>
  | ConversationInteractionBaseDTO<"mcp_elicitation", {
      serverName?: string;
      message?: string;
      prompt?: string;
      requestedSchema?: {
        properties?: Record<string, AgentInteractionSchemaFieldDTO>;
        required?: string[];
      };
    }>
  | ConversationInteractionBaseDTO<"dynamic_tool", {
      tool?: string;
      argumentsPreview?: string;
      acceptedContentKinds?: Array<"text" | "image">;
    }>;

export type ConversationInteractionResponse =
  | { kind: "approval"; decision: "accept" | "decline" }
  | { kind: "user-input"; answers: Record<string, string> }
  | { kind: "permission"; decision: "accept" | "decline"; scope?: "turn" | "session" }
  | { kind: "mcp-elicitation"; decision: "accept" | "decline"; content?: Record<string, string | number | boolean> }
  | {
      kind: "dynamic-tool";
      success: boolean;
      content: Array<{ kind: "text"; text: string } | { kind: "image"; url: string }>;
    };

export type ConversationSearchResultDTO = ConversationSearchResultResponse;

export type ConversationSearchPageDTO = Omit<ConversationSearchPageResponse, "results"> & {
  results: ConversationSearchResultDTO[];
};

export type ConversationPreviewMessageDTO = ConversationPreviewMessageResponse;

export type ConversationDefaultModelCandidateDTO = ConversationDefaultModelCandidateResponse;

export type ConversationStatusFilter = "active" | "archived" | "all";
export type ConversationStarredFilter = "all" | "starred" | "unstarred";
export type ConversationShareFilter = "all" | "shared" | "unshared";
export type ConversationProjectFilter = "all" | "unassigned" | string;
export type ConversationProjectStatusFilter = "active" | "archived" | "all";
export type ConversationProjectMCPDefaultMode = "inherit" | "custom";

export type ConversationProjectDTO = Omit<ConversationProjectResponse, "mcpDefaultMode"> & {
  mcpDefaultMode: ConversationProjectMCPDefaultMode;
};

export type ConversationInputResourceDTO = Omit<ConversationInputResourceResponse, "kind"> & {
  kind: "skill" | "app-mention";
};

export type ConversationInputResourceCatalogDTO = Omit<ConversationInputResourceCatalogResponse, "items"> & {
  items: ConversationInputResourceDTO[];
};

export type MessageDTO = Omit<
  MessageResponse,
  | "modelIcon"
  | "modelVendor"
  | "platformModelName"
  | "upstreamModelName"
> & {
  branchReason: "default" | "retry" | "edit";
  platformModelName?: string;
  upstreamModelName?: string;
  modelVendor?: string;
  modelIcon?: string;
  myFeedback: "up" | "down" | "";
};

export type ConversationRunDTO = Omit<RunResponse, "taskType">;

export type ConversationExportDTO = Omit<
  ConversationExportResponse,
  "compatibility" | "conversation" | "messages" | "runs"
> & {
  conversation: ConversationDTO;
  messages: MessageDTO[];
  runs: ConversationRunDTO[];
  compatibility: ConversationExportResponse["compatibility"];
};

export type ContextArtifactDTO = ContextArtifactResponse;

export type AgentPlanStepDTO = {
  step: string;
  status: "pending" | "inProgress" | "completed";
};

export type AgentFileChangeDTO = {
  path: string;
  previousPath?: string;
  change: string;
  additions?: number;
  deletions?: number;
  binary?: boolean;
  diff?: string;
  truncated?: boolean;
};

export type AgentExecutionItemDTO = {
  itemID: string;
  kind: string;
  status: string;
  command?: string;
  cwd?: string;
  durationMs?: number;
  commandActions?: Array<Record<string, unknown>>;
  tool?: string;
  toolType?: string;
  arguments?: string;
  result?: string;
  output?: string;
  error?: string;
  exitCode?: number;
  text?: string;
  phase?: "commentary" | "final_answer" | string;
  summary?: string[];
  content?: string[];
  changes?: AgentFileChangeDTO[];
  diff?: string;
  truncated?: boolean;
};

export type AgentTokenUsageDTO = {
  inputTokens?: number;
  outputTokens?: number;
  cachedInputTokens?: number;
  reasoningTokens?: number;
  totalTokens?: number;
};

export type AgentExecutionEventPayloadDTO = {
  error?: { code?: string; message?: string } | string;
  turn?: { status?: string; durationMs?: number; error?: { code?: string; message?: string } | string };
  explanation?: string;
  plan?: AgentPlanStepDTO[];
  itemID?: string;
  item?: AgentExecutionItemDTO;
  delta?: string;
  phase?: "commentary" | "final_answer" | string;
  summaryIndex?: number;
  contentIndex?: number;
  outputDelta?: string;
  patch?: string;
  diff?: string;
  truncated?: boolean;
  changes?: AgentFileChangeDTO[];
  tokenUsage?: AgentTokenUsageDTO & { total?: AgentTokenUsageDTO; last?: AgentTokenUsageDTO };
  fromModel?: string;
  toModel?: string;
  reason?: string;
  interactionID?: string;
};

export type ConversationExecutionEventDTO = {
  runID: string;
  seq: number;
  kind: string;
  payload: AgentExecutionEventPayloadDTO;
  occurredAt: string;
};

export type ConversationExecutionEventPageDTO = {
  events: ConversationExecutionEventDTO[];
  cursor: number;
  hasMore: boolean;
};

export type CreateConversationRequest = ContractCreateConversationRequest;

export type CreateConversationProjectRequest = Omit<ContractCreateConversationProjectRequest, "mcpDefaultMode"> & {
  mcpDefaultMode?: ConversationProjectMCPDefaultMode;
};

export type UpdateConversationProjectRequest = Omit<ContractUpdateConversationProjectRequest, "mcpDefaultMode"> & {
  mcpDefaultMode?: ConversationProjectMCPDefaultMode;
};

export type ReorderConversationProjectsRequest = ContractReorderConversationProjectsRequest;

export type SetConversationProjectRequest = ContractSetConversationProjectRequest;

export type BatchSetConversationProjectRequest = ContractBatchSetConversationProjectRequest;

export type BatchSetConversationProjectResult = BatchSetConversationProjectResponse;

export type ConversationOptions = Record<string, unknown>;

export type UpstreamDebugInfo = ModelProbeDebugResponse;

export type RenameConversationRequest = ContractRenameConversationRequest;

export type UpdateConversationLabelsRequest = ContractUpdateConversationLabelsRequest;

export type SetConversationStarRequest = ContractSetConversationStarRequest;

export type SetConversationArchiveRequest = ContractSetConversationArchiveRequest;

export type DeleteConversationData = Omit<ConversationDeleteResponse, "quota"> & {
  quota?: UserStorageQuotaDTO;
};

export type CreateConversationShareRequest = ContractCreateConversationShareRequest;

export type ConversationShareDTO = ConversationShareResponse;

export type RevokeConversationSharesRequest = ContractRevokeConversationSharesRequest;

export type RevokeConversationSharesResult = RevokeConversationSharesResponse;

export type PublicSharedMessageDTO = PublicSharedMessageResponse;

export type PublicSharedConversationDTO = Omit<PublicSharedConversationResponse, "messages"> & {
  messages: PublicSharedMessageDTO[];
};

export type SetMessageFeedbackRequest = ContractSetMessageFeedbackRequest;

export type UpdateMessageRequest = ContractUpdateMessageRequest;

export type MessageFeedbackResult = Omit<MessageFeedbackResponse, "myFeedback"> & {
  myFeedback: "up" | "down" | "";
};

export type SendMessageRequest = Omit<ContractSendMessageRequest, "options"> & {
  options?: ConversationOptions;
  keyBindingID: string;
};

export type MediaImageRequest = {
  prompt: string;
  model?: string;
  options?: ConversationOptions;
  clientRunID?: string;
  fileIDs?: string[];
  maskFileID?: string;
  parentMessagePublicID?: string;
  sourceMessagePublicID?: string;
  branchReason?: "default" | "retry" | "edit";
};

export type MediaVideoRequest = {
  prompt: string;
  model?: string;
  options?: ConversationOptions;
  clientRunID?: string;
  fileIDs?: string[];
  parentMessagePublicID?: string;
  sourceMessagePublicID?: string;
  branchReason?: "default" | "retry" | "edit";
};

export type SendMessageResult = Omit<SendMessageResponse, "assistantMessage" | "metadataRefreshHint" | "userMessage"> & {
  userMessage: MessageDTO;
  assistantMessage: MessageDTO;
  metadataRefreshHint?: "pending" | "not_needed" | "skipped_no_titleable_content" | string;
};

export type StreamMessageEvent =
  | {
      type: "file_proc";
      seq?: number;
      message: string;
    }
  | {
      type: "rag_search";
      seq?: number;
      message: string;
    }
  | {
      type: "delta";
      seq?: number;
      delta: string;
    }
  | {
      type: "usage";
      seq?: number;
      input_tokens: number;
      output_tokens: number;
      cache_read_tokens: number;
      cache_write_tokens: number;
      reasoning_tokens: number;
    }
  | {
      type: "execution_event";
      seq?: number;
      executionSeq: number;
      runID: string;
      kind: string;
      payload: AgentExecutionEventPayloadDTO;
      occurredAt: string;
    }
  | {
      type: "media_status";
      seq?: number;
      status: string;
      message: string;
      content_type?: string;
    }
  | {
      type: "media_image_delta";
      seq?: number;
      index?: number;
      b64_json: string;
      mime_type?: string;
      revised_prompt?: string;
    }
  | {
      type: "completed";
      seq?: number;
      data: SendMessageResult;
    }
  | {
      type: "compact_done";
      seq?: number;
      method: string;
      freed_tokens: number;
      kept_turns: number;
      summary_preview: string;
    }
  | {
      type: "error";
      seq?: number;
      message: string;
      errorCode?: string;
      debug?: UpstreamDebugInfo;
      data?: SendMessageResult;
    };
