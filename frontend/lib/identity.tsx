"use client";
import React, { createContext, useContext, useEffect, useState } from "react";
import { api, Identity } from "./api";

interface IdentityCtx {
  identities: Identity[];
  current: string; // identity id (e.g. "you", "a1", "b2")
  setCurrent: (id: string) => void;
  me: Identity | undefined;
}

const Ctx = createContext<IdentityCtx>({ identities: [], current: "you", setCurrent: () => {}, me: undefined });

export function IdentityProvider({ children }: { children: React.ReactNode }) {
  const [identities, setIdentities] = useState<Identity[]>([]);
  const [current, setCurrentState] = useState<string>("you");

  useEffect(() => {
    const saved = typeof window !== "undefined" ? window.localStorage.getItem("veritas.identity") : null;
    if (saved) setCurrentState(saved);
    const load = () => api.identities().then(setIdentities).catch(() => {});
    load();
    const t = setInterval(load, 5000); // refresh reputations
    return () => clearInterval(t);
  }, []);

  const setCurrent = (id: string) => {
    setCurrentState(id);
    if (typeof window !== "undefined") window.localStorage.setItem("veritas.identity", id);
  };

  const me = identities.find((i) => i.id === current);
  return <Ctx.Provider value={{ identities, current, setCurrent, me }}>{children}</Ctx.Provider>;
}

export const useIdentity = () => useContext(Ctx);
