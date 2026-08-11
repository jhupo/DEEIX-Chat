"use client";

import { ChangePasswordDialog } from "@/features/settings/components/sections/account/account-password-dialog";
import { AccountActiveSessionsSection } from "@/features/settings/components/sections/account/account-active-sessions";
import { AccountOverviewSection } from "@/features/settings/components/sections/account/account-overview";
import { useSettingsAccount } from "@/features/settings/hooks/use-settings-account";
import { SettingsPage, SettingsSectionSeparator } from "@/shared/components/settings-layout";

export function SettingsAccount() {
  const {
    viewer,
    sessions,
    loading,
    loggingOut,
    changingPassword,
    revokingSessionID,
    passwordDialogOpen,
    setPasswordDialogOpen,
    handleChangePassword,
    handleLogoutAll,
    handleLogoutSession,
  } = useSettingsAccount();

  return (
    <SettingsPage>
      <AccountOverviewSection
        viewer={viewer}
        loading={loading}
        loggingOut={loggingOut}
        changingPassword={changingPassword}
        onOpenPasswordDialog={() => setPasswordDialogOpen(true)}
        onLogoutAll={() => void handleLogoutAll()}
      />
      <SettingsSectionSeparator />
      <AccountActiveSessionsSection
        sessions={sessions}
        loading={loading}
        revokingSessionID={revokingSessionID}
        onLogoutSession={(session) => void handleLogoutSession(session)}
      />
      <ChangePasswordDialog
        open={passwordDialogOpen}
        onOpenChange={setPasswordDialogOpen}
        pending={changingPassword}
        onSubmit={handleChangePassword}
      />
    </SettingsPage>
  );
}
