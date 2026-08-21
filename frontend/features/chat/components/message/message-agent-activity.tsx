"use client";

import { ChevronDown, FileCode2, GitCompareArrows, ListChecks, Route, TerminalSquare } from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";

import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Badge } from "@/components/ui/badge";
import { Marker, MarkerContent } from "@/components/ui/marker";
import { TRACE_ROOT_CLASS } from "@/features/chat/components/shared/message-process-trace-shared";
import {
  type AgentActivityItem,
  type AgentRunSnapshot,
  useAgentRunSnapshot,
} from "@/features/chat/model/agent-run-store";
import { cn } from "@/lib/utils";

function statusKey(status: string): "running" | "completed" | "failed" | "interrupted" | "pending" | "inProgress" {
  if (status === "completed") return "completed";
  if (status === "failed") return "failed";
  if (status === "interrupted") return "interrupted";
  if (status === "inProgress") return "inProgress";
  if (status === "pending") return "pending";
  return "running";
}

function ActivityStatusBadge({ status }: { status: string }) {
  const t = useTranslations("chat.agent");
  return (
    <Badge
      variant="outline"
      className={cn("h-5 px-1.5 text-[10px] font-normal", status === "failed" && "border-destructive/30 text-destructive")}
    >
      {t(`activity.status.${statusKey(status)}`)}
    </Badge>
  );
}

function ActivitySection({
  icon,
  title,
  status,
  detail,
  children,
}: {
  icon: React.ReactNode;
  title: string;
  status?: string;
  detail?: string;
  children?: React.ReactNode;
}) {
  return (
    <section className="border-t border-border/30 py-2 first:border-t-0 first:pt-0 last:pb-0">
      <div className="flex min-w-0 items-center gap-2 text-[12px] leading-5">
        <span className="text-muted-foreground/62">{icon}</span>
        <span className="min-w-0 flex-1 truncate font-medium text-muted-foreground/82">{title}</span>
        {status ? <ActivityStatusBadge status={status} /> : null}
      </div>
      {detail ? <p className="mt-1 whitespace-pre-wrap break-words pl-5 text-[12px] leading-5 text-muted-foreground/68">{detail}</p> : null}
      {children ? <div className="mt-1.5 pl-5">{children}</div> : null}
    </section>
  );
}

function CommandContent({ item }: { item: Extract<AgentActivityItem, { kind: "command" }> }) {
  const t = useTranslations("chat.agent");
  return (
    <ActivitySection icon={<TerminalSquare className="size-3.5" />} title={item.command || t("activity.commandFallback")} status={item.status}>
      {item.output ? (
        <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border/35 bg-muted/25 px-2.5 py-2 font-mono text-[11px] leading-5 text-muted-foreground/88">{item.output}</pre>
      ) : null}
      {item.outputTruncated ? <p className="mt-1 text-[11px] text-muted-foreground/60">{t("activity.truncated")}</p> : null}
      {item.exitCode !== null ? <p className="mt-1 text-[11px] text-muted-foreground/60">{t("activity.exitCode", { code: item.exitCode })}</p> : null}
    </ActivitySection>
  );
}

function FileContent({ item }: { item: Extract<AgentActivityItem, { kind: "file" }> }) {
  const t = useTranslations("chat.agent");
  return (
    <ActivitySection icon={<FileCode2 className="size-3.5" />} title={t("activity.fileChanges", { count: item.files.length })} status={item.status}>
      {item.files.length > 0 ? (
        <ul className="space-y-2">
          {item.files.map((file) => (
            <li key={file.fileID} className="min-w-0 font-mono text-[11px] text-muted-foreground/82">
              <div className="flex min-w-0 items-center gap-2">
                <span className="min-w-0 flex-1 truncate" title={file.path}>{file.path}</span>
                {file.additions !== null ? <span className="shrink-0 text-foreground/65">+{file.additions}</span> : null}
                {file.deletions !== null ? <span className="shrink-0 text-destructive/70">-{file.deletions}</span> : null}
              </div>
              {file.diff ? <pre className="mt-1 max-h-64 overflow-auto whitespace-pre leading-5">{file.diff}</pre> : null}
              {file.truncated ? <p className="mt-1 font-sans text-[11px] text-muted-foreground/60">{t("activity.truncated")}</p> : null}
            </li>
          ))}
        </ul>
      ) : null}
      {item.diff ? <pre className="mt-1.5 max-h-64 overflow-auto whitespace-pre font-mono text-[11px] leading-5 text-muted-foreground/82">{item.diff}</pre> : null}
      {item.truncated ? <p className="mt-1 text-[11px] text-muted-foreground/60">{t("activity.truncated")}</p> : null}
    </ActivitySection>
  );
}

