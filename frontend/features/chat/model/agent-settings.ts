import type { AgentRuntimeProfileDTO } from "@/shared/api/agent-gateway";

function supportsAgentComposer(profile: AgentRuntimeProfileDTO): boolean {
  return profile.status === "ready" &&
    profile.provider === "codex" &&
    profile.manifest.threadSettings.model;
}

export function resolveAgentComposerProfile(
  profiles: AgentRuntimeProfileDTO[],
  preferredProfileID?: string,
): AgentRuntimeProfileDTO | undefined {
  const normalizedProfileID = preferredProfileID?.trim() ?? "";
  if (normalizedProfileID) {
    return profiles.find(
      (profile) => profile.profileId === normalizedProfileID && supportsAgentComposer(profile),
    );
  }
  return profiles.find(supportsAgentComposer);
}
