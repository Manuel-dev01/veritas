"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { css } from "@/lib/css";
import { useChain } from "@/lib/useChain";
import { SplitFlap } from "@/components/SplitFlap";

const MONO = "'JetBrains Mono',monospace";

export default function Landing() {
  const { state } = useChain(3000);
  const height = state?.height || 184772;
  // board re-spin cadence — the mockup re-clatters every row every ~6.2s (its nextBlock cycle)
  const [pulse, setPulse] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setPulse((p) => p + 1), 6200);
    return () => clearInterval(t);
  }, []);
  // board: live claims (top 3) if present, else illustrative
  const liveRows = (state?.claims || []).slice(0, 3).map((c) => ({
    claim: c.text || "(untitled claim)",
    status: c.notes?.some((n) => n.status === "HELPFUL") ? "HELPFUL" : c.notes && c.notes.length ? (c.notes.some((n) => n.status === "NOT_HELPFUL") ? "NOT HELPFUL" : "PENDING") : "OPEN",
  }));
  const rows = liveRows.length
    ? liveRows
    : [
        { claim: '"Chart drops the 2019 baseline — the rise is 14%, not 200%."', status: "HELPFUL" },
        { claim: '"The policy took effect after the period shown."', status: "NOT HELPFUL" },
        { claim: '"Foundation signed a statement denying the endorsement."', status: "PENDING" },
      ];

  return (
    <div style={css("font-family:'Archivo',sans-serif;background:oklch(0.15 0.008 80);color:oklch(0.95 0.006 95);width:100%;overflow:hidden;")}>
      {/* TICKER */}
      <div style={css("border-bottom:1px solid oklch(0.30 0.008 80);overflow:hidden;white-space:nowrap;background:oklch(0.13 0.008 80);")}>
        <div style={css(`display:inline-flex;gap:34px;font-family:${MONO};font-size:11px;color:oklch(0.62 0.01 80);padding:9px 0;animation:vmarquee 38s linear infinite;will-change:transform;`)}>
          {[0, 1].map((k) => (
            <span key={k} style={{ display: "inline-flex", gap: 34 }}>
              <span>h:{height.toLocaleString()} · recompute ✓</span>
              <span style={css("color:oklch(0.74 0.15 152);")}>note → HELPFUL when both camps agree</span>
              <span>bridging on-chain · EndBlock</span>
              <span>matrix factorization · note_intercept ≥ 0.40</span>
              <span>rep += honest_rating</span>
              <span>reputation is soulbound</span>
              <span>chain canopy.nested</span>
            </span>
          ))}
        </div>
      </div>

      {/* HERO */}
      <section style={css("width:100%;padding:30px 44px 76px;")}>
        <div style={css("display:flex;justify-content:space-between;align-items:center;margin-bottom:60px;")}>
          <div style={css("display:flex;align-items:center;gap:11px;")}><Logo size={28} /><span style={css("font-size:19px;font-weight:800;letter-spacing:-0.03em;")}>Veritas</span></div>
          <div style={css(`display:flex;align-items:center;gap:22px;font-family:${MONO};font-size:12px;`)}>
            <Link href="/app" style={css("color:oklch(0.72 0.01 80);text-decoration:none;")}>product</Link>
            <Link href="/app" style={css("text-decoration:none;background:oklch(0.74 0.15 152);color:oklch(0.16 0.03 152);padding:9px 16px;font-weight:600;")}>launch ↗</Link>
          </div>
        </div>
        <div style={css("max-width:1180px;")}>
          <div style={css(`font-family:${MONO};font-size:12px;letter-spacing:0.18em;text-transform:uppercase;color:oklch(0.74 0.15 152);margin-bottom:24px;`)}>Sovereign Canopy chain · the referee runs in the open</div>
          <h1 style={css("font-size:clamp(40px,6.6vw,96px);line-height:0.9;letter-spacing:-0.045em;font-weight:900;margin:0;max-width:16ch;")}>Community Notes, but nobody can secretly rig it.</h1>
          <p style={css("font-size:clamp(17px,1.7vw,21px);line-height:1.5;color:oklch(0.80 0.008 95);max-width:54ch;margin:28px 0 0;")}>The bridging algorithm <strong style={css("color:oklch(0.95 0.006 95);font-weight:600;")}>X and Meta run on private servers</strong> runs here on-chain — recomputed every block, verifiable by anyone.</p>
        </div>
        {/* board */}
        <div style={css("margin-top:56px;max-width:1320px;")}>
          <div style={css("display:flex;justify-content:space-between;align-items:flex-end;margin-bottom:14px;")}>
            <div style={css(`font-family:${MONO};font-size:12px;letter-spacing:0.16em;text-transform:uppercase;color:oklch(0.60 0.01 80);`)}>Public verdict board</div>
            <div style={css(`font-family:${MONO};font-size:12px;color:oklch(0.60 0.01 80);display:flex;align-items:center;gap:9px;`)}><span style={css("width:8px;height:8px;border-radius:50%;background:oklch(0.74 0.15 152);display:inline-block;box-shadow:0 0 0 4px oklch(0.74 0.15 152 / 0.2);")} />recompute · h:{height.toLocaleString()}</div>
          </div>
          <div style={css("background:oklch(0.10 0.008 80);border:1px solid oklch(0.27 0.008 80);padding:10px 22px;")}>
            {rows.map((r, i) => {
              const helpful = r.status === "HELPFUL";
              return (
                <div key={i} style={css("display:flex;align-items:center;gap:28px;padding:20px 0;border-bottom:1px solid oklch(0.18 0.008 80);")}>
                  <div style={css(`flex:1;min-width:0;font-family:'Archivo Narrow';font-weight:600;font-size:clamp(16px,1.55vw,22px);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:${helpful ? "oklch(0.86 0.006 95)" : "oklch(0.56 0.006 95)"};`)}>{r.claim}</div>
                  <SplitFlap text={r.status} helpful={helpful} big spinKey={pulse} spinDelayMs={i * 240} />
                </div>
              );
            })}
          </div>
        </div>
      </section>

      {/* THESIS */}
      <section style={css("background:oklch(0.96 0.006 95);color:oklch(0.21 0.008 80);padding:96px 44px;")}>
        <div style={css("max-width:1240px;margin:0 auto;")}>
          <div style={css(`font-family:${MONO};font-size:12px;letter-spacing:0.14em;text-transform:uppercase;color:oklch(0.50 0.01 80);margin-bottom:30px;`)}>§ the rule that changes everything</div>
          <div style={css("font-size:clamp(30px,4.6vw,62px);line-height:1.04;letter-spacing:-0.035em;font-weight:700;max-width:20ch;")}>A note goes <span style={css("color:oklch(0.55 0.16 152);")}>HELPFUL</span> only when people who normally <span style={css("color:oklch(0.55 0.13 38);")}>disagree</span> with each other <span style={css("color:oklch(0.50 0.11 250);")}>agree</span> it&apos;s fair.</div>
          <div style={css("display:grid;grid-template-columns:repeat(3,1fr);gap:0;margin-top:64px;border-top:1.5px solid oklch(0.21 0.008 80);")}>
            {[
              ["01", "A simple majority can be brigaded. A coalition of people who agree on nothing else, can't."],
              ["02", "The bridging score measures exactly that — agreement that crosses the divide, not raw vote count."],
              ["03", "Here, that computation is a public transaction. The referee can't be bought because everyone holds the whistle."],
            ].map(([n, t], i) => (
              <div key={i} style={css(`padding:26px ${i === 2 ? "0 26px 26px" : "26px"};${i < 2 ? "border-right:1px solid oklch(0.86 0.01 90);" : ""}${i === 0 ? "padding-left:0;" : ""}`)}>
                <div style={css(`font-family:${MONO};font-size:54px;font-weight:500;letter-spacing:-0.03em;line-height:1;${n === "03" ? "color:oklch(0.55 0.16 152);" : ""}`)}>{n}</div>
                <div style={css("font-size:16px;line-height:1.45;color:oklch(0.34 0.01 80);margin-top:16px;")}>{t}</div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* INTERACTIVE BRIDGING DEMO */}
      <BridgingDemo />

      {/* VIDEO BAND */}
      <section style={css("position:relative;height:560px;width:100%;overflow:hidden;background:oklch(0.115 0.008 80);border-top:1px solid oklch(0.30 0.008 80);border-bottom:1px solid oklch(0.30 0.008 80);")}>
        <EndBlockCanvas />
        <div style={css(`position:absolute;top:26px;left:44px;font-family:${MONO};font-size:11px;color:oklch(0.60 0.01 80);letter-spacing:0.08em;z-index:2;`)}>[ EndBlock · recompute loop ]</div>
        <div style={css(`position:absolute;top:26px;right:44px;font-family:${MONO};font-size:11px;color:oklch(0.60 0.01 80);z-index:2;`)}>notes enter pending · exit judged</div>
        <div style={css("position:absolute;bottom:28px;left:44px;right:44px;display:flex;justify-content:space-between;align-items:flex-end;gap:24px;z-index:2;pointer-events:none;")}>
          <div style={css("font-size:clamp(20px,2.5vw,32px);font-weight:700;letter-spacing:-0.02em;color:oklch(0.94 0.006 95);max-width:20ch;line-height:1.08;")}>Every block, the chain re-judges every note — live.</div>
          <div style={css(`font-family:${MONO};font-size:12px;color:oklch(0.58 0.01 80);text-align:right;line-height:1.7;`)}>no operator · no private server<br />just the chain, scoring in the open</div>
        </div>
      </section>

      {/* HOW IT WORKS */}
      <section style={css("background:oklch(0.96 0.006 95);color:oklch(0.21 0.008 80);padding:96px 44px;")}>
        <div style={css("max-width:1240px;margin:0 auto;")}>
          <div style={css(`font-family:${MONO};font-size:12px;letter-spacing:0.14em;text-transform:uppercase;color:oklch(0.50 0.01 80);margin-bottom:44px;`)}>§ the loop</div>
          {[
            ["step 01", "Submit a claim, attach a note", "Anyone posts a claim. Anyone attaches a note with sources. Both become on-chain objects with a permanent history.", "tx submit_claim\ntx submit_note\nstate → NEEDS RATINGS"],
            ["step 02", "Raters from every camp weigh in", "Each rating is signed and stored. The model learns each rater's latent viewpoint from their whole history — no one declares a side.", "tx rate_note\nviewpoint inferred\nsigned · stored"],
            ["step 03", "EndBlock recomputes the bridge", "Every block, the plugin re-scores every note via fixed-point matrix factorization. Cross-camp agreement clears the threshold → HELPFUL.", "EndBlock()\n∀ note: re-score\nstate → HELPFUL"],
            ["step 04", "Reputation settles, honestly", "Helping the bridge builds reputation. Gaming it decays reputation. The score is non-transferable — earned, never bought.", "rep += honest\nrep −= bad_faith\nsoulbound"],
          ].map(([s, h, d, code], i) => (
            <div key={i} style={css(`display:grid;grid-template-columns:90px 1fr 0.8fr;gap:30px;align-items:start;padding:30px 0;border-top:${i === 0 ? "1.5px" : "1px"} solid ${i === 0 ? "oklch(0.21 0.008 80)" : "oklch(0.86 0.01 90)"};`)}>
              <div style={css(`font-family:${MONO};font-size:13px;color:${s === "step 03" ? "oklch(0.55 0.16 152)" : "oklch(0.50 0.01 80)"};`)}>{s}</div>
              <div><div style={css("font-size:26px;font-weight:700;letter-spacing:-0.02em;")}>{h}</div><div style={css("font-size:16px;color:oklch(0.34 0.01 80);margin-top:8px;line-height:1.45;")}>{d}</div></div>
              <div style={css(`font-family:${MONO};font-size:12px;color:oklch(0.45 0.01 80);line-height:1.9;white-space:pre-line;`)}>{code}</div>
            </div>
          ))}
        </div>
      </section>

      {/* CTA */}
      <section style={css("background:oklch(0.15 0.008 80);color:oklch(0.95 0.006 95);padding:110px 44px;")}>
        <div style={css("max-width:760px;margin:0 auto;text-align:center;")}>
          <Logo size={46} />
          <h2 style={css("font-size:clamp(34px,5vw,64px);line-height:1.0;letter-spacing:-0.04em;font-weight:800;margin:24px 0 24px;")}>Don&apos;t trust the referee.<br />Read it.</h2>
          <p style={css("font-size:18px;line-height:1.5;color:oklch(0.78 0.008 95);max-width:46ch;margin:0 auto 38px;")}>Veritas is a sovereign Canopy Nested Chain. The fact-checking is the consensus. Open the protocol and watch a block judge itself.</p>
          <Link href="/app" style={css("text-decoration:none;background:oklch(0.74 0.15 152);color:oklch(0.16 0.03 152);padding:15px 28px;font-weight:700;font-size:16px;")}>Launch the protocol ↗</Link>
        </div>
      </section>

      <footer style={css(`background:oklch(0.13 0.008 80);border-top:1px solid oklch(0.30 0.008 80);padding:30px 44px;display:flex;justify-content:space-between;align-items:center;font-family:${MONO};font-size:12px;color:oklch(0.58 0.01 80);`)}>
        <div>veritas · sovereign chain · bridging on-chain</div>
        <Link href="/app" style={css("color:oklch(0.58 0.01 80);text-decoration:none;")}>product</Link>
      </footer>
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

// client-side teaching demo (simplified bridging, per the mockup's own note)
function BridgingDemo() {
  const [a, setA] = useState([true, true, false, true, false]);
  const [b, setB] = useState([false, true, true, false, false]);
  const ca = a.filter(Boolean).length, cb = b.filter(Boolean).length;
  const af = ca / 5, bf = cb / 5, total = ca + cb;
  const score = af + bf > 0 ? (2 * af * bf) / (af + bf) : 0;
  let status = "NOT HELPFUL", explain = "// agreement too lopsided to bridge";
  if (total < 2) { status = "NEEDS RATINGS"; explain = "// awaiting cross-camp ratings"; }
  else if (score >= 0.4 && af > 0 && bf > 0) { status = "HELPFUL"; explain = "// both camps agree → the note bridges the divide"; }
  else if (af === 0 || bf === 0) { status = "NOT HELPFUL"; explain = "// only one camp agrees → a majority, not a bridge"; }
  const helpful = status === "HELPFUL";
  const raterBtn = (on: boolean, camp: string) => {
    const col = camp === "a" ? "0.64 0.13 38" : "0.62 0.11 250";
    const base = `font-family:${MONO};font-size:12px;text-align:left;padding:10px 12px;transition:all .18s;width:100%;`;
    return on ? base + `background:oklch(${col});color:oklch(0.97 0.02 ${camp === "a" ? "38" : "250"});border:1px solid oklch(${col});font-weight:600;` : base + "background:transparent;color:oklch(0.55 0.01 80);border:1px solid oklch(0.32 0.008 80);";
  };
  const statusStyle = helpful ? "background:oklch(0.74 0.15 152);color:oklch(0.16 0.03 152);" : status === "NEEDS RATINGS" ? "background:oklch(0.30 0.008 80);color:oklch(0.72 0.01 80);" : "background:oklch(0.95 0.006 95);color:oklch(0.18 0.008 80);";
  return (
    <section style={css("background:oklch(0.13 0.008 80);color:oklch(0.95 0.006 95);padding:96px 44px;")}>
      <div style={css("max-width:1240px;margin:0 auto;")}>
        <div style={css(`font-family:${MONO};font-size:12px;letter-spacing:0.14em;text-transform:uppercase;color:oklch(0.74 0.15 152);margin-bottom:14px;`)}>§ run the referee yourself</div>
        <h2 style={css("font-size:clamp(28px,3.6vw,46px);line-height:1.05;letter-spacing:-0.03em;font-weight:700;margin:0 0 50px;max-width:18ch;")}>Toggle who agrees. Watch the status decide itself.</h2>
        <div style={css("display:grid;grid-template-columns:1fr 1.5fr 1fr;gap:0;border:1px solid oklch(0.30 0.008 80);align-items:stretch;")}>
          <div style={css("padding:30px 28px;border-right:1px solid oklch(0.30 0.008 80);")}>
            <div style={css(`font-family:${MONO};font-size:11px;letter-spacing:0.08em;text-transform:uppercase;color:oklch(0.66 0.13 38);margin-bottom:6px;`)}>Camp A · clay</div>
            <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.55 0.01 80);margin-bottom:22px;`)}>{ca}/5 rate helpful</div>
            <div style={css("display:flex;flex-direction:column;gap:10px;")}>
              {a.map((on, i) => <button key={i} onClick={() => setA((s) => s.map((v, j) => (j === i ? !v : v)))} style={css(raterBtn(on, "a"))}>rater_a{i + 1} · {on ? "helpful" : "—"}</button>)}
            </div>
          </div>
          <div style={css("padding:30px 32px;border-right:1px solid oklch(0.30 0.008 80);display:flex;flex-direction:column;justify-content:space-between;gap:24px;")}>
            <div style={css(helpful ? "padding:22px;border:1px solid oklch(0.74 0.15 152);background:oklch(0.74 0.15 152 / 0.10);box-shadow:0 0 0 4px oklch(0.74 0.15 152 / 0.08);transition:all .3s;" : "padding:22px;border:1px solid oklch(0.30 0.008 80);background:oklch(0.16 0.008 80);transition:all .3s;")}>
              <div style={css(`font-family:${MONO};font-size:10px;letter-spacing:0.08em;text-transform:uppercase;color:oklch(0.60 0.01 80);margin-bottom:12px;`)}>Note attached to a claim</div>
              <div style={css("font-size:18px;line-height:1.4;font-weight:500;")}>&quot;The chart drops the 2019 baseline, which makes the rise look ~3× steeper than it is. Full series: bls.gov/data.&quot;</div>
            </div>
            <div>
              <div style={css("display:flex;align-items:flex-end;justify-content:space-between;margin-bottom:14px;")}>
                <div><div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.55 0.01 80);`)}>bridge_score</div><div style={css(`font-family:${MONO};font-size:46px;font-weight:600;letter-spacing:-0.02em;line-height:1;`)}>{score.toFixed(2)}</div></div>
                <div style={css(`font-family:${MONO};font-size:13px;font-weight:600;letter-spacing:0.04em;padding:7px 13px;white-space:nowrap;${statusStyle}`)}>{status}</div>
              </div>
              <div style={css("display:flex;flex-direction:column;gap:6px;margin-bottom:12px;")}>
                <div style={css("height:7px;background:oklch(0.24 0.008 80);position:relative;overflow:hidden;")}><div style={css(`position:absolute;inset:0 auto 0 0;background:oklch(0.64 0.13 38);transition:width .3s;width:${af * 100}%;`)} /></div>
                <div style={css("height:7px;background:oklch(0.24 0.008 80);position:relative;overflow:hidden;")}><div style={css(`position:absolute;inset:0 auto 0 0;background:oklch(0.62 0.11 250);transition:width .3s;width:${bf * 100}%;`)} /></div>
              </div>
              <div style={css(`font-family:${MONO};font-size:12px;line-height:1.5;color:oklch(0.70 0.01 80);`)}>{explain}</div>
            </div>
          </div>
          <div style={css("padding:30px 28px;")}>
            <div style={css(`font-family:${MONO};font-size:11px;letter-spacing:0.08em;text-transform:uppercase;color:oklch(0.62 0.11 250);margin-bottom:6px;`)}>Camp B · slate</div>
            <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.55 0.01 80);margin-bottom:22px;`)}>{cb}/5 rate helpful</div>
            <div style={css("display:flex;flex-direction:column;gap:10px;")}>
              {b.map((on, i) => <button key={i} onClick={() => setB((s) => s.map((v, j) => (j === i ? !v : v)))} style={css(raterBtn(on, "b"))}>rater_b{i + 1} · {on ? "helpful" : "—"}</button>)}
            </div>
          </div>
        </div>
        <div style={css(`font-family:${MONO};font-size:11px;color:oklch(0.50 0.01 80);margin-top:16px;`)}>// the real protocol uses fixed-point matrix factorization over every rater. same principle: reward agreement across latent viewpoints.</div>
      </div>
    </section>
  );
}

// EndBlock conveyor — faithful port of the mockup's video-band canvas loop.
function EndBlockCanvas() {
  const ref = useRef<HTMLCanvasElement | null>(null);
  useEffect(() => {
    const cv = ref.current; if (!cv) return;
    const ctx = cv.getContext("2d"); if (!ctx) return;
    let w = 0, h = 0, raf = 0, last = 0, spawn = 0.3, block = 184772, pulse = 0, gateFlash = 0;
    const notes: { t: number; help: boolean; af: number; bf: number }[] = [];
    const resize = () => {
      const r = cv.getBoundingClientRect(); w = r.width; h = r.height;
      const dpr = Math.min(2, window.devicePixelRatio || 1);
      cv.width = Math.max(1, Math.round(w * dpr)); cv.height = Math.max(1, Math.round(h * dpr));
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    };
    resize(); window.addEventListener("resize", resize);
    const CLAY = "196,104,63", SLATE = "90,127,192", GREEN = "52,184,119", GRAY = "124,121,112";
    const lerp = (a: number, b: number, t: number) => a + (b - a) * t;
    const ease = (t: number) => (t < 0 ? 0 : t > 1 ? 1 : t * t * (3 - 2 * t));
    const mix = (a: string, b: string, t: number) => { const A = a.split(","), B = b.split(","); return A.map((v, i) => Math.round(+v + (+B[i] - +v) * t)).join(","); };
    const loop = (ts: number) => {
      raf = requestAnimationFrame(loop);
      if (!w) return;
      let dt = (ts - last) / 1000; if (!last || dt > 0.1 || dt < 0) dt = 0.016; last = ts;
      ctx.clearRect(0, 0, w, h);
      const base = h * 0.5, gateX = w * 0.5, DUR = 7.8, pw = 156, ph = 44;
      ctx.strokeStyle = "rgba(150,148,140,0.12)"; ctx.lineWidth = 1;
      ctx.beginPath(); ctx.moveTo(0, base + 22); ctx.lineTo(w, base + 22); ctx.stroke();
      spawn -= dt;
      if (spawn <= 0) { spawn = 1.2; const help = Math.random() < 0.68; notes.push({ t: 0, help, af: help ? 0.66 + Math.random() * 0.22 : Math.random() < 0.5 ? 0.86 : 0.18, bf: help ? 0.62 + Math.random() * 0.22 : Math.random() < 0.5 ? 0.12 : 0.82 }); }
      pulse += dt; if (pulse >= 3.6) { pulse = 0; block++; gateFlash = 1; }
      gateFlash = Math.max(0, gateFlash - dt * 1.5);
      const gAl = Math.min(1, 0.5 + 0.32 * Math.sin(pulse * 1.7) + gateFlash * 0.5);
      ctx.save(); ctx.shadowColor = `rgba(${GREEN},0.9)`; ctx.shadowBlur = 13 + gateFlash * 28;
      ctx.strokeStyle = `rgba(${GREEN},${gAl})`; ctx.lineWidth = 2;
      ctx.beginPath(); ctx.moveTo(gateX, h * 0.15); ctx.lineTo(gateX, h * 0.85); ctx.stroke(); ctx.restore();
      for (let i = notes.length - 1; i >= 0; i--) { notes[i].t += dt / DUR; if (notes[i].t >= 1.08) notes.splice(i, 1); }
      notes.forEach((n) => {
        const x = -0.1 * w + n.t * 1.2 * w;
        const judged = x > gateX;
        const after = Math.max(0, Math.min(1, (x - gateX) / (w * 0.14)));
        const y = lerp(base, n.help ? base - h * 0.24 : base + h * 0.2, ease(after));
        const vcol = n.help ? GREEN : "96,93,86";
        const rgb = judged ? mix(GRAY, vcol, ease(after)) : GRAY;
        const al = Math.max(0, Math.min(Math.min(1, n.t / 0.05), Math.min(1, (1.08 - n.t) / 0.14)));
        ctx.globalAlpha = al;
        ctx.fillStyle = `rgba(${rgb},0.13)`; ctx.strokeStyle = `rgba(${rgb},0.85)`; ctx.lineWidth = 1.4;
        ctx.beginPath(); (ctx as any).roundRect(x - pw / 2, y - ph / 2, pw, ph, 7); ctx.fill(); ctx.stroke();
        ctx.fillStyle = `rgb(${rgb})`; ctx.font = '600 11px "JetBrains Mono", monospace'; ctx.textAlign = "left"; ctx.textBaseline = "middle";
        ctx.fillText(judged ? (n.help ? "HELPFUL" : "NOT HELPFUL") : "pending", x - pw / 2 + 13, y - 8);
        const barX = x - pw / 2 + 13, barW = pw - 26, grow = Math.min(1, n.t * 1.7);
        ctx.fillStyle = `rgba(${CLAY},${al * 0.92})`; ctx.fillRect(barX, y + 5, barW * (judged ? n.af : grow * 0.45), 2.6);
        ctx.fillStyle = `rgba(${SLATE},${al * 0.92})`; ctx.fillRect(barX, y + 11, barW * (judged ? n.bf : grow * 0.4), 2.6);
        ctx.globalAlpha = 1;
      });
      ctx.fillStyle = `rgba(${GREEN},0.95)`; ctx.font = '600 13px "JetBrains Mono", monospace'; ctx.textAlign = "center"; ctx.textBaseline = "alphabetic";
      ctx.fillText("EndBlock()", gateX, h * 0.15 - 14);
      ctx.fillStyle = "rgba(150,148,140,0.85)"; ctx.font = '500 12px "JetBrains Mono", monospace';
      ctx.fillText("h:" + block.toLocaleString(), gateX, h * 0.85 + 26);
      ctx.textAlign = "left";
    };
    raf = requestAnimationFrame(loop);
    return () => { cancelAnimationFrame(raf); window.removeEventListener("resize", resize); };
  }, []);
  return <canvas ref={ref} style={{ position: "absolute", inset: 0, width: "100%", height: "100%", display: "block" }} />;
}
