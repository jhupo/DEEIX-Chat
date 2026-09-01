"use client";

import { useLocale, useTranslations } from "next-intl";
import * as React from "react";

import { ChevronDown } from "@/components/animate-ui/icons/chevron-down";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
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
const AGENT_RUN_ACCORDION_VALUE = "message-agent-run";

export function MessageAgentRun({
  agentRun,
}: {
  agentRun: AgentRunSnapshot;
}) {
  const t = useTranslations("chat.agent");
  const locale = useLocale();
  const active = agentRun.status === "running" || agentRun.status === "waiting_interaction";
  const [accordionValue, setAccordionValue] = React.useState(() =>
    active ? AGENT_RUN_ACCORDION_VALUE : "",
  );

  React.useEffect(() => {
    if (active) {
      setAccordionValue(AGENT_RUN_ACCORDION_VALUE);
      return;
    }
    setAccordionValue("");
  }, [active]);

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
  const open = accordionValue === AGENT_RUN_ACCORDION_VALUE;

  return (
    <div className={AGENT_RUN_ROOT_CLASS}>
      <Accordion
        type="single"
        collapsible
        value={accordionValue}
        onValueChange={(value) => setAccordionValue(value || "")}
        className="w-full"
      >
        <AccordionItem value={AGENT_RUN_ACCORDION_VALUE} className="border-b-0">
          <AccordionTrigger
            iconPosition="none"
            className="group/activity min-h-0 justify-between gap-1.5 border-b border-border/45 py-1.5 text-left no-underline hover:no-underline"
          >
            <Marker
              render={<span />}
              className={cn(
                "inline-flex min-h-0 w-auto text-[13px] font-medium transition-colors",
                !active && "text-muted-foreground group-hover/activity:text-foreground",
              )}
            >
              <MarkerContent className={cn("min-w-0", active && "shimmer")}>{title}</MarkerContent>
            </Marker>
            <ChevronDown
              className={cn(
                "size-3.5 shrink-0 text-muted-foreground transition-transform duration-200 group-hover/activity:text-foreground",
                open && "rotate-180",
              )}
            />
          </AccordionTrigger>
          <AccordionContent className="px-0 pb-0 pt-2 duration-[350ms] ease-in-out">
            <MessageAgentActivity run={agentRun} showPlan={false} />
          </AccordionContent>
        </AccordionItem>
      </Accordion>
    </div>
  );
}
