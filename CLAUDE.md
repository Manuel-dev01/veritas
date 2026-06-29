# CLAUDE.md — Veritas (Canopy Vibe Code Contest #2)

This is the operating manual for building **Veritas**, an on-chain Community Notes app-chain on Canopy Network. Read this fully before acting. Obey it over your own assumptions. When this file conflicts with the repo's `AGENTS.md` or the actual template code, **the repo wins** — surface the conflict to me, do not silently reconcile.

---

## 0. What we are building (one paragraph)

Veritas is a sovereign Canopy Nested Chain that runs the Community Notes "bridging" algorithm on-chain. Users submit claims, attach notes to claims, and rate notes. Every block, the plugin's `EndBlock` recomputes each note's status using a **bridging score** — a note becomes `HELPFUL` only when it earns agreement from raters who normally disagree with each other, not merely from a majority. Honest participation builds non-transferable reputation; bad-faith participation decays it. The wow: the scoring algorithm that X and Meta run on private servers runs here transparently, on-chain, verifiable by anyone.

**One-line pitch:** "Community Notes, but nobody can secretly rig it — the fact-checking referee runs on-chain where anyone can verify its work."

---

## 1. Hard constraints (violating any of these fails the contest or breaks consensus)

1. **Custom transaction types are mandatory.** The plugin MUST define its own transaction message types in `.proto`. We define at least three: `SubmitClaim`, `SubmitNote`, `RateNote`.
2. **It must touch the chain over RPC (ports 50002 / 50003).** Mocked-data frontends and standalone scripts that never hit the chain are explicitly disqualified. Every user action in the demo must be a real signed tx submitted via RPC, landing in a block, mutating state.
3. **Determinism is non-negotiable.** The plugin runs independently on every validator and must produce byte-identical state from identical input. This means in ALL plugin logic, especially `EndBlock`:
   - **No floating-point math.** Use integer / fixed-point arithmetic only. (See §5.)
   - **No map iteration without sorting.** Go map iteration order is randomized. Always collect keys, sort them, then iterate.
   - **No wall-clock time, no `rand`, no goroutine-timing-dependent logic, no network/disk reads** inside state transitions.
   - Any non-determinism here will fork the chain. Treat this as the #1 correctness rule.
4. **Signing is BLS12-381** over the protobuf-encoded transaction bytes. NOT Ed25519, NOT secp256k1. The tx-submission tool and frontend must sign with BLS12-381 or transactions will be rejected.
5. **The plugin is a separate process**, not a fork of consensus code. It implements the lifecycle interface and talks to the Canopy node over a Unix socket via protobuf. We do not touch consensus, P2P, or storage internals.

---

## 2. Build order (do NOT skip ahead)

Work in these phases. Do not start a phase until the previous one's "Done when" is met. After each phase, stop and report status to me before proceeding.

**Phase 0 — Toolchain proof (guestbook first).**
Before writing any Veritas logic, get a local single-node Canopy chain running and complete the official guestbook walkthrough (ezeike.github.io/canopy-app-guide/walkthrough.html) verbatim. This proves: proto generation works, the plugin process starts and connects over the Unix socket, you can submit a tx via RPC, and you can query state.
- Done when: a guestbook entry submitted via RPC appears in chain state queried via RPC, on a local node.

**Phase 1 — Veritas proto + skeleton plugin.**
Define the three transaction message types in `.proto`, generate Go code, and stub the lifecycle functions (`Genesis`, `BeginBlock`, `CheckTx`, `DeliverTx`, `EndBlock`) with the message routing in place but trivial bodies.
- Done when: a `SubmitClaim` tx is accepted by `CheckTx`, applied by `DeliverTx`, and the claim is queryable via RPC.

**Phase 2 — State model + DeliverTx for all three types.**
Implement the full state model (§4) and the `DeliverTx` handlers for `SubmitClaim`, `SubmitNote`, `RateNote`. No scoring yet — just correct, validated, persisted state.
- Done when: claim → note → multiple ratings all persist correctly and are queryable; `CheckTx` rejects malformed/unauthorized txs.

**Phase 3 — The bridging scorer in `EndBlock`.**
Implement the deterministic fixed-point bridging algorithm (§5). This is the heart of the project.
- Done when: feeding ratings from opposing-pattern raters flips a note to `HELPFUL`, while a note rated helpful by only one cohort stays `NEEDS_MORE_RATINGS`; status changes happen in `EndBlock` and are reproducible across two nodes.

