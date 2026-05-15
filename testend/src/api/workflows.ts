import { deleteEmpty, getJSON, getPage, patchJSON, postJSON } from './client';
import type { Workflow, WorkflowVersion, TriggerState } from '@/types/domain';

/** Op for workflow Create/Edit — opaque JSON shapes per backend's ParseOps. */
export type WorkflowOp = Record<string, unknown> & { op: string };

export const wfAPI = {
  list: (limit = 200, cursor?: string, enabledOnly = false) =>
    getPage<Workflow>('/api/v1/workflows', { limit, cursor, enabled: enabledOnly ? 'true' : undefined }),

  get: (id: string) => getJSON<Workflow>(`/api/v1/workflows/${id}`),

  create: (ops: WorkflowOp[], changeReason?: string) =>
    postJSON<{ workflow: Workflow; version: WorkflowVersion }>('/api/v1/workflows', {
      ops,
      changeReason,
    }),

  updateMeta: (
    id: string,
    patch: {
      name?: string;
      description?: string;
      tags?: string[];
      enabled?: boolean;
      concurrency?: string;
      needsAttention?: boolean;
      attentionReason?: string;
    },
  ) => patchJSON<Workflow>(`/api/v1/workflows/${id}`, patch),

  remove: (id: string) => deleteEmpty(`/api/v1/workflows/${id}`),

  revert: (id: string, targetVersion: number) =>
    postJSON<WorkflowVersion>(`/api/v1/workflows/${id}:revert`, { targetVersion }),

  trigger: (id: string, input: Record<string, unknown> = {}) =>
    postJSON<{ runId: string }>(`/api/v1/workflows/${id}:trigger`, { input }),

  versions: (id: string, status?: string) =>
    getPage<WorkflowVersion>(`/api/v1/workflows/${id}/versions`, { status, limit: 200 }),

  getVersion: (id: string, v: string | number) =>
    getJSON<WorkflowVersion>(`/api/v1/workflows/${id}/versions/${v}`),

  pending: (id: string) => getJSON<WorkflowVersion>(`/api/v1/workflows/${id}/pending`),
  acceptPending: (id: string) => postJSON<WorkflowVersion>(`/api/v1/workflows/${id}/pending:accept`),
  rejectPending: (id: string) => postJSON<void>(`/api/v1/workflows/${id}/pending:reject`),

  triggerStates: (id: string) => getJSON<TriggerState[]>(`/api/v1/workflows/${id}/triggers`),
};
