#!/usr/bin/env bash
# Verify the one-click demo: after /api/seed, the target note is PENDING (camp A only); rating it from
# camp B bridges it to HELPFUL. Pass the targetNote id as $1.
set -u
G=localhost:8080
T="${1:?pass targetNote id}"
show() { curl -s $G/api/state | python3 -c "
import sys,json
t='$T'; d=json.load(sys.stdin)
for c in d['claims']:
  for n in c['notes']:
    if n['id']==t:
      s=n.get('score') or {}
      print(' status',n['status'],'intercept',s.get('intercept'),'cA',s.get('countA'),'cB',s.get('countB'),'raters',s.get('numRaters'))
"; }
echo "waiting ~48s for seed txs to land + MF to learn polarity"; sleep 48
echo "=== BEFORE camp-B (expect NEEDS_MORE_RATINGS / not HELPFUL) ==="; show
echo "=== rate target HELPFUL from camp B (b1,b2,b3) ==="
for r in b1 b2 b3; do curl -s -X POST $G/api/rate -H 'Content-Type: application/json' -d "{\"identity\":\"$r\",\"noteId\":\"$T\",\"value\":\"helpful\"}"; echo; done
echo "waiting ~40s for inclusion + MF rescore"; sleep 40
echo "=== AFTER camp-B (expect HELPFUL) ==="; show
