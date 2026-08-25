"use client";

import * as React from "react";

import type {
  AgentExecutionEventPayloadDTO,
  AgentExecutionItemDTO,
  AgentFileChangeDTO,
  AgentTokenUsageDTO,
  ConversationExecutionEventDTO,
  ConversationInteractionDTO,
  ConversationInteractionStatus,
} from "@/shared/api/conversation.types";

export type AgentRunStatus = "idle" | "running" | "waiting_interaction" | "completed" | "interrupted" | "failed";
export type AgentActivityStatus = "running" | "completed" | "interrupted" | "failed";

export type AgentPlanStep = {
  key: string;
  text: string;
  status: "pending" | "inProgress" | "completed";
};

export type AgentCommandActivity = {
  itemID: string;
  seq: number;
  kind: "command";
  status: AgentActivityStatus;
  command: string;
  cwd: string;
  durationMS: number | null;
  commandActions: Array<Record<string, unknown>>;
  output: string;
  outputTruncated: boolean;
  exitCode: number | null;
};

export type AgentTextActivity = {
  itemID: string;
  seq: number;
  kind: "commentary" | "reasoning";
  status: AgentActivityStatus;
  text: string;
  truncated: boolean;
};

export type AgentFileChange = {
  fileID: string;
  path: string;
  previousPath: string;
  change: string;
  additions: number | null;
  deletions: number | null;
  binary: boolean;
  diff: string;
  truncated: boolean;
};

export type AgentFileActivity = {
  itemID: string;
  seq: number;
  kind: "file";
  status: AgentActivityStatus;
  files: AgentFileChange[];
  diff: string;
  truncated: boolean;
};

export type AgentUsage = Required<AgentTokenUsageDTO> & { scope: "thread" };
export type AgentActivityItem = AgentCommandActivity | AgentFileActivity | AgentTextActivity;

export type AgentRunSnapshot = {
  runID: string;
  status: AgentRunStatus;
  durationMS: number | null;
  planExplanation: string;
  plan: AgentPlanStep[];
  planSeq: number;
  items: AgentActivityItem[];
  diff: string;
  diffTruncated: boolean;
  diffSeq: number;
  files: AgentFileChange[];
  usage: AgentUsage | null;
  actualModel: string;
  previousModel: string;
  rerouteReason: string;
  rerouteSeq: number;
  interactions: ConversationInteractionDTO[];
  lastExecutionSeq: number;
};

export type AgentExecutionRecoverySnapshot = {
  contiguousSeq: number;
  highestSeq: number;
  hasGap: boolean;
};

const EMPTY_RUN: AgentRunSnapshot = Object.freeze({
  runID: "",
  status: "idle",
  durationMS: null,
  planExplanation: "",
  plan: [],
  planSeq: 0,
  items: [],
  diff: "",
  diffTruncated: false,
  diffSeq: 0,
  files: [],
  usage: null,
  actualModel: "",
  previousModel: "",
  rerouteReason: "",
  rerouteSeq: 0,
  interactions: [],
  lastExecutionSeq: 0,
});

const runs = new Map<string, AgentRunSnapshot>();
const eventJournals = new Map<string, Map<number, ConversationExecutionEventDTO>>();
const executionEvents = new Map<number, ConversationExecutionEventDTO>();
const listeners = new Set<() => void>();
const EMPTY_RECOVERY: AgentExecutionRecoverySnapshot = Object.freeze({
  contiguousSeq: 0,
  highestSeq: 0,
  hasGap: false,
});
let recoverySnapshot = EMPTY_RECOVERY;
let activeContextKey = "";
let activeConversationID = "";

