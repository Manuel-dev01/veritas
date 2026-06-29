#!/usr/bin/env bash
# Phase 2 end-to-end verification against the live local Canopy chain.
# Proves: SubmitClaim (regression) -> SubmitNote (requires claim) -> RateNote (multiple raters),
# plus the negative paths (rate-own-note, note-on-missing-claim) landing in failed-txs.
# Run inside WSL:  bash ~/veritas/phase2-verify.sh
set -u

QRPC="http://localhost:50002"
ARPC="http://localhost:50003"
SIGN="$HOME/veritas/veritas-sign"
VKEY="$HOME/.canopy/validator_key.json"
VADDR="d46e9a2042b4f2bdb69362aec7398f2ec623faa8"
PW="testpassword123"
WAIT=25
NONCE_BASE=$(date +%s)

say() { echo; echo "============================================================"; echo "## $*"; echo "============================================================"; }

# --- helpers ---------------------------------------------------------------
newkey() { # nickname -> address (lowercase hex)
  curl -s -X POST "$ARPC/v1/admin/keystore-new-key" -d "{\"nickname\":\"$1\",\"password\":\"$PW\"}" | tr -d '"'
}
getkey() { # address -> private key hex (admin RPC returns Go field name "PrivateKey"; pretty-printed)
  curl -s -X POST "$ARPC/v1/admin/keystore-get" -d "{\"address\":\"$1\",\"password\":\"$PW\"}" | tr -d ' \n' | grep -o '"PrivateKey":"[0-9a-fA-F]*"' | cut -d'"' -f4
}
balance() { # address -> amount (strip spaces/newlines so ": 123" matches)
  curl -s -X POST "$QRPC/v1/query/account" -d "{\"address\":\"$1\"}" | tr -d ' \n' | grep -o '"amount":[0-9]*' | head -1 | cut -d: -f2
}
events_for() { # address -> pretty event list
  curl -s -X POST "$QRPC/v1/query/events-by-address" -d "{\"address\":\"$1\",\"perPage\":100}"
}
failed_for() { # address -> failed tx json
  curl -s -X POST "$QRPC/v1/query/failed-txs" -d "{\"address\":\"$1\",\"perPage\":100}"
}

say "0. Node + plugin sanity"
curl -s -X POST "$QRPC/v1/query/height" -d "{}"
echo "validator balance: $(balance $VADDR)"

say "1. Create + fund two rater accounts"
R1=$(newkey "veritas_rater1_$NONCE_BASE")
R2=$(newkey "veritas_rater2_$NONCE_BASE")
echo "rater1=$R1"
echo "rater2=$R2"
K1=$(getkey "$R1")
K2=$(getkey "$R2")
echo "rater1 key len=${#K1} rater2 key len=${#K2}"
"$SIGN" -type send -key "$VKEY" -to "$R1" -amount 300000 -nonce $((NONCE_BASE+1))
"$SIGN" -type send -key "$VKEY" -to "$R2" -amount 300000 -nonce $((NONCE_BASE+2))
echo "funding txs submitted; waiting ${WAIT}s for inclusion..."
sleep $WAIT
echo "rater1 balance: $(balance $R1)"
echo "rater2 balance: $(balance $R2)"

say "2. SubmitClaim (regression of Phase 1 refactor)"
COUT=$("$SIGN" -type submit_claim -key "$VKEY" -content-hash cafe02 -url "https://example.com/phase2" -text "phase2 claim" -nonce $((NONCE_BASE+10)))
echo "$COUT"
CLAIMID=$(echo "$COUT" | grep -o 'claimId=[0-9a-f]*' | head -1 | cut -d= -f2)
echo ">> CLAIMID=$CLAIMID"
echo "waiting ${WAIT}s for the claim to land in state..."
sleep $WAIT

say "3. SubmitNote attached to the claim (author = validator)"
NOUT=$("$SIGN" -type submit_note -key "$VKEY" -claim-id "$CLAIMID" -body "this needs context: phase2 note" -content-hash beef02 -nonce $((NONCE_BASE+20)))
echo "$NOUT"
NOTEID=$(echo "$NOUT" | grep -o 'noteId=[0-9a-f]*' | head -1 | cut -d= -f2)
echo ">> NOTEID=$NOTEID"

say "4. Negative: SubmitNote on a NON-existent claim (expect DeliverTx ErrClaimNotFound)"
"$SIGN" -type submit_note -key "$VKEY" -claim-id "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff" -body "orphan note" -nonce $((NONCE_BASE+21))
echo "waiting ${WAIT}s for note + orphan to be applied..."
sleep $WAIT

say "5. RateNote from two distinct raters (expect both persist + note_rated events)"
"$SIGN" -type rate_note -key "$K1" -note-id "$NOTEID" -rating helpful -nonce $((NONCE_BASE+30))
"$SIGN" -type rate_note -key "$K2" -note-id "$NOTEID" -rating somewhat -nonce $((NONCE_BASE+31))

say "6. Negative: author rates own note (expect DeliverTx ErrCannotRateOwnNote)"
"$SIGN" -type rate_note -key "$VKEY" -note-id "$NOTEID" -rating helpful -nonce $((NONCE_BASE+32))
echo "waiting ${WAIT}s for ratings to be applied..."
sleep $WAIT

say "7. Read back via RPC — events for validator (claim_created + note_created)"
events_for "$VADDR"

say "8. Events for rater1 + rater2 (note_rated)"
events_for "$R1"
events_for "$R2"

say "9. Balances after fees"
echo "validator: $(balance $VADDR)"
echo "rater1:    $(balance $R1)"
echo "rater2:    $(balance $R2)"

say "10. failed-txs (expect the 2 negatives: rate-own-note + note-on-missing-claim)"
echo "-- validator failed --"; failed_for "$VADDR"

say "DONE. CLAIMID=$CLAIMID NOTEID=$NOTEID R1=$R1 R2=$R2"
