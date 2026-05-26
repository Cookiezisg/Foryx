import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, pickList, qk } from "@shared/api";
import { useSessionStore } from "../../session/@x/workflow";
import type {
  Workflow,
  WorkflowVersion,
  EditWorkflowVars,
  RunWorkflowVars,
  CapabilityCheckResult,
} from "../model/types";

export function useWorkflows() {
  const uid = useSessionStore((s) => s.currentUserId);
  return useQuery<Workflow[]>({
    queryKey: qk.workflows(),
    queryFn: () => apiFetch("/workflows?limit=200"),
    select: pickList<Workflow>,
    enabled: !!uid,
  });
}

export function useWorkflow(id: string) {
  return useQuery<Workflow>({
    queryKey: qk.workflow(id),
    queryFn: () => apiFetch(`/workflows/${id}`),
    enabled: !!id,
  });
}

export function useWorkflowVersions(id: string) {
  return useQuery<WorkflowVersion[]>({
    queryKey: qk.workflowVersions(id),
    queryFn: () => apiFetch(`/workflows/${id}/versions`),
    select: pickList<WorkflowVersion>,
    enabled: !!id,
  });
}

export function useAcceptWorkflow() {
  const qc = useQueryClient();
  return useMutation<Workflow, Error, string>({
    mutationFn: (id) => apiFetch(`/workflows/${id}/pending:accept`, { method: "POST" }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.workflows() });
      qc.invalidateQueries({ queryKey: qk.workflow(id) });
      qc.invalidateQueries({ queryKey: qk.workflowVersions(id) });
    },
  });
}

export function useRejectWorkflow() {
  const qc = useQueryClient();
  return useMutation<null, Error, string>({
    mutationFn: (id) => apiFetch(`/workflows/${id}/pending:reject`, { method: "POST" }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.workflows() });
      qc.invalidateQueries({ queryKey: qk.workflow(id) });
      qc.invalidateQueries({ queryKey: qk.workflowVersions(id) });
    },
  });
}

export function useDeleteWorkflow() {
  const qc = useQueryClient();
  return useMutation<null, Error, string>({
    mutationFn: (id) => apiFetch(`/workflows/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.workflows() }),
  });
}

export function useUpdateWorkflow(id: string) {
  const qc = useQueryClient();
  return useMutation<Workflow, Error, Partial<Workflow>>({
    mutationFn: (patch) =>
      apiFetch(`/workflows/${id}`, { method: "PATCH", body: patch }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.workflow(id) });
      qc.invalidateQueries({ queryKey: qk.workflowVersions(id) });
    },
  });
}

// Manual workflow trigger (= scheduler StartRun with kind=manual).
// Backend: POST /workflows/{id}:trigger.
export function useRunWorkflow() {
  return useMutation<unknown, Error, RunWorkflowVars>({
    mutationFn: ({ id, input }) =>
      apiFetch(`/workflows/${id}:trigger`, { method: "POST", body: { input: input || {} } }),
  });
}

// Apply edit ops (creates/iterates pending version). Used by WorkflowEditor
// autosave: diff vs original → ops array → POST :edit.
//
// 把 ops 应用到当前 workflow，产/迭代 pending；编辑器 autosave 用。
export function useEditWorkflow(id: string) {
  const qc = useQueryClient();
  return useMutation<Workflow, Error, EditWorkflowVars>({
    mutationFn: ({ ops, changeReason }) =>
      apiFetch(`/workflows/${id}:edit`, {
        method: "POST",
        body: { ops, changeReason: changeReason || "manual edit" },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.workflow(id) });
      qc.invalidateQueries({ queryKey: qk.workflowVersions(id) });
    },
  });
}

// Capability check: POST /workflows/{id}:capability-check.
export function useCapabilityCheck() {
  return useMutation<CapabilityCheckResult, Error, string>({
    mutationFn: (id) =>
      apiFetch(`/workflows/${id}:capability-check`, { method: "POST" }),
  });
}