function emitChange() {
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function emptyRun(runID: string): AgentRunSnapshot {
  return { ...EMPTY_RUN, runID };
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function rawString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function textParts(value: unknown): string {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string" && item.trim().length > 0).join("\n\n")
    : "";
}

function finiteNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function activityStatus(value: unknown, fallback: AgentActivityStatus = "running"): AgentActivityStatus {
  switch (stringValue(value).toLowerCase()) {
    case "completed":
    case "success":
      return "completed";
    case "interrupted":
    case "cancelled":
    case "canceled":
      return "interrupted";
    case "failed":
    case "error":
      return "failed";
    default:
      return fallback;
  }
}

function turnStatus(payload: AgentExecutionEventPayloadDTO): AgentRunStatus {
  const value = stringValue(payload.turn?.status ?? payload.status).toLowerCase();
  if (value === "completed" || value === "success") return "completed";
  if (value === "interrupted" || value === "cancelled" || value === "canceled") return "interrupted";
  if (value === "failed" || value === "error") return "failed";
  return "running";
}

function normalizePlan(payload: AgentExecutionEventPayloadDTO): AgentPlanStep[] {
  return (payload.plan ?? []).flatMap((step, index) => {
    const text = stringValue(step.step ?? step.text);
    if (!text) return [];
    const rawStatus = stringValue(step.status);
    const status = rawStatus === "completed" ? "completed" : rawStatus === "inProgress" || rawStatus === "in_progress" ? "inProgress" : "pending";
    return [{ key: `${index}:${text}`, text, status } satisfies AgentPlanStep];
  });
}

function normalizeFiles(values: AgentFileChangeDTO[] | undefined): AgentFileChange[] {
  return (values ?? []).flatMap((file, index) => {
    const path = stringValue(file.path);
    if (!path) return [];
    return [{
      fileID: stringValue(file.fileID) || `${index}:${path}`,
      path,
      previousPath: stringValue(file.previousPath),
      change: stringValue(file.change) || "modify",
      additions: finiteNumber(file.additions),
      deletions: finiteNumber(file.deletions),
      binary: file.binary === true,
      diff: rawString(file.diff),
      truncated: file.truncated === true,
    } satisfies AgentFileChange];
  });
}

function itemID(payload: AgentExecutionEventPayloadDTO, seq: number, kind: string): string {
  return stringValue(payload.itemID ?? payload.item?.itemID) || `${kind}:${seq}`;
}

function isFileItem(item: AgentExecutionItemDTO | undefined): boolean {
  const kind = stringValue(item?.type ?? item?.kind).toLowerCase();
  return kind.includes("file") || Boolean(item?.files?.length || item?.changes?.length || item?.diff);
}

function normalizeItem(payload: AgentExecutionEventPayloadDTO, seq: number, terminal: boolean): AgentActivityItem | null {
  const item = payload.item;
  if (!item) return null;
  const fileItem = isFileItem(item);
  const kind = stringValue(item.type ?? item.kind).toLowerCase();
  const status = activityStatus(item.status ?? payload.status, terminal ? "completed" : "running");
  if (kind === "agentmessage") {
    if (stringValue(item.phase) !== "commentary") return null;
    return {
      itemID: itemID(payload, seq, "commentary"),
      seq,
      kind: "commentary",
      status,
      text: rawString(item.text),
      truncated: item.truncated === true,
    };
  }
  if (kind === "reasoning") {
    return {
      itemID: itemID(payload, seq, "reasoning"),
      seq,
      kind: "reasoning",
      status,
      text: textParts(item.summary),
      truncated: item.truncated === true,
    };
  }
  if (!fileItem && !kind.includes("command")) return null;
  const id = itemID(payload, seq, fileItem ? "file" : "command");
  if (fileItem) {
    return {
      itemID: id,
      seq,
      kind: "file",
      status,
      files: normalizeFiles(item.files ?? item.changes ?? payload.files ?? payload.changes),
      diff: rawString(item.diff ?? payload.patch ?? payload.diff),
      truncated: item.truncated === true,
    };
  }
  return {
    itemID: id,
    seq,
    kind: "command",
    status,
    command: stringValue(item.command),
    cwd: rawString(item.cwd),
    durationMS: finiteNumber(item.durationMs),
    commandActions: Array.isArray(item.commandActions) ? item.commandActions : [],
    output: rawString(item.aggregatedOutput ?? item.output),
    outputTruncated: item.truncated === true,
    exitCode: finiteNumber(item.exitCode),
  };
}

function upsertItem(items: AgentActivityItem[], item: AgentActivityItem): AgentActivityItem[] {
  const index = items.findIndex((current) => current.itemID === item.itemID);
  if (index < 0) return [...items, item];
  const next = items.slice();
  const current = next[index];
  if (current.kind === "file" && item.kind === "file") {
    const currentFiles = new Map(current.files.map((file) => [file.path, file]));
    for (const file of item.files) {
      const previous = currentFiles.get(file.path);
      currentFiles.set(file.path, previous && !file.diff ? {
        ...file,
        diff: previous.diff,
        truncated: previous.truncated || file.truncated,
      } : file);
    }
    next[index] = {
      ...current,
      ...item,
      seq: current.seq,
      diff: item.diff || current.diff,
      truncated: current.truncated || item.truncated,
      files: [...currentFiles.values()],
    };
  } else {
    next[index] = current.kind === item.kind
      ? { ...current, ...item, seq: current.seq } as AgentActivityItem
      : item;
  }
  return next;
}

function usageValue(usage: AgentTokenUsageDTO | undefined): AgentUsage | null {
  if (!usage) return null;
  const value = {
    inputTokens: finiteNumber(usage.inputTokens) ?? 0,
    outputTokens: finiteNumber(usage.outputTokens) ?? 0,
    cachedInputTokens: finiteNumber(usage.cachedInputTokens ?? usage.cacheReadTokens) ?? 0,
    cacheReadTokens: finiteNumber(usage.cacheReadTokens ?? usage.cachedInputTokens) ?? 0,
    reasoningTokens: finiteNumber(usage.reasoningTokens) ?? 0,
    totalTokens: finiteNumber(usage.totalTokens) ?? 0,
    scope: "thread" as const,
  };
  return Object.values(value).some((item) => typeof item === "number" && item > 0) ? value : null;
}

function reduceAgentExecutionEvent(
  current: AgentRunSnapshot,
  event: ConversationExecutionEventDTO,
): AgentRunSnapshot {
  const next: AgentRunSnapshot = { ...current, lastExecutionSeq: event.seq };
  const payload = event.payload;
  switch (event.kind) {
    case "turn/started":
      next.status = "running";
      break;
    case "turn/completed":
      next.status = turnStatus(payload);
      next.durationMS = finiteNumber(payload.turn?.durationMs);
      break;
    case "turn/plan/updated":
      next.plan = normalizePlan(payload);
      next.planExplanation = stringValue(payload.explanation);
      if (next.planSeq === 0) next.planSeq = event.seq;
      break;
    case "item/started":
    case "item/completed": {
      const item = normalizeItem(payload, event.seq, event.kind === "item/completed");
      if (item) next.items = upsertItem(next.items, item);
      break;
    }
    case "item/commandExecution/outputDelta": {
      const id = itemID(payload, event.seq, "command");
      const delta = rawString(payload.delta ?? payload.outputDelta);
      const currentItem = next.items.find(
        (item) => item.itemID === id && item.kind === "command",
      ) as AgentCommandActivity | undefined;
      next.items = upsertItem(next.items, {
        itemID: id,
        seq: currentItem?.seq ?? event.seq,
        kind: "command",
        status: currentItem?.status ?? "running",
        command: currentItem?.command ?? "",
        cwd: currentItem?.cwd ?? "",
        durationMS: currentItem?.durationMS ?? null,
        commandActions: currentItem?.commandActions ?? [],
        output: `${currentItem?.output ?? ""}${delta}`,
        outputTruncated: currentItem?.outputTruncated ?? false,
        exitCode: currentItem?.exitCode ?? null,
      });
      break;
    }
    case "item/agentMessage/delta": {
      if (stringValue(payload.phase) !== "commentary") break;
      const id = itemID(payload, event.seq, "commentary");
      const currentItem = next.items.find(
        (item) => item.itemID === id && item.kind === "commentary",
      ) as AgentTextActivity | undefined;
      next.items = upsertItem(next.items, {
        itemID: id,
        seq: currentItem?.seq ?? event.seq,
        kind: "commentary",
        status: currentItem?.status ?? "running",
        text: `${currentItem?.text ?? ""}${rawString(payload.delta)}`,
        truncated: currentItem?.truncated ?? payload.truncated === true,
      });
      break;
    }
    case "item/reasoning/summaryTextDelta": {
      const id = itemID(payload, event.seq, "reasoning");
      const currentItem = next.items.find(
        (item) => item.itemID === id && item.kind === "reasoning",
      ) as AgentTextActivity | undefined;
      next.items = upsertItem(next.items, {
        itemID: id,
        seq: currentItem?.seq ?? event.seq,
        kind: "reasoning",
        status: currentItem?.status ?? "running",
        text: `${currentItem?.text ?? ""}${rawString(payload.delta)}`,
        truncated: currentItem?.truncated ?? payload.truncated === true,
      });
      break;
    }
    case "item/reasoning/summaryPartAdded": {
      const id = itemID(payload, event.seq, "reasoning");
      const currentItem = next.items.find(
        (item) => item.itemID === id && item.kind === "reasoning",
      ) as AgentTextActivity | undefined;
      if (currentItem?.text && !currentItem.text.endsWith("\n\n")) {
        next.items = upsertItem(next.items, { ...currentItem, text: `${currentItem.text}\n\n` });
      }
      break;
    }
    case "item/fileChange/patchUpdated": {
      const id = itemID(payload, event.seq, "file");
      const currentItem = next.items.find(
        (item) => item.itemID === id && item.kind === "file",
      ) as AgentFileActivity | undefined;
      const files = normalizeFiles(payload.files ?? payload.changes);
      next.items = upsertItem(next.items, {
        itemID: id,
        seq: currentItem?.seq ?? event.seq,
        kind: "file",
        status: currentItem?.status ?? "running",
        files: files.length > 0 ? files : currentItem?.files ?? [],
        diff: rawString(payload.patch ?? payload.diff) || currentItem?.diff || "",
        truncated: currentItem?.truncated === true || payload.truncated === true,
      });
      break;
    }
    case "turn/diff/updated":
      next.diff = rawString(payload.diff ?? payload.patch);
      next.diffTruncated = payload.truncated === true;
      if (next.diffSeq === 0) next.diffSeq = event.seq;
      next.files = normalizeFiles(payload.files ?? payload.changes);
      break;
    case "thread/tokenUsage/updated":
      next.usage = usageValue(payload.tokenUsage?.total ?? payload.tokenUsage ?? payload.usage);
      break;
    case "model/rerouted":
      next.actualModel = stringValue(payload.toModel ?? payload.model);
      next.previousModel = stringValue(payload.fromModel);
      next.rerouteReason = stringValue(payload.reason);
      if (next.rerouteSeq === 0) next.rerouteSeq = event.seq;
      break;
  }
  return next;
}

export function hasAgentRunActivity(run: AgentRunSnapshot): boolean {
  return run.status !== "idle" || run.plan.length > 0 || run.items.length > 0 ||
    Boolean(run.diff || run.actualModel || run.usage || run.interactions.length > 0);
}

export function hasComposerAgentActivity(
  run: Pick<AgentRunSnapshot, "status" | "plan"> & {
    interactions: Array<Pick<ConversationInteractionDTO, "status">>;
  },
): boolean {
  const hasUnresolvedInteraction = run.interactions.some((item) => item.status !== "resolved");
  const active = run.status === "running" || run.status === "waiting_interaction" || hasUnresolvedInteraction;
  return active && (run.plan.length > 0 || hasUnresolvedInteraction);
}

function isTerminalStatus(status: AgentRunStatus): boolean {
  return status === "completed" || status === "interrupted" || status === "failed";
}

function rebuildRun(runID: string, interactions = runs.get(runID)?.interactions ?? []): AgentRunSnapshot {
  const journal = eventJournals.get(runID);
  const rebuilt = [...(journal?.values() ?? [])]
    .sort((left, right) => left.seq - right.seq)
    .reduce(reduceAgentExecutionEvent, emptyRun(runID));
  return applyInteractionStatus(rebuilt, interactions);
}

function applyInteractionStatus(
  run: AgentRunSnapshot,
  interactions: ConversationInteractionDTO[],
): AgentRunSnapshot {
  const waiting = interactions.some((item) => item.status === "pending" || item.status === "responding");
  if (waiting && !isTerminalStatus(run.status)) {
    return { ...run, status: "waiting_interaction", interactions };
  }
  return { ...run, interactions };
}

export function applyAgentExecutionEvent(
  event: ConversationExecutionEventDTO,
  sourceConversationID = "",
): boolean {
  return applyAgentExecutionEvents([event], sourceConversationID) === 1;
}

export function applyAgentExecutionEvents(
  events: ConversationExecutionEventDTO[],
  sourceConversationID = "",
): number {
  const normalizedSourceConversationID = sourceConversationID.trim();
  if (normalizedSourceConversationID && normalizedSourceConversationID !== activeConversationID) return 0;

  const acceptedByRun = new Map<string, ConversationExecutionEventDTO[]>();
  let highestSeq = recoverySnapshot.highestSeq;
  let acceptedCount = 0;
  for (const event of events) {
    const runID = event.runID.trim();
    if (!runID || !Number.isSafeInteger(event.seq) || event.seq <= 0 || executionEvents.has(event.seq)) {
      continue;
    }
    const journal = eventJournals.get(runID) ?? new Map<number, ConversationExecutionEventDTO>();
    executionEvents.set(event.seq, event);
    journal.set(event.seq, event);
    eventJournals.set(runID, journal);
    const accepted = acceptedByRun.get(runID) ?? [];
    accepted.push(event);
    acceptedByRun.set(runID, accepted);
    highestSeq = Math.max(highestSeq, event.seq);
    acceptedCount += 1;
  }
  if (acceptedCount === 0) return 0;

  let contiguousSeq = recoverySnapshot.contiguousSeq;
  while (executionEvents.has(contiguousSeq + 1)) contiguousSeq += 1;
  recoverySnapshot = {
    contiguousSeq,
    highestSeq,
    hasGap: highestSeq > contiguousSeq,
  };

  for (const [runID, accepted] of acceptedByRun) {
    const current = runs.get(runID);
    const sorted = accepted.sort((left, right) => left.seq - right.seq);
    const next = current && sorted[0].seq > current.lastExecutionSeq
      ? sorted.reduce(reduceAgentExecutionEvent, current)
      : rebuildRun(runID);
    runs.set(runID, applyInteractionStatus(next, current?.interactions ?? []));
  }
  emitChange();
  return acceptedCount;
}

export function replaceActiveAgentInteractions(items: ConversationInteractionDTO[]) {
  const activeByRun = new Map<string, ConversationInteractionDTO[]>();
  for (const item of items) {
    const runID = item.runID?.trim();
    if (!runID) continue;
    const current = activeByRun.get(runID) ?? [];
    const index = current.findIndex((existing) => existing.interactionID === item.interactionID);
    if (index >= 0) current[index] = item;
    else current.push(item);
    activeByRun.set(runID, current);
  }
  let changed = false;
  for (const [runID, run] of runs) {
    const interactionsByID = new Map(
      run.interactions
        .filter((item) => item.status === "resolved")
        .map((item) => [item.interactionID, item]),
    );
    for (const item of activeByRun.get(runID) ?? []) interactionsByID.set(item.interactionID, item);
    const interactions = [...interactionsByID.values()];
    if (JSON.stringify(run.interactions) === JSON.stringify(interactions)) continue;
    runs.set(runID, rebuildRun(runID, interactions));
    changed = true;
  }
  for (const [runID, interactions] of activeByRun) {
    if (!runs.has(runID)) {
      runs.set(runID, rebuildRun(runID, interactions));
      changed = true;
    }
  }
  if (changed) emitChange();
}

export function updateAgentInteraction(interaction: ConversationInteractionDTO) {
  const runID = interaction.runID?.trim();
  if (!runID) return;
  const current = runs.get(runID) ?? emptyRun(runID);
  const interactions = current.interactions.filter((item) => item.interactionID !== interaction.interactionID);
  interactions.push(interaction);
  runs.set(runID, rebuildRun(runID, interactions));
  emitChange();
}

export function setAgentInteractionStatus(interactionID: string, status: ConversationInteractionStatus) {
  for (const [runID, run] of runs) {
    const index = run.interactions.findIndex((item) => item.interactionID === interactionID);
    if (index < 0) continue;
    const interactions = run.interactions.slice();
    interactions[index] = { ...interactions[index], status } as ConversationInteractionDTO;
    runs.set(runID, rebuildRun(runID, interactions));
    emitChange();
    return;
  }
}

export function setAgentRunContext(contextKey: string, conversationID: string) {
  const normalizedContextKey = contextKey.trim();
  const normalizedConversationID = conversationID.trim();
  if (normalizedContextKey === activeContextKey && normalizedConversationID === activeConversationID) return;
  activeContextKey = normalizedContextKey;
  activeConversationID = normalizedConversationID;
  runs.clear();
  eventJournals.clear();
  executionEvents.clear();
  recoverySnapshot = EMPTY_RECOVERY;
  emitChange();
}

export function getAgentExecutionRecoverySnapshot(): AgentExecutionRecoverySnapshot {
  return recoverySnapshot;
}

export function getAgentRunSnapshot(runID: string | undefined): AgentRunSnapshot {
  return runs.get(runID?.trim() || "") ?? EMPTY_RUN;
}

export function useAgentExecutionRecoverySnapshot(): AgentExecutionRecoverySnapshot {
  return React.useSyncExternalStore(
    subscribe,
    getAgentExecutionRecoverySnapshot,
    () => EMPTY_RECOVERY,
  );
}

export function useAgentRunSnapshot(runID: string | undefined): AgentRunSnapshot {
  const normalizedRunID = runID?.trim() || "";
  return React.useSyncExternalStore(
    subscribe,
    () => getAgentRunSnapshot(normalizedRunID),
    () => EMPTY_RUN,
  );
}
