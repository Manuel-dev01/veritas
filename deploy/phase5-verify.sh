#!/usr/bin/env bash
# Phase 5 proof: the whole claim -> note -> rating -> score -> reputation lifecycle driven entirely
# by the `veritas` Go CLI — every tx BLS-signed + submitted by the CLI, every read decoded by the
# CLI's `query` (no python decoder, no raw curl).
# Run inside WSL:  bash ~/veritas/phase5-verify.sh
set -u

V="$HOME/veritas/veritas"
VKEY="$HOME/.canopy/validator_key.json"
VADDR="d46e9a2042b4f2bdb69362aec7398f2ec623faa8"
WAIT=16; N=$(date +%s)

say() { echo; echo "============================================================"; echo "## $*"; echo "============================================================"; }
jget() { python3 -c "import sys,json; print(json.load(sys.stdin).get('$1',''))"; }

say "1. keys new + fund four raters (all via the CLI)"
A1=$("$V" keys new -name p5a1_$N --json | jget address)
A2=$("$V" keys new -name p5a2_$N --json | jget address)
B1=$("$V" keys new -name p5b1_$N --json | jget address)
B2=$("$V" keys new -name p5b2_$N --json | jget address)
echo "A1=$A1 A2=$A2 B1=$B1 B2=$B2"
KA1=$("$V" keys get -address $A1 --json | jget privateKey)
KA2=$("$V" keys get -address $A2 --json | jget privateKey)
KB1=$("$V" keys get -address $B1 --json | jget privateKey)
KB2=$("$V" keys get -address $B2 --json | jget privateKey)
for A in $A1 $A2 $B1 $B2; do "$V" send -key "$VKEY" -to "$A" -amount 250000 >/dev/null; done
echo "funded; waiting ${WAIT}s"; sleep $WAIT

say "2. submit claims + notes (CLI prints derived ids)"
CLAIM=$("$V" claim -key "$VKEY" -text "phase5 target claim" -content-hash c5c5 -nonce $((N+10)) --json | jget claimId)
CS=$("$V"    claim -key "$VKEY" -text "phase5 seed claim"   -content-hash 5eed -nonce $((N+11)) --json | jget claimId)
echo "targetClaim=$CLAIM seedClaim=$CS"; sleep $WAIT
T=$("$V" note -key "$VKEY" -claim-id "$CLAIM" -body "this is the target note" -nonce $((N+20)) --json | jget noteId)
S=$("$V" note -key "$VKEY" -claim-id "$CS"    -body "polarity seed note"     -nonce $((N+21)) --json | jget noteId)
echo "targetNote=$T seedNote=$S"; sleep $WAIT

say "3. seed polarity (A helpful / B not), then target: A-cohort then B-cohort rate HELPFUL"
"$V" rate -key "$KA1" -note-id "$S" -rating helpful -nonce $((N+30)) >/dev/null
"$V" rate -key "$KA2" -note-id "$S" -rating helpful -nonce $((N+31)) >/dev/null
"$V" rate -key "$KB1" -note-id "$S" -rating not     -nonce $((N+32)) >/dev/null
"$V" rate -key "$KB2" -note-id "$S" -rating not     -nonce $((N+33)) >/dev/null
sleep $WAIT
"$V" rate -key "$KA1" -note-id "$T" -rating helpful -nonce $((N+40)) >/dev/null
"$V" rate -key "$KA2" -note-id "$T" -rating helpful -nonce $((N+41)) >/dev/null
echo "cohort A only — T should stay NEEDS_MORE; waiting ${WAIT}s"; sleep $WAIT
"$V" rate -key "$KB1" -note-id "$T" -rating helpful -nonce $((N+50)) >/dev/null
"$V" rate -key "$KB2" -note-id "$T" -rating helpful -nonce $((N+51)) >/dev/null
echo "cohort B joins — T should flip HELPFUL; waiting ${WAIT}s"; sleep $WAIT

say "4. READ the lifecycle entirely via 'veritas query' (no python, no curl)"
echo "-- claim_created (validator) --"; "$V" query -address "$VADDR" -event claim
echo "-- note_created (validator) --";  "$V" query -address "$VADDR" -event note
echo "-- note_scored (validator): NEEDS_MORE then HELPFUL flip --"; "$V" query -address "$VADDR" -event score
echo "-- note_rated (B1) --";           "$V" query -address "$B1" -event rating
echo "-- reputation_changed (B1) --";   "$V" query -address "$B1" -event reputation
echo "-- JSON sample (scores) --";      "$V" query -address "$VADDR" -event score --json

say "DONE.  targetClaim=$CLAIM targetNote=$T  (the flip + reputation should appear at end_block)"
