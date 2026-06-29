package contract

import (
	"sort"
)

/*
scoring.go — pure, deterministic fixed-point bridging math (CLAUDE.md §3/§5).

This file holds ONLY pure functions (input slices/structs -> output structs; no I/O, no state
handles, no float/rand/time/map-order). Isolating the consensus-critical scoring here keeps it
unit-testable for determinism (scoring_test.go runs identical-output and shuffled-input-order
assertions).

The "bridging" idea (a simplified, fully deterministic approximation of Community Notes): a note is
HELPFUL only when raters who *normally disagree* both find it helpful. v1 splits raters into two
cohorts by the sign of their average deviation from per-note consensus — a "generous vs. harsh"
polarity proxy for "people who normally disagree" — then scores a note by the MINIMUM of the two
cohorts' mean helpfulness, so a high score requires cross-divide agreement.

Determinism rules enforced here:
  - No float anywhere; rating values map to fixed-point integers scaled by 1000.
  - Polarity uses only the SIGN of a signed integer sum -> no signed division, no rounding ambiguity.
  - The only divisions are non-negative integer means; Go integer division rounds toward zero, which
    for non-negative operands is floor (round-down) — the single rounding rule used throughout.
  - Every map is converted to a SORTED slice before iteration (sorted by string(id bytes)).
*/

// fixed-point scale: all fractional values are integers scaled by 1000 (0.5 -> 500).
const fixedPointScale = 1000

// fixed-point rating values.
const (
	fpHelpful    = 1000
	fpSomewhat   = 500
	fpNotHelpful = 0
)

// scorer tuning (named so they're easy to explain in the demo and easy to tune).
const (
	minCohortRatings    = 2   // each cohort must have at least this many ratings to score a note
	helpfulThreshold    = 600 // bridge score >= this -> HELPFUL
	notHelpfulThreshold = 250 // bridge score <= this -> NOT_HELPFUL
)

// RatingInput is one (note, rater, value) observation fed to the scorer (decoded from state).
type RatingInput struct {
	NoteID []byte
	Rater  []byte
	Value  RatingValue
}

// NoteScore is the scorer's verdict for a single note (carries the breakdown for the event/UI).
type NoteScore struct {
	NoteID []byte
	Status NoteStatus
	Bridge int64 // min(MeanA, MeanB) when both cohorts qualify, else 0
	MeanA  int64
	MeanB  int64
	CountA int
	CountB int
}

// ratingToFP maps a rating enum to its fixed-point helpfulness; ok=false for UNSPECIFIED (skipped).
func ratingToFP(v RatingValue) (int64, bool) {
	switch v {
	case RatingValue_RATING_HELPFUL:
		return fpHelpful, true
	case RatingValue_RATING_SOMEWHAT:
		return fpSomewhat, true
	case RatingValue_RATING_NOT_HELPFUL:
		return fpNotHelpful, true
	default:
		return 0, false
	}
}

// meanFP returns the round-down (floor, for non-negative inputs) fixed-point mean of sum/count.
func meanFP(sum int64, count int) int64 {
	if count == 0 {
		return 0
	}
	return sum / int64(count)
}

// ScoreNotes computes the status of each target note from the full set of ratings, applying the
// simplified bridging algorithm. It is pure and deterministic: output depends only on the input
// values, not their order. Returns one NoteScore per target, ordered by target note id.
func ScoreNotes(all []RatingInput, targets [][]byte) []NoteScore {
	// noteRatings[noteId] = list of (rater, valueFP); raterNotes[rater] = list of (noteId, valueFP).
	type rv struct {
		id  string // counterpart id (rater for noteRatings, note for raterNotes)
		val int64
	}
	noteRatings := map[string][]rv{}
	raterNotes := map[string][]rv{}
	for _, r := range all {
		fp, ok := ratingToFP(r.Value)
		if !ok {
			continue // defensively skip unspecified/invalid (CheckTx already rejects these)
		}
		nid, rid := string(r.NoteID), string(r.Rater)
		noteRatings[nid] = append(noteRatings[nid], rv{id: rid, val: fp})
		raterNotes[rid] = append(raterNotes[rid], rv{id: nid, val: fp})
	}

	// (1) per-note consensus mean over ALL raters (cohort-agnostic), iterating sorted note ids.
	consensus := map[string]int64{}
	for _, nid := range sortedKeys(noteRatings) {
		var sum int64
		rs := noteRatings[nid]
		for _, e := range rs {
			sum += e.val
		}
		consensus[nid] = meanFP(sum, len(rs))
	}

	// (2) per-rater polarity = sign of summed deviation from consensus across their rated notes.
	// cohortA = deviationSum >= 0 (generous/neutral); cohortB = deviationSum < 0 (harsh). Iterate
	// sorted rater ids and sorted note ids within each rater so the (unused) order is deterministic.
	cohortA := map[string]bool{}
	for _, rid := range sortedKeys(raterNotes) {
		var devSum int64
		notes := raterNotes[rid]
		sort.Slice(notes, func(i, j int) bool { return notes[i].id < notes[j].id })
		for _, e := range notes {
			devSum += e.val - consensus[e.id]
		}
		cohortA[rid] = devSum >= 0 // tie/zero -> cohort A
	}

	// (3)+(4) score each target note: partition its raters by cohort, mean each cohort, bridge = min.
	out := make([]NoteScore, 0, len(targets))
	sortedTargets := append([][]byte(nil), targets...)
	sort.Slice(sortedTargets, func(i, j int) bool { return string(sortedTargets[i]) < string(sortedTargets[j]) })
	for _, target := range sortedTargets {
		nid := string(target)
		score := NoteScore{NoteID: target, Status: NoteStatus_NEEDS_MORE_RATINGS}
		rs := append([]rv(nil), noteRatings[nid]...)
		sort.Slice(rs, func(i, j int) bool { return rs[i].id < rs[j].id })
		var sumA, sumB int64
		for _, e := range rs {
			if cohortA[e.id] {
				sumA += e.val
				score.CountA++
			} else {
				sumB += e.val
				score.CountB++
			}
		}
		score.MeanA = meanFP(sumA, score.CountA)
		score.MeanB = meanFP(sumB, score.CountB)
		// require cross-divide coverage: at least minCohortRatings in EACH cohort
		if score.CountA < minCohortRatings || score.CountB < minCohortRatings {
			score.Status = NoteStatus_NEEDS_MORE_RATINGS
			out = append(out, score)
			continue
		}
		score.Bridge = score.MeanA
		if score.MeanB < score.Bridge {
			score.Bridge = score.MeanB // min(meanA, meanB)
		}
		switch {
		case score.Bridge >= helpfulThreshold:
			score.Status = NoteStatus_HELPFUL
		case score.Bridge <= notHelpfulThreshold:
			score.Status = NoteStatus_NOT_HELPFUL
		default:
			score.Status = NoteStatus_NEEDS_MORE_RATINGS
		}
		out = append(out, score)
	}
	return out
}

// sortedKeys returns the keys of a map[string][]rv-like map in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
