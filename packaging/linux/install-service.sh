#!/usr/bin/env bash
set -euo pipefail
BIN="${1:-}"
if [ -z "$BIN" ]; then
  arch="$(dpkg --print-architecture 2>/dev/null || uname -m)"
  [ "$arch" = "x86_64" ] && arch=amd64
  [ "$arch" = "aarch64" ] && arch=arm64
  tmp="$(mktemp)"
  curl -fsSL "https://github.com/cyrstrstn/Portivane/releases/latest/download/portivane-linux-${arch}.tar.gz" -o "$tmp"
  mkdir -p /usr/bin
  tar -xzf "$tmp" -C /usr/bin
  rm -f "$tmp"
else
  install -Dm755 "$BIN" /usr/bin/portivane
fi
unit="$(dirname "$0")/portivane.service"
if [ ! -f "$unit" ]; then
  unit="$(mktemp)"
  curl -fsSL https://raw.githubusercontent.com/cyrstrstn/Portivane/main/packaging/linux/portivane.service -o "$unit"
fi
install -Dm644 "$unit" /etc/systemd/system/portivane.service
systemctl daemon-reload
systemctl enable --now portivane.service
echo "Portivane is running at http://127.0.0.1:4747"
