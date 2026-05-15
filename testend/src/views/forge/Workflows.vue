<script setup lang="ts">
/**
 * Workflows list — paginated DAG-style automation flows.
 * Click row → /forge/workflows/{id}.
 */
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { wfAPI } from '@/api/workflows';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, shortID } from '@/utils/format';
import type { Workflow } from '@/types/domain';

const router = useRouter();
const ui = useUIStore();
const items = ref<Workflow[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    const page = await wfAPI.list(200);
    items.value = page.items;
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

function open(id: string) {
  router.push(`/forge/workflows/${id}`);
}
</script>

<template>
  <div class="view">
    <ViewHeader title="Workflows" :subtitle="`${items.length} forged · DAG automation flows`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">{{ loading ? '...' : 'refresh' }}</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <table class="table">
        <thead>
          <tr>
            <th style="width: 200px">name</th>
            <th>description</th>
            <th style="width: 80px">enabled</th>
            <th style="width: 80px">live</th>
            <th style="width: 120px">last fired</th>
            <th style="width: 100px">updated</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="w in items" :key="w.id" class="row-clickable" @click="open(w.id)">
            <td>
              <div class="cell-name">{{ w.name }}</div>
              <div class="dim xs mono">{{ shortID(w.id, 10) }}</div>
            </td>
            <td class="ellipsis-cell">{{ w.description }}</td>
            <td>
              <span class="pill" :class="w.enabled ? 'ok' : ''">{{ w.enabled ? 'enabled' : 'disabled' }}</span>
              <span v-if="w.needsAttention" class="pill warn" title="needsAttention">⚠</span>
            </td>
            <td class="mono xs">{{ w.liveRuns ?? 0 }}</td>
            <td class="dim xs">{{ w.lastFiredAt ? timeAgo(w.lastFiredAt) : '—' }}</td>
            <td class="dim xs">{{ timeAgo(w.updatedAt) }}</td>
            <td><button class="btn ghost sm" @click.stop="ui.showRaw(w.name, w)">raw</button></td>
          </tr>
          <tr v-if="!loading && items.length === 0">
            <td colspan="7" class="empty-row">
              No workflows forged yet. Use chat agent's <code>create_workflow</code> tool to forge one.
            </td>
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
.cell-name { font-weight: 600; }
.ellipsis-cell { max-width: 500px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.empty-row code { background: var(--bg-2); padding: 1px 6px; border-radius: 4px; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); margin: var(--sp-2) var(--sp-3); }
</style>
