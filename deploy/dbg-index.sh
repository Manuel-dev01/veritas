#!/usr/bin/env bash
set -u
G=localhost:8080
echo "=== /api/state now ==="
curl -s $G/api/state | python3 -c "import sys,json;d=json.load(sys.stdin);print('height',d['height'],'claims',len(d['claims']),'log',len(d['log']))"
H=$(curl -s -X POST localhost:50002/v1/query/height -d '{}' | python3 -c "import sys,json;print(json.load(sys.stdin)['height'])")
echo "chain height=$H"
echo "=== scan events-by-height across the chain for claim_created ==="
for h in $(seq 1 $H); do
  n=$(curl -s -X POST localhost:50002/v1/query/events-by-height -d "{\"height\":$h,\"perPage\":50}" | python3 -c "import sys,json;d=json.load(sys.stdin);r=d.get('results') or [];print(len(r))" 2>/dev/null)
  if [ "${n:-0}" != "0" ]; then echo "  height $h: $n events"; fi
done
echo "=== raw events at a couple heights (look for claim_created) ==="
curl -s -X POST localhost:50002/v1/query/events-by-height -d "{\"height\":3,\"perPage\":50}" | head -c 600; echo
