// Shared SSE connection factory. EventSource ships its own auto-reconnect
// and replays the last `id:` value via Last-Event-ID — matches backend
// /eventlog, /notifications, /forge handlers.
//
// State machine (mirrors spec §4.5):
//   - activeUserId null → no connection (would 401 instantly; pointless)
//   - activeUserId set  → connect with ?userID=<id> (EventSource can't
//     send custom headers, so the SSE auth path reads the query)
//   - connection drops permanently while the captured uid still matches
//     the current activeUserId → self-heal: clear activeUserId so App.jsx
//     re-renders into onboarding / picker
//   - activeUserId changes mid-stream → the calling hook (useEventLog
//     etc.) keys its useEffect on activeUserId and rebuilds via close
//     + new createSSE
//
// 共享 SSE 工厂；activeUserId 为空时不建连接；连接被永久关闭且 captured
// uid 还等于当前 activeUserId 时清掉它触发 self-heal。账号切换由 hook 的
// useEffect 重建。

import { apiUrl } from "../../bridge/wails.js";
// TODO(阶段4): settings 下沉 shared 或由 app 注入 lang 后移除此豁免
// eslint-disable-next-line boundaries/dependencies
import { useSettings } from "../../store/settings.js";

export type SSEEventMeta = { seq: number; raw: string };

export type SSEEventHandler = (payload: unknown, meta: SSEEventMeta) => void;

export interface CreateSSEOpts {
  path: string;
  eventHandlers: Record<string, SSEEventHandler>;
  onStatus?: (status: "connecting" | "connected" | "disconnected") => void;
}

export interface SSEController {
  close(): void;
}

const NOOP_CONTROLLER: SSEController = { close: () => {} };

export function createSSE({ path, eventHandlers, onStatus }: CreateSSEOpts): SSEController {
  const uid = useSettings.getState().activeUserId;

  // Idle state: no user, no connection.
  if (!uid) {
    if (onStatus) onStatus("disconnected");
    return NOOP_CONTROLLER;
  }

  const base = apiUrl("/api/v1" + path);
  const url = `${base}${base.includes("?") ? "&" : "?"}userID=${encodeURIComponent(uid)}`;

  const es = new EventSource(url);

  if (onStatus) onStatus("connecting");
  es.addEventListener("open", () => onStatus?.("connected"));
  es.addEventListener("error", () => {
    // readyState 0 = CONNECTING (about to retry), 2 = CLOSED (terminal).
    if (es.readyState !== EventSource.CLOSED) {
      onStatus?.("connecting");
      return;
    }
    onStatus?.("disconnected");
    // Self-heal: connection closed permanently. If our captured uid still
    // equals the current store value, the backend rejected (likely 401 on
    // a stale id) → clear so App.jsx flips into onboarding. If the store
    // already moved on (account switch / REST 401 cleared first), do
    // nothing — the hook's useEffect will rebuild.
    //
    // 自愈：连接被永久关闭。captured uid 仍 = store 当前值时清掉。
    const current = useSettings.getState().activeUserId;
    if (current === uid) {
      try { useSettings.getState().set({ activeUserId: null }); } catch { /* store unavailable in tests */ }
    }
  });

  for (const [evt, handler] of Object.entries(eventHandlers)) {
    es.addEventListener(evt, (e: MessageEvent) => {
      let payload: unknown = null;
      try { payload = JSON.parse(e.data); } catch { /* fall through */ }
      try { handler(payload, { seq: parseInt(e.lastEventId || "0", 10), raw: e.data }); }
      catch (err) { console.error(`[SSE ${path}] handler ${evt} threw`, err); }
    });
  }

  return { close: () => es.close() };
}
