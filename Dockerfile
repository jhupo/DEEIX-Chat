# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim AS frontend-builder

WORKDIR /src

ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0

ARG NEXT_PUBLIC_API_BASE_URL=""
ENV NEXT_PUBLIC_API_BASE_URL=${NEXT_PUBLIC_API_BASE_URL}

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json ./frontend/package.json
COPY backend/package.json ./backend/package.json
COPY packages/api-contract/package.json ./packages/api-contract/package.json
COPY frontend/scripts ./frontend/scripts
COPY frontend/public/pwa ./frontend/public/pwa

RUN corepack enable

RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store \
    && pnpm install --frozen-lockfile --prefer-offline --filter @deeix/web

COPY VERSION /src/VERSION
COPY scripts /src/scripts
COPY frontend ./frontend
COPY packages/api-contract ./packages/api-contract

WORKDIR /src/frontend

# 如果你的 Next 版本支持，可以在 next.config 里开启 turbopack build filesystem cache
RUN --mount=type=cache,id=next-cache,target=/src/frontend/.next/cache \
    pnpm build


FROM golang:1.26.5-bookworm AS backend-builder

WORKDIR /src/backend

ARG GIT_COMMIT=unknown
ARG BUILD_TIME=""
COPY VERSION /src/VERSION
COPY backend/go.mod backend/go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY backend ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    VERSION="$(cat /src/VERSION)" \
    && if [ -z "${BUILD_TIME}" ]; then BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"; fi \
    && CGO_ENABLED=0 \
       go build -trimpath \
       -ldflags="-s -w -X github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/buildinfo.Version=${VERSION} -X github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/buildinfo.Commit=${GIT_COMMIT} -X github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/buildinfo.BuildTime=${BUILD_TIME}" \
       -o /out/deeix-chat ./cmd/server

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    VERSION="$(cat /src/VERSION)" \
    && AGENT_DIR=/out/agent/releases/current \
    && mkdir -p "$AGENT_DIR" \
    && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_COMMIT}" -o "$AGENT_DIR/deeix-agent-windows-x64.exe" ./cmd/deeix-agent \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_COMMIT}" -o "$AGENT_DIR/deeix-agent-linux-x64" ./cmd/deeix-agent \
    && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_COMMIT}" -o "$AGENT_DIR/deeix-agent-macos-arm64" ./cmd/deeix-agent \
    && cd "$AGENT_DIR" \
    && sha256sum deeix-agent-windows-x64.exe > deeix-agent-windows-x64.exe.sha256 \
    && sha256sum deeix-agent-linux-x64 > deeix-agent-linux-x64.sha256 \
    && sha256sum deeix-agent-macos-arm64 > deeix-agent-macos-arm64.sha256


FROM debian:bookworm-slim AS runtime-deps

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/*


FROM debian:bookworm-slim AS runtime

WORKDIR /app

COPY --from=runtime-deps /etc/ssl/certs /etc/ssl/certs
COPY --from=runtime-deps /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=runtime-deps /etc/localtime /etc/localtime
COPY --from=runtime-deps /etc/timezone /etc/timezone
COPY --from=backend-builder /out/deeix-chat /app/image-runtime/deeix-chat
COPY --from=frontend-builder /src/frontend/out /app/image-runtime/frontend/out
COPY --from=backend-builder /out/agent /app/image-runtime/frontend/out/agent
COPY VERSION /app/image-runtime/VERSION
RUN test -s /app/image-runtime/frontend/out/agent/releases/current/deeix-agent-windows-x64.exe \
  && test -s /app/image-runtime/frontend/out/agent/releases/current/deeix-agent-windows-x64.exe.sha256 \
  && test -s /app/image-runtime/frontend/out/agent/releases/current/deeix-agent-linux-x64 \
  && test -s /app/image-runtime/frontend/out/agent/releases/current/deeix-agent-linux-x64.sha256 \
  && test -s /app/image-runtime/frontend/out/agent/releases/current/deeix-agent-macos-arm64 \
  && test -s /app/image-runtime/frontend/out/agent/releases/current/deeix-agent-macos-arm64.sha256
RUN find /app/image-runtime -type f -print0 \
  | sort -z \
  | xargs -0 sha256sum \
  | sha256sum \
  | cut -d ' ' -f 1 > /tmp/deeix-image-digest \
  && mv /tmp/deeix-image-digest /app/image-runtime/IMAGE_DIGEST
COPY --chmod=0755 deploy/docker-entrypoint.sh /usr/local/bin/deeix-chat-entrypoint
COPY LICENSE NOTICE /app/licenses/DEEIX-Chat/

ENV FRONTEND_DIST_DIR=/app/runtime/current/frontend/out

EXPOSE 8080

VOLUME ["/app/storage", "/app/data", "/app/runtime"]

ENTRYPOINT ["/usr/local/bin/deeix-chat-entrypoint"]
