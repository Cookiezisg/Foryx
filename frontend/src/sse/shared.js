// Shared SSE connection factory. EventSource ships its own auto-reconnect
// and automatically replays the last `id:` value via the Last-Event-ID
// header — which is exactly what the backend's /eventlog, /notifications,
// and /forge handlers honour. We don't manually close on transient
// errors; we only close if the caller does (component unmount).
//
// Multi-user: EventSource cannot send custom headers, so we append
// ?userID=<id> to the URL (backend auth middleware reads it as
// fallback for SSE clients — see backend/internal/transport/httpapi/
// middleware/auth.go).
//
// 共享 SSE 连接工厂；多账号靠 ?userID= 兜底（EventSource 不能自定义 header）。

import { apiUrl } from "../bridge/wails.js";
import { useSettings } from "../store/settings.js";

export function createSSE({ path, eventHandlers, onStatus }) {
  const base = apiUrl("/api/v1" + path);
  const uid = useSettings.getState().activeUserId;
  const url = uid ? `${base}${base.includes("?") ? "&" : "?"}userID=${encodeURIComponent(uid)}` : base;

  const es = new EventSource(url);

  if (onStatus) {
    onStatus("connecting");
    es.addEventListener("open", () => onStatus("connected"));
    es.addEventListener("error", () => {
      // readyState 0 = CONNECTING (about to retry), 2 = CLOSED (terminal).
      onStatus(es.readyState === EventSource.CLOSED ? "disconnected" : "connecting");
    });
  }

  for (const [evt, handler] of Object.entries(eventHandlers)) {
    es.addEventListener(evt, (e) => {
      let payload = null;
      try { payload = JSON.parse(e.data); } catch { /* fall through */ }
      try { handler(payload, { seq: parseInt(e.lastEventId || "0", 10), raw: e.data }); }
      catch (err) { console.error(`[SSE ${path}] handler ${evt} threw`, err); }
    });
  }

  return { close: () => es.close() };
}
