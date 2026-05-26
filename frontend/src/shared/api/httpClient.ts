// apiFetch — the only fetch wrapper. Resolves baseUrl from Wails bridge,
// adds JSON headers, strips the `data` envelope (§N1), normalises errors
// into thrown ApiError with .code / .status for TanStack to surface.
//
// apiFetch —— 唯一 fetch wrapper：从 Wails bridge 取 baseUrl，注 JSON 头，
// 剥 §N1 envelope，统一报错带 code/status。

import { apiUrl } from "../../bridge/wails.js";
import { getUserId, notifyAuthFailure } from "./authProvider.js";

export { setUserIdProvider, setOnAuthFailure } from "./authProvider.js";

export class ApiError extends Error {
  code: string;
  status: number;
  details?: unknown;

  constructor(
    message: string,
    opts: { code?: string; status?: number; details?: unknown } = {},
  ) {
    super(message);
    this.name = "ApiError";
    this.code = opts.code ?? "UNKNOWN";
    this.status = opts.status ?? 0;
    this.details = opts.details;
  }
}

// §N1 envelope: { data, nextCursor?, hasMore? }
export type Envelope<T> = { data: T; nextCursor?: string | null; hasMore?: boolean };

export type ListResult<T> = { items: T[]; nextCursor?: string | null; hasMore?: boolean };

export interface ApiFetchOpts {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
  signal?: AbortSignal;
  parseJSON?: boolean;
}

// activeUserHeader — reads the injected provider and returns the
// X-Forgify-User-ID header pair. Returns {} when null; backend will then
// reject user-scoped routes with 401 / UNAUTH_NO_USER, triggering
// self-heal below.
//
// 读注入 provider 取 userId 注头；为空时后端 401，触发下方 self-heal。
function activeUserHeader(): Record<string, string> {
  const id = getUserId();
  return id ? { "X-Forgify-User-ID": id } : {};
}

export async function apiFetch<T = unknown>(path: string, opts: ApiFetchOpts = {}): Promise<T> {
  const { method = "GET", body, headers, signal, parseJSON = true } = opts;
  const url = apiUrl("/api/v1" + path);

  const init: RequestInit = {
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

  let res: Response;
  try {
    res = await fetch(url, init);
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "network error";
    throw new ApiError(msg, { code: "NETWORK", status: 0 });
  }

  if (res.status === 204) return null as T;

  if (!res.ok) {
    let payload: { error?: { code?: string; message?: string; details?: unknown } } | null = null;
    try { payload = await res.json(); } catch { /* swallow; we surface via message */ }
    const code = payload?.error?.code || `HTTP_${res.status}`;
    const message = payload?.error?.message || `request failed: ${res.status} ${res.statusText}`;
    // Self-heal: stale or missing userId — notify session to re-resolve.
    // resolveSession picks a fresh /users list and updates currentUserId.
    // Still throw so the caller can surface the failure.
    //
    // 自愈：userId 失效 → 通知 session 重新 resolve；仍 throw 让调用方感知失败。
    if (res.status === 401 && code === "UNAUTH_NO_USER") {
      notifyAuthFailure();
    }
    throw new ApiError(message, { code, status: res.status, details: payload?.error?.details });
  }

  if (!parseJSON) return res as unknown as T;

  const json: unknown = await res.json();
  // §N1 envelope: { data, nextCursor?, hasMore? }. Return the body unwrapped
  // but preserve pagination fields when present.
  if (json && typeof json === "object" && "data" in json) {
    const envelope = json as Record<string, unknown>;
    if ("nextCursor" in envelope || "hasMore" in envelope) {
      return { items: envelope["data"], nextCursor: envelope["nextCursor"], hasMore: envelope["hasMore"] } as T;
    }
    return envelope["data"] as T;
  }
  return json as T;
}

// Stable empty array — every hook's `select: pickList` must return the
// same reference when underlying data is missing, otherwise zustand /
// React.useSyncExternalStore consumers see a "new" value every render
// and infinite-loop. Freeze it for safety.
//
// 稳定空数组：每个 hook 的 select 在数据缺失时必须返回同一引用，否则
// useSyncExternalStore 消费者每渲染都看到 "新值" → 死循环。
export const EMPTY_ARRAY: readonly never[] = Object.freeze([] as never[]);

export function pickList<T>(d: unknown): T[] {
  if (Array.isArray(d)) return d as T[];
  if (d && typeof d === "object" && Array.isArray((d as Record<string, unknown>)["items"])) {
    return (d as Record<string, unknown>)["items"] as T[];
  }
  return EMPTY_ARRAY as unknown as T[];
}
