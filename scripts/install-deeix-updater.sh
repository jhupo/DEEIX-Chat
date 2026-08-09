#!/usr/bin/env bash
set -euo pipefail

deployment_arg=${1:?usage: install-deeix-updater.sh /absolute/deployment owner/repo [loopback-app-url]}
repository=${2:?repository is required}
app_url=${3:-http://127.0.0.1:8080}
case "$deployment_arg" in /*) ;; *) echo "deployment directory must be absolute" >&2; exit 2;; esac
case "$deployment_arg$repository$app_url" in *[$'\n\r\t ']* ) echo "arguments contain whitespace" >&2; exit 2;; esac
[[ "$deployment_arg" =~ ^/[A-Za-z0-9._/-]+$ && "$deployment_arg" != *".."* && "$deployment_arg" != *"&"* && "$deployment_arg" != *"|"* && "$deployment_arg" != *\\* ]] || { echo "deployment directory contains unsupported characters" >&2; exit 2; }
[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "repository must be owner/repo" >&2; exit 2; }
[[ "$app_url" =~ ^https?://(127\.0\.0\.1|localhost)(:([0-9]{1,5}))?$ ]] || { echo "app URL must be loopback http(s)" >&2; exit 2; }
if [[ -n "${BASH_REMATCH[3]:-}" ]]; then ((BASH_REMATCH[3] >= 1 && BASH_REMATCH[3] <= 65535)) || { echo "invalid app port" >&2; exit 2; }; fi
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 2; }
command -v systemctl >/dev/null
docker compose version >/dev/null

deployment_dir=$(realpath -e -- "$deployment_arg")
[[ "$deployment_dir" =~ ^/[A-Za-z0-9._/-]+$ && "$deployment_dir" != *".."* && "$deployment_dir" != *"&"* && "$deployment_dir" != *"|"* && "$deployment_dir" != *\\* ]] || { echo "canonical deployment directory contains unsupported characters" >&2; exit 2; }
[[ -d "$deployment_dir" && -f "$deployment_dir/docker-compose.full.yml" && ! -L "$deployment_dir/docker-compose.full.yml" ]] || { echo "missing regular docker-compose.full.yml" >&2; exit 2; }
compose_file=$(realpath -e -- "$deployment_dir/docker-compose.full.yml")
[[ "$compose_file" == "$deployment_dir/"* ]] || { echo "compose file escapes deployment directory" >&2; exit 2; }
env_file="$deployment_dir/.env"
if [[ -e "$env_file" ]]; then [[ -f "$env_file" && ! -L "$env_file" ]] || { echo "invalid .env file" >&2; exit 2; }; else install -m 0600 /dev/null "$env_file"; fi
env_file=$(realpath -e -- "$env_file")
[[ "$env_file" == "$deployment_dir/"* ]] || { echo "env file escapes deployment directory" >&2; exit 2; }

bundle_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
binary="$bundle_dir/deeix-updater"
service_template="$bundle_dir/deeix-updater.service"
[[ -f "$binary" && ! -L "$binary" && -x "$binary" && -f "$service_template" && ! -L "$service_template" ]] || { echo "invalid updater bundle" >&2; exit 2; }

install -d -m 0750 /var/lib/deeix-updater /run/deeix-updater /etc/deeix-updater /usr/local/lib/deeix-updater
install -m 0750 "$binary" /usr/local/lib/deeix-updater/deeix-updater.new
mv -f /usr/local/lib/deeix-updater/deeix-updater.new /usr/local/lib/deeix-updater/deeix-updater
sed "s|__DEPLOYMENT_DIR__|$deployment_dir|g" "$service_template" > /etc/systemd/system/deeix-updater.service.new
chmod 0644 /etc/systemd/system/deeix-updater.service.new
mv -f /etc/systemd/system/deeix-updater.service.new /etc/systemd/system/deeix-updater.service
umask 077
cat > /etc/deeix-updater/deeix-updater.env.new <<EOF
DEEIX_UPDATER_REPOSITORY=$repository
DEEIX_UPDATER_SOCKET_PATH=/run/deeix-updater/deeix-updater.sock
DEEIX_UPDATER_STATE_FILE=/var/lib/deeix-updater/journal.json
DEEIX_UPDATER_DEPLOYMENT_DIR=$deployment_dir
DEEIX_UPDATER_COMPOSE_FILE=$deployment_dir/docker-compose.full.yml
DEEIX_UPDATER_ENV_FILE=$env_file
DEEIX_UPDATER_APP_BASE_URL=$app_url
EOF
mv -f /etc/deeix-updater/deeix-updater.env.new /etc/deeix-updater/deeix-updater.env
systemctl daemon-reload
systemctl enable deeix-updater.service
systemctl restart deeix-updater.service
systemctl --no-pager --full status deeix-updater.service || true
printf 'Updater socket: %s\n' /run/deeix-updater/deeix-updater.sock
