package contract

import (
	"crypto/sha256"
	"encoding/binary"
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
	Bridge int64 // v1: min(MeanA,MeanB); MF: the note intercept (the bridging signal)
	MeanA  int64 // mean helpfulness within latent cohort A (factor >= 0)
	MeanB  int64 // mean helpfulness within latent cohort B (factor < 0)
	CountA int
	CountB int
	// MF-only breakdown (zero under v1):
	NoteIntercept int64 // b_n — high = helpful across the polarity axis (the bridge)
	NoteFactor    int64 // f_n — the note's position on the latent polarity axis
	GlobalMu      int64 // μ — global intercept (the average rating level)
	NumRaters     int   // ratings contributing to this note
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

// =============================================================================================
// Matrix-factorization scorer (the real Community Notes model, made deterministic).
//
// We model each rating as  r_un ≈ μ + b_u + b_n + f_u·f_n  (1-D latent factor, exactly production's
// d=1) and train it with FULL-BATCH gradient descent on the MSE loss with L2 regularization. A note
// is HELPFUL when its note-intercept b_n clears a threshold: a high b_n means the note is rated
// helpful in a way the latent polarity factor f cannot explain — i.e. agreement that BRIDGES the
// divide. Helpfulness that comes from one faction is absorbed into f_u·f_n and leaves b_n low.
//
// DETERMINISM (the #1 rule — this runs on every validator and must be byte-identical):
//   - Integer-only fixed-point at scale mfScale; one rounding rule (truncate toward zero) everywhere.
//   - Full-batch GD: gradients summed over ratings in SORTED (noteId,rater) order; params updated over
//     SORTED ids. No SGD, no shuffling ⇒ output depends only on the input set, never its order.
//   - Deterministic init from sha256(id) replaces the random init real MF uses — it breaks the
//     symmetry GD needs to discover the polarity axis without ever touching rand/time.
//   - No float, no maps iterated unsorted, no wall-clock.
// =============================================================================================

// fixed-point scale for MF parameters (separate, larger than the rating scale, for gradient precision).
const mfScale int64 = 1_000_000

// MF training hyper-parameters (named so they're tunable + explainable). Learning rate and L2 are
// rationals so all arithmetic stays integer.
const (
	mfEpochs          = 600
	mfLRNum     int64 = 1 // learning rate = mfLRNum/mfLRDen (0.5)
	mfLRDen     int64 = 2
	mfRegNum    int64 = 2 // L2 lambda = mfRegNum/mfRegDen (0.02) applied to every parameter
	mfRegDen    int64 = 100
	mfInitMag   int64 = 50_000      // factor init in ±0.05·mfScale, seeded per id
	mfParamCap  int64 = 5_000_000   // clamp params to ±5·mfScale (overflow / divergence guard)
	mfMinRaters       = 3           // a note needs this many ratings before MF assigns a terminal status
)

// MF status thresholds on the note intercept b_n (calibrated in scoring_mf_test.go). The HELPFUL
// threshold is 0.40·mfScale — the same 0.40 note-intercept cutoff Community Notes uses in production.
const (
	mfHelpfulIntercept    int64 = 400_000  // b_n >= 0.40 -> HELPFUL
	mfNotHelpfulIntercept int64 = -200_000 // b_n <= -0.20 -> NOT_HELPFUL
)

// ratingToFPMF maps a rating to MF scale {0, mfScale/2, mfScale}; ok=false for UNSPECIFIED.
func ratingToFPMF(v RatingValue) (int64, bool) {
	switch v {
	case RatingValue_RATING_HELPFUL:
		return mfScale, true
	case RatingValue_RATING_SOMEWHAT:
		return mfScale / 2, true
	case RatingValue_RATING_NOT_HELPFUL:
		return 0, true
	default:
		return 0, false
	}
}

// mulFP multiplies two mfScale-scaled values, returning an mfScale-scaled result (truncate toward 0).
func mulFP(a, b int64) int64 { return a * b / mfScale }

// clampFP bounds a parameter to ±mfParamCap.
func clampFP(x int64) int64 {
	if x > mfParamCap {
		return mfParamCap
	}
	if x < -mfParamCap {
		return -mfParamCap
	}
	return x
}

// initFromID derives a deterministic signed factor seed in [-mfInitMag, +mfInitMag] from sha256(id).
// This replaces random init (which would be non-deterministic) while still breaking symmetry so the
// latent factors can separate along the polarity axis.
func initFromID(id []byte) int64 {
	h := sha256.Sum256(id)
	u := binary.BigEndian.Uint32(h[:4])
	span := 2*mfInitMag + 1
	return int64(u)%span - mfInitMag
}

// MFModel holds the trained parameters. Maps are keyed by string(id bytes).
type MFModel struct {
	Mu int64
	BU map[string]int64 // rater intercepts
	BN map[string]int64 // note intercepts (the bridging signal)
	FU map[string]int64 // rater latent factors
	FN map[string]int64 // note latent factors
}

// mfObs is one rating observation in MF fixed-point.
type mfObs struct {
	nid, rid string
	r        int64
}

// TrainMF fits μ, b_u, b_n, f_u, f_n by deterministic full-batch gradient descent. Pure.
func TrainMF(all []RatingInput) MFModel {
	m := MFModel{BU: map[string]int64{}, BN: map[string]int64{}, FU: map[string]int64{}, FN: map[string]int64{}}
	// collect valid observations + the user/note id sets
	obs := make([]mfObs, 0, len(all))
	users, notes := map[string]struct{}{}, map[string]struct{}{}
	for _, x := range all {
		fp, ok := ratingToFPMF(x.Value)
		if !ok {
			continue
		}
		nid, rid := string(x.NoteID), string(x.Rater)
		obs = append(obs, mfObs{nid: nid, rid: rid, r: fp})
		users[rid] = struct{}{}
		notes[nid] = struct{}{}
	}
	R := int64(len(obs))
	if R == 0 {
		return m
	}
	// deterministic order for gradient sums (output must not depend on input order)
	sort.Slice(obs, func(i, j int) bool {
		if obs[i].nid != obs[j].nid {
			return obs[i].nid < obs[j].nid
		}
		return obs[i].rid < obs[j].rid
	})
	userIDs, noteIDs := sortedKeys(users), sortedKeys(notes)
	for _, u := range userIDs {
		m.FU[u] = initFromID([]byte(u)) // BU defaults to 0
	}
	for _, n := range noteIDs {
		m.FN[n] = initFromID([]byte(n)) // BN defaults to 0
	}

	// step(g) = g * lr ; reg(theta) = theta * lr * lambda  (both integer; MSE loss => divide data grad by R)
	step := func(g int64) int64 { return g * mfLRNum / mfLRDen }
	reg := func(theta int64) int64 { return theta * mfLRNum * mfRegNum / (mfLRDen * mfRegDen) }

	for epoch := 0; epoch < mfEpochs; epoch++ {
		var gMu int64
		gBU := map[string]int64{}
		gBN := map[string]int64{}
		gFU := map[string]int64{}
		gFN := map[string]int64{}
		for _, o := range obs { // sorted order
			pred := m.Mu + m.BU[o.rid] + m.BN[o.nid] + mulFP(m.FU[o.rid], m.FN[o.nid])
			err := o.r - pred
			gMu += err
			gBU[o.rid] += err
			gBN[o.nid] += err
			gFU[o.rid] += mulFP(err, m.FN[o.nid])
			gFN[o.nid] += mulFP(err, m.FU[o.rid])
		}
		// apply averaged data gradient (÷R) minus L2, over sorted ids
		m.Mu = clampFP(m.Mu + step(gMu/R) - reg(m.Mu))
		for _, u := range userIDs {
			m.BU[u] = clampFP(m.BU[u] + step(gBU[u]/R) - reg(m.BU[u]))
			m.FU[u] = clampFP(m.FU[u] + step(gFU[u]/R) - reg(m.FU[u]))
		}
		for _, n := range noteIDs {
			m.BN[n] = clampFP(m.BN[n] + step(gBN[n]/R) - reg(m.BN[n]))
			m.FN[n] = clampFP(m.FN[n] + step(gFN[n]/R) - reg(m.FN[n]))
		}
	}
	return m
}

// ScoreNotesMF trains the MF model once over all ratings, then classifies each target note from its
// learned intercept b_n. Pure and deterministic; returns one NoteScore per target, ordered by note id.
func ScoreNotesMF(all []RatingInput, targets [][]byte) []NoteScore {
	model := TrainMF(all)

	// gather each note's ratings (for the min-raters gate and the latent-cohort breakdown).
	type rv struct {
		rid string
		val int64
	}
	byNote := map[string][]rv{}
	for _, x := range all {
		fp, ok := ratingToFPMF(x.Value)
		if !ok {
			continue
		}
		nid := string(x.NoteID)
		byNote[nid] = append(byNote[nid], rv{rid: string(x.Rater), val: fp})
	}

	sortedTargets := append([][]byte(nil), targets...)
	sort.Slice(sortedTargets, func(i, j int) bool { return string(sortedTargets[i]) < string(sortedTargets[j]) })

	out := make([]NoteScore, 0, len(sortedTargets))
	for _, target := range sortedTargets {
		nid := string(target)
		rs := append([]rv(nil), byNote[nid]...)
		sort.Slice(rs, func(i, j int) bool { return rs[i].rid < rs[j].rid })

		sc := NoteScore{
			NoteID:        target,
			Status:        NoteStatus_NEEDS_MORE_RATINGS,
			GlobalMu:      model.Mu,
			NoteIntercept: model.BN[nid],
			NoteFactor:    model.FN[nid],
			NumRaters:     len(rs),
		}
		sc.Bridge = sc.NoteIntercept // continuity with the v1 "bridge score" field

		// latent cohorts: split this note's raters by the SIGN of their learned factor f_u.
		var sumA, sumB int64
		for _, e := range rs {
			if model.FU[e.rid] >= 0 {
				sumA += e.val
				sc.CountA++
			} else {
				sumB += e.val
				sc.CountB++
			}
		}
		sc.MeanA = meanFP(sumA, sc.CountA)
		sc.MeanB = meanFP(sumB, sc.CountB)

		if sc.NumRaters >= mfMinRaters {
			switch {
			case sc.NoteIntercept >= mfHelpfulIntercept:
				sc.Status = NoteStatus_HELPFUL
			case sc.NoteIntercept <= mfNotHelpfulIntercept:
				sc.Status = NoteStatus_NOT_HELPFUL
			}
		}
		out = append(out, sc)
	}
	return out
}
