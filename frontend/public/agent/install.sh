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
  Linux-x86_64) platform="linux-x64" ;;
  Darwin-arm64) platform="macos-arm64" ;;
  *) echo "This operating system package is not published." >&2; exit 1 ;;
esac

base="${DEEIX_AGENT_RELEASE_BASE:-${server%/}/agent/releases/current}"
archive="deeix-agent-bridge-$platform.tar.gz"
install_dir="${DEEIX_AGENT_HOME:-$HOME/.local/share/deeix-agent-bridge}"
bin_dir="${DEEIX_AGENT_BIN:-$HOME/.local/bin}"
temporary="$(mktemp -d)"
staged="$install_dir.new.$$"
backup="$install_dir.old.$$"
trap 'rm -rf "$temporary" "$staged"' EXIT INT TERM
curl -fL --retry 3 "$base/$archive" -o "$temporary/$archive"
curl -fL --retry 3 "$base/$archive.sha256" -o "$temporary/$archive.sha256"
(cd "$temporary" && shasum -a 256 -c "$archive.sha256")
mkdir -p "$(dirname "$install_dir")" "$bin_dir" "$staged"
tar -xzf "$temporary/$archive" -C "$staged"

set -- install --server "$server" --user "$user" --workspace "$workspace" --codex "$codex"
[ -z "$name" ] || set -- "$@" --name "$name"
"$staged/deeix-agent-bridge" "$@"

if [ "$(uname -s)" = "Linux" ]; then
  systemctl --user stop deeix-agent-bridge.service >/dev/null 2>&1 || true
else
  launchctl bootout "gui/$(id -u)/com.deeix.agent-bridge" >/dev/null 2>&1 || true
fi
if [ -d "$install_dir" ]; then mv "$install_dir" "$backup"; fi
if mv "$staged" "$install_dir"; then
  :
else
  if [ -d "$backup" ]; then mv "$backup" "$install_dir"; fi
  exit 1
fi
ln -sf "$install_dir/deeix-agent-bridge" "$bin_dir/deeix-agent-bridge"

start_service() {
  if [ "$(uname -s)" = "Linux" ]; then
    service_dir="$HOME/.config/systemd/user"
    mkdir -p "$service_dir" || return 1
    cat > "$service_dir/deeix-agent-bridge.service" <<EOF
[Unit]
Description=DEEIX Agent Bridge
After=network-online.target

[Service]
ExecStart=$bin_dir/deeix-agent-bridge start
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF
    systemctl --user daemon-reload || return 1
    systemctl --user enable --now deeix-agent-bridge.service || return 1
  else
    launch_dir="$HOME/Library/LaunchAgents"
    label="com.deeix.agent-bridge"
    plist="$launch_dir/$label.plist"
    mkdir -p "$launch_dir" || return 1
    cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>$label</string>
<key>ProgramArguments</key><array><string>$bin_dir/deeix-agent-bridge</string><string>start</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>StandardOutPath</key><string>$HOME/Library/Logs/deeix-agent-bridge.log</string>
<key>StandardErrorPath</key><string>$HOME/Library/Logs/deeix-agent-bridge.log</string>
</dict></plist>
EOF
    launchctl bootout "gui/$(id -u)/$label" >/dev/null 2>&1 || true
    launchctl bootstrap "gui/$(id -u)" "$plist" || return 1
  fi
}

if start_service; then
  rm -rf "$backup"
else
  rm -rf "$install_dir"
  if [ -d "$backup" ]; then
    mv "$backup" "$install_dir"
    start_service >/dev/null 2>&1 || true
  fi
  exit 1
fi
echo "DEEIX Agent Bridge is installed and running."
