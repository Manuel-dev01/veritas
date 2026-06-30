# Veritas frontend

Next.js 14 (app-router, TypeScript) UI for Veritas — a landing page and the live product. It talks
**only** to the Go gateway (`veritas serve`), never to the chain directly (the browser can't produce the
chain's BLS signatures, and the RPC has no CORS). See the [root README](../README.md) for the full
architecture.

## Run (local dev)

```bash
cp .env.example .env.local        # NEXT_PUBLIC_GATEWAY=http://localhost:8080
npm install
npm run dev                       # → http://localhost:3217
```

Requires the gateway up at `NEXT_PUBLIC_GATEWAY` (run `veritas serve -port 8080` in WSL beside the
node). The board polls `GET /api/state` every ~2s; writes go through `POST /api/claim|note|rate`.

> Ports `3000` and `3100` are intentionally avoided — Veritas uses **3217** (`package.json` `dev`/`start`).

## Layout

- `app/page.tsx` — landing (ticker, split-flap verdict board on live data, interactive bridging demo,
  the `EndBlock` conveyor canvas, thesis/how-it-works/CTA).
- `app/app/page.tsx` — the live product (board, note drill-in with the bridge breakdown, submit/rate
  overlays, reputation, live `EndBlock` log, identity switcher, "⟲ seed demo" button).
- `components/SplitFlap.tsx` — the airport-board status tiles (clatter → settle; periodic re-spin).
- `lib/` — `api.ts` (typed gateway client), `useChain.ts` (state poller), `identity.tsx` (per-browser
  identity context), `css.ts` (mockup CSS-string → React style parser).

## Deploy

Build is static-friendly (`npm run build`). For a public deploy that shows live data, point
`NEXT_PUBLIC_GATEWAY` at an HTTPS tunnel to your gateway and redeploy (the value is baked at build time).
