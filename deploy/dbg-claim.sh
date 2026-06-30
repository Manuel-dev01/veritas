#!/usr/bin/env bash
set -u
YOU=$(curl -s localhost:8080/api/identities | python3 -c 'import sys,json; d=json.load(sys.stdin); print(next(i["address"] for i in d if i["id"]=="you"))')
echo "you=$YOU"
echo "=== raw claim_created events for you (text field?) ==="
~/veritas/veritas query -address "$YOU" -event claim --json
