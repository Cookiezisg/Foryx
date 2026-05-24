// apiFetch — the only fetch wrapper. Resolves baseUrl from Wails bridge,
// adds JSON headers, strips the `data` envelope (§N1), normalises errors
// into thrown Error with `.code` / `.status` for TanStack to surface.
//
// apiFetch —— 唯一 fetch wrapper：从 Wails bridge 取 baseUrl，注 JSON 头，
// 剥 §N1 envelope，统一报错带 code/status。

import { apiUrl } from "../bridge/wails.js";
import { useSettings } from "../store/settings.js";

export class ApiError extends Error {
  constructor(message, { code = "UNKNOWN", status = 0, details } = {}) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.details = details;
  }
}

// activeUserHeader — reads settings.activeUserId and returns the
// X-Forgify-User-ID header pair. Returns {} when null; backend will then
// reject user-scoped routes with 401 / UNAUTH_NO_USER, triggering
// self-heal below.
//
// 读 settings.activeUserId 注 X-Forgify-User-ID；为空时用户路由会被
// 后端 401 拒，触发下方 self-heal。
function activeUserHeader() {
  const id = useSettings.getState().activeUserId;
  return id ? { "X-Forgify-User-ID": id } : {};
}

export async function apiFetch(path, { method = "GET", body, headers, signal, parseJSON = true } = {}) {
  const url = apiUrl("/api/v1" + path);

  const init = {
    method,
    headers: {
      Accept: "application/json",
      ...(body ? { "Content-Type": "application/json" } : {}),
      ...activeUserHeader(),
      ...(headers || {}),
    },
    signal,
  };
  if (body != null) {
    init.body = typeof body === "string" ? body : JSON.stringify(body);
  }

  let res;
  try {
    res = await fetch(url, init);
  } catch (err) {
    throw new ApiError(err.message || "network error", { code: "NETWORK", status: 0 });
  }

  if (res.status === 204) return null;

  if (!res.ok) {
    let payload = null;
    try { payload = await res.json(); } catch { /* swallow; we surface via message */ }
    const code = payload?.error?.code || `HTTP_${res.status}`;
    const message = payload?.error?.message || `request failed: ${res.status} ${res.statusText}`;
    // Self-heal: stale or missing activeUserId. Clear it so App.jsx's effect
    // re-renders into onboarding or auto-selects the only remaining user.
    // Still throw so the caller can surface the failure.
    //
    // 自愈：activeUserId 失效；清掉后 App.jsx 的 effect 会切回 onboarding
    // 或 auto-select。仍 throw 让调用方知道这次请求失败。
    if (res.status === 401 && code === "UNAUTH_NO_USER") {
      try { useSettings.getState().set({ activeUserId: null }); } catch { /* store unavailable in tests */ }
    }
    throw new ApiError(message, { code, status: res.status, details: payload?.error?.details });
  }

  if (!parseJSON) return res;

  const json = await res.json();
  // §N1 envelope: { data, nextCursor?, hasMore? }. Return the body unwrapped
  // but preserve pagination fields when present.
  if (json && typeof json === "object" && "data" in json) {
    if ("nextCursor" in json || "hasMore" in json) {
      return { items: json.data, nextCursor: json.nextCursor, hasMore: json.hasMore };
    }
    return json.data;
  }
  return json;
}

// Stable empty array — every hook's `select: pickList` must return the
// same reference when underlying data is missing, otherwise zustand /
// React.useSyncExternalStore consumers see a "new" value every render
// and infinite-loop. Freeze it for safety.
//
// 稳定空数组：每个 hook 的 select 在数据缺失时必须返回同一引用，否则
// useSyncExternalStore 消费者每渲染都看到 "新值" → 死循环。
export const EMPTY_ARRAY = Object.freeze([]);

export function pickList(d) {
  if (Array.isArray(d)) return d;
  if (d && Array.isArray(d.items)) return d.items;
  return EMPTY_ARRAY;
}

// query key factories — centralise query keys to avoid magic strings.
//
// query key 工厂；集中管理，避免散落 magic string。
export const qk = {
  health:           () => ["health"],
  users:            () => ["users"],
  conversations:    () => ["conversations"],
  conversation:     (id) => ["conv", id],
  messages:         (convId) => ["conv-messages", convId],
  apikeys:          () => ["api-keys"],
  providers:        () => ["providers"],
  modelConfigs:     () => ["model-configs"],
  functions:        () => ["functions"],
  function:         (id) => ["function", id],
  functionVersions: (id) => ["function-versions", id],
  handlers:         () => ["handlers"],
  handler:          (id) => ["handler", id],
  handlerVersions:  (id) => ["handler-versions", id],
  handlerConfig:    (id) => ["handler-config", id],
  workflows:        () => ["workflows"],
  workflow:         (id) => ["workflow", id],
  workflowVersions: (id) => ["workflow-versions", id],
  flowruns:         () => ["flowruns"],
  flowrun:          (id) => ["flowrun", id],
  flowrunNodes:     (id) => ["flowrun-nodes", id],
  skills:           () => ["skills"],
  skill:            (id) => ["skill", id],
  mcpServers:       () => ["mcp-servers"],
  memories:         (type) => ["memories", type || "all"],
  memory:           (name) => ["memory", name],
  documents:        () => ["documents"],
  document:         (id) => ["document", id],
  relations:        (entityId) => ["relations", entityId],
  notificationsSnap:() => ["notifications-snapshot"],
};
