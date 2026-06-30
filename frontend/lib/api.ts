// Typed client for the Veritas gateway (veritas serve). The gateway signs + submits BLS txs and
// decodes chain events; the browser never signs. Mirrors cli/client.go + cli/serve.go.

const BASE = process.env.NEXT_PUBLIC_GATEWAY || "http://localhost:8080";

export interface Score {
  intercept: number; factor: number; mu: number;
  meanA: number; meanB: number; countA: number; countB: number;
  numRaters: number; status: string; height: number;
}
export interface Note {
  id: string; claimId: string; author: string; body: string; url: string;
  status: string; height: number; score: Score | null;
}
export interface Claim {
  id: string; submitter: string; url: string; text: string; height: number; notes: Note[];
}
export interface LogLine { height: number; kind: string; text: string }
export interface ChainState { height: number; claims: Claim[]; log: LogLine[] }
export interface Identity { id: string; name: string; camp: string; address: string; reputation: number }
export interface Account { address: string; scoreFp: number; camp: string; ledger: LogLine[] }

// Sent on every gateway request. `ngrok-skip-browser-warning` bypasses the ngrok-free interstitial
// (which otherwise returns an HTML warning page instead of our JSON); harmless on non-ngrok backends.
const SKIP = { "ngrok-skip-browser-warning": "true" } as const;

async function getJSON<T>(path: string): Promise<T> {
  const r = await fetch(BASE + path, { headers: { ...SKIP } });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}
async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const r = await fetch(BASE + path, {
    method: "POST", headers: { "Content-Type": "application/json", ...SKIP }, body: JSON.stringify(body),
  });
  const d = await r.json().catch(() => ({}));
  if (!r.ok || (d && d.error)) throw new Error((d && d.error) || `request failed (${r.status})`);
  return d as T;
}

export const api = {
  base: BASE,
  state: () => getJSON<ChainState>("/api/state"),
  identities: () => getJSON<Identity[]>("/api/identities"),
  account: (address: string) => getJSON<Account>("/api/account?address=" + address),
  claim: (identity: string, text: string, url: string) =>
    postJSON<{ claimId: string; txHash: string }>("/api/claim", { identity, text, url }),
  note: (identity: string, claimId: string, body: string, url: string) =>
    postJSON<{ noteId: string; txHash: string }>("/api/note", { identity, claimId, body, url }),
  rate: (identity: string, noteId: string, value: string) =>
    postJSON<{ txHash: string }>("/api/rate", { identity, noteId, value }),
  seed: () => postJSON<{ seedClaim: string; targetClaim: string; targetNote: string; hint: string }>("/api/seed", {}),
};

// fp formats a fixed-point (×1e6) on-chain value as a 0–1 score string (display only).
export const fp = (v: number) => (v / 1_000_000).toFixed(2);
