"use client";

import * as React from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { changePassword, getCurrentActiveSessions, getMe, logoutAll, logoutSession } from "@/shared/api/auth";
import type { ActiveSessionDTO, UserDTO } from "@/shared/api/auth.types";
import { clearSessionAndRedirectToLogin } from "@/shared/auth/session";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import {
  getAgentCommand,
  listAgentDevices,
  revokeAgentDevice,
  type AgentDeviceDTO,
  updateAgentDevice,
} from "@/shared/api/agent-gateway";
import { useDevices } from "@/features/devices";

export function useSettingsAccount() {
  const t = useTranslations("settings.accountPage.toasts");
  const translateError = useLocalizedErrorMessage();
  const [viewer, setViewer] = React.useState<UserDTO | null>(null);
  const [sessions, setSessions] = React.useState<ActiveSessionDTO[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [loggingOut, setLoggingOut] = React.useState(false);
  const [changingPassword, setChangingPassword] = React.useState(false);
  const [revokingSessionID, setRevokingSessionID] = React.useState("");
  const [passwordDialogOpen, setPasswordDialogOpen] = React.useState(false);
  const [addDeviceDialogOpen, setAddDeviceDialogOpen] = React.useState(false);
  const [revokingDeviceID, setRevokingDeviceID] = React.useState("");
  const [updatingDeviceID, setUpdatingDeviceID] = React.useState("");
  const { devices, loading: devicesLoading, refresh: refreshDevices } = useDevices();

  const loadAccountData = React.useCallback(async () => {
    setLoading(true);
    try {
      const token = await resolveAccessToken();
      if (!token) {
        setViewer(null);
        setSessions([]);
        return;
      }
      const [nextViewer, sessionData] = await Promise.all([getMe(token), getCurrentActiveSessions(token)]);
      setViewer(nextViewer);
      setSessions(sessionData.results);
    } catch (error) {
      toast.error(t("loadFailed"), { description: translateError(error, t("retryLater")) });
    } finally {
      setLoading(false);
    }
  }, [t, translateError]);

  React.useEffect(() => {
    void loadAccountData();
  }, [loadAccountData]);

  const handleChangePassword = React.useCallback(async (payload: { currentPassword: string; newPassword: string }) => {
    if (changingPassword) return;
    setChangingPassword(true);
    try {
      const token = await resolveAccessToken();
      if (!token) throw new Error(t("sessionMissing"));
      await changePassword(token, payload);
      toast.success(t("passwordChanged"), { description: t("passwordChangedDescription") });
      setPasswordDialogOpen(false);
      clearSessionAndRedirectToLogin();
    } catch (error) {
      toast.error(t("changePasswordFailed"), { description: translateError(error, t("retryLater")) });
    } finally {
      setChangingPassword(false);
    }
  }, [changingPassword, t, translateError]);

  const handleLogoutAll = React.useCallback(async () => {
    if (loggingOut) return;
    setLoggingOut(true);
    try {
      const token = await resolveAccessToken();
      if (token) await logoutAll(token);
    } catch {
      // Ensure the local session is cleared even when remote revocation fails.
    } finally {
      clearSessionAndRedirectToLogin();
      setLoggingOut(false);
    }
  }, [loggingOut]);

  const handleLogoutSession = React.useCallback(async (session: ActiveSessionDTO) => {
    const sessionID = session.sessionID.trim();
    if (!sessionID || revokingSessionID) return;
    setRevokingSessionID(sessionID);
    try {
      const token = await resolveAccessToken();
      if (!token) throw new Error(t("sessionMissing"));
      await logoutSession(token, sessionID);
      if (session.current) {
        clearSessionAndRedirectToLogin();
        return;
      }
      setSessions((current) => current.filter((item) => item.sessionID !== sessionID));
      toast.success(t("sessionLoggedOut"));
    } catch (error) {
      toast.error(t("logoutSessionFailed"), { description: translateError(error, t("retryLater")) });
    } finally {
      setRevokingSessionID("");
    }
  }, [revokingSessionID, t, translateError]);

  const handleRevokeDevice = React.useCallback(async (device: AgentDeviceDTO) => {
    if (revokingDeviceID) return;
    setRevokingDeviceID(device.deviceId);
    try {
      const token = await resolveAccessToken();
      if (!token) throw new Error(t("sessionMissing"));
      await revokeAgentDevice(token, device.deviceId);
      await refreshDevices();
      toast.success(t("deviceRevoked"));
    } catch (error) {
      toast.error(t("deviceRevokeFailed"), { description: translateError(error, t("retryLater")) });
    } finally {
      setRevokingDeviceID("");
    }
  }, [refreshDevices, revokingDeviceID, t, translateError]);

  const handleUpdateDevice = React.useCallback(async (device: AgentDeviceDTO) => {
    if (updatingDeviceID || !device.updateAvailable) return;
    setUpdatingDeviceID(device.deviceId);
    try {
      const token = await resolveAccessToken();
      if (!token) throw new Error(t("sessionMissing"));
      const queued = await updateAgentDevice(token, device.deviceId);
      if (!device.online) {
        toast.info(t("deviceUpdateStarted"), { description: t("deviceUpdateStartedDescription") });
        return;
      }
      let accepted = false;
      for (let attempt = 0; attempt < 120; attempt++) {
        await new Promise((resolve) => window.setTimeout(resolve, 500));
        const command = await getAgentCommand(token, queued.commandId);
        if (command.status === "error") throw new Error(command.errorMessage || t("deviceUpdateFailed"));
        if (command.status === "completed") {
          accepted = true;
          break;
        }
      }
      if (!accepted) throw new Error(t("deviceUpdateTimeout"));
      for (let attempt = 0; attempt < 180; attempt++) {
        await new Promise((resolve) => window.setTimeout(resolve, 1_000));
        try {
          const devices = await listAgentDevices(token);
          const updated = devices.find((item) => item.deviceId === device.deviceId);
          if (updated?.online && updated.agentVersion === device.latestAgentVersion) {
            await refreshDevices();
            toast.success(t("deviceUpdated"), { description: t("deviceUpdatedDescription", { version: updated.agentVersion }) });
            return;
          }
        } catch {
          // The device may briefly interrupt the connection while its service is replaced.
        }
      }
      await refreshDevices();
      toast.info(t("deviceUpdateStarted"), { description: t("deviceUpdateStartedDescription") });
    } catch (error) {
      toast.error(t("deviceUpdateFailed"), { description: translateError(error, t("retryLater")) });
    } finally {
      setUpdatingDeviceID("");
    }
  }, [refreshDevices, t, translateError, updatingDeviceID]);

  return {
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
  };
}
