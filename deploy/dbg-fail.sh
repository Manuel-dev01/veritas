#!/usr/bin/env bash
set -u
YOU=$(curl -s localhost:8080/api/identities | python3 -c 'import sys,json;d=json.load(sys.stdin);print(next(i["address"] for i in d if i["id"]=="you"))')
VAL=d46e9a2042b4f2bdb69362aec7398f2ec623faa8
echo "you=$YOU"
echo "=== you balance ==="
curl -s -X POST localhost:50002/v1/query/account -d "{\"address\":\"$YOU\"}"
echo; echo "=== validator balance ==="
curl -s -X POST localhost:50002/v1/query/account -d "{\"address\":\"$VAL\"}"
echo; echo "=== failed-txs (by you) ==="
curl -s -X POST localhost:50002/v1/query/failed-txs -d "{\"address\":\"$YOU\",\"perPage\":20}"
echo; echo "=== failed-txs (by validator) ==="
curl -s -X POST localhost:50002/v1/query/failed-txs -d "{\"address\":\"$VAL\",\"perPage\":20}" | head -c 400
echo; echo "=== /api/state raw ==="
curl -s localhost:8080/api/state | head -c 300
