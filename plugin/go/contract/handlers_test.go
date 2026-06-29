package contract

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
)

/*
handlers_test.go — table tests for the Veritas CheckTx/DeliverTx handlers.

The Deliver* handlers talk to the FSM through the StateBackend interface; here we inject an
in-memory fakeBackend so the full handler (key wiring -> decision -> writes -> event) runs without
a node. The Check* handlers are pure and tested directly. These tests pin down the Phase 2 state
model: claim->note->ratings persistence, fee charging, the claim->note index, the dirty-set marker,
idempotency/overwrite semantics, and every error path.
*/

const testFee = uint64(10000)

// ---- in-memory StateBackend fake ------------------------------------------------------------

type fakeBackend struct {
	kv     map[string][]byte
	writes int // number of StateWrite calls (to assert "no write happened" on rejects)
}

func newFakeBackend() *fakeBackend { return &fakeBackend{kv: map[string][]byte{}} }

func (f *fakeBackend) StateRead(_ *Contract, req *PluginStateReadRequest) (*PluginStateReadResponse, *PluginError) {
	results := make([]*PluginReadResult, 0, len(req.Keys))
	for _, k := range req.Keys {
		res := &PluginReadResult{QueryId: k.QueryId}
		if v, ok := f.kv[string(k.Key)]; ok {
			res.Entries = []*PluginStateEntry{{Key: k.Key, Value: v}}
		}
		results = append(results, res)
	}
	return &PluginStateReadResponse{Results: results}, nil
}

func (f *fakeBackend) StateWrite(_ *Contract, req *PluginStateWriteRequest) (*PluginStateWriteResponse, *PluginError) {
	f.writes++
	for _, s := range req.Sets {
		f.kv[string(s.Key)] = s.Value
	}
	for _, d := range req.Deletes {
		delete(f.kv, string(d.Key))
	}
	return &PluginStateWriteResponse{}, nil
}

// seeding / inspection helpers

func (f *fakeBackend) seedAccount(addr []byte, amount uint64) {
	bz, err := Marshal(&Account{Address: addr, Amount: amount})
	if err != nil {
		panic(err)
	}
	f.kv[string(KeyForAccount(addr))] = bz
}

func (f *fakeBackend) seedFeePool(chainID, amount uint64) {
	bz, err := Marshal(&Pool{Amount: amount})
	if err != nil {
		panic(err)
	}
	f.kv[string(KeyForFeePool(chainID))] = bz
}

func (f *fakeBackend) seedClaim(c *Claim) {
	bz, err := Marshal(c)
	if err != nil {
		panic(err)
	}
	f.kv[string(KeyForClaim(c.Id))] = bz
}

func (f *fakeBackend) seedNote(n *Note) {
	bz, err := Marshal(n)
	if err != nil {
		panic(err)
	}
	f.kv[string(KeyForNote(n.Id))] = bz
}

func (f *fakeBackend) has(key []byte) bool { _, ok := f.kv[string(key)]; return ok }

func (f *fakeBackend) accountAmount(addr []byte) uint64 {
	a := new(Account)
	if err := Unmarshal(f.kv[string(KeyForAccount(addr))], a); err != nil {
		panic(err)
	}
	return a.Amount
}

func (f *fakeBackend) poolAmount(chainID uint64) uint64 {
	p := new(Pool)
	if err := Unmarshal(f.kv[string(KeyForFeePool(chainID))], p); err != nil {
		panic(err)
	}
	return p.Amount
}

func newTestContract(f *fakeBackend) *Contract {
	return &Contract{Config: Config{ChainId: 1}, plugin: f}
}

// addr builds a deterministic 20-byte address filled with b.
func addr(b byte) []byte {
	a := make([]byte, 20)
	for i := range a {
		a[i] = b
	}
	return a
}

