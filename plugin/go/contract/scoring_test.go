package contract

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

/*
scoring_test.go — correctness + determinism for the pure bridging scorer.

Correctness pins the demo-defining behavior: a seed note establishes two opposing cohorts (A =
"generous", B = "harsh"), then a target note is HELPFUL only when BOTH cohorts rate it helpful;
one cohort alone is never enough. Determinism runs identical inputs 1000x and many shuffled input
orderings, asserting byte-identical output (the §5 requirement).
*/

// nid builds a deterministic 32-byte note id filled with b.
func nid(b byte) []byte {
	id := make([]byte, 32)
	for i := range id {
		id[i] = b
	}
	return id
}

// scoresKey serializes []NoteScore into a stable string for equality comparison.
func scoresKey(scores []NoteScore) string {
	parts := make([]string, len(scores))
	for i, s := range scores {
		parts[i] = fmt.Sprintf("%x:%d:%d:%d:%d:%d:%d", s.NoteID, s.Status, s.Bridge, s.MeanA, s.MeanB, s.CountA, s.CountB)
	}
	return strings.Join(parts, "|")
}

// rate is a tiny constructor for a RatingInput.
func rate(note, rater []byte, v RatingValue) RatingInput {
	return RatingInput{NoteID: note, Rater: rater, Value: v}
}

var (
	a1 = addr(0xA1)
	a2 = addr(0xA2)
	b1 = addr(0xB1)
	b2 = addr(0xB2)
	nS = nid(0x55) // seed note that establishes polarity
	nT = nid(0x77) // target note under test
)

// seedPolarity returns ratings on note S that put A1,A2 in cohort A (generous) and B1,B2 in cohort B
// (harsh): A-raters rate S helpful, B-raters rate S not-helpful (consensus 500 -> A dev +500, B -500).
func seedPolarity() []RatingInput {
	return []RatingInput{
		rate(nS, a1, RatingValue_RATING_HELPFUL),
		rate(nS, a2, RatingValue_RATING_HELPFUL),
		rate(nS, b1, RatingValue_RATING_NOT_HELPFUL),
		rate(nS, b2, RatingValue_RATING_NOT_HELPFUL),
	}
}

func scoreOf(t *testing.T, all []RatingInput, target []byte) NoteScore {
	t.Helper()
	res := ScoreNotes(all, [][]byte{target})
	if len(res) != 1 {
		t.Fatalf("expected 1 score, got %d", len(res))
	}
	return res[0]
}

