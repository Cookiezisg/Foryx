<script setup lang="ts">
/**
 * Triggers — aggregated across all workflows. Backend has no
 * `/api/v1/triggers` endpoint, so we iterate `/api/v1/workflows` and pull
 * each workflow's `/triggers` list. Refresh on demand.
 */
import { onMounted, ref, computed } from 'vue';
import { useRouter } from 'vue-router';
import { wfAPI } from '@/api/workflows';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, shortID, statusClass } from '@/utils/format';
import type { Workflow, TriggerState } from '@/types/domain';

const router = useRouter();

interface Row extends TriggerState {
  workflowName: string;
}

const rows = ref<Row[]>([]);
const workflows = ref<Workflow[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);
const kindFilter = ref<string>('');

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    const page = await wfAPI.list(200);
    workflows.value = page.items;
    const out: Row[] = [];
    for (const w of workflows.value) {
      try {
        const ts = await wfAPI.triggerStates(w.id);
        for (const t of ts) {
          out.push({ ...t, workflowName: w.name });
        }
      } catch {
        // skip workflows that error
      }
    }
    rows.value = out;
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

const filtered = computed(() =>
  kindFilter.value ? rows.value.filter((r) => r.kind === kindFilter.value) : rows.value,
);

function openWorkflow(id: string) {
  router.push(`/forge/workflows/${id}`);
}
</script>

<template>
  <div class="view">
    <ViewHeader title="Triggers" :subtitle="`${filtered.length} triggers across ${workflows.length} workflows`">
      <template #actions>
        <select v-model="kindFilter" class="sm">
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
            <th style="width: 200px">workflow</th>
            <th style="width: 160px">node</th>
            <th style="width: 100px">kind</th>
            <th style="width: 100px">status</th>
            <th style="width: 130px">next fire</th>
            <th style="width: 130px">last fire</th>
            <th>error</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in filtered" :key="`${t.workflowId}-${t.nodeId}`">
            <td>
              <a class="link mono" @click="openWorkflow(t.workflowId)">{{ t.workflowName }}</a>
              <div class="dim xs mono">{{ shortID(t.workflowId, 8) }}</div>
            </td>
            <td class="mono xs">{{ t.nodeId }}</td>
            <td><span class="pill info">{{ t.kind }}</span></td>
            <td><span class="pill" :class="statusClass(t.status)">{{ t.status }}</span></td>
            <td class="dim xs">{{ t.nextFireAt ? timeAgo(t.nextFireAt) : '—' }}</td>
            <td class="dim xs">{{ t.lastFiredAt ? timeAgo(t.lastFiredAt) : '—' }}</td>
            <td class="ellipsis-cell">{{ t.lastError ?? '—' }}</td>
          </tr>
          <tr v-if="filtered.length === 0">
            <td colspan="7" class="empty-row">No triggers registered.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.link { color: var(--accent); cursor: pointer; }
.ellipsis-cell { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
