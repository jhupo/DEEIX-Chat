import type { ProcessTraceLabels } from "@/features/chat/hooks/use-process-trace-labels";
import type { ChatPromptTrace, ChatTraceEvent, RAGCitation } from "@/features/chat/types/messages";

export const TRACE_KIND_CONTEXT_PLANNING = "context_planning";
export const TRACE_KIND_RAG = "content_retrieval";
export const TRACE_KIND_FILE_CONTEXT = "file_context";
export const TRACE_KIND_CONTEXT_COMPACTION = "context_compaction";
export const TRACE_KIND_SKILL_CONTEXT = "skill_context";
export const TRACE_KIND_MCP_ATTACHMENT_PROCESSOR = "mcp_attachment_processor";

export type FileContextBadge = {
  fileID?: string;
  name: string;
  label: string;
  description?: string;
  tab: "extract" | "preview";
};

export type TraceStage = {
  label: string;
  kind?: string;
  status?: string;
  trigger: string;
  detail: string;
  details: string[];
};

export type TraceDisplayEvent = {
  event: ChatTraceEvent;
  kind: "think" | "tool";
};

export function parseRAGCitations(payloadJson: string | undefined): RAGCitation[] {
  if (!payloadJson) return [];
  try {
    const parsed = JSON.parse(payloadJson) as { citations?: RAGCitation[] };
    return Array.isArray(parsed.citations) ? parsed.citations : [];
  } catch {
    return [];
  }
}

function readStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => (typeof item === "string" ? item.trim() : "")).filter(Boolean);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function readString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function readNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function firstStringFromRecord(record: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = readString(record[key]);
    if (value) return value;
  }
  return "";
}

