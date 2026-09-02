#!/bin/sh
# shellcheck shell=sh
set -eu

server=""
user=""
workspace=""
name=""
codex="codex"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --server) server="$2"; shift 2 ;;
    --user) user="$2"; shift 2 ;;
    --workspace) workspace="$2"; shift 2 ;;
    --name) name="$2"; shift 2 ;;
    --codex) codex="$2"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done
[ -n "$server" ] && [ -n "$user" ] || {
  echo "usage: install.sh --server URL --user PUBLIC_ID [--workspace PATH]" >&2
  exit 2
}

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) asset="deeix-agent-linux-x64"; service="systemd"; checksum="sha256sum" ;;
  Darwin-arm64) asset="deeix-agent-macos-arm64"; service="launchd"; checksum="shasum" ;;
  *) echo "This operating system package is not published." >&2; exit 1 ;;
esac

base="${DEEIX_AGENT_RELEASE_BASE:-${server%/}/agent/releases/current}"
if [ "$(uname -s)" = "Darwin" ]; then
  data_dir="${DEEIX_AGENT_DATA_DIR:-$HOME/Library/Application Support/DEEIX/Agent}"
else
  data_dir="${DEEIX_AGENT_DATA_DIR:-$HOME/.local/share/deeix-agent}"
fi
install_dir="${DEEIX_AGENT_HOME:-$data_dir/bin}"
bin_dir="${DEEIX_AGENT_BIN:-$HOME/.local/bin}"
installed="$install_dir/deeix-agent"
temporary="$(mktemp -d)"
download="$temporary/$asset"
backup="$installed.previous"
config_path="$data_dir/config.json"
config_backup="$temporary/config.json.previous"
had_config=0
trap 'rm -rf "$temporary"' EXIT INT TERM

if [ -f "$config_path" ]; then
  cp -p "$config_path" "$config_backup"
  had_config=1
fi

curl -fL --retry 3 "$base/$asset" -o "$download"
curl -fL --retry 3 "$base/$asset.sha256" -o "$download.sha256"
if [ "$checksum" = "sha256sum" ]; then
  (cd "$temporary" && sha256sum -c "$asset.sha256")
else
  (cd "$temporary" && shasum -a 256 -c "$asset.sha256")
fi
chmod 755 "$download"

set -- install --server "$server" --user "$user" --codex "$codex" --data-dir "$data_dir"
[ -z "$workspace" ] || set -- "$@" --workspace "$workspace"
[ -z "$name" ] || set -- "$@" --name "$name"
"$download" "$@"

