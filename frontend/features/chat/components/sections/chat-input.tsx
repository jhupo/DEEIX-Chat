"use client";

import { Box, CornerDownRight, Eye, EyeOff, Film, Image, ImageOff, ImagePlus, LoaderCircle, PencilLine, Plug, Trash2 } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import dynamic from "next/dynamic";
import { useLocale, useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";

import { AudioLines } from "@/components/animate-ui/icons/audio-lines";
import { Blocks } from "@/components/animate-ui/icons/blocks";
import { Crop } from "@/components/animate-ui/icons/crop";
import { Link as LinkIcon } from "@/components/animate-ui/icons/link";
import { Pause } from "@/components/animate-ui/icons/pause";
import { Send } from "@/components/animate-ui/icons/send";
import { X as XIcon } from "@/components/animate-ui/icons/x";
import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
  AttachmentTrigger,
} from "@/components/ui/attachment";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group";
import { PlusIcon } from "@/components/ui/plus";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { MessageAgentComposerActivity } from "@/features/chat/components/message/message-agent-activity";
import { ChatAgentSettings } from "@/features/chat/components/sections/chat-agent-settings";
import { ChatMCP } from "@/features/chat/components/sections/chat-mcp";
import { ChatModelPicker } from "@/features/chat/components/sections/chat-model-picker";
import { ChatMentionMenuPortal } from "@/features/chat/components/shared/chat-mention-menu";
import {
  type ChatMentionMenuKind,
  useChatMentionMenu,
} from "@/features/chat/hooks/use-chat-mention-menu";
import {
  type SpeechInputErrorCode,
  useChatSpeechInput,
} from "@/features/chat/hooks/use-chat-speech-input";
import { useMarkdownPreviewSync } from "@/features/chat/hooks/use-markdown-preview-sync";
import {
  resolveChatModelReasoningSetting,
  setChatModelReasoningEffort,
} from "@/features/chat/model/chat-model-reasoning";
import type { ChatSubmitDecision } from "@/features/chat/model/chat-task";
import { isMediaSubmitTask, resolveChatSubmitDecision } from "@/features/chat/model/chat-task";
import type {
  ChatModelOption,
  PendingAttachment,
  UploadingAttachment,
} from "@/features/chat/types/chat-runtime";
import {
  formatClipboardMarkdownPaste,
  resolveClipboardMarkdownPaste,
} from "@/features/chat/utils/markdown-paste";
import type { SendShortcut } from "@/features/settings/types/settings";
import { cn } from "@/lib/utils";
import type { AgentModelDTO, AgentTurnSettings } from "@/shared/api/agent-gateway";
import type { ConversationInputResourceDTO, ConversationOptions } from "@/shared/api/conversation.types";
import type { FileObjectDTO } from "@/shared/api/file.types";
import type { MCPToolDTO } from "@/shared/api/mcp.types";
import type { SkillSummaryDTO } from "@/shared/api/skills.types";
import { StreamdownRender } from "@/shared/components/markdown/streamdown-render";
import { useDialogSnapshot } from "@/shared/hooks/use-dialog-snapshot";
import { formatBytes, resolveFileExtension, resolveFileIcon } from "@/shared/lib/file-display";
import { resolveFileProcessingBadge } from "@/shared/lib/file-processing";
import {
  isModelOptionPathFiltered,
  type ModelOptionPolicy,
} from "@/shared/lib/model-option-policy";
import { isSendShortcutEvent } from "@/shared/lib/platform-shortcuts";

const FilePreviewDialog = dynamic(
  () => import("@/shared/components/file-preview/preview-dialog").then((module) => module.FilePreviewDialog),
  { ssr: false },
);

type QueuedComposerMessage = {
  id: string;
  content: string;
  attachmentCount: number;
  executionMode: "cloud" | "gateway";
  steering: boolean;
};

type ChatInputProps = {
  draft: string;
  executionMode: "cloud" | "gateway";
  gatewayReady: boolean;
  gatewayStatus?: string;
  loading: boolean;
  sending: boolean;
  agentRunID?: string;
  agentSettingsDisabled: boolean;
  uploading: boolean;
  isConversationMode: boolean;
  maxFilesPerMessage: number;
  fileMode?: "auto" | "full_context" | "rag";
  sendShortcut?: SendShortcut;
  inputHeight?: "compact" | "standard" | "loose";
  attachments: PendingAttachment[];
  uploadingAttachments: UploadingAttachment[];
  modelOptions: ChatModelOption[];
  requestProtocol: string;
  selectedKeyBindingID: string;
  selectedPlatformModelName: string;
  agentModels: AgentModelDTO[];
  agentSettings: AgentTurnSettings | null;
  agentSettingsLoading: boolean;
  agentSettingsError?: string;
  agentAutoReviewEnabled: boolean;
  availableTools: MCPToolDTO[];
  inputResources?: ConversationInputResourceDTO[];
  selectedToolIDs: number[];
  selectedSkills: SkillSummaryDTO[];
  selectedInputResources: ConversationInputResourceDTO[];
  defaultToolIDs: number[];
  queuedMessages: QueuedComposerMessage[];
  htmlVisualPromptEnabled: boolean;
  maxSelectedTools: number;
  maxSelectedSkills: number;
  toolsLoading: boolean;
  options: ConversationOptions;
  defaultOptions: ConversationOptions;
  modelOptionPolicy: ModelOptionPolicy | null;
  modelLoading: boolean;
  modelDisabled?: boolean;
  dropActive?: boolean;
  onDraftChange: (value: string) => void;
  onModelChange: (platformModelName: string) => void;
  onAgentSettingsChange: (settings: AgentTurnSettings) => void;
  onModelCatalogRefresh?: () => void | Promise<void>;
  onSelectedToolsChange: (toolIDs: number[]) => void;
  onSelectedSkillsChange: (skills: SkillSummaryDTO[]) => void;
  onSelectedInputResourcesChange: (resources: ConversationInputResourceDTO[]) => void;
  onDefaultToolsChange: (toolIDs: number[]) => void | Promise<void>;
  onHTMLVisualPromptChange: (enabled: boolean) => void;
  onOptionsChange: React.Dispatch<React.SetStateAction<ConversationOptions>>;
  onAttachExistingFile: (file: FileObjectDTO) => void | Promise<void>;
  onUploadFiles: (files: File[]) => void | Promise<void>;
  onCaptureScreenshot: () => void | Promise<void>;
  onRemoveAttachment: (fileID: string) => void;
  onSendMessage: () => void | Promise<void>;
  onStopMessage: () => void;
  onDeleteQueuedMessage: (id: string) => void;
  onEditQueuedMessage: (id: string, content: string) => void;
  onGuideQueuedMessage: (id: string) => void;
};

