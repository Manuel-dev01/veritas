#!/usr/bin/env bash
# Reset the local dev chain: stop EVERYTHING (gateway + node + plugin + pty), fund genesis, wipe the
# state DB so it re-bootstraps from height 1. Keeps config/genesis/keystore/validator_key.
#
# It deliberately does NOT relaunch — nohup&disown does not survive the wsl.exe session teardown, so the
# node + gateway must be started as PERSISTENT foreground processes. Run this, then relaunch:
#   bash ~/veritas/reset-clean.sh
#   ( cd ~/veritas/node && python3 ~/veritas/pty-run.py 0 ~/veritas/node-run.log )   # node, keep running
#   ( cd ~/veritas && ./veritas serve -port 8080 )                                    # gateway, keep running
# The gateway re-seeds + funds the demo identities on startup; POST /api/seed for the demo scenario.
set -e
echo "stopping gateway + node + plugin + pty driver..."
pkill -x veritas    || true
pkill -f pty-run.py || true     # safe: this script's argv is "bash reset-clean.sh", not the pattern
pkill -x canopy     || true
pkill -x go-plugin  || true
sleep 4
if pgrep -x canopy >/dev/null; then echo "ERROR: canopy still running — aborting wipe (would race)"; exit 1; fi
echo "canopy stopped (pgrep clean)."

echo "funding genesis account(s)..."
python3 - <<'PY'
import json, os
p = os.path.expanduser("~/.canopy/genesis.json")
g = json.load(open(p))
for a in g.get("accounts", []):
    a["amount"] = 10_000_000_000
json.dump(g, open(p, "w"), indent=2)
print("genesis accounts funded:", [(a.get("address"), a["amount"]) for a in g.get("accounts", [])])
PY

echo "wiping state DB (~/.canopy/canopy)..."
rm -rf ~/.canopy/canopy
echo "RESET DONE — chain will re-bootstrap at height 1 when the node relaunches."
