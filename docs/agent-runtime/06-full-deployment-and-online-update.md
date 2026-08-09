# Full Deployment And Online Update

| Area | v1 status | Later hardening |
| --- | --- | --- |
| Host updater | Implemented: systemd process, Unix socket, journal and lock | Updater self-update |
| Release trust | Fixed GitHub repository, HTTPS, strict manifest and immutable digest | Signed attestations and verification |
| Install authorization | Exact `superadmin` plus explicit browser confirmation | Step-up authentication and CSRF controls |
| Failure handling | Pull failure is `failed`; post-start failure is `outcome_unknown` | Backup-aware, compatibility-aware rollback |

## Operator Install And Upgrade

For a stable release, download the matching `deeix-updater-linux-amd64.tar.gz` or `deeix-updater-linux-arm64.tar.gz`, verify its `.sha256`, and extract it. Each bundle contains `deeix-updater`, `deeix-updater.service`, and `install-deeix-updater.sh` in one directory. Run:

```bash
sudo ./install-deeix-updater.sh /srv/deeix-chat owner/repo http://127.0.0.1:8080
```

The supplied deployment directory must be canonical and absolute, with regular non-symlink `docker-compose.full.yml` and `.env` files. The installer verifies Docker Compose/systemd/root access, atomically installs the binary and generated systemd unit, writes `/etc/deeix-updater/deeix-updater.env`, then enables and restarts the service. Its state is `/var/lib/deeix-updater/journal.json`; its socket is `/run/deeix-updater/deeix-updater.sock`.

## Release Contract And Boundary

The updater derives the only feed from `https://github.com/<owner/repo>/releases/latest/download/update-manifest.json`. Schema v1 fixes repository, stable `vMAJOR.MINOR.PATCH` tag/version, commit, UTC publication time, release URL, GHCR repository, Linux platforms, and `sha256:` image digest. The manifest digest is SHA-256 over the exact downloaded bytes. It pulls only `ghcr.io/<owner/repo>@sha256:<digest>`.

This is a v1 trust boundary: HTTPS to the configured GitHub repository and strict digest validation. It does not claim signed release verification. The app mounts only `/run/deeix-updater` read-only; it never receives `docker.sock`, a Compose path, registry URL, command, or host credential. The host updater is the only Docker/Compose authority.

## Admin API And UI

The application exposes `GET /api/v1/admin/update/status`, `POST /api/v1/admin/update/check`, `POST /api/v1/admin/update/install`, and `GET /api/v1/admin/update/jobs/:job_id`. Each requires the exact `superadmin` role. Install accepts only `version`, `manifestDigest`, `confirmation`, and required `Idempotency-Key`; actor user ID/name and request ID are derived server-side and journaled.

The About dialog checks through this API, displays current/candidate versions and release notes, then requires a second in-dialog confirmation. It retains one idempotency key per install attempt and polls the durable job across app replacement. There is no browser-to-GitHub update check and no rollback control.

## Journal, Lock, And Crash Semantics

Jobs progress through `queued`, `pulling`, `applying`, `verifying`, then `succeeded`, `failed`, or `outcome_unknown`. A cross-process create-exclusive lock is written before the queued job is durable and released only after a terminal outcome. Restart converts nonterminal jobs to `outcome_unknown` and removes the recovered lock; an orphan lock without a matching job fails closed for operator reconciliation. Idempotency replay returns its recorded job; a changed request or actor binding conflicts.

Before candidate start, a pull failure leaves `.env` untouched. The updater passes the digest-pinned candidate image explicitly to both `docker compose pull app` and `docker compose up -d --no-deps app`, then checks the running app container image exactly matches that candidate. After application start begins, a command/readiness/version/image-verification error remains `outcome_unknown`. No automatic or UI rollback is attempted because application startup currently runs PostgreSQL migrations and there is no manifest compatibility plus verified backup/restore contract.

## Tag Workflow And Verification

Stable tags must exactly equal `v$(cat VERSION)`. The release workflow keeps the existing branch/dev multiarch image flow, resolves the published multiarch digest, builds static Linux amd64/arm64 updater bundles, writes compact `update-manifest.json` plus checksums whose entries use release asset basenames, and creates or updates the GitHub Release assets.

Local disposable-copy verification passed the updater and config suites and the Linux updater cross-build. `gofmt`, `bash -n scripts/install-deeix-updater.sh`, `pnpm --filter @deeix/web check`, and `git diff --check` also passed locally. The exact Go 1.26.5 full backend suite, `docker compose -f docker-compose.full.yml config`, and `actionlint` remain CI/deployment-machine verification because the local toolchain or executables are unavailable.

Future work retains signed attestations, step-up/CSRF protection, backup/restore, compatibility-aware rollback, updater self-update, and the remaining sole-Full-profile deployment cleanup.
