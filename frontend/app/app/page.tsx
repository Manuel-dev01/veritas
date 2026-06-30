"use client";
import { useEffect, useMemo, useState } from "react";
import { css } from "@/lib/css";
import { api, fp, Account, Claim, Note } from "@/lib/api";
import { useChain } from "@/lib/useChain";
import { IdentityProvider, useIdentity } from "@/lib/identity";
import { SplitFlap } from "@/components/SplitFlap";

const MONO = "'JetBrains Mono',monospace";

// map an on-chain note status enum to a board label
function noteLabel(status: string): string {
  if (status === "HELPFUL") return "HELPFUL";
  if (status === "NOT_HELPFUL") return "NOT HELPFUL";
  return "PENDING";
}
// claim-level headline status from its notes
function claimLabel(c: Claim): string {
  if (!c.notes || c.notes.length === 0) return "OPEN";
  if (c.notes.some((n) => n.status === "HELPFUL")) return "HELPFUL";
  if (c.notes.some((n) => n.status === "NOT_HELPFUL")) return "NOT HELPFUL";
  return "PENDING";
}
// the note a claim row drills into: prefer a HELPFUL one, else highest intercept, else first
function primaryNote(c: Claim): Note | undefined {
  if (!c.notes || c.notes.length === 0) return undefined;
  return [...c.notes].sort((a, b) => (b.score?.intercept || -1e9) - (a.score?.intercept || -1e9))[0];
}

