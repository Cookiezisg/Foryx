<script setup lang="ts">
/**
 * Notification history — full ring buffer of `/api/v1/notifications` SSE
 * events with type filter + per-type counters. Newest first.
 */
import { computed, ref } from 'vue';
import { useNotificationsStore } from '@/stores/notifications';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timestamp, shortID } from '@/utils/format';

const notifs = useNotificationsStore();
const ui = useUIStore();

const typeFilter = ref<string>('');

const rows = computed(() =>
  typeFilter.value ? notifs.events.filter((e) => e.type === typeFilter.value) : notifs.events,
);

const knownTypes = computed(() => Object.keys(notifs.counts));
</script>

<template>
  <div class="view">
    <ViewHeader title="Notification history" :subtitle="`${rows.length} of ${notifs.events.length} (ring buffer 1000) · types: ${knownTypes.join(', ') || 'none yet'}`">
      <template #actions>
        <select v-model="typeFilter" class="sm">
          <option value="">all types</option>
          <option v-for="t in knownTypes" :key="t" :value="t">{{ t }} ({{ notifs.counts[t] }})</option>
        </select>
        <button class="btn ghost sm" @click="notifs.clear">clear</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <table class="table">
        <thead>
          <tr>
            <th style="width: 80px">seq</th>
            <th style="width: 140px">type</th>
            <th style="width: 220px">id</th>
            <th style="width: 120px">action</th>
            <th style="width: 220px">convId</th>
            <th>data</th>
            <th style="width: 130px">at</th>
            <th style="width: 50px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="n in rows" :key="`${n.seq}-${n.type}-${n.id}`">
            <td class="mono xs dim">{{ n.seq }}</td>
            <td><span class="pill info">{{ n.type }}</span></td>
            <td class="mono xs">{{ shortID(n.id, 12) }}</td>
            <td class="mono xs">{{ n.action ?? '—' }}</td>
            <td class="mono xs">{{ n.conversationId ? shortID(n.conversationId, 10) : '—' }}</td>
            <td class="mono xs data-cell">{{ n.data ? JSON.stringify(n.data) : '' }}</td>
            <td class="dim xs">{{ timestamp(n.receivedAt) }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(`${n.type} ${n.id}`, n.raw)">raw</button></td>
          </tr>
          <tr v-if="rows.length === 0">
            <td colspan="8" class="empty-row">No notifications captured yet this session.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.data-cell { max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
</style>
