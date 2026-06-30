#!/usr/bin/env bash
# MF proof: drive the calibrated matrix-factorization scenario on the live chain via the CLI, then
# read back the on-chain note intercepts. Two factions (A,B) are polarized by seed notes; the bridge
# note (both factions helpful) must flip HELPFUL with note_intercept >= 400000 (=0.40), while the
# one-sided note stays NEEDS_MORE and the panned note goes NOT_HELPFUL.
# Run inside WSL (after reset-chain.sh + a few blocks):  bash ~/veritas/mf-verify.sh
set -u
V="$HOME/veritas/veritas"
VKEY="$HOME/.canopy/validator_key.json"
VADDR="d46e9a2042b4f2bdb69362aec7398f2ec623faa8"
WAIT=18; N=$(date +%s); RN=1
say(){ echo; echo "============================================================"; echo "## $*"; echo "============================================================"; }
jget(){ python3 -c "import sys,json; print(json.load(sys.stdin).get('$1',''))"; }

say "1. create + fund two factions A{A1,A2,A3} and B{B1,B2,B3}"
declare -A K
for who in A1 A2 A3 B1 B2 B3; do
  a=$("$V" keys new -name mf_${who}_$N --json | jget address)
  K[$who]=$("$V" keys get -address "$a" --json | jget privateKey)
  "$V" send -key "$VKEY" -to "$a" -amount 400000 >/dev/null
  echo "  $who = $a"
done
echo "funded; waiting ${WAIT}s"; sleep $WAIT

say "2. claim + six notes (3 polarizing seeds, bridge, one-sided, panned)"
C=$("$V" claim -key "$VKEY" -text "mf scenario claim" -content-hash dd01 -nonce $((N+1)) --json | jget claimId)
sleep $WAIT
declare -A NOTE
i=0
for nm in S1 S2 S3 TB TO TX; do
  NOTE[$nm]=$("$V" note -key "$VKEY" -claim-id "$C" -body "note $nm" -nonce $((N+10+i)) --json | jget noteId)
  echo "  $nm = ${NOTE[$nm]}"; i=$((i+1))
done
sleep $WAIT

rate_faction(){ local note=$1 val=$2; shift 2; for who in "$@"; do "$V" rate -key "${K[$who]}" -note-id "$note" -rating "$val" -nonce $((RN++)) >/dev/null; done; }

say "3. submit ratings: polarize A vs B on seeds, then the test notes"
rate_faction "${NOTE[S1]}" helpful A1 A2 A3 ; rate_faction "${NOTE[S1]}" not B1 B2 B3
rate_faction "${NOTE[S2]}" not     A1 A2 A3 ; rate_faction "${NOTE[S2]}" helpful B1 B2 B3
rate_faction "${NOTE[S3]}" helpful A1 A2 A3 ; rate_faction "${NOTE[S3]}" not B1 B2 B3
rate_faction "${NOTE[TB]}" helpful A1 A2 A3 B1 B2 B3      # bridge: both factions helpful
rate_faction "${NOTE[TO]}" helpful A1 A2 A3              # one-sided: faction A only
rate_faction "${NOTE[TX]}" not     A1 A2 A3 B1 B2 B3      # panned
echo "ratings submitted; waiting ${WAIT}s x2 for inclusion + EndBlock MF recompute"; sleep $WAIT; sleep $WAIT

say "4. read the on-chain MF verdicts (note_scored events, decoded by the CLI)"
"$V" query -address "$VADDR" -event score
echo
echo "-- JSON for the bridge note --"
"$V" query -address "$VADDR" -event score --json | python3 -c "
import sys,json
tb='${NOTE[TB]}'
for it in json.load(sys.stdin):
    if it['payload'].get('noteId')==tb: print(json.dumps(it['payload'],indent=2)); break
"
say "EXPECT: TB=HELPFUL (noteIntercept>=400000), TO=NEEDS_MORE_RATINGS, TX=NOT_HELPFUL — all at end_block"
