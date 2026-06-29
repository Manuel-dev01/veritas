package main

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/canopy-network/go-plugin/contract"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// mkEvent wraps a proto payload as a rawEvent exactly as the node renders it (Any hex under orderId).
func mkEvent(t *testing.T, eventType string, m proto.Message) rawEvent {
	t.Helper()
	any, err := anypb.New(m)
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(any)
	if err != nil {
		t.Fatal(err)
	}
	e := rawEvent{EventType: eventType, Height: 42, Reference: "end_block", Address: "aa"}
	e.Msg.OrderID = hex.EncodeToString(b)
	return e
}

func TestDecodeEventPayload_RoundTrip(t *testing.T) {
	e := mkEvent(t, "note_scored", &contract.NoteScoredEvent{
		NoteId: []byte{1, 2, 3}, Status: contract.NoteStatus_HELPFUL,
		BridgeScore: 1000, MeanA: 1000, MeanB: 1000, CountA: 2, CountB: 2, Height: 42,
	})
	msg, err := decodeEventPayload(e)
	if err != nil {
		t.Fatal(err)
	}
	sc, ok := msg.(*contract.NoteScoredEvent)
	if !ok {
		t.Fatalf("decoded wrong type %T", msg)
	}
	if sc.Status != contract.NoteStatus_HELPFUL || sc.BridgeScore != 1000 || sc.CountA != 2 {
		t.Fatalf("bad decode: %+v", sc)
	}
}

func TestDecodeEventPayload_NonCustomSkipped(t *testing.T) {
	if msg, err := decodeEventPayload(rawEvent{EventType: "reward"}); err != nil || msg != nil {
		t.Fatalf("core event with no orderId should decode to (nil,nil), got (%v,%v)", msg, err)
	}
}

func TestRenderEvents_FilterAndJSON(t *testing.T) {
	evs := []rawEvent{
		mkEvent(t, "note_scored", &contract.NoteScoredEvent{NoteId: []byte{1}, Status: contract.NoteStatus_HELPFUL, BridgeScore: 1000}),
		mkEvent(t, "reputation_changed", &contract.ReputationChangedEvent{Account: []byte{2}, ScoreFp: 500, Delta: 500, Reason: "rater_correct"}),
	}

	// JSON output filtered to score only
	out, err := renderEvents(evs, "score", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "note_scored") || strings.Contains(out, "reputation_changed") {
		t.Fatalf("event filter failed:\n%s", out)
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}

	// human-readable, reputation only
	out2, err := renderEvents(evs, "reputation", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2, "reputation_changed") || !strings.Contains(out2, "rater_correct") {
		t.Fatalf("reputation render missing fields:\n%s", out2)
	}
}
