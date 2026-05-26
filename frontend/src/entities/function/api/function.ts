import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, pickList, qk } from "@shared/api";
import { useSessionStore } from "../../session/@x/function";
import type { FunctionEntity, FunctionVersion, RunFunctionVars, RunFunctionResult } from "../model/types";

export function useFunctions() {
  const uid = useSessionStore((s) => s.currentUserId);
  return useQuery<FunctionEntity[]>({
    queryKey: qk.functions(),
    queryFn: () => apiFetch("/functions?limit=200"),
    select: pickList<FunctionEntity>,
    enabled: !!uid,
  });
}

export function useFunction(id: string) {
  return useQuery<FunctionEntity>({
    queryKey: qk.function(id),
    queryFn: () => apiFetch(`/functions/${id}`),
    enabled: !!id,
  });
}

export function useFunctionVersions(id: string) {
  return useQuery<FunctionVersion[]>({
    queryKey: qk.functionVersions(id),
    queryFn: () => apiFetch(`/functions/${id}/versions`),
    select: pickList<FunctionVersion>,
    enabled: !!id,
  });
}

// Backend routes pending accept/reject under /{kind}s/{id}/pending:accept
// (not the {idAction} dispatch). Revert lives on the {idAction} switch.
//
// 后端 accept/reject 走 /{kind}s/{id}/pending:accept，与 :revert 路径不同。
export function useAcceptFunction() {
  const qc = useQueryClient();
  return useMutation<FunctionEntity, Error, string>({
    mutationFn: (id) => apiFetch(`/functions/${id}/pending:accept`, { method: "POST" }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.functions() });
      qc.invalidateQueries({ queryKey: qk.function(id) });
      qc.invalidateQueries({ queryKey: qk.functionVersions(id) });
    },
  });
}

export function useRejectFunction() {
  const qc = useQueryClient();
  return useMutation<FunctionEntity, Error, string>({
    mutationFn: (id) => apiFetch(`/functions/${id}/pending:reject`, { method: "POST" }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.functions() });
      qc.invalidateQueries({ queryKey: qk.function(id) });
      qc.invalidateQueries({ queryKey: qk.functionVersions(id) });
    },
  });
}

export function useRevertFunction() {
  const qc = useQueryClient();
  return useMutation<FunctionEntity, Error, string>({
    mutationFn: (id) => apiFetch(`/functions/${id}:revert`, { method: "POST" }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.functions() });
      qc.invalidateQueries({ queryKey: qk.function(id) });
    },
  });
}

export function useRunFunction() {
  return useMutation<RunFunctionResult, Error, RunFunctionVars>({
    mutationFn: ({ id, inputs }) =>
      apiFetch(`/functions/${id}:run`, { method: "POST", body: { inputs } }),
  });
}

export function useDeleteFunction() {
  const qc = useQueryClient();
  return useMutation<null, Error, string>({
    mutationFn: (id) => apiFetch(`/functions/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.functions() }),
  });
}
