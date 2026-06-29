#!/usr/bin/env bash
# Focused RateNote proof: rate an existing note from two distinct funded keystore accounts,
# then read back the persisted ratings via note_rated events.
# Usage (inside WSL):  bash ~/veritas/phase2-rate.sh <noteIdHex> <raterAddr1> <raterAddr2>
set -u

QRPC="http://localhost:50002"
ARPC="http://localhost:50003"
SIGN="$HOME/veritas/veritas-sign"
PW="testpassword123"
WAIT=25
NOTEID="$1"
R1="$2"
R2="$3"
N=$(date +%s)

getkey() { curl -s -X POST "$ARPC/v1/admin/keystore-get" -d "{\"address\":\"$1\",\"password\":\"$PW\"}" | tr -d ' \n' | grep -o '"PrivateKey":"[0-9a-fA-F]*"' | cut -d'"' -f4; }
balance() { curl -s -X POST "$QRPC/v1/query/account" -d "{\"address\":\"$1\"}" | tr -d ' \n' | grep -o '"amount":[0-9]*' | head -1 | cut -d: -f2; }

K1=$(getkey "$R1")
K2=$(getkey "$R2")
echo "note=$NOTEID"
echo "rater1=$R1 keyLen=${#K1} balanceBefore=$(balance $R1)"
echo "rater2=$R2 keyLen=${#K2} balanceBefore=$(balance $R2)"

echo "--- rater1 rates HELPFUL ---"
"$SIGN" -type rate_note -key "$K1" -note-id "$NOTEID" -rating helpful -nonce $((N+1))
echo "--- rater2 rates SOMEWHAT ---"
"$SIGN" -type rate_note -key "$K2" -note-id "$NOTEID" -rating somewhat -nonce $((N+2))
echo "waiting ${WAIT}s for inclusion..."
sleep $WAIT

echo "rater1 balanceAfter=$(balance $R1)   rater2 balanceAfter=$(balance $R2)"
echo "=== events for rater1 (expect note_rated) ==="
curl -s -X POST "$QRPC/v1/query/events-by-address" -d "{\"address\":\"$R1\",\"perPage\":20}"
echo
echo "=== events for rater2 (expect note_rated) ==="
curl -s -X POST "$QRPC/v1/query/events-by-address" -d "{\"address\":\"$R2\",\"perPage\":20}"
echo
echo "=== failed-txs rater1 (expect empty) ==="
curl -s -X POST "$QRPC/v1/query/failed-txs" -d "{\"address\":\"$R1\",\"perPage\":20}"
echo
echo "=== failed-txs rater2 (expect empty) ==="
curl -s -X POST "$QRPC/v1/query/failed-txs" -d "{\"address\":\"$R2\",\"perPage\":20}"
echo
echo "DONE"
