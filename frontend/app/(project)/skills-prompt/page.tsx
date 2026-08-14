"use client";

import { useChatSession } from "@/features/chat";
import { AgentPluginsPage } from "@/features/devices/components/agent-plugins-page";
import { SkillsPromptPage } from "@/features/prompts/components/sections/skills-prompt-page";

export default function Page() {
  const { executionMode } = useChatSession();

  return executionMode === "gateway" ? <AgentPluginsPage /> : <SkillsPromptPage />;
}
