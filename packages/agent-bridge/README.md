# @deeix/agent-bridge

Local Gateway runtime for DEEIX Agent execution. The package owns device pairing, outbound WSS, local command validation,
workspace path containment, crash-safe command/event/source journals, verified image/audio downloads, and the pinned Codex
app-server `0.147.0` adapter. A command becomes receipt-ready only after all referenced artifacts pass size and SHA-256
validation; short-lived artifact grants are never written to the Bridge WAL.

It does not contain Browser APIs, Chat execution, Sub2 commerce, provider credentials, or Cloud persistence.

```bash
pnpm --filter @deeix/agent-bridge check
pnpm --filter @deeix/agent-bridge test
pnpm --filter @deeix/agent-bridge build
pnpm --filter @deeix/agent-bridge check:schema
```

After building, pair once with a code created by the signed-in DEEIX user, then run with explicit local registrations:

```bash
node dist/src/cli.js pair --server https://HOST --code ENROLLMENT_CODE --name DEVICE_NAME
node dist/src/cli.js run --profile PROFILE_ID --workspace WORKSPACE_ID=/absolute/project/path --codex /path/to/codex
```

The Cloud-visible `userId` comes from the existing `identity_users.public_id`; the Bridge stores only opaque device,
profile, workspace, and source references. Provider IDs and absolute paths remain local. Runtime admission sends only an
HMAC proof computed from the Codex API key; it never sends or persists that key in the Bridge protocol.
