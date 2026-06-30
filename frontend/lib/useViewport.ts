"use client";
import { useEffect, useState } from "react";

// useIsMobile — true when the viewport is narrower than `bp`. The app is styled with inline styles
// (no media queries), so layout that must change on small screens reads this and switches values.
// Starts false (desktop) for stable SSR/first paint, then corrects on mount + resize.
export function useIsMobile(bp = 760): boolean {
  const [m, setM] = useState(false);
  useEffect(() => {
    const update = () => setM(window.innerWidth < bp);
    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, [bp]);
  return m;
}
