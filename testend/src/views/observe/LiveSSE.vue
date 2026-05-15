<script setup lang="ts">
/**
 * Live SSE — global tail of the three per-user backend streams in three
 * columns: eventlog (chat side), notifications (entity changes), forge
 * (trinity forging progress). Combines existing stores into one panoramic
 * view. Newest first.
 */
import { computed } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useNotificationsStore } from '@/stores/notifications';
import { useForgeStore } from '@/stores/forge';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timestamp } from '@/utils/format';

const chat = useChatStore();
const notifs = useNotificationsStore();
const forge = useForgeStore();
const ui = useUIStore();

const eventlogRows = computed(() => chat.rawEvents.slice(0, 200));
const notifRows = computed(() => notifs.events.slice(0, 200));
const forgeRows = computed(() => forge.events.slice(0, 200));
</script>

<template>
  <div class="view">
    <ViewHeader
      title="Live SSE"
      :subtitle="`3 streams · eventlog ${eventlogRows.length} · notif ${notifRows.length} · forge ${forgeRows.length} (newest 200 each)`"
    />
    <div class="cols">
      <div class="col">
        <header class="col-head">
          <span class="pill info">eventlog</span>
          <span class="dim small">{{ eventlogRows.length }}</span>
        </header>
        <div class="scroll">
          <article v-for="e in eventlogRows" :key="`el-${e.seq}-${e.event}`" class="row" @click="ui.showRaw(e.event, e.data)">
            <div class="row-l">
              <span class="seq mono xs">{{ e.seq }}</span>
              <span class="evt mono xs">{{ e.event }}</span>
            </div>
            <div class="row-r mono xs ellipsis">{{ JSON.stringify(e.data) }}</div>
            <div class="dim xs t">{{ timestamp(e.at) }}</div>
          </article>
        </div>
      </div>

      <div class="col">
        <header class="col-head">
          <span class="pill info">notifications</span>
          <span class="dim small">{{ notifRows.length }}</span>
        </header>
        <div class="scroll">
          <article v-for="n in notifRows" :key="`nf-${n.seq}-${n.type}-${n.id}`" class="row" @click="ui.showRaw(n.type, n.raw)">
            <div class="row-l">
              <span class="seq mono xs">{{ n.seq }}</span>
              <span class="evt mono xs">{{ n.type }}</span>
            </div>
            <div class="row-r mono xs ellipsis">{{ n.id }} · {{ n.action ?? '' }}</div>
            <div class="dim xs t">{{ timestamp(n.receivedAt) }}</div>
          </article>
        </div>
      </div>

      <div class="col">
        <header class="col-head">
          <span class="pill info">forge</span>
          <span class="dim small">{{ forgeRows.length }}</span>
        </header>
        <div class="scroll">
          <article v-for="f in forgeRows" :key="`fg-${f.seq}-${f.event}`" class="row" @click="ui.showRaw(f.event, f.raw)">
            <div class="row-l">
              <span class="seq mono xs">{{ f.seq }}</span>
              <span class="evt mono xs">{{ f.event }}</span>
            </div>
            <div class="row-r mono xs ellipsis">
              {{ f.scope ? `${f.scope.kind}:${f.scope.id}` : '' }}
              <span v-if="f.operation">· {{ f.operation }}</span>
              <span v-if="f.attempt"> · attempt {{ f.attempt }}</span>
            </div>
            <div class="dim xs t">{{ timestamp(f.receivedAt) }}</div>
          </article>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.cols { flex: 1; display: flex; gap: var(--sp-2); padding: 0 var(--sp-3) var(--sp-3); min-height: 0; }
.col {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--bg-1);
  border: 1px solid var(--border-1);
  border-radius: var(--radius-md);
  min-width: 0;
}
.col-head {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  padding: var(--sp-2);
  border-bottom: 1px solid var(--border-1);
  flex-shrink: 0;
}
.scroll { flex: 1; overflow: auto; padding: 4px; }
.row {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: var(--sp-1);
  padding: 4px var(--sp-1);
  border-bottom: 1px solid var(--border-2);
  cursor: pointer;
  font-size: var(--fs-xs);
}
.row:hover { background: var(--bg-hover); }
.row-l { display: flex; gap: 4px; }
.seq { color: var(--fg-3); }
.evt { color: var(--fg-2); font-weight: 600; }
.row-r { min-width: 0; }
.ellipsis { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.t { color: var(--fg-3); }
</style>
