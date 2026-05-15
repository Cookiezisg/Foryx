import { deleteEmpty, getJSON, getPage, patchJSON, postJSON } from './client';
import type { Function as Fn, FunctionVersion, ExecutionRow } from '@/types/domain';

export interface FunctionCreate {
  name: string;
  description?: string;
  code: string;
  tags?: string[];
  parameters?: { name: string; type: string; required?: boolean; default?: unknown; description?: string }[];
  returnSchema?: Record<string, unknown>;
  dependencies?: string[];
  pythonVersion?: string;
  changeReason?: string;
}

export const fnAPI = {
  list: (limit = 200, cursor?: string) =>
    getPage<Fn>('/api/v1/functions', { limit, cursor }),

  get: (id: string) => getJSON<Fn>(`/api/v1/functions/${id}`),

  create: (in_: FunctionCreate) =>
    postJSON<{ function: Fn; version: FunctionVersion }>('/api/v1/functions', in_),

  updateMeta: (id: string, patch: { name?: string; description?: string; tags?: string[] }) =>
    patchJSON<Fn>(`/api/v1/functions/${id}`, patch),

  remove: (id: string) => deleteEmpty(`/api/v1/functions/${id}`),

  run: (id: string, args: Record<string, unknown>, version?: string) =>
    postJSON<{ result: unknown; ok: boolean; output?: unknown; errorMsg?: string; elapsedMs?: number }>(
      `/api/v1/functions/${id}:run`,
      { args, version },
    ),

  revert: (id: string, targetVersion: number) =>
    postJSON<FunctionVersion>(`/api/v1/functions/${id}:revert`, { targetVersion }),

  versions: (id: string, status?: string) =>
    getPage<FunctionVersion>(`/api/v1/functions/${id}/versions`, { status, limit: 200 }),

  getVersion: (id: string, v: string | number) =>
    getJSON<FunctionVersion>(`/api/v1/functions/${id}/versions/${v}`),

  pending: (id: string) => getJSON<FunctionVersion>(`/api/v1/functions/${id}/pending`),
  acceptPending: (id: string) => postJSON<FunctionVersion>(`/api/v1/functions/${id}/pending:accept`),
  /** Reject is POST (not DELETE) per backend mux. */
  rejectPending: (id: string) => postJSON<void>(`/api/v1/functions/${id}/pending:reject`),

  executions: (id: string, query: Record<string, string | number | undefined> = {}) =>
    getJSON<{
      executions: ExecutionRow[];
      count: number;
      hasMore: boolean;
      nextCursor: string;
      aggregates?: { okCount: number; failedCount: number; cancelledCount: number; timeoutCount: number; avgElapsedMs: number; p95ElapsedMs: number };
    }>(`/api/v1/functions/${id}/executions${queryString(query)}`),

  getExecution: (execId: string) =>
    getJSON<ExecutionRow & { hints?: Record<string, unknown> }>(`/api/v1/function-executions/${execId}`),
};

function queryString(q: Record<string, string | number | undefined>): string {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (v !== undefined && v !== '') params.append(k, String(v));
  }
  const s = params.toString();
  return s ? `?${s}` : '';
}