**Phase 4 — Reputation + decay.**
Add non-transferable rater/author reputation updated in `EndBlock`, with slow decay for inactivity.
- Done when: reputation moves correctly and decays deterministically; no float, no map-order bugs.

**Phase 5 — Tx-submission tooling (BLS12-381).**
A Go CLI that builds, BLS-signs, and submits each tx type to RPC. This is also what the frontend's signing will mirror.
- Done when: all three tx types can be submitted end-to-end from the CLI against a local node.

**Phase 6 — Frontend (Next.js + TypeScript).**
A clean UI: list claims, view notes under a claim with live status, submit notes/ratings (signed BLS12-381), and a panel that visualizes the bridging recompute. Reads state over RPC 50002.
- Done when: full loop works in the browser against the local chain.

**Phase 7 — Demo + open-source polish.**
README with the one-line pitch, architecture, "why an app-chain" justification, and run instructions. Record the demo (§7).

---

## 3. Repo layout (target)

```
veritas/
├── CLAUDE.md                 # this file
├── README.md                 # pitch, architecture, run instructions
├── docs/
│   └── VERITAS_SPEC.md       # design of record — read before coding
├── proto/
│   └── veritas.proto         # SubmitClaim, SubmitNote, RateNote messages
├── plugin/                   # the Go plugin (separate process)
│   ├── main.go               # contract.StartPlugin(), lifecycle wiring
│   ├── handlers.go           # CheckTx / DeliverTx per message type
│   ├── endblock.go           # bridging scorer + reputation (DETERMINISTIC)
│   ├── state.go              # state keys, read/write helpers
│   ├── scoring.go            # pure fixed-point bridging math (unit-tested)
│   └── scoring_test.go       # determinism + correctness tests
├── cli/                      # BLS12-381 tx submission tool
│   └── main.go
├── frontend/                 # Next.js + TypeScript
└── genesis.json              # initial state
```

Keep the bridging math in `scoring.go` as **pure functions** (input structs → output structs, no I/O, no state handles). This makes it unit-testable for determinism and keeps the consensus-critical logic isolated.

---

## 4. State model

All state is a key-value store reached through the plugin socket interface (never a direct DB). Use clear, prefixed, deterministically-ordered keys.

- `claim/{claimId}` → Claim{ id, contentHash, url, submitter, createdHeight }
- `note/{noteId}` → Note{ id, claimId, author, body, contentHash, createdHeight, status }
  - status ∈ { `NEEDS_MORE_RATINGS`, `HELPFUL`, `NOT_HELPFUL` }
- `rating/{noteId}/{rater}` → Rating{ noteId, rater, value, createdHeight }
  - value ∈ { `HELPFUL`, `SOMEWHAT`, `NOT_HELPFUL` } → map to fixed-point {1000, 500, 0}
- `rep/{account}` → Reputation{ account, scoreFP, lastActiveHeight }  // scoreFP is fixed-point integer
- `noteIndexByClaim/{claimId}/{noteId}` → 1   // for enumeration without scanning
- `notesNeedingScore/{noteId}` → 1   // dirty-set so EndBlock only rescans changed notes

IDs: derive deterministically (e.g. hash of submitter + nonce + height), never from time or randomness.

**EndBlock efficiency:** do not rescan all notes every block. Maintain a dirty-set (`notesNeedingScore/...`) written by `DeliverTx` when a rating lands, and only rescore those notes in `EndBlock`. This keeps per-block work bounded and the demo snappy.

---

## 5. The bridging algorithm (deterministic, fixed-point)

Goal: a note is `HELPFUL` only when raters who *normally disagree* both rate it helpful. v1 is a simplified, fully deterministic approximation of Community Notes' bridging idea — be explicit in the README that it is simplified, not the production matrix-factorization model.

**Fixed-point convention:** represent all fractional values as integers scaled by 1000 (so 0.5 → 500). Do every multiply/divide with integer math and document rounding (round-half-down, consistently). Never use `float64`.

