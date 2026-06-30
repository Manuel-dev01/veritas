import type { CSSProperties } from "react";

// css parses a CSS declaration string (as used verbatim in the original design mockups) into a React style
// object, so mockup inline styles port near-verbatim. Splits on ';' (no ';' appears inside our values
// — gradients/oklch use only commas) and on the first ':' (values like linear-gradient have none).
export function css(s: string): CSSProperties {
  const out: Record<string, string> = {};
  for (const decl of s.split(";")) {
    const i = decl.indexOf(":");
    if (i < 0) continue;
    const prop = decl.slice(0, i).trim();
    if (!prop) continue;
    const val = decl.slice(i + 1).trim();
    const key = prop.replace(/-([a-z])/g, (_m, c: string) => c.toUpperCase());
    out[key] = val;
  }
  return out as CSSProperties;
}

// merge concatenates css strings (handy for conditional style fragments).
export function merge(...parts: (string | false | null | undefined)[]): CSSProperties {
  return css(parts.filter(Boolean).join(";"));
}