// decodeEvent asserts a single event of the given type was emitted and decodes its payload into out.
func decodeEvent(t *testing.T, resp *PluginDeliverResponse, eventType string, out proto.Message) {
	t.Helper()
	if len(resp.Events) != 1 {
		t.Fatalf("expected exactly 1 event, got %d", len(resp.Events))
	}
	ev := resp.Events[0]
	if ev.EventType != eventType {
		t.Fatalf("event type = %q, want %q", ev.EventType, eventType)
	}
	custom, ok := ev.Msg.(*Event_Custom)
	if !ok || custom.Custom == nil || custom.Custom.Msg == nil {
		t.Fatalf("event missing custom payload")
	}
	if err := custom.Custom.Msg.UnmarshalTo(out); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
}

// ---- CheckTx (pure) -------------------------------------------------------------------------

func TestCheckMessageSubmitClaim(t *testing.T) {
	c := newTestContract(newFakeBackend())
	good := addr(1)
	tests := []struct {
		name    string
		msg     *MessageSubmitClaim
		wantErr bool
	}{
		{"valid content hash", &MessageSubmitClaim{Submitter: good, ContentHash: []byte{0xaa}}, false},
		{"valid url only", &MessageSubmitClaim{Submitter: good, Url: "https://x.example"}, false},
		{"bad address length", &MessageSubmitClaim{Submitter: []byte{1, 2, 3}, ContentHash: []byte{0xaa}}, true},
		{"empty claim", &MessageSubmitClaim{Submitter: good}, true},
		{"text too long", &MessageSubmitClaim{Submitter: good, ContentHash: []byte{0xaa}, Text: string(make([]byte, maxClaimTextLen+1))}, true},
		{"url too long", &MessageSubmitClaim{Submitter: good, Url: string(make([]byte, maxURLLen+1))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := c.CheckMessageSubmitClaim(tt.msg)
			if (resp.Error != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", resp.Error, tt.wantErr)
			}
			if !tt.wantErr && (len(resp.AuthorizedSigners) != 1 || !bytes.Equal(resp.AuthorizedSigners[0], tt.msg.Submitter)) {
				t.Fatalf("authorized signers = %x, want [%x]", resp.AuthorizedSigners, tt.msg.Submitter)
			}
		})
	}
}

func TestCheckMessageSubmitNote(t *testing.T) {
	c := newTestContract(newFakeBackend())
	good := addr(1)
	claimID := DeriveClaimID(good, 1, 10)
	tests := []struct {
		name    string
		msg     *MessageSubmitNote
		wantErr bool
	}{
		{"valid", &MessageSubmitNote{Author: good, ClaimId: claimID, Body: "context here"}, false},
		{"bad address", &MessageSubmitNote{Author: []byte{1}, ClaimId: claimID, Body: "x"}, true},
		{"empty claim id", &MessageSubmitNote{Author: good, Body: "x"}, true},
		{"empty body", &MessageSubmitNote{Author: good, ClaimId: claimID, Body: ""}, true},
		{"body too long", &MessageSubmitNote{Author: good, ClaimId: claimID, Body: string(make([]byte, maxNoteBodyLen+1))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := c.CheckMessageSubmitNote(tt.msg)
			if (resp.Error != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", resp.Error, tt.wantErr)
			}
			if !tt.wantErr && (len(resp.AuthorizedSigners) != 1 || !bytes.Equal(resp.AuthorizedSigners[0], tt.msg.Author)) {
				t.Fatalf("authorized signers = %x, want [%x]", resp.AuthorizedSigners, tt.msg.Author)
			}
		})
	}
}

