# Veritas — demo guide (~2 minutes)

The one thing to land on camera: **a note rated helpful by one camp alone stays unproven; the moment the
*opposing* camp also rates it helpful, the on-chain `EndBlock` recompute flips it to HELPFUL — and it's
all real chain state, not a UI mock.**

Two ways to drive it: the **UI path** (click-through, best for the video) and the **CLI path**
(`deploy/demo.sh`, zero clicks, reproducible). Both submit real BLS-signed transactions over RPC.

---

## Before recording — bring the stack up

In WSL, with the chain reset to a funded genesis (`deploy/reset-chain.sh`) and a few blocks produced:

```bash
# 1. node + plugin (keeps running; ~20s blocks). Ensure pluginTimeoutMS=5000 in ~/.canopy/config.json.
cd ~/veritas/node && python3 ~/veritas/pty-run.py 0 ~/veritas/node-run.log

# 2. gateway (signs, indexes, CORS, seeds demo identities)
~/veritas/veritas serve -port 8080
```

```bash
# 3. frontend (Windows)
cd frontend && npm run dev      # → http://localhost:3217
```

Sanity: `curl -s localhost:8080/api/state` returns JSON with a rising `height`; `curl -s
localhost:8080/api/identities` lists 7 (`you`, `rater_a1..3`, `rater_b1..3`).

---

## The recording — beat by beat

**(0:00) Prove it's a real chain.** Show the WSL panes: node logs ticking blocks
(`tail -f /tmp/plugin/go-plugin.log` shows begin/end each block), gateway running. Cut to the app at
`localhost:3217/app` — the top bar reads `recompute · h:<height>` and the height is climbing.

**(0:15) Submit a claim + note.** Open the submit overlay → add a claim ("Chart drops the 2019
baseline…") and a note with a source URL. Each returns a **txHash** — point at it. The new note shows
**`NEEDS_MORE_RATINGS`** ("PENDING" on the board). *This is a real `SubmitClaim` / `SubmitNote` tx over
RPC.*

**(0:40) One camp isn't enough.** Switch identity to **camp A** (`rater_a1`), rate the note **Helpful**;
repeat as `rater_a2`, `rater_a3`. Wait one block. Open the note drill-in: the bridge breakdown shows
camp A filled, camp B empty, note intercept **below 0.40** → still **`NEEDS_MORE_RATINGS`**. Say it: *a
majority of one side is not a bridge.*

**(1:10) Cross-camp agreement promotes it.** Switch to **camp B** (`rater_b1/2/3`), rate the same note
**Helpful**. At the next block, the board **re-clatters to `HELPFUL`** (green), the Live panel logs
`note_scored … → HELPFUL`, and the drill-in shows **both camp bars filled, note intercept ≥ 0.40**. This
is the wow.

**(1:40) Prove it's on-chain, not UI.** Drop to a terminal and read the verdict straight from chain
events:

```bash
~/veritas/veritas query -address d46e9a2042b4f2bdb69362aec7398f2ec623faa8 -event score
# → note_scored: status HELPFUL, noteIntercept >= 400000 (0.40), at end_block
```

(Or show the tx in its block via `query -height <H>`.) The UI is just reading what the chain computed.

---

## Shortcut: one-click seed (for a tight take)

The gateway can pre-build the polarized scenario so the camps already disagree and a target note sits at
`NEEDS_MORE_RATINGS` rated by camp A only — then you only record the camp-B flip:

- In the app, click **"⟲ seed demo"** (calls `POST /api/seed`), or `curl -X POST localhost:8080/api/seed`.
- Wait ~1 minute (seed txs land + MF learns the polarity axis).
- Rate the target note **Helpful from camp B** → it flips to **HELPFUL**.

Verified numbers for this scenario: target sits at **intercept ≈ 0.37, NEEDS_MORE_RATINGS** (camp A
only) → after camp B, **intercept ≈ 0.46, HELPFUL**. (`deploy/seed-flip.sh <targetNoteId>` checks this
end-to-end.)

---

## Zero-click: the CLI driver

`deploy/demo.sh` reproduces the whole arc on-chain via the CLI (no browser), printing the status at each
beat — ideal for a deterministic capture or a smoke test before recording:

```bash
bash ~/veritas/demo.sh         # copy deploy/demo.sh into WSL first (strip CRLF)
```

It funds two camps, polarizes them with seed notes, creates one target note, rates it from camp A
(prints `NEEDS_MORE_RATINGS`), then from camp B (prints `HELPFUL`, `noteIntercept ≥ 400000`).

---

## What sells it

- Every action is a **real, BLS-signed transaction** landing in a block — no mocked state.
- The promotion is decided by the **chain's `EndBlock`**, recomputed every block, **byte-reproducible**
  by any validator (see `docs/SCORING.md`).
- The rule is visible: one camp → unproven; **cross-camp agreement → HELPFUL**.
