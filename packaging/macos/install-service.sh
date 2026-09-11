#!/usr/bin/env bash
set -euo pipefail
arch="$(uname -m)"; [ "$arch" = "x86_64" ] && arch=amd64; [ "$arch" = "arm64" ] && arch=arm64
app="$HOME/Applications/Portivane.app"
mkdir -p "$HOME/Applications" "$HOME/Library/LaunchAgents" "$app/Contents/MacOS"
tmp="$(mktemp -d)"
curl -fsSL "https://github.com/cyrstrstn/Portivane/releases/latest/download/portivane-macos-${arch}.dmg" -o "$tmp/portivane.dmg"
hdiutil attach "$tmp/portivane.dmg" -nobrowse -mountpoint "$tmp/mnt" >/dev/null
ditto "$tmp/mnt/Portivane.app" "$app"
hdiutil detach "$tmp/mnt" >/dev/null
cat > "$HOME/Library/LaunchAgents/com.portivane.agent.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Label</key><string>com.portivane.agent</string><key>ProgramArguments</key><array><string>$app/Contents/MacOS/portivane</string></array><key>EnvironmentVariables</key><dict><key>PORTIVANE_SERVICE</key><string>1</string><key>PORTIVANE_NO_BROWSER</key><string>1</string></dict><key>RunAtLoad</key><true/><key>KeepAlive</key><true/></dict></plist>
PLIST
launchctl unload "$HOME/Library/LaunchAgents/com.portivane.agent.plist" 2>/dev/null || true
launchctl load "$HOME/Library/LaunchAgents/com.portivane.agent.plist"
open "$app"
echo 'Portivane installed and running as a background macOS agent.'
