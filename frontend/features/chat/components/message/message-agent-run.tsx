"use client";

import { useLocale, useTranslations } from "next-intl";

import { Marker, MarkerContent } from "@/components/ui/marker";
import {
  formatAgentRunDuration,
  MessageAgentActivity,
} from "@/features/chat/components/message/message-agent-activity";
import {
  type AgentRunSnapshot,
  hasAgentRunActivity,
} from "@/features/chat/model/agent-run-store";
import { cn } from "@/lib/utils";

const AGENT_RUN_ROOT_CLASS = "chat-screenshot-omit mb-2 w-full max-w-[900px]";

export function MessageAgentRun({
  agentRun,
}: {
  agentRun: AgentRunSnapshot;
}) {
  const t = useTranslations("chat.agent");
  const locale = useLocale();
  const active = agentRun.status === "running" || agentRun.status === "waiting_interaction";

  if (!hasAgentRunActivity(agentRun)) {
    return null;
  }

  const duration = formatAgentRunDuration(agentRun.durationMS, locale);
  const title = active
    ? t("activity.title.running")
    : duration
      ? t("activity.elapsed", { duration })
      : t(
          `activity.title.${agentRun.status === "idle" || agentRun.status === "waiting_interaction" ? "completed" : agentRun.status}`,
        );

  return (
    <div className={AGENT_RUN_ROOT_CLASS}>
      <h3 className="flex min-h-0 items-center py-1.5 text-left">
        <Marker
          render={<span />}
          className={cn(
            "inline-flex min-h-0 w-auto text-[13px] font-medium transition-colors",
            !active && "text-muted-foreground",
          )}
        >
          <MarkerContent className={cn("min-w-0", active && "shimmer")}>{title}</MarkerContent>
        </Marker>
      </h3>
      <MessageAgentActivity run={agentRun} showPlan={false} />
    </div>
  );
}
