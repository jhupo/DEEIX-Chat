import type { DeviceResponseDocData, WorkspaceDoc } from "@deeix/api-contract";

import { authedRequest } from "@/shared/api/authed-client";

export type AgentDeviceDTO = Omit<DeviceResponseDocData, "lastSeenAt" | "platform" | "status"> & {
  platform: "windows" | "darwin" | "linux";
  status: "active" | "revoked";
  lastSeenAt: string | null;
};

export type AgentWorkspaceDTO = WorkspaceDoc;

export async function listAgentDevices(accessToken: string): Promise<AgentDeviceDTO[]> {
  return authedRequest<AgentDeviceDTO[]>("/api/v1/agent/devices", { accessToken }, true);
}

export async function listAgentWorkspaces(accessToken: string, deviceId: string): Promise<AgentWorkspaceDTO[]> {
  return authedRequest<AgentWorkspaceDTO[]>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/workspaces`,
    { accessToken },
    true,
  );
}

export async function revokeAgentDevice(accessToken: string, deviceId: string): Promise<void> {
  await authedRequest(`/api/v1/agent/devices/${encodeURIComponent(deviceId)}`, {
    accessToken,
    method: "DELETE",
  }, true);
}
