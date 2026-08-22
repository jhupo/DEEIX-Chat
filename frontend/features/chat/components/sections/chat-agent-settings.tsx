"use client";

import { ShieldAlert, ShieldCheck, ShieldOff } from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";

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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { InputGroupButton } from "@/components/ui/input-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { AgentTurnSettings } from "@/shared/api/agent-gateway";

const FULL_ACCESS_CONFIRMATION_KEY = "deeix-chat:agent-full-access-confirmed:v1";

type AgentApprovalMode = "request" | "auto" | "full";

const REQUEST_APPROVAL_SETTINGS: Pick<
  AgentTurnSettings,
  "approvalPolicy" | "approvalsReviewer" | "sandboxPolicy"
> = {
  approvalPolicy: "on-request",
  approvalsReviewer: "user",
  sandboxPolicy: "workspace-write",
};

const AUTO_REVIEW_SETTINGS: Pick<
  AgentTurnSettings,
  "approvalPolicy" | "approvalsReviewer" | "sandboxPolicy"
> = {
  approvalPolicy: "on-request",
  approvalsReviewer: "auto_review",
  sandboxPolicy: "workspace-write",
};

const FULL_ACCESS_SETTINGS: Pick<
  AgentTurnSettings,
  "approvalPolicy" | "approvalsReviewer" | "sandboxPolicy"
> = {
  approvalPolicy: "never",
  approvalsReviewer: "user",
  sandboxPolicy: "danger-full-access",
};

function approvalMode(settings: AgentTurnSettings): AgentApprovalMode {
  if (settings.sandboxPolicy === "danger-full-access") {
    return "full";
  }
  return settings.approvalsReviewer === "auto_review" ? "auto" : "request";
}

function fullAccessConfirmed(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  try {
    return window.localStorage.getItem(FULL_ACCESS_CONFIRMATION_KEY) === "true";
  } catch {
    return false;
  }
}

function rememberFullAccessConfirmation(): void {
  try {
    window.localStorage.setItem(FULL_ACCESS_CONFIRMATION_KEY, "true");
  } catch {
    // The confirmation still applies to the current selection when storage is unavailable.
  }
}

type ChatAgentSettingsProps = {
  settings: AgentTurnSettings | null;
  autoReviewEnabled: boolean;
  loading: boolean;
  disabled: boolean;
  error?: string;
  onChange: (settings: AgentTurnSettings) => void;
};

export function ChatAgentSettings({
  settings,
  autoReviewEnabled,
  loading,
  disabled,
  error,
  onChange,
}: ChatAgentSettingsProps) {
  const t = useTranslations("chat.agent.settings");
  const [fullAccessDialogOpen, setFullAccessDialogOpen] = React.useState(false);
  const selectedMode = settings ? approvalMode(settings) : "request";
  const unavailable = disabled || loading || !settings || Boolean(error);
  const ModeIcon = selectedMode === "full" ? ShieldOff : selectedMode === "auto" ? ShieldCheck : ShieldAlert;

  const selectMode = React.useCallback((value: string) => {
    if (!settings || !["request", "auto", "full"].includes(value)) {
      return;
    }
    const mode = value as AgentApprovalMode;
    if (mode === "auto" && !autoReviewEnabled) {
      return;
    }
    if (mode === "full" && !fullAccessConfirmed()) {
      setFullAccessDialogOpen(true);
      return;
    }
    const modeSettings = mode === "full"
      ? FULL_ACCESS_SETTINGS
      : mode === "auto" ? AUTO_REVIEW_SETTINGS : REQUEST_APPROVAL_SETTINGS;
    onChange({ ...settings, ...modeSettings });
  }, [autoReviewEnabled, onChange, settings]);

  const confirmFullAccess = React.useCallback(() => {
    if (!settings) {
      return;
    }
    rememberFullAccessConfirmation();
    onChange({ ...settings, ...FULL_ACCESS_SETTINGS });
  }, [onChange, settings]);

  if (loading) {
    return (
      <InputGroupButton
        type="button"
        variant="ghost"
        size="icon-sm"
        className="size-7 rounded-md text-muted-foreground sm:size-8"
        disabled
        aria-label={t("loading")}
      >
        <ShieldAlert className="size-4 animate-pulse" strokeWidth={1.7} />
      </InputGroupButton>
    );
  }

  if (!settings || error) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex">
            <InputGroupButton
              type="button"
              variant="ghost"
              size="icon-sm"
              className="size-7 rounded-md text-destructive sm:size-8"
              disabled
              aria-label={error || t("unavailable")}
            >
              <ShieldAlert className="size-4" strokeWidth={1.7} />
            </InputGroupButton>
          </span>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-72 text-xs">
          {error || t("unavailable")}
        </TooltipContent>
      </Tooltip>
    );
  }

  return (
    <>
      <DropdownMenu>
          <Tooltip>
            <TooltipTrigger asChild>
              <DropdownMenuTrigger asChild>
                <InputGroupButton
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 rounded-md px-1.5 text-xs text-muted-foreground sm:h-8 sm:px-2"
                  disabled={unavailable}
                  aria-label={t("approvalMode")}
                >
                  <ModeIcon className="size-3.5" strokeWidth={1.7} />
                  <span className="hidden sm:inline">{t(`modes.${selectedMode}`)}</span>
                </InputGroupButton>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent side="top" className="text-xs">{t("approvalMode")}</TooltipContent>
          </Tooltip>
          <DropdownMenuContent align="end" side="bottom" sideOffset={8}>
            <DropdownMenuLabel>{t("approvalMode")}</DropdownMenuLabel>
            <DropdownMenuRadioGroup value={selectedMode} onValueChange={selectMode}>
              <DropdownMenuRadioItem value="request">{t("modes.request")}</DropdownMenuRadioItem>
              {autoReviewEnabled ? (
                <DropdownMenuRadioItem value="auto">{t("modes.auto")}</DropdownMenuRadioItem>
              ) : null}
              <DropdownMenuRadioItem value="full" className="text-destructive focus:text-destructive">
                {t("modes.full")}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog open={fullAccessDialogOpen} onOpenChange={setFullAccessDialogOpen}>
        <AlertDialogContent size="compact">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-destructive">{t("fullAccessTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("fullAccessDescription")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("cancel")}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirmFullAccess}>
              {t("confirmFullAccess")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
