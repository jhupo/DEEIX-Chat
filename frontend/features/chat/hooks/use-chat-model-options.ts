"use client";

import * as React from "react";
import { useTranslations } from "next-intl";

import type {
  ChatModelOption,
  ModelOptionControl,
  ModelOptionControlType,
} from "@/features/chat/types/chat-runtime";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { parseProtocolsJSON } from "@/shared/lib/model-protocols";
import { resolveChatProtocol } from "@/shared/model/chat-protocol";
import { sanitizeConversationOptions } from "@/features/chat/model/conversation-options";
import {
  DEFAULT_CHAT_CONTENT_WIDTH,
  parseChatContentWidth,
  type ChatContentWidth,
} from "@/shared/model/chat-content-width";
import { listConversationRuns } from "@/shared/api/conversation";
import { listPublicModels } from "@/shared/api/model";
import { getMCPPolicy, getModelOptionPolicy } from "@/shared/api/settings";
import type { PublicModelDTO } from "@/shared/api/model.types";
import type { ModelOptionPolicy } from "@/shared/lib/model-option-policy";
import { parseKindsJSON } from "@/shared/model/llm-schema";
import { resolveConversationDefaultModel } from "@/shared/model/conversation-default-model";
import type { ConversationOptions } from "@/shared/api/conversation.types";
import type { SendShortcut } from "@/features/settings/types/settings";
import { parseSendShortcut } from "@/features/settings/utils/chat-settings";
import { useUserSettings } from "@/shared/model/user-settings-store";

type ModelCatalogRefreshResult = {
  models: PublicModelDTO[];
  modelOptionPolicy: ModelOptionPolicy | null;
};

function parseJSONObject(raw: string): Record<string, unknown> | null {
  const normalized = raw.trim();
  if (!normalized) {
    return null;
  }
  try {
    const parsed = JSON.parse(normalized) as unknown;
    if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object") {
      return null;
    }
    return parsed as Record<string, unknown>;
  } catch {
    return null;
  }
}

function resolveChatContentWidth(settings: Record<string, string>): ChatContentWidth {
  return parseChatContentWidth(settings["chat.content_width"]);
}

function resolveDefaultOptions(raw: string): ConversationOptions {
  const parsed = parseJSONObject(raw);
  if (!parsed) {
    return {};
  }
  const defaults = parsed.defaultOptions;
  const defaultOptions = defaults === null || Array.isArray(defaults) || typeof defaults !== "object"
    ? {}
    : sanitizeConversationOptions(defaults as ConversationOptions);
  return defaultOptions;
}

const MODEL_OPTION_CONTROL_TYPES = new Set<ModelOptionControlType>(["boolean", "number", "select", "text"]);

function normalizeOptionControlPath(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  return value
    .split(".")
    .map((segment) => segment.trim())
    .filter(Boolean)
    .join(".");
}

function normalizeOptionControlType(value: unknown): ModelOptionControlType | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const normalized = value.trim();
  if (!MODEL_OPTION_CONTROL_TYPES.has(normalized as ModelOptionControlType)) {
    return undefined;
  }
  return normalized as ModelOptionControlType;
}

function normalizeOptionControlString(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const normalized = value.trim();
  return normalized || undefined;
}

function normalizeOptionControlOptions(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const options = Array.from(
    new Set(
      value
        .map((item) => (typeof item === "string" ? item.trim() : ""))
        .filter(Boolean),
    ),
  );
  return options.length > 0 ? options : undefined;
}

function resolveLockedOptionPaths(raw: string): string[] {
  const parsed = parseJSONObject(raw);
  const rawPaths = parsed?.lockedOptionPaths;
  if (!Array.isArray(rawPaths)) {
    return [];
  }
  return Array.from(
    new Set(
      rawPaths
        .map((item) => normalizeOptionControlPath(item))
        .filter(Boolean),
    ),
  );
}