start_service() {
  if [ "$service" = "systemd" ]; then
    service_dir="$HOME/.config/systemd/user"
    mkdir -p "$service_dir" || return 1
    cat > "$service_dir/deeix-agent.service" <<EOF
[Unit]
Description=DEEIX Agent
After=network-online.target

[Service]
ExecStart="$installed" start --data-dir "$data_dir"
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF
    systemctl --user daemon-reload || return 1
    systemctl --user enable --now deeix-agent.service || return 1
  else
    launch_dir="$HOME/Library/LaunchAgents"
    plist="$launch_dir/com.deeix.agent.plist"
    mkdir -p "$launch_dir" || return 1
    installed_xml=$(printf '%s' "$installed" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g')
    data_dir_xml=$(printf '%s' "$data_dir" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g')
    cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.deeix.agent</string>
<key>ProgramArguments</key><array><string>$installed_xml</string><string>start</string><string>--data-dir</string><string>$data_dir_xml</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>StandardOutPath</key><string>$data_dir_xml/service.log</string>
<key>StandardErrorPath</key><string>$data_dir_xml/service.log</string>
</dict></plist>
EOF
    launchctl bootstrap "gui/$(id -u)" "$plist" || return 1
  fi
}

stop_service() {
  if [ "$service" = "systemd" ]; then
    systemctl --user stop deeix-agent.service >/dev/null 2>&1 || true
  else
    launchctl bootout "gui/$(id -u)/com.deeix.agent" >/dev/null 2>&1 || true
  fi
}

restore_config() {
  if [ "$had_config" -eq 0 ]; then
    rm -f "$config_path"
    return 0
  fi
  cp -p "$config_backup" "$config_path.rollback" || return 1
  mv -f "$config_path.rollback" "$config_path"
}

had_binary=0
replacement_started=0
link_replaced=0
service_attempted=0
[ ! -f "$installed" ] || had_binary=1
rollback() {
  rollback_failed=0
  stop_service
  restore_config || rollback_failed=1
  if [ "$had_binary" -eq 1 ]; then
    if [ -f "$backup" ]; then
      rm -f "$installed" || rollback_failed=1
      mv "$backup" "$installed" || rollback_failed=1
    fi
    if [ "$rollback_failed" -eq 0 ]; then
      start_service >/dev/null 2>&1 || rollback_failed=1
    fi
  else
    if [ "$replacement_started" -eq 1 ]; then rm -f "$installed" || rollback_failed=1; fi
    if [ "$service_attempted" -eq 1 ]; then
      if [ "$service" = "systemd" ]; then
        systemctl --user disable deeix-agent.service >/dev/null 2>&1 || true
        rm -f "$HOME/.config/systemd/user/deeix-agent.service" || rollback_failed=1
        systemctl --user daemon-reload >/dev/null 2>&1 || rollback_failed=1
      else
        rm -f "$HOME/Library/LaunchAgents/com.deeix.agent.plist" || rollback_failed=1
      fi
    fi
    if [ "$link_replaced" -eq 1 ]; then rm -f "$bin_dir/deeix-agent" || rollback_failed=1; fi
  fi
  [ "$rollback_failed" -eq 0 ]
}

mkdir -p "$install_dir" "$bin_dir"
rm -f "$backup"
stop_service
if [ "$had_binary" -eq 1 ] && ! mv "$installed" "$backup"; then
  if ! rollback; then echo "DEEIX Agent rollback failed" >&2; fi
  echo "DEEIX Agent executable backup failed" >&2
  exit 1
fi
if ! mv "$download" "$installed"; then
  if ! rollback; then echo "DEEIX Agent rollback failed" >&2; fi
  echo "DEEIX Agent executable installation failed" >&2
  exit 1
fi
replacement_started=1
if ! chmod 755 "$installed" || ! ln -sf "$installed" "$bin_dir/deeix-agent"; then
  if ! rollback; then echo "DEEIX Agent rollback failed" >&2; fi
  echo "DEEIX Agent command installation failed" >&2
  exit 1
fi
link_replaced=1
if ! rm -f "$data_dir/runtime-status.json"; then
  if ! rollback; then echo "DEEIX Agent rollback failed" >&2; fi
  echo "DEEIX Agent runtime status reset failed" >&2
  exit 1
fi
service_attempted=1
if start_service; then
  attempts=0
  connected_seconds=0
  while [ "$attempts" -lt 120 ] && [ "$connected_seconds" -lt 20 ]; do
    attempts=$((attempts + 1))
    sleep 1
    if [ -f "$data_dir/runtime-status.json" ] && grep -Eq '"state"[[:space:]]*:[[:space:]]*"connected"' "$data_dir/runtime-status.json"; then
      connected_seconds=$((connected_seconds + 1))
    else
      connected_seconds=0
    fi
  done
  if [ "$connected_seconds" -lt 20 ]; then
    detail="no runtime log was written"
    [ ! -f "$data_dir/agent.log" ] || detail=$(tail -n 1 "$data_dir/agent.log")
    if ! rollback; then echo "DEEIX Agent rollback failed" >&2; fi
    echo "DEEIX Agent service did not connect: $detail" >&2
    exit 1
  fi
  rm -f "$backup"
else
  if ! rollback; then echo "DEEIX Agent rollback failed" >&2; fi
  echo "DEEIX Agent service installation failed" >&2
  exit 1
fi
echo "DEEIX Agent is installed and running: $installed"
