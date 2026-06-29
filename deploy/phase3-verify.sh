#!/usr/bin/env bash
# Phase 3 end-to-end proof of the bridging scorer on the live local chain.
# Establishes two opposing cohorts via a seed note, then shows a target note flip
# NEEDS_MORE_RATINGS -> HELPFUL only after BOTH cohorts rate it helpful — all in EndBlock.
# Run inside WSL:  bash ~/veritas/phase3-verify.sh
set -u

QRPC="http://localhost:50002"
ARPC="http://localhost:50003"
SIGN="$HOME/veritas/veritas-sign"
DECODE="$HOME/veritas/decode_events.py"
VKEY="$HOME/.canopy/validator_key.json"
VADDR="d46e9a2042b4f2bdb69362aec7398f2ec623faa8"
PW="testpassword123"
WAIT=16
N=$(date +%s)

say() { echo; echo "============================================================"; echo "## $*"; echo "============================================================"; }
newkey() { curl -s -X POST "$ARPC/v1/admin/keystore-new-key" -d "{\"nickname\":\"$1\",\"password\":\"$PW\"}" | tr -d '"'; }
getkey() { curl -s -X POST "$ARPC/v1/admin/keystore-get" -d "{\"address\":\"$1\",\"password\":\"$PW\"}" | tr -d ' \n' | grep -o '"PrivateKey":"[0-9a-fA-F]*"' | cut -d'"' -f4; }
claimid() { echo "$1" | grep -o 'claimId=[0-9a-f]*' | head -1 | cut -d= -f2; }
noteid()  { echo "$1" | grep -o 'noteId=[0-9a-f]*'  | head -1 | cut -d= -f2; }
scored()  { curl -s -X POST "$QRPC/v1/query/events-by-address" -d "{\"address\":\"$VADDR\",\"perPage\":150}" | python3 "$DECODE" "$1"; }

say "1. Create + fund four raters (two future cohorts)"
A1=$(newkey "p3_a1_$N"); A2=$(newkey "p3_a2_$N"); B1=$(newkey "p3_b1_$N"); B2=$(newkey "p3_b2_$N")
echo "A1=$A1 A2=$A2 B1=$B1 B2=$B2"
KA1=$(getkey "$A1"); KA2=$(getkey "$A2"); KB1=$(getkey "$B1"); KB2=$(getkey "$B2")
"$SIGN" -type send -key "$VKEY" -to "$A1" -amount 200000 -nonce $((N+1)) >/dev/null
"$SIGN" -type send -key "$VKEY" -to "$A2" -amount 200000 -nonce $((N+2)) >/dev/null
"$SIGN" -type send -key "$VKEY" -to "$B1" -amount 200000 -nonce $((N+3)) >/dev/null
"$SIGN" -type send -key "$VKEY" -to "$B2" -amount 200000 -nonce $((N+4)) >/dev/null
echo "funded; waiting ${WAIT}s"; sleep $WAIT

say "2. Validator submits a seed claim + target claim"
CS=$(claimid "$("$SIGN" -type submit_claim -key "$VKEY" -content-hash 5eed01 -text "seed claim" -nonce $((N+10)))")
CT=$(claimid "$("$SIGN" -type submit_claim -key "$VKEY" -content-hash 7a7601 -text "target claim" -nonce $((N+11)))")
echo "seedClaim=$CS targetClaim=$CT"; echo "waiting ${WAIT}s"; sleep $WAIT

say "3. Validator attaches a seed note S and target note T"
S=$(noteid "$("$SIGN" -type submit_note -key "$VKEY" -claim-id "$CS" -body "seed note for polarity" -nonce $((N+20)))")
T=$(noteid "$("$SIGN" -type submit_note -key "$VKEY" -claim-id "$CT" -body "the claim is misleading" -nonce $((N+21)))")
echo "seedNote S=$S"; echo "targetNote T=$T"; echo "waiting ${WAIT}s"; sleep $WAIT

say "4. Establish polarity on S: A-cohort rates HELPFUL, B-cohort rates NOT_HELPFUL"
"$SIGN" -type rate_note -key "$KA1" -note-id "$S" -rating helpful -nonce $((N+30)) >/dev/null
"$SIGN" -type rate_note -key "$KA2" -note-id "$S" -rating helpful -nonce $((N+31)) >/dev/null
"$SIGN" -type rate_note -key "$KB1" -note-id "$S" -rating not     -nonce $((N+32)) >/dev/null
"$SIGN" -type rate_note -key "$KB2" -note-id "$S" -rating not     -nonce $((N+33)) >/dev/null
echo "polarity ratings submitted; waiting ${WAIT}s"; sleep $WAIT

say "5. PHASE A — only cohort A rates the TARGET note helpful (expect: stays NEEDS_MORE_RATINGS)"
"$SIGN" -type rate_note -key "$KA1" -note-id "$T" -rating helpful -nonce $((N+40)) >/dev/null
"$SIGN" -type rate_note -key "$KA2" -note-id "$T" -rating helpful -nonce $((N+41)) >/dev/null
echo "waiting ${WAIT}s for EndBlock rescore"; sleep $WAIT
echo "-- note_scored events for TARGET note T (newest last) --"
scored "$T"

say "6. PHASE B — cohort B also rates the TARGET helpful (expect: flips to HELPFUL in EndBlock)"
"$SIGN" -type rate_note -key "$KB1" -note-id "$T" -rating helpful -nonce $((N+50)) >/dev/null
"$SIGN" -type rate_note -key "$KB2" -note-id "$T" -rating helpful -nonce $((N+51)) >/dev/null
echo "waiting ${WAIT}s for EndBlock rescore"; sleep $WAIT
echo "-- note_scored events for TARGET note T (newest last) --"
scored "$T"

say "DONE.  seedNote=$S  targetNote=$T  (the flip should appear at an 'end_block' reference)"
