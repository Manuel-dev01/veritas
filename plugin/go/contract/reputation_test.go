package contract

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

/*
reputation_test.go — correctness + determinism for the pure reputation math.
*/

func deltasKey(ds []RepDelta) string {
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = fmt.Sprintf("%x=%d(%s)", d.Account, d.Delta, d.Reason)
	}
	return strings.Join(parts, "|")
}

func findDelta(ds []RepDelta, account []byte) (RepDelta, bool) {
	for _, d := range ds {
		if string(d.Account) == string(account) {
			return d, true
		}
	}
	return RepDelta{}, false
}

func TestRepDeltasForResolution(t *testing.T) {
	author := addr(0xAA)
	ah := addr(0xA1) // helpful rater
	bh := addr(0xB1) // helpful rater
	cn := addr(0xC1) // not-helpful rater
	ds := addr(0xD1) // somewhat rater (neutral)
	noteRatings := []RatingInput{
		rate(nil, ah, RatingValue_RATING_HELPFUL),
		rate(nil, bh, RatingValue_RATING_HELPFUL),
		rate(nil, cn, RatingValue_RATING_NOT_HELPFUL),
		rate(nil, ds, RatingValue_RATING_SOMEWHAT),
	}

	t.Run("HELPFUL resolution rewards author + helpful, penalizes not-helpful, neutral somewhat", func(t *testing.T) {
		out := repDeltasForResolution(NoteStatus_HELPFUL, author, noteRatings)
		if d, _ := findDelta(out, author); d.Delta != repAuthorHelpfulGain || d.Reason != "author_helpful" {
			t.Fatalf("author delta wrong: %+v", d)
		}
		if d, _ := findDelta(out, ah); d.Delta != repRaterCorrectGain || d.Reason != "rater_correct" {
			t.Fatalf("helpful rater delta wrong: %+v", d)
		}
		if d, _ := findDelta(out, cn); d.Delta != -repRaterWrongLoss || d.Reason != "rater_wrong" {
			t.Fatalf("not-helpful rater delta wrong: %+v", d)
		}
		if _, ok := findDelta(out, ds); ok {
			t.Fatalf("somewhat rater should be neutral (no delta)")
		}
		if len(out) != 4 { // author + 2 helpful + 1 not-helpful
			t.Fatalf("expected 4 deltas, got %d: %s", len(out), deltasKey(out))
		}
	})

	t.Run("NOT_HELPFUL resolution is symmetric", func(t *testing.T) {
		out := repDeltasForResolution(NoteStatus_NOT_HELPFUL, author, noteRatings)
		if d, _ := findDelta(out, author); d.Delta != -repAuthorNotHelpfulLoss || d.Reason != "author_not_helpful" {
			t.Fatalf("author delta wrong: %+v", d)
		}
		if d, _ := findDelta(out, cn); d.Delta != repRaterCorrectGain || d.Reason != "rater_correct" {
			t.Fatalf("not-helpful rater should gain: %+v", d)
		}
		if d, _ := findDelta(out, ah); d.Delta != -repRaterWrongLoss || d.Reason != "rater_wrong" {
			t.Fatalf("helpful rater should lose: %+v", d)
		}
	})

	t.Run("non-terminal status -> no deltas", func(t *testing.T) {
		if out := repDeltasForResolution(NoteStatus_NEEDS_MORE_RATINGS, author, noteRatings); out != nil {
			t.Fatalf("expected nil, got %s", deltasKey(out))
		}
	})

	t.Run("output sorted by account, order-independent", func(t *testing.T) {
		baseline := deltasKey(repDeltasForResolution(NoteStatus_HELPFUL, author, noteRatings))
		// sorted ascending check
		out := repDeltasForResolution(NoteStatus_HELPFUL, author, noteRatings)
		for i := 1; i < len(out); i++ {
			if string(out[i-1].Account) > string(out[i].Account) {
				t.Fatalf("deltas not sorted by account: %s", deltasKey(out))
			}
		}
		for i := 0; i < 500; i++ {
			shuffled := append([]RatingInput(nil), noteRatings...)
			r := rand.New(rand.NewSource(int64(i)))
			r.Shuffle(len(shuffled), func(x, y int) { shuffled[x], shuffled[y] = shuffled[y], shuffled[x] })
			if got := deltasKey(repDeltasForResolution(NoteStatus_HELPFUL, author, shuffled)); got != baseline {
				t.Fatalf("seed %d differs:\n got=%s\nwant=%s", i, got, baseline)
			}
		}
	})
}

func TestApplyDelta_FloorsAtZero(t *testing.T) {
	if got := applyDelta(100, 500); got != 600 {
		t.Fatalf("gain: got %d", got)
	}
	if got := applyDelta(100, -250); got != 0 {
		t.Fatalf("loss should floor at 0: got %d", got)
	}
	if got := applyDelta(0, -1000); got != 0 {
		t.Fatalf("loss from 0 stays 0: got %d", got)
	}
}

func TestDecayStep(t *testing.T) {
	// within threshold -> unchanged
	if got := decayStep(1000, decayThresholdBlocks); got != 1000 {
		t.Fatalf("within threshold should not decay: got %d", got)
	}
	// past threshold -> decays one step
	if got := decayStep(1000, decayThresholdBlocks+1); got != 1000-decayPerBlock {
		t.Fatalf("past threshold should decay: got %d", got)
	}
	// floors at 0
	if got := decayStep(50, decayThresholdBlocks+100); got != 0 {
		t.Fatalf("decay should floor at 0: got %d", got)
	}
	// zero stays zero
	if got := decayStep(0, decayThresholdBlocks+100); got != 0 {
		t.Fatalf("0 stays 0: got %d", got)
	}
}