function ActivityRows({ run }: { run: AgentRunSnapshot }) {
  const t = useTranslations("chat.agent");
  return (
    <div>
      {run.plan.length > 0 ? (
        <ActivitySection icon={<ListChecks className="size-3.5" />} title={t("activity.plan", { count: run.plan.length })} detail={run.planExplanation}>
          <ol className="space-y-1">
            {run.plan.map((step) => (
              <li key={step.key} className="flex min-w-0 items-start gap-2 text-[12px] leading-5 text-muted-foreground/82">
                <ActivityStatusBadge status={step.status} />
                <span className="min-w-0 break-words">{step.text}</span>
              </li>
            ))}
          </ol>
        </ActivitySection>
      ) : null}
      {run.items.map((item) => item.kind === "command" ? <CommandContent key={item.itemID} item={item} /> : <FileContent key={item.itemID} item={item} />)}
      {run.diff && !run.items.some((item) => item.kind === "file" && item.diff === run.diff) ? (
        <ActivitySection icon={<GitCompareArrows className="size-3.5" />} title={t("activity.diff")}>
          <pre className="max-h-64 overflow-auto whitespace-pre font-mono text-[11px] leading-5 text-muted-foreground/82">{run.diff}</pre>
          {run.diffTruncated ? <p className="mt-1 text-[11px] text-muted-foreground/60">{t("activity.truncated")}</p> : null}
        </ActivitySection>
      ) : null}
      {run.actualModel ? <ActivitySection icon={<Route className="size-3.5" />} title={t("activity.modelRerouted", { model: run.actualModel })} detail={run.rerouteReason} /> : null}
      {run.usage ? (
        <ActivitySection
          icon={<span className="font-mono text-[10px]">#</span>}
          title={t("activity.tokenUsage")}
          detail={t("activity.tokenSummary", { input: run.usage.inputTokens, output: run.usage.outputTokens, total: run.usage.totalTokens })}
        />
      ) : null}
    </div>
  );
}

export function MessageAgentActivity({ runID }: { runID: string | undefined }) {
  const t = useTranslations("chat.agent");
  const run = useAgentRunSnapshot(runID);
  const active = run.status === "running" || run.status === "waiting_interaction";
  const hasActivity = run.status !== "idle" || run.plan.length > 0 || run.items.length > 0 || Boolean(run.diff || run.actualModel || run.usage);
  const [open, setOpen] = React.useState(false);
  const manuallyCollapsed = React.useRef(false);

  React.useEffect(() => {
    if (active && !manuallyCollapsed.current) setOpen(true);
  }, [active, run.lastExecutionSeq]);

  if (!hasActivity) return null;
  const commands = run.items.filter((item) => item.kind === "command").length;
  const files = new Set([...run.files.map((file) => file.path), ...run.items.flatMap((item) => item.kind === "file" ? item.files.map((file) => file.path) : [])]).size;
  const titleKey = active ? "running" : statusKey(run.status);

  return (
    <div className={TRACE_ROOT_CLASS}>
      <Accordion type="single" collapsible value={open ? "agent-activity" : ""} onValueChange={(value) => {
        const nextOpen = value === "agent-activity";
        manuallyCollapsed.current = active && !nextOpen;
        setOpen(nextOpen);
      }}>
        <AccordionItem value="agent-activity" className="border-b-0">
          <AccordionTrigger iconPosition="none" className="group/agent-activity min-h-0 justify-between gap-2 py-0.5 no-underline hover:no-underline">
            <div className="min-w-0 flex-1">
              <Marker render={<span />} className="inline-flex min-h-0 w-auto text-[13px] font-medium">
                <MarkerContent className={cn("min-w-0", active && "shimmer")}>{t(`activity.title.${titleKey}`)}</MarkerContent>
              </Marker>
              <div className="mt-0.5 flex min-w-0 items-center gap-2 truncate text-[11px] text-muted-foreground/62">
                {run.plan.length > 0 ? <span className="inline-flex items-center gap-1"><ListChecks className="size-3" />{run.plan.length}</span> : null}
                {commands > 0 ? <span className="inline-flex items-center gap-1"><TerminalSquare className="size-3" />{commands}</span> : null}
                {files > 0 ? <span className="inline-flex items-center gap-1"><FileCode2 className="size-3" />{files}</span> : null}
                {run.diff ? <GitCompareArrows className="size-3" /> : null}
                {run.actualModel ? <Route className="size-3" /> : null}
              </div>
            </div>
            <ChevronDown className={cn("size-3.5 shrink-0 text-muted-foreground transition-transform", open && "rotate-180")} />
          </AccordionTrigger>
          <AccordionContent className="px-0 pb-0 pt-1.5"><ActivityRows run={run} /></AccordionContent>
        </AccordionItem>
      </Accordion>
    </div>
  );
}
