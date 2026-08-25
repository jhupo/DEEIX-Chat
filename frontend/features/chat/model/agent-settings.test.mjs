import assert from "node:assert/strict";
import test from "node:test";

import { resolveAgentComposerProfile } from "./agent-settings.ts";

function profile(profileId, overrides = {}) {
  return {
    profileId,
    status: "ready",
    provider: "codex",
    manifest: { threadSettings: { model: true } },
    ...overrides,
  };
}

test("uses a ready Codex profile before a workspace is selected", () => {
  const selected = resolveAgentComposerProfile([
    profile("proving", { status: "proving" }),
    profile("codex-default"),
  ]);

  assert.equal(selected?.profileId, "codex-default");
});

test("honors an explicit profile without falling back to another runtime", () => {
  const profiles = [profile("codex-default"), profile("workspace-profile")];

  assert.equal(resolveAgentComposerProfile(profiles, "workspace-profile")?.profileId, "workspace-profile");
  assert.equal(resolveAgentComposerProfile(profiles, "missing"), undefined);
});
