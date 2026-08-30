"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";

import { SidebarConversationsProvider } from "@/entities/conversation";
import { AppSidebar } from "@/features/layouts/components/navigation/app-sidebar";
import { MobileHeader } from "@/features/layouts/components/sections/mobile-header";
import { LayoutConversationNavigationProvider } from "@/features/layouts/context/layout-conversation-navigation-context";
import { ChatSessionProvider, useChatSession } from "@/features/chat";
import { DeviceProvider, ExecutionModeSwitch, useDevices } from "@/features/devices";
import { AppearancePreferencesSync } from "@/features/settings";
import {
  SidebarInset,
  SidebarProvider,
  useSidebarActions,
  useSidebarIsMobile,
  useSidebarMobileOpen,
} from "@/components/ui/sidebar";
import { UserLocaleSync } from "@/i18n/user-locale-sync";

const AnnouncementDialogHost = dynamic(
  () => import("@/features/announcements").then((mod) => mod.AnnouncementDialogHost),
  { ssr: false },
);

function ProjectLayoutShell({
  children,
}: {
  children: React.ReactNode;
}) {
  const tRecent = useTranslations("recent");
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const isMobile = useSidebarIsMobile();
  const openMobile = useSidebarMobileOpen();
  const { setOpenMobile } = useSidebarActions();
  const { executionMode, requestNewConversation, setExecutionMode } = useChatSession();
  const { defaultDevice, loading: devicesLoading } = useDevices();
  const showModeSwitch = pathname === "/chat";
  const routeKey = `${pathname}?${searchParams.toString()}`;
  const previousRouteKeyRef = React.useRef(routeKey);

  React.useEffect(() => {
    if (previousRouteKeyRef.current === routeKey) {
      return;
    }

    previousRouteKeyRef.current = routeKey;
    if (isMobile && openMobile) {
      setOpenMobile(false);
    }
  }, [isMobile, openMobile, routeKey, setOpenMobile]);

  React.useEffect(() => {
    if (!devicesLoading && !defaultDevice && executionMode === "gateway" && !searchParams.get("conversation_id")) {
      setExecutionMode("cloud");
    }
  }, [defaultDevice, devicesLoading, executionMode, searchParams, setExecutionMode]);

  const handleCreateConversation = React.useCallback(() => {
    requestNewConversation({ projectID: "" });
    if (pathname === "/chat") {
      window.history.pushState(null, "", "/chat");
      return;
    }
    router.push("/chat");
  }, [pathname, requestNewConversation, router]);

  return (
    <>
      <SidebarConversationsProvider
        key={`${executionMode}:${defaultDevice?.deviceId ?? ""}`}
        bulkPendingTitle={tRecent("dialogs.bulk.pending")}
        executionDeviceID={executionMode === "gateway" ? defaultDevice?.deviceId ?? "" : ""}
        executionType={executionMode}
        newConversationTitle={tRecent("newChat")}
      >
        <AppSidebar onCreateConversation={handleCreateConversation} />
        <SidebarInset>
          <MobileHeader onCreateConversation={handleCreateConversation} showModeSwitch={showModeSwitch} />
          {showModeSwitch ? (
            <div className="pointer-events-none absolute inset-x-0 top-2 z-30 hidden justify-center md:flex">
              <ExecutionModeSwitch />
            </div>
          ) : null}
          <div className="flex h-full min-h-0 flex-1 flex-col gap-4 overflow-hidden pb-2 md:p-4 md:pt-0">
            {children}
          </div>
        </SidebarInset>
      </SidebarConversationsProvider>
    </>
  );
}

export function ProjectLayout({
  children,
  defaultSidebarOpen = true,
}: {
  children: React.ReactNode;
  defaultSidebarOpen?: boolean;
}) {
  return (
    <>
      <UserLocaleSync />
      <AppearancePreferencesSync />
      <AnnouncementDialogHost />
      <SidebarProvider className="h-svh overflow-hidden" defaultOpen={defaultSidebarOpen}>
        <LayoutConversationNavigationProvider>
          <DeviceProvider>
            <ChatSessionProvider>
              <ProjectLayoutShell>{children}</ProjectLayoutShell>
            </ChatSessionProvider>
          </DeviceProvider>
        </LayoutConversationNavigationProvider>
      </SidebarProvider>
    </>
  );
}
