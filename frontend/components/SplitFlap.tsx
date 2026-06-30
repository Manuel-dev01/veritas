"use client";
import { useEffect, useRef, useState } from "react";
import { css } from "@/lib/css";

// SplitFlap — the airport-board status display, ported from the mockup. Tiles clatter through random
// letters and settle (staggered) onto the target string; HELPFUL settles green.
const CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
const rc = () => CHARS[Math.floor(Math.random() * 26)];

type Tile = { target: string; char: string; locked: boolean };

function tileStyle(t: Tile, helpful: boolean, big: boolean): string {
  const sz = big ? "width:30px;height:44px;font-size:22px;" : "width:26px;height:38px;font-size:18px;";
  const base =
    "flex:none;display:flex;align-items:center;justify-content:center;font-family:'JetBrains Mono',monospace;font-weight:600;border-radius:2px;" + sz;
  if (t.target === " ") return big ? "width:14px;flex:none;" : "width:11px;flex:none;";
  if (!t.locked)
    return base + "color:oklch(0.78 0.006 95);background:linear-gradient(to bottom, oklch(0.31 0.008 80) 0 47%, oklch(0.10 0.008 80) 47% 53%, oklch(0.27 0.008 80) 53% 100%);";
  if (helpful)
    return base + "color:oklch(0.16 0.03 152);background:linear-gradient(to bottom, oklch(0.79 0.15 152) 0 47%, oklch(0.55 0.13 152) 47% 53%, oklch(0.72 0.15 152) 53% 100%);";
  return base + "color:oklch(0.84 0.006 95);background:linear-gradient(to bottom, oklch(0.33 0.008 80) 0 47%, oklch(0.11 0.008 80) 47% 53%, oklch(0.28 0.008 80) 53% 100%);";
}

export function SplitFlap({ text, helpful = false, big = false }: { text: string; helpful?: boolean; big?: boolean }) {
  const [tiles, setTiles] = useState<Tile[]>(() => [...text].map((ch) => ({ target: ch, char: ch === " " ? " " : rc(), locked: ch === " " })));
  const timers = useRef<ReturnType<typeof setTimeout>[]>([]);

  // continuous clatter of any unlocked tiles
  useEffect(() => {
    const tick = setInterval(() => {
      setTiles((ts) => {
        let changed = false;
        const n = ts.map((t) => (t.locked ? t : ((changed = true), { ...t, char: rc() })));
        return changed ? n : ts;
      });
    }, 55);
    return () => clearInterval(tick);
  }, []);

  // re-spin whenever the target changes
  useEffect(() => {
    timers.current.forEach(clearTimeout);
    timers.current = [];
    const targets = [...text];
    setTiles(targets.map((ch) => (ch === " " ? { target: " ", char: " ", locked: true } : { target: ch, char: rc(), locked: false })));
    let order = 0;
    targets.forEach((ch, j) => {
      if (ch === " ") return;
      const delay = 380 + order * 80;
      order++;
      timers.current.push(
        setTimeout(() => {
          setTiles((ts) => {
            const n = ts.slice();
            if (!n[j]) return ts;
            n[j] = { ...n[j], char: n[j].target, locked: true };
            return n;
          });
        }, delay),
      );
    });
    return () => timers.current.forEach(clearTimeout);
  }, [text]);

  return (
    <div style={{ display: "flex", gap: "4px", flex: "none", alignItems: "center" }}>
      {tiles.map((t, i) => (
        <span key={i} style={css(tileStyle(t, helpful, big))}>
          {t.char}
        </span>
      ))}
    </div>
  );
}
