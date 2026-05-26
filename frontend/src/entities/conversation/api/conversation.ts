import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, pickList, qk } from "@shared/api";
import { useSessionStore } from "../../session/@x/conversation";
import type {
  Conversation,
  Message,
  CreateConversationBody,
  UpdateConversationPatch,
  SendMessageBody,
} from "../model/types";

export function useConversations() {
  const uid = useSessionStore((s) => s.currentUserId);
  return useQuery<Conversation[]>({
    queryKey: qk.conversations(),
    queryFn: () => apiFetch("/conversations?limit=100"),
    select: pickList<Conversation>,
    enabled: !!uid,
  });
}

export function useConversation(id: string) {
  return useQuery<Conversation>({
    queryKey: qk.conversation(id),
    queryFn: () => apiFetch(`/conversations/${id}`),
    enabled: !!id,
  });
}

export function useConversationMessages(convId: string) {
  return useQuery<Message[]>({
    queryKey: qk.messages(convId),
    queryFn: () => apiFetch(`/conversations/${convId}/messages?limit=200`),
    select: pickList<Message>,
    enabled: !!convId,
  });
}

export function useCreateConversation() {
  const qc = useQueryClient();
  return useMutation<Conversation, Error, CreateConversationBody | undefined>({
    mutationFn: (body) =>
      apiFetch("/conversations", { method: "POST", body: body || {} }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.conversations() }),
  });
}

export function useUpdateConversation(id: string) {
  const qc = useQueryClient();
  return useMutation<Conversation, Error, UpdateConversationPatch>({
    mutationFn: (patch) =>
      apiFetch(`/conversations/${id}`, { method: "PATCH", body: patch }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.conversations() });
      qc.invalidateQueries({ queryKey: qk.conversation(id) });
    },
  });
}

export function useDeleteConversation() {
  const qc = useQueryClient();
  return useMutation<null, Error, string>({
    mutationFn: (id) =>
      apiFetch(`/conversations/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.conversations() }),
  });
}

// useSendMessage — no invalidate; the eventlog SSE drives UI updates instead.
//
// useSendMessage 不 invalidate；SSE 推回事件驱动 UI，不走 REST refetch。
export function useSendMessage(convId: string) {
  return useMutation<unknown, Error, SendMessageBody>({
    mutationFn: (body) =>
      apiFetch(`/conversations/${convId}/messages`, { method: "POST", body }),
  });
}

export function useCancelStream(convId: string) {
  return useMutation<null, Error, void>({
    mutationFn: () =>
      apiFetch(`/conversations/${convId}/stream`, { method: "DELETE" }),
  });
}
