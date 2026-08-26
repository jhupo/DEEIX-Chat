"use client";

import {
  Check,
  ChevronDown,
  Circle,
  CircleAlert,
  FileCode2,
  GitCompareArrows,
  ListChecks,
  Route,
  TerminalSquare,
  Wrench,
} from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { MessageAgentInteractionControl } from "@/features/chat/components/message/message-agent-interaction";
import {
  type AgentActivityItem,
  type AgentCommandActivity,
  type AgentFileChange,
  type AgentRunSnapshot,
  type AgentToolActivity,
  hasComposerAgentActivity,
  useAgentRunSnapshot,
} from "@/features/chat/model/agent-run-store";
import { cn } from "@/lib/utils";
import type { ConversationInteractionDTO } from "@/shared/api/conversation.types";
import { StreamdownRender } from "@/shared/components/markdown/streamdown-render";

function statusKey(status: string): "running" | "completed" | "failed" | "interrupted" | "pending" | "inProgress" {
  if (status === "completed") return "completed";
  if (status === "failed") return "failed";
  if (status === "interrupted") return "interrupted";
  if (status === "inProgress") return "inProgress";
  if (status === "pending") return "pending";
  return "running";
}

function formatDuration(durationMS: number | null): string {
  if (durationMS === null) return "";
  if (durationMS < 1000) return `${Math.round(durationMS)}ms`;
  const seconds = Math.round(durationMS / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  if (minutes < 60) return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}

function ActivityStatus({ status }: { status: string }) {
  const t = useTranslations("chat.agent");
  const failed = status === "failed";
  const Icon = status === "completed" ? Check : failed ? CircleAlert : Circle;
  return (
    <span className={cn("inline-flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground/65", failed && "text-destructive/80")}>
      <Icon className="size-3" />
      {t(`activity.status.${statusKey(status)}`)}
    </span>
  );
}

function CommandRow({ item }: { item: AgentCommandActivity }) {
  const t = useTranslations("chat.agent");
  const [open, setOpen] = React.useState(false);
  const hasDetails = Boolean(item.output || item.cwd || item.exitCode !== null || item.durationMS !== null);
  const duration = formatDuration(item.durationMS);

  return (
    <div className="border-t border-border/25 py-1.5 first:border-t-0">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="group/command flex w-full min-w-0 items-center gap-2 rounded-sm py-0.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/35"
            aria-expanded={open}
            onClick={() => hasDetails && setOpen((value) => !value)}
          >
            <TerminalSquare className="size-3.5 shrink-0 text-muted-foreground/62" />
            <ActivityStatus status={item.status} />
            {duration ? <span className="shrink-0 text-[11px] tabular-nums text-muted-foreground/55">{duration}</span> : null}
            <code className="min-w-0 flex-1 truncate text-[12px] text-foreground/82">{item.command || t("activity.commandFallback")}</code>
            {hasDetails ? <ChevronDown className={cn("size-3.5 shrink-0 text-muted-foreground transition-transform", open && "rotate-180")} /> : null}
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" align="start" className="max-w-[min(42rem,calc(100vw-2rem))] space-y-1 font-mono text-[11px]">
          <div className="break-all">{item.command || t("activity.commandFallback")}</div>
          {item.cwd ? <div className="break-all text-muted-foreground">{item.cwd}</div> : null}
          <div className="flex gap-3 text-muted-foreground">
            {duration ? <span>{duration}</span> : null}
            {item.exitCode !== null ? <span>{t("activity.exitCode", { code: item.exitCode })}</span> : null}
          </div>
        </TooltipContent>
      </Tooltip>
      {open ? (
        <div className="ml-5 mt-1.5 space-y-1.5">
          {item.cwd ? <div className="break-all font-mono text-[11px] text-muted-foreground/62">{item.cwd}</div> : null}
          {item.output ? (
            <pre className="max-h-64 overflow-auto whitespace-pre-wrap border-l border-border/45 pl-2.5 font-mono text-[11px] leading-5 text-muted-foreground/88 [overflow-wrap:anywhere]">{item.output}</pre>
          ) : null}
          {item.outputTruncated ? <p className="text-[11px] text-muted-foreground/60">{t("activity.truncated")}</p> : null}
        </div>
      ) : null}
    </div>
  );
}

function ToolRow({ item }: { item: AgentToolActivity }) {
  const [open, setOpen] = React.useState(false);
  const hasDetails = Boolean(item.input || item.output || item.error || item.durationMS !== null);
  const duration = formatDuration(item.durationMS);

  return (
    <div className="border-t border-border/25 py-1.5 first:border-t-0">
      <button
        type="button"
        className="group/tool flex w-full min-w-0 items-center gap-2 rounded-sm py-0.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/35"
        aria-expanded={open}
        onClick={() => hasDetails && setOpen((value) => !value)}
      >
        <Wrench className="size-3.5 shrink-0 text-muted-foreground/62" />
        <ActivityStatus status={item.status} />
        {duration ? <span className="shrink-0 text-[11px] tabular-nums text-muted-foreground/55">{duration}</span> : null}
        <span className="min-w-0 flex-1 truncate text-[12px] text-foreground/82">
          {item.name}
        </span>
        {hasDetails ? (
          <ChevronDown className={cn("size-3.5 shrink-0 text-muted-foreground transition-transform", open && "rotate-180")} />
        ) : null}
      </button>
      {open ? (
        <div className="ml-5 mt-1.5 space-y-2 border-l border-border/45 pl-2.5 text-[11px] leading-5 text-muted-foreground/88">
          {item.input ? <pre className="max-h-48 overflow-auto whitespace-pre-wrap [overflow-wrap:anywhere]">{item.input}</pre> : null}
          {item.output ? <pre className="max-h-64 overflow-auto whitespace-pre-wrap [overflow-wrap:anywhere]">{item.output}</pre> : null}
          {item.error ? <pre className="whitespace-pre-wrap text-destructive/80 [overflow-wrap:anywhere]">{item.error}</pre> : null}
        </div>
      ) : null}
    </div>
  );
}

function interactionMatchesItem(interaction: ConversationInteractionDTO, item: AgentActivityItem): boolean {
  if (interaction.kind === "command_approval" && item.kind === "command") {
    const requestCommand = interaction.request.command?.trim();
    return Boolean(requestCommand && item.command && (requestCommand === item.command || item.command.includes(requestCommand)));
  }
  if (interaction.kind === "file_approval" && item.kind === "file") {
    const paths = new Set((interaction.request.changes ?? []).map((file) => file.path?.trim()).filter(Boolean));
    return item.files.some((file) => paths.has(file.path));
  }
  return false;
}

function ActivityItemRow({ item }: { item: AgentActivityItem }) {
  const t = useTranslations("chat.agent");
  if (item.kind === "command") return <CommandRow item={item} />;
  if (item.kind === "tool") return <ToolRow item={item} />;
  if (item.kind === "file") {
    return <FileChangesRow files={item.files} fallbackDiff={item.diff} fallbackTruncated={item.truncated} status={item.status} />;
  }
  if (!item.text.trim()) return null;
  if (item.kind === "reasoning") {
    return (
      <div className="border-t border-border/25 py-2 first:border-t-0">
        <StreamdownRender content={item.text} streaming={item.status === "running"} variant="thinking" />
        {item.truncated ? <p className="mt-1 text-[11px] text-muted-foreground/60">{t("activity.truncated")}</p> : null}
      </div>
    );
  }
  return (
    <div className="py-2 text-[13px] leading-6 text-foreground/88">
      <StreamdownRender content={item.text} streaming={item.status === "running"} />
      {item.truncated ? <p className="mt-1 text-[11px] text-muted-foreground/60">{t("activity.truncated")}</p> : null}
    </div>
  );
}

function AgentPlanSteps({ run }: { run: AgentRunSnapshot }) {
  return (
    <ol className="space-y-1 pl-5">
      {run.plan.map((step) => (
        <li key={step.key} className="flex min-w-0 items-start gap-2 text-[12px] leading-5 text-muted-foreground/82">
          <ActivityStatus status={step.status} />
          <span className="min-w-0 break-words">{step.text}</span>
        </li>
      ))}
    </ol>
  );
}

export function MessageAgentActivity({
  run,
  showPlan = true,
}: {
  run: AgentRunSnapshot;
  showPlan?: boolean;
}) {
  const t = useTranslations("chat.agent");
  const interactions = run.interactions.filter((item) => item.status === "resolved");
  const timeline = React.useMemo(() => buildActivityTimeline(run, showPlan), [run, showPlan]);
  const matched = new Set<string>();

  return (
    <div className="space-y-0">
      {timeline.map((entry) => {
        if (entry.kind === "plan") {
          return (
            <section key="plan" className="border-t border-border/25 py-2 first:border-t-0">
              <div className="mb-1.5 flex items-center gap-2 text-[12px] font-medium text-muted-foreground/80">
                <ListChecks className="size-3.5" />
                {t("activity.plan", { count: run.plan.length })}
              </div>
              {run.planExplanation ? <p className="mb-1.5 whitespace-pre-wrap text-[12px] leading-5 text-muted-foreground/72">{run.planExplanation}</p> : null}
              <AgentPlanSteps run={run} />
            </section>
          );
        }
        if (entry.kind === "diff") {
          return <FileChangesRow key="diff" run={run} />;
        }
        if (entry.kind === "reroute") {
          return (
            <div key="reroute" className="flex items-center gap-2 border-t border-border/25 py-2 text-[12px] text-muted-foreground/72">
              <Route className="size-3.5" />
              <span>{t("activity.modelRerouted", { model: run.actualModel })}</span>
              {run.rerouteReason ? <span className="truncate">{run.rerouteReason}</span> : null}
            </div>
          );
        }
        if (entry.kind === "operations") {
          const inlineInteractions = interactions.filter((interaction) => {
            if (!entry.items.some((item) => interactionMatchesItem(interaction, item))) return false;
            matched.add(interaction.interactionID);
            return true;
          });
          return (
            <React.Fragment key={`operations:${entry.seq}`}>
              <OperationGroup items={entry.items} />
              {inlineInteractions.map((interaction) => (
                <MessageAgentInteractionControl key={interaction.interactionID} interaction={interaction} />
              ))}
            </React.Fragment>
          );
        }
        const item = entry.item;
        if (!item) return null;
        const inlineInteractions = interactions.filter((interaction) => {
          if (!interactionMatchesItem(interaction, item)) return false;
          matched.add(interaction.interactionID);
          return true;
        });
        return (
          <React.Fragment key={item.itemID}>
            <ActivityItemRow item={item} />
            {inlineInteractions.map((interaction) => <MessageAgentInteractionControl key={interaction.interactionID} interaction={interaction} />)}
          </React.Fragment>
        );
      })}
      {interactions.filter((interaction) => !matched.has(interaction.interactionID)).map((interaction) => (
        <MessageAgentInteractionControl key={interaction.interactionID} interaction={interaction} />
      ))}
    </div>
  );
}

function ComposerAgentActivityContent({ run }: { run: AgentRunSnapshot }) {
  const t = useTranslations("chat.agent");
  const [planOpen, setPlanOpen] = React.useState(true);
  const interactions = run.interactions.filter((item) => item.status !== "resolved");
  const completedSteps = run.plan.filter((step) => step.status === "completed").length;
  const currentStep = run.plan.find((step) => step.status === "inProgress")
    ?? run.plan.find((step) => step.status === "pending")
    ?? run.plan.at(-1);

  return (
    <section
      className="relative z-[5] mx-3 mb-[-10px] max-h-[40dvh] overflow-y-auto overscroll-contain rounded-t-2xl rounded-b-xl border border-border/45 bg-pure/95 px-3 pb-4 pt-2 shadow-xs backdrop-blur-xl scroll-fade-12"
      aria-live="polite"
    >
      {interactions.length > 0 ? (
        <div>
          {interactions.map((interaction) => (
            <MessageAgentInteractionControl key={interaction.interactionID} interaction={interaction} />
          ))}
        </div>
      ) : null}
      {run.plan.length > 0 ? (
        <div className={cn(interactions.length > 0 && "mt-2 border-t border-border/35 pt-2")}>
          <button
            type="button"
            className="flex w-full min-w-0 items-center gap-2 rounded-md py-1 text-left text-[12px] text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/35"
            aria-expanded={planOpen}
            onClick={() => setPlanOpen((open) => !open)}
          >
            <ListChecks className="size-3.5 shrink-0" />
            <span className="shrink-0 font-medium text-foreground/88">{t("activity.title.running")}</span>
            <span className="shrink-0 tabular-nums text-muted-foreground/70">
              {completedSteps}/{run.plan.length}
            </span>
            {currentStep ? <span className="min-w-0 flex-1 truncate">{currentStep.text}</span> : null}
            <ChevronDown className={cn("size-3.5 shrink-0 transition-transform", planOpen && "rotate-180")} />
          </button>
          {planOpen ? (
            <div className="pb-1 pt-1">
              {run.planExplanation ? (
                <p className="mb-1.5 whitespace-pre-wrap text-[12px] leading-5 text-muted-foreground/72">
                  {run.planExplanation}
                </p>
              ) : null}
              <AgentPlanSteps run={run} />
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

export function MessageAgentComposerActivity({ runID }: { runID?: string }) {
  const run = useAgentRunSnapshot(runID);
  if (!hasComposerAgentActivity(run)) {
    return null;
  }
  return <ComposerAgentActivityContent key={run.runID} run={run} />;
}

function collectChangedFiles(run: AgentRunSnapshot): AgentFileChange[] {
  const byPath = new Map<string, AgentFileChange>();
  for (const file of run.files) byPath.set(file.path, file);
  for (const item of run.items) {
    if (item.kind !== "file") continue;
    for (const file of item.files) {
      byPath.set(file.path, file.diff || !item.diff ? file : {
        ...file,
        diff: item.diff,
        truncated: file.truncated || item.truncated,
      });
    }
  }
  return [...byPath.values()];
}

type ActivityTimelineEntry =
  | { kind: "plan"; seq: number }
  | { kind: "item"; seq: number; item: AgentActivityItem }
  | { kind: "operations"; seq: number; items: AgentOperationActivity[] }
  | { kind: "diff"; seq: number }
  | { kind: "reroute"; seq: number };

type AgentOperationActivity = Extract<AgentActivityItem, { kind: "command" | "file" | "tool" }>;

function activityTimelineRank(kind: ActivityTimelineEntry["kind"]): number {
  switch (kind) {
    case "plan":
      return 0;
    case "item":
    case "operations":
      return 1;
    case "diff":
      return 2;
    case "reroute":
      return 3;
  }
}

function buildActivityTimeline(run: AgentRunSnapshot, showPlan: boolean): ActivityTimelineEntry[] {
  const nonReasoningItems = run.items.filter((item) => item.kind !== "reasoning");
  const visibleItems = nonReasoningItems.length > 0
    ? nonReasoningItems
    : run.items.filter((item) => item.kind === "reasoning").slice(-1);
  const entries: ActivityTimelineEntry[] = visibleItems.map((item) => ({ kind: "item", seq: item.seq, item }));
  if (showPlan && run.plan.length > 0) {
    entries.push({ kind: "plan", seq: run.planSeq });
  }
  const hasFileItem = run.items.some((item) => item.kind === "file");
  if (!hasFileItem && (run.files.length > 0 || run.diff)) {
    entries.push({ kind: "diff", seq: run.diffSeq || Number.MAX_SAFE_INTEGER });
  }
  if (run.actualModel) {
    entries.push({ kind: "reroute", seq: run.rerouteSeq || Number.MAX_SAFE_INTEGER });
  }
  const sorted = entries.sort((left, right) => left.seq - right.seq || activityTimelineRank(left.kind) - activityTimelineRank(right.kind));
  const grouped: ActivityTimelineEntry[] = [];
  for (const entry of sorted) {
    if (
      entry.kind === "item" &&
      (entry.item.kind === "command" || entry.item.kind === "file" || entry.item.kind === "tool")
    ) {
      const previous = grouped.at(-1);
      if (previous?.kind === "operations") {
        previous.items.push(entry.item);
      } else {
        grouped.push({ kind: "operations", seq: entry.seq, items: [entry.item] });
      }
      continue;
    }
    grouped.push(entry);
  }
  return grouped;
}

function commandActionFiles(items: AgentOperationActivity[], actionType: string): number {
  const paths = new Set<string>();
  for (const item of items) {
    if (item.kind !== "command") continue;
    for (const action of item.commandActions) {
      if (String(action.type ?? "").toLowerCase() !== actionType) continue;
      const path = String(action.path ?? "").trim();
      if (path) paths.add(path);
    }
  }
  return paths.size;
}

function OperationGroup({ items }: { items: AgentOperationActivity[] }) {
  const t = useTranslations("chat.agent");
  const running = items.some((item) => item.status === "running");
  const [open, setOpen] = React.useState(running);
  const commands = items.filter((item) => item.kind === "command").length;
  const tools = items.filter((item) => item.kind === "tool").length;
  const editedFiles = new Set(
    items.flatMap((item) => item.kind === "file" ? item.files.map((file) => file.path) : []),
  ).size;
  const readFiles = commandActionFiles(items, "read");

  React.useEffect(() => {
    if (running) setOpen(true);
  }, [running]);

  let summary: string;
  if (editedFiles > 0 && commands > 0) {
    summary = t("activity.operations.editedAndCommands", { files: editedFiles, commands });
  } else if (readFiles > 0 && commands > 0) {
    summary = t("activity.operations.readAndCommands", { files: readFiles, commands });
  } else if (commands > 0 && tools > 0) {
    summary = t("activity.operations.commandsAndTools", { commands, tools });
  } else if (commands > 0) {
    summary = t(running ? "activity.commandsRunning" : "activity.commandsRan", { count: commands });
  } else if (editedFiles > 0) {
    summary = t("activity.fileChanges", { count: editedFiles });
  } else {
    summary = t(running ? "activity.toolsRunning" : "activity.toolsRan", { count: tools });
  }
  const Icon = editedFiles > 0 ? GitCompareArrows : commands > 0 ? TerminalSquare : Wrench;

  return (
    <section className="border-t border-border/25 py-1.5 first:border-t-0">
      <button
        type="button"
        className="group/operations flex w-full min-w-0 items-center gap-1.5 rounded-sm py-0.5 text-left text-[12px] text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/35"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <Icon className="size-3.5 shrink-0" />
        <span className={cn("min-w-0 truncate", running && "shimmer")}>{summary}</span>
        <ChevronDown className={cn("size-3.5 shrink-0 transition-transform duration-200", open && "rotate-180")} />
      </button>
      {open ? (
        <div className="ml-5 mt-1">
          {items.map((item) => <ActivityItemRow key={item.itemID} item={item} />)}
        </div>
      ) : null}
    </section>
  );
}

function fileName(path: string): string {
  return path.replaceAll("\\", "/").split("/").filter(Boolean).at(-1) || path;
}

function diffLineClassName(line: string): string {
  if (line.startsWith("+++") || line.startsWith("---") || line.startsWith("@@")) return "text-sky-700 dark:text-sky-300";
  if (line.startsWith("+")) return "bg-emerald-500/10 text-emerald-800 dark:text-emerald-300";
  if (line.startsWith("-")) return "bg-red-500/10 text-red-800 dark:text-red-300";
  return "text-muted-foreground";
}

function FileDiffTooltip({
  file,
  fallbackDiff,
  fallbackTruncated,
}: {
  file: AgentFileChange;
  fallbackDiff: string;
  fallbackTruncated: boolean;
}) {
  const t = useTranslations("chat.agent");
  const diffLines = (file.diff || fallbackDiff).trimEnd().split(/\r?\n/);
  const previewLines = diffLines.slice(0, 80);
  const truncated = file.truncated || !file.diff && fallbackTruncated || diffLines.length > previewLines.length;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button type="button" className="min-w-0 flex-1 truncate rounded-sm text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/35">
          {fileName(file.path)}
        </button>
      </TooltipTrigger>
      <TooltipContent
        side="top"
        align="start"
        sideOffset={6}
        collisionPadding={12}
        className="max-h-[min(28rem,70dvh)] w-[min(46rem,calc(100vw-1.5rem))] overflow-auto border border-border bg-popover p-0 text-popover-foreground shadow-lg"
      >
        <div className="sticky top-0 z-10 border-b border-border/60 bg-popover px-3 py-2 font-mono text-[11px] text-muted-foreground [overflow-wrap:anywhere]">
          {file.path}
        </div>
        {previewLines.some(Boolean) ? (
          <pre className="min-w-max py-2 font-mono text-[11px] leading-5">
            {previewLines.map((line, index) => (
              <span key={`${index}:${line}`} className={cn("block min-h-5 px-3 whitespace-pre", diffLineClassName(line))}>{line || " "}</span>
            ))}
          </pre>
        ) : null}
        {truncated ? <div className="border-t border-border/60 px-3 py-1.5 text-[11px] text-muted-foreground">{t("activity.truncated")}</div> : null}
      </TooltipContent>
    </Tooltip>
  );
}

function FileChangesRow({
  run,
  files,
  fallbackDiff,
  fallbackTruncated = false,
  status = "completed",
}: {
  run?: AgentRunSnapshot;
  files?: AgentFileChange[];
  fallbackDiff?: string;
  fallbackTruncated?: boolean;
  status?: string;
}) {
  const t = useTranslations("chat.agent");
  const changedFiles = files ?? (run ? collectChangedFiles(run) : []);
  const diff = fallbackDiff ?? run?.diff ?? "";
  const truncated = fallbackTruncated || Boolean(run?.diffTruncated);
  if (changedFiles.length === 0 && !diff) return null;

  return (
    <div className="border-t border-border/25 py-2">
      <div className="flex max-w-full items-center gap-2 py-1 text-[12px] font-medium text-muted-foreground">
        <GitCompareArrows className="size-3.5 shrink-0" />
        <ActivityStatus status={status} />
        <span className="truncate">{t("activity.fileChanges", { count: changedFiles.length })}</span>
      </div>
      {changedFiles.length > 0 ? (
        <ul className="mt-1 space-y-0.5 pl-5 font-mono text-[11px] text-muted-foreground/72">
          {changedFiles.map((file) => (
            <li key={`${file.previousPath}:${file.path}`} className="flex min-w-0 items-center gap-2">
              <FileCode2 className="size-3.5 shrink-0 text-muted-foreground/62" />
              <FileDiffTooltip file={file} fallbackDiff={diff} fallbackTruncated={truncated} />
              {file.additions !== null ? <span className="text-foreground/65">+{file.additions}</span> : null}
              {file.deletions !== null ? <span className="text-destructive/70">-{file.deletions}</span> : null}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

export function formatAgentRunDuration(durationMS: number | null): string {
  return formatDuration(durationMS);
}
