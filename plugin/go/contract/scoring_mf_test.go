package contract

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

/*
scoring_mf_test.go — correctness/calibration + determinism for the fixed-point matrix-factorization
scorer (ScoreNotesMF / TrainMF in scoring.go).

The scenario builds two opposing factions (A vs B) via polarizing seed notes, then checks the
bridging behaviour: a note rated helpful by BOTH factions earns a high note-intercept (HELPFUL),
while a note rated helpful by only ONE faction has its helpfulness absorbed by the latent factor
(low intercept -> not HELPFUL). Determinism tests assert byte-identical output across repeats and
across shuffled input order.
*/

func mfRating(note, rater string, v RatingValue) RatingInput {
	return RatingInput{NoteID: []byte(note), Rater: []byte(rater), Value: v}
}

// mfScenario: factions A={A1,A2,A3}, B={B1,B2,B3}. Seed notes polarize them; then four test notes.
func mfScenario() (ratings []RatingInput, targets [][]byte) {
	H, X := RatingValue_RATING_HELPFUL, RatingValue_RATING_NOT_HELPFUL
	A := []string{"A1", "A2", "A3"}
	B := []string{"B1", "B2", "B3"}
	add := func(note string, raters []string, v RatingValue) {
		for _, r := range raters {
			ratings = append(ratings, mfRating(note, r, v))
		}
	}
	// polarizing seeds: A and B disagree, in both directions, to fix the latent axis
	add("S1", A, H)
	add("S1", B, X)
	add("S2", A, X)
	add("S2", B, H)
	add("S3", A, H)
	add("S3", B, X)
	// T_bridge: BOTH factions rate helpful -> should be HELPFUL (cross-divide agreement)
	add("T_bridge", A, H)
	add("T_bridge", B, H)
	// T_oneside: only faction A rates helpful -> should NOT be HELPFUL (factor explains it)
	add("T_oneside", A, H)
	// T_bad: everyone rates not-helpful -> NOT_HELPFUL
	add("T_bad", A, X)
	add("T_bad", B, X)
	targets = [][]byte{[]byte("T_bridge"), []byte("T_oneside"), []byte("T_bad")}
	return ratings, targets
}

func scoreByNote(scores []NoteScore) map[string]NoteScore {
	m := map[string]NoteScore{}
	for _, s := range scores {
		m[string(s.NoteID)] = s
	}
	return m
}

func TestMFCalibration(t *testing.T) {
	ratings, targets := mfScenario()
	scores := ScoreNotesMF(ratings, targets)
	by := scoreByNote(scores)

	for _, s := range scores {
		t.Logf("%-10s status=%-20s intercept=%-9d factor=%-9d mu=%d raters=%d (cohorts A=%d B=%d meanA=%d meanB=%d)",
			string(s.NoteID), s.Status, s.NoteIntercept, s.NoteFactor, s.GlobalMu, s.NumRaters, s.CountA, s.CountB, s.MeanA, s.MeanB)
	}

	if got := by["T_bridge"].Status; got != NoteStatus_HELPFUL {
		t.Errorf("T_bridge: cross-faction agreement must be HELPFUL, got %s (intercept=%d)", got, by["T_bridge"].NoteIntercept)
	}
	if got := by["T_oneside"].Status; got == NoteStatus_HELPFUL {
		t.Errorf("T_oneside: single-faction helpful must NOT be HELPFUL, got %s (intercept=%d)", got, by["T_oneside"].NoteIntercept)
	}
	if got := by["T_bad"].Status; got != NoteStatus_NOT_HELPFUL {
		t.Errorf("T_bad: universally unhelpful must be NOT_HELPFUL, got %s (intercept=%d)", got, by["T_bad"].NoteIntercept)
	}
	// the bridge note's intercept must exceed the one-sided note's (the discriminating signal)
	if by["T_bridge"].NoteIntercept <= by["T_oneside"].NoteIntercept {
		t.Errorf("bridge intercept (%d) should exceed one-sided intercept (%d)", by["T_bridge"].NoteIntercept, by["T_oneside"].NoteIntercept)
	}
}

func TestMFDeterministicRepeat(t *testing.T) {
	ratings, targets := mfScenario()
	want := ScoreNotesMF(ratings, targets)
	for i := 0; i < 1000; i++ {
		if got := ScoreNotesMF(ratings, targets); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d differs from run 0 — MF scorer is not deterministic", i)
		}
	}
}

func TestMFShuffleInvariant(t *testing.T) {
	ratings, targets := mfScenario()
	want := ScoreNotesMF(ratings, targets)
	rng := rand.New(rand.NewSource(12345)) // test-only shuffling; the scorer must be order-invariant
	for i := 0; i < 50; i++ {
		shuffled := append([]RatingInput(nil), ratings...)
		rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		if got := ScoreNotesMF(shuffled, targets); !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffle %d changed output — MF scorer depends on input order", i)
		}
	}
}

// (compile-time) ensure the helper is referenced even if assertions above change.
var _ = fmt.Sprintf