func TestScoreNotes_Correctness(t *testing.T) {
	t.Run("one cohort only -> NEEDS_MORE_RATINGS", func(t *testing.T) {
		all := append(seedPolarity(),
			rate(nT, a1, RatingValue_RATING_HELPFUL),
			rate(nT, a2, RatingValue_RATING_HELPFUL),
		)
		s := scoreOf(t, all, nT)
		if s.Status != NoteStatus_NEEDS_MORE_RATINGS {
			t.Fatalf("status=%v, want NEEDS_MORE_RATINGS (%+v)", s.Status, s)
		}
		if s.CountA != 2 || s.CountB != 0 {
			t.Fatalf("expected countA=2 countB=0, got %+v", s)
		}
	})

	t.Run("cross-cohort helpful -> HELPFUL", func(t *testing.T) {
		all := append(seedPolarity(),
			rate(nT, a1, RatingValue_RATING_HELPFUL),
			rate(nT, a2, RatingValue_RATING_HELPFUL),
			rate(nT, b1, RatingValue_RATING_HELPFUL),
			rate(nT, b2, RatingValue_RATING_HELPFUL),
		)
		s := scoreOf(t, all, nT)
		if s.Status != NoteStatus_HELPFUL {
			t.Fatalf("status=%v, want HELPFUL (%+v)", s.Status, s)
		}
		if s.MeanA != 1000 || s.MeanB != 1000 || s.Bridge != 1000 {
			t.Fatalf("expected means/bridge 1000, got %+v", s)
		}
		if s.CountA != 2 || s.CountB != 2 {
			t.Fatalf("expected 2/2 cohort split, got %+v", s)
		}
	})

	t.Run("both cohorts not-helpful -> NOT_HELPFUL", func(t *testing.T) {
		all := append(seedPolarity(),
			rate(nT, a1, RatingValue_RATING_NOT_HELPFUL),
			rate(nT, a2, RatingValue_RATING_NOT_HELPFUL),
			rate(nT, b1, RatingValue_RATING_NOT_HELPFUL),
			rate(nT, b2, RatingValue_RATING_NOT_HELPFUL),
		)
		s := scoreOf(t, all, nT)
		if s.Status != NoteStatus_NOT_HELPFUL {
			t.Fatalf("status=%v, want NOT_HELPFUL (%+v)", s.Status, s)
		}
		if s.Bridge != 0 {
			t.Fatalf("expected bridge 0, got %+v", s)
		}
	})

	t.Run("single rater -> NEEDS_MORE_RATINGS", func(t *testing.T) {
		all := append(seedPolarity(), rate(nT, a1, RatingValue_RATING_HELPFUL))
		s := scoreOf(t, all, nT)
		if s.Status != NoteStatus_NEEDS_MORE_RATINGS {
			t.Fatalf("status=%v, want NEEDS_MORE_RATINGS (%+v)", s.Status, s)
		}
	})

	t.Run("one cohort helpful, other somewhat -> mid bridge stays NEEDS_MORE", func(t *testing.T) {
		// meanA=1000, meanB=500 -> bridge=500, which is between thresholds (250,600) -> NEEDS_MORE.
		all := append(seedPolarity(),
			rate(nT, a1, RatingValue_RATING_HELPFUL),
			rate(nT, a2, RatingValue_RATING_HELPFUL),
			rate(nT, b1, RatingValue_RATING_SOMEWHAT),
			rate(nT, b2, RatingValue_RATING_SOMEWHAT),
		)
		s := scoreOf(t, all, nT)
		if s.Status != NoteStatus_NEEDS_MORE_RATINGS || s.Bridge != 500 {
			t.Fatalf("expected NEEDS_MORE with bridge 500, got %+v", s)
		}
	})
}

func TestScoreNotes_DeterministicRepeated(t *testing.T) {
	all := append(seedPolarity(),
		rate(nT, a1, RatingValue_RATING_HELPFUL),
		rate(nT, a2, RatingValue_RATING_HELPFUL),
		rate(nT, b1, RatingValue_RATING_HELPFUL),
		rate(nT, b2, RatingValue_RATING_HELPFUL),
	)
	targets := [][]byte{nT, nS}
	baseline := scoresKey(ScoreNotes(all, targets))
	for i := 0; i < 1000; i++ {
		if got := scoresKey(ScoreNotes(all, targets)); got != baseline {
			t.Fatalf("iteration %d differs:\n got=%s\nwant=%s", i, got, baseline)
		}
	}
}

func TestScoreNotes_ShuffledInputOrderIdentical(t *testing.T) {
	all := append(seedPolarity(),
		rate(nT, a1, RatingValue_RATING_HELPFUL),
		rate(nT, a2, RatingValue_RATING_HELPFUL),
		rate(nT, b1, RatingValue_RATING_HELPFUL),
		rate(nT, b2, RatingValue_RATING_HELPFUL),
		rate(nT, a1, RatingValue_RATING_HELPFUL), // duplicate observation: must not change result
	)
	targets := [][]byte{nT, nS}
	baseline := scoresKey(ScoreNotes(all, targets))
	for i := 0; i < 1000; i++ {
		shuffled := append([]RatingInput(nil), all...)
		r := rand.New(rand.NewSource(int64(i))) // fixed seed per iter -> reproducible test, many orders
		r.Shuffle(len(shuffled), func(x, y int) { shuffled[x], shuffled[y] = shuffled[y], shuffled[x] })
		// also shuffle target order; ScoreNotes must sort internally and return a stable order
		shTargets := [][]byte{nS, nT}
		if i%2 == 0 {
			shTargets = [][]byte{nT, nS}
		}
		if got := scoresKey(ScoreNotes(shuffled, shTargets)); got != baseline {
			t.Fatalf("shuffle seed %d differs:\n got=%s\nwant=%s", i, got, baseline)
		}
	}
}
