#!/usr/bin/env bash
# Reset the local dev chain to a fresh height-0 state funded generously, keeping the same validator
# key/genesis identity. Wipes ONLY the state DB (~/.canopy/canopy); config/genesis/keystore persist.
# Run by filename inside WSL:  bash ~/veritas/reset-chain.sh
set -e

echo "stopping node + plugin + pty driver..."
pkill -f pty-run.py || true
pkill -x canopy || true
pkill -x go-plugin || true
sleep 3

echo "funding the genesis account(s) generously..."
python3 - <<'PY'
import json, os
p = os.path.expanduser("~/.canopy/genesis.json")
g = json.load(open(p))
for a in g.get("accounts", []):
    a["amount"] = 10_000_000_000  # 10B uCNPY, plenty for many demo accounts + fees
json.dump(g, open(p, "w"), indent=2)
print("genesis accounts ->", g["accounts"])
PY

echo "wiping state DB (~/.canopy/canopy) so the chain re-bootstraps from genesis..."
rm -rf ~/.canopy/canopy

echo "relaunching node from genesis..."
cd "$HOME/veritas/node"
nohup python3 "$HOME/veritas/pty-run.py" 0 "$HOME/veritas/node-run.log" >/dev/null 2>&1 &
disown
echo "relaunched (pty pid $!)"