func TestCheckMessageRateNote(t *testing.T) {
	c := newTestContract(newFakeBackend())
	good := addr(1)
	noteID := DeriveNoteID(good, DeriveClaimID(good, 1, 10), 1, 11)
	tests := []struct {
		name    string
		msg     *MessageRateNote
		wantErr bool
	}{
		{"valid", &MessageRateNote{Rater: good, NoteId: noteID, Value: RatingValue_RATING_HELPFUL}, false},
		{"bad address", &MessageRateNote{Rater: []byte{1}, NoteId: noteID, Value: RatingValue_RATING_HELPFUL}, true},
		{"empty note id", &MessageRateNote{Rater: good, Value: RatingValue_RATING_HELPFUL}, true},
		{"unspecified value", &MessageRateNote{Rater: good, NoteId: noteID, Value: RatingValue_RATING_UNSPECIFIED}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := c.CheckMessageRateNote(tt.msg)
			if (resp.Error != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", resp.Error, tt.wantErr)
			}
			if !tt.wantErr && (len(resp.AuthorizedSigners) != 1 || !bytes.Equal(resp.AuthorizedSigners[0], tt.msg.Rater)) {
				t.Fatalf("authorized signers = %x, want [%x]", resp.AuthorizedSigners, tt.msg.Rater)
			}
		})
	}
}

// ---- DeliverTx: SubmitClaim -----------------------------------------------------------------

func TestDeliverMessageSubmitClaim(t *testing.T) {
	submitter := addr(0xa1)
	const height = uint64(100)

	t.Run("happy path persists, charges fee, emits event", func(t *testing.T) {
		f := newFakeBackend()
		f.seedAccount(submitter, 1_000_000)
		f.seedFeePool(1, 0)
		c := newTestContract(f)
		msg := &MessageSubmitClaim{Submitter: submitter, ContentHash: []byte{0xde, 0xad}, Url: "https://x", Text: "the sky is green", Nonce: 7}

		resp := c.DeliverMessageSubmitClaim(msg, testFee, height)
		if resp.Error != nil {
			t.Fatalf("unexpected error: %v", resp.Error)
		}
		id := DeriveClaimID(submitter, 7, height)
		claim := new(Claim)
		if err := Unmarshal(f.kv[string(KeyForClaim(id))], claim); err != nil {
			t.Fatalf("claim not persisted: %v", err)
		}
		if !bytes.Equal(claim.Id, id) || claim.Text != "the sky is green" || claim.Url != "https://x" || !bytes.Equal(claim.Submitter, submitter) || claim.CreatedHeight != height {
			t.Fatalf("claim record wrong: %+v", claim)
		}
		if got := f.accountAmount(submitter); got != 1_000_000-testFee {
			t.Fatalf("submitter balance = %d, want %d", got, 1_000_000-testFee)
		}
		if got := f.poolAmount(1); got != testFee {
			t.Fatalf("fee pool = %d, want %d", got, testFee)
		}
		var got ClaimCreatedEvent
		decodeEvent(t, resp, "claim_created", &got)
		if !bytes.Equal(got.ClaimId, id) || !bytes.Equal(got.Submitter, submitter) || got.CreatedHeight != height {
			t.Fatalf("event payload wrong: %+v", &got)
		}
	})

	t.Run("insufficient funds rejected, no write", func(t *testing.T) {
		f := newFakeBackend()
		f.seedAccount(submitter, testFee-1)
		f.seedFeePool(1, 0)
		c := newTestContract(f)
		resp := c.DeliverMessageSubmitClaim(&MessageSubmitClaim{Submitter: submitter, ContentHash: []byte{1}, Nonce: 1}, testFee, height)
		if resp.Error == nil {
			t.Fatal("expected insufficient funds error")
		}
		if f.writes != 0 {
			t.Fatalf("expected no state write, got %d", f.writes)
		}
	})

	t.Run("replay is idempotent no-op", func(t *testing.T) {
		f := newFakeBackend()
		f.seedAccount(submitter, 1_000_000)
		f.seedFeePool(1, 0)
		id := DeriveClaimID(submitter, 3, height)
		f.seedClaim(&Claim{Id: id, Submitter: submitter, CreatedHeight: height})
		c := newTestContract(f)
		resp := c.DeliverMessageSubmitClaim(&MessageSubmitClaim{Submitter: submitter, ContentHash: []byte{1}, Nonce: 3}, testFee, height)
		if resp.Error != nil {
			t.Fatalf("unexpected error: %v", resp.Error)
		}
		if f.writes != 0 {
			t.Fatalf("replay should not write, got %d writes", f.writes)
		}
		if got := f.accountAmount(submitter); got != 1_000_000 {
			t.Fatalf("replay charged fee: balance %d", got)
		}
		if len(resp.Events) != 0 {
			t.Fatalf("replay should emit no events, got %d", len(resp.Events))
		}
	})

	t.Run("draining account deletes it", func(t *testing.T) {
		f := newFakeBackend()
		f.seedAccount(submitter, testFee) // exactly the fee
		f.seedFeePool(1, 0)
		c := newTestContract(f)
		resp := c.DeliverMessageSubmitClaim(&MessageSubmitClaim{Submitter: submitter, ContentHash: []byte{1}, Nonce: 9}, testFee, height)
		if resp.Error != nil {
			t.Fatalf("unexpected error: %v", resp.Error)
		}
		if f.has(KeyForAccount(submitter)) {
			t.Fatal("drained submitter account should be deleted")
		}
		if got := f.poolAmount(1); got != testFee {
			t.Fatalf("fee pool = %d, want %d", got, testFee)
		}
	})
}

