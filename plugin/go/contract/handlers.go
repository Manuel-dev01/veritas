package contract

import (
	"bytes"
	"math/rand"

	"google.golang.org/protobuf/types/known/anypb"
)

/*
handlers.go — Veritas custom transaction handlers.

Phase 1 fully implements the SubmitClaim path (validate -> charge fee -> persist claim ->
emit ClaimCreated event). SubmitNote/RateNote are routed with stateless validation only; their
state transitions land in Phase 2. Determinism: ids derive from signed-tx fields, all state
keys are sorted/explicit, no time/rand affects state content (rand is used only for socket
QueryId correlation, exactly as the base template does).
*/

const (
	maxClaimTextLen = 280  // bound on MessageSubmitClaim.text
	maxURLLen       = 2048 // bound on a claim/note url
	maxNoteBodyLen  = 560  // bound on MessageSubmitNote.body
)

// StateBackend is the subset of the plugin connection the handlers depend on. The real
// *Plugin satisfies it (talking to the FSM over the socket); tests inject an in-memory fake
// so DeliverTx handlers can be table-tested without a running node.
type StateBackend interface {
	StateRead(c *Contract, request *PluginStateReadRequest) (*PluginStateReadResponse, *PluginError)
	StateWrite(c *Contract, request *PluginStateWriteRequest) (*PluginStateWriteResponse, *PluginError)
}

// chargeFee debits the payer and credits the fee pool, returning the resulting state ops:
// the fee-pool set, plus the payer set (or a delete if the account is drained to 0, matching
// DeliverMessageSend). It mutates payer/feePool in place and is deterministic given its inputs.
func chargeFee(payer *Account, payerKey []byte, feePool *Pool, feePoolKey []byte, fee uint64) (sets []*PluginSetOp, deletes []*PluginDeleteOp, err *PluginError) {
	if payer.Amount < fee {
		return nil, nil, ErrInsufficientFunds()
	}
	payer.Amount -= fee
	feePool.Amount += fee
	feePoolBz, e := Marshal(feePool)
	if e != nil {
		return nil, nil, e
	}
	sets = append(sets, &PluginSetOp{Key: feePoolKey, Value: feePoolBz})
	if payer.Amount == 0 {
		deletes = append(deletes, &PluginDeleteOp{Key: payerKey})
	} else {
		payerBz, e2 := Marshal(payer)
		if e2 != nil {
			return nil, nil, e2
		}
		sets = append(sets, &PluginSetOp{Key: payerKey, Value: payerBz})
	}
	return sets, deletes, nil
}

// CheckMessageSubmitClaim statelessly validates a 'submit_claim' message.
func (c *Contract) CheckMessageSubmitClaim(msg *MessageSubmitClaim) *PluginCheckResponse {
	if len(msg.Submitter) != 20 {
		return &PluginCheckResponse{Error: ErrInvalidAddress()}
	}
	// a claim must reference something: a content hash and/or a url
	if len(msg.ContentHash) == 0 && msg.Url == "" {
		return &PluginCheckResponse{Error: ErrEmptyClaim()}
	}
	if len(msg.Text) > maxClaimTextLen {
		return &PluginCheckResponse{Error: ErrTextTooLong()}
	}
	if len(msg.Url) > maxURLLen {
		return &PluginCheckResponse{Error: ErrUrlTooLong()}
	}
	return &PluginCheckResponse{AuthorizedSigners: [][]byte{msg.Submitter}}
}

