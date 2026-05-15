import { deleteEmpty, getJSON, getPage, patchJSON, postJSON } from './client';
import type { Handler, HandlerVersion, ExecutionRow, InitArgSpec, MethodSpec } from '@/types/domain';

export interface HandlerCreate {
  name: string;
  description?: string;
  tags?: string[];
  imports?: string;
  initBody?: string;
  shutdownBody?: string;
  methods: MethodSpec[];
  initArgsSchema?: InitArgSpec[];
  dependencies?: string[];
  pythonVersion?: string;
  changeReason?: string;
}

export const hdAPI = {
  list: (limit = 200, cursor?: string) =>
    getPage<Handler>('/api/v1/handlers', { limit, cursor }),

  get: (id: string) => getJSON<Handler>(`/api/v1/handlers/${id}`),

  create: (in_: HandlerCreate) =>
    postJSON<{ handler: Handler; version: HandlerVersion }>('/api/v1/handlers', in_),

  updateMeta: (id: string, patch: { name?: string; description?: string; tags?: string[] }) =>
    patchJSON<Handler>(`/api/v1/handlers/${id}`, patch),

  remove: (id: string) => deleteEmpty(`/api/v1/handlers/${id}`),

  call: (id: string, method: string, args: Record<string, unknown>) =>
    postJSON<{ result: unknown }>(`/api/v1/handlers/${id}:call`, { method, args }),

  revert: (id: string, targetVersion: number) =>
    postJSON<HandlerVersion>(`/api/v1/handlers/${id}:revert`, { targetVersion }),

  versions: (id: string, status?: string) =>
    getPage<HandlerVersion>(`/api/v1/handlers/${id}/versions`, { status, limit: 200 }),

  getVersion: (id: string, v: string | number) =>
    getJSON<HandlerVersion>(`/api/v1/handlers/${id}/versions/${v}`),

  pending: (id: string) => getJSON<HandlerVersion>(`/api/v1/handlers/${id}/pending`),
  acceptPending: (id: string) => postJSON<HandlerVersion>(`/api/v1/handlers/${id}/pending:accept`),
  rejectPending: (id: string) => postJSON<void>(`/api/v1/handlers/${id}/pending:reject`),

  getConfig: (id: string) =>
    getJSON<{ configState: string; config: Record<string, unknown> }>(`/api/v1/handlers/${id}/config`),

  updateConfig: (id: string, config: Record<string, unknown>) =>
    postJSON<{ updated: boolean; configState: string }>(`/api/v1/handlers/${id}/config`, { config }),

  clearConfig: (id: string) => deleteEmpty(`/api/v1/handlers/${id}/config`),

  calls: (id: string, query: Record<string, string | number | undefined> = {}) =>
    getJSON<{
      calls: ExecutionRow[];
      count: number;
      hasMore: boolean;
      nextCursor: string;
      aggregates?: { okCount: number; failedCount: number; cancelledCount: number; timeoutCount: number; avgElapsedMs: number; p95ElapsedMs: number };
    }>(`/api/v1/handlers/${id}/calls${queryString(query)}`),

  getCall: (callId: string) =>
    getJSON<ExecutionRow & { hints?: Record<string, unknown> }>(`/api/v1/handler-calls/${callId}`),
};

function queryString(q: Record<string, string | number | undefined>): string {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) if (v !== undefined && v !== '') params.append(k, String(v));
  const s = params.toString();
  return s ? `?${s}` : '';
}