// ---- DeliverTx: SubmitNote ------------------------------------------------------------------

func TestDeliverMessageSubmitNote(t *testing.T) {
	author := addr(0xb2)
	submitter := addr(0xa1)
	const height = uint64(200)
	claimID := DeriveClaimID(submitter, 1, 50)

	seedReady := func() *fakeBackend {
		f := newFakeBackend()
		f.seedAccount(author, 1_000_000)
		f.seedFeePool(1, 0)
		f.seedClaim(&Claim{Id: claimID, Submitter: submitter, CreatedHeight: 50})
		return f
	}

	t.Run("happy path persists note, index, fee, event; no dirty marker", func(t *testing.T) {
		f := seedReady()
		c := newTestContract(f)
		msg := &MessageSubmitNote{Author: author, ClaimId: claimID, Body: "needs context", ContentHash: []byte{0x01}, Nonce: 4}

		resp := c.DeliverMessageSubmitNote(msg, testFee, height)
		if resp.Error != nil {
			t.Fatalf("unexpected error: %v", resp.Error)
		}
		noteID := DeriveNoteID(author, claimID, 4, height)
		note := new(Note)
		if err := Unmarshal(f.kv[string(KeyForNote(noteID))], note); err != nil {
			t.Fatalf("note not persisted: %v", err)
		}
		if !bytes.Equal(note.Id, noteID) || !bytes.Equal(note.ClaimId, claimID) || !bytes.Equal(note.Author, author) || note.Body != "needs context" || note.Status != NoteStatus_NEEDS_MORE_RATINGS {
			t.Fatalf("note record wrong: %+v", note)
		}
		if !f.has(KeyForNoteIndex(claimID, noteID)) {
			t.Fatal("claim->note index not written")
		}
		if f.has(KeyForDirtyNote(noteID)) {
			t.Fatal("a fresh note (no ratings) must not be in the dirty-set")
		}
		if got := f.accountAmount(author); got != 1_000_000-testFee {
			t.Fatalf("author balance = %d", got)
		}
		if got := f.poolAmount(1); got != testFee {
			t.Fatalf("fee pool = %d", got)
		}
		var got NoteCreatedEvent
		decodeEvent(t, resp, "note_created", &got)
		if !bytes.Equal(got.NoteId, noteID) || !bytes.Equal(got.ClaimId, claimID) || got.Body != "needs context" || got.Status != NoteStatus_NEEDS_MORE_RATINGS {
			t.Fatalf("event payload wrong: %+v", &got)
		}
	})

	t.Run("missing claim rejected, no write", func(t *testing.T) {
		f := newFakeBackend()
		f.seedAccount(author, 1_000_000)
		f.seedFeePool(1, 0)
		c := newTestContract(f)
		resp := c.DeliverMessageSubmitNote(&MessageSubmitNote{Author: author, ClaimId: claimID, Body: "x", Nonce: 1}, testFee, height)
		if resp.Error == nil || resp.Error.Code != ErrClaimNotFound().Code {
			t.Fatalf("expected ErrClaimNotFound, got %v", resp.Error)
		}
		if f.writes != 0 {
			t.Fatalf("expected no write, got %d", f.writes)
		}
	})

	t.Run("insufficient funds rejected", func(t *testing.T) {
		f := seedReady()
		f.seedAccount(author, testFee-1)
		c := newTestContract(f)
		resp := c.DeliverMessageSubmitNote(&MessageSubmitNote{Author: author, ClaimId: claimID, Body: "x", Nonce: 2}, testFee, height)
		if resp.Error == nil || resp.Error.Code != ErrInsufficientFunds().Code {
			t.Fatalf("expected ErrInsufficientFunds, got %v", resp.Error)
		}
		if f.writes != 0 {
			t.Fatalf("expected no write, got %d", f.writes)
		}
	})

	t.Run("replay is idempotent no-op", func(t *testing.T) {
		f := seedReady()
		noteID := DeriveNoteID(author, claimID, 8, height)
		f.seedNote(&Note{Id: noteID, ClaimId: claimID, Author: author, Status: NoteStatus_NEEDS_MORE_RATINGS})
		c := newTestContract(f)
		resp := c.DeliverMessageSubmitNote(&MessageSubmitNote{Author: author, ClaimId: claimID, Body: "x", Nonce: 8}, testFee, height)
		if resp.Error != nil {
			t.Fatalf("unexpected error: %v", resp.Error)
		}
		if f.writes != 0 {
			t.Fatalf("replay should not write, got %d", f.writes)
		}
		if got := f.accountAmount(author); got != 1_000_000 {
			t.Fatalf("replay charged fee: balance %d", got)
		}
	})
}

