import type { DeviceResponseDocData } from "@deeix/api-contract";

import { authedRequest } from "@/shared/api/authed-client";

export type AgentDeviceDTO = Omit<DeviceResponseDocData, "lastSeenAt" | "platform" | "status"> & {
  platform: "windows" | "darwin" | "linux";
  status: "active" | "revoked";
  lastSeenAt: string | null;
  agentVersion: string;
  latestAgentVersion: string;
  updateAvailable: boolean;
};

export async function listAgentDevices(accessToken: string): Promise<AgentDeviceDTO[]> {
  return authedRequest<AgentDeviceDTO[]>("/api/v1/agent/devices", { accessToken }, true);
}

export async function revokeAgentDevice(accessToken: string, deviceId: string): Promise<void> {
  await authedRequest(`/api/v1/agent/devices/${encodeURIComponent(deviceId)}`, {
    accessToken,
    method: "DELETE",
  }, true);
}

export async function updateAgentDevice(
  accessToken: string,
  deviceId: string,
): Promise<{ commandId: string; status: string }> {
  return authedRequest<{ commandId: string; status: string }>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/update`,
    {
      accessToken,
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
    },
    true,
  );
}

export type AgentRuntimeProfileDTO = {
  profileId: string;
  deviceId: string;
  provider: string;
  status: "proving" | "ready";
  manifest: { commands?: string[] };
};

export async function listAgentRuntimeProfiles(
  accessToken: string,
  deviceId: string,
): Promise<AgentRuntimeProfileDTO[]> {
  return authedRequest<AgentRuntimeProfileDTO[]>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/profiles`,
    { accessToken },
    true,
  );
}

export async function registerAgentWorkspace(
  accessToken: string,
  deviceId: string,
  input: { profileId: string; path: string; create: boolean },
): Promise<{ commandId: string; status: string }> {
  return authedRequest<{ commandId: string; status: string }>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/workspaces`,
    {
      accessToken,
      method: "POST",
      body: input,
      headers: { "Idempotency-Key": crypto.randomUUID() },
    },
    true,
  );
}

export async function renameAgentWorkspace(
  accessToken: string,
  deviceId: string,
  workspaceId: string,
  name: string,
): Promise<{ commandId: string; status: string }> {
  return authedRequest<{ commandId: string; status: string }>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/workspaces/${encodeURIComponent(workspaceId)}`,
    {
      accessToken,
      method: "PATCH",
      body: { name },
      headers: { "Idempotency-Key": crypto.randomUUID() },
    },
    true,
  );
}

export async function unregisterAgentWorkspace(
  accessToken: string,
  deviceId: string,
  workspaceId: string,
): Promise<{ commandId: string; status: string }> {
  return authedRequest<{ commandId: string; status: string }>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/workspaces/${encodeURIComponent(workspaceId)}`,
    {
      accessToken,
      method: "DELETE",
      headers: { "Idempotency-Key": crypto.randomUUID() },
    },
    true,
  );
}

export async function getAgentCommand(
  accessToken: string,
  commandId: string,
): Promise<{ commandId: string; status: string; errorMessage?: string }> {
  return authedRequest<{ commandId: string; status: string; errorMessage?: string }>(
    `/api/v1/agent/commands/${encodeURIComponent(commandId)}`,
    { accessToken },
    true,
  );
}

export async function waitForAgentCommand(
  accessToken: string,
  commandId: string,
  attempts = 120,
): Promise<{ commandId: string; status: string; errorMessage?: string } | null> {
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    await new Promise((resolve) => setTimeout(resolve, 500));
    const command = await getAgentCommand(accessToken, commandId);
    if (command.status === "completed" || command.status === "error") {
      return command;
    }
  }
  return null;
}
