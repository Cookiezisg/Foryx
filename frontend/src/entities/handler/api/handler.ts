import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, pickList, qk } from "@shared/api";
import { useSessionStore } from "../../session/@x/handler";
import type { Handler, HandlerVersion, HandlerConfig, CallHandlerVars, CallHandlerResult } from "../model/types";

export function useHandlers() {
  const uid = useSessionStore((s) => s.currentUserId);
  return useQuery<Handler[]>({
    queryKey: qk.handlers(),
    queryFn: () => apiFetch("/handlers?limit=200"),
    select: pickList<Handler>,
    enabled: !!uid,
  });
}

export function useHandler(id: string) {
  return useQuery<Handler>({
    queryKey: qk.handler(id),
    queryFn: () => apiFetch(`/handlers/${id}`),
    enabled: !!id,
  });
}

export function useHandlerVersions(id: string) {
  return useQuery<HandlerVersion[]>({
    queryKey: qk.handlerVersions(id),
    queryFn: () => apiFetch(`/handlers/${id}/versions`),
    select: pickList<HandlerVersion>,
    enabled: !!id,
  });
}

export function useHandlerConfig(id: string) {
  return useQuery<HandlerConfig>({
    queryKey: qk.handlerConfig(id),
    queryFn: () => apiFetch(`/handlers/${id}/config`),
    enabled: !!id,
  });
}

// Backend accept/reject under /{kind}s/{id}/pending:accept (not the {idAction} dispatch).
//
// 后端 accept/reject 走 /{kind}s/{id}/pending:accept，与 :call/:revert 路径不同。
export function useAcceptHandler() {
  const qc = useQueryClient();
  return useMutation<Handler, Error, string>({
    mutationFn: (id) => apiFetch(`/handlers/${id}/pending:accept`, { method: "POST" }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.handlers() });
      qc.invalidateQueries({ queryKey: qk.handler(id) });
      qc.invalidateQueries({ queryKey: qk.handlerVersions(id) });
    },
  });
}

export function useRejectHandler() {
  const qc = useQueryClient();
  return useMutation<null, Error, string>({
    mutationFn: (id) => apiFetch(`/handlers/${id}/pending:reject`, { method: "POST" }),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.handlers() });
      qc.invalidateQueries({ queryKey: qk.handler(id) });
      qc.invalidateQueries({ queryKey: qk.handlerVersions(id) });
    },
  });
}

export function useCallHandler() {
  return useMutation<CallHandlerResult, Error, CallHandlerVars>({
    mutationFn: ({ id, method, args }) =>
      apiFetch(`/handlers/${id}:call`, { method: "POST", body: { method, args } }),
  });
}

export function useDeleteHandler() {
  const qc = useQueryClient();
  return useMutation<null, Error, string>({
    mutationFn: (id) => apiFetch(`/handlers/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.handlers() }),
  });
}
