import type {
  DeviceResponseDocData,
  ProviderManifestDoc,
  ResourceSnapshotDoc,
  RuntimeProfileDoc,
  WorkspaceDoc,
} from "@deeix/api-contract";

import { authedFetch, authedRequest } from "@/shared/api/authed-client";

export async function streamAgentEvents(
  accessToken: string,
  signal: AbortSignal,
  onEvent: (type: "ready" | "change") => void,
): Promise<void> {
  const response = await authedFetch(
    "/api/v1/agent/events/stream",
    { accessToken, signal },
    true,
  );
  if (!response.body) {
    throw new Error("agent event stream body is empty");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      const raw = line.trim();
      if (!raw) continue;
      const event: unknown = JSON.parse(raw);
      if (event !== null && typeof event === "object") {
        const type = Reflect.get(event, "type");
        if (type === "ready" || type === "change") onEvent(type);
      }
    }
    if (done) return;
  }
}

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

export type AgentReasoningEffort = "low" | "medium" | "high" | "xhigh";

export type AgentTurnSettings = {
  model: string;
  reasoningEffort: AgentReasoningEffort;
  approvalPolicy: "on-request" | "never";
  approvalsReviewer: "user" | "auto_review";
  sandboxPolicy: "workspace-write" | "danger-full-access";
};

export type AgentProviderManifestDTO = Omit<ProviderManifestDoc, "threadSettings"> & {
  threadSettings: ProviderManifestDoc["threadSettings"] & {
    approvalsReviewer?: string[];
  };
};

export type AgentRuntimeProfileDTO = Omit<RuntimeProfileDoc, "manifest" | "status"> & {
  status: "proving" | "ready";
  manifest: AgentProviderManifestDTO;
};

export type AgentWorkspaceDTO = WorkspaceDoc;
export type AgentResourceSnapshotDTO = ResourceSnapshotDoc;

export type AgentModelDTO = {
  id: string;
  displayName: string;
  description: string;
  isDefault: boolean;
  defaultReasoningEffort: AgentReasoningEffort;
  supportedReasoningEfforts: AgentReasoningEffort[];
};

const AGENT_REASONING_EFFORTS = new Set<AgentReasoningEffort>([
  "low",
  "medium",
  "high",
  "xhigh",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function parseReasoningEffort(value: unknown): AgentReasoningEffort | null {
  return typeof value === "string" && AGENT_REASONING_EFFORTS.has(value as AgentReasoningEffort)
    ? value as AgentReasoningEffort
    : null;
}

export function parseAgentModelsResource(
  value: unknown,
  allowedReasoningEfforts?: readonly string[],
): AgentModelDTO[] {
  if (!isRecord(value) || !Array.isArray(value.data)) {
    return [];
  }
  const allowed = allowedReasoningEfforts
    ? new Set(
        allowedReasoningEfforts
          .map(parseReasoningEffort)
          .filter((item): item is AgentReasoningEffort => item !== null),
      )
    : AGENT_REASONING_EFFORTS;
  const seen = new Set<string>();
  const models: AgentModelDTO[] = [];
  for (const rawModel of value.data) {
    if (!isRecord(rawModel) || rawModel.hidden === true) {
      continue;
    }
    const id = typeof rawModel.id === "string" ? rawModel.id.trim() : "";
    const model = typeof rawModel.model === "string" ? rawModel.model.trim() : "";
    const displayName = typeof rawModel.displayName === "string" ? rawModel.displayName.trim() : "";
    const description = typeof rawModel.description === "string" ? rawModel.description.trim() : "";
    if (
      !id || id.length > 512 || !model || !displayName || seen.has(id) ||
      typeof rawModel.isDefault !== "boolean" || typeof rawModel.hidden !== "boolean"
    ) {
      continue;
    }
    const rawEfforts = Array.isArray(rawModel.supportedReasoningEfforts)
      ? rawModel.supportedReasoningEfforts
      : [];
    const supportedReasoningEfforts: AgentReasoningEffort[] = [];
    for (const rawEffort of rawEfforts) {
      if (!isRecord(rawEffort) || typeof rawEffort.description !== "string") {
        continue;
      }
      const effort = parseReasoningEffort(rawEffort.reasoningEffort);
      if (effort && allowed.has(effort) && !supportedReasoningEfforts.includes(effort)) {
        supportedReasoningEfforts.push(effort);
      }
    }
    const declaredDefault = parseReasoningEffort(rawModel.defaultReasoningEffort);
    if (!declaredDefault) {
      continue;
    }
    const defaultReasoningEffort = declaredDefault && supportedReasoningEfforts.includes(declaredDefault)
      ? declaredDefault
      : supportedReasoningEfforts[0];
    if (!defaultReasoningEffort) {
      continue;
    }
    seen.add(id);
    models.push({
      id,
      displayName,
      description,
      isDefault: rawModel.isDefault,
      defaultReasoningEffort,
      supportedReasoningEfforts,
    });
  }
  return models;
}

export function includeCurrentAgentModel(
  models: AgentModelDTO[],
  currentModel: string,
  currentReasoningEffort: string,
  allowedReasoningEfforts: readonly string[],
): AgentModelDTO[] {
  const id = currentModel.trim();
  if (!id || models.some((item) => item.id === id)) {
    return models;
  }
  const supportedReasoningEfforts = allowedReasoningEfforts
    .map(parseReasoningEffort)
    .filter((item): item is AgentReasoningEffort => item !== null);
  if (supportedReasoningEfforts.length === 0) {
    return models;
  }
  const requestedEffort = parseReasoningEffort(currentReasoningEffort);
  return [
    {
      id,
      displayName: id,
      description: "",
      isDefault: false,
      defaultReasoningEffort: requestedEffort && supportedReasoningEfforts.includes(requestedEffort)
        ? requestedEffort
        : supportedReasoningEfforts[0],
      supportedReasoningEfforts,
    },
    ...models,
  ];
}

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

export async function listAgentWorkspaces(
  accessToken: string,
  deviceId: string,
): Promise<AgentWorkspaceDTO[]> {
  return authedRequest<AgentWorkspaceDTO[]>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/workspaces`,
    { accessToken },
    true,
  );
}

export async function getAgentProfileResource(
  accessToken: string,
  deviceId: string,
  profileId: string,
  resource: string,
): Promise<AgentResourceSnapshotDTO> {
  return authedRequest<AgentResourceSnapshotDTO>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/profiles/${encodeURIComponent(profileId)}/resources/${encodeURIComponent(resource)}`,
    { accessToken },
    true,
  );
}

export async function refreshAgentProfileResource(
  accessToken: string,
  deviceId: string,
  profileId: string,
  resource: string,
): Promise<{ commandId: string; status: string }> {
  return authedRequest<{ commandId: string; status: string }>(
    `/api/v1/agent/devices/${encodeURIComponent(deviceId)}/profiles/${encodeURIComponent(profileId)}/resources/${encodeURIComponent(resource)}/refresh`,
    {
      accessToken,
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
    },
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
