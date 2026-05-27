import { useSessionStore } from "./sessionStore";
import { fetchUsers } from "../api/session";

// Module-level in-flight guard — deduplicates concurrent resolveSession calls
// (StrictMode double-invoke, multiple SSE disconnect events, etc.).
//
// 模块级在途去重：StrictMode 双调 / 多路 SSE 断连同时触发时复用同一 Promise。
let inflight: Promise<void> | null = null;

// resolveSession — identity resolution always based on a fresh /users fetch.
// Mirrors computeBootState's onboarding/valid logic (boot.js) but the data
// source is a direct network call, never a stale TanStack snapshot.
//
// 永远基于 fresh /users 解析身份；复刻 boot.js computeBootState 的判定；
// stale/null currentUserId → 选 users[0]，绝不从 stale 喂回循环。
export async function resolveSession(): Promise<void> {
  if (inflight) return inflight;
  inflight = _resolve().finally(() => { inflight = null; });
  return inflight;
}

async function _resolve(): Promise<void> {
  const s = useSessionStore.getState();
  s.setStatus("loading");
  const users = await fetchUsers();
  if (users.length === 0) {
    // Clear any stale userId so SSE channels don't try to connect with it.
    s.setCurrentUser(null);
    s.setStatus("onboarding");
    return;
  }
  const valid = !!s.currentUserId && users.some((u) => u.id === s.currentUserId);
  if (!valid) s.setCurrentUser(users[0].id);
  s.setStatus("ready");
}
