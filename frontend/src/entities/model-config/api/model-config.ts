import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, pickList, qk } from "@shared/api";
import type { ModelConfig, Provider, Scenario, UpsertModelConfigBody } from "../model/types";

export function useProviders() {
  return useQuery<Provider[]>({
    queryKey: qk.providers(),
    queryFn: () => apiFetch("/providers"),
  });
}

// useScenarios — backend's authoritative scenario whitelist. Replaces the
// old hardcoded fallback in ModelsTab that drifted from backend (3 of 5
// scenarios silently 400'd as INVALID_SCENARIO).
//
// 后端 scenario 白名单权威源;ModelsTab 旧硬编码 5 项里 3 项后端不支持,
// 改从这里取。
export function useScenarios() {
  return useQuery<Scenario[]>({
    queryKey: qk.scenarios(),
    queryFn: () => apiFetch("/scenarios"),
    select: pickList<Scenario>,
    staleTime: 5 * 60 * 1000,
  });
}

export function useModelConfigs() {
  return useQuery<ModelConfig[]>({
    queryKey: qk.modelConfigs(),
    queryFn: () => apiFetch("/model-configs"),
    select: pickList<ModelConfig>,
  });
}

export function useUpsertModelConfig() {
  const qc = useQueryClient();
  return useMutation<ModelConfig, Error, { scenario: string } & UpsertModelConfigBody>({
    mutationFn: ({ scenario, ...body }) =>
      apiFetch(`/model-configs/${scenario}`, { method: "PUT", body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.modelConfigs() }),
  });
}
