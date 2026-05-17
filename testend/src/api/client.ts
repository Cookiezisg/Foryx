/**
 * HTTP API client — single source of truth for backend calls.
 *
 * - Unwraps the {data}/{error} envelope; throws `ApiError` on 4xx/5xx.
 * - Pagination helpers (cursor-based).
 * - Used by every domain module (conversations.ts, functions.ts, ...).
 */

import type { Envelope, ApiError, Page } from '@/types/api';

/** Error thrown for non-2xx responses; carries the envelope's `error` field. */
export class HttpError extends Error {
  status: number;
  code: string;
  details?: unknown;

  constructor(status: number, error: ApiError) {
    super(error.message || `HTTP ${status}`);
    this.status = status;
    this.code = error.code;
    this.details = error.details;
  }
}

/** §multi-user: client stamps the active user's ID into X-Forgify-User-ID on every request. */
const ACTIVE_USER_STORAGE_KEY = 'forgify:active-user';
const LOCALE_STORAGE_KEY = 'forgify:locale';

export function getActiveUserID(): string | null {
  try {
    return localStorage.getItem(ACTIVE_USER_STORAGE_KEY);
  } catch {
    return null;
  }
}

export function setActiveUserID(uid: string | null) {
  try {
    if (uid) localStorage.setItem(ACTIVE_USER_STORAGE_KEY, uid);
    else localStorage.removeItem(ACTIVE_USER_STORAGE_KEY);
  } catch {
    /* ignore */
  }
}

/** §i18n: client sends Accept-Language based on persisted locale (synced with vue-i18n). */
function getActiveLocale(): string | null {
  try {
    return localStorage.getItem(LOCALE_STORAGE_KEY);
  } catch {
    return null;
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  options: { headers?: Record<string, string> } = {},
): Promise<Envelope<T>> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...options.headers,
  };
  const uid = getActiveUserID();
  if (uid && !headers['X-Forgify-User-ID']) {
    headers['X-Forgify-User-ID'] = uid;
  }
  const locale = getActiveLocale();
  if (locale && !headers['Accept-Language']) {
    headers['Accept-Language'] = locale;
  }
  let payload: BodyInit | undefined;
  if (body !== undefined && body !== null) {
    if (body instanceof FormData) {
      payload = body;
    } else {
      headers['Content-Type'] = 'application/json';
      payload = JSON.stringify(body);
    }
  }

  const res = await fetch(path, { method, headers, body: payload });
  if (res.status === 204) {
    return {} as Envelope<T>;
  }

  let env: Envelope<T>;
  const text = await res.text();
  try {
    env = text ? (JSON.parse(text) as Envelope<T>) : {};
  } catch {
    // Non-JSON body (e.g. dev/* SQL plain text errors). Wrap as ApiError.
    throw new HttpError(res.status, {
      code: `HTTP_${res.status}`,
      message: text || res.statusText,
    });
  }
  if (!res.ok) {
    throw new HttpError(res.status, env.error ?? { code: `HTTP_${res.status}`, message: text });
  }
  return env;
}

export async function getJSON<T>(path: string): Promise<T> {
  const env = await request<T>('GET', path);
  return env.data as T;
}

export async function postJSON<T, B = unknown>(path: string, body?: B): Promise<T> {
  const env = await request<T>('POST', path, body);
  return env.data as T;
}

export async function patchJSON<T, B = unknown>(path: string, body?: B): Promise<T> {
  const env = await request<T>('PATCH', path, body);
  return env.data as T;
}

export async function putJSON<T, B = unknown>(path: string, body?: B): Promise<T> {
  const env = await request<T>('PUT', path, body);
  return env.data as T;
}

export async function deleteEmpty(path: string): Promise<void> {
  await request<void>('DELETE', path);
}

/** DELETE that returns a body (e.g. document soft-delete returns `{id, deletedCount}`). */
export async function deleteJSON<T>(path: string): Promise<T> {
  const env = await request<T>('DELETE', path);
  return env.data as T;
}

/**
 * Cursor-paginated GET. The backend returns `{data: [], nextCursor, hasMore}`.
 *
 * @param path  e.g. `/api/v1/conversations`
 * @param query optional querystring params (object)
 */
export async function getPage<T>(
  path: string,
  query: Record<string, string | number | undefined> = {},
): Promise<Page<T>> {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== '') qs.append(k, String(v));
  }
  const url = qs.toString() ? `${path}?${qs.toString()}` : path;
  const env = await request<T[]>('GET', url);
  return {
    items: (env.data as T[]) ?? [],
    nextCursor: env.nextCursor ?? '',
    hasMore: env.hasMore ?? false,
  };
}

/**
 * Non-envelope JSON fetch — used for `/dev/*` endpoints that don't follow
 * the `{data}` envelope convention.
 */
export async function getRaw<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) {
    const text = await res.text();
    throw new HttpError(res.status, { code: `HTTP_${res.status}`, message: text });
  }
  return (await res.json()) as T;
}

/** POST a raw JSON body and parse a raw JSON response (no envelope). */
export async function postRaw<T, B = unknown>(path: string, body: B): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new HttpError(res.status, { code: `HTTP_${res.status}`, message: text });
  }
  return (await res.json()) as T;
}