function resolveOptionControls(raw: string): ModelOptionControl[] {
  const parsed = parseJSONObject(raw);
  const rawControls = parsed?.optionControls;
  if (!Array.isArray(rawControls)) {
    return [];
  }
  const lockedPaths = new Set(resolveLockedOptionPaths(raw));

  const controls = rawControls.flatMap((item): ModelOptionControl[] => {
    if (item === null || Array.isArray(item) || typeof item !== "object") {
      return [];
    }
    const source = item as Record<string, unknown>;
    const path = normalizeOptionControlPath(source.path);
    if (!path) {
      return [];
    }
    const control: ModelOptionControl = { path };
    if (lockedPaths.has(path)) {
      control.locked = true;
    }
    const type = normalizeOptionControlType(source.type);
    const label = normalizeOptionControlString(source.label);
    const description = normalizeOptionControlString(source.description);
    const placeholder = normalizeOptionControlString(source.placeholder);
    const options = normalizeOptionControlOptions(source.options);
    if (type) {
      control.type = type;
    }
    if (label) {
      control.label = label;
    }
    if (description) {
      control.description = description;
    }
    if (placeholder) {
      control.placeholder = placeholder;
    }
    if (options) {
      control.options = options;
    }
    return [control];
  });

  return controls.filter((item, index) => controls.findIndex((candidate) => candidate.path === item.path) === index);
}

function resolveVideoExtensionConfig(raw: string, protocols: string[]): ChatModelOption["videoExtension"] {
  const parsed = parseJSONObject(raw);
  const mediaTasks = parsed?.mediaTasks;
  const taskSource = mediaTasks && typeof mediaTasks === "object" && !Array.isArray(mediaTasks)
    ? (mediaTasks as Record<string, unknown>).video_extension
    : undefined;
  const task = taskSource && typeof taskSource === "object" && !Array.isArray(taskSource)
    ? (taskSource as Record<string, unknown>)
    : null;
  if (!protocols.includes("xai_video_extensions") || task?.enabled === false) {
    return null;
  }
  const defaultOptions = task?.defaultOptions && typeof task.defaultOptions === "object" && !Array.isArray(task.defaultOptions)
    ? sanitizeConversationOptions(task.defaultOptions as ConversationOptions)
    : { duration: 6 };
  const rawControls = Array.isArray(task?.optionControls)
    ? task.optionControls
    : [{ path: "duration", type: "select", options: ["2", "3", "4", "5", "6", "7", "8", "9", "10"] }];
  const optionControls = rawControls.flatMap((item): ModelOptionControl[] => {
    if (!item || typeof item !== "object" || Array.isArray(item)) return [];
    const source = item as Record<string, unknown>;
    if (normalizeOptionControlPath(source.path) !== "duration") return [];
    return [{
      path: "duration",
      type: normalizeOptionControlType(source.type) ?? "select",
      label: normalizeOptionControlString(source.label),
      description: normalizeOptionControlString(source.description),
      options: normalizeOptionControlOptions(source.options) ?? ["2", "3", "4", "5", "6", "7", "8", "9", "10"],
    }];
  });
  return { enabled: true, defaultOptions, optionControls };
}

function resolveMCPMaxSelectedTools(value: unknown): number {
  const numeric = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) {
    return 32;
  }
  return Math.min(Math.floor(numeric), 128);
}

function toChatModelOption(item: PublicModelDTO): ChatModelOption {
  const protocols = parseProtocolsJSON(item.protocolsJSON);
  return {
    platformModelName: item.platformModelName,
    icon: item.icon,
    vendor: item.vendor,
    vendorName: item.vendorName,
    vendorIcon: item.vendorIcon,
    displayGroupID: item.displayGroupID,
    displayGroupName: item.displayGroupName,
    displayGroupIcon: item.displayGroupIcon,
    kinds: parseKindsJSON(item.kindsJSON),
    protocols,
    defaultOptions: resolveDefaultOptions(item.capabilitiesJSON),
    optionControls: resolveOptionControls(item.capabilitiesJSON),
    lockedOptionPaths: resolveLockedOptionPaths(item.capabilitiesJSON),
    videoExtension: resolveVideoExtensionConfig(item.capabilitiesJSON, protocols),
  };
}

