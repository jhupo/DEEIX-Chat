"use client";

import * as React from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import type { ChatSettings } from "@/features/settings/types/settings";
import {
  DEFAULT_CHAT_SETTINGS,
  groupModelsForPresentation,
  parseChatSettings,
} from "@/features/settings/utils/chat-settings";
import { useAuthSession } from "@/shared/auth/auth-session-context";
import { listPublicModels } from "@/shared/api/model";
import { getChatContextPolicy } from "@/shared/api/settings";
import type { PublicModelDTO } from "@/shared/api/model.types";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import { updateUserSettings, useUserSettings } from "@/shared/model/user-settings-store";

type UseSettingsChatResult = {
  settings: ChatSettings;
  loading: boolean;
  contextCompressionEnabled: boolean;
  modelGroups: ReturnType<typeof groupModelsForPresentation>;
  handleBool: (key: string, field: keyof ChatSettings) => (checked: boolean) => void;
  handleEnum: (key: string, field: keyof ChatSettings) => (value: string) => void;
  handleDefaultModel: (value: string) => void;
};

export function useSettingsChat(): UseSettingsChatResult {
  const t = useTranslations("settings.chatPage.toasts");
  const translateError = useLocalizedErrorMessage();
  const { accessToken } = useAuthSession();
  const userSettings = useUserSettings();
  const [models, setModels] = React.useState<PublicModelDTO[]>([]);
  const [metadataLoading, setMetadataLoading] = React.useState(true);
  const [contextCompressionEnabled, setContextCompressionEnabled] = React.useState(false);

  React.useEffect(() => {
    let cancelled = false;

    void (async () => {
      try {
        const [modelList, contextPolicy] = await Promise.all([
          listPublicModels(accessToken).catch((): PublicModelDTO[] => []),
          getChatContextPolicy(accessToken).catch(() => ({ contextCompactEnabled: false })),
        ]);

        if (cancelled) {
          return;
        }

        setModels(modelList);
        setContextCompressionEnabled(contextPolicy.contextCompactEnabled);
      } finally {
        if (!cancelled) {
          setMetadataLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [accessToken]);

  const settings = React.useMemo(
    () => userSettings.loaded ? parseChatSettings(userSettings.settings) : DEFAULT_CHAT_SETTINGS,
    [userSettings.loaded, userSettings.settings],
  );
  const modelGroups = React.useMemo(() => groupModelsForPresentation(models), [models]);

  const persistSetting = React.useCallback(
    (_field: keyof ChatSettings, key: string, value: string) => {
      void updateUserSettings(accessToken, { [key]: value })
        .catch((error) => {
          toast.error(t("saveFailed"), { description: translateError(error, t("retryLater")) });
        });
    },
    [accessToken, t, translateError],
  );

  const handleBool = React.useCallback(
    (key: string, field: keyof ChatSettings) => (checked: boolean) => {
      persistSetting(field, key, checked ? "true" : "false");
    },
    [persistSetting],
  );

  const handleEnum = React.useCallback(
    (key: string, field: keyof ChatSettings) => (value: string) => {
      persistSetting(field, key, value);
    },
    [persistSetting],
  );

  const handleDefaultModel = React.useCallback(
    (value: string) => {
      const code = value === "none" ? "" : value;
      persistSetting("defaultModel", "chat.default_model", code);
    },
    [persistSetting],
  );

  return {
    settings,
    loading: metadataLoading || !userSettings.loaded,
    contextCompressionEnabled,
    modelGroups,
    handleBool,
    handleEnum,
    handleDefaultModel,
  };
}
