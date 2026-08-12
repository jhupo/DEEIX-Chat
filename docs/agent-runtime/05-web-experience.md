# Web 体验

## 1. Static route and runtime boundary

Agent uses one static export-compatible page:

```text
/agent
/agent?thread_id=<thread_public_id>
```

`frontend/next.config.ts` has `output: "export"` without `trailingSlash`, so there is no runtime thread path segment. Static hosting rewrites `/agent` to `/agent.html` while preserving the original query string. `thread_id` is validated as a public ID before API request. Route navigation updates query state with router push/replace; reload, bookmarked link and share all resolve through `/agent`.

Chat remains `/chat?conversation_id=...`. Each route mounts its own runtime: chat keeps `SidebarConversationsProvider` and `ChatSessionProvider`; Agent mounts a page-scoped `AgentRuntimeProvider`/reducer. The Agent reducer never creates, resets or reads a ChatSession.

## 2. Navigation and launcher

### 2.1 Sidebar seam

`NAVIGATION_ITEMS` and `NavMainItem` retain the existing `newChat` command, callback and shortcut plumbing. Main navigation gets a primary Agent link to `/agent`. `AppSidebar` renders a separate `NavAgentThreads` section alongside existing chat project/starred/recent sections.

`NavAgentThreads`:

- loads only Agent thread summaries, grouped by selected device/workspace and respecting User ownership;
- renders recent and pinned AgentThread records with device status indicator;
- links to `/agent?thread_id=<public_id>`;
- has an accessible heading, empty state and retry state;
- does not reuse conversation project selection, Conversation IDs or ChatSession state.

Mobile header exposes Agent navigation through the existing navigation pattern. New Chat remains a Chat command, never a two-mode menu.

### 2.2 `/agent` launcher

Without `thread_id`, `/agent` shows a compact operational launcher rather than a marketing screen:

1. device picker with online, degraded, offline, revoking and revoked state;
2. profile picker filtered to ready/capability-compatible profiles;
3. workspace picker grouped by selected device, with local picker action when supported;
4. recent/pinned Agent thread list for that device/workspace;
5. create-thread form with model, permission profile, collaboration mode and initial input where manifest allows them.

Profile schema mismatch, missing auth and offline device state have explicit status text. Create is disabled until ownership, workspace and profile validation succeed. Submitting creates a queued thread projection and navigates to its query URL; Web does not await a live socket before showing the command state.

## 3. Workbench layout

With `thread_id`, the page uses a three-region workbench that collapses predictably on narrow viewports.

| Region | Contents | Data source |
| --- | --- | --- |
| Header/toolbar | thread title, device/profile/workspace, connection state, pin, archive, fork, rename, share, inspector toggle | AgentThread projection and capability manifest |
| Main timeline | turn inputs, plan, items, diffs, artifacts, interactions and terminal summaries | AgentTurn/Item/Interaction plus thread event reducer |
| Inspector drawer | selected item raw-safe data, files, diff, command output, usage, audit link and resource shortcuts | redacted item projection/resources |

Toolbar actions have icon plus accessible name, tooltip and disabled reason. Rename uses the separate idempotent name path with bounded normalized `name` and queued `thread.rename` state; provider result/event updates title. Pin updates DEEIX-owned cloud `is_pinned` through claim-first HTTP idempotency and has no Provider/app-server dispatch; labels and share are also DEEIX metadata actions. A provider Git metadata editor uses the separate idempotent provider-metadata path with generated `gitInfo` validation for optional nullable `sha`, `branch`, and `originUrl`, plus queued command state; it rejects every other field and never edits labels, share, or pin. Archive/delete/fork actions display command lifecycle states rather than assuming immediate device execution.

`thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

## 4. Timeline item renderers

The reducer produces a chronological thread view by `thread_seq`, then item ordinal/chunk ordering. Components use discriminated item/event kinds:

| Item or event | Renderer | Interaction |
| --- | --- | --- |
| user input / agent message | Markdown-safe message block with copy control | complete text arrives through item delta/completed |
| reasoning | collapsed reasoning disclosure with streaming indicator | user-expanded; terminal state preserved |
| plan | compact ordered plan panel | `turn/plan/updated` changes plan only; it never substitutes item journal |
| command execution | stable-height command block with status, stdout/stderr chunks and exit result | output chunks deduplicate; sensitive environment values omitted |
| diff/file change | file summary with changed-line count and diff inspector | opens signed file/diff reference; terminal item owns final content |
| MCP/tool/app | provider-labelled operation block | linked resource details respect capability and redaction |
| artifact | filename, MIME, size, transfer state, download action | download uses short-lived artifact endpoint |
| runtime/error/extension | concise notice with expandable redacted payload | unknown provider extension remains inspectable without breaking flow |

`item/*` is the complete item projection source. `turn/plan/updated` and `turn/diff/updated` may contain empty item lists, so their panels update independent state and never erase timeline items. `item.completed` finalizes an item; `turn.completed` finalizes a turn.

Streaming text is appended by `(item_id, channel, chunk_seq)`. Delta before started creates a placeholder. Completed merges structured terminal payload, while stale delta remains display-only and cannot overwrite terminal fields.

## 5. Composer and turn controls

Composer uses the selected thread's profile manifest rather than a generic provider selector.

- New-thread submission creates a `thread.create` command with typed thread-start settings only. Optional initial input is retained in a provisional `awaiting_thread` turn and becomes exactly one `turn.start` after Bridge binds the thread source ref, preserving one-submit UX while exposing queued create then queued turn phases.
- Existing idle thread sends `turn.start`; active thread presents `turn.steer` and interrupt controls.
- Model, effort, permission profile, collaboration mode and input attachment controls render only when the profile declares compatible capability.
- Attachments upload through existing object-storage UI and are presented to Agent API as upload refs; local paths never appear in browser state.
- Submit, steer, interrupt and retry have stable button dimensions, keyboard activation and command pending/error state.

Turn control response surfaces queued, delivery-started, receipt acknowledged and Bridge-settled command state; receipt acknowledgement is not provider completion. A repeat click keeps the same idempotency key until the server response is resolved. The client does not expose an arbitrary provider method or arbitrary JSON editor.

## 6. Interactions

Pending `AgentInteraction` appears in a focused drawer/card associated with its thread and, when scope is `turn`, its turn/item. Typed renderers cover command approval, file approval, permission decision, user input, MCP elicitation and dynamic tool request. A no-turn `mcpServer/elicitation/request` is rendered as a thread-level interaction with profile/request context rather than a synthetic turn.

1. Display includes action, workspace-relative scope, deadline and redacted context.
2. Required decision/input widgets are keyboard-first and validate locally before submit.
3. Submit calls `POST /api/v1/agent/interactions/:interaction_id/respond` with `Idempotency-Key`.
4. UI changes from pending to responding after accepted CAS. Before first delivery, expiry may close it through its same-sequence tombstone; after delivery starts it remains responding until projected resolved/provider-cleared or Bridge terminal result/recovery writes expired/failed, with normalized terminal reason.
5. Another tab or duplicate click receives current projection and does not deliver a second provider response.

Sensitive content is visibly redacted. Approval UI states that local Bridge/provider policy will revalidate scope; it does not represent a browser-only permission boundary.

## 7. Resource surfaces

Resource pages live under static `/agent` view state, using query/panel state rather than dynamic page segments.

| Surface | Content | Actions |
| --- | --- | --- |
| Sessions/threads | device/workspace filtered history, archived/pinned state, provider summary | open, fork, archive, delete, refresh |
| Workspaces/files | workspace metadata, Git summary, signed file browser, transfer history | local picker, browse, attach, download |
| Models/modes | models, effort, permission profiles, collaboration modes | select for typed thread/turn settings |
| Skills/Hooks | workspace skills, extra roots summary, hook list | read-only refresh/diagnostics; no skill-root or skill-config mutation |
| Plugins/marketplaces/Apps | installed resources, version/status, app details | first-release plugins are read-only capability-gated; Apps read state |
| MCP | server status, resource/tool summaries, OAuth pending state | read-only redacted status/refresh; no reload or OAuth start |
| Config | requirement-driven fields, configured secret indicators, diagnostics | read-only diagnostics; no write/batch-write control |
| Account | login status and rate limits | read-only auth projection refresh; no local-auth start/logout/token refresh |

Each surface caches by `profile_id`/`workspace_id` plus snapshot timestamp. Refresh reads or a safe typed diagnostic refresh may query the device without persisting auth/config; values without a ready local profile show stale timestamp and no mutation control.

## 8. Reducer, replay and offline behavior

`AgentRuntimeProvider` state contains selected IDs, thread projection, turns, items, interactions, resource snapshots, command states and `lastAppliedSeq`. It is created for `/agent` and disposed on route exit.

1. Reconnect with a mounted reducer establishes notifier subscription first, then requests `GET /api/v1/agent/threads/:thread_id/events?after_seq=lastAppliedSeq` strictly from the existing `lastAppliedSeq`; it never overwrites that cursor from a thread header.
2. Cold load, cursor gap or compaction recovery establishes notifier subscription first, then fetches one atomic aggregate snapshot containing thread, included turns/items/interactions and `snapshotSeq` from a single database read snapshot. The reducer replaces state, sets `lastAppliedSeq=snapshotSeq`, then replays events `> snapshotSeq`; the HTTP contract/DTO exposes `snapshotSeq`.
3. Reducer ignores `seq <= lastAppliedSeq`, deduplicates chunks and merges terminal item/turn states.
4. Notifier is wake-up only: every wake and transport reconnect queries the configured-database event API again from the committed cursor. Tests cover events committed between subscribe, snapshot and replay.
5. Device offline leaves historical items readable and shows command status `waiting_for_device`; command intent remains durable in `agent_commands`.
6. Profile restart/schema mismatch shows runtime banner and resource staleness; no speculative replay from browser state occurs.

The reducer never observes WSS `bridge_seq` or `server_seq`; those are Bridge delivery cursors. Web order uses only `thread_seq`/`AgentEvent.seq`.

## 9. Accessibility and responsive behavior

- Sidebar, toolbar, timeline, composer, interaction drawer and inspector have semantic landmarks and visible focus treatment.
- Icon controls provide text alternative and tooltip. Toggle state uses `aria-pressed`/`aria-expanded`; disabled reason is exposed to assistive technology.
- Timeline uses a restrained live region: streaming chunks coalesce before announcement, terminal/error/interaction events announce once.
- Interaction drawer traps focus while a blocking response is active, returns focus to invoking timeline item on resolution, and presents deadline in text.
- Command output, diffs and raw inspector are keyboard-scrollable regions with labels. Color never carries status alone.
- On mobile, inspector and resources open as sheets, timeline remains the primary reading order, and controls retain 44px minimum touch target.

## 10. Acceptance scenarios

1. `/agent` static export opens launcher; selecting a thread produces `/agent?thread_id=<public_id>` and direct reload restores it.
2. Existing New Chat command continues to create/navigate chat conversation; Agent link and `NavAgentThreads` do not affect chat providers.
3. A ready Codex profile creates thread, starts turn, streams text/reasoning/command/diff, and reaches item/turn terminal state in correct order.
4. A command approval and user input each survive refresh, accept one response, and resume the same provider turn. A no-turn MCP elicitation survives refresh, accepts one thread-level response, and reaches `serverRequest/resolved` without creating a synthetic turn.
5. Browser disconnect during item delta reconnects from its retained `lastAppliedSeq` with no duplicate chunk or terminal rollback; cold load and gap recovery use the atomic `snapshotSeq` path even when events commit between subscribe, snapshot and replay.
6. Bridge disconnect queues turn/interaction intent, reconnect resends ordered commands with the same command ID after ambiguous write, and cached Bridge result avoids duplicate provider call.
7. Offline, schema mismatch, denied file ref, failed transfer, a revoking device with settlement-only work, and revoked device show actionable state without hiding synchronized history; settlement reconnection stays device-local and Web exposes no new-work controls during revoking. First release does not offer a force-terminal action for unsettled local provider work.
8. Keyboard-only and screen-reader walkthrough covers navigation, composer, interaction response, diff inspector and resource sheets.
9. Browser attempts to create local-auth/config/skill/MCP OAuth or credential mutation commands are rejected before `AgentCommand` creation; fixtures prove no Browser command changes `config.toml`, auth files, skill roots/config, MCP credentials, environment, or keychain.
