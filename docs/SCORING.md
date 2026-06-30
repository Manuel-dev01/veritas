# Veritas scoring — on-chain Community Notes matrix factorization (deterministic, fixed-point)

Veritas runs the **real Community Notes scoring model on-chain**, every block, deterministically.
This is the wow: the matrix-factorization model that X and Meta run on private servers runs here in
the open, byte-reproducibly verifiable by anyone.

## The model

Each rating is modeled as

```
r_un  ≈  μ + b_u + b_n + f_u · f_n
```

- `μ` — global intercept (the average rating level)
- `b_u` — rater intercept (how generous/harsh this rater is overall)
- `b_n` — **note intercept** — the bridging signal
- `f_u`, `f_n` — 1-D latent factors (rater and note positions on a learned **polarity axis**) — this is
  exactly production Community Notes' `d = 1`

A note is **HELPFUL when `b_n ≥ 0.40`** (the same 0.40 note-intercept threshold Community Notes uses),
`NOT_HELPFUL` when `b_n ≤ -0.20`, else `NEEDS_MORE_RATINGS` (after a minimum rating count).

**Why the note intercept is the bridge.** The factor term `f_u · f_n` explains agreement *within* a
faction (raters on the same side of the polarity axis). Helpfulness that only one faction sees is
absorbed into that term and leaves `b_n` low. Helpfulness that survives *across* the axis — both sides
rate it helpful — cannot be explained by `f_u · f_n` (one note factor can't be high for raters on both
signs at once), so it is forced into `b_n`. A high note intercept ⇒ cross-divide agreement ⇒ HELPFUL.

The model is trained by **full-batch gradient descent** on the regularized squared error
`Σ (r − r̂)² + λ(‖b‖² + ‖f‖²)`, implemented in [`scoring.go`](../plugin/go/contract/scoring.go)
(`TrainMF` / `ScoreNotesMF`).

## Making it deterministic (the #1 constraint)

The plugin runs independently on every validator and must produce byte-identical state. Gradient
descent + floating point is the natural enemy of that, so:

- **Integer-only fixed point.** All parameters are `int64` scaled by `mfScale = 1_000_000`; ratings map
  to `{0, 500000, 1000000}`. One rounding rule everywhere: multiply-then-divide truncating toward zero
  (`mulFP`). No `float64` exists in the scoring path.
- **Full-batch gradient descent, not SGD.** Gradients are summed over ratings in **sorted
  `(noteId, rater)` order**; parameters are updated over **sorted ids**. The result depends only on the
  set of ratings, never their arrival order — proven by a shuffled-input test.
- **Deterministic init replaces random init.** Real MF seeds factors randomly to break symmetry;
  randomness would fork the chain. Instead each factor is seeded from `sha256(id)` (`initFromID`) — a
  fixed, id-dependent, symmetry-breaking value with no RNG.
- **Fixed hyper-parameters.** Epoch count, learning rate, and L2 are compile-time constants (rationals,
  so the math stays integer). Parameters are clamped to bound overflow.
- No `time`, no `rand` (the `rand.Uint64()` in EndBlock is only a socket request id, never state).

### Proof
- [`scoring_mf_test.go`](../plugin/go/contract/scoring_mf_test.go): the scorer is byte-identical across
  1000 repeats and across 50 shuffled input orderings; and the calibration scenario classifies a
  cross-faction note HELPFUL, a single-faction note not-helpful, and a panned note NOT_HELPFUL.
- [`replay_test.go`](../plugin/go/contract/replay_test.go): an identical transaction history replayed
  through the real handlers + MF EndBlock on **two independent backends** yields an identical per-block
  state digest ("app hash") — the two-node identical-state guarantee, in-process.

## Cost / scope

v1 of the on-chain trainer retrains the model from a **global** scan of all ratings on each block a
note is dirty — bounded by total ratings, which is fine at app/demo scale. A production deployment
would train against a snapshot and/or maintain incremental aggregates; the dirty-set already bounds
which notes are re-written each block.

## Fallback

The earlier simplified bridging scorer (`ScoreNotes` in `scoring.go` — cohort split by deviation sign,
score = `min(meanA, meanB)`) is **retained and still tested**. EndBlock selects the scorer in one line
([`endblock.go`](../plugin/go/contract/endblock.go), `ScoreNotesMF` → swap to `ScoreNotes`).

> Note: this supersedes CLAUDE.md §5, which scoped v1 as "a simplified approximation, NOT the
> production matrix-factorization model." That scoping was intentionally lifted to ship the real model;
> v1 remains as the documented fallback.