// DeliverMessageSubmitClaim charges the fee, persists the claim record, and emits a
// ClaimCreated event (the authoritative RPC read path). createdHeight comes from the
// signed transaction (request.Tx.CreatedHeight), so the derived claim id is deterministic.
func (c *Contract) DeliverMessageSubmitClaim(msg *MessageSubmitClaim, fee, createdHeight uint64) *PluginDeliverResponse {
	id := DeriveClaimID(msg.Submitter, msg.Nonce, createdHeight)
	var (
		submitterKey               = KeyForAccount(msg.Submitter)
		feePoolKey                 = KeyForFeePool(c.Config.ChainId)
		claimKey                   = KeyForClaim(id)
		submitterQ, feeQ, claimQ   = rand.Uint64(), rand.Uint64(), rand.Uint64()
		submitterBz, feePoolBz, cb []byte
	)
	// batch-read the submitter account, the fee pool, and any existing claim (for idempotency)
	resp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Keys: []*PluginKeyRead{
			{QueryId: submitterQ, Key: submitterKey},
			{QueryId: feeQ, Key: feePoolKey},
			{QueryId: claimQ, Key: claimKey},
		}})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if resp.Error != nil {
		return &PluginDeliverResponse{Error: resp.Error}
	}
	for _, r := range resp.Results {
		if len(r.Entries) == 0 {
			continue
		}
		switch r.QueryId {
		case submitterQ:
			submitterBz = r.Entries[0].Value
		case feeQ:
			feePoolBz = r.Entries[0].Value
		case claimQ:
			cb = r.Entries[0].Value
		}
	}
	// idempotent on replay: if the claim already exists, do nothing
	if len(cb) > 0 {
		return &PluginDeliverResponse{}
	}
	submitter, feePool := new(Account), new(Pool)
	if e := Unmarshal(submitterBz, submitter); e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	if e := Unmarshal(feePoolBz, feePool); e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	// charge the transaction fee (submitter -> fee pool)
	feeSets, deletes, ferr := chargeFee(submitter, submitterKey, feePool, feePoolKey, fee)
	if ferr != nil {
		return &PluginDeliverResponse{Error: ferr}
	}
	// build the claim record
	claim := &Claim{
		Id:            id,
		ContentHash:   msg.ContentHash,
		Url:           msg.Url,
		Text:          msg.Text,
		Submitter:     msg.Submitter,
		CreatedHeight: createdHeight,
	}
	claimBz, e := Marshal(claim)
	if e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	sets := append([]*PluginSetOp{{Key: claimKey, Value: claimBz}}, feeSets...)
	wResp, err := c.plugin.StateWrite(c, &PluginStateWriteRequest{Sets: sets, Deletes: deletes})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if wResp.Error != nil {
		return &PluginDeliverResponse{Error: wResp.Error}
	}
	// emit the ClaimCreated event (queryable via /v1/query/events-by-height|address)
	payload, pe := anypb.New(&ClaimCreatedEvent{
		ClaimId:       id,
		Submitter:     msg.Submitter,
		ContentHash:   msg.ContentHash,
		Url:           msg.Url,
		CreatedHeight: createdHeight,
		Text:          msg.Text,
	})
	if pe != nil {
		return &PluginDeliverResponse{Error: ErrMarshal(pe)}
	}
	ev := &Event{
		EventType: "claim_created",
		Msg:       &Event_Custom{Custom: &EventCustom{Msg: payload}},
		Height:    createdHeight,
		ChainId:   c.Config.ChainId,
		Address:   msg.Submitter,
	}
	return &PluginDeliverResponse{Events: []*Event{ev}}
}

// CheckMessageSubmitNote statelessly validates a 'submit_note' message (Phase 1: validation only).
func (c *Contract) CheckMessageSubmitNote(msg *MessageSubmitNote) *PluginCheckResponse {
	if len(msg.Author) != 20 {
		return &PluginCheckResponse{Error: ErrInvalidAddress()}
	}
	if len(msg.ClaimId) == 0 {
		return &PluginCheckResponse{Error: ErrClaimNotFound()}
	}
	if msg.Body == "" || len(msg.Body) > maxNoteBodyLen {
		return &PluginCheckResponse{Error: ErrBodyTooLong()}
	}
	if len(msg.Url) > maxURLLen {
		return &PluginCheckResponse{Error: ErrUrlTooLong()}
	}
	return &PluginCheckResponse{AuthorizedSigners: [][]byte{msg.Author}}
}

