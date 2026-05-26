// SSEProvider — mounts the 3 SSE hooks exactly once and exposes the
// combined health status via React context. Sidebar reads it through
// useSSEHealth().
//
// SSEProvider —— 单例挂载 3 个 SSE hook，合成状态点给 sidebar 用。

import React, { createContext, useContext, useMemo } from "react";
import { useEventLog } from "./useEventLog";
import { useNotifications } from "./useNotifications";
import { useForge } from "./useForge";

const Ctx = createContext(null);

export function SSEProvider({ children }: { children: React.ReactNode }) {
  const eventlog = useEventLog();
  const { status: notifs, unread, clearUnread } = useNotifications();
  const forge = useForge();

  const value = useMemo(() => ({
    eventlog, notifs, forge,
    unread, clearUnread,
    overall: deriveOverall(eventlog, notifs, forge),
  }), [eventlog, notifs, forge, unread, clearUnread]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSSEHealth() {
  const ctx = useContext(Ctx);
  if (!ctx) return { overall: "unknown", eventlog: "unknown", notifs: "unknown", forge: "unknown", unread: 0, clearUnread: () => {} };
  return ctx;
}

function deriveOverall(a: string, b: string, c: string) {
  const all = [a, b, c];
  if (all.every((s) => s === "connected")) return "ok";
  if (all.some((s) => s === "disconnected")) return "err";
  return "warn";
}
