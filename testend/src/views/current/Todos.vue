<script setup lang="ts">
/**
 * Todos — derived from the notifications stream (type=todo) filtered to the
 * current conversation. Backend has no list-todos HTTP endpoint; todos are
 * tool-driven (TaskCreate / TaskUpdate / TaskList / TaskGet system tools)
 * and surfaced via the notifications SSE stream with slim payload
 * `{action, id, conversationId}` per E1.
 *
 * What we render: the action ledger as it accrues during this browser
 * session. Full per-todo state would need a TaskList tool call.
 */
import { computed } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useNotificationsStore } from '@/stores/notifications';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { timestamp, shortID } from '@/utils/format';

const conv = useConvStore();
const notifs = useNotificationsStore();
const ui = useUIStore();

const rows = computed(() =>
  notifs.forConv(conv.selectedId).filter((n) => n.type === 'todo'),
);

/* Roll up the ledger into a current-state view: latest seq per todo id. */
const latestById = computed(() => {
  const m = new Map<string, (typeof rows.value)[number]>();
  for (const r of rows.value) {
    const prev = m.get(r.id);
    if (!prev || r.seq > prev.seq) m.set(r.id, r);
  }
  return Array.from(m.values()).sort((a, b) => b.seq - a.seq);
});
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Todos" hint="选个对话先" />
  </div>
  <div v-else class="view">
    <ViewHeader
      title="Todos"
      :subtitle="`conv ${conv.selectedId} · ${latestById.length} distinct todos · ${rows.length} actions`"
    />
    <div class="scroll">
      <p class="dim small note">
        Note: backend has no list-todos endpoint. This view rolls up the
        notifications ledger by todo id and shows the latest action seen
        this session. Use the TaskList tool from chat to get authoritative state.
      </p>
      <table class="table">
        <thead>
          <tr>
            <th style="width: 200px">id</th>
            <th style="width: 120px">latest action</th>
            <th>delta</th>
            <th style="width: 80px">seq</th>
            <th style="width: 120px">at</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in latestById" :key="t.id">
            <td class="mono xs">{{ shortID(t.id, 10) }}</td>
            <td><span class="pill info">{{ t.action ?? '?' }}</span></td>
            <td class="mono xs ellipsis-cell">{{ JSON.stringify(t.data ?? {}) }}</td>
            <td class="mono xs dim">{{ t.seq }}</td>
            <td class="dim xs">{{ timestamp(t.receivedAt) }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(`todo ${t.id}`, t.raw)">raw</button></td>
          </tr>
          <tr v-if="latestById.length === 0">
            <td colspan="6" class="empty-row">No todo actions yet for this conversation.</td>
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
.note {
  padding: var(--sp-2) var(--sp-3);
  font-style: italic;
}
.ellipsis-cell {
  max-width: 400px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty-row {
  text-align: center;
  color: var(--fg-3);
  padding: var(--sp-6) 0;
  font-style: italic;
}
</style>
