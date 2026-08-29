"use client";

import * as React from "react";

import { useChatMessageSubmit } from "@/features/chat/hooks/use-chat-message-submit";
import { useChatStreamBuffer } from "@/features/chat/hooks/use-chat-stream-buffer";
import type { ChatAreaMessage } from "@/features/chat/types/messages";
import type {
  ChatModelOption,
  PendingAttachment,
  PendingExchangeMap,
} from "@/features/chat/types/chat-runtime";
import type {
  ConversationDTO,
  ConversationInputResourceDTO,
  ConversationOptions,
  MessageDTO,
} from "@/shared/api/conversation.types";
import type { SkillSummaryDTO } from "@/shared/api/skills.types";

export function useChatSubmitStream({
  conversationID,
  conversationScopeKey,
  executionMode,
  activeConversation,
  selectedPlatformModelName,
  selectedKeyBindingID,
  modelOptions,
  selectedToolIDs,
  selectedSkills,
  selectedKnowledgeBaseIDs,
  selectedInputResources,
  htmlVisualPromptEnabled,
  options,
  draft,
  attachments,
  maxFilesPerMessage,
  uploading,
  restoreDraftOnFailure,
  autoGenerateLabels,
  prependNewConversation,
  onConversationCreated,
  touchByPublicID,
  reload,
  replaceMessage,
  setDraft,
  setAttachments,
  releaseAttachments,
  getPendingExchanges,
  pendingExchanges,
  setPendingExchanges,
  setBranchSelections,
  showConversationLayout,
  setShowConversationLayout,
  visibleMessageCount,
  currentLeafMessage,
  visibleMessages,
  combinedMessages,
  serverMessagePublicIDs,
  activeGenerationRunsRef,
  failedGenerationRunsRef,
  generationSeqByRunRef,
  resumeGenerationActive,
}: {
  conversationID: string | null;
  conversationScopeKey: string;
  executionMode: "cloud" | "gateway";
  activeConversation: ConversationDTO | null;
  selectedPlatformModelName: string;
  selectedKeyBindingID: string;
  modelOptions: ChatModelOption[];
  selectedToolIDs: number[];
  selectedSkills: SkillSummaryDTO[];
  selectedKnowledgeBaseIDs: string[];
  selectedInputResources: ConversationInputResourceDTO[];
  htmlVisualPromptEnabled: boolean;
  options: ConversationOptions;
  draft: string;
  attachments: PendingAttachment[];
  maxFilesPerMessage: number;
  uploading: boolean;
  restoreDraftOnFailure: boolean;
  autoGenerateLabels: boolean;
  prependNewConversation: (platformModelName: string) => Promise<ConversationDTO | null | undefined>;
  onConversationCreated?: (conversationPublicID: string) => void;
  touchByPublicID: (publicID: string, patch: Partial<ConversationDTO>) => void;
  reload: () => void;
  replaceMessage: (message: MessageDTO) => void;
  setDraft: React.Dispatch<React.SetStateAction<string>>;
  setAttachments: React.Dispatch<React.SetStateAction<PendingAttachment[]>>;
  releaseAttachments: (items: PendingAttachment[]) => void;
  getPendingExchanges: () => PendingExchangeMap;
  pendingExchanges: PendingExchangeMap;
  setPendingExchanges: React.Dispatch<React.SetStateAction<PendingExchangeMap>>;
  setBranchSelections: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  showConversationLayout: boolean;
  setShowConversationLayout: React.Dispatch<React.SetStateAction<boolean>>;
  visibleMessageCount: number;
  currentLeafMessage: ChatAreaMessage | null;
  visibleMessages: ChatAreaMessage[];
  combinedMessages: ChatAreaMessage[];
  serverMessagePublicIDs: Set<string>;
  activeGenerationRunsRef?: React.RefObject<Set<string>>;
  failedGenerationRunsRef?: React.RefObject<Set<string>>;
  generationSeqByRunRef?: React.MutableRefObject<Record<string, number>>;
  resumeGenerationActive?: boolean;
}) {
  const streamBuffer = useChatStreamBuffer({
    setPendingExchanges,
  });

  const messageSubmit = useChatMessageSubmit({
    conversationID,
    conversationScopeKey,
    executionMode,
    activeConversation,
    selectedPlatformModelName,
    selectedKeyBindingID,
    modelOptions,
    selectedToolIDs,
    selectedSkills,
    selectedKnowledgeBaseIDs,
    selectedInputResources,
    htmlVisualPromptEnabled,
    options,
    draft,
    attachments,
    maxFilesPerMessage,
    uploading,
    restoreDraftOnFailure,
    autoGenerateLabels,
    prependNewConversation,
    onConversationCreated,
    touchByPublicID,
    reload,
    replaceMessage,
    setDraft,
    setAttachments,
    releaseAttachments,
    getPendingExchanges,
    pendingExchanges,
    setPendingExchanges,
    setBranchSelections,
    showConversationLayout,
    setShowConversationLayout,
    visibleMessageCount,
    currentLeafMessage,
    visibleMessages,
    combinedMessages,
    serverMessagePublicIDs,
    enqueueStreamText: streamBuffer.enqueueStreamText,
    flushStreamTextNow: streamBuffer.flushStreamTextNow,
    resetStreamBuffer: streamBuffer.resetStreamBuffer,
    startStream: streamBuffer.startStream,
    activeGenerationRunsRef,
    failedGenerationRunsRef,
    generationSeqByRunRef,
    resumeGenerationActive,
  });

  return messageSubmit;
}
