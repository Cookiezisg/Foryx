<script setup lang="ts">
/**
 * Eventlog raw — every SSE event from /api/v1/eventlog scoped to the
 * currently selected conversation. Newest first. Tap a row to dump full
 * payload via the raw-JSON modal.
 */
import { computed, ref } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useChatStore } from '@/stores/chat';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { timestamp } from '@/utils/format';

function eventCls(event: string): string {
  if (event === 'block_delta') return 'info';
  if (event === 'message_stop' || event === 'block_stop') return 'ok';
  if (event === 'message_start' || event === 'block_start') return 'pending';
  return '';
}

const conv = useConvStore();
const chat = useChatStore();
const ui = useUIStore();

const typeFilter = ref<string>('all');
const types = ['all', 'message_start', 'message_stop', 'block_start', 'block_delta', 'block_stop'] as const;

const rows = computed(() => {
  if (!conv.selectedId) return [];
  return chat.rawEvents.filter(
    (e) => e.convId === conv.selectedId && (typeFilter.value === 'all' || e.event === typeFilter.value),
  );
});
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Eventlog raw" hint="选个对话先" />
  </div>
  <div v-else class="view">
    <ViewHeader title="Eventlog raw" :subtitle="`conv ${conv.selectedId} · ${rows.length} events captured this session`">
      <template #actions>
        <select v-model="typeFilter" class="sm">
          <option v-for="t in types" :key="t" :value="t">{{ t }}</option>
        </select>
      </template>
    </ViewHeader>
    <div class="scroll">
      <table class="table mono">
        <thead>
          <tr>
            <th style="width: 80px">seq</th>
            <th style="width: 160px">event</th>
            <th>data (preview)</th>
            <th style="width: 100px">at</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="`${r.seq}-${r.event}`">
            <td class="dim">{{ r.seq }}</td>
            <td>
              <span class="pill" :class="eventCls(r.event)">{{ r.event }}</span>
            </td>
            <td class="data-cell">{{ JSON.stringify(r.data) }}</td>
            <td class="dim xs">{{ timestamp(r.at) }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(`${r.event} #${r.seq}`, r.data)">raw</button></td>
          </tr>
          <tr v-if="rows.length === 0">
            <td colspan="5" class="empty-row">
              No events captured yet for this conversation. Send a message in chat to see events stream in here.
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.view-pad {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.scroll {
  flex: 1;
  overflow: auto;
  padding: 0 var(--sp-3) var(--sp-3);
}
.data-cell {
  max-width: 600px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--fs-xs);
}
.empty-row {
  text-align: center;
  color: var(--fg-3);
  padding: var(--sp-6) 0;
  font-style: italic;
}
</style>