function AppInner() {
  const { state, refresh } = useChain(2000);
  const { identities, current, setCurrent, me } = useIdentity();
  const [overlay, setOverlay] = useState<null | "note" | "submit" | "rep" | "live">(null);
  const [activeClaim, setActiveClaim] = useState<string | null>(null);
  const [busy, setBusy] = useState<string>("");

  const claims = state?.claims || [];
  const log = state?.log || [];
  const height = state?.height || 0;

  const close = () => setOverlay(null);
  const act = async (label: string, fn: () => Promise<unknown>) => {
    setBusy(label);
    try {
      await fn();
      await refresh();
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setBusy("");
    }
  };

  return (
    <div style={css("font-family:'Archivo',sans-serif;background:oklch(0.12 0.008 80);color:oklch(0.95 0.006 95);min-height:100vh;width:100%;position:relative;")}>
      {/* TOP BAR */}
      <header style={css("height:58px;display:flex;align-items:center;justify-content:space-between;padding:0 32px;border-bottom:1px solid oklch(0.22 0.008 80);position:sticky;top:0;background:oklch(0.115 0.008 80);z-index:6;")}>
        <div style={css("display:flex;align-items:center;gap:11px;")}>
          <Logo size={26} />
          <span style={css("font-size:18px;font-weight:800;letter-spacing:-0.03em;")}>Veritas</span>
          <span style={css(`font-family:${MONO};font-size:12px;color:oklch(0.52 0.01 80);margin-left:6px;`)}>/ the ledger</span>
        </div>
        <div style={css("display:flex;align-items:center;gap:18px;")}>
          <IdentitySwitcher />
          <div style={css(`display:flex;align-items:center;gap:9px;font-family:${MONO};font-size:12px;color:oklch(0.62 0.01 80);`)}>
            <span style={css("width:8px;height:8px;border-radius:50%;background:oklch(0.74 0.15 152);display:inline-block;box-shadow:0 0 0 4px oklch(0.74 0.15 152 / 0.18);")} />
            recompute · h:{height.toLocaleString()}
          </div>
        </div>
      </header>

      {/* BOARD */}
      <main style={css("max-width:1080px;margin:0 auto;padding:46px 32px 150px;")}>
        <h1 style={css("font-size:32px;font-weight:800;letter-spacing:-0.035em;margin:0;")}>Public verdict board</h1>
        <p style={css(`font-family:${MONO};font-size:12.5px;color:oklch(0.55 0.01 80);margin:10px 0 0;`)}>Every note, re-judged every block. Click any row to open the record.</p>

        <div style={css("position:relative;margin-top:34px;padding-left:96px;")}>
          <div style={css("position:absolute;left:62px;top:6px;bottom:6px;width:1px;background:oklch(0.24 0.008 80);")} />
          {claims.length === 0 && (
            <div style={css(`font-family:${MONO};font-size:13px;color:oklch(0.5 0.01 80);padding:20px 0;`)}>No claims yet — hit ＋ submit to post the first claim.</div>
          )}
          {claims.map((c) => {
            const label = claimLabel(c);
            const helpful = label === "HELPFUL";
            return (
              <div key={c.id} style={{ position: "relative" }}>
                <div style={css(`position:absolute;left:-96px;top:26px;width:50px;text-align:right;font-family:${MONO};font-size:11px;color:${helpful ? "oklch(0.55 0.13 152)" : "oklch(0.48 0.01 80)"};`)}>{c.height.toLocaleString()}</div>
                <div style={css(`position:absolute;left:-34px;top:30px;width:7px;height:7px;border-radius:50%;background:${helpful ? "oklch(0.74 0.15 152)" : "oklch(0.40 0.008 80)"};box-shadow:0 0 0 3px oklch(0.12 0.008 80);`)} />
                <button onClick={() => { setActiveClaim(c.id); setOverlay("note"); }} style={css("display:flex;align-items:center;gap:26px;width:100%;text-align:left;padding:18px 6px;border-bottom:1px solid oklch(0.19 0.008 80);border-radius:4px;")}>
                  <div style={css(`flex:1;min-width:0;font-family:'Archivo Narrow';font-weight:600;font-size:21px;letter-spacing:0.01em;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:${helpful ? "oklch(0.90 0.006 95)" : "oklch(0.62 0.006 95)"};`)}>{c.text || "(untitled claim)"}</div>
                  <SplitFlap text={label} helpful={helpful} />
                  <span style={css(`font-family:${MONO};font-size:15px;color:oklch(0.42 0.01 80);flex:none;width:16px;`)}>›</span>
                </button>
              </div>
            );
          })}
        </div>
      </main>

      {/* FLOATING ACTION BAR */}
      <div style={css("position:fixed;left:50%;bottom:26px;transform:translateX(-50%);display:flex;align-items:center;gap:4px;background:oklch(0.175 0.008 80);border:1px solid oklch(0.32 0.008 80);border-radius:999px;padding:7px 8px;box-shadow:0 10px 40px rgba(0,0,0,.5);z-index:7;")}>
        <button onClick={() => setOverlay("submit")} style={css(`font-family:${MONO};font-size:12.5px;color:oklch(0.16 0.03 152);background:oklch(0.74 0.15 152);font-weight:600;padding:10px 18px;border-radius:999px;`)}>＋ submit</button>
        <button disabled={busy === "seed"} onClick={() => act("seed", async () => { const r = await api.seed(); alert(r.hint); })} style={css(`font-family:${MONO};font-size:12.5px;color:oklch(0.82 0.006 95);padding:10px 16px;border-radius:999px;`)}>{busy === "seed" ? "seeding…" : "⟲ seed demo"}</button>
        <button onClick={() => setOverlay("rep")} style={css(`font-family:${MONO};font-size:12.5px;color:oklch(0.82 0.006 95);padding:10px 16px;border-radius:999px;`)}>◆ reputation</button>
        <button onClick={() => setOverlay("live")} style={css(`font-family:${MONO};font-size:12.5px;color:oklch(0.82 0.006 95);padding:10px 16px;border-radius:999px;display:flex;align-items:center;gap:7px;`)}>
          <span style={css("width:7px;height:7px;border-radius:50%;background:oklch(0.74 0.15 152);display:inline-block;")} />live
        </button>
      </div>

      {overlay && <button onClick={close} style={css("position:fixed;inset:0;background:oklch(0.10 0.008 80 / 0.72);backdrop-filter:blur(2px);z-index:8;animation:ovBackdrop .18s ease;width:100%;")} />}

      {overlay === "note" && <NoteOverlay claim={claims.find((c) => c.id === activeClaim)} close={close} busy={busy} onRate={(noteId, value) => act("rate", () => api.rate(current, noteId, value))} />}
      {overlay === "submit" && <SubmitOverlay claims={claims} close={close} busy={busy} current={current}
        onClaim={(text, url) => act("claim", () => api.claim(current, text, url))}
        onNote={(claimId, body, url) => act("note", () => api.note(current, claimId, body, url))} />}
      {overlay === "rep" && <RepOverlay me={me?.address} close={close} />}
      {overlay === "live" && <LiveOverlay height={height} claims={claims} log={log} close={close} />}
    </div>
  );
}

