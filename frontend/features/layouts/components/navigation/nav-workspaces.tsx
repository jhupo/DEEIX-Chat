"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChevronDown, Folder, LoaderCircle, Plus } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Collapsible } from "@/components/ui/collapsible";
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@/components/ui/sidebar";
import { useSidebarConversations } from "@/entities/conversation";
import { useChatSession } from "@/features/chat";
import { useDevices } from "@/features/devices";
import { useLayoutActiveConversation } from "@/features/layouts/hooks/use-layout-active-conversation";
import { useSidebarConversationNavigation } from "@/features/layouts/hooks/use-sidebar-conversation-navigation";
import {
  getAgentWorkspaceResource,
  refreshAgentWorkspaceResource,
} from "@/shared/api/agent-gateway";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { CollapsibleMotionContent } from "@/shared/components/collapsible-motion-content";
import { cn } from "@/lib/utils";

const SESSION_REFRESH_TIMEOUT_MS = 35_000;
const SESSION_REFRESH_POLL_MS = 750;

function wait(duration: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, duration));
}

export function NavWorkspaces() {
  const t = useTranslations("recent");
  const router = useRouter();
  const { executionMode, requestNewConversation } = useChatSession();
  const { defaultDevice, defaultWorkspace, selectDefaultWorkspace, workspaces } = useDevices();
  const { items, reload } = useSidebarConversations();
  const activeConversationID = useLayoutActiveConversation();
  const onNavigate = useSidebarConversationNavigation();
  const [openWorkspaceIDs, setOpenWorkspaceIDs] = React.useState<Set<string>>(() => new Set());
  const [refreshing, setRefreshing] = React.useState(false);
  const refreshScopeRef = React.useRef("");

  React.useEffect(() => {
    if (defaultWorkspace) {
      setOpenWorkspaceIDs((current) => new Set(current).add(defaultWorkspace.workspaceId));
    }
  }, [defaultWorkspace]);

  React.useEffect(() => {
    const deviceID = defaultDevice?.deviceId ?? "";
    const scope = `${deviceID}:${workspaces.map((item) => item.workspaceId).join(",")}`;
    if (executionMode !== "gateway" || !deviceID || workspaces.length === 0 || refreshScopeRef.current === scope) {
      return;
    }
    refreshScopeRef.current = scope;
    let cancelled = false;

    async function refreshSessions() {
      const token = await resolveAccessToken();
      if (!token || cancelled) return;
      setRefreshing(true);
      try {
        await Promise.allSettled(workspaces.map(async (workspace) => {
          const startedAt = Date.now();
          await refreshAgentWorkspaceResource(token, deviceID, workspace.workspaceId, "sessions");
          while (!cancelled && Date.now() - startedAt < SESSION_REFRESH_TIMEOUT_MS) {
            try {
              const snapshot = await getAgentWorkspaceResource(token, deviceID, workspace.workspaceId, "sessions");
              if (new Date(snapshot.refreshedAt).getTime() >= startedAt - 1000) return;
            } catch {
              // The first refresh creates the snapshot, so a short missing interval is expected.
            }
            await wait(SESSION_REFRESH_POLL_MS);
          }
        }));
        if (!cancelled) await reload();
      } catch {
        // Keep the last projected list; reconnecting or changing modes retries the refresh.
      } finally {
        if (!cancelled) setRefreshing(false);
      }
    }

    void refreshSessions();
    return () => {
      cancelled = true;
    };
  }, [defaultDevice?.deviceId, executionMode, reload, workspaces]);

  if (executionMode !== "gateway") return null;

  return (
    <SidebarGroup className="px-2 py-2 group-data-[collapsible=icon]:hidden">
      <SidebarGroupLabel className="flex px-2 text-xs">
        <span>{t("projects.title")}</span>
        {refreshing ? <LoaderCircle aria-hidden className="ml-auto size-3 animate-spin" /> : null}
      </SidebarGroupLabel>
      <div className="space-y-0.5">
        {workspaces.map((workspace) => {
          const open = openWorkspaceIDs.has(workspace.workspaceId);
          const conversations = items.filter((item) => item.executionWorkspaceID === workspace.workspaceId);
          return (
            <Collapsible key={workspace.workspaceId} open={open}>
              <div className="group/workspace flex h-8 items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  className="h-8 min-w-0 flex-1 justify-start gap-2 px-2 text-sm font-normal hover:bg-sidebar-accent"
                  onClick={() => setOpenWorkspaceIDs((current) => {
                    const next = new Set(current);
                    if (open) next.delete(workspace.workspaceId);
                    else next.add(workspace.workspaceId);
                    return next;
                  })}
                >
                  <Folder aria-hidden className="size-4 shrink-0 stroke-1.5" />
                  <span className="min-w-0 flex-1 truncate text-left">{workspace.name}</span>
                  <ChevronDown aria-hidden className={cn("size-3 shrink-0 transition-transform", !open && "-rotate-90")} />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-7 shrink-0 opacity-0 group-hover/workspace:opacity-100 focus-visible:opacity-100"
                  aria-label={t("newChat")}
                  onClick={() => {
                    selectDefaultWorkspace(workspace.workspaceId);
                    requestNewConversation({ workspaceID: workspace.workspaceId });
                    router.push("/chat");
                  }}
                >
                  <Plus aria-hidden className="size-3.5" />
                </Button>
              </div>
              <CollapsibleMotionContent open={open}>
                <SidebarMenuSub className="mr-0 pr-0">
                  {conversations.map((item) => {
                    const href = `/chat?conversation_id=${encodeURIComponent(item.publicID)}`;
                    return (
                      <SidebarMenuSubItem key={item.publicID}>
                        <SidebarMenuSubButton asChild isActive={activeConversationID === item.publicID}>
                          <Link href={href} prefetch={false} onClick={(event) => onNavigate(href, event)}>
                            <span>{item.title || t("untitled")}</span>
                          </Link>
                        </SidebarMenuSubButton>
                      </SidebarMenuSubItem>
                    );
                  })}
                </SidebarMenuSub>
              </CollapsibleMotionContent>
            </Collapsible>
          );
        })}
      </div>
    </SidebarGroup>
  );
}
