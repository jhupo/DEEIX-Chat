"use client";

import { Bot, Brain, ChevronDown, ShieldAlert, ShieldCheck, ShieldOff } from "lucide-react";
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
import type { AgentModelDTO, AgentTurnSettings } from "@/shared/api/agent-gateway";

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
  models: AgentModelDTO[];
  settings: AgentTurnSettings | null;
  autoReviewEnabled: boolean;
  loading: boolean;
  disabled: boolean;
  error?: string;
  onChange: (settings: AgentTurnSettings) => void;
};

export function ChatAgentSettings({
  models,
  settings,
  autoReviewEnabled,
  loading,
  disabled,
  error,
  onChange,
}: ChatAgentSettingsProps) {
  const t = useTranslations("chat.agent.settings");
  const [fullAccessDialogOpen, setFullAccessDialogOpen] = React.useState(false);
  const selectedModel = settings ? models.find((model) => model.id === settings.model) ?? null : null;
  const selectedMode = settings ? approvalMode(settings) : "request";
  const unavailable = disabled || loading || !settings || models.length === 0 || Boolean(error);
  const ModeIcon = selectedMode === "full" ? ShieldOff : selectedMode === "auto" ? ShieldCheck : ShieldAlert;

  const selectModel = React.useCallback((modelID: string) => {
    if (!settings) {
      return;
    }
    const model = models.find((item) => item.id === modelID);
    if (!model) {
      return;
    }
    onChange({
      ...settings,
      model: model.id,
      reasoningEffort: model.supportedReasoningEfforts.includes(settings.reasoningEffort)
        ? settings.reasoningEffort
        : model.defaultReasoningEffort,
    });
  }, [models, onChange, settings]);

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
        <Bot className="size-4 animate-pulse" strokeWidth={1.6} />
      </InputGroupButton>
    );
  }

  if (!settings || models.length === 0 || error) {
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
      <div className="flex min-w-0 shrink items-center gap-0.5 sm:gap-1">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <InputGroupButton
              type="button"
              variant="ghost"
              size="sm"
              className="min-w-0 max-w-40 flex-1 shrink overflow-hidden rounded-lg px-1.5 text-xs sm:max-w-52 sm:px-2"
              disabled={unavailable}
              aria-label={t("model")}
            >
              <Bot className="size-3.5 shrink-0 text-muted-foreground" strokeWidth={1.7} />
              <span className="min-w-0 truncate">{selectedModel?.displayName ?? settings.model}</span>
              <ChevronDown className="size-3 shrink-0 text-muted-foreground" strokeWidth={1.7} />
            </InputGroupButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="bottom" sideOffset={8} className="w-64 max-w-[calc(100vw-2rem)]">
            <DropdownMenuLabel>{t("model")}</DropdownMenuLabel>
            <DropdownMenuRadioGroup value={settings.model} onValueChange={selectModel}>
              {models.map((model) => (
                <DropdownMenuRadioItem key={model.id} value={model.id} className="items-start whitespace-normal">
                  <span className="min-w-0">
                    <span className="block font-medium [overflow-wrap:anywhere]">{model.displayName}</span>
                    {model.description ? (
                      <span className="mt-0.5 line-clamp-2 block text-[11px] leading-4 text-muted-foreground [overflow-wrap:anywhere]">
                        {model.description}
                      </span>
                    ) : null}
                  </span>
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>

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
                  aria-label={t("reasoning")}
                >
                  <Brain className="size-3.5" strokeWidth={1.7} />
                  <span className="hidden sm:inline">{t(`efforts.${settings.reasoningEffort}`)}</span>
                </InputGroupButton>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent side="top" className="text-xs">{t("reasoning")}</TooltipContent>
          </Tooltip>
          <DropdownMenuContent align="end" side="bottom" sideOffset={8}>
            <DropdownMenuLabel>{t("reasoning")}</DropdownMenuLabel>
            <DropdownMenuRadioGroup
              value={settings.reasoningEffort}
              onValueChange={(value) => {
                if (selectedModel?.supportedReasoningEfforts.includes(value as AgentTurnSettings["reasoningEffort"])) {
                  onChange({ ...settings, reasoningEffort: value as AgentTurnSettings["reasoningEffort"] });
                }
              }}
            >
              {selectedModel?.supportedReasoningEfforts.map((effort) => (
                <DropdownMenuRadioItem key={effort} value={effort}>
                  {t(`efforts.${effort}`)}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>

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
      </div>

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
