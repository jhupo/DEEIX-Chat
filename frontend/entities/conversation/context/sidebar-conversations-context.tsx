"use client";

import * as React from "react";

import { useSidebarConversationsController } from "@/entities/conversation/hooks/use-sidebar-conversations";
import type { SidebarConversationsControllerValue } from "@/entities/conversation/types/sidebar-conversations";

const SidebarConversationsContext = React.createContext<SidebarConversationsControllerValue | null>(null);

export function SidebarConversationsProvider({
  bulkPendingTitle,
  children,
  executionDeviceID,
  executionType,
  newConversationTitle,
}: {
  bulkPendingTitle: string;
  children: React.ReactNode;
  executionDeviceID: string;
  executionType: "cloud" | "gateway";
  newConversationTitle: string;
}) {
  const value = useSidebarConversationsController({ bulkPendingTitle, executionDeviceID, executionType, newConversationTitle });
  return <SidebarConversationsContext.Provider value={value}>{children}</SidebarConversationsContext.Provider>;
}

export function useSidebarConversations() {
  const context = React.useContext(SidebarConversationsContext);
  if (!context) {
    throw new Error("useSidebarConversations must be used within SidebarConversationsProvider");
  }
  return context;
}
