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

const sleep = (ms: number) => new Promise((res) => setTimeout(res, ms));

// The public path runs over an ngrok-free tunnel that occasionally drops a request (connection reset
// / 502-504) with no response received. Retry those transient failures with small backoff. Reads are
// always safe to retry; for writes we only retry when NO response came back (network throw) or the
// tunnel returned 5xx — never on a 4xx/app error — so a request the server actually accepted is not
// re-sent (rate is idempotent anyway; claim/note carry a fresh nonce, so we avoid needless dupes).
const RETRYABLE = new Set([502, 503, 504]);

async function getJSON<T>(path: string): Promise<T> {
  let last: unknown;
  for (let attempt = 0; attempt < 3; attempt++) {
    let r: Response;
    try {
      r = await fetch(BASE + path, { headers: { ...SKIP } });
    } catch (e) {
      last = e; if (attempt < 2) { await sleep(300 * (attempt + 1)); continue; } break; // network drop → retry
    }
    if (RETRYABLE.has(r.status) && attempt < 2) { await sleep(300 * (attempt + 1)); continue; }
    if (!r.ok) throw new Error(await r.text());
    return (await r.json()) as T;
  }
  throw last instanceof Error ? last : new Error("request failed");
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  let last: unknown;
  for (let attempt = 0; attempt < 3; attempt++) {
    let r: Response;
    try {
      r = await fetch(BASE + path, {
        method: "POST", headers: { "Content-Type": "application/json", ...SKIP }, body: JSON.stringify(body),
      });
    } catch (e) {
      // No response received (DNS / connection reset / timeout) → the request likely never landed;
      // safe to retry a transient tunnel blip.
      last = e; if (attempt < 2) { await sleep(300 * (attempt + 1)); continue; } break;
    }
    if (RETRYABLE.has(r.status) && attempt < 2) { await sleep(300 * (attempt + 1)); continue; } // tunnel/gateway 5xx
    const d = await r.json().catch(() => ({}));
    // A 4xx or an app-level {error} is a real rejection — surface it, never retry.
    if (!r.ok || (d && d.error)) throw new Error((d && d.error) || `request failed (${r.status})`);
    return d as T;
  }
  throw last instanceof Error ? last : new Error("request failed");
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
