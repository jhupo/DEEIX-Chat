"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  ConversationShareDialog,
  sharePatchFromDTO,
  useConversationExport,
  useSidebarConversations,
} from "@/entities/conversation";
import { ChatArea, ChatAreaLoadError, ChatAreaSkeleton } from "@/features/chat/components/sections/chat-area";
import { ChatArtifactWorkspace } from "@/features/chat/components/sections/chat-artifact";
import { ChatEmptyState } from "@/features/chat/components/sections/chat-empty";
import { ChatInput } from "@/features/chat/components/sections/chat-input";
import { ChatScreenshotPreviewDialog } from "@/features/chat/components/sections/chat-screenshot-preview-dialog";
import { useChatSession } from "@/features/chat/context/chat-session-context";
import { useAgentRunHydration } from "@/features/chat/hooks/use-agent-run-hydration";
import { useChatArtifacts } from "@/features/chat/hooks/use-chat-artifacts";
import { useChatAttachments } from "@/features/chat/hooks/use-chat-attachments";
import { useChatComposerSelection } from "@/features/chat/hooks/use-chat-composer-selection";
import { useChatComposerState } from "@/features/chat/hooks/use-chat-composer-state";
import { useChatData } from "@/features/chat/hooks/use-chat-data";
import { useChatKeyBindings } from "@/features/chat/hooks/use-chat-key-bindings";
import { useChatModelOptions } from "@/features/chat/hooks/use-chat-model-options";
import { useChatRuntime } from "@/features/chat/hooks/use-chat-runtime";
import { useChatScreenshot } from "@/features/chat/hooks/use-chat-screenshot";
import { useChatViewerProfile } from "@/features/chat/hooks/use-chat-viewer-profile";
import { useChatVisualPrompt } from "@/features/chat/hooks/use-chat-visual-prompt";
import { useNewConversationDefaults } from "@/features/chat/hooks/use-new-conversation-defaults";
import {
  cloneConversationOptions,
  isConversationOptionsObject,
  sanitizeConversationOptions,
} from "@/features/chat/model/conversation-options";
import { modelSupportsChatImageTool } from "@/features/chat/model/chat-task";
import { toPendingAttachment } from "@/features/chat/model/message-submit";
import type { ChatAreaMessage, MessageAttachment } from "@/features/chat/types/messages";
import { useDevices } from "@/features/devices";
import { useSettingsChatPreferences } from "@/features/settings/hooks/use-settings-chat-preferences";
import { cn } from "@/lib/utils";
import {
  type AgentModelDTO,
  type AgentProviderManifestDTO,
  type AgentTurnSettings,
  getAgentProfileResource,
  includeCurrentAgentModel,
  listAgentRuntimeProfiles,
  listAgentWorkspaces,
  parseAgentModelsResource,
  refreshAgentProfileResource,
  waitForAgentCommand,
} from "@/shared/api/agent-gateway";
import { getConversation, listConversationInputResources } from "@/shared/api/conversation";
import type { ConversationDTO, ConversationInputResourceDTO, ConversationOptions } from "@/shared/api/conversation.types";
import type { FileObjectDTO } from "@/shared/api/file.types";
import { listAvailableMCPTools } from "@/shared/api/mcp";
import type { MCPToolDTO } from "@/shared/api/mcp.types";
import { getUserSettings, patchUserSettings } from "@/shared/api/user-settings";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { DeleteFilesOption } from "@/shared/components/delete-files-option";
import { parseConversationLabelsJSON } from "@/shared/lib/conversation-labels";
import {
  hasMultipleImageAttachmentProcessors,
  normalizeImageAttachmentProcessorSelection,
} from "@/shared/lib/mcp-tool-selection";
import { resolveChatContentWidthClassName } from "@/shared/model/chat-content-width";
import { resolveChatProtocol } from "@/shared/model/chat-protocol";

const AGENT_SETTINGS_STORAGE_PREFIX = "deeix-chat:agent-settings:v1:";
const AGENT_MODELS_STALE_MS = 5 * 60 * 1000;
const DEFAULT_MCP_TOOLS_SETTING_KEY = "chat.default_mcp_tool_ids";
const EMPTY_CONVERSATION_OPTIONS: ConversationOptions = {};
const TOP_LOAD_OLDER_MESSAGES_THRESHOLD_PX = 48;
const SCREENSHOT_PREVIEW_CLOSE_DELAY_MS = 220;
function dragEventContainsFiles(event: React.DragEvent<HTMLElement>): boolean {
  return Array.from(event.dataTransfer.types ?? []).includes("Files");
}

function droppedFiles(event: React.DragEvent<HTMLElement>): File[] {
  return Array.from(event.dataTransfer.files ?? []).filter((file) => file.name.trim() || file.size > 0);
}

function agentSettingsStorageKey(
  deviceID: string,
  profileID: string,
  workspaceID: string,
  conversationID: string,
): string {
  return `${AGENT_SETTINGS_STORAGE_PREFIX}${encodeURIComponent(deviceID)}:${encodeURIComponent(profileID)}:${encodeURIComponent(workspaceID)}:${encodeURIComponent(conversationID)}`;
}

function readAgentSettings(storageKey: string): AgentTurnSettings | null {
  if (typeof window === "undefined" || !storageKey) {
    return null;
  }
  try {
    const parsed = JSON.parse(window.localStorage.getItem(storageKey) ?? "null") as Partial<AgentTurnSettings> | null;
    if (!parsed || typeof parsed !== "object") {
      return null;
    }
    if (
      typeof parsed.model !== "string" ||
      !["low", "medium", "high", "xhigh"].includes(parsed.reasoningEffort ?? "") ||
      !["on-request", "never"].includes(parsed.approvalPolicy ?? "") ||
      !["user", "auto_review"].includes(parsed.approvalsReviewer ?? "") ||
      !["workspace-write", "danger-full-access"].includes(parsed.sandboxPolicy ?? "")
    ) {
      return null;
    }
    return parsed as AgentTurnSettings;
  } catch {
    return null;
  }
}

function writeAgentSettings(storageKey: string, settings: AgentTurnSettings): void {
  if (typeof window === "undefined" || !storageKey) {
    return;
  }
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(settings));
  } catch {
    // Settings remain valid for the current page when storage is unavailable.
  }
}

function manifestSupportsAutoReview(manifest: AgentProviderManifestDTO): boolean {
  return manifest.threadSettings.approvalsReviewer?.includes("auto_review") === true;
}

type AgentApprovalSettings = Pick<
  AgentTurnSettings,
  "approvalPolicy" | "approvalsReviewer" | "sandboxPolicy"
>;

function parseAgentApprovalSettings(value: {
  approvalPolicy?: string;
  approvalsReviewer?: string;
  sandboxPolicy?: string;
} | null | undefined): AgentApprovalSettings | null {
  if (
    value?.approvalPolicy === "on-request" &&
    value.approvalsReviewer === "user" &&
    value.sandboxPolicy === "workspace-write"
  ) {
    return { approvalPolicy: "on-request", approvalsReviewer: "user", sandboxPolicy: "workspace-write" };
  }
  if (
    value?.approvalPolicy === "on-request" &&
    value.approvalsReviewer === "auto_review" &&
    value.sandboxPolicy === "workspace-write"
  ) {
    return { approvalPolicy: "on-request", approvalsReviewer: "auto_review", sandboxPolicy: "workspace-write" };
  }
  if (
    value?.approvalPolicy === "never" &&
    value.approvalsReviewer === "user" &&
    value.sandboxPolicy === "danger-full-access"
  ) {
    return { approvalPolicy: "never", approvalsReviewer: "user", sandboxPolicy: "danger-full-access" };
  }
  return null;
}

function parseDefaultMCPToolIDs(raw: string | null | undefined): number[] {
  const value = raw?.trim();
  if (!value) {
    return [];
  }
  try {
    const parsed = JSON.parse(value) as unknown;
    if (!Array.isArray(parsed)) {
      return [];
    }
    const seen = new Set<number>();
    const result: number[] = [];
    for (const item of parsed) {
      const id = typeof item === "number" ? item : Number(item);
      if (Number.isSafeInteger(id) && id > 0 && !seen.has(id)) {
        seen.add(id);
        result.push(id);
      }
    }
    return result;
  } catch {
    return [];
  }
}

function normalizeAvailableMCPTools(tools: MCPToolDTO[]): MCPToolDTO[] {
  const seen = new Set<number>();
  return tools.filter((tool) => {
    if (!Number.isSafeInteger(tool.id) || tool.id <= 0 || seen.has(tool.id)) {
      return false;
    }
    const status = typeof tool.status === "string" ? tool.status.trim() : "";
    if (status && status !== "active") {
      return false;
    }
    seen.add(tool.id);
    return true;
  });
}

