"use client";

import * as React from "react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { SpinnerLabel } from "@/components/ui/spinner";
import type { UserDTO } from "@/shared/api/auth.types";
import { CopyActionButton } from "@/shared/components/copy-action";
import { SettingsSection } from "@/shared/components/settings-layout";

function ActionRow({ title, action }: { title: string; action: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <p className="min-w-0 flex-1 text-xs font-medium">{title}</p>
      <div className="flex shrink-0 justify-end">{action}</div>
    </div>
  );
}

export function AccountOverviewSection({
  viewer,
  loading,
  loggingOut,
  changingPassword,
  onOpenPasswordDialog,
  onLogoutAll,
}: {
  viewer: UserDTO | null;
  loading: boolean;
  loggingOut: boolean;
  changingPassword: boolean;
  onOpenPasswordDialog: () => void;
  onLogoutAll: () => void;
}) {
  const t = useTranslations("settings.accountPage");

  return (
    <SettingsSection title={t("title")}>
      <ActionRow
        title={t("password")}
        action={
          <Button type="button" variant="outline" disabled={loading || changingPassword} onClick={onOpenPasswordDialog}>
            {t("actions.update")}
          </Button>
        }
      />
      <ActionRow
        title={t("logoutAllDevices")}
        action={
          <Button type="button" variant="outline" disabled={loading || loggingOut} onClick={onLogoutAll}>
            {loggingOut ? <SpinnerLabel>{t("actions.loggingOut")}</SpinnerLabel> : t("actions.logOut")}
          </Button>
        }
      />
      <div className="flex items-center justify-between gap-4">
        <p className="min-w-0 flex-1 text-xs font-medium">{t("publicID")}</p>
        <div className="flex min-w-0 max-w-[min(60vw,26rem)] shrink items-center gap-2 rounded-lg bg-muted/35 px-2 py-1 text-xs text-muted-foreground">
          <span className="max-w-[min(75vw,26rem)] truncate">{viewer?.publicID || "-"}</span>
          <CopyActionButton
            type="button"
            variant="ghost"
            size="icon"
            value={viewer?.publicID || ""}
            messages={{ copied: t("toasts.publicIDCopied"), failed: t("toasts.copyFailed"), failedDescription: t("toasts.retryLater") }}
            disabled={!viewer?.publicID}
            aria-label={t("copyPublicID")}
            className="size-4 p-3"
          />
        </div>
      </div>
    </SettingsSection>
  );
}
