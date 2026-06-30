#!/usr/bin/env bash
# Backend smoke test: drive claim -> note -> rate entirely through the gateway HTTP API and confirm
# the indexer reflects it (claim text, note body/url, MF score). Run in WSL: bash ~/veritas/gw-smoke.sh
set -u
G=localhost:8080
post(){ curl -s -X POST "$G/$1" -H 'Content-Type: application/json' -d "$2"; }
jget(){ python3 -c "import sys,json;print(json.load(sys.stdin).get('$1',''))"; }
say(){ echo; echo "## $*"; }

until curl -s $G/api/identities --max-time 2 | grep -q address; do sleep 2; done
say "identities"; curl -s $G/api/identities | python3 -m json.tool
say "wait for identity funding to land (block)"; sleep 24

say "POST /api/claim (as you)"
CR=$(post api/claim '{"identity":"you","text":"This chart proves unemployment tripled under the new policy.","url":"https://bls.gov/data"}')
echo "$CR"; C=$(echo "$CR" | jget claimId); echo "claimId=$C"
sleep 24
say "state: claim present with text?"; curl -s $G/api/state | python3 -c "import sys,json;d=json.load(sys.stdin);print('height',d['height']);[print('claim',c['id'][:12],repr(c['text']),'notes',len(c['notes'])) for c in d['claims']]"

say "POST /api/note"
NR=$(post api/note "{\"identity\":\"you\",\"claimId\":\"$C\",\"body\":\"The chart drops the 2019 baseline; full series shows +14%, not 200%.\",\"url\":\"https://bls.gov/series\"}")
echo "$NR"; N=$(echo "$NR" | jget noteId); echo "noteId=$N"
sleep 24

say "rate from all camps (smoke: just confirm scoring runs)"
for r in a1 a2 a3 b1 b2 b3; do post api/rate "{\"identity\":\"$r\",\"noteId\":\"$N\",\"value\":\"helpful\"}" >/dev/null; echo -n "$r "; done; echo
echo "waiting for MF EndBlock"; sleep 44

say "final state (note status + score)"
curl -s $G/api/state | python3 -c "
import sys,json
d=json.load(sys.stdin); print('height',d['height'])
for c in d['claims']:
  print('CLAIM',c['id'][:12],repr(c['text'])[:60])
  for n in c['notes']:
    s=n.get('score') or {}
    print('  NOTE',n['id'][:12],'status',n['status'],'body',repr(n['body'])[:50],'url',n['url'])
    if s: print('     score: intercept',s['intercept'],'status',s['status'],'cA',s['countA'],'cB',s['countB'],'raters',s['numRaters'])
"
say "log feed"; curl -s $G/api/state | python3 -c "import sys,json;[print(l['height'],l['kind'],l['text']) for l in json.load(sys.stdin)['log']]"