type ComposerModeIndicator = {
  label: string;
  intro: string;
  description: string;
  icon: React.ComponentType<{ className?: string; strokeWidth?: number }>;
  tone: "default" | "warning";
};

function resolveComposerModeIndicator(
  decision: ChatSubmitDecision,
  t: (key: string) => string,
): ComposerModeIndicator | null {
  if (
    decision.blockedReason === "image_task_rejects_non_image_attachments" ||
    decision.blockedReason === "video_task_rejects_non_image_attachments"
  ) {
    return {
      label: t("mediaMode.invalidFile"),
      intro: t("mediaMode.invalidFileIntro"),
      description: t(`mediaMode.blockedDescriptions.${decision.blockedReason}`),
      icon: ImageOff,
      tone: "warning",
    };
  }
  if (decision.task === "image_generation") {
    return {
      label: t("mediaMode.imageGeneration"),
      intro: t("mediaMode.imageGenerationIntro"),
      description: decision.blockedReason
        ? t(`mediaMode.blockedDescriptions.${decision.blockedReason}`)
        : t("mediaMode.imageGenerationDescription"),
      icon: Image,
      tone: "default",
    };
  }
  if (decision.task === "image_edit") {
    return {
      label: t("mediaMode.imageEdit"),
      intro: t("mediaMode.imageEditIntro"),
      description: decision.blockedReason
        ? t(`mediaMode.blockedDescriptions.${decision.blockedReason}`)
        : t("mediaMode.imageEditDescription"),
      icon: ImagePlus,
      tone: "default",
    };
  }
  if (decision.task === "video_generation") {
    return {
      label: t("mediaMode.videoGeneration"),
      intro: t("mediaMode.videoGenerationIntro"),
      description: decision.blockedReason
        ? t(`mediaMode.blockedDescriptions.${decision.blockedReason}`)
        : t("mediaMode.videoGenerationDescription"),
      icon: Film,
      tone: "default",
    };
  }
  return null;
}

function clipboardFilesFromPaste(event: React.ClipboardEvent<HTMLTextAreaElement>): File[] {
  const itemFiles = Array.from(event.clipboardData.items ?? [])
    .filter((item) => item.kind === "file")
    .map((item) => item.getAsFile())
    .filter((file): file is File => file !== null);
  const sourceFiles = itemFiles.length > 0 ? itemFiles : Array.from(event.clipboardData.files ?? []);
  const pastedAt = Date.now();

  return sourceFiles.map((file, index) => {
    if (file.name.trim()) {
      return file;
    }
    const extension = file.type.startsWith("image/") ? ".png" : "";
    const prefix = file.type.startsWith("image/") ? "pasted-image" : "pasted-file";
    return new File([file], `${prefix}-${pastedAt}-${index + 1}${extension}`, {
      type: file.type,
      lastModified: file.lastModified,
    });
  });
}

function formatAttachmentFileType(fileName: string) {
  return resolveFileExtension(fileName).toUpperCase() || "FILE";
}

function formatAttachmentMeta(fileName: string, sizeBytes: number) {
  return `${formatAttachmentFileType(fileName)} · ${formatBytes(sizeBytes)}`;
}

