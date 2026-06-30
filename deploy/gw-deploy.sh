#!/usr/bin/env bash
# Deploy freshly cross-compiled plugin + CLI and wipe state — but DO NOT relaunch (the node + gateway
# are relaunched as persistent background tasks by the caller). Run by filename to avoid the pkill -f
# self-kill trap. Usage in WSL: bash ~/veritas/gw-deploy.sh
set -e
REPO="/mnt/c/Users/DELL 5420/Desktop/hackathons/veritas"
echo "stopping node + plugin..."
pkill -x canopy || true
pkill -x go-plugin || true
pkill -f pty-run.py || true
sleep 3
echo "deploying binaries..."
cp "$REPO/plugin/go/go-plugin" "$HOME/veritas/node/plugin/go/go-plugin"; chmod +x "$HOME/veritas/node/plugin/go/go-plugin"
cp "$REPO/cli/veritas" "$HOME/veritas/veritas"; chmod +x "$HOME/veritas/veritas"
echo "wiping state DB..."
rm -rf "$HOME/.canopy/canopy"
echo "deployed + wiped (caller relaunches node + gateway)"
