"use client";

import * as React from "react";

import { useChatRunState } from "@/features/chat/hooks/use-chat-run-state";
import type { ConversationRunStore } from "@/features/chat/model/conversation-run-store";
import { readSessionID } from "@/shared/auth/session";

function executionModeStorageKey() {
  return `deeix.execution_mode:${readSessionID()}`;
}

function readStoredExecutionMode(storageKey: string): "cloud" | "gateway" {
  try {
    const stored = window.localStorage.getItem(storageKey);
    if (stored === "gateway") {
      return "gateway";
    }
  } catch {
    // Storage availability must not block the application shell.
  }
  return "cloud";
}

type ChatSessionContextValue = {
  newConversationRevision: number;
  newConversationProjectID: string;
  detachConversationRun: (runID: string) => void;
  finishConversationRun: (runID: string) => void;
  registerConversationRun: (runID: string, conversationPublicID: string) => void;
  requestNewConversation: (options?: { projectID?: string }) => void;
  executionMode: "cloud" | "gateway";
  setExecutionMode: (mode: "cloud" | "gateway") => void;
};

const ChatSessionContext = React.createContext<ChatSessionContextValue | null>(null);
const ConversationRunStoreContext = React.createContext<ConversationRunStore | null>(null);

export function ChatSessionProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = React.useState({ revision: 0, projectID: "" });
  const {
    detachConversationRun,
    finishConversationRun,
    registerConversationRun,
    store,
  } = useChatRunState();
  const [storageKey] = React.useState(executionModeStorageKey);
  const [executionMode, setExecutionMode] = React.useState<"cloud" | "gateway">(() => readStoredExecutionMode(storageKey));
  const updateExecutionMode = React.useCallback((mode: "cloud" | "gateway") => {
    setExecutionMode(mode);
    try {
      window.localStorage.setItem(storageKey, mode);
    } catch {
      // The in-memory mode remains authoritative when storage is unavailable.
    }
  }, [storageKey]);
  const requestNewConversation = React.useCallback((options?: { projectID?: string }) => {
    setState((prev) => ({
      revision: prev.revision + 1,
      projectID: options?.projectID?.trim() ?? "",
    }));
  }, []);

  const value = React.useMemo(
    () => ({
      newConversationRevision: state.revision,
      newConversationProjectID: state.projectID,
      detachConversationRun,
      finishConversationRun,
      registerConversationRun,
      requestNewConversation,
      executionMode,
      setExecutionMode: updateExecutionMode,
    }),
    [
      detachConversationRun,
      executionMode,
      finishConversationRun,
      registerConversationRun,
      requestNewConversation,
      state.projectID,
      state.revision,
      updateExecutionMode,
    ],
  );

  return (
    <ConversationRunStoreContext.Provider value={store}>
      <ChatSessionContext.Provider value={value}>{children}</ChatSessionContext.Provider>
    </ConversationRunStoreContext.Provider>
  );
}

export function useChatSession() {
  const context = React.useContext(ChatSessionContext);
  if (!context) {
    throw new Error("useChatSession must be used within ChatSessionProvider");
  }
  return context;
}

export function useConversationRunning(conversationPublicID: string): boolean {
  const store = React.useContext(ConversationRunStoreContext);
  if (!store) {
    throw new Error("useConversationRunning must be used within ChatSessionProvider");
  }
  const conversationID = conversationPublicID.trim();
  const subscribe = React.useCallback(
    (listener: () => void) => store.subscribe(conversationID, listener),
    [conversationID, store],
  );
  const getSnapshot = React.useCallback(
    () => store.isConversationRunning(conversationID),
    [conversationID, store],
  );
  return React.useSyncExternalStore(subscribe, getSnapshot, () => false);
}
