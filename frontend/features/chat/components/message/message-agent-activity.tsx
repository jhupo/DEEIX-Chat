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
} from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { MessageAgentInteractionControl } from "@/features/chat/components/message/message-agent-interaction";
import type {
  AgentActivityItem,
  AgentCommandActivity,
  AgentFileChange,
  AgentRunSnapshot,
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

function interactionMatchesItem(interaction: ConversationInteractionDTO, item: AgentActivityItem): boolean {
  if (interaction.kind === "command_approval" && item.kind === "command") {
    const requestCommand = interaction.request.command?.trim();
    return Boolean(requestCommand && item.command && (requestCommand === item.command || item.command.includes(requestCommand)));
  }
  if (interaction.kind === "file_approval" && item.kind === "file") {
    const paths = new Set((interaction.request.files ?? interaction.request.changes ?? []).map((file) => file.path?.trim()).filter(Boolean));
    return item.files.some((file) => paths.has(file.path));
  }
  return false;
}

function ActivityItemRow({ item }: { item: AgentActivityItem }) {
  const t = useTranslations("chat.agent");
  if (item.kind === "command") return <CommandRow item={item} />;
  if (item.kind === "file") {
    return (
      <div className="flex min-w-0 items-center gap-2 border-t border-border/25 py-2 first:border-t-0">
        <FileCode2 className="size-3.5 shrink-0 text-muted-foreground/62" />
        <ActivityStatus status={item.status} />
        <span className="min-w-0 flex-1 truncate text-[12px] text-foreground/82">{t("activity.fileChanges", { count: item.files.length })}</span>
      </div>
    );
  }
  if (!item.text.trim()) return null;
  return (
    <div className={cn("py-2 text-[13px] leading-6 text-foreground/88", item.kind === "reasoning" && "text-muted-foreground/88")}>
      <StreamdownRender content={item.text} streaming={item.status === "running"} />
      {item.truncated ? <p className="mt-1 text-[11px] text-muted-foreground/60">{t("activity.truncated")}</p> : null}
    </div>
  );
}

export function MessageAgentActivity({ run }: { run: AgentRunSnapshot }) {
  const t = useTranslations("chat.agent");
  const interactions = run.interactions.filter((item) => item.status !== "resolved");
  const matched = new Set<string>();

  return (
    <div className="space-y-1">
      {run.plan.length > 0 ? (
        <section className="pb-2">
          <div className="mb-1.5 flex items-center gap-2 text-[12px] font-medium text-muted-foreground/80">
            <ListChecks className="size-3.5" />
            {t("activity.plan", { count: run.plan.length })}
          </div>
          {run.planExplanation ? <p className="mb-1.5 whitespace-pre-wrap text-[12px] leading-5 text-muted-foreground/72">{run.planExplanation}</p> : null}
          <ol className="space-y-1 pl-5">
            {run.plan.map((step) => (
              <li key={step.key} className="flex min-w-0 items-start gap-2 text-[12px] leading-5 text-muted-foreground/82">
                <ActivityStatus status={step.status} />
                <span className="min-w-0 break-words">{step.text}</span>
              </li>
            ))}
          </ol>
        </section>
      ) : null}
      {run.items.map((item) => {
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
      {run.actualModel ? (
        <div className="flex items-center gap-2 border-t border-border/25 py-2 text-[12px] text-muted-foreground/72">
          <Route className="size-3.5" />
          <span>{t("activity.modelRerouted", { model: run.actualModel })}</span>
          {run.rerouteReason ? <span className="truncate">{run.rerouteReason}</span> : null}
        </div>
      ) : null}
    </div>
  );
}

function collectChangedFiles(run: AgentRunSnapshot): AgentFileChange[] {
  const byPath = new Map<string, AgentFileChange>();
  for (const file of run.files) byPath.set(file.path, file);
  for (const item of run.items) {
    if (item.kind !== "file") continue;
    for (const file of item.files) byPath.set(file.path, file);
  }
  return [...byPath.values()];
}

export function MessageAgentFileSummary({
  run,
  onOpenDiff,
}: {
  run: AgentRunSnapshot;
  onOpenDiff?: (input: { diff: string; files: AgentFileChange[]; truncated: boolean }) => void;
}) {
  const t = useTranslations("chat.agent");
  const files = collectChangedFiles(run);
  if (run.status === "running" || run.status === "waiting_interaction" || files.length === 0 && !run.diff) return null;

  return (
    <div className="mt-5 w-full border-t border-border/35 pt-3">
      <Button
        type="button"
        variant="ghost"
        className="h-auto max-w-full justify-start gap-2 px-0 py-1 text-[12px] font-medium text-muted-foreground hover:bg-transparent hover:text-foreground"
        disabled={!onOpenDiff}
        onClick={() => onOpenDiff?.({ diff: run.diff, files, truncated: run.diffTruncated })}
      >
        <GitCompareArrows className="size-3.5 shrink-0" />
        <span className="truncate">{t("activity.fileChanges", { count: files.length })}</span>
      </Button>
      {files.length > 0 ? (
        <ul className="mt-1 space-y-0.5 pl-5 font-mono text-[11px] text-muted-foreground/72">
          {files.map((file) => (
            <li key={file.fileID} className="flex min-w-0 items-center gap-2">
              <span className="min-w-0 flex-1 truncate" title={file.path}>{file.path}</span>
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