function Logo({ size }: { size: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 132 132">
      <line x1="20" y1="22" x2="66" y2="106" stroke="oklch(0.68 0.13 38)" strokeWidth="15" strokeLinecap="square" />
      <line x1="112" y1="22" x2="66" y2="106" stroke="oklch(0.66 0.11 250)" strokeWidth="15" strokeLinecap="square" />
      <circle cx="66" cy="106" r="15" fill="oklch(0.74 0.15 152)" />
    </svg>
  );
}

function IdentitySwitcher() {
  const { identities, current, setCurrent } = useIdentity();
  const me = identities.find((i) => i.id === current);
  const campColor = me?.camp === "A" ? "oklch(0.66 0.13 38)" : me?.camp === "B" ? "oklch(0.62 0.11 250)" : "oklch(0.74 0.15 152)";
  return (
    <div style={css(`display:flex;align-items:center;gap:8px;font-family:${MONO};font-size:12px;`)}>
      <span style={css("color:oklch(0.5 0.01 80);")}>acting as</span>
      <span style={css(`width:7px;height:7px;border-radius:50%;background:${campColor};display:inline-block;`)} />
      <select value={current} onChange={(e) => setCurrent(e.target.value)}
        style={css(`font-family:${MONO};font-size:12px;background:oklch(0.18 0.008 80);color:oklch(0.88 0.006 95);border:1px solid oklch(0.32 0.008 80);border-radius:6px;padding:5px 8px;`)}>
        {identities.map((i) => (
          <option key={i.id} value={i.id}>{i.name}{i.camp !== "neutral" ? ` · camp ${i.camp}` : ""}</option>
        ))}
      </select>
    </div>
  );
}

