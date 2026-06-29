package contract

import (
	"math/rand"
	"sort"

	"google.golang.org/protobuf/types/known/anypb"
)

/*
endblock.go — Veritas per-block recompute hook.

Each block EndBlock: (1) rescore the notes marked dirty by RateNote via the pure bridging scorer
(scoring.go), persisting status changes and emitting a NoteScoredEvent per rescore; (2) when a note
transitions into a terminal status, mint/burn non-transferable reputation for its author and raters
(reputation.go); (3) decay the reputation of inactive accounts. Reputation is only ever changed here
by the protocol — no transaction can move it — so it is inherently non-transferable.

Determinism: ids/accounts/keys are sorted before iteration; scoring + reputation math is pure integer
arithmetic; block height comes from the request (never wall-clock); rand is used only for socket
QueryId correlation. State writes issued here commit to the block being applied.

v1 note: polarity, reputation and decay are recomputed from GLOBAL range-scans each block — bounded
by total ratings/accounts (fine at demo scale). A production version would maintain incremental
aggregates and lazy decay; the dirty-set already bounds which notes are (re)scored.
*/

// EndBlock() is code that is executed at the end of 'applying' a block.
func (c *Contract) EndBlock(req *PluginEndRequest) *PluginEndResponse {
	height := req.Height

	// (1) read all reputation records (needed for decay every block).
	repResp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Ranges: []*PluginRangeRead{{QueryId: rand.Uint64(), Prefix: repScanPrefix()}},
	})
	if err != nil {
		return &PluginEndResponse{Error: err}
	}
	if repResp.Error != nil {
		return &PluginEndResponse{Error: repResp.Error}
	}
	repMap := map[string]*Reputation{}
	for _, r := range repResp.Results {
		for _, e := range r.Entries {
			rec := new(Reputation)
			if e2 := Unmarshal(e.Value, rec); e2 != nil {
				return &PluginEndResponse{Error: e2}
			}
			repMap[string(rec.Account)] = rec
		}
	}

	// (2) read the dirty-set (note id is the last key segment).
	dResp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Ranges: []*PluginRangeRead{{QueryId: rand.Uint64(), Prefix: dirtyNoteScanPrefix()}},
	})
	if err != nil {
		return &PluginEndResponse{Error: err}
	}
	if dResp.Error != nil {
		return &PluginEndResponse{Error: dResp.Error}
	}
	var dirtyIDs [][]byte
	for _, r := range dResp.Results {
		for _, e := range r.Entries {
			segs := splitLenPrefix(e.Key)
			if len(segs) < 2 {
				continue
			}
			dirtyIDs = append(dirtyIDs, segs[len(segs)-1])
		}
	}
	sort.Slice(dirtyIDs, func(i, j int) bool { return string(dirtyIDs[i]) < string(dirtyIDs[j]) })

	// nothing rated and no reputation to decay -> no work
	if len(dirtyIDs) == 0 && len(repMap) == 0 {
		return &PluginEndResponse{}
	}

	// (3) read ALL ratings (values self-describe note/rater/value); track latest activity per rater.
	rResp, err := c.plugin.StateRead(c, &PluginStateReadRequest{
		Ranges: []*PluginRangeRead{{QueryId: rand.Uint64(), Prefix: ratingScanPrefix()}},
	})
	if err != nil {
		return &PluginEndResponse{Error: err}
	}
	if rResp.Error != nil {
		return &PluginEndResponse{Error: rResp.Error}
	}
	var ratings []RatingInput
	ratingsByNote := map[string][]RatingInput{}
	latestRatingHeight := map[string]uint64{}
	for _, r := range rResp.Results {
		for _, e := range r.Entries {
			rec := new(Rating)
			if e2 := Unmarshal(e.Value, rec); e2 != nil {
				return &PluginEndResponse{Error: e2}
			}
			ri := RatingInput{NoteID: rec.NoteId, Rater: rec.Rater, Value: rec.Value}
			ratings = append(ratings, ri)
			ratingsByNote[string(rec.NoteId)] = append(ratingsByNote[string(rec.NoteId)], ri)
			if rec.CreatedHeight > latestRatingHeight[string(rec.Rater)] {
				latestRatingHeight[string(rec.Rater)] = rec.CreatedHeight
			}
		}
	}

	var (
		sets    []*PluginSetOp
		deletes []*PluginDeleteOp
		events  []*Event
	)
	changedRep := map[string]bool{}

	// (4) score the dirty notes and detect terminal transitions.
	if len(dirtyIDs) > 0 {
		keys := make([]*PluginKeyRead, 0, len(dirtyIDs))
		for _, id := range dirtyIDs {
			keys = append(keys, &PluginKeyRead{QueryId: rand.Uint64(), Key: KeyForNote(id)})
		}
		nResp, nerr := c.plugin.StateRead(c, &PluginStateReadRequest{Keys: keys})
		if nerr != nil {
			return &PluginEndResponse{Error: nerr}
		}
		if nResp.Error != nil {
			return &PluginEndResponse{Error: nResp.Error}
		}
		notesByID := map[string]*Note{}
		for _, r := range nResp.Results {
			if len(r.Entries) == 0 {
				continue
			}
			note := new(Note)
			if e2 := Unmarshal(r.Entries[0].Value, note); e2 != nil {
				return &PluginEndResponse{Error: e2}
			}
			notesByID[string(note.Id)] = note
		}

		scores := ScoreNotes(ratings, dirtyIDs)
		for _, sc := range scores {
			deletes = append(deletes, &PluginDeleteOp{Key: KeyForDirtyNote(sc.NoteID)})
			note, ok := notesByID[string(sc.NoteID)]
			if !ok {
				continue // marker still cleared
			}
			oldStatus := note.Status
			if sc.Status != oldStatus {
				note.Status = sc.Status
				nb, e2 := Marshal(note)
				if e2 != nil {
					return &PluginEndResponse{Error: e2}
				}
				sets = append(sets, &PluginSetOp{Key: KeyForNote(sc.NoteID), Value: nb})
			}
			// note_scored event (every rescore)
			scoredPayload, pe := anypb.New(&NoteScoredEvent{
				NoteId: sc.NoteID, ClaimId: note.ClaimId, Status: sc.Status, BridgeScore: sc.Bridge,
				MeanA: sc.MeanA, MeanB: sc.MeanB, CountA: uint32(sc.CountA), CountB: uint32(sc.CountB), Height: height,
			})
			if pe != nil {
				return &PluginEndResponse{Error: ErrMarshal(pe)}
			}
			events = append(events, &Event{
				EventType: "note_scored", Msg: &Event_Custom{Custom: &EventCustom{Msg: scoredPayload}},
				Height: height, Reference: "end_block", ChainId: c.Config.ChainId, Address: note.Author,
			})

			// reputation on a terminal transition (resolution)
			if sc.Status != oldStatus && (sc.Status == NoteStatus_HELPFUL || sc.Status == NoteStatus_NOT_HELPFUL) {
				for _, d := range repDeltasForResolution(sc.Status, note.Author, ratingsByNote[string(sc.NoteID)]) {
					ev, rerr := c.applyRepDelta(repMap, changedRep, d, height)
					if rerr != nil {
						return &PluginEndResponse{Error: rerr}
					}
					events = append(events, ev)
				}
			}
		}
	}

	// (5) inactivity decay over all reputation records (sorted accounts).
	repAccts := make([]string, 0, len(repMap))
	for acct := range repMap {
		repAccts = append(repAccts, acct)
	}
	sort.Strings(repAccts)
	for _, acct := range repAccts {
		rec := repMap[acct]
		effectiveLast := rec.LastActiveHeight
		if h := latestRatingHeight[acct]; h > effectiveLast {
			effectiveLast = h
		}
		var inactive uint64
		if height > effectiveLast {
			inactive = height - effectiveLast
		}
		newScore := decayStep(rec.ScoreFp, inactive)
		if newScore != rec.ScoreFp {
			delta := newScore - rec.ScoreFp
			rec.ScoreFp = newScore
			changedRep[acct] = true
			ev, rerr := c.repEvent(rec.Account, newScore, delta, "decay", height)
			if rerr != nil {
				return &PluginEndResponse{Error: rerr}
			}
			events = append(events, ev)
		}
	}

	// (6) persist changed reputation records (sorted accounts), then commit everything.
	changedAccts := make([]string, 0, len(changedRep))
	for acct := range changedRep {
		changedAccts = append(changedAccts, acct)
	}
	sort.Strings(changedAccts)
	for _, acct := range changedAccts {
		rb, e2 := Marshal(repMap[acct])
		if e2 != nil {
			return &PluginEndResponse{Error: e2}
		}
		sets = append(sets, &PluginSetOp{Key: KeyForRep(repMap[acct].Account), Value: rb})
	}

	if len(sets) > 0 || len(deletes) > 0 {
		wResp, werr := c.plugin.StateWrite(c, &PluginStateWriteRequest{Sets: sets, Deletes: deletes})
		if werr != nil {
			return &PluginEndResponse{Error: werr}
		}
		if wResp.Error != nil {
			return &PluginEndResponse{Error: wResp.Error}
		}
	}
	return &PluginEndResponse{Events: events}
}

