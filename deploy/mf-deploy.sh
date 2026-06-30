#!/usr/bin/env bash
# Deploy the freshly cross-compiled MF plugin + CLI, wipe state, and relaunch from genesis.
# MUST be run by filename (bash ~/veritas/mf-deploy.sh) so this process's cmdline is "bash
# mf-deploy.sh" — that keeps the `pty-run.py` pattern out of the caller's argv, avoiding the
# pkill -f self-kill trap.
set -e
REPO="/mnt/c/Users/DELL 5420/Desktop/hackathons/veritas"

echo "stopping node + plugin + pty driver..."
pkill -f pty-run.py || true
pkill -x canopy || true
pkill -x go-plugin || true
sleep 3

echo "deploying MF binaries..."
cp "$REPO/plugin/go/go-plugin" "$HOME/veritas/node/plugin/go/go-plugin"
chmod +x "$HOME/veritas/node/plugin/go/go-plugin"
cp "$REPO/cli/veritas" "$HOME/veritas/veritas"
chmod +x "$HOME/veritas/veritas"
echo "  deployed plugin MF-symbol count: $(grep -a -c note_intercept "$HOME/veritas/node/plugin/go/go-plugin")"
echo "  deployed CLI   MF-symbol count: $(grep -a -c noteIntercept "$HOME/veritas/veritas")"

echo "wiping state DB so the chain re-bootstraps from genesis..."
rm -rf "$HOME/.canopy/canopy"

echo "relaunching node from genesis..."
cd "$HOME/veritas/node"
nohup python3 "$HOME/veritas/pty-run.py" 0 "$HOME/veritas/node-run.log" >/dev/null 2>&1 &
disown
echo "relaunched (pty pid $!)"
