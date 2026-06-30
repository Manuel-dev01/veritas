#!/usr/bin/env bash
# Veritas demo driver — the graded beat, end-to-end on the live chain via the CLI (no browser).
#
#   one camp helpful  -> note stays NEEDS_MORE_RATINGS (a majority, not a bridge)
#   + opposing camp   -> EndBlock MF flips it to HELPFUL (note_intercept crosses 0.40)
#
# It funds two camps, polarizes them with seed notes (so MF learns the latent axis), creates ONE target
# note, then reveals the flip in two phases, printing the on-chain verdict at each.
#
# Run inside WSL after the node is up + the chain has a funded genesis (deploy/reset-clean.sh):
#   cp /mnt/c/.../deploy/demo.sh ~/veritas/demo.sh && sed -i 's/\r$//' ~/veritas/demo.sh
#   bash ~/veritas/demo.sh
set -u
V="$HOME/veritas/veritas"
VKEY="$HOME/.canopy/validator_key.json"
VADDR="d46e9a2042b4f2bdb69362aec7398f2ec623faa8"   # genesis-funded validator (claim/note submitter)
WAIT="${WAIT:-18}"; N=$(date +%s); RN=1
say(){ echo; echo "============================================================"; echo "## $*"; echo "============================================================"; }
jget(){ python3 -c "import sys,json; print(json.load(sys.stdin).get('$1',''))"; }

# print the latest on-chain verdict for the target note (newest note_scored event)
show_target(){
  "$V" query -address "$VADDR" -event score --json | python3 -c "
import sys,json
t='${T}'; best=None
for it in json.load(sys.stdin):
    p=it.get('payload',{})
    if p.get('noteId')==t and (best is None or p.get('height',0)>=best.get('height',0)): best=p
if best is None: print('  (no score event yet)')
else: print('  status', best.get('status'), '| noteIntercept', best.get('noteIntercept'),
            '| countA', best.get('countA'), 'countB', best.get('countB'), '| numRaters', best.get('numRaters'))
"
}

say "1. create + fund two camps  A{A1,A2,A3}  B{B1,B2,B3}"
declare -A K
for who in A1 A2 A3 B1 B2 B3; do
  a=$("$V" keys new -name demo_${who}_$N --json | jget address)
  K[$who]=$("$V" keys get -address "$a" --json | jget privateKey)
  "$V" send -key "$VKEY" -to "$a" -amount 400000 >/dev/null
  echo "  $who = $a"
done
echo "funded; waiting ${WAIT}s for inclusion"; sleep "$WAIT"

say "2. one claim + three polarizing seed notes + one TARGET note"
C=$("$V" claim -key "$VKEY" -text "demo: chart drops the 2019 baseline" -content-hash de01 -nonce $((N+1)) --json | jget claimId)
sleep "$WAIT"
declare -A NOTE
i=0
for nm in S1 S2 S3 T; do
  NOTE[$nm]=$("$V" note -key "$VKEY" -claim-id "$C" -body "demo note $nm" -nonce $((N+10+i)) --json | jget noteId)
  echo "  $nm = ${NOTE[$nm]}"; i=$((i+1))
done
T="${NOTE[T]}"
sleep "$WAIT"

rate(){ local note=$1 val=$2; shift 2; for who in "$@"; do "$V" rate -key "${K[$who]}" -note-id "$note" -rating "$val" -nonce $((RN++)) >/dev/null; done; }

say "3. polarize the camps on the seed notes (A and B disagree)"
rate "${NOTE[S1]}" helpful A1 A2 A3 ; rate "${NOTE[S1]}" not B1 B2 B3
rate "${NOTE[S2]}" not     A1 A2 A3 ; rate "${NOTE[S2]}" helpful B1 B2 B3
rate "${NOTE[S3]}" helpful A1 A2 A3 ; rate "${NOTE[S3]}" not B1 B2 B3
echo "seed ratings in; waiting for MF to learn the axis"; sleep "$WAIT"; sleep "$WAIT"

say "4. PHASE A — rate the TARGET helpful from camp A ONLY"
rate "$T" helpful A1 A2 A3
echo "waiting for inclusion + EndBlock rescore"; sleep "$WAIT"; sleep "$WAIT"
echo ">> EXPECT: NEEDS_MORE_RATINGS, noteIntercept < 400000 (one camp is not a bridge)"
show_target

say "5. PHASE B — the OPPOSING camp B now also rates it helpful"
rate "$T" helpful B1 B2 B3
echo "waiting for inclusion + EndBlock rescore"; sleep "$WAIT"; sleep "$WAIT"
echo ">> EXPECT: HELPFUL, noteIntercept >= 400000 (0.40) — cross-camp agreement bridges the divide"
show_target

say "done — the chain decided this in EndBlock, byte-reproducibly. Target note: $T"