// ---- DeliverTx: RateNote --------------------------------------------------------------------

func TestDeliverMessageRateNote(t *testing.T) {
	author := addr(0xb2)
	rater := addr(0xc3)
	const height = uint64(300)
	claimID := DeriveClaimID(addr(0xa1), 1, 50)
	noteID := DeriveNoteID(author, claimID, 1, 200)

	seedReady := func() *fakeBackend {
		f := newFakeBackend()
		f.seedAccount(rater, 1_000_000)
		f.seedFeePool(1, 0)
		f.seedNote(&Note{Id: noteID, ClaimId: claimID, Author: author, Status: NoteStatus_NEEDS_MORE_RATINGS})
		return f
	}

	t.Run("happy path persists rating, marks dirty, charges fee, emits event", func(t *testing.T) {
		f := seedReady()
		c := newTestContract(f)
		msg := &MessageRateNote{Rater: rater, NoteId: noteID, Value: RatingValue_RATING_HELPFUL, Nonce: 1}

		resp := c.DeliverMessageRateNote(msg, testFee, height)
		if resp.Error != nil {
			t.Fatalf("unexpected error: %v", resp.Error)
		}
		rating := new(Rating)
		if err := Unmarshal(f.kv[string(KeyForRating(noteID, rater))], rating); err != nil {
			t.Fatalf("rating not persisted: %v", err)
		}
		if !bytes.Equal(rating.NoteId, noteID) || !bytes.Equal(rating.Rater, rater) || rating.Value != RatingValue_RATING_HELPFUL || rating.CreatedHeight != height {
			t.Fatalf("rating record wrong: %+v", rating)
		}
		if !f.has(KeyForDirtyNote(noteID)) {
			t.Fatal("note should be marked dirty for EndBlock rescore")
		}
		if got := f.accountAmount(rater); got != 1_000_000-testFee {
			t.Fatalf("rater balance = %d", got)
		}
		if got := f.poolAmount(1); got != testFee {
			t.Fatalf("fee pool = %d", got)
		}
		var got NoteRatedEvent
		decodeEvent(t, resp, "note_rated", &got)
		if !bytes.Equal(got.NoteId, noteID) || !bytes.Equal(got.Rater, rater) || got.Value != RatingValue_RATING_HELPFUL {
			t.Fatalf("event payload wrong: %+v", &got)
		}
	})

	t.Run("missing note rejected, no write", func(t *testing.T) {
		f := newFakeBackend()
		f.seedAccount(rater, 1_000_000)
		f.seedFeePool(1, 0)
		c := newTestContract(f)
		resp := c.DeliverMessageRateNote(&MessageRateNote{Rater: rater, NoteId: noteID, Value: RatingValue_RATING_HELPFUL, Nonce: 1}, testFee, height)
		if resp.Error == nil || resp.Error.Code != ErrNoteNotFound().Code {
			t.Fatalf("expected ErrNoteNotFound, got %v", resp.Error)
		}
		if f.writes != 0 {
			t.Fatalf("expected no write, got %d", f.writes)
		}
	})

	t.Run("author cannot rate own note", func(t *testing.T) {
		f := seedReady()
		f.seedAccount(author, 1_000_000)
		c := newTestContract(f)
		resp := c.DeliverMessageRateNote(&MessageRateNote{Rater: author, NoteId: noteID, Value: RatingValue_RATING_HELPFUL, Nonce: 1}, testFee, height)
		if resp.Error == nil || resp.Error.Code != ErrCannotRateOwnNote().Code {
			t.Fatalf("expected ErrCannotRateOwnNote, got %v", resp.Error)
		}
		if f.writes != 0 {
			t.Fatalf("expected no write, got %d", f.writes)
		}
	})

	t.Run("insufficient funds rejected", func(t *testing.T) {
		f := seedReady()
		f.seedAccount(rater, testFee-1)
		c := newTestContract(f)
		resp := c.DeliverMessageRateNote(&MessageRateNote{Rater: rater, NoteId: noteID, Value: RatingValue_RATING_HELPFUL, Nonce: 1}, testFee, height)
		if resp.Error == nil || resp.Error.Code != ErrInsufficientFunds().Code {
			t.Fatalf("expected ErrInsufficientFunds, got %v", resp.Error)
		}
		if f.writes != 0 {
			t.Fatalf("expected no write, got %d", f.writes)
		}
	})

	t.Run("re-vote overwrites the same rating key", func(t *testing.T) {
		f := seedReady()
		c := newTestContract(f)
		if resp := c.DeliverMessageRateNote(&MessageRateNote{Rater: rater, NoteId: noteID, Value: RatingValue_RATING_HELPFUL, Nonce: 1}, testFee, height); resp.Error != nil {
			t.Fatalf("first rating failed: %v", resp.Error)
		}
		if resp := c.DeliverMessageRateNote(&MessageRateNote{Rater: rater, NoteId: noteID, Value: RatingValue_RATING_NOT_HELPFUL, Nonce: 2}, testFee, height+1); resp.Error != nil {
			t.Fatalf("re-vote failed: %v", resp.Error)
		}
		rating := new(Rating)
		if err := Unmarshal(f.kv[string(KeyForRating(noteID, rater))], rating); err != nil {
			t.Fatalf("rating missing: %v", err)
		}
		if rating.Value != RatingValue_RATING_NOT_HELPFUL || rating.CreatedHeight != height+1 {
			t.Fatalf("re-vote did not overwrite: %+v", rating)
		}
		if got := f.accountAmount(rater); got != 1_000_000-2*testFee {
			t.Fatalf("both votes should each charge a fee: balance %d", got)
		}
	})
}
