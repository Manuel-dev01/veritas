<div align="center">

# Veritas

### Community Notes, but nobody can secretly rig it — the fact-checking referee runs on-chain where anyone can verify its work.

A sovereign **Canopy Nested Chain** that runs the Community Notes **bridging algorithm** on-chain. Every
block, the plugin re-scores every note with deterministic, fixed-point **matrix factorization** — the
same model X and Meta run on private servers, here in the open and byte-reproducible by anyone.

**Live:** [landing](https://frontend-kappa-flax-87.vercel.app) · [the app](https://frontend-kappa-flax-87.vercel.app/app)
&nbsp;|&nbsp; built for **Canopy Vibe Code Contest #2** (Social-Fi)

</div>

> [!NOTE]
> The public `/app` reads live data from a locally-running chain + gateway exposed over a tunnel, so it
> depends on the demo host being online; the landing page is fully client-side and always works. To run
> the whole stack yourself, see [Quickstart](#quickstart).

---

## The idea

Community Notes' core innovation is **bridging**: a note is shown only when it earns agreement from
raters who *normally disagree with each other* — not from a simple majority. So a surfaced note is one
both "sides" find fair, and a brigade of like-minded accounts can't force one through.

The catch in the real world: **that algorithm runs on a private corporate server.** You have to *trust*
that the neutral referee is actually neutral and unmodified.

**Veritas puts the referee on-chain.** The bridging recompute is a public state transition executed by
every validator each block. Nobody can quietly retune the thresholds, suppress a note, or weight a
cohort — because everyone holds the whistle.

> A note goes **HELPFUL** only when people who normally **disagree** with each other **agree** it's fair.

---

## Why an app-chain, not a smart contract

This is the architectural crux — Veritas *needs* sovereignty, it isn't a contract that happens to be
on a chain:

- **Per-block global recompute is free in `EndBlock`, gas-prohibitive on EVM.** Re-scoring every note
  and decaying reputation over global state every block is exactly the work validators already do for
  free inside a Canopy plugin — and exactly the work that forces every existing Community Notes system
  off-chain onto a trusted server.
- **Reputation must be native, non-transferable state — not a token.** Minting it as an ERC-20 invites a
  speculative death spiral. A sovereign chain enforces non-transferability at the protocol level:
  reputation can only be minted/burned by the protocol in `EndBlock`; no transaction can move it.
- **Credible neutrality requires sovereignty.** A truth/context layer can't share blockspace with, or be
  censorable by, a memecoin next door.

---

## Architecture

Three tiers, with a thin Go gateway bridging the browser to the chain:

```
  Browser ──HTTPS──▶  Next.js (frontend, :3217)
                         │  fetch /api/*
                         ▼
                      Go gateway  "veritas serve" (:8080)
                      • signs txs (BLS12-381)         • CORS
                      • in-memory event indexer        • seeds demo identities
                         │  POST /v1/tx · GET /v1/query/events-by-*
                         ▼
                      Canopy node  (RPC :50002 query · :50003 admin)
                         │  Unix socket (length-prefixed protobuf)
                         ▼
                      Veritas Go plugin  (separate process)
                      Genesis · CheckTx · DeliverTx · EndBlock(bridging + reputation)
```

**Why the gateway exists (and the browser doesn't sign).** The chain signs with **BDN-on-G2 via
`kyber-bls12381`**, whose hash-to-curve no JavaScript BLS library reproduces — so a browser can't
produce a valid signature. And this Canopy build's RPC has **no CORS** and **no plugin-state read
endpoint** (decoded **events** are the only read path). The gateway reuses the CLI's proven
`Client.BuildAndSubmit` (signing) + `EventsByHeight` / event decoding (reading) to solve both, running
in WSL beside the node. (Browser-side signing via a WASM build of the Go signer is on the roadmap.)

---

## Custom transaction types

The contest requires custom transaction message types; the plugin defines three (in
[`plugin/go/proto/tx.proto`](plugin/go/proto/tx.proto)), all **BLS12-381-signed** and submitted via RPC:

| Message | Fields | Effect (`DeliverTx`) |
|---|---|---|
| `MessageSubmitClaim` | `submitter, content_hash, url, text, nonce` | writes `claim/{id}`; `id = sha256(submitter‖nonce‖height)` |
| `MessageSubmitNote` | `author, claim_id, body, content_hash, url, nonce` | requires claim; writes `note/{id}` (status `NEEDS_MORE_RATINGS`) + index |
| `MessageRateNote` | `rater, note_id, value, nonce` | requires note; one rating per `(note,rater)` (overwrites); marks the note dirty |

Enums: `NoteStatus { NEEDS_MORE_RATINGS=1, HELPFUL=2, NOT_HELPFUL=3 }`,
`RatingValue { RATING_HELPFUL=1, RATING_SOMEWHAT=2, RATING_NOT_HELPFUL=3 }`.

State is a prefixed key-value store: `claim`=24, `note`=25, `rating`=26, `rep`=27, `noteIndexByClaim`=28,
`notesNeedingScore`=29 (the dirty-set, so `EndBlock` only re-scores notes that changed).

---

## The on-chain scorer

Veritas runs the **real Community Notes matrix-factorization model** ([`plugin/go/contract/scoring.go`](plugin/go/contract/scoring.go),
full write-up in [`docs/SCORING.md`](docs/SCORING.md)). Each rating is modeled as

```
r_un  ≈  μ + b_u + b_n + f_u · f_n
```

- `μ` global intercept · `b_u` rater intercept · **`b_n` note intercept (the bridging signal)** ·
  `f_u, f_n` the 1-D latent factors that place raters and notes on a learned **polarity axis**
  (production Community Notes' `d = 1`).
- **Why the note intercept *is* the bridge:** agreement *within* one faction is absorbed by `f_u·f_n`;
  agreement that survives *across* the axis (both signs rate it helpful) can't be — one note factor
  can't be high for raters on both signs at once — so it's forced into `b_n`. High `b_n` ⇒ cross-divide
  agreement ⇒ **HELPFUL**.
- Thresholds: **`b_n ≥ 0.40` → HELPFUL** (`mfHelpfulIntercept = 400_000` at `mfScale = 1e6`, the same
  0.40 Community Notes uses), `b_n ≤ -0.20` → NOT_HELPFUL, else NEEDS_MORE_RATINGS (min 3 ratings).
- Trained by **full-batch gradient descent** (`mfEpochs = 600`) on regularized squared error.

A simplified v1 bridging scorer (`min(meanA, meanB)` over deviation-sign cohorts) is retained and
tested as the documented fallback (`ScoreNotes`; selected in one line in `endblock.go`).

### Determinism — the #1 correctness rule

The plugin runs independently on every validator and must produce **byte-identical** state, so the
scoring path has:

- **Integer-only fixed point** (`int64 × 1e6`), one rounding rule (multiply-then-divide toward zero) —
  no `float64` anywhere.
- **Full-batch GD over sorted `(noteId, rater)` order** — result depends on the *set* of ratings, never
  arrival order.
- **`sha256(id)`-seeded factor init** replaces random init (real MF seeds randomly to break symmetry;
  randomness would fork the chain).
- **No `time`, no `rand`** in state logic (the lone `rand.Uint64()` is a socket request id, never state);
  all map iteration is over sorted keys.

Proven by [`scoring_mf_test.go`](plugin/go/contract/scoring_mf_test.go) (byte-identical across 1000
repeats **and** shuffled input orderings, plus a calibration scenario) and
[`replay_test.go`](plugin/go/contract/replay_test.go) (an identical tx history replayed on **two
independent backends** yields an identical per-block state digest — the two-validator guarantee, proven
in-process).

### Reputation (native, non-transferable)

Updated only in `EndBlock` on a note's terminal transition: author of a HELPFUL note `+1000`, correct
raters `+500`, wrong raters `−250`; NOT_HELPFUL is symmetric (author `−500`). Inactivity decay removes
`100`/block after `10` idle blocks, floored at zero. All integer math, over sorted keys.

---

## Repo layout

```
veritas/
├── proto → plugin/go/proto/tx.proto   custom tx + record + event messages
├── plugin/go/                          the Canopy plugin (separate process)
│   ├── contract/  handlers · endblock · scoring (MF + v1) · reputation · state · *_test.go
│   ├── crypto/    BLS12-381 signing
│   └── main.go    StartPlugin + lifecycle wiring
├── cli/                                BLS CLI + the "veritas serve" gateway
│   ├── client.go query.go keys.go      build/sign/submit + event read/decode
│   └── serve.go                        indexer + signer + CORS gateway
├── frontend/                           Next.js + TypeScript UI (landing + live app)
├── deploy/                             build/run/verify scripts + demo driver
└── docs/  VERITAS_SPEC.md · SCORING.md
```

---

## Quickstart

**Environment model: build on Windows, run in WSL2.** The node + plugin need a Linux runtime (Unix
socket); the plugin is pure Go and cross-compiles to a Linux ELF, so WSL needs no Go toolchain. Full
detail (one-time toolchain, node init, reset) is in [`deploy/README.md`](deploy/README.md).

```bash
# 1. Build the plugin (Windows → linux/amd64) and stage next to pluginctl.sh in the node dir
cd plugin/go && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o go-plugin .

# 2. Build the CLI / gateway (one binary)
cd cli && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o veritas .

# 3. Run the node + plugin in WSL (keeps running; ~20s blocks). Set pluginTimeoutMS=5000 in
#    ~/.canopy/config.json — the MF EndBlock needs more than the 1s default.
cd ~/veritas/node && python3 ~/veritas/pty-run.py 0 ~/veritas/node-run.log

# 4. Run the gateway (signs, indexes, CORS) beside the node
~/veritas/veritas serve -port 8080            # -validator-key defaults to ~/.canopy/validator_key.json

# 5. Run the frontend (Windows)
cd frontend && npm install && npm run dev      # → http://localhost:3217  (NEXT_PUBLIC_GATEWAY=http://localhost:8080)
```

The CLI also drives every tx type directly:
`veritas claim|note|rate|send|query|keys` (global flags `-rpc :50002`, `-admin-rpc :50003`, `-key`,
`-json`). See [`cli/README.md`](cli/README.md).

---

## Demo

The graded beat: rate a note **helpful from one camp only** → it stays `NEEDS_MORE_RATINGS`; add **the
opposing camp** → at the next block `EndBlock` flips it to **HELPFUL** on-chain (note intercept crosses
`0.40`). Every step is a real BLS-signed transaction landing in a block — no mocks. `deploy/demo.sh`
drives this end-to-end via the CLI for a zero-click reproduction.

## Roadmap / out of scope for v1

- `AttestSource` — committee/oracle attestation of external URL content hashes (a native Canopy
  primitive a contract can't replicate).
- **WASM browser-side signing** so the browser signs directly and the gateway becomes optional.
- Snapshot / incremental MF training (v1 retrains from a global rating scan each dirty block — fine at
  demo scale, bounded by the dirty-set).
- Multi-validator production deployment; ZK/privacy; any transferable token tied to reputation
  (deliberately avoided).

---

## License

[MIT](LICENSE) © 2026 Manuel-dev01. The vendored Canopy plugin template under `plugin/go/` retains its
own MIT license (© Canopy Network).
