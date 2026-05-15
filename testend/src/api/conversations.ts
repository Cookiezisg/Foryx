import { deleteEmpty, getJSON, getPage, patchJSON, postJSON } from './client';
import type { Conversation, Message } from '@/types/domain';

export const convAPI = {
  list: (limit = 100) => getPage<Conversation>('/api/v1/conversations', { limit }),

  get: (id: string) => getJSON<Conversation>(`/api/v1/conversations/${id}`),

  create: (title = '') =>
    postJSON<Conversation>('/api/v1/conversations', { title }),

  rename: (id: string, title: string) =>
    patchJSON<Conversation>(`/api/v1/conversations/${id}`, { title }),

  setSystemPrompt: (id: string, systemPrompt: string) =>
    patchJSON<Conversation>(`/api/v1/conversations/${id}`, { systemPrompt }),

  remove: (id: string) => deleteEmpty(`/api/v1/conversations/${id}`),

  /* messages */

  messages: (convId: string, limit = 200, cursor?: string) =>
    getPage<Message>(`/api/v1/conversations/${convId}/messages`, { limit, cursor }),

  sendMessage: (convId: string, content: string, attachmentIds: string[] = []) =>
    postJSON<{ messageId: string }>(`/api/v1/conversations/${convId}/messages`, {
      content,
      attachmentIds,
    }),

  /** Cancel the active stream (if any). Backend returns 204. */
  cancel: (convId: string) =>
    deleteEmpty(`/api/v1/conversations/${convId}/stream`),

  /**
   * Deliver an AskUserQuestion answer back to the blocked tool.
   * Backend wire (handlers/answers.go): POST → 204,
   *   body `{toolCallId, answer:string, skipped?:bool}`.
   * If skipped=true, backend substitutes "(user skipped)" sentinel so the
   * agent can decide its own default behaviour (2026-05 #6 redesign).
   */
  deliverAnswer: (convId: string, toolCallId: string, answer: string, skipped = false) =>
    postJSON<void>(`/api/v1/conversations/${convId}/answers`, {
      toolCallId,
      answer,
      skipped,
    }),

  /** History replay for the SSE eventlog (after 410 SEQ_TOO_OLD). */
  eventlogHistory: (convId: string, from = 0) =>
    getJSON<unknown[]>(`/api/v1/conversations/${convId}/eventlog?from=${from}`),
};
