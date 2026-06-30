package contract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
)

/*
replay_test.go — in-process cross-node determinism proof.

We replay an identical, ordered transaction history (claims -> notes -> ratings -> EndBlock recompute)
through the REAL handlers and the REAL fixed-point MF EndBlock on TWO independent in-memory backends,
and assert they produce a byte-identical per-block state digest (an "app hash") and identical emitted
events. This is the two-validator identical-state guarantee, validated in-process: same input ⇒ same
state, every block, including the matrix-factorization scoring.
*/

// stateDigest is a deterministic SHA-256 over the whole KV store (sorted keys, length-delimited) —
// the in-process analogue of a validator's app/state hash.
func stateDigest(f *fakeBackend) string {
	keys := make([]string, 0, len(f.kv))
	for k := range f.kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	var lb [8]byte
	for _, k := range keys {
		binary.BigEndian.PutUint64(lb[:], uint64(len(k)))
		h.Write(lb[:])
		h.Write([]byte(k))
		v := f.kv[k]
		binary.BigEndian.PutUint64(lb[:], uint64(len(v)))
		h.Write(lb[:])
		h.Write(v)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// eventsDigest hashes the ordered events emitted in a block (their marshaled payloads).
func eventsDigest(events []*Event) string {
	h := sha256.New()
	for _, e := range events {
		h.Write([]byte(e.EventType))
		if c, ok := e.Msg.(*Event_Custom); ok && c.Custom != nil && c.Custom.Msg != nil {
			b, _ := proto.Marshal(c.Custom.Msg)
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

type blockDigest struct{ state, events string }

// runScenario replays a fixed history on a fresh backend and returns the per-block digests.
func runScenario(t *testing.T, label string) []blockDigest {
	t.Helper()
	const fee = testFee
	author := addr(0x01)
	A := [][]byte{addr(0xA1), addr(0xA2), addr(0xA3)}
	B := [][]byte{addr(0xB1), addr(0xB2), addr(0xB3)}

	f := newFakeBackend()
	f.seedFeePool(1, 0)
	f.seedAccount(author, 1_000_000)
	for _, r := range append(append([][]byte{}, A...), B...) {
		f.seedAccount(r, 1_000_000)
	}
	c := &Contract{Config: Config{ChainId: 1}, plugin: f}

	var digests []blockDigest
	endBlock := func(h uint64) {
		resp := c.EndBlock(&PluginEndRequest{Height: h})
		if resp.Error != nil {
			t.Fatalf("%s: EndBlock h=%d error: %v", label, h, resp.Error)
		}
		digests = append(digests, blockDigest{state: stateDigest(f), events: eventsDigest(resp.Events)})
	}
	claim := func(nonce, h uint64) []byte {
		r := c.DeliverMessageSubmitClaim(&MessageSubmitClaim{Submitter: author, ContentHash: []byte{byte(nonce)}, Nonce: nonce}, fee, h)
		if r.Error != nil {
			t.Fatalf("%s: claim error: %v", label, r.Error)
		}
		return DeriveClaimID(author, nonce, h)
	}
	note := func(cl []byte, nonce, h uint64) []byte {
		r := c.DeliverMessageSubmitNote(&MessageSubmitNote{Author: author, ClaimId: cl, Body: "note", Nonce: nonce}, fee, h)
		if r.Error != nil {
			t.Fatalf("%s: note error: %v", label, r.Error)
		}
		return DeriveNoteID(author, cl, nonce, h)
	}
	rate := func(rater, nid []byte, v RatingValue, nonce, h uint64) {
		r := c.DeliverMessageRateNote(&MessageRateNote{Rater: rater, NoteId: nid, Value: v, Nonce: nonce}, fee, h)
		if r.Error != nil {
			t.Fatalf("%s: rate error: %v", label, r.Error)
		}
	}
	rateAll := func(raters [][]byte, nid []byte, v RatingValue, h uint64) {
		for i, r := range raters {
			rate(r, nid, v, uint64(i), h)
		}
	}
	H, X := RatingValue_RATING_HELPFUL, RatingValue_RATING_NOT_HELPFUL

	// block 100: one claim. EndBlock (no dirty).
	cl := claim(1, 100)
	endBlock(100)

	// block 101: six notes on the claim. EndBlock (no ratings yet).
	s1, s2, s3 := note(cl, 1, 101), note(cl, 2, 101), note(cl, 3, 101)
	tBridge, tOne, tBad := note(cl, 4, 101), note(cl, 5, 101), note(cl, 6, 101)
	endBlock(101)

	// block 102: polarizing seed ratings + the test notes; EndBlock runs the MF recompute.
	rateAll(A, s1, H, 102)
	rateAll(B, s1, X, 102)
	rateAll(A, s2, X, 102)
	rateAll(B, s2, H, 102)
	rateAll(A, s3, H, 102)
	rateAll(B, s3, X, 102)
	rateAll(A, tBridge, H, 102)
	rateAll(B, tBridge, H, 102)
	rateAll(A, tOne, H, 102)
	rateAll(A, tBad, X, 102)
	rateAll(B, tBad, X, 102)
	endBlock(102)

	// block 103: no new ratings (dirty cleared); exercises decay/idempotency.
	endBlock(103)
	return digests
}

func TestReplayDeterminism(t *testing.T) {
	runA := runScenario(t, "node-A")
	runB := runScenario(t, "node-B")
	if len(runA) != len(runB) {
		t.Fatalf("block count differs: %d vs %d", len(runA), len(runB))
	}
	for i := range runA {
		if runA[i].state != runB[i].state {
			t.Fatalf("block %d STATE digest differs:\n  A=%s\n  B=%s", i, runA[i].state, runB[i].state)
		}
		if runA[i].events != runB[i].events {
			t.Fatalf("block %d EVENTS digest differs:\n  A=%s\n  B=%s", i, runA[i].events, runB[i].events)
		}
	}
	t.Logf("two independent replays agree on all %d per-block app-hashes (incl. MF scoring).", len(runA))
	t.Logf("final app-hash = %s", runA[len(runA)-1].state)
}