function filterAvailableMCPToolIDs(toolIDs: number[], tools: MCPToolDTO[], limit?: number): number[] {
  const availableIDs = new Set(tools.map((tool) => tool.id));
  const result = toolIDs.filter((id) => availableIDs.has(id));
  return typeof limit === "number" && limit >= 0 ? result.slice(0, limit) : result;
}

export function AppChatArea() {
  const t = useTranslations("chat");
  const tRecent = useTranslations("recent");
  const tScreenshot = useTranslations("chat.screenshot");
  const router = useRouter();
  const searchParams = useSearchParams();
  const routeConversationID = searchParams.get("conversation_id")?.trim() || null;
  const routeProjectID = searchParams.get("project_id")?.trim() || null;
  const { newConversationRevision, newConversationProjectID: requestedNewConversationProjectID, requestNewConversation, executionMode, setExecutionMode } = useChatSession();
  const { defaultDevice, devices, selectDefaultDevice } = useDevices();
  const [locallyCreatedConversationID, setLocallyCreatedConversationID] = React.useState<string | null>(null);
  const [newConversationOverride, setNewConversationOverride] = React.useState<{
    ignoredConversationID: string | null;
  } | null>(null);
  const previousNewConversationRevisionRef = React.useRef(newConversationRevision);

  React.useEffect(() => {
    if (previousNewConversationRevisionRef.current === newConversationRevision) {
      return;
    }
    previousNewConversationRevisionRef.current = newConversationRevision;
    setLocallyCreatedConversationID(null);
    setNewConversationOverride({
      ignoredConversationID: routeConversationID,
    });
  }, [newConversationRevision, routeConversationID]);

  React.useEffect(() => {
    if (routeConversationID) {
      setLocallyCreatedConversationID(null);
    }
  }, [routeConversationID]);

  React.useEffect(() => {
    setNewConversationOverride((prev) =>
      prev && routeConversationID !== prev.ignoredConversationID ? null : prev,
    );
  }, [routeConversationID]);

  const resolvedRouteConversationID = routeConversationID ?? locallyCreatedConversationID;
  const conversationID =
    newConversationOverride && resolvedRouteConversationID === newConversationOverride.ignoredConversationID
      ? null
      : resolvedRouteConversationID;
  const onNewConversationFromLoadError = React.useCallback(() => {
    const projectID = routeProjectID ?? "";
    requestNewConversation({ projectID });
    router.push(projectID ? `/chat?project_id=${encodeURIComponent(projectID)}` : "/chat");
  }, [requestNewConversation, routeProjectID, router]);
  const activeGenerationRunsRef = React.useRef<Set<string>>(new Set());
  const generationSeqByRunRef = React.useRef<Record<string, number>>({});
  const failedGenerationRunsRef = React.useRef<Set<string>>(new Set());
  const {
    autoGenerateLabels,
    deleteFilesByDefault,
  } = useSettingsChatPreferences();
  const {
    items,
    projects,
    prependNewConversation,
    touchByPublicID,
    renameByPublicID,
    regenerateTitleByPublicID,
    updateLabelsByPublicID,
    setStarByPublicID,
    setProjectByPublicID,
    deleteByPublicID,
  } = useSidebarConversations();
  const {
    cancelResumedGeneration,
    loading,
    loadingOlder,
    errorMsg,
    hasOlder,
    loadOlderMessages,
    loadAllOlderMessages,
    messages,
    reload,
    replaceMessage,
    resumingRunID,
  } = useChatData(conversationID, {
    activeGenerationRunsRef,
    failedGenerationRunsRef,
    generationSeqByRunRef,
  });
  const { greetingTitle } = useChatViewerProfile();
  const [manualConversationTitle, setManualConversationTitle] = React.useState("");
  const [shareDialogOpen, setShareDialogOpen] = React.useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = React.useState(false);
  const [deleteFiles, setDeleteFiles] = React.useState(false);
  const [deleteExecutionType, setDeleteExecutionType] = React.useState<"cloud" | "gateway">("cloud");
  const deleteFilesID = React.useId();
  const activeConversation = React.useMemo(() => {
    if (!conversationID) {
      return null;
    }
    return items.find((item) => item.publicID === conversationID) ?? null;
  }, [conversationID, items]);
  const [loadedConversation, setLoadedConversation] = React.useState<ConversationDTO | null>(null);
  React.useEffect(() => {
    const normalizedConversationID = conversationID?.trim() || "";
    if (!normalizedConversationID) {
      setLoadedConversation(null);
      return;
    }

    if (loading) {
      return;
    }

    let cancelled = false;
    async function loadConversation() {
      const token = await resolveAccessToken();
      if (!token) {
        return;
      }
      const item = await getConversation(token, normalizedConversationID);
      if (cancelled) {
        return;
      }
      setLoadedConversation(item);
    }

    void loadConversation().catch(() => {
      if (!cancelled) {
        setLoadedConversation(null);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [conversationID, loading]);
  const currentConversation =
    loadedConversation?.publicID === conversationID ? loadedConversation : activeConversation;
  useAgentRunHydration({
    conversationID,
    deviceID: currentConversation?.executionDeviceID,
    profileID: currentConversation?.executionProfileID,
    workspaceID: currentConversation?.executionWorkspaceID,
  });
  const executionModeConversationRef = React.useRef("");
  React.useEffect(() => {
    if (!currentConversation) {
      executionModeConversationRef.current = "";
      return;
    }
    if (executionModeConversationRef.current === currentConversation.publicID) return;
    executionModeConversationRef.current = currentConversation.publicID;
    setExecutionMode(currentConversation.executionType);
    if (
      currentConversation.executionType === "gateway" &&
      currentConversation.executionDeviceID &&
      currentConversation.executionDeviceID !== defaultDevice?.deviceId
    ) {
      void selectDefaultDevice(currentConversation.executionDeviceID);
    }
  }, [currentConversation, defaultDevice?.deviceId, selectDefaultDevice, setExecutionMode]);
  const activeRouteProject = React.useMemo(() => {
    if (!routeProjectID || conversationID) {
      return null;
    }
    return projects.find((item) => item.publicID === routeProjectID) ?? null;
  }, [conversationID, projects, routeProjectID]);
  const newConversationProjectID = !conversationID ? routeProjectID ?? requestedNewConversationProjectID : "";
  const newConversationProject = React.useMemo(
    () => projects.find((item) => item.publicID === newConversationProjectID) ?? null,
    [newConversationProjectID, projects],
  );
  const selectedProjectID = newConversationProject?.publicID ?? "";
  const executionProjectID = selectedProjectID;
  const prependNewConversationInContext = React.useCallback(
    (platformModelName?: string) => prependNewConversation(
      platformModelName,
      executionMode === "gateway" ? executionProjectID : selectedProjectID || undefined,
      executionMode === "gateway"
        ? { type: "gateway", deviceID: defaultDevice?.deviceId ?? "" }
        : { type: "cloud" },
    ),
    [defaultDevice, executionMode, executionProjectID, prependNewConversation, selectedProjectID],
  );
  const chatKeyBindings = useChatKeyBindings();

  const {
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
    selectedPlatformModelName: cloudSelectedPlatformModelName,
    setSelectedPlatformModelName,
  } = useChatModelOptions({
    conversationPublicID: conversationID,
    conversationModel: currentConversation?.model ?? null,
    resetToken: newConversationRevision,
    groupPlatform: chatKeyBindings.selectedRemoteKey?.groupPlatform,
  });
  const {
    conversationKey,
    draft,
    attachments,
    setDraft,
    setAttachments,
    appendAttachmentsForKey,
  } = useChatComposerState(conversationID, {
    preserveDrafts: preserveConversationDrafts,
    resetToken: newConversationRevision,
  });
  const selectedModel = React.useMemo(
    () => modelOptions.find((item) => item.platformModelName === cloudSelectedPlatformModelName) ?? null,
    [cloudSelectedPlatformModelName, modelOptions],
  );
  const selectedChatProtocol = resolveChatProtocol(
    chatKeyBindings.selectedRemoteKey?.groupPlatform ?? "",
    selectedModel?.protocols ?? [],
  );
  const modelOptionPolicyDisabled = modelOptionPolicy?.mode?.trim() === "disabled";
  const refreshModelCatalogForComposer = React.useCallback(async () => {
    await refreshModelCatalog();
  }, [refreshModelCatalog]);
  const [options, setOptions] = React.useState<ConversationOptions>({});
  const [availableTools, setAvailableTools] = React.useState<MCPToolDTO[]>([]);
  const [toolsLoading, setToolsLoading] = React.useState(true);
  const {
    selectedToolIDs,
    selectedSkills,
    selectedInputResources,
    setSelectedToolIDs,
    setSelectedSkills,
    setSelectedInputResources,
  } = useChatComposerSelection({
    conversationKey,
    createdConversationID: locallyCreatedConversationID,
    resetToken: newConversationRevision,
    hasConversation: Boolean(conversationID),
  });
  const selectedComposerInputResources = React.useMemo(
    () => selectedInputResources.filter((item) =>
      executionMode === "cloud"
        ? item.resourceRef.startsWith("plugin:")
        : !item.resourceRef.startsWith("plugin:"),
    ),
    [executionMode, selectedInputResources],
  );
  const [inputResources, setInputResources] = React.useState<ConversationInputResourceDTO[]>([]);
  const [inputResourceScope, setInputResourceScope] = React.useState("");
  const [inputResourcesReady, setInputResourcesReady] = React.useState(false);
  const inputResourceDeviceID = currentConversation?.executionType === "gateway"
    ? currentConversation.executionDeviceID
    : executionMode === "gateway" ? defaultDevice?.deviceId ?? "" : "";
  const inputResourceWorkspaceID = currentConversation?.executionType === "gateway"
    ? currentConversation.executionWorkspaceID
    : executionMode === "gateway" ? executionProjectID : "";
  const inputResourceDeviceOnline = devices.some(
    (device) => device.deviceId === inputResourceDeviceID && device.online,
  );
  React.useEffect(() => {
    if (!inputResourceDeviceID || !inputResourceWorkspaceID) {
      setInputResources([]);
      setInputResourceScope("");
      setInputResourcesReady(false);
      return;
    }
    const scope = `${inputResourceDeviceID}:${inputResourceWorkspaceID}`;
    setInputResources([]);
    setInputResourceScope("");
    setInputResourcesReady(false);
    const controller = new AbortController();
    void (async () => {
      while (!controller.signal.aborted) {
        const token = await resolveAccessToken();
        if (!token || controller.signal.aborted) {
          return;
        }
        const catalog = await listConversationInputResources(
          token,
          inputResourceDeviceID,
          inputResourceWorkspaceID,
          controller.signal,
        );
        if (controller.signal.aborted) {
          return;
        }
        setInputResources(catalog.items);
        setInputResourceScope(scope);
        setInputResourcesReady(catalog.ready);
        if (catalog.ready || !inputResourceDeviceOnline) {
          return;
        }
        await new Promise((resolve) => window.setTimeout(resolve, 2_000));
      }
    })().catch(() => {
      if (!controller.signal.aborted) {
        setInputResources([]);
        setInputResourceScope(scope);
        setInputResourcesReady(false);
      }
    });
    return () => controller.abort();
  }, [inputResourceDeviceID, inputResourceDeviceOnline, inputResourceWorkspaceID]);
  React.useEffect(() => {
    if (executionMode !== "gateway") {
      return;
    }
    if (!inputResourcesReady || inputResourceScope !== `${inputResourceDeviceID}:${inputResourceWorkspaceID}`) {
      return;
    }
    const availableRefs = new Set(inputResources.map((item) => item.resourceRef));
    setSelectedInputResources((current) => current.filter((item) => availableRefs.has(item.resourceRef)));
  }, [executionMode, inputResourceDeviceID, inputResourceScope, inputResourceWorkspaceID, inputResources, inputResourcesReady, setSelectedInputResources]);
  const [agentModels, setAgentModels] = React.useState<AgentModelDTO[]>([]);
  const [agentSettings, setAgentSettings] = React.useState<AgentTurnSettings | null>(null);
  const [agentSettingsLoading, setAgentSettingsLoading] = React.useState(false);
  const [agentSettingsError, setAgentSettingsError] = React.useState("");
  const [agentAutoReviewEnabled, setAgentAutoReviewEnabled] = React.useState(false);
  const [agentApprovalModeSelectionRequired, setAgentApprovalModeSelectionRequired] = React.useState(false);
  const [agentSettingsStorageScope, setAgentSettingsStorageScope] = React.useState("");
  const [agentSettingsProfileID, setAgentSettingsProfileID] = React.useState("");
  React.useEffect(() => {
    if (executionMode !== "gateway") {
      setAgentModels([]);
      setAgentSettings(null);
      setAgentSettingsError("");
      setAgentAutoReviewEnabled(false);
      setAgentApprovalModeSelectionRequired(false);
      setAgentSettingsStorageScope("");
      setAgentSettingsProfileID("");
      setAgentSettingsLoading(false);
      return;
    }
    if (!inputResourceDeviceID || !inputResourceWorkspaceID) {
      setAgentModels([]);
      setAgentSettings(null);
      setAgentSettingsError("");
      setAgentAutoReviewEnabled(false);
      setAgentApprovalModeSelectionRequired(false);
      setAgentSettingsStorageScope("");
      setAgentSettingsProfileID("");
      setAgentSettingsLoading(false);
      return;
    }

    let cancelled = false;
    setAgentModels([]);
    setAgentSettings(null);
    setAgentSettingsError("");
    setAgentAutoReviewEnabled(false);
    setAgentApprovalModeSelectionRequired(false);
    setAgentSettingsStorageScope("");
    setAgentSettingsProfileID("");
    setAgentSettingsLoading(true);
    void (async () => {
      const token = await resolveAccessToken();
      if (!token) {
        throw new Error(t("agent.settings.errors.unauthorized"));
      }
      const [profiles, workspaces] = await Promise.all([
        listAgentRuntimeProfiles(token, inputResourceDeviceID),
        listAgentWorkspaces(token, inputResourceDeviceID),
      ]);
      if (cancelled) {
        return;
      }
      const workspace = workspaces.find((item) => item.workspaceId === inputResourceWorkspaceID);
      const persistedGatewayTarget = currentConversation?.executionType === "gateway";
      const profileID = persistedGatewayTarget
        ? currentConversation.executionProfileID
        : workspace?.profileId ?? "";
      const profile = profiles.find((item) => item.profileId === profileID && item.status === "ready");
      if ((!persistedGatewayTarget && !workspace) || !profile || profile.provider !== "codex" || !profile.manifest.threadSettings.model) {
        throw new Error(t("agent.settings.errors.profileUnavailable"));
      }
      if (
        !profile.manifest.threadSettings.approvalPolicy.includes("on-request") ||
        !profile.manifest.threadSettings.approvalPolicy.includes("never") ||
        !profile.manifest.threadSettings.sandboxPolicy.includes("workspace-write") ||
        !profile.manifest.threadSettings.sandboxPolicy.includes("danger-full-access")
      ) {
        throw new Error(t("agent.settings.errors.capabilityUnavailable"));
      }

      let snapshot: Awaited<ReturnType<typeof getAgentProfileResource>> | null = null;
      try {
        snapshot = await getAgentProfileResource(token, inputResourceDeviceID, profile.profileId, "models");
      } catch {
        snapshot = null;
      }
      const refreshedAt = snapshot ? Date.parse(snapshot.refreshedAt) : Number.NaN;
      const snapshotStale = !Number.isFinite(refreshedAt) || Date.now() - refreshedAt > AGENT_MODELS_STALE_MS;
      if ((!snapshot || snapshotStale) && inputResourceDeviceOnline) {
        const queued = await refreshAgentProfileResource(token, inputResourceDeviceID, profile.profileId, "models");
        const completed = await waitForAgentCommand(token, queued.commandId);
        if (!completed || completed.status !== "completed") {
          throw new Error(completed?.errorMessage || t("agent.settings.errors.modelsUnavailable"));
        }
        snapshot = await getAgentProfileResource(token, inputResourceDeviceID, profile.profileId, "models");
      }
      if (!snapshot) {
        throw new Error(t("agent.settings.errors.modelsUnavailable"));
      }

      const conversationModel = currentConversation?.executionType === "gateway"
        ? currentConversation.model.trim()
        : "";
      const conversationReasoningEffort = currentConversation?.executionType === "gateway"
        ? currentConversation.reasoningEffort
        : "";
      const models = includeCurrentAgentModel(
        parseAgentModelsResource(snapshot.data, profile.manifest.threadSettings.reasoningEffort),
        conversationModel,
        conversationReasoningEffort,
        profile.manifest.threadSettings.reasoningEffort,
      );
      if (models.length === 0) {
        throw new Error(t("agent.settings.errors.modelsUnavailable"));
      }
      const gatewayConversationID = currentConversation?.executionType === "gateway"
        ? currentConversation.publicID.trim()
        : "";
      const storageScope = gatewayConversationID
        ? agentSettingsStorageKey(
            inputResourceDeviceID,
            profile.profileId,
            inputResourceWorkspaceID,
            gatewayConversationID,
          )
        : "";
      const persisted = storageScope ? readAgentSettings(storageScope) : null;
      const selectedModel = conversationModel
        ? models.find((item) => item.id === conversationModel)
        : persisted
          ? models.find((item) => item.id === persisted.model)
          : models.find((item) => item.isDefault) ?? models[0];
      if (!selectedModel) {
        throw new Error(t("agent.settings.errors.modelsUnavailable"));
      }
      const autoReviewEnabled = manifestSupportsAutoReview(profile.manifest);
      const approvalSettings = parseAgentApprovalSettings(
        currentConversation?.executionType === "gateway" ? {
          approvalPolicy: currentConversation.approvalPolicy,
          approvalsReviewer: currentConversation.approvalsReviewer,
          sandboxPolicy: currentConversation.sandboxPolicy,
        } : null,
      ) ?? parseAgentApprovalSettings(persisted) ?? {
        approvalPolicy: "on-request",
        approvalsReviewer: "user",
        sandboxPolicy: "workspace-write",
      } satisfies AgentApprovalSettings;
      const storedConversationReasoningEffort = selectedModel.supportedReasoningEfforts.find(
        (effort) => effort === conversationReasoningEffort,
      );
      const reasoningEffort = storedConversationReasoningEffort
        ? storedConversationReasoningEffort
        : persisted?.model === selectedModel.id && selectedModel.supportedReasoningEfforts.includes(persisted.reasoningEffort)
          ? persisted.reasoningEffort
          : selectedModel.defaultReasoningEffort;
      const nextSettings: AgentTurnSettings = {
        model: selectedModel.id,
        reasoningEffort,
        ...approvalSettings,
      };
      if (cancelled) {
        return;
      }
      setAgentModels(models);
      setAgentSettings(nextSettings);
      setAgentAutoReviewEnabled(autoReviewEnabled);
      setAgentApprovalModeSelectionRequired(
        nextSettings.approvalsReviewer === "auto_review" && !autoReviewEnabled,
      );
      setAgentSettingsStorageScope(storageScope);
      setAgentSettingsProfileID(profile.profileId);
      if (storageScope) {
        writeAgentSettings(storageScope, nextSettings);
      }
    })().catch((error: unknown) => {
      if (!cancelled) {
        const message = error instanceof Error && error.message.trim()
          ? error.message
          : t("agent.settings.errors.modelsUnavailable");
        setAgentModels([]);
        setAgentSettings(null);
        setAgentSettingsError(message);
        setAgentAutoReviewEnabled(false);
        setAgentApprovalModeSelectionRequired(false);
        setAgentSettingsStorageScope("");
        setAgentSettingsProfileID("");
        toast.error(t("agent.settings.unavailable"), {
          id: `agent-settings-${inputResourceDeviceID}-${inputResourceWorkspaceID}`,
          description: message,
        });
      }
    }).finally(() => {
      if (!cancelled) {
        setAgentSettingsLoading(false);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [
    currentConversation?.executionProfileID,
    currentConversation?.publicID,
    currentConversation?.executionType,
    currentConversation?.model,
    currentConversation?.reasoningEffort,
    currentConversation?.approvalPolicy,
    currentConversation?.approvalsReviewer,
    currentConversation?.sandboxPolicy,
    executionMode,
    inputResourceDeviceID,
    inputResourceDeviceOnline,
    inputResourceWorkspaceID,
    t,
  ]);
  const onAgentSettingsChange = React.useCallback((nextSettings: AgentTurnSettings) => {
    setAgentSettings(nextSettings);
    setAgentApprovalModeSelectionRequired(
      nextSettings.approvalsReviewer === "auto_review" && !agentAutoReviewEnabled,
    );
    writeAgentSettings(agentSettingsStorageScope, nextSettings);
  }, [agentAutoReviewEnabled, agentSettingsStorageScope]);
  const onConversationCreated = React.useCallback((conversationPublicID: string) => {
    if (
      executionMode === "gateway" &&
      agentSettings &&
      agentSettingsProfileID &&
      inputResourceDeviceID &&
      inputResourceWorkspaceID
    ) {
      writeAgentSettings(
        agentSettingsStorageKey(
          inputResourceDeviceID,
          agentSettingsProfileID,
          inputResourceWorkspaceID,
          conversationPublicID,
        ),
        agentSettings,
      );
    }
    setLocallyCreatedConversationID(conversationPublicID);
  }, [
    agentSettings,
    agentSettingsProfileID,
    executionMode,
    inputResourceDeviceID,
    inputResourceWorkspaceID,
  ]);
  const selectedPlatformModelName = executionMode === "gateway"
    ? agentSettings?.model ?? ""
    : cloudSelectedPlatformModelName;
  const effectiveOptions = React.useMemo<ConversationOptions>(() => executionMode === "gateway" && agentSettings
    ? {
        reasoningEffort: agentSettings.reasoningEffort,
        approvalPolicy: agentSettings.approvalPolicy,
        approvalsReviewer: agentSettings.approvalsReviewer,
        sandboxPolicy: agentSettings.sandboxPolicy,
      }
    : modelOptionPolicyDisabled ? EMPTY_CONVERSATION_OPTIONS : options,
  [agentSettings, executionMode, modelOptionPolicyDisabled, options]);
  const [defaultToolIDs, setDefaultToolIDs] = React.useState<number[]>([]);
  const newConversationSelectionKey = `${newConversationRevision}:${newConversationProjectID || "unassigned"}`;
  const newConversationDefaultMCPToolIDs = React.useMemo(
    () => normalizeImageAttachmentProcessorSelection(
      filterAvailableMCPToolIDs(
        newConversationProject?.mcpDefaultMode === "custom"
          ? newConversationProject.defaultMCPToolIDs
          : defaultToolIDs,
        availableTools,
        mcpMaxSelectedTools,
      ),
      availableTools,
    ),
    [availableTools, defaultToolIDs, mcpMaxSelectedTools, newConversationProject],
  );
  const newConversationDefaultSkillIDs = React.useMemo(
    () => (newConversationProject?.defaultSkillIDs ?? []).slice(0, mcpMaxSelectedTools),
    [mcpMaxSelectedTools, newConversationProject],
  );
  const { onSelectedSkillsChange, onSelectedToolsChange: applySelectedToolsChange } = useNewConversationDefaults({
    conversationID,
    contextKey: newConversationSelectionKey,
    defaultsPending: Boolean(newConversationProjectID && !newConversationProject),
    defaultMCPToolIDs: newConversationDefaultMCPToolIDs,
    defaultSkillIDs: newConversationDefaultSkillIDs,
    toolsLoading,
    setSelectedToolIDs,
    setSelectedSkills,
  });
  const onSelectedToolsChange = React.useCallback((nextToolIDs: number[]) => {
    if (hasMultipleImageAttachmentProcessors(nextToolIDs, availableTools)) {
      toast.error(t("composer.mcpImageProcessorLimitTitle"), {
        description: t("composer.mcpImageProcessorLimitDescription"),
      });
      return;
    }
    applySelectedToolsChange(nextToolIDs);
  }, [applySelectedToolsChange, availableTools, t]);
  React.useEffect(() => {
    if (toolsLoading) {
      return;
    }
    const normalized = normalizeImageAttachmentProcessorSelection(
      filterAvailableMCPToolIDs(selectedToolIDs, availableTools, mcpMaxSelectedTools),
      availableTools,
    );
    if (normalized.length === selectedToolIDs.length && normalized.every((id, index) => id === selectedToolIDs[index])) {
      return;
    }
    setSelectedToolIDs(normalized);
  }, [availableTools, mcpMaxSelectedTools, selectedToolIDs, setSelectedToolIDs, toolsLoading]);
  const htmlVisualPrompt = useChatVisualPrompt();
  const initializedOptionsModelRef = React.useRef("");
  const selectedModelDefaultOptionsRef = React.useRef<ConversationOptions>({});
  const fileDragDepthRef = React.useRef(0);
  const [fileDragActive, setFileDragActive] = React.useState(false);

  React.useEffect(() => {
    setSelectedToolIDs((current) => {
      if (current.length <= mcpMaxSelectedTools) {
        return current;
      }
      return current.slice(0, mcpMaxSelectedTools);
    });
  }, [mcpMaxSelectedTools, setSelectedToolIDs]);

  React.useEffect(() => {
    const platformModelName = selectedModel?.platformModelName.trim() || "";
    if (!platformModelName) {
      initializedOptionsModelRef.current = "";
      selectedModelDefaultOptionsRef.current = {};
      setOptions({});
      return;
    }
    const nextDefaultOptions = cloneConversationOptions(selectedModel.defaultOptions);
    const previousDefaultOptions = selectedModelDefaultOptionsRef.current;
    if (initializedOptionsModelRef.current !== platformModelName) {
      initializedOptionsModelRef.current = platformModelName;
      selectedModelDefaultOptionsRef.current = nextDefaultOptions;
      setOptions(nextDefaultOptions);
      return;
    }
    selectedModelDefaultOptionsRef.current = nextDefaultOptions;
    const previousDefaultOptionsJSON = JSON.stringify(previousDefaultOptions);
    if (previousDefaultOptionsJSON === JSON.stringify(nextDefaultOptions)) {
      return;
    }
    setOptions((currentOptions) => {
      if (JSON.stringify(currentOptions) !== previousDefaultOptionsJSON) {
        return currentOptions;
      }
      return cloneConversationOptions(nextDefaultOptions);
    });
  }, [selectedModel]);

  const setModelOptions = React.useCallback(
    (action: React.SetStateAction<ConversationOptions>) => {
      setOptions((previous) => {
        const next = typeof action === "function" ? action(previous) : action;
        return isConversationOptionsObject(next) ? sanitizeConversationOptions(next) : {};
      });
    },
    [],
  );

  React.useEffect(() => {
    let cancelled = false;

    async function loadTools() {
      setToolsLoading(true);
      try {
        const token = await resolveAccessToken();
        if (!token) {
          if (!cancelled) {
            setAvailableTools([]);
            setSelectedToolIDs([]);
          }
          return;
        }
        const [toolsResult, settings] = await Promise.all([
          listAvailableMCPTools(token),
          getUserSettings(token).catch(() => ({} as Record<string, string>)),
        ]);
        if (cancelled) {
          return;
        }
        const tools = normalizeAvailableMCPTools(toolsResult);
        const userDefaultToolIDs = normalizeImageAttachmentProcessorSelection(
          filterAvailableMCPToolIDs(
            parseDefaultMCPToolIDs(settings[DEFAULT_MCP_TOOLS_SETTING_KEY]),
            tools,
            mcpMaxSelectedTools,
          ),
          tools,
        );
        setAvailableTools(tools);
        setDefaultToolIDs(userDefaultToolIDs);
        setSelectedToolIDs((previous) => normalizeImageAttachmentProcessorSelection(
          filterAvailableMCPToolIDs(previous, tools, mcpMaxSelectedTools),
          tools,
        ));
      } catch {
        if (!cancelled) {
          setAvailableTools([]);
          setSelectedToolIDs([]);
        }
      } finally {
        if (!cancelled) {
          setToolsLoading(false);
        }
      }
    }

    void loadTools();
    return () => {
      cancelled = true;
    };
  }, [conversationID, mcpMaxSelectedTools, setSelectedToolIDs]);

  const onDefaultToolIDsChange = React.useCallback(async (nextToolIDs: number[]) => {
    const nextDefaults = filterAvailableMCPToolIDs(nextToolIDs, availableTools, mcpMaxSelectedTools);
    if (hasMultipleImageAttachmentProcessors(nextDefaults, availableTools)) {
      toast.error(t("composer.mcpImageProcessorLimitTitle"), {
        description: t("composer.mcpImageProcessorLimitDescription"),
      });
      return;
    }
    const previousDefaults = defaultToolIDs;
    setDefaultToolIDs(nextDefaults);
    try {
      const token = await resolveAccessToken();
      if (!token) {
        throw new Error(t("composer.sessionExpired"));
      }
      await patchUserSettings(token, {
        [DEFAULT_MCP_TOOLS_SETTING_KEY]: JSON.stringify(nextDefaults),
      });
      toast.success(t("composer.defaultMCPToolsSaved"));
    } catch (error) {
      setDefaultToolIDs(previousDefaults);
      toast.error(t("composer.defaultMCPToolsSaveFailed"), {
        description: error instanceof Error ? error.message : t("composer.retryLater"),
      });
    }
  }, [availableTools, defaultToolIDs, mcpMaxSelectedTools, t]);

  const {
    uploading,
    uploadingAttachments,
    maxFilesPerMessage,
    fileMode,
    releaseAttachments,
    onRemoveAttachment,
    onUploadFiles,
    onCaptureScreenshot,
  } = useChatAttachments({
    conversationKey,
    attachments,
    setAttachments,
    appendAttachmentsForKey,
  });

  const {
    currentLeafMessage,
    onCycleMessageBranch,
    onEditAssistantMessage,
    onEditUserMessage,
    onContinueAssistantMessage,
    onRetryAssistantMessage,
    onRetryUserMessage,
    onSendMessage,
    onStopMessage,
    onDeleteQueuedMessage,
    onEditQueuedMessage,
    onGuideQueuedMessage,
    queuedMessages,
    sending,
    conversationRunActive,
    visibleMessageCount,
    visibleMessages,
    isConversationMode,
  } = useChatRuntime({
    conversationID,
    resetToken: newConversationRevision,
    executionMode,
    messages,
    activeConversation: currentConversation,
    selectedPlatformModelName,
    selectedKeyBindingID: chatKeyBindings.selectedKeyBindingID,
    modelOptions,
    selectedToolIDs,
    selectedSkills,
    selectedInputResources: selectedComposerInputResources,
    htmlVisualPromptEnabled: executionMode === "cloud" && htmlVisualPrompt.enabled,
    options: effectiveOptions,
    draft,
    attachments,
    maxFilesPerMessage,
    uploading,
    restoreDraftOnFailure,
    autoGenerateLabels,
    prependNewConversation: prependNewConversationInContext,
    onConversationCreated,
    touchByPublicID,
    reload,
    replaceMessage,
    setDraft,
    setAttachments,
    releaseAttachments,
    activeGenerationRunsRef,
    failedGenerationRunsRef,
    generationSeqByRunRef,
    resumingRunID,
  });
  const generating = sending;
  const uploadDropDisabled = loading || uploading;
  const onStopActiveMessage = React.useCallback(() => {
    const visibleRunID = currentLeafMessage?.runID?.trim() || "";
    if (resumingRunID && visibleRunID === resumingRunID) {
      void cancelResumedGeneration();
      return;
    }
    if (onStopMessage()) {
      return;
    }
  }, [
    cancelResumedGeneration,
    currentLeafMessage?.runID,
    onStopMessage,
    resumingRunID,
  ]);

  const messageContentRef = React.useRef<HTMLDivElement | null>(null);
  const loadingOlderInFlightRef = React.useRef(false);
  const onScroll = React.useCallback(
    (event: React.UIEvent<HTMLDivElement>) => {
      const viewport = event.currentTarget;
      const distanceFromBottom = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight;
      if (
        viewport.scrollTop > TOP_LOAD_OLDER_MESSAGES_THRESHOLD_PX ||
        distanceFromBottom <= TOP_LOAD_OLDER_MESSAGES_THRESHOLD_PX ||
        !hasOlder ||
        loadingOlder ||
        loadingOlderInFlightRef.current
      ) {
        return;
      }

      loadingOlderInFlightRef.current = true;
      Promise.resolve(loadOlderMessages())
        .catch(() => undefined)
        .finally(() => {
          loadingOlderInFlightRef.current = false;
        });
    },
    [hasOlder, loadOlderMessages, loadingOlder],
  );

  const onEditGeneratedImageAttachment = React.useCallback(
    (attachment: MessageAttachment, sourceModelName?: string) => {
      const alreadyAttached = attachments.some((item) => item.fileID === attachment.fileID);
      if (!alreadyAttached && maxFilesPerMessage > 0 && attachments.length >= maxFilesPerMessage) {
        toast.error(t("attachments.limitReached"), {
          description: t("attachments.maxUploadFiles", { count: maxFilesPerMessage }),
        });
        return;
      }

      const pendingAttachment = toPendingAttachment(attachment);
      setAttachments((previous) => {
        if (previous.some((item) => item.fileID === pendingAttachment.fileID)) {
          return previous;
        }
        return [...previous, pendingAttachment];
      });

      const supportsImageEdit = (model: (typeof modelOptions)[number]) =>
        model.kinds.includes("image_edit") || modelSupportsChatImageTool(model);
      const selectedSupportsImageEdit = selectedModel ? supportsImageEdit(selectedModel) : false;
      if (!selectedSupportsImageEdit) {
        const normalizedSourceModelName = sourceModelName?.trim() || "";
        const sourceModel = modelOptions.find(
          (item) => item.platformModelName === normalizedSourceModelName && supportsImageEdit(item),
        );
        const fallbackModel = sourceModel ?? modelOptions.find(supportsImageEdit);
        if (fallbackModel) {
          setSelectedPlatformModelName(fallbackModel.platformModelName);
        }
      }

    },
    [
      attachments,
      maxFilesPerMessage,
      modelOptions,
      selectedModel,
      setAttachments,
      setSelectedPlatformModelName,
      t,
    ],
  );

  const onAttachExistingFile = React.useCallback(
    (file: FileObjectDTO) => {
      const alreadyAttached = attachments.some((item) => item.fileID === file.fileID);
      if (alreadyAttached) {
        return;
      }
      if (maxFilesPerMessage > 0 && attachments.length >= maxFilesPerMessage) {
        toast.error(t("attachments.limitReached"), {
          description: t("attachments.maxUploadFiles", { count: maxFilesPerMessage }),
        });
        return;
      }
      setAttachments((previous) => {
        if (previous.some((item) => item.fileID === file.fileID)) {
          return previous;
        }
        return [
          ...previous,
          {
            fileID: file.fileID,
            fileName: file.fileName,
            mimeType: file.mimeType,
            detectedMime: file.detectedMIME,
            fileCategory: file.fileCategory,
            sizeBytes: file.sizeBytes,
            processingStatus: file.processingStatus,
            processingReady: file.processingReady,
            processingErrorCode: file.processingErrorCode,
            processingErrorMessage: file.processingErrorMessage,
            extractStatus: file.extractStatus,
            embedStatus: file.embedStatus,
            ragReady: false,
            ragReason: "",
            ocrUsed: false,
            ragOptOut: file.ragOptOut,
          },
        ];
      });
    },
    [attachments, maxFilesPerMessage, setAttachments, t],
  );

  React.useEffect(() => {
    setManualConversationTitle("");
  }, [conversationID]);

  React.useEffect(() => {
    const nextTitle = currentConversation?.title?.trim();
    if (nextTitle) {
      setManualConversationTitle(nextTitle);
    }
  }, [currentConversation?.publicID, currentConversation?.title]);

  const actionConversationID = React.useMemo(() => (conversationID || "").trim(), [conversationID]);
  const canOperateConversation = actionConversationID.length > 0;
  const activeConversationTitle = React.useMemo(
    () => manualConversationTitle || currentConversation?.title?.trim() || t("untitledConversation"),
    [currentConversation?.title, manualConversationTitle, t],
  );
  const activeConversationStarred = Boolean(currentConversation?.isStarred);
  const activeConversationLabels = React.useMemo(
    () => parseConversationLabelsJSON(currentConversation?.labelsJSON ?? "[]"),
    [currentConversation?.labelsJSON],
  );
  const activeConversationShared = currentConversation?.shareStatus === "active" && Boolean(currentConversation.shareID?.trim());
  const shareDefaultMessagePublicIDs = React.useMemo(
    () =>
      visibleMessages
        .filter((item) => !item.isPending && Boolean(item.serverMessageID) && item.publicID.trim())
        .map((item) => item.publicID.trim()),
    [visibleMessages],
  );

  const screenshotMessages = React.useMemo(
    () => ({
      emptySelection: tScreenshot("emptySelection"),
      generating: tScreenshot("generating"),
      ready: tScreenshot("ready"),
      failed: tScreenshot("failed"),
      loadLimitReached: tScreenshot("loadLimitReached"),
      tooLarge: tScreenshot("tooLarge"),
      downloaded: tScreenshot("downloaded"),
      copied: tScreenshot("copied"),
      copyFailed: tScreenshot("copyFailed"),
      copyUnsupported: tScreenshot("copyUnsupported"),
    }),
    [tScreenshot],
  );
  const screenshot = useChatScreenshot({
    conversationID: actionConversationID || null,
    messageContentRef,
    conversationTitle: activeConversationTitle,
    onLoadAllMessages: loadAllOlderMessages,
    messages: screenshotMessages,
  });
  const screenshotPreview = screenshot.preview;
  const closeScreenshotPreview = screenshot.closePreview;
  const [screenshotPreviewOpen, setScreenshotPreviewOpen] = React.useState(false);
  const screenshotPreviewCloseTimerRef = React.useRef<number | null>(null);

  const clearScreenshotPreviewCloseTimer = React.useCallback(() => {
    if (screenshotPreviewCloseTimerRef.current === null) {
      return;
    }
    window.clearTimeout(screenshotPreviewCloseTimerRef.current);
    screenshotPreviewCloseTimerRef.current = null;
  }, []);

  React.useEffect(() => {
    if (!screenshotPreview) {
      setScreenshotPreviewOpen(false);
      return;
    }
    clearScreenshotPreviewCloseTimer();
    setScreenshotPreviewOpen(true);
  }, [clearScreenshotPreviewCloseTimer, screenshotPreview]);

  React.useEffect(() => clearScreenshotPreviewCloseTimer, [clearScreenshotPreviewCloseTimer]);

  const closeScreenshotPreviewDialog = React.useCallback(() => {
    setScreenshotPreviewOpen(false);
    clearScreenshotPreviewCloseTimer();
    screenshotPreviewCloseTimerRef.current = window.setTimeout(() => {
      screenshotPreviewCloseTimerRef.current = null;
      closeScreenshotPreview();
    }, SCREENSHOT_PREVIEW_CLOSE_DELAY_MS);
  }, [clearScreenshotPreviewCloseTimer, closeScreenshotPreview]);

  const onToggleActiveConversationStar = React.useCallback(async () => {
    if (!canOperateConversation) {
      return;
    }
    await setStarByPublicID(actionConversationID, !activeConversationStarred);
  }, [actionConversationID, activeConversationStarred, canOperateConversation, setStarByPublicID]);

  const onRenameActiveConversation = React.useCallback(
    async (title: string) => {
      if (!canOperateConversation) {
        return;
      }
      const normalized = title.trim();
      if (!normalized) {
        return;
      }
      const updated = await renameByPublicID(actionConversationID, normalized);
      setManualConversationTitle(updated?.title?.trim() || normalized);
    },
    [actionConversationID, canOperateConversation, renameByPublicID],
  );

  const onAutoRenameActiveConversation = React.useCallback(async () => {
    if (!canOperateConversation) {
      return;
    }
    try {
      const updated = await regenerateTitleByPublicID(actionConversationID);
      if (updated?.title?.trim()) {
        setManualConversationTitle(updated.title.trim());
      }
    } catch (error) {
      toast.error(t("labelMenu.autoRenameFailed"));
      throw error;
    }
  }, [actionConversationID, canOperateConversation, regenerateTitleByPublicID, t]);

  const onUpdateActiveConversationLabels = React.useCallback(
    async (labels: string[]) => {
      if (!canOperateConversation) {
        return;
      }
      const updated = await updateLabelsByPublicID(actionConversationID, labels);
      if (!updated) {
        throw new Error("conversation labels were not updated");
      }
    },
    [actionConversationID, canOperateConversation, updateLabelsByPublicID],
  );

  const onRequestDeleteActiveConversation = React.useCallback(() => {
    if (!canOperateConversation || !currentConversation) {
      return;
    }
    const targetExecutionType = currentConversation.executionType;
    setDeleteExecutionType(targetExecutionType);
    setDeleteFiles(targetExecutionType === "cloud" && deleteFilesByDefault);
    setDeleteDialogOpen(true);
  }, [canOperateConversation, currentConversation, deleteFilesByDefault]);

  const onConfirmDeleteActiveConversation = React.useCallback(async () => {
    if (!canOperateConversation) {
      return;
    }
    const ok = await deleteByPublicID(actionConversationID, { deleteFiles });
    if (ok) {
      setDeleteDialogOpen(false);
      setDeleteFiles(false);
      router.push("/chat");
    }
  }, [actionConversationID, canOperateConversation, deleteByPublicID, deleteFiles, router]);

  const onSetActiveConversationProject = React.useCallback(
    async (projectID?: string) => {
      if (!canOperateConversation) {
        return;
      }
      await setProjectByPublicID(actionConversationID, projectID);
    },
    [actionConversationID, canOperateConversation, setProjectByPublicID],
  );

  const onShareActiveConversation = React.useCallback(() => {
    if (!canOperateConversation) {
      return;
    }
    setShareDialogOpen(true);
  }, [canOperateConversation]);

  const exportActiveConversation = useConversationExport({
    successMessage: t("exportJSONSuccess"),
    failureMessage: t("exportJSONFailed"),
  });

  const onExportActiveConversation = React.useCallback(async () => {
    if (!canOperateConversation) {
      return;
    }
    await exportActiveConversation(actionConversationID);
  }, [actionConversationID, canOperateConversation, exportActiveConversation]);

  const messagesWithInlineError = React.useMemo<ChatAreaMessage[]>(() => {
    const errors = [
      modelsErrorMsg.trim()
        ? {
            title: t("modelListLoadFailed"),
            message: modelsErrorMsg.trim(),
          }
        : null,
    ].filter((item): item is NonNullable<typeof item> => item !== null);

    if (errors.length === 0) {
      return visibleMessages;
    }

    return [
      ...visibleMessages,
      {
        key: `chat-inline-error-${conversationID ?? "current"}`,
        publicID: `chat-inline-error-${conversationID ?? "current"}`,
        parentPublicID: visibleMessages.at(-1)?.publicID ?? null,
        sourcePublicID: null,
        role: "system",
        content: "",
        branchReason: "default",
        isPending: false,
        isStreaming: false,
        inlineAlert: {
          title: errors.map((item) => item.title).join(" / "),
          message: errors.map((item) => item.message).join("\n"),
        },
      },
    ];
  }, [conversationID, modelsErrorMsg, t, visibleMessages]);

  const artifactWorkspace = useChatArtifacts({
    conversationID,
    messages: messagesWithInlineError,
  });
  const workspaceRef = React.useRef<HTMLDivElement | null>(null);
  const artifactResizeCleanupRef = React.useRef<(() => void) | null>(null);
  const [artifactResizing, setArtifactResizing] = React.useState(false);
  const hasInlineArtifact = Boolean((artifactWorkspace.activeArtifact || artifactWorkspace.activeDiff) && artifactWorkspace.isInlineViewport);
  const workspaceGridColumns = hasInlineArtifact
    ? `minmax(0, ${1 - artifactWorkspace.artifactRatio}fr) minmax(0, ${artifactWorkspace.artifactRatio}fr)`
    : "minmax(0, 1fr) minmax(0, 0fr)";

  React.useEffect(() => () => {
    artifactResizeCleanupRef.current?.();
  }, []);

  const onArtifactResizeStart = React.useCallback((event: React.PointerEvent<HTMLButtonElement>) => {
    const workspace = workspaceRef.current;
    if (!workspace || event.button !== 0) {
      return;
    }

    event.preventDefault();
    artifactResizeCleanupRef.current?.();
    setArtifactResizing(true);
    const resizeHandle = event.currentTarget;
    const pointerID = event.pointerId;
    const startClientX = event.clientX;
    const startRatio = artifactWorkspace.artifactRatio;

    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    let stopped = false;
    const stopResize = () => {
      if (stopped) {
        return;
      }

      stopped = true;
      artifactResizeCleanupRef.current = null;
      setArtifactResizing(false);
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
      if (resizeHandle.hasPointerCapture(pointerID)) {
        resizeHandle.releasePointerCapture(pointerID);
      }
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerup", stopResize);
      window.removeEventListener("pointercancel", stopResize);
      window.removeEventListener("blur", stopResize);
      document.removeEventListener("visibilitychange", stopResizeWhenHidden);
      resizeHandle.removeEventListener("lostpointercapture", stopResize);
    };
    const updateRatio = (clientX: number) => {
      const rect = workspace.getBoundingClientRect();
      if (rect.width <= 0) {
        stopResize();
        return;
      }

      const ratio = startRatio - ((clientX - startClientX) / rect.width);
      artifactWorkspace.setArtifactRatio(ratio);
    };
    const onPointerMove = (moveEvent: PointerEvent) => updateRatio(moveEvent.clientX);
    const stopResizeWhenHidden = () => {
      if (document.visibilityState === "hidden") {
        stopResize();
      }
    };

    resizeHandle.setPointerCapture(pointerID);
    artifactResizeCleanupRef.current = stopResize;
    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", stopResize);
    window.addEventListener("pointercancel", stopResize);
    window.addEventListener("blur", stopResize);
    document.addEventListener("visibilitychange", stopResizeWhenHidden);
    resizeHandle.addEventListener("lostpointercapture", stopResize);
  }, [artifactWorkspace]);

  const selectedModelDefaultOptions = modelOptionPolicyDisabled
    ? EMPTY_CONVERSATION_OPTIONS
    : (selectedModel?.defaultOptions ?? EMPTY_CONVERSATION_OPTIONS);
  const resetFileDragState = React.useCallback(() => {
    fileDragDepthRef.current = 0;
    setFileDragActive(false);
  }, []);
  const onFileDragEnter = React.useCallback((event: React.DragEvent<HTMLDivElement>) => {
    if (!dragEventContainsFiles(event)) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    if (uploadDropDisabled) {
      return;
    }
    fileDragDepthRef.current += 1;
    setFileDragActive(true);
  }, [uploadDropDisabled]);
  const onFileDragOver = React.useCallback((event: React.DragEvent<HTMLDivElement>) => {
    if (!dragEventContainsFiles(event)) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = uploadDropDisabled ? "none" : "copy";
  }, [uploadDropDisabled]);
  const onFileDragLeave = React.useCallback((event: React.DragEvent<HTMLDivElement>) => {
    if (!dragEventContainsFiles(event)) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    fileDragDepthRef.current = Math.max(0, fileDragDepthRef.current - 1);
    if (fileDragDepthRef.current === 0) {
      setFileDragActive(false);
    }
  }, []);
  const onFileDrop = React.useCallback((event: React.DragEvent<HTMLDivElement>) => {
    if (!dragEventContainsFiles(event)) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    const files = droppedFiles(event);
    resetFileDragState();
    if (uploadDropDisabled || files.length === 0) {
      return;
    }
    void onUploadFiles(files);
  }, [onUploadFiles, resetFileDragState, uploadDropDisabled]);
  React.useEffect(() => {
    if (uploadDropDisabled) {
      resetFileDragState();
    }
  }, [resetFileDragState, uploadDropDisabled]);

  const gatewayDevice = currentConversation?.executionType === "gateway" && currentConversation.executionDeviceID
    ? devices.find((device) => device.deviceId === currentConversation.executionDeviceID) ?? null
    : defaultDevice;
  const gatewayReady = executionMode === "cloud" || Boolean(gatewayDevice?.online) &&
    (Boolean(currentConversation) || Boolean(executionProjectID)) &&
    Boolean(agentSettings) && !agentSettingsLoading && !agentSettingsError &&
    !agentApprovalModeSelectionRequired;
  let gatewayStatus = "";
  if (executionMode === "gateway") {
    if (!gatewayDevice) {
      gatewayStatus = t("submit.deviceUnavailable");
    } else if (!gatewayDevice.online) {
      gatewayStatus = t("submit.deviceOffline");
    } else if (!currentConversation && !executionProjectID) {
      gatewayStatus = t("submit.projectRequired");
    } else if (agentSettingsLoading) {
      gatewayStatus = t("agent.settings.loading");
    } else if (agentSettingsError || !agentSettings) {
      gatewayStatus = agentSettingsError || t("agent.settings.unavailable");
    } else if (agentApprovalModeSelectionRequired) {
      gatewayStatus = t("agent.settings.errors.autoReviewUnavailable");
    }
  }

  const chatInputProps = {
    draft,
    executionMode,
    gatewayReady,
    gatewayStatus,
    loading,
    sending: generating,
    agentSettingsDisabled: executionMode === "gateway" && conversationRunActive,
    uploading,
    isConversationMode,
    maxFilesPerMessage,
    fileMode,
    sendShortcut,
    inputHeight,
    attachments,
    uploadingAttachments,
    modelOptions,
    requestProtocol: selectedChatProtocol,
    selectedKeyBindingID: chatKeyBindings.selectedKeyBindingID,
    selectedPlatformModelName: cloudSelectedPlatformModelName,
    agentModels,
    agentSettings,
    agentSettingsLoading,
    agentSettingsError,
    agentAutoReviewEnabled,
    availableTools: executionMode === "cloud" ? availableTools : [],
    inputResources: executionMode === "gateway" ? inputResources : undefined,
    selectedToolIDs: executionMode === "cloud" ? selectedToolIDs : [],
    selectedSkills: executionMode === "cloud" ? selectedSkills : [],
    selectedInputResources: selectedComposerInputResources,
    defaultToolIDs,
    queuedMessages,
    htmlVisualPromptEnabled: htmlVisualPrompt.enabled,
    maxSelectedTools: mcpMaxSelectedTools,
    toolsLoading,
    options: effectiveOptions,
    defaultOptions: selectedModelDefaultOptions,
    modelOptionPolicy,
    modelLoading: modelsLoading,
    dropActive: fileDragActive,
    onDraftChange: setDraft,
    onModelChange: setSelectedPlatformModelName,
    onAgentSettingsChange,
    onModelCatalogRefresh: refreshModelCatalogForComposer,
    onSelectedToolsChange,
    maxSelectedSkills: mcpMaxSelectedTools,
    onSelectedSkillsChange,
    onSelectedInputResourcesChange: setSelectedInputResources,
    onDefaultToolsChange: onDefaultToolIDsChange,
    onHTMLVisualPromptChange: htmlVisualPrompt.setEnabled,
    onOptionsChange: setModelOptions,
    onAttachExistingFile,
    onUploadFiles,
    onCaptureScreenshot,
    onRemoveAttachment,
    onSendMessage,
    onStopMessage: onStopActiveMessage,
    onDeleteQueuedMessage,
    onEditQueuedMessage,
    onGuideQueuedMessage,
  };
  const chatContentWidthClassName = resolveChatContentWidthClassName(contentWidth);
  const isConversationLoading = Boolean(conversationID) && loading && visibleMessageCount === 0 && messagesWithInlineError.length === 0;
  const isConversationLoadFailed = Boolean(conversationID) && !loading && errorMsg.trim().length > 0 && visibleMessageCount === 0;
  const shouldUseCenteredComposer =
    !isConversationLoading && !isConversationLoadFailed && !isConversationMode && messagesWithInlineError.length === 0;

  return (
    <div
      className="relative flex h-full min-h-0 w-full flex-1 flex-col overflow-hidden md:overflow-visible"
      onDragEnter={onFileDragEnter}
      onDragOver={onFileDragOver}
      onDragLeave={onFileDragLeave}
      onDrop={onFileDrop}
    >
      {shouldUseCenteredComposer ? (
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
          <ChatEmptyState
            greetingTitle={activeRouteProject?.name || greetingTitle}
            badgeLabel={activeRouteProject ? t("projectMode") : undefined}
            badgeTooltip={activeRouteProject ? t("projectModeTooltip") : undefined}
            contentWidthClassName={chatContentWidthClassName}
          >
            <ChatInput {...chatInputProps} />
          </ChatEmptyState>
        </div>
      ) : (
        <div
          ref={workspaceRef}
          className={cn(
            "relative grid min-h-0 flex-1 overflow-hidden",
            artifactResizing
              ? "transition-none"
              : "transition-[grid-template-columns] duration-500 ease-[cubic-bezier(0.16,1,0.3,1)]",
            hasInlineArtifact && "md:overflow-visible",
          )}
          style={{ gridTemplateColumns: workspaceGridColumns }}
        >
          <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
            <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
              {isConversationLoading ? (
                <ChatAreaSkeleton contentWidthClassName={chatContentWidthClassName} />
              ) : isConversationLoadFailed ? (
                <ChatAreaLoadError onRefresh={reload} onNewConversation={onNewConversationFromLoadError} />
              ) : (
                <ChatArea
                  title={activeConversationTitle}
                  starred={activeConversationStarred}
                  canOperateConversation={canOperateConversation}
                  messages={messagesWithInlineError}
                  busy={generating}
                  messageContentRef={messageContentRef}
                  onScroll={onScroll}
                  onRetryUserMessage={onRetryUserMessage}
                  onRetryAssistantMessage={onRetryAssistantMessage}
                  onContinueAssistantMessage={onContinueAssistantMessage}
                  onEditAssistantMessage={onEditAssistantMessage}
                  onEditUserMessage={onEditUserMessage}
                  modelOptions={modelOptions}
                  selectedPlatformModelName={selectedPlatformModelName}
                  onModelChange={setSelectedPlatformModelName}
                  onModelCatalogRefresh={refreshModelCatalogForComposer}
                  onEditImageAttachment={onEditGeneratedImageAttachment}
                  onOpenCodeArtifact={artifactWorkspace.openArtifact}
                  onOpenAgentDiff={artifactWorkspace.openDiff}
                  onCycleMessageBranch={onCycleMessageBranch}
                  onToggleStar={onToggleActiveConversationStar}
                  onRename={onRenameActiveConversation}
                  onAutoRename={onAutoRenameActiveConversation}
                  labels={activeConversationLabels}
                  onUpdateLabels={onUpdateActiveConversationLabels}
                  projectMenu={{
                    label: t("labelMenu.moveToProject"),
                    unassignedLabel: t("labelMenu.unassignedProject"),
                    currentProjectID: currentConversation?.projectID,
                    projects,
                    onSelect: onSetActiveConversationProject,
                  }}
                  onShare={onShareActiveConversation}
                  shareActive={activeConversationShared}
                  onExport={onExportActiveConversation}
                  onDelete={onRequestDeleteActiveConversation}
                  markdownRender={markdownRender}
                  showModelInfo={showModelInfo}
                  showLatency={showLatency}
                  showTokenUsage={showTokenUsage}
                  splitRightInset={hasInlineArtifact}
                  contentWidthClassName={chatContentWidthClassName}
                  onScreenshotFull={screenshot.captureFullConversation}
                  onScreenshotSelect={screenshot.startSelectionScreenshot}
                  screenshot={{
                    selectionMode: screenshot.selectionMode,
                    selectedIDs: screenshot.selectedIDs,
                    selectedCount: screenshot.selectedCount,
                    capturing: screenshot.capturing,
                    onToggleSelection: screenshot.toggleSelection,
                    onSelectAll: screenshot.selectMany,
                    onClearSelection: screenshot.clearSelection,
                    onPruneSelection: screenshot.pruneSelection,
                    onCapture: screenshot.captureSelectedMessages,
                    onExit: screenshot.exitSelectionMode,
                  }}
                />
              )}
            </div>

            {!isConversationLoadFailed ? (
              <div className="relative z-10 shrink-0 px-3 pb-3 md:px-6">
                <div className={cn("mx-auto w-full", chatContentWidthClassName)}>
                  <ChatInput {...chatInputProps} />
                </div>
              </div>
            ) : null}
          </div>

          <ChatArtifactWorkspace
            artifact={artifactWorkspace.activeArtifact}
            diff={artifactWorkspace.activeDiff}
            artifacts={artifactWorkspace.artifacts}
            isInlineViewport={artifactWorkspace.isInlineViewport}
            onArtifactChange={artifactWorkspace.selectArtifact}
            onClose={artifactWorkspace.closeArtifact}
            onResizeReset={artifactWorkspace.resetArtifactRatio}
            onResizeStart={onArtifactResizeStart}
          />
        </div>
      )}

      <ChatScreenshotPreviewDialog
        open={screenshotPreviewOpen}
        onOpenChange={(open) => {
          if (!open) {
            closeScreenshotPreviewDialog();
          }
        }}
        previewURL={screenshotPreview?.url ?? null}
        clipboardSupported={screenshot.clipboardSupported}
        onDownload={screenshot.downloadPreview}
        onCopy={screenshot.copyPreviewToClipboard}
      />

      {canOperateConversation ? (
        <>
          <ConversationShareDialog
            open={shareDialogOpen}
            onOpenChange={setShareDialogOpen}
            conversationPublicID={actionConversationID}
            conversationTitle={activeConversationTitle}
            defaultMessagePublicIDs={shareDefaultMessagePublicIDs}
            onShareChange={(share) => {
              touchByPublicID(actionConversationID, sharePatchFromDTO(share));
            }}
          />

          <AlertDialog
            open={deleteDialogOpen}
            onOpenChange={(open) => {
              setDeleteDialogOpen(open);
              if (!open) {
                setDeleteFiles(false);
              }
            }}
          >
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>{tRecent("dialogs.deleteTitle")}</AlertDialogTitle>
                <AlertDialogDescription>
                  {tRecent(deleteExecutionType === "gateway" ? "dialogs.deleteWorkDescription" : "dialogs.deleteDescription", {
                    label: tRecent("deleteConversationLabel", { title: activeConversationTitle }),
                  })}
                </AlertDialogDescription>
                {deleteExecutionType === "cloud" ? (
                  <DeleteFilesOption
                    id={deleteFilesID}
                    checked={deleteFiles}
                    onCheckedChange={setDeleteFiles}
                  />
                ) : null}
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>{tRecent("dialogs.cancel")}</AlertDialogCancel>
                <AlertDialogAction variant="destructive" onClick={() => void onConfirmDeleteActiveConversation()}>
                  {tRecent("dialogs.delete")}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </>
      ) : null}
    </div>
  );
}
