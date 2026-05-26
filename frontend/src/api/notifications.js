// Notifications REST snapshot — read recent notifications without subscribing
// to SSE. SSE delivery itself is in sse/useNotifications.js (Phase 4).
//
// 通知 REST 快照；SSE 推送在 sse/useNotifications.js（Phase 4）。

import { useQuery } from "@tanstack/react-query";
import { apiFetch, qk, pickList } from "./client.js";
import { useSessionStore } from "@entities/session";

export function useNotificationsSnapshot(limit = 50) {
  const uid = useSessionStore((s) => s.currentUserId);
  return useQuery({
    queryKey: qk.notificationsSnap(),
    queryFn: () => apiFetch(`/notifications?limit=${limit}`, { headers: { Accept: "application/json" } }),
    select: pickList,
    enabled: !!uid,
  });
}
