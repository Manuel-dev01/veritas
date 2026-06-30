"use client";
import { useEffect, useRef, useState } from "react";
import { api, ChainState } from "./api";

// useChain polls the gateway's /api/state every `ms` and returns the latest board + height + log.
export function useChain(ms = 2000): { state: ChainState | null; refresh: () => void } {
  const [state, setState] = useState<ChainState | null>(null);
  const alive = useRef(true);

  const refresh = () => api.state().then((s) => { if (alive.current) setState(s); }).catch(() => {});

  useEffect(() => {
    alive.current = true;
    refresh();
    const t = setInterval(refresh, ms);
    return () => { alive.current = false; clearInterval(t); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ms]);

  return { state, refresh };
}
