<script setup lang="ts">
/**
 * Executions — D22 execution log unified view. The backend has per-entity
 * endpoints (function_executions / handler_calls / flowrun_nodes /
 * mcp_calls / skill_executions) but no global /executions endpoint, so
 * we aggregate by iterating functions + handlers.
 *
 * MCP calls + skill executions are not yet exposed via dedicated list
 * endpoints; flowrun_nodes are visible per-flowrun in the FlowRunDetail
 * view.
 */
import { onMounted, ref, computed } from 'vue';
import { fnAPI } from '@/api/functions';
import { hdAPI } from '@/api/handlers';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, duration, shortID, statusClass } from '@/utils/format';
import type { ExecutionRow, Function as Fn, Handler } from '@/types/domain';

const ui = useUIStore();

interface Row extends ExecutionRow {
  kind: 'function' | 'handler';
  ownerId: string;
  ownerName: string;
}

const rows = ref<Row[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);
const kindFilter = ref<'all' | 'function' | 'handler'>('all');
const statusFilter = ref<string>('');

async function refresh() {
  loading.value = true;
  err.value = null;
  rows.value = [];
  try {
    const [fns, hds] = await Promise.all([fnAPI.list(200), hdAPI.list(200)]);
    const out: Row[] = [];
    for (const f of fns.items as Fn[]) {
      try {
        const r = await fnAPI.executions(f.id, { limit: 50 });
        for (const e of r.executions ?? []) {
          out.push({ ...e, kind: 'function', ownerId: f.id, ownerName: f.name });
        }
      } catch { /* skip */ }
    }
    for (const h of hds.items as Handler[]) {
      try {
        const r = await hdAPI.calls(h.id, { limit: 50 });
        for (const c of r.calls ?? []) {
          out.push({ ...c, kind: 'handler', ownerId: h.id, ownerName: h.name });
        }
      } catch { /* skip */ }
    }
    // Sort newest first
    out.sort((a, b) => b.startedAt.localeCompare(a.startedAt));
    rows.value = out;
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

const filtered = computed(() => {
  let xs = rows.value;
  if (kindFilter.value !== 'all') xs = xs.filter((r) => r.kind === kindFilter.value);
  if (statusFilter.value) xs = xs.filter((r) => r.status === statusFilter.value);
  return xs;
});
</script>

<template>
  <div class="view">
    <ViewHeader title="Executions" :subtitle="`${filtered.length} rows · function + handler (D22)`">
      <template #actions>
        <select v-model="kindFilter" class="sm">
          <option value="all">all kinds</option>
          <option value="function">function</option>
          <option value="handler">handler</option>
        </select>
        <select v-model="statusFilter" class="sm">
          <option value="">any status</option>
          <option value="ok">ok</option>
          <option value="failed">failed</option>
          <option value="cancelled">cancelled</option>
          <option value="timeout">timeout</option>
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
            <th style="width: 100px">kind</th>
            <th style="width: 200px">owner</th>
            <th style="width: 90px">status</th>
            <th style="width: 80px">elapsed</th>
            <th style="width: 100px">started</th>
            <th>trigger / method</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in filtered" :key="`${r.kind}-${r.id}`">
            <td class="mono xs">{{ shortID(r.id, 10) }}</td>
            <td><span class="pill info">{{ r.kind }}</span></td>
            <td>
              <div class="mono">{{ r.ownerName }}</div>
              <div class="dim xs mono">{{ shortID(r.ownerId, 8) }}</div>
            </td>
            <td><span class="pill" :class="statusClass(r.status)">{{ r.status }}</span></td>
            <td class="mono xs">{{ duration(r.elapsedMs) }}</td>
            <td class="dim xs">{{ timeAgo(r.startedAt) }}</td>
            <td class="mono xs">{{ r.trigger ?? (r as any).methodName ?? '—' }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(r.id, r)">raw</button></td>
          </tr>
          <tr v-if="!loading && filtered.length === 0">
            <td colspan="8" class="empty-row">No executions recorded.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
