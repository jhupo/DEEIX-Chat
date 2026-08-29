"use client";

import * as React from "react";

import { useUserSettings } from "@/shared/model/user-settings-store";

type ChatPreferences = {
  autoGenerateTitle: boolean;
  autoGenerateLabels: boolean;
  autoExpandThinking: boolean;
  autoExpandToolCalls: boolean;
  deleteFilesByDefault: boolean;
};

type ChatPreferencesState = ChatPreferences & {
  loaded: boolean;
};

const DEFAULT_CHAT_PREFERENCES: ChatPreferences = {
  autoGenerateTitle: true,
  autoGenerateLabels: true,
  autoExpandThinking: true,
  autoExpandToolCalls: true,
  deleteFilesByDefault: false,
};

function resolveChatPreferences(settings: Record<string, string>): ChatPreferences {
  return {
    autoGenerateTitle: settings["chat.auto_generate_title"] !== "false",
    autoGenerateLabels: settings["chat.auto_generate_labels"] !== "false",
    autoExpandThinking: settings["chat.auto_expand_thinking"] !== "false",
    autoExpandToolCalls: settings["chat.auto_expand_tool_calls"] !== "false",
    deleteFilesByDefault: settings["chat.delete_conversation_files_by_default"] === "true",
  };
}

export function useSettingsChatPreferences(): ChatPreferencesState {
  const { settings, loaded } = useUserSettings();
  const preferences = React.useMemo(
    () => loaded ? resolveChatPreferences(settings) : DEFAULT_CHAT_PREFERENCES,
    [loaded, settings],
  );
  return { ...preferences, loaded };
}
