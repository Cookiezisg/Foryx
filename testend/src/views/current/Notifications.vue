<script setup lang="ts">
/**
 * Notifications (current conv) — entity-state events for the selected
 * conversation. Subset of the global notifications stream filtered by
 * the conv id in the payload.
 */
import { computed } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useNotificationsStore } from '@/stores/notifications';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { timestamp } from '@/utils/format';

const conv = useConvStore();
const notifs = useNotificationsStore();
const ui = useUIStore();

const rows = computed(() => notifs.forConv(conv.selectedId));
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Notifications" hint="选个对话先" />
  </div>
  <div v-else class="view">
    <ViewHeader title="Notifications" :subtitle="`conv ${conv.selectedId} · ${rows.length} notifications`" />
    <div class="scroll">
      <table class="table">
        <thead>
          <tr>
            <th style="width: 70px">seq</th>
            <th style="width: 140px">type</th>
            <th style="width: 200px">id</th>
            <th style="width: 120px">action</th>
            <th>data</th>
            <th style="width: 90px">at</th>
            <th style="width: 50px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="n in rows" :key="`${n.seq}-${n.type}-${n.id}`">
            <td class="dim mono xs">{{ n.seq }}</td>
            <td>
              <span class="pill info">{{ n.type }}</span>
            </td>
            <td class="mono xs ellipsis">{{ n.id }}</td>
            <td class="mono xs">{{ n.action ?? '—' }}</td>
            <td class="mono xs data-cell">{{ n.data ? JSON.stringify(n.data) : '' }}</td>
            <td class="dim xs">{{ timestamp(n.receivedAt) }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(`${n.type} ${n.id}`, n.raw)">raw</button></td>
          </tr>
          <tr v-if="rows.length === 0">
            <td colspan="7" class="empty-row">No notifications for this conversation yet.</td>
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