function NoteOverlay({ claim, close, onRate, busy }: { claim?: Claim; close: () => void; onRate: (noteId: string, value: string) => void; busy: string }) {
  const note = claim ? primaryNote(claim) : undefined;
  const status = note ? noteLabel(note.status) : "OPEN";
  const helpful = status === "HELPFUL";
  const sc = note?.score;
  const scoreStr = sc ? fp(sc.intercept) : "—";
  const aPct = sc ? Math.round((sc.meanA / 1_000_000) * 100) : 0;
  const bPct = sc ? Math.round((sc.meanB / 1_000_000) * 100) : 0;
  const total = sc?.numRaters ?? 0;
  const verdict = helpful ? "bridged · shown publicly" : sc ? (total < 3 ? "awaiting cross-camp ratings" : "failed to bridge") : "no note yet";
  const btnBase = `flex:1;font-family:${MONO};font-size:13px;padding:13px;border:1px solid oklch(0.32 0.008 80);background:oklch(0.18 0.008 80);color:oklch(0.82 0.006 95);`;
  return (
    <Panel width={920} close={close}>
      <div style={css("display:flex;align-items:center;gap:22px;padding:22px 28px;border-bottom:1px solid oklch(0.26 0.008 80);background:oklch(0.115 0.008 80);")}>
        <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.52 0.01 80);flex:none;`)}>claim {claim ? claim.id.slice(0, 8) : ""} /</div>
        <div style={css("flex:1;min-width:0;font-family:'Archivo Narrow';font-weight:600;font-size:22px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;")}>{claim?.text}</div>
        <SplitFlap text={status} helpful={helpful} />
        <CloseBtn close={close} />
      </div>
      <div style={css("padding:28px;display:grid;grid-template-columns:1.45fr 1fr;gap:30px;")}>
        <div>
          <div style={css(`font-family:${MONO};font-size:11px;letter-spacing:0.08em;text-transform:uppercase;color:oklch(0.50 0.01 80);margin-bottom:12px;`)}>the note {note ? "· " + note.id.slice(0, 8) : ""}</div>
          {note ? (
            <>
              <div style={css("font-size:20px;line-height:1.45;font-weight:500;margin-bottom:16px;")}>{note.body}</div>
              {note.url && <div style={css(`font-family:${MONO};font-size:12px;color:oklch(0.45 0.16 152);word-break:break-all;`)}>↳ {note.url}</div>}
              <div style={css("background:oklch(0.115 0.008 80);border:1px solid oklch(0.26 0.008 80);padding:20px;margin-top:24px;")}>
                <div style={css(`font-family:${MONO};font-size:11px;letter-spacing:0.06em;text-transform:uppercase;color:oklch(0.50 0.01 80);margin-bottom:16px;`)}>Your rating — moves the bridge live</div>
                <div style={css("display:flex;gap:10px;")}>
                  <button disabled={!!busy} onClick={() => onRate(note.id, "helpful")} style={css(btnBase)}>{busy === "rate" ? "…" : "Helpful"}</button>
                  <button disabled={!!busy} onClick={() => onRate(note.id, "not")} style={css(btnBase)}>Not helpful</button>
                </div>
                <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.52 0.01 80);margin-top:14px;`)}>your viewpoint is inferred from history, never declared.</div>
              </div>
            </>
          ) : (
            <div style={css(`font-family:${MONO};font-size:13px;color:oklch(0.55 0.01 80);`)}>No note on this claim yet. Use ＋ submit → Note to add one.</div>
          )}
        </div>
        <div style={css("background:oklch(0.115 0.008 80);border:1px solid oklch(0.26 0.008 80);padding:24px;")}>
          <div style={css(`font-family:${MONO};font-size:11px;letter-spacing:0.06em;text-transform:uppercase;color:oklch(0.50 0.01 80);margin-bottom:18px;`)}>Bridge breakdown</div>
          <div style={css("text-align:center;margin-bottom:22px;")}>
            <div style={css(`font-family:${MONO};font-size:50px;font-weight:600;letter-spacing:-0.03em;line-height:1;color:${helpful ? "oklch(0.55 0.16 152)" : "oklch(0.92 0.006 95)"};`)}>{scoreStr}</div>
            <div style={css(`font-family:${MONO};font-size:10px;color:oklch(0.50 0.01 80);margin-top:6px;`)}>note_intercept · threshold 0.40</div>
          </div>
          <Bar label="camp A agree" color="oklch(0.66 0.13 38)" fill="oklch(0.64 0.13 38)" pct={aPct} />
          <div style={{ height: 16 }} />
          <Bar label="camp B agree" color="oklch(0.62 0.11 250)" fill="oklch(0.62 0.11 250)" pct={bPct} />
          <div style={css(`border-top:1px solid oklch(0.24 0.008 80);margin-top:22px;padding-top:14px;font-family:${MONO};font-size:12px;line-height:1.7;color:oklch(0.62 0.01 80);`)}>
            <div>ratings = {total}{sc ? ` (A:${sc.countA} · B:${sc.countB})` : ""}</div>
            <div>verdict = {verdict}</div>
          </div>
        </div>
      </div>
    </Panel>
  );
}

function Bar({ label, color, fill, pct }: { label: string; color: string; fill: string; pct: number }) {
  return (
    <div>
      <div style={css(`display:flex;justify-content:space-between;font-family:${MONO};font-size:11px;margin-bottom:6px;`)}>
        <span style={css(`color:${color};`)}>{label}</span><span>{pct}%</span>
      </div>
      <div style={css("height:8px;background:oklch(0.22 0.008 80);position:relative;overflow:hidden;")}>
        <div style={css(`position:absolute;inset:0 auto 0 0;background:${fill};transition:width .3s;width:${pct}%;`)} />
      </div>
    </div>
  );
}

