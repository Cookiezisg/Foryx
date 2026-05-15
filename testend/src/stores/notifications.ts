/**
 * Notifications store — accumulates events from the /api/v1/notifications
 * SSE stream into a client-side ring buffer (max 1000 entries).
 *
 * Used by:
 *   - sidebar status pill (count, latest type)
 *   - OBSERVE › Notification history view
 *   - CURRENT CONV › Notifications view (filtered by conversationId)
 */

import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { subscribe } from '@/api/sse';

export interface NotificationEvent {
  seq: number;
  receivedAt: number;
  type: string;            // entity type ("conversation", "function", "workflow", "flowrun", ...)
  id: string;              // entity ID
  conversationId?: string; // optional scope
  action?: string;
  data?: Record<string, unknown>;
  raw: unknown;
}

const MAX = 1000;

export const useNotificationsStore = defineStore('notifications', () => {
  const events = ref<NotificationEvent[]>([]);
  const connected = ref(false);
  let unsub: (() => void) | null = null;

  function start() {
    if (unsub) return;
    unsub = subscribe('notifications', (ev) => {
      // Notifications stream emits a single envelope shape:
      // {type, id, data, conversationId} — see events-design.md §11.3.
      const env = ev.data as { type?: string; id?: string; data?: Record<string, unknown>; conversationId?: string };
      const n: NotificationEvent = {
        seq: ev.id,
        receivedAt: ev.receivedAt,
        type: env.type ?? 'unknown',
        id: env.id ?? '',
        conversationId: env.conversationId,
        action: env.data?.action as string | undefined,
        data: env.data,
        raw: ev.data,
      };
      events.value.unshift(n);
      if (events.value.length > MAX) events.value.length = MAX;
      connected.value = true;
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

  /* derived selectors */
  function forConv(convId: string | null) {
    if (!convId) return [] as NotificationEvent[];
    return events.value.filter((e) => e.conversationId === convId);
  }

  const counts = computed(() => {
    const c: Record<string, number> = {};
    for (const e of events.value) c[e.type] = (c[e.type] ?? 0) + 1;
    return c;
  });

  return {
    events,
    connected,
    counts,
    start,
    stop,
    clear,
    forConv,
  };
});