export function useChatModelOptions({
  conversationPublicID,
  conversationModel,
  resetToken,
  groupPlatform,
}: {
  conversationPublicID: string | null;
  conversationModel?: string | null;
  resetToken?: number;
  groupPlatform?: string;
}) {
  const t = useTranslations("chat.models");
  const userSettings = useUserSettings();
  const [availableModels, setAvailableModels] = React.useState<PublicModelDTO[]>([]);
  const [modelsLoading, setModelsLoading] = React.useState(true);
  const [modelsErrorMsg, setModelsErrorMsg] = React.useState("");
  const [selectedPlatformModelName, setSelectedPlatformModelName] = React.useState("");
  const [userDefaultModel, setUserDefaultModel] = React.useState("");
  const [sendShortcut, setSendShortcut] = React.useState<SendShortcut>("enter");
  const [restoreDraftOnFailure, setRestoreDraftOnFailure] = React.useState(true);
  const [preserveConversationDrafts, setPreserveConversationDrafts] = React.useState(true);
  const [inputHeight, setInputHeight] = React.useState<"compact" | "standard" | "loose">("standard");
  const [contentWidth, setContentWidth] = React.useState<ChatContentWidth>(DEFAULT_CHAT_CONTENT_WIDTH);
  const [markdownRender, setMarkdownRender] = React.useState(true);
  const [showModelInfo, setShowModelInfo] = React.useState(true);
  const [showLatency, setShowLatency] = React.useState(true);
  const [showTokenUsage, setShowTokenUsage] = React.useState(true);
  const [modelOptionPolicy, setModelOptionPolicy] = React.useState<ModelOptionPolicy | null>(null);
  const [mcpMaxSelectedTools, setMCPMaxSelectedTools] = React.useState(32);
  const activeConversationRef = React.useRef<string | null>(null);
  const userSelectedModelRef = React.useRef(false);
  const runModelRequestRef = React.useRef(0);
  const modelCatalogRequestRef = React.useRef<Promise<ModelCatalogRefreshResult> | null>(null);
  const compatibleModels = React.useMemo(
    () => availableModels.filter(
      (model) => parseKindsJSON(model.kindsJSON).includes("chat") &&
        Boolean(resolveChatProtocol(groupPlatform ?? "", parseProtocolsJSON(model.protocolsJSON))),
    ),
    [availableModels, groupPlatform],
  );

  const selectPlatformModelName = React.useCallback((platformModelName: string) => {
    userSelectedModelRef.current = true;
    setSelectedPlatformModelName(platformModelName);
  }, []);

  const loadModelCatalog = React.useCallback((accessToken?: string): Promise<ModelCatalogRefreshResult> => {
    if (modelCatalogRequestRef.current) return modelCatalogRequestRef.current;

    let request: Promise<ModelCatalogRefreshResult>;
    request = (async () => {
      const token = accessToken?.trim() || await resolveAccessToken();
      if (!token) {
        throw new Error("missing access token");
      }

      const [models, modelOptionPolicy] = await Promise.all([
        listPublicModels(token),
        getModelOptionPolicy(token).catch(() => null),
      ]);
      return { models, modelOptionPolicy };
    })().finally(() => {
      if (modelCatalogRequestRef.current === request) modelCatalogRequestRef.current = null;
    });

    modelCatalogRequestRef.current = request;
    return request;
  }, []);

  const applyModelCatalog = React.useCallback((catalog: ModelCatalogRefreshResult) => {
    setAvailableModels(catalog.models);
    setModelOptionPolicy(catalog.modelOptionPolicy);
  }, []);

  const refreshModelCatalog = React.useCallback(async (): Promise<PublicModelDTO[]> => {
    const catalog = await loadModelCatalog();
    applyModelCatalog(catalog);
    setModelsErrorMsg("");
    return catalog.models;
  }, [applyModelCatalog, loadModelCatalog]);

  React.useEffect(() => {
    let cancelled = false;

    async function loadModels() {
      setModelsLoading(true);
      setModelsErrorMsg("");
      try {
        const token = await resolveAccessToken();
        if (!token) {
          setModelsErrorMsg(t("signInRequired"));
          return;
        }
        const [catalog, nextMCPPolicy] = await Promise.all([
          loadModelCatalog(token),
          getMCPPolicy(token).catch(() => null),
        ]);
        if (cancelled) {
          return;
        }
        applyModelCatalog(catalog);
        setMCPMaxSelectedTools(resolveMCPMaxSelectedTools(nextMCPPolicy?.maxSelectedToolsPerMessage));
      } catch {
        if (!cancelled) {
          setModelsErrorMsg(t("loadFailed"));
        }
      } finally {
        if (!cancelled) {
          setModelsLoading(false);
        }
      }
    }

    void loadModels();
    return () => {
      cancelled = true;
    };
  }, [applyModelCatalog, loadModelCatalog, t]);

  React.useEffect(() => {
    const settings = userSettings.settings;
    setUserDefaultModel(settings["chat.default_model"]?.trim() ?? "");
    setSendShortcut(parseSendShortcut(settings["chat.send_on_enter"]));
    setRestoreDraftOnFailure(settings["chat.restore_draft_on_failure"] !== "false");
    setPreserveConversationDrafts(settings["chat.preserve_conversation_drafts"] !== "false");
    setMarkdownRender(settings["chat.markdown_render"] !== "false");
    setShowModelInfo(settings["chat.show_model_info"] !== "false");
    setShowLatency(settings["chat.show_latency"] !== "false");
    setShowTokenUsage(settings["chat.show_token_usage"] !== "false");
    setInputHeight(
      settings["chat.input_height"] === "compact" || settings["chat.input_height"] === "loose"
        ? settings["chat.input_height"]
        : "standard",
    );
    setContentWidth(resolveChatContentWidth(settings));
  }, [userSettings.settings]);

  React.useEffect(() => {
    const normalizedConversationID = conversationPublicID?.trim() || null;
    if (!normalizedConversationID) {
      // 无会话状态也可能来自当前页点击“新对话”，要保留用户刚在选择器里切换的模型。
      activeConversationRef.current = null;
      return;
    }
    const targetConversationID = normalizedConversationID;

    const conversationChanged = activeConversationRef.current !== normalizedConversationID;
    if (conversationChanged) {
      activeConversationRef.current = normalizedConversationID;
      userSelectedModelRef.current = false;
    }

    const fallbackModel = conversationModel?.trim() || "";
    if (!userSelectedModelRef.current) {
      setSelectedPlatformModelName(fallbackModel);
    }

    let cancelled = false;
    const requestID = runModelRequestRef.current + 1;
    runModelRequestRef.current = requestID;

    async function loadLatestRunModel() {
      const token = await resolveAccessToken();
      if (!token) {
        return;
      }

      const runs = await listConversationRuns(token, targetConversationID, { page: 1, pageSize: 1 });
      if (cancelled || requestID !== runModelRequestRef.current || userSelectedModelRef.current) {
        return;
      }

      const latestRunModel = runs.results[0]?.platformModelName?.trim() || "";
      setSelectedPlatformModelName(latestRunModel || fallbackModel);
    }

    void loadLatestRunModel().catch(() => undefined);

    return () => {
      cancelled = true;
    };
  }, [conversationModel, conversationPublicID, resetToken]);

  React.useEffect(() => {
    if (compatibleModels.length === 0) {
      return;
    }
    if (conversationPublicID?.trim()) {
      return;
    }

    let cancelled = false;
    async function applyDefaultModel() {
      const token = await resolveAccessToken();
      if (!token || cancelled || userSelectedModelRef.current) {
        return;
      }
      const result = await resolveConversationDefaultModel({
        accessToken: token,
        availableModels: compatibleModels,
        userDefaultModel,
      });
      if (!cancelled && !userSelectedModelRef.current) {
        setSelectedPlatformModelName(result.platformModelName);
      }
    }

    void applyDefaultModel().catch(() => {
      if (!cancelled && !userSelectedModelRef.current) {
        setSelectedPlatformModelName(compatibleModels[0]?.platformModelName ?? "");
      }
    });
    return () => {
      cancelled = true;
    };
  }, [compatibleModels, conversationPublicID, resetToken, userDefaultModel]);

  React.useEffect(() => {
    if (compatibleModels.length === 0) {
      setSelectedPlatformModelName("");
      return;
    }
    setSelectedPlatformModelName((current) =>
      compatibleModels.some((item) => item.platformModelName === current)
        ? current
        : (compatibleModels[0]?.platformModelName ?? ""),
    );
  }, [compatibleModels]);

  const modelOptions = React.useMemo<ChatModelOption[]>(
    () =>
      compatibleModels.map(toChatModelOption),
    [compatibleModels],
  );

  return {
    modelOptions,
    refreshModelCatalog,
    modelsLoading,
    modelsErrorMsg,
    sendShortcut,
    restoreDraftOnFailure,
    preserveConversationDrafts,
    inputHeight,
    contentWidth,
    markdownRender,
    showModelInfo,
    showLatency,
    showTokenUsage,
    modelOptionPolicy,
    mcpMaxSelectedTools,
    selectedPlatformModelName,
    setSelectedPlatformModelName: selectPlatformModelName,
  };
}
