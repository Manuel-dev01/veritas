#!/usr/bin/env bash
set -u
G=localhost:8080
until curl -s $G/api/identities --max-time 2 | grep -q address; do sleep 2; done
echo "gateway up; waiting for funding"; sleep 22
echo "submitting claim with text..."
curl -s -X POST $G/api/claim -H 'Content-Type: application/json' -d '{"identity":"you","text":"TEXTCHECK unemployment tripled","url":"https://bls.gov"}'
echo; echo "waiting for inclusion + index"; sleep 24
echo "=== claims in state (text must be non-empty) ==="
curl -s $G/api/state | python3 -c "import sys,json;d=json.load(sys.stdin);print('height',d['height']);[print(' text=',repr(c['text']),'url=',c['url'],'notes=',len(c['notes'])) for c in d['claims']]"