// DeliverMessageSubmitNote charges the fee, persists the note record (requiring the referenced
// claim to exist), writes the claim->note enumeration index, and emits a NoteCreated event.
// The note id derives from signed-tx fields, so it is deterministic; a replay finds the note
// already present and is a no-op. A fresh note starts NEEDS_MORE_RATINGS and is NOT marked dirty
// (it has no ratings to score; the dirty-set is driven by RateNote, per the state model §4).
func (c *Contract) DeliverMessageSubmitNote(msg *MessageSubmitNote, fee, createdHeight uint64) *PluginDeliverResponse {
	noteID := DeriveNoteID(msg.Author, msg.ClaimId, msg.Nonce, createdHeight)
	var (
		authorKey                            = KeyForAccount(msg.Author)
		feePoolKey                           = KeyForFeePool(c.Config.ChainId)
		claimKey                             = KeyForClaim(msg.ClaimId)
		noteKey                              = KeyForNote(noteID)
		authorQ, feeQ, claimQ, noteQ         = rand.Uint64(), rand.Uint64(), rand.Uint64(), rand.Uint64()
		authorBz, feePoolBz, claimBz, noteBz []byte
	)
	// batch-read the author account, the fee pool, the referenced claim, and any existing note
	resp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Keys: []*PluginKeyRead{
			{QueryId: authorQ, Key: authorKey},
			{QueryId: feeQ, Key: feePoolKey},
			{QueryId: claimQ, Key: claimKey},
			{QueryId: noteQ, Key: noteKey},
		}})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if resp.Error != nil {
		return &PluginDeliverResponse{Error: resp.Error}
	}
	for _, r := range resp.Results {
		if len(r.Entries) == 0 {
			continue
		}
		switch r.QueryId {
		case authorQ:
			authorBz = r.Entries[0].Value
		case feeQ:
			feePoolBz = r.Entries[0].Value
		case claimQ:
			claimBz = r.Entries[0].Value
		case noteQ:
			noteBz = r.Entries[0].Value
		}
	}
	// the claim being annotated must exist
	if len(claimBz) == 0 {
		return &PluginDeliverResponse{Error: ErrClaimNotFound()}
	}
	// idempotent on replay: if the note already exists, do nothing
	if len(noteBz) > 0 {
		return &PluginDeliverResponse{}
	}
	author, feePool := new(Account), new(Pool)
	if e := Unmarshal(authorBz, author); e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	if e := Unmarshal(feePoolBz, feePool); e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	feeSets, deletes, ferr := chargeFee(author, authorKey, feePool, feePoolKey, fee)
	if ferr != nil {
		return &PluginDeliverResponse{Error: ferr}
	}
	note := &Note{
		Id:            noteID,
		ClaimId:       msg.ClaimId,
		Author:        msg.Author,
		Body:          msg.Body,
		ContentHash:   msg.ContentHash,
		CreatedHeight: createdHeight,
		Status:        NoteStatus_NEEDS_MORE_RATINGS,
		Url:           msg.Url,
	}
	noteRecordBz, e := Marshal(note)
	if e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	sets := []*PluginSetOp{
		{Key: noteKey, Value: noteRecordBz},
		{Key: KeyForNoteIndex(msg.ClaimId, noteID), Value: []byte{1}},
	}
	sets = append(sets, feeSets...)
	wResp, err := c.plugin.StateWrite(c, &PluginStateWriteRequest{Sets: sets, Deletes: deletes})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if wResp.Error != nil {
		return &PluginDeliverResponse{Error: wResp.Error}
	}
	// emit the NoteCreated event (queryable via /v1/query/events-by-height|address)
	payload, pe := anypb.New(&NoteCreatedEvent{
		NoteId:        noteID,
		ClaimId:       msg.ClaimId,
		Author:        msg.Author,
		Body:          msg.Body,
		ContentHash:   msg.ContentHash,
		CreatedHeight: createdHeight,
		Status:        NoteStatus_NEEDS_MORE_RATINGS,
		Url:           msg.Url,
	})
	if pe != nil {
		return &PluginDeliverResponse{Error: ErrMarshal(pe)}
	}
	ev := &Event{
		EventType: "note_created",
		Msg:       &Event_Custom{Custom: &EventCustom{Msg: payload}},
		Height:    createdHeight,
		ChainId:   c.Config.ChainId,
		Address:   msg.Author,
	}
	return &PluginDeliverResponse{Events: []*Event{ev}}
}

// CheckMessageRateNote statelessly validates a 'rate_note' message (Phase 1: validation only).
func (c *Contract) CheckMessageRateNote(msg *MessageRateNote) *PluginCheckResponse {
	if len(msg.Rater) != 20 {
		return &PluginCheckResponse{Error: ErrInvalidAddress()}
	}
	if len(msg.NoteId) == 0 {
		return &PluginCheckResponse{Error: ErrNoteNotFound()}
	}
	if msg.Value == RatingValue_RATING_UNSPECIFIED {
		return &PluginCheckResponse{Error: ErrInvalidRatingValue()}
	}
	return &PluginCheckResponse{AuthorizedSigners: [][]byte{msg.Rater}}
}

