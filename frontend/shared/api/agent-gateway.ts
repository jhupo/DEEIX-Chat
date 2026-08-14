import type { DeviceResponseDocData, RuntimeProfileDoc, WorkspaceDoc } from "@deeix/api-contract";

import { authedRequest } from "@/shared/api/authed-client";

export type AgentDeviceDTO = Omit<DeviceResponseDocData, "lastSeenAt" | "platform" | "status"> & {
  platform: "windows" | "darwin" | "linux";
  status: "active" | "revoked";
  lastSeenAt: string | null;
};

export type AgentWorkspaceDTO = WorkspaceDoc;
export type AgentRuntimeProfileDTO = RuntimeProfileDoc;

export type AgentResourceSnapshotDTO = {
  resource: string;
  scope: "profile" | "workspace";
  deviceId: string;
  profileId: string;
  workspaceId?: string;
  data: unknown;
  refreshedAt: string;
};

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

export async function listAgentRuntimeProfiles(accessToken: string, deviceId: string): Promise<AgentRuntimeProfileDTO[]> {
  return authedRequest<AgentRuntimeProfileDTO[]>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/profiles`,
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

export async function getAgentWorkspaceResource(
  accessToken: string,
  deviceId: string,
  workspaceId: string,
  resource: string,
): Promise<AgentResourceSnapshotDTO> {
  return authedRequest<AgentResourceSnapshotDTO>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/workspaces/${encodeURIComponent(workspaceId)}/resources/${encodeURIComponent(resource)}`,
    { accessToken },
    true,
  );
}

export async function refreshAgentWorkspaceResource(
  accessToken: string,
  deviceId: string,
  workspaceId: string,
  resource: string,
): Promise<void> {
  await authedRequest(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/workspaces/${encodeURIComponent(workspaceId)}/resources/${encodeURIComponent(resource)}/refresh`,
    {
      accessToken,
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
    },
    true,
  );
}
