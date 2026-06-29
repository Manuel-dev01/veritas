#!/usr/bin/env bash
# Deploy freshly cross-compiled binaries into the WSL node dir and restart the node so the
# plugin re-handshakes (re-registering any new tx/event types).
# Run by filename inside WSL:  bash ~/veritas/deploy-restart.sh
# Invoking by filename (not inline) keeps the kill pattern out of the caller's cmdline, avoiding
# the pkill -f self-match trap.
set -e
REPO="/mnt/c/Users/DELL 5420/Desktop/hackathons/veritas"
SRC="$REPO/plugin/go"
CLI="$REPO/cli"

echo "stopping node + plugin + pty driver..."
pkill -f pty-run.py || true
pkill -x canopy || true
pkill -x go-plugin || true
sleep 3

echo "deploying fresh binaries..."
cp "$SRC/go-plugin" "$HOME/veritas/node/plugin/go/go-plugin"
chmod +x "$HOME/veritas/node/plugin/go/go-plugin"
cp "$CLI/veritas" "$HOME/veritas/veritas"   # Phase 5 CLI (replaces veritas-sign)
chmod +x "$HOME/veritas/veritas"

echo "relaunching node..."
cd "$HOME/veritas/node"
nohup python3 "$HOME/veritas/pty-run.py" 0 "$HOME/veritas/node-run.log" >/dev/null 2>&1 &
disown
echo "relaunched (pty pid $!)"
