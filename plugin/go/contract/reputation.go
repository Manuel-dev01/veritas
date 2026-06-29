package contract

import "sort"

/*
reputation.go — pure, deterministic reputation math (CLAUDE.md §5, Phase 4).

Non-transferable reputation is minted/burned only by the protocol in EndBlock; no transaction can
move it. Honest participation builds it, bad-faith participation decays it:
  - When a note resolves HELPFUL, its author and the raters who called it helpful gain; raters on
    the wrong side lose a little.
  - Symmetric for a NOT_HELPFUL resolution (penalize the misleading author + the wrong side, reward
    the raters who correctly flagged it). The spec details only the HELPFUL case; the symmetric
    NOT_HELPFUL handling is a deliberate extension serving "bad-faith decays it".
  - Inactivity decay nibbles idle accounts toward zero each block.

Like scoring.go these are PURE functions (no I/O, no float/rand/time/map-order). All fractional
values are integers scaled by 1000; reputation floors at 0 (gains are uncapped).
*/

// reputation tuning (named so they're easy to explain in the demo and easy to tune).
const (
	repAuthorHelpfulGain    = 1000 // author of a note that resolves HELPFUL
	repRaterCorrectGain     = 500  // rater on the winning side of a resolution
	repRaterWrongLoss       = 250  // rater on the losing side of a resolution
	repAuthorNotHelpfulLoss = 500  // author of a note that resolves NOT_HELPFUL

	decayThresholdBlocks = 10  // blocks of inactivity before decay begins
	decayPerBlock        = 100 // amount removed each block once past the threshold
)

// RepDelta is a single reputation adjustment with a human-readable reason (carried into the event).
type RepDelta struct {
	Account []byte
	Delta   int64
	Reason  string
}

// repDeltasForResolution returns the per-account reputation deltas produced when a note resolves to
// a terminal status, derived from its author and its ratings. Output is ordered by account id for
// determinism; somewhat ratings are neutral. Returns nil for a non-terminal status.
func repDeltasForResolution(status NoteStatus, author []byte, noteRatings []RatingInput) []RepDelta {
	var authorDelta int64
	var authorReason string
	var helpfulReason, notHelpfulReason string // reasons applied to helpful / not-helpful raters
	var helpfulDelta, notHelpfulDelta int64
	switch status {
	case NoteStatus_HELPFUL:
		authorDelta, authorReason = repAuthorHelpfulGain, "author_helpful"
		helpfulDelta, helpfulReason = repRaterCorrectGain, "rater_correct"
		notHelpfulDelta, notHelpfulReason = -repRaterWrongLoss, "rater_wrong"
	case NoteStatus_NOT_HELPFUL:
		authorDelta, authorReason = -repAuthorNotHelpfulLoss, "author_not_helpful"
		helpfulDelta, helpfulReason = -repRaterWrongLoss, "rater_wrong"
		notHelpfulDelta, notHelpfulReason = repRaterCorrectGain, "rater_correct"
	default:
		return nil // not a terminal resolution
	}

	deltas := []RepDelta{{Account: author, Delta: authorDelta, Reason: authorReason}}
	// sort the note's ratings by rater id so deltas are produced deterministically
	sorted := append([]RatingInput(nil), noteRatings...)
	sort.Slice(sorted, func(i, j int) bool { return string(sorted[i].Rater) < string(sorted[j].Rater) })
	for _, r := range sorted {
		switch r.Value {
		case RatingValue_RATING_HELPFUL:
			deltas = append(deltas, RepDelta{Account: r.Rater, Delta: helpfulDelta, Reason: helpfulReason})
		case RatingValue_RATING_NOT_HELPFUL:
			deltas = append(deltas, RepDelta{Account: r.Rater, Delta: notHelpfulDelta, Reason: notHelpfulReason})
			// RATING_SOMEWHAT (and anything else) is neutral
		}
	}
	// final ordering by account id (author included), stable & deterministic
	sort.SliceStable(deltas, func(i, j int) bool { return string(deltas[i].Account) < string(deltas[j].Account) })
	return deltas
}

// applyDelta adds delta to score, flooring at 0 (reputation is never negative).
func applyDelta(score, delta int64) int64 {
	score += delta
	if score < 0 {
		return 0
	}
	return score
}

// decayStep applies one block of inactivity decay: if the account has been inactive longer than the
// threshold and still has reputation, remove decayPerBlock (floored at 0); otherwise unchanged.
func decayStep(score int64, inactiveBlocks uint64) int64 {
	if score <= 0 || inactiveBlocks <= decayThresholdBlocks {
		return score
	}
	score -= decayPerBlock
	if score < 0 {
		return 0
	}
	return score
}
