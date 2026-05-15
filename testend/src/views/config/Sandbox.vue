<script setup lang="ts">
/**
 * Sandbox — mise-managed runtimes (python/node/rust/...) + per-plugin
 * envs (forge / mcp / skill / conversation). Disk usage rollup + bootstrap
 * status; destroy actions for cleanup.
 */
import { onMounted, ref, computed } from 'vue';
import { sandboxAPI } from '@/api/resources';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import SubTabBar from '@/components/common/SubTabBar.vue';
import { timeAgo, bytes, statusClass, shortID } from '@/utils/format';
import type { SandboxRuntime, SandboxEnv } from '@/types/domain';

const ui = useUIStore();
const runtimes = ref<SandboxRuntime[]>([]);
const envs = ref<SandboxEnv[]>([]);
const usage = ref<{ totalBytes: number; runtimeBytes: number; envBytes: number } | null>(null);
const bootstrap = ref<{ ready: boolean; miseBin?: string; message?: string } | null>(null);
const loading = ref(false);
const err = ref<string | null>(null);

const tab = ref<'runtimes' | 'envs'>('runtimes');

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    runtimes.value = await sandboxAPI.runtimes();
    // Backend's /sandbox/envs requires ownerKind; iterate the 4 known
    // owner kinds (per D-redo-8) and aggregate.
    const kinds = ['forge_function', 'forge_handler', 'mcp', 'skill', 'conversation'];
    const all: SandboxEnv[] = [];
    for (const k of kinds) {
      try {
        const rows = await sandboxAPI.envs({ ownerKind: k });
        all.push(...rows);
      } catch { /* skip empty / unsupported */ }
    }
    envs.value = all;
    usage.value = await sandboxAPI.diskUsage();
    bootstrap.value = await sandboxAPI.bootstrapStatus();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

const tabs = computed(() => [
  { id: 'runtimes', label: 'Runtimes', badge: runtimes.value.length },
  { id: 'envs', label: 'Envs', badge: envs.value.length },
]);

async function destroyRuntime(id: string) {
  if (!confirm(`Destroy runtime ${shortID(id, 10)}?`)) return;
  try { await sandboxAPI.destroyRuntime(id); ui.toast('ok', 'destroyed'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}

async function destroyEnv(id: string) {
  if (!confirm(`Destroy env ${shortID(id, 10)}?`)) return;
  try { await sandboxAPI.destroyEnv(id); ui.toast('ok', 'destroyed'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}

async function doGC() {
  try { await sandboxAPI.action('gc'); ui.toast('ok', 'gc done'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}

async function doRetryBootstrap() {
  try { await sandboxAPI.action('retry-bootstrap'); ui.toast('ok', 'bootstrap retrying'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
</script>

<template>
  <div class="view">
    <ViewHeader
      title="Sandbox"
      :subtitle="usage ? `${bytes(usage.totalBytes)} on disk · ${runtimes.length} runtimes · ${envs.length} envs` : 'loading...'"
    >
      <template #actions>
        <button class="btn ghost sm" @click="doGC">gc</button>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
      </template>
    </ViewHeader>
    <div v-if="bootstrap" class="bootstrap-row">
      <span class="pill" :class="bootstrap.ready ? 'ok' : 'err'">{{ bootstrap.ready ? 'bootstrap ready' : 'bootstrap not ready' }}</span>
      <span v-if="bootstrap.miseBin" class="mono xs dim">{{ bootstrap.miseBin }}</span>
      <span v-if="bootstrap.message" class="dim small">{{ bootstrap.message }}</span>
      <button v-if="!bootstrap.ready" class="btn primary sm" @click="doRetryBootstrap">retry</button>
    </div>
    <SubTabBar v-model="tab" :tabs="tabs" />
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>

      <table v-if="tab === 'runtimes'" class="table">
        <thead><tr><th style="width: 200px">id</th><th style="width: 100px">kind</th><th style="width: 100px">version</th><th>path</th><th style="width: 90px">size</th><th style="width: 100px">installed</th><th style="width: 80px"></th></tr></thead>
        <tbody>
          <tr v-for="r in runtimes" :key="r.id">
            <td class="mono xs">{{ shortID(r.id, 10) }}</td>
            <td><span class="pill info">{{ r.kind }}</span></td>
            <td class="mono xs">{{ r.version }}</td>
            <td class="mono xs ellipsis-cell">{{ r.path }}</td>
            <td class="mono xs">{{ bytes(r.sizeBytes) }}</td>
            <td class="dim xs">{{ timeAgo(r.installedAt) }}</td>
            <td><button class="btn danger sm" @click="destroyRuntime(r.id)">delete</button></td>
          </tr>
          <tr v-if="runtimes.length === 0"><td colspan="7" class="empty-row">No runtimes installed.</td></tr>
        </tbody>
      </table>

      <table v-else class="table">
        <thead><tr><th style="width: 200px">id</th><th style="width: 120px">owner kind</th><th style="width: 200px">owner id</th><th style="width: 90px">status</th><th>path</th><th style="width: 100px">last used</th><th style="width: 80px"></th></tr></thead>
        <tbody>
          <tr v-for="e in envs" :key="e.id">
            <td class="mono xs">{{ shortID(e.id, 10) }}</td>
            <td><span class="pill info">{{ e.ownerKind }}</span></td>
            <td class="mono xs">{{ shortID(e.ownerId, 10) }}</td>
            <td><span class="pill" :class="statusClass(e.status)">{{ e.status }}</span></td>
            <td class="mono xs ellipsis-cell">{{ e.path }}</td>
            <td class="dim xs">{{ e.lastUsedAt ? timeAgo(e.lastUsedAt) : '—' }}</td>
            <td><button class="btn danger sm" @click="destroyEnv(e.id)">delete</button></td>
          </tr>
          <tr v-if="envs.length === 0"><td colspan="7" class="empty-row">No envs.</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.bootstrap-row {
  display: flex; align-items: center; gap: var(--sp-2);
  padding: var(--sp-2) var(--sp-3); background: var(--bg-1); border-bottom: 1px solid var(--border-1);
}
.ellipsis-cell { max-width: 350px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
