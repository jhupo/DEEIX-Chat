export const CHAT_PROTOCOLS = [
  "openai_responses",
  "openai_chat_completions",
  "openrouter_responses",
  "openrouter_chat_completions",
  "anthropic_messages",
  "google_generate_content",
  "xai_responses",
] as const;

export type ChatProtocol = (typeof CHAT_PROTOCOLS)[number];

const PROTOCOLS_BY_GROUP_PLATFORM: Record<string, ChatProtocol[]> = {
  anthropic: ["anthropic_messages"],
  composite: [
    "openai_responses",
    "openrouter_responses",
    "anthropic_messages",
    "google_generate_content",
    "xai_responses",
    "openai_chat_completions",
    "openrouter_chat_completions",
  ],
  openai: ["openai_responses", "openai_chat_completions"],
  grok: ["xai_responses", "openai_responses", "openai_chat_completions"],
  gemini: ["google_generate_content", "openai_chat_completions"],
  antigravity: ["google_generate_content", "openai_chat_completions"],
};

export function resolveChatProtocol(groupPlatform: string, modelProtocols: string[]): ChatProtocol | "" {
  const platform = groupPlatform.trim().toLowerCase();
  const allowed = PROTOCOLS_BY_GROUP_PLATFORM[platform] ?? [];
  const configured = new Set(modelProtocols.map((protocol) => protocol.trim().toLowerCase()));
  return allowed.find((protocol) => configured.has(protocol)) ?? "";
}