// applyRepDelta applies one resolution delta to the in-memory reputation map (creating the record if
// missing), floors at 0, refreshes LastActiveHeight (participation = activity), marks the account
// changed, and returns the reputation_changed event.
func (c *Contract) applyRepDelta(repMap map[string]*Reputation, changed map[string]bool, d RepDelta, height uint64) (*Event, *PluginError) {
	acct := string(d.Account)
	rec, ok := repMap[acct]
	if !ok {
		rec = &Reputation{Account: d.Account}
		repMap[acct] = rec
	}
	rec.ScoreFp = applyDelta(rec.ScoreFp, d.Delta)
	rec.LastActiveHeight = height
	changed[acct] = true
	return c.repEvent(rec.Account, rec.ScoreFp, d.Delta, d.Reason, height)
}

// repEvent builds a reputation_changed event (Address=account, Reference="end_block").
func (c *Contract) repEvent(account []byte, scoreFp, delta int64, reason string, height uint64) (*Event, *PluginError) {
	payload, pe := anypb.New(&ReputationChangedEvent{
		Account: account, ScoreFp: scoreFp, Delta: delta, Reason: reason, Height: height,
	})
	if pe != nil {
		return nil, ErrMarshal(pe)
	}
	return &Event{
		EventType: "reputation_changed", Msg: &Event_Custom{Custom: &EventCustom{Msg: payload}},
		Height: height, Reference: "end_block", ChainId: c.Config.ChainId, Address: account,
	}, nil
}
