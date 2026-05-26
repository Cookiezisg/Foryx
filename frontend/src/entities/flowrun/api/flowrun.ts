import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, pickList, qk } from "@shared/api";
import { useSessionStore } from "../../session/@x/flowrun";
import type {
  FlowRun,
  FlowRunNode,
  FlowRunsParams,
  ApproveNodeVars,
  RejectNodeVars,
} from "../model/types";

export function useFlowRuns(params: FlowRunsParams = {}) {
  const uid = useSessionStore((s) => s.currentUserId);
  const merged = { limit: "100", ...params } as Record<string, string>;
  const qs = new URLSearchParams(merged).toString();
  return useQuery<FlowRun[]>({
    queryKey: [...qk.flowruns(), params],
    queryFn: () => apiFetch(`/flowruns?${qs}`),
    select: pickList<FlowRun>,
    enabled: !!uid,
  });
}

export function useFlowRun(id: string) {
  return useQuery<FlowRun>({
    queryKey: qk.flowrun(id),
    queryFn: () => apiFetch(`/flowruns/${id}`),
    enabled: !!id,
  });
}

export function useFlowRunNodes(id: string) {
  return useQuery<FlowRunNode[]>({
    queryKey: qk.flowrunNodes(id),
    queryFn: () => apiFetch(`/flowruns/${id}/nodes`),
    select: pickList<FlowRunNode>,
    enabled: !!id,
  });
}

// Backend: cancel = DELETE /flowruns/{id} (not POST :cancel).
// 后端 cancel 走 DELETE，不是 :cancel。
export function useCancelFlowRun() {
  const qc = useQueryClient();
  return useMutation<unknown, Error, string>({
    mutationFn: (id) => apiFetch(`/flowruns/${id}`, { method: "DELETE" }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.flowruns() });
      qc.invalidateQueries({ queryKey: qk.flowrun(id) });
    },
  });
}

// Backend: POST /flowruns/{id}/approvals/{nodeId} with {decision, reason}.
// decision: "approve" / "reject".
// 后端是 /approvals/{nodeId}（不是 /nodes/{nodeId}:approve），body 带 decision。
export function useApproveNode() {
  const qc = useQueryClient();
  return useMutation<unknown, Error, ApproveNodeVars>({
    mutationFn: ({ runId, nodeId, decision = "approve", reason = "" }) =>
      apiFetch(`/flowruns/${runId}/approvals/${nodeId}`, {
        method: "POST",
        body: { decision, reason },
      }),
    onSuccess: (_, { runId }) => {
      qc.invalidateQueries({ queryKey: qk.flowruns() });
      qc.invalidateQueries({ queryKey: qk.flowrun(runId) });
      qc.invalidateQueries({ queryKey: qk.flowrunNodes(runId) });
    },
  });
}

export function useRejectNode() {
  const qc = useQueryClient();
  return useMutation<unknown, Error, RejectNodeVars>({
    mutationFn: ({ runId, nodeId, reason = "" }) =>
      apiFetch(`/flowruns/${runId}/approvals/${nodeId}`, {
        method: "POST",
        body: { decision: "reject", reason },
      }),
    onSuccess: (_, { runId }) => {
      qc.invalidateQueries({ queryKey: qk.flowruns() });
      qc.invalidateQueries({ queryKey: qk.flowrun(runId) });
    },
  });
}

export function useTriageFlowRun() {
  return useMutation<unknown, Error, string>({
    mutationFn: (id) => apiFetch(`/flowruns/${id}:triage`, { method: "POST" }),
  });
}
