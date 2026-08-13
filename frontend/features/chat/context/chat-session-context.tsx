"use client";

import * as React from "react";

type ChatSessionContextValue = {
  newConversationRevision: number;
  newConversationProjectID: string;
  newConversationWorkspaceID: string;
  requestNewConversation: (options?: { projectID?: string; workspaceID?: string }) => void;
  executionMode: "cloud" | "gateway";
  setExecutionMode: (mode: "cloud" | "gateway") => void;
};

const ChatSessionContext = React.createContext<ChatSessionContextValue | null>(null);

export function ChatSessionProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = React.useState({ revision: 0, projectID: "", workspaceID: "" });
  const [executionMode, setExecutionMode] = React.useState<"cloud" | "gateway">("cloud");
  const requestNewConversation = React.useCallback((options?: { projectID?: string; workspaceID?: string }) => {
    setState((prev) => ({
      revision: prev.revision + 1,
      projectID: options?.projectID?.trim() ?? "",
      workspaceID: options?.workspaceID?.trim() ?? "",
    }));
  }, []);

  const value = React.useMemo(
    () => ({
      newConversationRevision: state.revision,
      newConversationProjectID: state.projectID,
      newConversationWorkspaceID: state.workspaceID,
      requestNewConversation,
      executionMode,
      setExecutionMode,
    }),
    [executionMode, requestNewConversation, state.projectID, state.revision, state.workspaceID],
  );

  return <ChatSessionContext.Provider value={value}>{children}</ChatSessionContext.Provider>;
}

export function useChatSession() {
  const context = React.useContext(ChatSessionContext);
  if (!context) {
    throw new Error("useChatSession must be used within ChatSessionProvider");
  }
  return context;
}
