"use client";

import * as React from "react";

import { listAgentDevices, type AgentDeviceDTO } from "@/shared/api/agent-gateway";
import { getUserSettings, patchUserSettings } from "@/shared/api/user-settings";
import { useAuthSession } from "@/shared/auth/auth-session-context";

const DEFAULT_DEVICE_KEY = "agent.default_device_id";

function devicesEqual(current: AgentDeviceDTO[], next: AgentDeviceDTO[]): boolean {
  return current.length === next.length && current.every((device, index) => {
    const candidate = next[index];
    return candidate !== undefined &&
      device.agentVersion === candidate.agentVersion &&
      device.createdAt === candidate.createdAt &&
      device.deviceId === candidate.deviceId &&
      device.lastSeenAt === candidate.lastSeenAt &&
      device.latestAgentVersion === candidate.latestAgentVersion &&
      device.name === candidate.name &&
      device.online === candidate.online &&
      device.platform === candidate.platform &&
      device.status === candidate.status &&
      device.updateAvailable === candidate.updateAvailable &&
      device.updatedAt === candidate.updatedAt &&
      device.userId === candidate.userId;
  });
}

type DeviceContextValue = {
  devices: AgentDeviceDTO[];
  defaultDeviceId: string;
  defaultDevice: AgentDeviceDTO | null;
  loading: boolean;
  refresh: () => Promise<void>;
  selectDefaultDevice: (deviceId: string) => Promise<void>;
};

const DeviceContext = React.createContext<DeviceContextValue | null>(null);

export function DeviceProvider({ children }: { children: React.ReactNode }) {
  const { accessToken } = useAuthSession();
  const [devices, setDevices] = React.useState<AgentDeviceDTO[]>([]);
  const [defaultDeviceId, setDefaultDeviceId] = React.useState("");
  const [loading, setLoading] = React.useState(true);
  const accessTokenRef = React.useRef(accessToken);
  const refreshInFlightRef = React.useRef(false);
  accessTokenRef.current = accessToken;

  const applyDevices = React.useCallback((nextDevices: AgentDeviceDTO[], preferredDeviceId?: string) => {
    const active = nextDevices.filter((item) => item.status === "active");
    setDevices((current) => devicesEqual(current, nextDevices) ? current : nextDevices);
    setDefaultDeviceId((current) => {
      const preferred = preferredDeviceId ?? current;
      return active.some((item) => item.deviceId === preferred) ? preferred : (active[0]?.deviceId ?? "");
    });
  }, []);

  const refresh = React.useCallback(async () => {
    if (refreshInFlightRef.current) {
      return;
    }
    refreshInFlightRef.current = true;
    try {
      const nextDevices = await listAgentDevices(accessToken);
      if (accessTokenRef.current === accessToken) {
        applyDevices(nextDevices);
      }
    } catch {
      // Keep the last known state during a transient polling failure.
    } finally {
      refreshInFlightRef.current = false;
    }
  }, [accessToken, applyDevices]);

  React.useEffect(() => {
    let active = true;
    setLoading(true);
    void Promise.all([listAgentDevices(accessToken), getUserSettings(accessToken)])
      .then(([nextDevices, settings]) => {
        if (active) applyDevices(nextDevices, settings[DEFAULT_DEVICE_KEY]?.trim() ?? "");
      })
      .catch(() => {
        if (active) {
          setDevices([]);
          setDefaultDeviceId("");
        }
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") {
        void refresh();
      }
    };
    const timer = window.setInterval(refreshWhenVisible, 15_000);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    return () => {
      active = false;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", refreshWhenVisible);
    };
  }, [accessToken, applyDevices, refresh]);

  const selectDefaultDevice = React.useCallback(async (deviceId: string) => {
    const selected = devices.find((item) => item.deviceId === deviceId && item.status === "active");
    if (!selected) return;
    await patchUserSettings(accessToken, { [DEFAULT_DEVICE_KEY]: selected.deviceId });
    setDefaultDeviceId(selected.deviceId);
  }, [accessToken, devices]);

  const defaultDevice = devices.find((item) => item.deviceId === defaultDeviceId) ?? null;
  const value = React.useMemo(() => ({
    devices,
    defaultDeviceId,
    defaultDevice,
    loading,
    refresh,
    selectDefaultDevice,
  }), [defaultDevice, defaultDeviceId, devices, loading, refresh, selectDefaultDevice]);

  return <DeviceContext.Provider value={value}>{children}</DeviceContext.Provider>;
}

export function useDevices() {
  const context = React.useContext(DeviceContext);
  if (!context) throw new Error("useDevices must be used within DeviceProvider");
  return context;
}
