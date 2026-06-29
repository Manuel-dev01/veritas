# VERITAS_SPEC.md — Design of Record

This is the authoritative spec for what Veritas does and how its pieces fit. `CLAUDE.md` governs *how we build*; this governs *what we build*. Read both before writing code. Where this conflicts with the actual Canopy template/`AGENTS.md`, the template wins — flag the conflict.

---

## 1. Problem & thesis

Community Notes (X) and its Meta clone are the dominant 2025–26 mechanism for adding crowd-sourced context to misleading posts. Their core innovation is a **bridging algorithm**: a note is shown only when it earns approval from raters who *historically disagree*, so surfaced notes are ones both "sides" find fair. The weakness: the algorithm runs on a private corporate server. The public must trust that the neutral arbiter is actually neutral and unmodified.

**Veritas thesis:** put the bridging algorithm on-chain. Make the referee verifiable. A sovereign Canopy chain can run the per-block recompute natively and cheaply, store reputation as non-transferable native state, and remain credibly neutral because no single party controls its block production or can pause it.

## 2. Why this *needs* an app-chain (the architectural justification)

- **Per-block global recompute is free in `EndBlock`, gas-prohibitive on EVM.** Recomputing bridging scores and decaying reputation every block over global state is exactly the workload validators run for free in a Canopy plugin, and exactly the workload that forces every existing Community Notes system off-chain onto a trusted server. This is the crux.
- **Reputation must be native, non-transferable state — not a token.** Making it an ERC-20 invites the friend.tech speculative death spiral. A sovereign chain enforces non-transferability at the protocol level.
- **Credible neutrality requires sovereignty.** A truth/context layer can't share blockspace with, or be censorable by, a memecoin next door.
- **(Optional, later) consensus-as-oracle** for attesting external URL content hashes via committee quorum — a native Canopy primitive, not something a contract can do.

## 3. Actors

- **Submitter** — registers a claim (a post/URL/statement that may need context).
- **Author** — writes a note attached to a claim.
- **Rater** — rates notes helpful / somewhat / not helpful. Raters' histories define their cohort.
- **Reader** — consumes notes and their on-chain status (no tx needed; read-only via RPC).

## 4. Transaction types (custom — the contest requirement)

All three are defined in `proto/veritas.proto`, BLS12-381-signed, submitted via RPC.

### 4.1 `SubmitClaim`
Registers something that may need context.
- Fields: `submitter` (address), `url` (string, optional), `content_hash` (bytes — hash of the claim text/target), `text` (string, short), `nonce` (uint64).
- `CheckTx`: non-empty content_hash or url; text length bounded; valid signature; nonce check.
- `DeliverTx`: write `claim/{claimId}`; claimId = hash(submitter‖nonce‖height). Idempotent on replay.

### 4.2 `SubmitNote`
Attaches a note to an existing claim.
- Fields: `author` (address), `claim_id` (bytes), `body` (string), `content_hash` (bytes), `nonce` (uint64).
- `CheckTx`: body length bounded (e.g. ≤ 280–560 chars); valid signature; nonce.
- `DeliverTx`: require `claim/{claim_id}` exists; write `note/{noteId}` with status `NEEDS_MORE_RATINGS`; write `noteIndexByClaim/{claim_id}/{noteId}`. noteId = hash(author‖claim_id‖nonce‖height).

### 4.3 `RateNote`
Rates a note. One rating per (note, rater); later ratings overwrite.
- Fields: `rater` (address), `note_id` (bytes), `value` (enum: HELPFUL | SOMEWHAT | NOT_HELPFUL), `nonce` (uint64).
- `CheckTx`: value is a valid enum; valid signature; nonce. Stateless only.
- `DeliverTx`: require `note/{note_id}` exists; rater ≠ note.author (can't rate own note); write `rating/{note_id}/{rater}`; add `note_id` to the `notesNeedingScore` dirty-set; touch `rep/{rater}.lastActiveHeight`.

> Add a 4th type later only if time allows: `AttestSource` (committee/oracle content-hash attestation). Out of scope for v1; do not schema-churn to add it once Phase 2 is done (see CLAUDE.md stop-and-ask).

## 5. Lifecycle wiring

- `Genesis()` — load any seed accounts/claims from `genesis.json` (can be empty for v1; optionally seed a few demo claims).
- `BeginBlock()` — no-op v1.
- `CheckTx()` — stateless validation per type (signature, field bounds, enum validity, nonce). No state reads.
- `DeliverTx()` — route by message type to the handlers in §4; persist state; maintain indexes and dirty-set.
- `EndBlock()` — (a) rescore every note in `notesNeedingScore`, clear the set; (b) apply reputation updates for notes that changed status; (c) apply inactivity decay. All deterministic, fixed-point, sorted-key iteration. (See CLAUDE.md §5.)

## 6. Scoring summary (full detail in CLAUDE.md §5)

Cohort assignment from rating history → per-cohort fixed-point mean helpfulness → bridging score = min(meanA, meanB) with per-cohort minimum counts → thresholds map to status. Fixed-point ×1000, integer math only, sorted iteration only, no clocks/rand. Thresholds are named constants.

## 7. Frontend (Next.js + TS)

- **Claims list** — all claims, newest first (read via RPC).
- **Claim detail** — notes under the claim, each showing live status badge and current bridging score, plus the per-cohort means (the "wow" visualization).
- **Submit note / rate note** — forms that build, BLS12-381-sign, and submit txs via RPC; show the tx landing in a block.
- **Cohort/score panel** — visualizes cohort A vs B and how min(meanA, meanB) drives status. This is what sells the bridging concept on camera.
- Reads only via RPC 50002; never mocks state.

## 8. Out of scope for v1 (say so in README)

- Production-faithful matrix-factorization bridging (we ship a deterministic simplification).
- ZK / privacy features.
- Cross-chain attestation / oracle (`AttestSource`) — noted as roadmap.
- Token economics / any transferable token tied to reputation (deliberately avoided).

## 9. Success criteria

A reviewer can clone the repo, run a local node + plugin, open the frontend, submit a claim+note, rate it from two opposing cohorts, and watch the on-chain `EndBlock` recompute flip the note to HELPFUL — with every step a real RPC transaction and verifiable chain state. That single loop satisfies originality, the custom-tx requirement, the RPC requirement, and the wow factor at once.