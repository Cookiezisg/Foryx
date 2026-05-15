import { deleteEmpty, getJSON, getPage, postJSON } from './client';
import type { FlowRun, FlowRunNode } from '@/types/domain';

export const flowrunAPI = {
  list: (query: { workflowId?: string; status?: string; triggerKind?: string; limit?: number; cursor?: string } = {}) =>
    getPage<FlowRun>('/api/v1/flowruns', query),

  get: (id: string) => getJSON<FlowRun>(`/api/v1/flowruns/${id}`),

  nodes: (id: string, query: { nodeType?: string; status?: string; limit?: number; cursor?: string } = {}) =>
    getPage<FlowRunNode>(`/api/v1/flowruns/${id}/nodes`, query),

  cancel: (id: string) => deleteEmpty(`/api/v1/flowruns/${id}`),

  approve: (id: string, nodeId: string, decision: 'approved' | 'rejected', reason?: string) =>
    postJSON<{ resumed: boolean }>(`/api/v1/flowruns/${id}/approvals/${nodeId}`, {
      decision,
      reason,
    }),
};
