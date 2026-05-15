<script setup lang="ts">
/**
 * Routes — every HTTP endpoint the backend has registered (sourced from
 * `/dev/routes`). Copy-as-curl helper for quick smoke testing.
 */
import { onMounted, ref, computed } from 'vue';
import { devAPI } from '@/api/dev';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';

interface Route { method: string; path: string; handler: string }

const ui = useUIStore();
const routes = ref<Route[]>([]);
const filter = ref('');
const methodFilter = ref<string>('');
const loading = ref(false);
const err = ref<string | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    routes.value = await devAPI.routes();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

const methods = computed(() => Array.from(new Set(routes.value.map((r) => r.method))).sort());

const filtered = computed(() => {
  let rs = routes.value;
  if (methodFilter.value) rs = rs.filter((r) => r.method === methodFilter.value);
  const q = filter.value.trim().toLowerCase();
  if (q) rs = rs.filter((r) => r.path.toLowerCase().includes(q) || r.handler.toLowerCase().includes(q));
  return rs;
});

async function copy(r: Route) {
  const port = window.location.port || '5174';
  let curl = `curl -X ${r.method} http://localhost:${port}${r.path}`;
  if (r.method === 'POST' || r.method === 'PATCH' || r.method === 'PUT') {
    curl += " -H 'Content-Type: application/json' -d '{}'";
  }
  try {
    await navigator.clipboard.writeText(curl);
    ui.toast('ok', '已复制 curl');
  } catch (e) {
    ui.toast('err', `复制失败: ${(e as Error).message}`);
  }
}

function methodClass(m: string): string {
  switch (m) {
    case 'GET': return 'ok';
    case 'POST': return 'info';
    case 'PUT': return 'pending';
    case 'PATCH': return 'pending';
    case 'DELETE': return 'err';
    default: return '';
  }
}
</script>

<template>
  <div class="view">
    <ViewHeader title="Routes" :subtitle="`${filtered.length} of ${routes.length} routes`">
      <template #actions>
        <select v-model="methodFilter" class="sm">
          <option value="">all methods</option>
          <option v-for="m in methods" :key="m" :value="m">{{ m }}</option>
        </select>
        <input v-model="filter" placeholder="filter path/handler…" class="sm" />
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <table class="table">
        <thead>
          <tr>
            <th style="width: 90px">method</th>
            <th>path</th>
            <th>handler</th>
            <th style="width: 70px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in filtered" :key="`${r.method} ${r.path}`">
            <td><span class="pill" :class="methodClass(r.method)">{{ r.method }}</span></td>
            <td class="mono">{{ r.path }}</td>
            <td class="dim small">{{ r.handler }}</td>
            <td><button class="btn ghost sm" @click="copy(r)" title="copy as curl">⎘</button></td>
          </tr>
          <tr v-if="filtered.length === 0">
            <td colspan="4" class="empty-row">No routes match.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-4) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