// DeliverMessageRateNote charges the fee, persists the rater's rating of a note (requiring the
// note to exist and forbidding an author from rating their own note), marks the note dirty for
// the EndBlock rescore (Phase 3), and emits a NoteRated event. The rating key is (noteId, rater),
// so a genuine re-vote (new nonce -> new tx) overwrites the same key; exact replays are rejected
// upstream by the FSM's tx-hash de-duplication.
func (c *Contract) DeliverMessageRateNote(msg *MessageRateNote, fee, createdHeight uint64) *PluginDeliverResponse {
	var (
		raterKey                   = KeyForAccount(msg.Rater)
		feePoolKey                 = KeyForFeePool(c.Config.ChainId)
		noteKey                    = KeyForNote(msg.NoteId)
		raterQ, feeQ, noteQ        = rand.Uint64(), rand.Uint64(), rand.Uint64()
		raterBz, feePoolBz, noteBz []byte
	)
	// batch-read the rater account, the fee pool, and the note being rated
	resp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Keys: []*PluginKeyRead{
			{QueryId: raterQ, Key: raterKey},
			{QueryId: feeQ, Key: feePoolKey},
			{QueryId: noteQ, Key: noteKey},
		}})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if resp.Error != nil {
		return &PluginDeliverResponse{Error: resp.Error}
	}
	for _, r := range resp.Results {
		if len(r.Entries) == 0 {
			continue
		}
		switch r.QueryId {
		case raterQ:
			raterBz = r.Entries[0].Value
		case feeQ:
			feePoolBz = r.Entries[0].Value
		case noteQ:
			noteBz = r.Entries[0].Value
		}
	}
	// the note being rated must exist
	if len(noteBz) == 0 {
		return &PluginDeliverResponse{Error: ErrNoteNotFound()}
	}
	note := new(Note)
	if e := Unmarshal(noteBz, note); e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	// an author cannot rate their own note (keeps the bridging signal honest)
	if bytes.Equal(note.Author, msg.Rater) {
		return &PluginDeliverResponse{Error: ErrCannotRateOwnNote()}
	}
	rater, feePool := new(Account), new(Pool)
	if e := Unmarshal(raterBz, rater); e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	if e := Unmarshal(feePoolBz, feePool); e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	feeSets, deletes, ferr := chargeFee(rater, raterKey, feePool, feePoolKey, fee)
	if ferr != nil {
		return &PluginDeliverResponse{Error: ferr}
	}
	rating := &Rating{
		NoteId:        msg.NoteId,
		Rater:         msg.Rater,
		Value:         msg.Value,
		CreatedHeight: createdHeight,
	}
	ratingBz, e := Marshal(rating)
	if e != nil {
		return &PluginDeliverResponse{Error: e}
	}
	sets := []*PluginSetOp{
		{Key: KeyForRating(msg.NoteId, msg.Rater), Value: ratingBz},
		{Key: KeyForDirtyNote(msg.NoteId), Value: []byte{1}},
	}
	sets = append(sets, feeSets...)
	wResp, err := c.plugin.StateWrite(c, &PluginStateWriteRequest{Sets: sets, Deletes: deletes})
	if err != nil {
		return &PluginDeliverResponse{Error: err}
	}
	if wResp.Error != nil {
		return &PluginDeliverResponse{Error: wResp.Error}
	}
	// emit the NoteRated event (queryable via /v1/query/events-by-height|address)
	payload, pe := anypb.New(&NoteRatedEvent{
		NoteId:        msg.NoteId,
		Rater:         msg.Rater,
		Value:         msg.Value,
		CreatedHeight: createdHeight,
	})
	if pe != nil {
		return &PluginDeliverResponse{Error: ErrMarshal(pe)}
	}
	ev := &Event{
		EventType: "note_rated",
		Msg:       &Event_Custom{Custom: &EventCustom{Msg: payload}},
		Height:    createdHeight,
		ChainId:   c.Config.ChainId,
		Address:   msg.Rater,
	}
	return &PluginDeliverResponse{Events: []*Event{ev}}
}
