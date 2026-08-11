# Full Deployment And Online Update

DEEIX Chat has one deployment profile: application, PostgreSQL with pgvector, and Redis from the root `compose.yaml`. SQLite, memory-only cache, Lite deployment, and a host updater are not deployment options.

## Initial Full Deployment

```bash
cp config.example.yaml config.yaml
cat > .env <<'EOF'
DEEIX_BIND_ADDRESS=0.0.0.0
DEEIX_HTTP_PORT=50001
SUB2_BASE_URL=https://api.ovload.com
EOF
docker compose -f compose.yaml up -d
```

The application container uses `restart: unless-stopped`. PostgreSQL and Redis must pass their health checks before the application starts.

`SUB2_BASE_URL` is the only Sub2 deployment setting and defaults to `https://api.ovload.com`. It must be a canonical HTTP(S) origin without credentials, path, query, or fragment; production requires HTTPS. The application derives its internal Sub2 instance fingerprint from this origin.

## v0.4 Clean-Slate Upgrade

Before any v0.4 schema mutation, startup checks for populated legacy identity schema. A populated legacy identity table or data that cannot satisfy the Sub2 Principal shape causes the upgrade to stop before it changes the schema. Back up the existing PostgreSQL instance, then deploy v0.4 with a fresh PostgreSQL database or volume.

This is a clean-slate release: there is no data migration, compatibility layer, old-login continuity, or old-chat-history continuity. SQLite, Lite, and old local billing deployment paths remain unsupported.

A fresh v0.4 deployment leaves `REDIS_DB` unset and uses logical database `0`. When cutting over against a Redis instance that still contains the legacy cache, operators may select an unused logical database in `.env`, for example `REDIS_DB=1`, instead of flushing those keys.

## Persistent Data And Runtime

| Named volume | Container path | Ownership |
| --- | --- | --- |
| `deeix-chat-app-storage` | `/app/storage` | Uploaded and generated files |
| `deeix-chat-app-data` | `/app/data` | Application files and update journal |
| `deeix-chat-app-runtime` | `/app/runtime` | Active and previously installed application releases |
| `deeix-chat-postgres-data` | `/var/lib/postgresql/data` | PostgreSQL business data and migrations |
| `deeix-chat-redis-data` | `/data` | Redis cache and wake state |

An online update writes only `deeix-chat-app-runtime` and the journal in `deeix-chat-app-data`. It does not recreate PostgreSQL, flush Redis, replace `config.yaml`, or touch uploaded files.

The image contains a baseline release under `/app/image-runtime`. The entrypoint seeds that release into the runtime volume on first boot and atomically selects `/app/runtime/current`. If a later container image contains a version newer than the active runtime release, it seeds and selects the image version. Recreating the container with the same named volume preserves an application release installed online.

## Online Update Flow

The Superadmin About page uses these endpoints:

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/admin/update/status` | Read running version, candidate, and latest job |
| `POST` | `/api/v1/admin/update/check` | Fetch and validate the latest release manifest |
| `POST` | `/api/v1/admin/update/install` | Download, verify, extract, and activate a release |
| `GET` | `/api/v1/admin/update/jobs/:job_id` | Poll durable job progress |
| `POST` | `/api/v1/admin/update/restart` | Exit after the HTTP response so Docker restarts into the active release |

All endpoints require the exact `superadmin` role. Install requests require an `Idempotency-Key` plus exact version, manifest digest, and confirmation string. Actor identity and request ID come from the authenticated server context and are journaled.

Jobs progress through `queued`, `pulling`, `applying`, then `succeeded`, `failed`, or `outcome_unknown`. A process restart during a nonterminal job records `outcome_unknown`. Installation success means the bundle is active on disk; the UI then offers **Restart and apply**, waits for the target version to answer, and reloads.

## Release Contract

The only feed is:

```text
https://github.com/<repository>/releases/latest/download/update-manifest.json
```

Schema v2 fixes the repository, stable `vMAJOR.MINOR.PATCH` tag/version, 40-character commit, UTC publication time, canonical GitHub release URL, and one application bundle per Linux architecture. Each bundle entry includes the canonical GitHub Release asset URL, exact byte size, and `sha256:` digest.

The updater selects only `linux/amd64` or `linux/arm64` matching the running process. It limits manifest, archive, and extracted sizes; verifies the exact byte count and SHA-256; rejects absolute paths, traversal, links, devices, and other special archive entries; requires `VERSION`, `deeix-chat`, and `frontend/out/index.html`; and atomically switches the `current` symlink only after validation.

No Docker socket, Compose command, host credential, or registry credential is available to the application. Online update does not rebuild an image on the server.

## Update Proxy

`UPDATE_PROXY_URL` or `update.proxy_url` accepts a forward proxy URL using `http`, `https`, `socks5`, or `socks5h`:

```dotenv
UPDATE_PROXY_URL=http://127.0.0.1:7890
```

The proxy is applied to both the GitHub manifest request and Release bundle download. A URL-prefix download mirror is a different mechanism and is not accepted as this setting. Leave the value empty for a direct connection.

## Tag Workflow

A stable tag must equal `v$(cat VERSION)`. The release job builds the static frontend once, compiles the server for Linux amd64 and arm64, and publishes:

```text
deeix-chat-linux-amd64.tar.gz
deeix-chat-linux-amd64.tar.gz.sha256
deeix-chat-linux-arm64.tar.gz
deeix-chat-linux-arm64.tar.gz.sha256
update-manifest.json
update-manifest.json.sha256
```

Each application archive has the same top-level layout expected by the updater: `VERSION`, `deeix-chat`, and `frontend/out`.

## One-Time Migration From The Host Updater

Deploy the first image containing this architecture with the current `compose.yaml`:

```bash
docker compose -f compose.yaml pull app
docker compose -f compose.yaml up -d app
```

Verify `/readyz`, `/api/v1/version`, a new Sub2-backed login, PostgreSQL, Redis, and uploaded files. Do not expect legacy login or chat-history continuity after the v0.4 fresh-database cutover. After that verification, remove the old host updater service and socket mount. Future stable application releases use the admin online update flow.

There is no automatic database rollback. After the v0.4 clean-slate cutover, operators still need normal PostgreSQL backups before later releases that introduce schema migrations.
