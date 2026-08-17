"use client";

import { ChangePasswordDialog } from "@/features/settings/components/sections/account/account-password-dialog";
import { AccountActiveSessionsSection } from "@/features/settings/components/sections/account/account-active-sessions";
import { AccountActiveDevicesSection } from "@/features/settings/components/sections/account/account-active-devices";
import { AccountAddDeviceDialog } from "@/features/settings/components/sections/account/account-add-device-dialog";
import { AccountOverviewSection } from "@/features/settings/components/sections/account/account-overview";
import { useSettingsAccount } from "@/features/settings/hooks/use-settings-account";
import { SettingsPage, SettingsSectionSeparator } from "@/shared/components/settings-layout";

export function SettingsAccount() {
  const {
    viewer,
    sessions,
    devices,
    devicesLoading,
    loading,
    loggingOut,
    changingPassword,
    revokingSessionID,
    passwordDialogOpen,
    addDeviceDialogOpen,
    revokingDeviceID,
    updatingDeviceID,
    setPasswordDialogOpen,
    setAddDeviceDialogOpen,
    handleChangePassword,
    handleLogoutAll,
    handleLogoutSession,
    handleRevokeDevice,
    handleUpdateDevice,
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
      <AccountActiveDevicesSection
        devices={devices}
        loading={devicesLoading}
        revokingDeviceId={revokingDeviceID}
        updatingDeviceId={updatingDeviceID}
        addDisabled={loading || devicesLoading || !viewer?.publicID}
        onAdd={() => setAddDeviceDialogOpen(true)}
        onRevoke={(device) => void handleRevokeDevice(device)}
        onUpdate={(device) => void handleUpdateDevice(device)}
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
      <AccountAddDeviceDialog
        open={addDeviceDialogOpen}
        onOpenChange={setAddDeviceDialogOpen}
        publicUserID={viewer?.publicID || ""}
      />
    </SettingsPage>
  );
}
