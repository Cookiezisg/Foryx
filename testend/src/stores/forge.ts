/**
 * Forge SSE store — accumulates events from /api/v1/forge into a ring
 * buffer for the OBSERVE › Live SSE + per-entity forge progress panels.
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';
import { subscribe } from '@/api/sse';

export interface ForgeEvent {
  seq: number;
  receivedAt: number;
  event: 'forge_started' | 'forge_op_applied' | 'forge_env_attempt' | 'forge_completed' | string;
  scope?: { kind: 'function' | 'handler' | 'workflow'; id: string };
  operation?: string;
  attempt?: number;
  status?: string;
  conversationId?: string;
  toolCallId?: string;
  versionId?: string;
  versionNumber?: number;
  elapsedMs?: number;
  error?: string;
  raw: unknown;
}

const MAX = 500;

export const useForgeStore = defineStore('forgeSSE', () => {
  const events = ref<ForgeEvent[]>([]);
  let unsub: (() => void) | null = null;

  function start() {
    if (unsub) return;
    unsub = subscribe('forge', (ev) => {
      const data = ev.data as Record<string, unknown>;
      const wrapped: ForgeEvent = {
        seq: ev.id,
        receivedAt: ev.receivedAt,
        event: ev.event,
        scope: data.scope as ForgeEvent['scope'],
        operation: data.operation as string | undefined,
        attempt: data.attempt as number | undefined,
        status: data.status as string | undefined,
        conversationId: data.conversationId as string | undefined,
        toolCallId: data.toolCallId as string | undefined,
        versionId: data.versionId as string | undefined,
        versionNumber: data.versionNumber as number | undefined,
        elapsedMs: data.elapsedMs as number | undefined,
        error: data.error as string | undefined,
        raw: ev.data,
      };
      events.value.unshift(wrapped);
      if (events.value.length > MAX) events.value.length = MAX;
    });
  }

  function stop() {
    if (unsub) {
      unsub();
      unsub = null;
    }
  }

  function clear() {
    events.value = [];
  }

  /** Events for one trinity entity. */
  function forEntity(kind: 'function' | 'handler' | 'workflow', id: string) {
    return events.value.filter((e) => e.scope && e.scope.kind === kind && e.scope.id === id);
  }

  return { events, start, stop, clear, forEntity };
});
