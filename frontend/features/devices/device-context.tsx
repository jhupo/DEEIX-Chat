"use client";

import * as React from "react";

import { listAgentDevices, listAgentWorkspaces, type AgentDeviceDTO, type AgentWorkspaceDTO } from "@/shared/api/agent-gateway";
import { getUserSettings, patchUserSettings } from "@/shared/api/user-settings";
import { useAuthSession } from "@/shared/auth/auth-session-context";

const DEFAULT_DEVICE_KEY = "agent.default_device_id";

type DeviceContextValue = {
  devices: AgentDeviceDTO[];
  defaultDeviceId: string;
  defaultDevice: AgentDeviceDTO | null;
  defaultWorkspace: AgentWorkspaceDTO | null;
  loading: boolean;
  refresh: () => Promise<void>;
  selectDefaultDevice: (deviceId: string) => Promise<void>;
};

const DeviceContext = React.createContext<DeviceContextValue | null>(null);

export function DeviceProvider({ children }: { children: React.ReactNode }) {
  const { accessToken } = useAuthSession();
  const [devices, setDevices] = React.useState<AgentDeviceDTO[]>([]);
  const [defaultDeviceId, setDefaultDeviceId] = React.useState("");
  const [defaultWorkspace, setDefaultWorkspace] = React.useState<AgentWorkspaceDTO | null>(null);
  const [loading, setLoading] = React.useState(true);

  const refresh = React.useCallback(async () => {
    setLoading(true);
    try {
      const [nextDevices, settings] = await Promise.all([
        listAgentDevices(accessToken),
        getUserSettings(accessToken),
      ]);
      const active = nextDevices.filter((item) => item.status === "active");
      const saved = settings[DEFAULT_DEVICE_KEY]?.trim() ?? "";
      const nextDefaultId = active.some((item) => item.deviceId === saved) ? saved : (active[0]?.deviceId ?? "");
      setDevices(nextDevices);
      setDefaultDeviceId(nextDefaultId);
      if (nextDefaultId) {
        const workspaces = await listAgentWorkspaces(accessToken, nextDefaultId);
        setDefaultWorkspace(workspaces.find((item) => item.status === "available") ?? workspaces[0] ?? null);
      } else {
        setDefaultWorkspace(null);
      }
    } catch {
      setDevices([]);
      setDefaultDeviceId("");
      setDefaultWorkspace(null);
    } finally {
      setLoading(false);
    }
  }, [accessToken]);

  React.useEffect(() => { void refresh(); }, [refresh]);

  const selectDefaultDevice = React.useCallback(async (deviceId: string) => {
    const selected = devices.find((item) => item.deviceId === deviceId && item.status === "active");
    if (!selected) return;
    await patchUserSettings(accessToken, { [DEFAULT_DEVICE_KEY]: selected.deviceId });
    setDefaultDeviceId(selected.deviceId);
    const workspaces = await listAgentWorkspaces(accessToken, selected.deviceId);
    setDefaultWorkspace(workspaces.find((item) => item.status === "available") ?? workspaces[0] ?? null);
  }, [accessToken, devices]);

  const defaultDevice = devices.find((item) => item.deviceId === defaultDeviceId) ?? null;
  const value = React.useMemo(() => ({
    devices,
    defaultDeviceId,
    defaultDevice,
    defaultWorkspace,
    loading,
    refresh,
    selectDefaultDevice,
  }), [defaultDevice, defaultDeviceId, defaultWorkspace, devices, loading, refresh, selectDefaultDevice]);

  return <DeviceContext.Provider value={value}>{children}</DeviceContext.Provider>;
}

export function useDevices() {
  const context = React.useContext(DeviceContext);
  if (!context) throw new Error("useDevices must be used within DeviceProvider");
  return context;
}