**v1 algorithm (simplified bridging):**
1. Assign each rater a **polarity** from their rating history — the sign of their average deviation from consensus across all notes they've rated. Bucket into two cohorts, A and B, deterministically (tie → cohort A). This is the "people who normally disagree" proxy. Compute polarity from persisted history, sorted by account id.
2. For the note being scored, compute the fixed-point mean helpfulness within cohort A (`meanA`) and within cohort B (`meanB`), each over sorted rater ids.
3. **Bridging score = min(meanA, meanB)** (a note scores high only if BOTH cohorts find it helpful — the cross-divide agreement that defines bridging). Require a minimum rating count per cohort (e.g. ≥2 each) before scoring; otherwise `NEEDS_MORE_RATINGS`.
4. Thresholds: bridging score ≥ 600 → `HELPFUL`; ≤ 250 → `NOT_HELPFUL`; else `NEEDS_MORE_RATINGS`.
5. Make thresholds named constants so they're easy to tune and easy to explain in the demo.

**Reputation (Phase 4):** when a note resolves `HELPFUL`, raters who rated it helpful and the note's author gain reputation; raters on the wrong side lose a little. Decay: each `EndBlock`, accounts inactive for > N blocks lose a small fixed amount, floored at zero. All integer math, all over sorted keys.

**Determinism checklist for this file — verify every time you touch it:**
- [ ] No `float`/`float64` anywhere.
- [ ] Every map turned into a sorted slice before iteration.
- [ ] No `time.Now()`, `rand`, or external reads.
- [ ] Rounding rule applied identically in all divisions.
- [ ] `scoring_test.go` runs the same inputs 1000× and asserts identical output, and runs a shuffled-input-order test asserting identical output.

---

## 6. Coding standards & guardrails

- **Stop-and-ask triggers.** Before doing any of these, stop and confirm with me: changing the transaction message schema after Phase 2; introducing any dependency that does floating-point or nondeterministic work in the plugin path; deviating from the build order; anything that would require editing Canopy core (we should never need to).
- **Determinism review.** Any change to `plugin/scoring.go`, `endblock.go`, or `handlers.go` must end with the §5 determinism checklist explicitly run and reported.
- **Test as you go.** Every `DeliverTx` handler gets a table test. The scorer gets determinism + correctness tests. Do not advance phases on untested consensus code.
- **Small commits, descriptive messages.** One logical change per commit. We are build-in-public; the git history is part of the story.
- **No secrets in the repo.** Keystore files, private keys, `keyfile.json` → `.gitignore`.
- **Match the official template's conventions.** Read `AGENTS.md` and the Go template before imposing structure; mirror its package layout and naming.
- **When unsure about a Canopy interface detail, read the template code, don't guess.** If still unclear, ask me — do not invent an API.

---

## 7. The demo (this is graded — design backward from it)

The video must, on a local chain, unambiguously show real custom transactions hitting RPC and the on-chain recompute working. Target ~2 minutes:
1. Show the local Canopy node + Veritas plugin running.
2. Submit a `SubmitClaim` and a `SubmitNote` via the UI (real BLS-signed tx → RPC). Note shows `NEEDS_MORE_RATINGS`.
3. Submit `RateNote` ratings from cohort-A accounts only → note stays `NEEDS_MORE_RATINGS` (proves majority-of-one-side isn't enough).
4. Submit `RateNote` ratings from cohort-B accounts → at the next block, `EndBlock` recompute flips it to `HELPFUL` on-chain.
5. Briefly show the tx in a block + the state change via RPC query, proving it's real chain state, not UI mock.

The "one cohort isn't enough, but cross-cohort agreement promotes it" beat is the wow. Make the UI visibly show the two cohorts and the bridging score.

---

## 8. Definition of done (contest checklist)

- [ ] Original, Social-Fi-themed, functional. ✓ (on-chain Community Notes)
- [ ] Built on a Canopy Template (Go plugin + TS frontend).
- [ ] Plugin defines custom transaction types (`SubmitClaim`, `SubmitNote`, `RateNote`).
- [ ] App interacts with local Canopy chain via RPC (50002/50003) — no mocks.
- [ ] Local demo video showing the function clearly.
- [ ] Open-source on GitHub with README + one-line pitch.
- [ ] Determinism verified (two-node identical state).
- [ ] "Why an app-chain, not a contract" justification written in README (per-block global recompute is free in EndBlock, gas-prohibitive on EVM; reputation is native non-transferable state; credible neutrality needs sovereignty).