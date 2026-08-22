import type { ModelOptionControl } from "@/features/chat/types/chat-runtime";
import type { ConversationOptions } from "@/shared/api/conversation.types";

const REASONING_OPTION_VALUES: Record<string, string[]> = {
  effort: ["low", "medium", "high", "xhigh", "max"],
  "output_config.effort": ["low", "medium", "high"],
  "reasoning.effort": ["low", "medium", "high"],
  reasoning_effort: ["minimal", "low", "medium", "high", "xhigh"],
  "generationConfig.thinkingConfig.thinkingLevel": ["low", "medium", "high"],
  "thinking.thinking_level": ["low", "medium", "high"],
  "thinking.thinkingLevel": ["low", "medium", "high"],
  "thinkingConfig.thinkingLevel": ["low", "medium", "high"],
};

export type ChatModelReasoningSetting = {
  path: string;
  value: string;
  options: string[];
  disabled: boolean;
};

function optionAtPath(options: ConversationOptions, path: string): unknown {
  let current: unknown = options;
  for (const segment of path.split(".")) {
    if (current === null || typeof current !== "object" || Array.isArray(current)) {
      return undefined;
    }
    current = (current as Record<string, unknown>)[segment];
  }
  return current;
}

export function resolveChatModelReasoningSetting({
  options,
  defaultOptions,
  optionControls,
  lockedOptionPaths,
}: {
  options: ConversationOptions;
  defaultOptions: ConversationOptions;
  optionControls: ModelOptionControl[];
  lockedOptionPaths: string[];
}): ChatModelReasoningSetting | null {
  const control = optionControls.find((item) => Object.hasOwn(REASONING_OPTION_VALUES, item.path));
  const path = control?.path ?? Object.keys(REASONING_OPTION_VALUES).find(
    (candidate) => optionAtPath(options, candidate) !== undefined || optionAtPath(defaultOptions, candidate) !== undefined,
  );
  if (!path) {
    return null;
  }

  const rawValue = optionAtPath(options, path) ?? optionAtPath(defaultOptions, path);
  const value = typeof rawValue === "string" ? rawValue.trim() : "";
  const configuredOptions = control?.options?.map((item) => item.trim()).filter(Boolean) ?? [];
  const supportedOptions = configuredOptions.length > 0 ? configuredOptions : REASONING_OPTION_VALUES[path];
  const normalizedOptions = Array.from(new Set(value ? [...supportedOptions, value] : supportedOptions));
  if (!value || normalizedOptions.length === 0) {
    return null;
  }

  return {
    path,
    value,
    options: normalizedOptions,
    disabled: control?.locked === true || lockedOptionPaths.includes(path),
  };
}

export function setChatModelReasoningEffort(
  options: ConversationOptions,
  path: string,
  value: string,
): ConversationOptions {
  const segments = path.split(".").filter(Boolean);
  if (segments.length === 0) {
    return options;
  }
  const [segment, ...rest] = segments;
  if (rest.length === 0) {
    return { ...options, [segment]: value };
  }
  const current = options[segment];
  return {
    ...options,
    [segment]: setChatModelReasoningEffort(
      current !== null && typeof current === "object" && !Array.isArray(current)
        ? current as ConversationOptions
        : {},
      rest.join("."),
      value,
    ),
  };
}
