"use client";

import * as React from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";

import { useChatSession } from "@/features/chat";
import { useDevices } from "@/features/devices/device-context";
import { cn } from "@/lib/utils";

export function ExecutionModeSwitch() {
  const t = useTranslations("common.navigation");
  const { executionMode, setExecutionMode, requestNewConversation } = useChatSession();
  const { defaultDevice, defaultWorkspace, loading } = useDevices();
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const workAvailable = Boolean(defaultDevice && defaultWorkspace);

  React.useEffect(() => {
    if (!loading && !workAvailable && executionMode === "gateway" && !searchParams.get("conversation_id")) {
      setExecutionMode("cloud");
    }
  }, [executionMode, loading, searchParams, setExecutionMode, workAvailable]);

  const selectMode = React.useCallback((mode: "cloud" | "gateway") => {
    if (mode === executionMode || (mode === "gateway" && !workAvailable)) return;
    setExecutionMode(mode);
    requestNewConversation({ projectID: "", workspaceID: "" });
    if (pathname === "/chat") window.history.pushState(null, "", "/chat");
    else router.push("/chat");
  }, [executionMode, pathname, requestNewConversation, router, setExecutionMode, workAvailable]);

  return (
    <div className="pointer-events-auto inline-grid h-7 w-36 grid-cols-2 rounded-full bg-muted/80 p-0.5 shadow-sm backdrop-blur supports-[backdrop-filter]:bg-muted/65">
      <button
        type="button"
        className={cn(
          "rounded-full px-2 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          executionMode === "cloud" ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground",
        )}
        onClick={() => selectMode("cloud")}
      >
        {t("chatMode")}
      </button>
      <button
        type="button"
        className={cn(
          "rounded-full px-2 text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-40",
          executionMode === "gateway" ? "bg-background font-medium shadow-sm" : "text-muted-foreground hover:text-foreground",
        )}
        disabled={!workAvailable}
        onClick={() => selectMode("gateway")}
      >
        {t("workMode")}
      </button>
    </div>
  );
}
