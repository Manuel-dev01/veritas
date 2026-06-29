#!/usr/bin/env bash
# Phase 4 proof: non-transferable reputation gains/losses on note resolution + inactivity decay,
# all in EndBlock. Reuses the Phase 3 cohort scenario, which naturally produces every reason:
#   - seed note S resolves NOT_HELPFUL  -> author -500, B-cohort (correct) +500, A-cohort (wrong) -250
#   - target note T resolves HELPFUL    -> author +1000, all four helpful raters +500
#   - then idle > decayThresholdBlocks  -> reputation_changed{reason:decay} stepping toward 0
# Run inside WSL:  bash ~/veritas/phase4-verify.sh
set -u

QRPC="http://localhost:50002"; ARPC="http://localhost:50003"
SIGN="$HOME/veritas/veritas-sign"; DECODE="$HOME/veritas/decode_events.py"
VKEY="$HOME/.canopy/validator_key.json"; VADDR="d46e9a2042b4f2bdb69362aec7398f2ec623faa8"
PW="testpassword123"; WAIT=16; N=$(date +%s)

say() { echo; echo "============================================================"; echo "## $*"; echo "============================================================"; }
newkey() { curl -s -X POST "$ARPC/v1/admin/keystore-new-key" -d "{\"nickname\":\"$1\",\"password\":\"$PW\"}" | tr -d '"'; }
getkey() { curl -s -X POST "$ARPC/v1/admin/keystore-get" -d "{\"address\":\"$1\",\"password\":\"$PW\"}" | tr -d ' \n' | grep -o '"PrivateKey":"[0-9a-fA-F]*"' | cut -d'"' -f4; }
claimid() { echo "$1" | grep -o 'claimId=[0-9a-f]*' | head -1 | cut -d= -f2; }
noteid()  { echo "$1" | grep -o 'noteId=[0-9a-f]*'  | head -1 | cut -d= -f2; }
# repof <addr> <label>: decode this account's reputation_changed events (newest last)
repof() { echo "-- reputation events: $2 ($1) --"; curl -s -X POST "$QRPC/v1/query/events-by-address" -d "{\"address\":\"$1\",\"perPage\":200}" | python3 "$DECODE" "$1"; }
repall() { repof "$VADDR" "author/validator"; repof "$A1" "A1"; repof "$A2" "A2"; repof "$B1" "B1"; repof "$B2" "B2"; }

say "1. Create + fund four raters"
A1=$(newkey "p4_a1_$N"); A2=$(newkey "p4_a2_$N"); B1=$(newkey "p4_b1_$N"); B2=$(newkey "p4_b2_$N")
echo "A1=$A1 A2=$A2 B1=$B1 B2=$B2"
KA1=$(getkey "$A1"); KA2=$(getkey "$A2"); KB1=$(getkey "$B1"); KB2=$(getkey "$B2")
"$SIGN" -type send -key "$VKEY" -to "$A1" -amount 200000 -nonce $((N+1)) >/dev/null
"$SIGN" -type send -key "$VKEY" -to "$A2" -amount 200000 -nonce $((N+2)) >/dev/null
"$SIGN" -type send -key "$VKEY" -to "$B1" -amount 200000 -nonce $((N+3)) >/dev/null
"$SIGN" -type send -key "$VKEY" -to "$B2" -amount 200000 -nonce $((N+4)) >/dev/null
echo "waiting ${WAIT}s"; sleep $WAIT

say "2. Validator submits two claims + two notes (seed S, target T)"
CS=$(claimid "$("$SIGN" -type submit_claim -key "$VKEY" -content-hash 5eed04 -text "seed" -nonce $((N+10)))")
CT=$(claimid "$("$SIGN" -type submit_claim -key "$VKEY" -content-hash 7a7604 -text "target" -nonce $((N+11)))")
echo "waiting ${WAIT}s"; sleep $WAIT
S=$(noteid "$("$SIGN" -type submit_note -key "$VKEY" -claim-id "$CS" -body "seed note" -nonce $((N+20)))")
T=$(noteid "$("$SIGN" -type submit_note -key "$VKEY" -claim-id "$CT" -body "target note" -nonce $((N+21)))")
echo "seedNote S=$S"; echo "targetNote T=$T"; echo "waiting ${WAIT}s"; sleep $WAIT

say "3. Seed polarity on S: A-cohort HELPFUL, B-cohort NOT  (S resolves NOT_HELPFUL -> losses/gains)"
"$SIGN" -type rate_note -key "$KA1" -note-id "$S" -rating helpful -nonce $((N+30)) >/dev/null
"$SIGN" -type rate_note -key "$KA2" -note-id "$S" -rating helpful -nonce $((N+31)) >/dev/null
"$SIGN" -type rate_note -key "$KB1" -note-id "$S" -rating not     -nonce $((N+32)) >/dev/null
"$SIGN" -type rate_note -key "$KB2" -note-id "$S" -rating not     -nonce $((N+33)) >/dev/null
echo "waiting ${WAIT}s for S resolution + reputation"; sleep $WAIT
echo ">> after S resolves NOT_HELPFUL: expect author -500, B1/B2 +500 (correct), A1/A2 -250 (wrong)"
repall

say "4. Target T: cohort A then cohort B rate HELPFUL  (T resolves HELPFUL -> gains)"
"$SIGN" -type rate_note -key "$KA1" -note-id "$T" -rating helpful -nonce $((N+40)) >/dev/null
"$SIGN" -type rate_note -key "$KA2" -note-id "$T" -rating helpful -nonce $((N+41)) >/dev/null
echo "waiting ${WAIT}s (T should stay NEEDS_MORE: one cohort)"; sleep $WAIT
"$SIGN" -type rate_note -key "$KB1" -note-id "$T" -rating helpful -nonce $((N+50)) >/dev/null
"$SIGN" -type rate_note -key "$KB2" -note-id "$T" -rating helpful -nonce $((N+51)) >/dev/null
echo "waiting ${WAIT}s for T resolution + reputation"; sleep $WAIT
echo ">> after T resolves HELPFUL: expect author +1000, A1/A2/B1/B2 +500 each (cumulative)"
repall

say "5. DECAY: stay idle > decayThresholdBlocks(=10) and watch reputation step toward 0"
echo "idling ~150s (no transactions)..."; sleep 150
echo ">> expect reputation_changed{reason:decay} events, scoreFP decreasing (floored at 0)"
repall

say "DONE.  S=$S  T=$T  A1=$A1 A2=$A2 B1=$B1 B2=$B2"