function ChatInputComponent({
  draft,
  executionMode,
  gatewayReady,
  gatewayStatus,
  loading,
  sending,
  agentRunID,
  agentSettingsDisabled,
  uploading,
  isConversationMode,
  fileMode,
  sendShortcut = "enter",
  inputHeight = "standard",
  attachments,
  uploadingAttachments,
  modelOptions,
  requestProtocol,
  selectedKeyBindingID,
  selectedPlatformModelName,
  agentModels,
  agentSettings,
  agentSettingsLoading,
  agentSettingsError,
  agentAutoReviewEnabled,
  availableTools,
  inputResources,
  selectedToolIDs,
  selectedSkills,
  selectedInputResources,
  defaultToolIDs,
  queuedMessages,
  htmlVisualPromptEnabled,
  maxSelectedTools,
  maxSelectedSkills,
  toolsLoading,
  options,
  defaultOptions,
  modelOptionPolicy,
  modelLoading,
  modelDisabled = false,
  dropActive = false,
  onDraftChange,
  onModelChange,
  onAgentSettingsChange,
  onModelCatalogRefresh,
  onSelectedToolsChange,
  onSelectedSkillsChange,
  onSelectedInputResourcesChange,
  onDefaultToolsChange,
  onHTMLVisualPromptChange,
  onOptionsChange,
  onAttachExistingFile,
  onUploadFiles,
  onCaptureScreenshot,
  onRemoveAttachment,
  onSendMessage,
  onStopMessage,
  onDeleteQueuedMessage,
  onEditQueuedMessage,
  onGuideQueuedMessage,
}: ChatInputProps) {
  const tChat = useTranslations("chat");
  const tComposer = useTranslations("chat.composer");
  const tFileStatus = useTranslations("files.status");
  const locale = useLocale();
  const [isBlocksHovered, setIsBlocksHovered] = React.useState(false);
  const [isVoiceHovered, setIsVoiceHovered] = React.useState(false);
  const [toolsMenuHovered, setToolsMenuHovered] = React.useState(false);
  const [toolsMenuOpen, setToolsMenuOpen] = React.useState(false);
  const [editingQueuedMessageID, setEditingQueuedMessageID] = React.useState<string | null>(null);
  const [editingQueuedMessageContent, setEditingQueuedMessageContent] = React.useState("");
  const handleSpeechInputError = React.useCallback((error: SpeechInputErrorCode) => {
    toast.error(tComposer("voiceErrorTitle"), {
      id: "chat-speech-input-error",
      description: tComposer(`voiceErrors.${error}`),
    });
  }, [tComposer]);
  const speechInput = useChatSpeechInput({
    draft,
    language: locale,
    listeningPlaceholder: tComposer("voiceListeningPlaceholder"),
    onDraftChange,
    onError: handleSpeechInputError,
    placeholder: tComposer("inputPlaceholder"),
    startingPlaceholder: tComposer("voiceStartingPlaceholder"),
  });
  const [hoveredTool, setHoveredTool] = React.useState<"upload" | "screenshot" | null>(null);
  const [ragWarnDismissed, setRagWarnDismissed] = React.useState(false);
  const [previewAttachment, setPreviewAttachment] = React.useState<PendingAttachment | null>(null);
  const [markdownPreview, setMarkdownPreview] = React.useState(false);
  const stablePreviewAttachment = useDialogSnapshot(previewAttachment);
  const fileInputRef = React.useRef<HTMLInputElement | null>(null);
  const inputGroupRef = React.useRef<HTMLDivElement | null>(null);
  const inputGroupMeasureRef = React.useRef<HTMLDivElement | null>(null);
  const textareaRef = React.useRef<HTMLTextAreaElement | null>(null);
  const markdownPreviewRef = React.useRef<HTMLDivElement | null>(null);
  const composingRef = React.useRef(false);
  const [inputGroupHeight, setInputGroupHeight] = React.useState<number | null>(null);
  const hasDraftText = draft.trim().length > 0;
  const hasSubmitContent = hasDraftText || attachments.length > 0;
  const showMarkdownPreview = markdownPreview && hasDraftText;
  const inputHeightClassName =
    inputHeight === "compact" ? "max-h-32" : inputHeight === "loose" ? "max-h-64" : "max-h-44";
  const { onPreviewScroll, onSourceScroll } = useMarkdownPreviewSync({
    enabled: showMarkdownPreview,
    previewRef: markdownPreviewRef,
    source: draft,
    textareaRef,
  });

  // Only relevant in RAG mode: all document attachments opted out of RAG.
  const docAttachments = attachments.filter(
    (attachment) => attachment.fileCategory !== "image" && attachment.fileCategory !== "video",
  );
  const allRagOptOut =
    fileMode === "rag" &&
    docAttachments.length > 0 &&
    docAttachments.every((a) => a.ragOptOut === true);
  const showRagWarn = allRagOptOut && !ragWarnDismissed;

  const closePreviewDialog = React.useCallback((open: boolean) => {
    if (!open) {
      setPreviewAttachment(null);
    }
  }, []);

  React.useEffect(() => {
    if (!hasDraftText) {
      setMarkdownPreview(false);
    }
  }, [hasDraftText]);

  React.useLayoutEffect(() => {
    const node = inputGroupMeasureRef.current;
    if (!node || typeof ResizeObserver === "undefined") {
      setInputGroupHeight(null);
      return;
    }

    let frameID = 0;
    const measure = () => {
      const inputGroupNode = inputGroupRef.current;
      const inputGroupStyle = inputGroupNode ? window.getComputedStyle(inputGroupNode) : null;
      const borderHeight =
        (Number.parseFloat(inputGroupStyle?.borderTopWidth ?? "") || 0) +
        (Number.parseFloat(inputGroupStyle?.borderBottomWidth ?? "") || 0);
      const contentHeight = node.scrollHeight || node.offsetHeight || node.getBoundingClientRect().height;
      const nextHeight = Math.ceil(contentHeight + borderHeight);
      if (nextHeight <= 0) {
        return;
      }
      setInputGroupHeight((previousHeight) => (previousHeight === nextHeight ? previousHeight : nextHeight));
    };

    measure();
    const scheduleMeasure = () => {
      window.cancelAnimationFrame(frameID);
      frameID = window.requestAnimationFrame(measure);
    };
    const resizeObserver = new ResizeObserver(scheduleMeasure);
    resizeObserver.observe(node);
    window.addEventListener("resize", scheduleMeasure);

    return () => {
      window.cancelAnimationFrame(frameID);
      window.removeEventListener("resize", scheduleMeasure);
      resizeObserver.disconnect();
    };
  }, []);

  const selectedModel = React.useMemo(
    () => executionMode === "cloud"
      ? modelOptions.find((item) => item.platformModelName === selectedPlatformModelName) ?? null
      : null,
    [executionMode, modelOptions, selectedPlatformModelName],
  );
  const selectedProtocol = requestProtocol.trim();
  const selectedAgentModel = React.useMemo(
    () => agentSettings ? agentModels.find((model) => model.id === agentSettings.model) ?? null : null,
    [agentModels, agentSettings],
  );
  const agentPickerModels = React.useMemo<ChatModelOption[]>(
    () => agentModels.map((model) => ({
      platformModelName: model.id,
      displayName: model.displayName,
      description: model.description,
      icon: "openai",
      vendor: "openai",
      vendorName: "OpenAI",
      vendorIcon: "openai",
      displayGroupID: null,
      displayGroupName: "",
      displayGroupIcon: "",
      kinds: ["chat"],
      protocols: [],
      defaultOptions: {},
      optionControls: [],
      lockedOptionPaths: [],
      nativeToolKeys: [],
      nativeTools: [],
    })),
    [agentModels],
  );
  const cloudReasoningSetting = React.useMemo(() => {
    if (!selectedModel) {
      return null;
    }
    const setting = resolveChatModelReasoningSetting({
      options,
      defaultOptions,
      optionControls: selectedModel.optionControls,
      lockedOptionPaths: selectedModel.lockedOptionPaths,
      isPathAvailable: (path) =>
        !modelOptionPolicy ||
        !isModelOptionPathFiltered({ policy: modelOptionPolicy, protocol: selectedProtocol, path }),
    });
    return setting;
  }, [defaultOptions, modelOptionPolicy, options, selectedModel, selectedProtocol]);
  const changePickerModel = React.useCallback((modelID: string) => {
    if (executionMode === "cloud") {
      onModelChange(modelID);
      return;
    }
    if (!agentSettings) {
      return;
    }
    const model = agentModels.find((item) => item.id === modelID);
    if (!model) {
      return;
    }
    onAgentSettingsChange({
      ...agentSettings,
      model: model.id,
      reasoningEffort: model.supportedReasoningEfforts.includes(agentSettings.reasoningEffort)
        ? agentSettings.reasoningEffort
        : model.defaultReasoningEffort,
    });
  }, [agentModels, agentSettings, executionMode, onAgentSettingsChange, onModelChange]);
  const changePickerReasoning = React.useCallback((value: string) => {
    if (executionMode === "gateway") {
      if (
        agentSettings && selectedAgentModel?.supportedReasoningEfforts.includes(
          value as AgentTurnSettings["reasoningEffort"],
        )
      ) {
        onAgentSettingsChange({
          ...agentSettings,
          reasoningEffort: value as AgentTurnSettings["reasoningEffort"],
        });
      }
      return;
    }
    if (!cloudReasoningSetting || !cloudReasoningSetting.options.includes(value)) {
      return;
    }
    onOptionsChange((current) => setChatModelReasoningEffort(current, cloudReasoningSetting.path, value));
  }, [agentSettings, cloudReasoningSetting, executionMode, onAgentSettingsChange, onOptionsChange, selectedAgentModel]);
  const selectedModelName = selectedModel?.platformModelName || selectedPlatformModelName;
  const submitDecision = resolveChatSubmitDecision(
    selectedModel,
    attachments,
    executionMode === "cloud" ? options : {},
    selectedInputResources,
  );
  const submitTask = submitDecision.task;
  const chatRouteReady = executionMode === "gateway"
    ? gatewayReady
    : Boolean(selectedKeyBindingID && selectedModelName && selectedProtocol);
  const canSend = hasSubmitContent && !loading && !uploading && chatRouteReady;
  const handleSendButtonClick = React.useCallback(() => {
    if (hasSubmitContent && !chatRouteReady) {
      toast.error(tChat("submit.sendFailed"), {
        description: executionMode === "gateway"
          ? gatewayStatus || tChat("submit.deviceOffline")
          : tChat("submit.selectModelFirst"),
      });
      return;
    }
    if (hasSubmitContent) {
      void onSendMessage();
      return;
    }
    if (sending) {
      onStopMessage();
      return;
    }
    speechInput.toggle();
  }, [
    chatRouteReady,
    executionMode,
    gatewayStatus,
    hasSubmitContent,
    onSendMessage,
    onStopMessage,
    sending,
    speechInput,
    tChat,
  ]);
  const isMediaMode = isMediaSubmitTask(submitTask);
  const composerModeIndicator = resolveComposerModeIndicator(submitDecision, tComposer);
  const ComposerModeIcon = composerModeIndicator?.icon;
  const showMCPToolsButton = availableTools.length > 0 && !isMediaMode;
  const showHTMLVisualPromptButton = executionMode === "cloud" && !isMediaMode;
  const hasComposerAttachments = attachments.length > 0 || uploadingAttachments.length > 0;
  const showSelectedResources = (selectedSkills.length > 0 || selectedInputResources.length > 0) && !isMediaMode;
  const {
    activeIndex: mentionActiveIndex,
    handleBlur: handleMentionBlur,
    handleChange: handleMentionChange,
    handleFocus: handleMentionFocus,
    handleKeyDown: handleMentionKeyDown,
    handleSelectionChange: handleMentionSelectionChange,
    menuID: mentionMenuID,
    menuLayout: mentionMenuLayout,
    menuRef: mentionMenuRef,
    menuReady: mentionMenuReady,
    open: showMentionMenu,
    sections: mentionSections,
    select: selectMentionItem,
  } = useChatMentionMenu({
    inputResources,
    disabled: loading || uploading || modelLoading || modelDisabled,
    draft,
    maxSelectedSkills,
    selectedSkills,
    selectedInputResources,
    anchorRef: inputGroupRef,
    textareaRef,
    onDraftChange,
    onSelectedSkillsChange,
    onSelectedInputResourcesChange,
    placementAnchor: "container",
    placementPreference: isConversationMode ? "top" : "bottom",
    onSkillLimitReached: () => {
      toast.error(tComposer("skillLimitTitle"), {
        description: tComposer("skillLimitDescription", { limit: maxSelectedSkills }),
      });
    },
  });
  const mentionSectionOffsets = React.useMemo(() => {
    const offsets = new Map<ChatMentionMenuKind, number>();
    let offset = 0;
    for (const section of mentionSections) {
      offsets.set(section.kind, offset);
      offset += section.items.length;
    }
    return offsets;
  }, [mentionSections]);
  const onSelectUploadTool = React.useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const onSelectScreenshotTool = React.useCallback(() => {
    void onCaptureScreenshot();
  }, [onCaptureScreenshot]);

  const finishQueuedMessageEdit = React.useCallback(() => {
    const id = editingQueuedMessageID;
    if (!id) {
      return;
    }
    const message = queuedMessages.find((item) => item.id === id);
    const content = editingQueuedMessageContent.trim();
    if (content.length === 0 && message?.attachmentCount === 0) {
      onDeleteQueuedMessage(id);
    } else {
      onEditQueuedMessage(id, content);
    }
    setEditingQueuedMessageID(null);
    setEditingQueuedMessageContent("");
  }, [editingQueuedMessageContent, editingQueuedMessageID, onDeleteQueuedMessage, onEditQueuedMessage, queuedMessages]);

  return (
    <div className="relative w-full text-left">
      <input
        ref={fileInputRef}
        type="file"
        multiple
        className="sr-only "
        onChange={(event) => {
          const files = Array.from(event.target.files ?? []);
          if (files.length > 0) {
            void onUploadFiles(files);
          }
          event.currentTarget.value = "";
        }}
      />

      {queuedMessages.length > 0 ? (
        <div className="relative z-0 mx-4 mb-[-10px] overflow-hidden rounded-t-2xl rounded-b-xl border border-border/30 bg-sidebar-accent/55 px-4 pb-4 pt-2 shadow-none">
          <div className="max-h-24 space-y-0.5 overflow-y-auto pr-1">
            {queuedMessages.map((message) => {
              const editing = editingQueuedMessageID === message.id;
              const label =
                message.content ||
                (message.attachmentCount > 0
                  ? tComposer("queuedAttachmentOnly", { count: message.attachmentCount })
                  : tComposer("queuedEmptyMessage"));
              return (
                <div
                  key={message.id}
                  className="group flex min-h-6 items-center gap-2 rounded-md px-0.5 text-[13px] text-muted-foreground"
                >
                  <CornerDownRight className="size-3 shrink-0 text-muted-foreground/55" strokeWidth={1.8} />
                  {editing ? (
                    <input
                      autoFocus
                      value={editingQueuedMessageContent}
                      className="min-w-0 flex-1 bg-transparent text-[13px] font-medium text-foreground outline-none placeholder:text-muted-foreground"
                      placeholder={tComposer("queuedEditPlaceholder")}
                      onBlur={finishQueuedMessageEdit}
                      onChange={(event) => setEditingQueuedMessageContent(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Escape") {
                          event.preventDefault();
                          setEditingQueuedMessageID(null);
                          setEditingQueuedMessageContent("");
                          return;
                        }
                        if (event.key === "Enter") {
                          event.preventDefault();
                          finishQueuedMessageEdit();
                        }
                      }}
                    />
                  ) : (
                    <button
                      type="button"
                      className="flex min-w-0 flex-1 items-center text-left font-medium text-muted-foreground transition-colors hover:text-foreground"
                      disabled={message.steering}
                      aria-label={tComposer("editQueuedMessage")}
                      onClick={() => {
                        setEditingQueuedMessageID(message.id);
                        setEditingQueuedMessageContent(message.content);
                      }}
                    >
                      <span className="min-w-0 truncate">{label}</span>
                      {message.content && message.attachmentCount > 0 ? (
                        <span className="ml-2 shrink-0 text-[11px] font-normal text-muted-foreground/60">
                          {tComposer("queuedAttachmentCount", { count: message.attachmentCount })}
                        </span>
                      ) : null}
                    </button>
                  )}
                  <div className="flex shrink-0 items-center gap-0.5">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          className="inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-background/70 hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/35 disabled:pointer-events-none disabled:opacity-50"
                          disabled={message.steering}
                          aria-busy={message.steering}
                          aria-label={tComposer(message.executionMode === "gateway" ? "steerQueuedMessageTitle" : "guideQueuedMessageTitle")}
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => {
                            onGuideQueuedMessage(message.id);
                            if (message.executionMode === "cloud" && sending) {
                              onStopMessage();
                            }
                          }}
                        >
                          <CornerDownRight className="size-3.5" strokeWidth={1.7} />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side="top" className="text-xs">
                        {tComposer(message.executionMode === "gateway" ? "steerQueuedMessageTitle" : "guideQueuedMessageTitle")}
                      </TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          className="inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-background/70 hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/35"
                          disabled={message.steering}
                          aria-label={tComposer("editQueuedMessage")}
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => {
                            setEditingQueuedMessageID(message.id);
                            setEditingQueuedMessageContent(message.content);
                          }}
                        >
                          <PencilLine className="size-3.5" strokeWidth={1.7} />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side="top" className="text-xs">
                        {tComposer("editQueuedMessage")}
                      </TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          className="inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-background/70 hover:text-destructive focus-visible:ring-[3px] focus-visible:ring-ring/35"
                          disabled={message.steering}
                          aria-label={tComposer("deleteQueuedMessage")}
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => onDeleteQueuedMessage(message.id)}
                        >
                          <Trash2 className="size-3.5" strokeWidth={1.7} />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent side="top" className="text-xs">
                        {tComposer("deleteQueuedMessage")}
                      </TooltipContent>
                    </Tooltip>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      ) : null}

      <MessageAgentComposerActivity runID={agentRunID} />

      <AnimatePresence initial={false}>
        {showMarkdownPreview && inputGroupHeight !== null ? (
          <motion.div
            ref={markdownPreviewRef}
            key="markdown-preview"
            role="region"
            aria-label={tComposer("markdownPreview")}
            className="absolute inset-x-0 z-[60] max-h-[40dvh] min-h-16 overflow-y-auto rounded-xl border-[0.5px] border-border/70 bg-pure/85 px-5 py-4 text-[15px] text-foreground shadow-xs backdrop-blur-xl scroll-fade-12"
            style={{ bottom: inputGroupHeight + 8 }}
            initial={{ opacity: 0, scale: 0.99, y: 4 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.99, y: 4 }}
            transition={{ duration: 0.12, ease: "easeOut" }}
            onScroll={onPreviewScroll}
          >
            <StreamdownRender content={draft} variant="user" sourcePositions />
          </motion.div>
        ) : null}
      </AnimatePresence>

      <InputGroup
        ref={inputGroupRef}
        className={cn(
          "relative z-10 flex-col items-stretch overflow-hidden rounded-3xl border-[0.5px] border-border/70 bg-pure shadow-xs transition-[height,border-color,background-color,box-shadow] duration-150 ease-out motion-reduce:transition-none has-[[data-slot=input-group-control]:focus-visible]:border-border has-[[data-slot=input-group-control]:focus-visible]:ring-0",
          inputGroupHeight === null && "h-auto",
          dropActive && "border-dashed border-foreground/30 bg-muted/20 shadow-none",
        )}
        style={inputGroupHeight === null ? undefined : { height: inputGroupHeight }}
      >
        <div ref={inputGroupMeasureRef} className="flex w-full flex-col">
          {showSelectedResources ? (
            <div className="flex w-full max-h-14 flex-wrap items-center justify-start gap-x-3 gap-y-1 overflow-y-auto px-5 pt-3">
              {selectedSkills.map((skill) => (
                <button
                  key={skill.id}
                  type="button"
                  className="group inline-flex h-6 max-w-48 items-center gap-1.5 text-sm font-medium text-primary transition-colors hover:text-primary/85 disabled:opacity-60"
                  disabled={loading || uploading}
                  onClick={() => onSelectedSkillsChange(selectedSkills.filter((item) => item.id !== skill.id))}
                  aria-label={skill.title}
                >
                  <Box className="size-4 shrink-0" strokeWidth={1.7} />
                  <span className="min-w-0 truncate">{skill.trigger || skill.title}</span>
                  <XIcon
                    size={12}
                    strokeWidth={1.7}
                    className="shrink-0 opacity-45 transition-opacity group-hover:opacity-80"
                  />
                </button>
              ))}
              {selectedInputResources.map((resource) => {
                const ResourceIcon = resource.kind === "skill" ? Box : Plug;
                return (
                  <button
                    key={resource.resourceRef}
                    type="button"
                    className="group inline-flex h-6 max-w-48 items-center gap-1.5 text-sm font-medium text-primary transition-colors hover:text-primary/85 disabled:opacity-60"
                    disabled={loading || uploading}
                    onClick={() => onSelectedInputResourcesChange(
                      selectedInputResources.filter((item) => item.resourceRef !== resource.resourceRef),
                    )}
                    aria-label={resource.name}
                  >
                    <ResourceIcon className="size-4 shrink-0" strokeWidth={1.7} />
                    <span className="min-w-0 truncate">{resource.name}</span>
                    <XIcon
                      size={12}
                      strokeWidth={1.7}
                      className="shrink-0 opacity-45 transition-opacity group-hover:opacity-80"
                    />
                  </button>
                );
              })}
            </div>
          ) : null}

          {hasComposerAttachments ? (
            <div className="w-full space-y-1 px-2.5 pt-1">
              {showRagWarn ? (
                <div className="flex items-center gap-2 rounded-lg border border-amber-200/70 bg-amber-50/70 px-3 py-2 text-[11px] text-amber-700 dark:border-amber-700/40 dark:bg-amber-950/30 dark:text-amber-400">
                  <span className="shrink-0">⚠</span>
                  <span className="flex-1">{tComposer("ragAllDisabled")}</span>
                  <button
                    type="button"
                    className="shrink-0 text-amber-500 hover:text-amber-700 dark:text-amber-500 dark:hover:text-amber-300"
                    onClick={() => setRagWarnDismissed(true)}
                    aria-label={tComposer("closeHint")}
                  >
                    ✕
                  </button>
                </div>
              ) : null}
              <AttachmentGroup className="max-h-[196px] w-full flex-col gap-2 overflow-y-auto scroll-fade-12 px-1.5 pb-1 pt-1 [-ms-overflow-style:none] [scrollbar-width:none] max-sm:scroll-fade-none sm:max-h-none sm:flex-row sm:scroll-fade-x sm:overflow-x-auto sm:overflow-y-visible sm:pr-1.5 [&::-webkit-scrollbar]:hidden">
                {attachments.map((item) => {
                  const badge = resolveFileProcessingBadge(item, (key, values) => tFileStatus(key, values));
                  const FileIcon = resolveFileIcon(item);
                  const failed = badge.tone === "danger" || badge.tone === "warning";
                  const processing = !failed && badge.tone !== "success";
                  const meta = formatAttachmentMeta(item.fileName, item.sizeBytes);
                  return (
                    <Attachment
                      key={item.fileID}
                      state={failed ? "error" : processing ? "processing" : "done"}
                      size="sm"
                      className="h-12 w-full border-0 bg-muted/35 px-2 text-left hover:bg-muted/50 dark:bg-white/[0.06] dark:hover:bg-white/[0.09] sm:w-[228px] sm:px-2.5"
                    >
                      <AttachmentMedia className="size-6 bg-transparent text-muted-foreground">
                        {processing ? (
                          <LoaderCircle className="size-5 animate-spin" strokeWidth={1.8} />
                        ) : (
                          <FileIcon className="size-5" strokeWidth={1.6} />
                        )}
                      </AttachmentMedia>
                      <AttachmentContent className="flex min-w-0 flex-1 flex-col justify-center px-0 py-0">
                        <AttachmentTitle className="text-[12px] leading-4 text-foreground/90" title={item.fileName}>
                          {item.fileName}
                        </AttachmentTitle>
                        <AttachmentDescription className="mt-1 flex min-w-0 items-center gap-1.5 text-[11px] leading-none">
                          <span className="min-w-0 shrink truncate" title={failed ? badge.detail : undefined}>
                            {failed ? `${badge.label} · ${meta}` : meta}
                          </span>
                          {item.ragOptOut && item.fileCategory !== "image" ? (
                            <span
                              className="shrink-0 rounded-md bg-muted/60 px-1.5 py-0.5 text-[10px] font-medium leading-none text-muted-foreground/65"
                              title={tComposer("ragDisabledTitle")}
                            >
                              {tComposer("ragOff")}
                            </span>
                          ) : null}
                        </AttachmentDescription>
                      </AttachmentContent>
                      <AttachmentTrigger
                        onClick={() => setPreviewAttachment(item)}
                        aria-label={tComposer("previewAttachment", { name: item.fileName })}
                      />
                      <AttachmentActions>
                        <AttachmentAction
                          type="button"
                          className="size-8 rounded-md text-muted-foreground hover:bg-accent hover:text-foreground sm:size-7"
                          onClick={() => onRemoveAttachment(item.fileID)}
                          aria-label={tComposer("removeAttachment", { name: item.fileName })}
                        >
                          <XIcon size={15} strokeWidth={1.8} animateOnHover="default" />
                        </AttachmentAction>
                      </AttachmentActions>
                    </Attachment>
                  );
                })}
                {uploadingAttachments.map((item) => (
                  <Attachment
                    key={item.tempID}
                    state="uploading"
                    size="sm"
                    className="h-12 w-full border-0 bg-muted/35 px-2.5 dark:bg-white/[0.06] sm:w-[228px]"
                    aria-label={tComposer("uploadingAttachment", { name: item.fileName })}
                  >
                    <AttachmentMedia className="size-6 bg-transparent text-muted-foreground">
                      <LoaderCircle className="size-5 animate-spin" strokeWidth={1.8} />
                    </AttachmentMedia>
                    <AttachmentContent className="flex min-w-0 flex-1 flex-col justify-center px-0 py-0">
                      <AttachmentTitle className="text-[12px] leading-4 text-foreground/90" title={item.fileName}>
                        {item.fileName}
                      </AttachmentTitle>
                      <AttachmentDescription className="mt-1 text-[11px] leading-none">
                        Uploading · {formatBytes(item.sizeBytes)}
                      </AttachmentDescription>
                    </AttachmentContent>
                  </Attachment>
                ))}
              </AttachmentGroup>
              {stablePreviewAttachment ? (
                <FilePreviewDialog
                  file={stablePreviewAttachment}
                  open={previewAttachment !== null}
                  onOpenChange={closePreviewDialog}
                />
              ) : null}
            </div>
          ) : null}

          <ChatMentionMenuPortal
            activeIndex={mentionActiveIndex}
            menuID={mentionMenuID}
            menuLayout={mentionMenuLayout}
            menuRef={mentionMenuRef}
            menuReady={mentionMenuReady}
            open={showMentionMenu}
            sectionOffsets={mentionSectionOffsets}
            sections={mentionSections}
            t={tComposer}
            onSelect={selectMentionItem}
          />

          <InputGroupTextarea
            ref={textareaRef}
            value={draft}
            disabled={loading || uploading}
            readOnly={speechInput.active}
            placeholder={dropActive ? tChat("attachments.dropTitle") : speechInput.placeholder}
            rows={1}
            aria-controls={showMentionMenu ? mentionMenuID : undefined}
            aria-expanded={showMentionMenu ? true : undefined}
            style={{ fontFamily: "var(--font-chat)", fontWeight: "var(--font-chat-weight)" }}
            className={cn(
              "rounded-3xl min-h-12 overflow-y-auto px-5 text-[15px] leading-6 placeholder:text-muted-foreground placeholder:font-[inherit] placeholder:leading-[inherit]",
              showSelectedResources || hasComposerAttachments ? "pt-2" : "pt-4",
              inputHeightClassName,
              speechInput.active ? "placeholder:font-normal placeholder:text-muted-foreground" : "",
            )}
            onFocus={handleMentionFocus}
            onBlur={handleMentionBlur}
            onChange={(event) => handleMentionChange(event.target.value)}
            onClick={handleMentionSelectionChange}
            onKeyUp={handleMentionSelectionChange}
            onSelect={handleMentionSelectionChange}
            onScroll={onSourceScroll}
            onPaste={(event) => {
              const files = clipboardFilesFromPaste(event);
              const markdownPaste = resolveClipboardMarkdownPaste(event.clipboardData);
              if (markdownPaste) {
                event.preventDefault();
                const textarea = event.currentTarget;
                const formatted = formatClipboardMarkdownPaste(
                  textarea.value,
                  textarea.selectionStart,
                  textarea.selectionEnd,
                  markdownPaste,
                );
                handleMentionChange(formatted.value);
                window.requestAnimationFrame(() => {
                  textareaRef.current?.setSelectionRange(formatted.caretIndex, formatted.caretIndex);
                });
              }

              if (files.length > 0) {
                if (!event.clipboardData.getData("text/plain")) {
                  event.preventDefault();
                }
                void onUploadFiles(files);
              }
            }}
            onCompositionStart={() => {
              composingRef.current = true;
            }}
            onCompositionEnd={() => {
              composingRef.current = false;
            }}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing || composingRef.current || event.key === "Process" || event.keyCode === 229) {
                return;
              }
              const shouldSend = isSendShortcutEvent(sendShortcut, event);

              if (handleMentionKeyDown(event)) {
                return;
              }

              if (shouldSend) {
                event.preventDefault();
                if (canSend) {
                  void onSendMessage();
                } else if (hasSubmitContent && executionMode === "gateway") {
                  toast.error(tChat("submit.sendFailed"), {
                    description: gatewayStatus || tChat("submit.deviceOffline"),
                  });
                }
              }
            }}
          />

          <InputGroupAddon align="block-end" className="min-w-0 items-center justify-between gap-1 pt-2">
            <div className={cn("flex items-center gap-0.5 sm:gap-1", executionMode === "gateway" ? "min-w-0 flex-1" : "shrink-0")}>
              <DropdownMenu
                modal={false}
                open={toolsMenuOpen}
                onOpenChange={(open) => {
                  setToolsMenuOpen(open);
                  if (!open) {
                    setToolsMenuHovered(false);
                  }
                }}
              >
                <DropdownMenuTrigger asChild>
                  <InputGroupButton
                    id="chat-tools-menu-trigger"
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="size-7 rounded-md text-muted-foreground hover:text-foreground sm:size-8"
                    disabled={loading || uploading}
                    aria-label={tComposer("openTools")}
                    onMouseEnter={() => setToolsMenuHovered(true)}
                    onMouseLeave={() => setToolsMenuHovered(false)}
                  >
                    <PlusIcon
                      size={20}
                      strokeWidth={1.4}
                      animate={toolsMenuHovered || toolsMenuOpen ? "default" : undefined}
                    />
                  </InputGroupButton>
                </DropdownMenuTrigger>
                <DropdownMenuContent side="bottom" align="start" sideOffset={8} className="w-36">
                  <DropdownMenuItem
                    onMouseEnter={() => setHoveredTool("upload")}
                    onMouseLeave={() => setHoveredTool((prev) => (prev === "upload" ? null : prev))}
                    onSelect={(event) => {
                      event.preventDefault();
                      onSelectUploadTool();
                    }}
                  >
                    <LinkIcon size={12} strokeWidth={1.5} animate={hoveredTool === "upload" ? "default" : undefined} />
                    {tComposer("uploadFile")}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onMouseEnter={() => setHoveredTool("screenshot")}
                    onMouseLeave={() => setHoveredTool((prev) => (prev === "screenshot" ? null : prev))}
                    onSelect={(event) => {
                      event.preventDefault();
                      onSelectScreenshotTool();
                    }}
                  >
                    <Crop size={12} strokeWidth={1.5} animate={hoveredTool === "screenshot" ? "default" : undefined} />
                    {tComposer("screenshot")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>

              {executionMode === "gateway" && agentSettings ? (
                <ChatAgentSettings
                  settings={agentSettings}
                  autoReviewEnabled={agentAutoReviewEnabled}
                  disabled={agentSettingsDisabled || loading || uploading}
                  onChange={onAgentSettingsChange}
                />
              ) : null}

              {showMCPToolsButton ? (
                <ChatMCP
                  availableTools={availableTools}
                  selectedToolIDs={selectedToolIDs}
                  defaultToolIDs={defaultToolIDs}
                  maxSelectedTools={maxSelectedTools}
                  disabled={loading || uploading || toolsLoading}
                  onSelectedToolsChange={onSelectedToolsChange}
                  onDefaultToolsChange={onDefaultToolsChange}
                />
              ) : null}

              {showHTMLVisualPromptButton ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <InputGroupButton
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      className={cn(
                        "size-7 rounded-md text-muted-foreground hover:text-foreground sm:size-8",
                        htmlVisualPromptEnabled && "bg-primary/10 text-primary hover:bg-primary/10 hover:text-primary",
                      )}
                      disabled={loading || uploading}
                      aria-label={tComposer("htmlVisualPrompt")}
                      aria-pressed={htmlVisualPromptEnabled}
                      onClick={() => onHTMLVisualPromptChange(!htmlVisualPromptEnabled)}
                      onMouseEnter={() => setIsBlocksHovered(true)}
                      onMouseLeave={() => setIsBlocksHovered(false)}
                    >
                      <Blocks
                        size={20}
                        strokeWidth={1.4}
                        animate={htmlVisualPromptEnabled ? "default" : isBlocksHovered ? "default" : undefined}
                      />
                    </InputGroupButton>
                  </TooltipTrigger>
                  <TooltipContent side="top" className="max-w-72 text-xs leading-5">
                    {htmlVisualPromptEnabled
                      ? tComposer("htmlVisualPromptEnabled")
                      : tComposer("htmlVisualPromptDisabled")}
                  </TooltipContent>
                </Tooltip>
              ) : null}

              {hasDraftText ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <InputGroupButton
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      className={cn(
                        "size-7 rounded-md text-muted-foreground hover:text-foreground sm:size-8",
                        showMarkdownPreview && "bg-primary/10 text-primary hover:bg-primary/10 hover:text-primary",
                      )}
                      disabled={speechInput.active}
                      aria-label={showMarkdownPreview ? tComposer("hideMarkdownPreview") : tComposer("previewMarkdown")}
                      aria-pressed={showMarkdownPreview}
                      onClick={() => setMarkdownPreview((visible) => !visible)}
                    >
                      {showMarkdownPreview ? (
                        <EyeOff className="size-4" strokeWidth={1.6} />
                      ) : (
                        <Eye className="size-4" strokeWidth={1.6} />
                      )}
                    </InputGroupButton>
                  </TooltipTrigger>
                  <TooltipContent side="top" className="text-xs">
                    {showMarkdownPreview ? tComposer("hideMarkdownPreview") : tComposer("previewMarkdown")}
                  </TooltipContent>
                </Tooltip>
              ) : null}
            </div>

            <div className={cn("flex items-center justify-end gap-1 sm:gap-1.5", executionMode === "cloud" ? "min-w-0 flex-1 overflow-hidden" : "shrink-0")}>
              {composerModeIndicator && ComposerModeIcon ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span
                      className={cn(
                        "inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg px-2 text-[11px] font-medium transition-colors",
                        composerModeIndicator.tone === "warning"
                          ? "bg-destructive/10 text-destructive"
                          : "bg-muted/60 text-muted-foreground",
                      )}
                    >
                      <ComposerModeIcon className="size-3.5" strokeWidth={1.7} />
                      <span className="hidden sm:inline">{composerModeIndicator.label}</span>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent side="top" align="end" className="max-w-72 text-xs leading-5">
                    {composerModeIndicator.intro} {composerModeIndicator.description}
                  </TooltipContent>
                </Tooltip>
              ) : null}
              <ChatModelPicker
                modelOptions={executionMode === "gateway" ? agentPickerModels : modelOptions}
                selectedPlatformModelName={executionMode === "gateway" ? agentSettings?.model ?? "" : selectedPlatformModelName}
                reasoning={executionMode === "gateway"
                  ? selectedAgentModel && agentSettings ? {
                      value: agentSettings.reasoningEffort,
                      options: selectedAgentModel.supportedReasoningEfforts,
                      disabled: agentSettingsDisabled,
                      onChange: changePickerReasoning,
                    } : undefined
                  : cloudReasoningSetting ? {
                      value: cloudReasoningSetting.value,
                      options: cloudReasoningSetting.options,
                      disabled: cloudReasoningSetting.disabled,
                      onChange: changePickerReasoning,
                    } : undefined}
                loading={executionMode === "gateway" ? agentSettingsLoading : modelLoading}
                disabled={executionMode === "gateway"
                  ? agentSettingsDisabled || loading || uploading || !agentSettings || Boolean(agentSettingsError)
                  : modelDisabled}
                onModelCatalogRefresh={executionMode === "cloud" ? onModelCatalogRefresh : undefined}
                onModelChange={changePickerModel}
              />

              <InputGroupButton
                type="button"
                variant="ghost"
                size="icon-sm"
                className="size-7 rounded-md text-muted-foreground hover:text-foreground sm:size-8"
                disabled={loading || uploading || (!hasSubmitContent && !chatRouteReady) || (!sending && !hasSubmitContent && !speechInput.supported)}
                onClick={handleSendButtonClick}
                onMouseEnter={() => setIsVoiceHovered(true)}
                onMouseLeave={() => setIsVoiceHovered(false)}
                aria-label={hasSubmitContent ? (sending ? tComposer("queueMessage") : tChat("send")) : sending ? tComposer("pauseGeneration") : speechInput.active ? tComposer("cancelVoiceInput") : tComposer("voiceInput")}
                title={hasSubmitContent ? (sending ? tComposer("queueMessage") : tChat("send")) : sending ? tComposer("pauseGeneration") : speechInput.supported ? (speechInput.active ? tComposer("cancelVoiceInput") : tComposer("voiceInput")) : tComposer("voiceUnsupported")}
              >
                {hasSubmitContent ? (
                  <Send
                    size={20}
                    strokeWidth={1.4}
                    animate={isVoiceHovered ? "default" : undefined}
                  />
                ) : sending ? (
                  <Pause
                    size={20}
                    strokeWidth={1.4}
                    animate="default-loop"
                  />
                ) : speechInput.status === "starting" ? (
                  <LoaderCircle className="size-5 animate-spin" strokeWidth={1.6} />
                ) : speechInput.active ? (
                  <AudioLines
                    size={20}
                    strokeWidth={1.4}
                    animate="default"
                  />
                ) : (
                  <AudioLines
                    size={20}
                    strokeWidth={1.4}
                    animate={isVoiceHovered ? "default" : undefined}
                  />
                )}
              </InputGroupButton>
            </div>
          </InputGroupAddon>
        </div>
      </InputGroup>

    </div>
  );
}

export const ChatInput = React.memo(ChatInputComponent);
ChatInput.displayName = "ChatInput";