function parseTracePayload(payloadJson: string | undefined): Record<string, unknown> | null {
  if (!payloadJson) return null;
  try {
    const parsed = JSON.parse(payloadJson) as unknown;
    return isRecord(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function formatFileContextCounts(
  counts: { included: number; skipped: number },
  labels: ProcessTraceLabels,
  includedDetail: boolean,
): string {
  const parts = [];
  if (counts.included > 0 || counts.skipped === 0) {
    parts.push(
      includedDetail
        ? labels.fileContext.includedDetail(counts.included)
        : labels.fileContext.includedSummary(counts.included),
    );
  }
  if (counts.skipped > 0) {
    parts.push(labels.fileContext.skipped(counts.skipped));
  }
  return parts.join(labels.fileContext.separator);
}

function readFileContextBadges(
  value: unknown,
  label: string,
  description: string,
  tab: "extract" | "preview",
): FileContextBadge[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((item) => {
    if (typeof item === "string") {
      const name = item.trim();
      return name ? [{ name, label, description, tab }] : [];
    }
    if (!isRecord(item)) return [];
    const fileID = firstStringFromRecord(item, ["file_id", "fileID", "id"]);
    const name = firstStringFromRecord(item, ["file_name", "fileName", "name", "title"]) || fileID;
    return name ? [{ fileID, name, label, description, tab }] : [];
  });
}

export function parseFileContextBadges(payloadJson: string | undefined, labels: ProcessTraceLabels): FileContextBadge[] {
  if (!payloadJson) return [];
  try {
    const parsed = JSON.parse(payloadJson) as {
      file_names?: string[];
      file_refs?: unknown[];
      file_groups?: Record<string, unknown>;
      file_group_refs?: Record<string, unknown>;
    };
    const groups = parsed.file_group_refs ?? parsed.file_groups ?? {};
    const badges = [
      ...readFileContextBadges(groups.direct_images, labels.fileBadges.directRead, labels.fileBadges.descriptions.directRead, "preview"),
      ...readFileContextBadges(groups.adaptive, labels.fileBadges.budget, labels.fileBadges.descriptions.budget, "extract"),
      ...readFileContextBadges(groups.retrieval, labels.fileBadges.retrieval, labels.fileBadges.descriptions.retrieval, "extract"),
      ...readFileContextBadges(
        groups.full_context,
        labels.fileBadges.fullContext,
        labels.fileBadges.descriptions.fullContext,
        "extract",
      ),
      ...readFileContextBadges(groups.skipped, labels.fileBadges.skipped, labels.fileBadges.descriptions.skipped, "extract"),
    ];
    if (badges.length > 0) return badges;
    const refs = readFileContextBadges(parsed.file_refs, labels.fileBadges.file, labels.fileBadges.descriptions.file, "extract");
    if (refs.length > 0) return refs;
    return readStringArray(parsed.file_names).map((name) => ({
      name,
      label: labels.fileBadges.file,
      description: labels.fileBadges.descriptions.file,
      tab: "extract",
    }));
  } catch {
    return [];
  }
}

function readTraceStagePayloads(payloadJson: string | undefined): Record<string, unknown>[] {
  const parsed = parseTracePayload(payloadJson);
  if (!parsed) return [];
  const stages = parsed.trace_stages;
  if (Array.isArray(stages)) {
    return stages.filter(isRecord);
  }
  return isRecord(parsed.trace_stage) ? [parsed.trace_stage] : [];
}

function traceStageKind(stage: TraceStage): string {
  return stage.kind ?? stage.label;
}

export function isContextPlanningTraceStage(stage: TraceStage): boolean {
  return traceStageKind(stage) === TRACE_KIND_CONTEXT_PLANNING;
}

export function isRAGTraceStage(stage: TraceStage): boolean {
  return traceStageKind(stage) === TRACE_KIND_RAG;
}

export function isFileContextTraceStage(stage: TraceStage): boolean {
  return traceStageKind(stage) === TRACE_KIND_FILE_CONTEXT;
}

export function isTraceStageError(stage: TraceStage): boolean {
  return ["error", "failed"].includes(stage.status?.trim() ?? "");
}

function structuredFileContextDetail(stage: Record<string, unknown>, labels: ProcessTraceLabels): string {
  const included = readNumber(stage.included_count) ?? 0;
  const skipped = readNumber(stage.skipped_count) ?? 0;
  return labels.fileContext.ready(formatFileContextCounts({ included, skipped }, labels, true));
}

function structuredRAGDetail(stage: Record<string, unknown>, labels: ProcessTraceLabels): string {
  const status = readString(stage.status);
  const fallback = readString(stage.fallback);
  const fileCount = readNumber(stage.file_count) ?? 0;
  const chunkCount = readNumber(stage.chunk_count) ?? 0;
  const hasFullText = fallback === "full_text";

  switch (status) {
    case "completed":
      return labels.rag.completed(fileCount, chunkCount);
    case "empty":
      return hasFullText ? labels.rag.emptyWithFullText : labels.rag.emptyNoFullText;
    case "low_score":
      return hasFullText ? labels.rag.lowScoreWithFullText : labels.rag.lowScoreNoFullText;
    case "skipped":
      return labels.rag.skippedFallback;
    default:
      return hasFullText ? labels.rag.incompleteWithFullText : labels.rag.incompleteNoFullText;
  }
}

function structuredCompactionDetails(stage: Record<string, unknown>, labels: ProcessTraceLabels): string[] {
  const fromTurn = readNumber(stage.from_turn) ?? 0;
  const toTurn = readNumber(stage.to_turn) ?? 0;
  const sourceTokens = readNumber(stage.source_tokens) ?? 0;
  const summaryTokens = readNumber(stage.summary_tokens) ?? 0;
  return [
    labels.compaction.detail,
    labels.compaction.range(fromTurn, toTurn),
    labels.compaction.tokens(sourceTokens, summaryTokens),
  ];
}

function structuredTraceStageDetails(stage: Record<string, unknown>, labels: ProcessTraceLabels): string[] {
  switch (readString(stage.kind)) {
    case TRACE_KIND_FILE_CONTEXT:
      return [structuredFileContextDetail(stage, labels)];
    case TRACE_KIND_RAG:
      return [structuredRAGDetail(stage, labels)];
    case TRACE_KIND_CONTEXT_COMPACTION:
      return structuredCompactionDetails(stage, labels);
    case TRACE_KIND_SKILL_CONTEXT:
      return readStringArray(stage.skill_titles);
    case TRACE_KIND_MCP_ATTACHMENT_PROCESSOR:
      return readStringArray(stage.file_names);
    default:
      return [];
  }
}

export function parseStructuredTraceStages(payloadJson: string | undefined, labels: ProcessTraceLabels): TraceStage[] {
  return readTraceStagePayloads(payloadJson)
    .flatMap((payload) => {
      const kind = readString(payload.kind);
      if (!kind) return [];
      const details = structuredTraceStageDetails(payload, labels);
      if (details.length === 0) return [];
      return [
        {
          label: readString(payload.label) || kind,
          kind,
          status: readString(payload.status),
          trigger: "",
          detail: details.join("\n"),
          details,
        },
      ];
    })
    .filter((stage, index, stages) => {
      const previous = stages[index - 1];
      return !previous || previous.kind !== stage.kind || previous.status !== stage.status || previous.detail !== stage.detail;
    });
}

function structuredProcessSummaryFromPayload(payloadJson: string | undefined, labels: ProcessTraceLabels): string {
  const stages = readTraceStagePayloads(payloadJson);
  const last = [...stages].reverse().find((stage) => readString(stage.kind));
  if (!last) return "";
  const kind = readString(last.kind);
  if (kind === TRACE_KIND_FILE_CONTEXT) {
    const included = readNumber(last.included_count) ?? 0;
    const skipped = readNumber(last.skipped_count) ?? 0;
    return formatFileContextCounts({ included, skipped }, labels, false);
  }
  if (kind === TRACE_KIND_RAG) {
    const status = readString(last.status);
    const fallback = readString(last.fallback);
    const hasFullText = fallback === "full_text";
    if (status === "completed") {
      return labels.rag.summary(readNumber(last.chunk_count) ?? 0);
    }
    if (status === "empty") {
      return hasFullText ? labels.rag.emptyWithFullText : labels.rag.emptyNoFullText;
    }
    if (status === "low_score") {
      return hasFullText ? labels.rag.lowScoreWithFullText : labels.rag.lowScoreNoFullText;
    }
    if (status === "skipped") {
      return labels.rag.skippedFallback;
    }
    return hasFullText ? labels.rag.incompleteWithFullText : labels.rag.incompleteNoFullText;
  }
  if (kind === TRACE_KIND_CONTEXT_COMPACTION) {
    const fromTurn = readNumber(last.from_turn);
    const toTurn = readNumber(last.to_turn);
    if (fromTurn !== null && toTurn !== null) {
      return labels.compaction.summary(fromTurn, toTurn);
    }
  }
  return "";
}

export function normalizeTraceListItem(text: string): string {
  return text.replace(/^[-*]\s+/, "").trim();
}

export function localizeProcessSummary(summary: string, payloadJson: string | undefined, labels: ProcessTraceLabels): string {
  const structuredSummary = structuredProcessSummaryFromPayload(payloadJson, labels);
  return structuredSummary || summary.trim();
}

export function displayTraceStageLabel(label: string, labels: ProcessTraceLabels): string {
  switch (label) {
    case TRACE_KIND_CONTEXT_PLANNING:
      return labels.stages.contextPlanning;
    case TRACE_KIND_RAG:
      return labels.stages.contentRetrieval;
    case TRACE_KIND_FILE_CONTEXT:
      return labels.stages.fileContext;
    case TRACE_KIND_CONTEXT_COMPACTION:
      return labels.stages.contextCompaction;
    case TRACE_KIND_SKILL_CONTEXT:
      return labels.stages.skillContext;
    default:
      return label;
  }
}

function promptTraceModeSentence(mode: string, labels: ProcessTraceLabels): string {
  switch (mode.trim()) {
    case "stateful":
      return labels.promptTrace.modes.stateful;
    case "full_retry":
      return labels.promptTrace.modes.fullRetry;
    default:
      return labels.promptTrace.modes.full;
  }
}

function promptTraceReasonLabel(reason: string, labels: ProcessTraceLabels): string {
  switch (reason.trim()) {
    case "":
      return "";
    case "route_or_branch_not_eligible":
      return "";
    case "missing_stored_fingerprint":
      return labels.promptTrace.reasons.missingStoredFingerprint;
    case "missing_current_fingerprint":
      return labels.promptTrace.reasons.missingCurrentFingerprint;
    case "prompt_fingerprint_mismatch":
      return labels.promptTrace.reasons.fingerprintMismatch;
    case "previous_response_rejected":
      return labels.promptTrace.reasons.previousRejected;
    default:
      return reason;
  }
}

function promptTraceStage(trace: ChatPromptTrace | undefined, labels: ProcessTraceLabels): TraceStage | null {
  if (!trace) return null;
  const cacheableBlocks = trace.blocks.filter((block) => block.cacheable).length;
  const historicalEvidence = trace.blocks
    .filter((block) => block.kind === "historical_evidence")
    .reduce((total, block) => total + block.sourceCount, 0);
  const dynamicSources = trace.blocks
    .filter((block) => block.kind === "dynamic_context")
    .reduce((total, block) => total + block.sourceCount, 0);

  const details = [
    labels.promptTrace.sentSummary(
      promptTraceModeSentence(trace.mode, labels),
      trace.sentMessageCount,
      trace.fullMessageCount,
      trace.sentTokenEstimate,
    ),
  ];
  if (trace.statefulSavedMessages > 0 || trace.statefulSavedTokens > 0) {
    details.push(labels.promptTrace.savedHistory(trace.statefulSavedMessages, trace.statefulSavedTokens));
  }

  const extras = [];
  if (cacheableBlocks > 0) extras.push(labels.promptTrace.cacheableBlocks(cacheableBlocks));
  if (historicalEvidence > 0) extras.push(labels.promptTrace.historicalEvidence(historicalEvidence));
  if (dynamicSources > 0) extras.push(labels.promptTrace.dynamicSources(dynamicSources));
  if (extras.length > 0) {
    details.push(labels.promptTrace.extraSummary(extras.join(labels.promptTrace.listSeparator)));
  }

  const reason = promptTraceReasonLabel(trace.statefulDisabledReason, labels);
  if (reason && !trace.statefulUsed) {
    details.push(labels.promptTrace.reasonLine(reason));
  }

  return {
    label: TRACE_KIND_CONTEXT_PLANNING,
    kind: TRACE_KIND_CONTEXT_PLANNING,
    trigger: "",
    detail: details.join("\n"),
    details,
  };
}

export function mergePromptTraceStage(stages: TraceStage[], trace: ChatPromptTrace | undefined, labels: ProcessTraceLabels): TraceStage[] {
  const stage = promptTraceStage(trace, labels);
  if (!stage) {
    return stages;
  }
  let replaced = false;
  const result = stages.flatMap((item) => {
    if (!isContextPlanningTraceStage(item)) {
      return [item];
    }
    if (replaced) {
      return [];
    }
    replaced = true;
    return [stage];
  });
  return replaced ? result : [...result, stage];
}