function SubmitOverlay({ claims, close, onClaim, onNote, busy, current }: { claims: Claim[]; close: () => void; onClaim: (t: string, u: string) => void; onNote: (cid: string, b: string, u: string) => void; busy: string; current: string }) {
  const [kind, setKind] = useState<"claim" | "note">("claim");
  const [text, setText] = useState("");
  const [url, setUrl] = useState("");
  const [claimId, setClaimId] = useState(claims[0]?.id || "");
  const isNote = kind === "note";
  const kindStyle = (on: boolean) => `font-family:${MONO};font-size:13px;padding:10px 16px;border:1px solid ${on ? "oklch(0.74 0.15 152);background:oklch(0.74 0.15 152);color:oklch(0.16 0.03 152);" : "oklch(0.32 0.008 80);background:transparent;color:oklch(0.66 0.01 80);"}`;
  const field = `width:100%;font-family:inherit;border:1px solid oklch(0.30 0.008 80);background:oklch(0.115 0.008 80);padding:14px;font-size:15px;line-height:1.5;color:oklch(0.92 0.006 95);`;
  const submit = () => { if (isNote) { if (!claimId) return alert("pick a claim"); onNote(claimId, text, url); } else onClaim(text, url); close(); };
  return (
    <Panel width={660} close={close}>
      <div style={css("display:flex;align-items:center;justify-content:space-between;padding:22px 28px;border-bottom:1px solid oklch(0.26 0.008 80);")}>
        <div><div style={css("font-size:21px;font-weight:700;letter-spacing:-0.02em;")}>Submit to the ledger</div><div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.52 0.01 80);margin-top:4px;`)}>a signed on-chain object — as {current}</div></div>
        <CloseBtn close={close} />
      </div>
      <div style={css("padding:26px 28px;")}>
        <div style={css("display:flex;gap:8px;margin-bottom:22px;")}>
          <button onClick={() => setKind("claim")} style={css(kindStyle(!isNote))}>Claim</button>
          <button onClick={() => setKind("note")} style={css(kindStyle(isNote))}>Note on a claim</button>
        </div>
        {isNote && (
          <>
            <Label>Claim to annotate</Label>
            <select value={claimId} onChange={(e) => setClaimId(e.target.value)} style={css(field + `font-family:${MONO};font-size:13px;margin-bottom:20px;`)}>
              {claims.map((c) => <option key={c.id} value={c.id}>{c.id.slice(0, 8)} · {c.text?.slice(0, 50)}</option>)}
            </select>
          </>
        )}
        <Label>{isNote ? "Your note" : "Your claim"}</Label>
        <textarea value={text} onChange={(e) => setText(e.target.value)} rows={4} placeholder={isNote ? "State the correction plainly. Notes that read as fair to people who disagree are the ones that bridge." : "State the claim exactly as it appears. Be specific and verifiable."} style={css(field)} />
        <div style={{ height: 16 }} />
        <Label>Source</Label>
        <input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://" style={css(field + `font-family:${MONO};font-size:13px;`)} />
        <div style={css("display:flex;justify-content:flex-end;align-items:center;margin-top:24px;")}>
          <button disabled={!!busy || !text} onClick={submit} style={css(`font-family:${MONO};font-size:13px;background:oklch(0.74 0.15 152);color:oklch(0.16 0.03 152);font-weight:600;padding:12px 22px;opacity:${!text ? "0.5" : "1"};`)}>{busy ? "signing…" : "sign & broadcast ↗"}</button>
        </div>
      </div>
    </Panel>
  );
}

function RepOverlay({ me, close }: { me?: string; close: () => void }) {
  const [acct, setAcct] = useState<Account | null>(null);
  useEffect(() => { if (me) api.account(me).then(setAcct).catch(() => {}); }, [me]);
  const camp = acct?.camp || "neutral";
  const pos = camp === "A" ? 12 : camp === "B" ? 84 : 48;
  const score = acct ? (acct.scoreFp / 1000).toFixed(1) : "0.0";
  const ledger = acct?.ledger || [];
  return (
    <Panel width={880} close={close}>
      <div style={css("display:flex;align-items:center;justify-content:space-between;padding:22px 28px;border-bottom:1px solid oklch(0.26 0.008 80);")}>
        <div><div style={css("font-size:21px;font-weight:700;letter-spacing:-0.02em;")}>Reputation</div><div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.52 0.01 80);margin-top:4px;`)}>soulbound · non-transferable · earned, never bought</div></div>
        <CloseBtn close={close} />
      </div>
      <div style={css("padding:26px 28px;display:grid;grid-template-columns:1fr 1.3fr;gap:24px;align-items:start;")}>
        <div style={css("background:oklch(0.115 0.008 80);border:1px solid oklch(0.26 0.008 80);padding:26px;")}>
          <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.55 0.01 80);`)}>{me ? me.slice(0, 10) : ""}…</div>
          <div style={css("font-size:68px;font-weight:800;letter-spacing:-0.04em;line-height:1;margin:12px 0 4px;color:oklch(0.78 0.15 152);")}>{score}</div>
          <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.58 0.01 80);`)}>reputation · soulbound</div>
          <div style={css("margin-top:24px;")}>
            <div style={css(`font-family:${MONO};font-size:10px;color:oklch(0.55 0.01 80);margin-bottom:10px;`)}>YOUR INFERRED VIEWPOINT</div>
            <div style={css("position:relative;height:24px;")}>
              <div style={css("position:absolute;top:11px;left:0;right:0;height:2px;background:linear-gradient(90deg,oklch(0.64 0.13 38),oklch(0.36 0.008 80),oklch(0.62 0.11 250));")} />
              <div style={css(`position:absolute;top:3px;left:${pos}%;width:14px;height:14px;border-radius:50%;background:oklch(0.78 0.15 152);border:2px solid oklch(0.155 0.008 80);`)} />
            </div>
            <div style={css(`display:flex;justify-content:space-between;font-family:${MONO};font-size:9px;color:oklch(0.56 0.01 80);margin-top:6px;`)}><span>camp A</span><span>camp B</span></div>
          </div>
        </div>
        <div>
          <div style={css(`font-family:${MONO};font-size:11px;letter-spacing:0.06em;text-transform:uppercase;color:oklch(0.50 0.01 80);margin-bottom:12px;`)}>Reputation ledger</div>
          <div style={css("border:1px solid oklch(0.26 0.008 80);")}>
            {ledger.length === 0 && <div style={css(`font-family:${MONO};font-size:12px;color:oklch(0.5 0.01 80);padding:16px;`)}>No reputation events yet — rate notes that bridge to earn.</div>}
            {ledger.slice().reverse().map((e, i) => (
              <div key={i} style={css("display:flex;align-items:center;justify-content:space-between;gap:14px;padding:13px 16px;border-bottom:1px solid oklch(0.19 0.008 80);background:oklch(0.13 0.008 80);")}>
                <div style={css(`font-family:${MONO};font-size:12.5px;`)}>{e.text}</div>
                <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.52 0.01 80);`)}>h{e.height}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </Panel>
  );
}

function LiveOverlay({ height, claims, log, close }: { height: number; claims: Claim[]; log: { height: number; kind: string; text: string }[]; close: () => void }) {
  const notesScored = claims.reduce((a, c) => a + (c.notes?.filter((n) => n.score).length || 0), 0);
  const flipped = claims.reduce((a, c) => a + (c.notes?.filter((n) => n.status === "HELPFUL").length || 0), 0);
  const lines = log.slice().reverse();
  return (
    <Panel width={900} close={close}>
      <div style={css("display:flex;align-items:center;justify-content:space-between;padding:22px 28px;border-bottom:1px solid oklch(0.26 0.008 80);")}>
        <div style={css("display:flex;align-items:center;gap:11px;")}><span style={css("width:9px;height:9px;border-radius:50%;background:oklch(0.74 0.15 152);display:inline-block;box-shadow:0 0 0 4px oklch(0.74 0.15 152 / 0.2);")} /><div><div style={css("font-size:21px;font-weight:700;letter-spacing:-0.02em;")}>Live · EndBlock</div><div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.52 0.01 80);margin-top:4px;`)}>every block, the plugin re-scores every dirty note — on-chain</div></div></div>
        <CloseBtn close={close} />
      </div>
      <div style={css("padding:26px 28px;display:grid;grid-template-columns:1fr 1.2fr;gap:24px;align-items:start;")}>
        <div style={css("background:oklch(0.115 0.008 80);border:1px solid oklch(0.26 0.008 80);padding:26px;")}>
          <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.55 0.01 80);`)}>current block</div>
          <div style={css(`font-family:${MONO};font-size:42px;font-weight:600;letter-spacing:-0.02em;line-height:1;margin:8px 0 22px;`)}>{height.toLocaleString()}</div>
          <div style={css("display:grid;grid-template-columns:1fr 1fr;gap:16px;font-family:" + MONO + ";")}>
            <div><div style={css("font-size:22px;font-weight:600;")}>{notesScored}</div><div style={css("font-size:10px;color:oklch(0.58 0.01 80);")}>notes scored</div></div>
            <div><div style={css("font-size:22px;font-weight:600;color:oklch(0.78 0.15 152);")}>{flipped}</div><div style={css("font-size:10px;color:oklch(0.58 0.01 80);")}>HELPFUL</div></div>
          </div>
        </div>
        <div style={css("background:oklch(0.11 0.008 80);border:1px solid oklch(0.26 0.008 80);padding:22px 24px;min-height:300px;")}>
          <div style={css(`font-family:${MONO};font-size:11px;letter-spacing:0.06em;text-transform:uppercase;color:oklch(0.55 0.01 80);margin-bottom:16px;`)}>recompute log · on-chain events</div>
          <div style={css(`display:flex;flex-direction:column;gap:8px;font-family:${MONO};font-size:12.5px;line-height:1.4;`)}>
            {lines.length === 0 && <div style={css("color:oklch(0.5 0.01 80);")}>waiting for scoring events… rate a note to trigger EndBlock.</div>}
            {lines.map((l, i) => (
              <div key={i} style={css(`animation:logIn .3s ease;color:${l.kind === "score" ? "oklch(0.78 0.15 152);font-weight:600" : "oklch(0.62 0.01 80)"};`)}>{`> h${l.height} ${l.kind}: ${l.text}`}</div>
            ))}
          </div>
        </div>
      </div>
    </Panel>
  );
}

function Panel({ width, close, children }: { width: number; close: () => void; children: React.ReactNode }) {
  return (
    <div style={css(`position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);width:${width}px;max-width:calc(100vw - 48px);max-height:calc(100vh - 60px);overflow-y:auto;background:oklch(0.155 0.008 80);border:1px solid oklch(0.34 0.008 80);box-shadow:0 40px 100px rgba(0,0,0,.6);z-index:9;animation:ovLift .22s cubic-bezier(.2,.7,.3,1);`)}>
      {children}
    </div>
  );
}
function CloseBtn({ close }: { close: () => void }) {
  return <button onClick={close} style={css(`font-family:${MONO};font-size:18px;color:oklch(0.55 0.01 80);flex:none;padding:0 4px;`)}>✕</button>;
}
function Label({ children }: { children: React.ReactNode }) {
  return <label style={css(`font-family:${MONO};font-size:11px;letter-spacing:0.06em;text-transform:uppercase;color:oklch(0.52 0.01 80);display:block;margin-bottom:8px;`)}>{children}</label>;
}

export default function AppPage() {
  return (
    <IdentityProvider>
      <AppInner />
    </IdentityProvider>
  );
}
