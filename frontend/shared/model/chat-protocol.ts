export const CHAT_PROTOCOLS = [
  "openai_chat_completions",
  "openai_responses",
  "anthropic_messages",
] as const;

export type ChatProtocol = (typeof CHAT_PROTOCOLS)[number];

export const DEFAULT_CHAT_PROTOCOL: ChatProtocol = "openai_chat_completions";

export function parseChatProtocol(value: string | undefined): ChatProtocol {
  return CHAT_PROTOCOLS.includes(value as ChatProtocol) ? value as ChatProtocol : DEFAULT_CHAT_PROTOCOL;
}

export function chatProtocolsForGroupPlatform(platform: string): ChatProtocol[] {
  switch (platform.trim().toLowerCase()) {
    case "anthropic":
      return ["anthropic_messages"];
    case "composite":
      return [...CHAT_PROTOCOLS];
    case "openai":
    case "grok":
      return ["openai_chat_completions", "openai_responses"];
    case "gemini":
    case "antigravity":
      return ["openai_chat_completions"];
    default:
      return [];
  }
}

export function resolveChatProtocol(platform: string, configured: string | undefined): ChatProtocol {
  const supported = chatProtocolsForGroupPlatform(platform);
  const protocol = parseChatProtocol(configured);
  return supported.includes(protocol) ? protocol : supported[0] ?? DEFAULT_CHAT_PROTOCOL;
}
