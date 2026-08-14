#!/bin/sh
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
[ -n "$server" ] && [ -n "$user" ] && [ -n "$workspace" ] || {
  echo "usage: install.sh --server URL --user PUBLIC_ID --workspace PATH" >&2
  exit 2
}

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) asset="deeix-agent-linux-x64"; service="systemd" ;;
  Darwin-arm64) asset="deeix-agent-macos-arm64"; service="launchd" ;;
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
trap 'rm -rf "$temporary"' EXIT INT TERM

curl -fL --retry 3 "$base/$asset" -o "$download"
curl -fL --retry 3 "$base/$asset.sha256" -o "$download.sha256"
(cd "$temporary" && shasum -a 256 -c "$asset.sha256")
chmod 755 "$download"

set -- install --server "$server" --user "$user" --workspace "$workspace" --codex "$codex" --data-dir "$data_dir"
[ -z "$name" ] || set -- "$@" --name "$name"
"$download" "$@"

if [ "$service" = "systemd" ]; then
  systemctl --user stop deeix-agent.service >/dev/null 2>&1 || true
else
  launchctl bootout "gui/$(id -u)/com.deeix.agent" >/dev/null 2>&1 || true
fi
mkdir -p "$install_dir" "$bin_dir"
rm -f "$backup"
[ ! -f "$installed" ] || mv "$installed" "$backup"
if mv "$download" "$installed"; then
  chmod 755 "$installed"
else
  [ ! -f "$backup" ] || mv "$backup" "$installed"
  exit 1
fi
ln -sf "$installed" "$bin_dir/deeix-agent"

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

rm -f "$data_dir/runtime-status.json"
rollback() {
  if [ "$service" = "systemd" ]; then
    systemctl --user stop deeix-agent.service >/dev/null 2>&1 || true
  else
    launchctl bootout "gui/$(id -u)/com.deeix.agent" >/dev/null 2>&1 || true
  fi
  rm -f "$installed"
  if [ -f "$backup" ]; then
    mv "$backup" "$installed"
    start_service >/dev/null 2>&1 || true
  fi
}

if start_service; then
  attempts=0
  while [ "$attempts" -lt 15 ] && [ ! -f "$data_dir/runtime-status.json" ]; do
    attempts=$((attempts + 1))
    sleep 1
  done
  if [ ! -f "$data_dir/runtime-status.json" ]; then rollback; exit 1; fi
  rm -f "$backup"
else
  rollback
  exit 1
fi
echo "DEEIX Agent is installed and running: $installed"
