<script setup lang="ts">
/**
 * FlowRuns — global list across all workflows. Filter by status / trigger kind.
 * Click row → /execute/flowruns/{id}.
 */
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { flowrunAPI } from '@/api/flowruns';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, duration, shortID, statusClass } from '@/utils/format';
import type { FlowRun } from '@/types/domain';

const router = useRouter();
const ui = useUIStore();
const items = ref<FlowRun[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);
const status = ref<string>('');
const triggerKind = ref<string>('');

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    const page = await flowrunAPI.list({
      status: status.value || undefined,
      triggerKind: triggerKind.value || undefined,
      limit: 200,
    });
    items.value = page.items;
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

function open(id: string) {
  router.push(`/execute/flowruns/${id}`);
}
</script>

<template>
  <div class="view">
    <ViewHeader title="FlowRuns" :subtitle="`${items.length} runs · across all workflows`">
      <template #actions>
        <select v-model="status" class="sm" @change="refresh">
          <option value="">any status</option>
          <option value="running">running</option>
          <option value="paused">paused</option>
          <option value="completed">completed</option>
          <option value="failed">failed</option>
          <option value="cancelled">cancelled</option>
        </select>
        <select v-model="triggerKind" class="sm" @change="refresh">
          <option value="">any kind</option>
          <option value="cron">cron</option>
          <option value="fsnotify">fsnotify</option>
          <option value="webhook">webhook</option>
          <option value="manual">manual</option>
        </select>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">{{ loading ? '...' : 'refresh' }}</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <table class="table">
        <thead>
          <tr>
            <th style="width: 200px">id</th>
            <th style="width: 200px">workflow</th>
            <th style="width: 80px">trigger</th>
            <th style="width: 100px">status</th>
            <th style="width: 90px">elapsed</th>
            <th style="width: 110px">started</th>
            <th>error</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in items" :key="r.id" class="row-clickable" @click="open(r.id)">
            <td class="mono xs">{{ shortID(r.id, 10) }}</td>
            <td class="mono xs">{{ shortID(r.workflowId, 10) }}</td>
            <td><span class="pill info">{{ r.triggerKind }}</span></td>
            <td><span class="pill" :class="statusClass(r.status)">{{ r.status }}</span></td>
            <td class="mono xs">{{ duration(r.elapsedMs) }}</td>
            <td class="dim xs">{{ timeAgo(r.startedAt) }}</td>
            <td class="ellipsis-cell">{{ r.errorMessage || '—' }}</td>
            <td><button class="btn ghost sm" @click.stop="ui.showRaw(r.id, r)">raw</button></td>
          </tr>
          <tr v-if="!loading && items.length === 0">
            <td colspan="8" class="empty-row">No flowruns. Trigger a workflow from its detail page.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.row-clickable { cursor: pointer; }
.row-clickable:hover td { background: var(--bg-hover); }
.ellipsis-cell { max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--fs-xs); }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); margin: var(--sp-2) var(--sp-3); }
</style>
