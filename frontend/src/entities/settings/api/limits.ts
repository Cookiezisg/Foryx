import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, qk } from "@shared/api";
import type { Limits } from "../model/limits";

// useLimits reads the live operational limits (settings.json "limits" overlaid
// on high-ceiling defaults).
//
// useLimits 读活动运行上限（settings.json "limits" 叠加高 ceiling 默认）。
export function useLimits() {
  return useQuery<Limits>({
    queryKey: qk.settingsLimits(),
    queryFn: () => apiFetch("/settings/limits"),
  });
}

// useUpdateLimits upserts the whole limits block; backend read-modify-writes so
// permissions/hooks survive. Response is the new live limits.
//
// useUpdateLimits upsert 整个 limits 块；后端 read-modify-write 保 permissions/hooks。
export function useUpdateLimits() {
  const qc = useQueryClient();
  return useMutation<Limits, Error, Limits>({
    mutationFn: (body) => apiFetch("/settings/limits", { method: "PUT", body }),
    onSuccess: (data) => qc.setQueryData(qk.settingsLimits(), data),
  });
}
