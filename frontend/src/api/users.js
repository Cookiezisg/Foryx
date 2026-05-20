// User (local profile) CRUD hooks. Backend identifies the active user
// via X-Forgify-User-ID header (or ?userID= for SSE); switching the
// active user in the UI is a settings.set({ activeUserId }) call +
// global queryClient.invalidateQueries() so all per-user data refreshes.
//
// 本地 profile CRUD；切换 user = 改 settings.activeUserId + 全量 invalidate。

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, qk, pickList } from "./client.js";

export function useUsers() {
  return useQuery({
    queryKey: qk.users(),
    queryFn: () => apiFetch("/users"),
    select: pickList,
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body) => apiFetch("/users", { method: "POST", body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.users() }),
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }) =>
      apiFetch(`/users/${id}`, { method: "PATCH", body: patch }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.users() }),
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id) => apiFetch(`/users/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.users() }),
  });
}
