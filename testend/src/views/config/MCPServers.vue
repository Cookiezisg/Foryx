<script setup lang="ts">
/**
 * MCP Servers — installed servers (live status + tool counts) + the
 * curated registry of available ones. Click an installed server to
 * inspect its tools; reconnect/health-check via action buttons; install
 * from registry.
 */
import { onMounted, ref, computed } from 'vue';
import { mcpAPI } from '@/api/resources';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import SubTabBar from '@/components/common/SubTabBar.vue';
import { timeAgo, statusClass } from '@/utils/format';
import type { MCPServerStatus, MCPRegistryEntry } from '@/types/domain';

const ui = useUIStore();
const servers = ref<MCPServerStatus[]>([]);
const registry = ref<MCPRegistryEntry[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

const tab = ref<'installed' | 'registry'>('installed');
const selected = ref<MCPServerStatus | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    servers.value = await mcpAPI.servers();
    registry.value = await mcpAPI.registry();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

const tabs = computed(() => [
  { id: 'installed', label: 'Installed', badge: servers.value.length },
  { id: 'registry', label: 'Registry', badge: registry.value.length },
]);

async function doReconnect(name: string) {
  try { await mcpAPI.reconnect(name); ui.toast('ok', 'reconnect'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
async function doHealth(name: string) {
  try { const r = await mcpAPI.healthCheck(name); ui.toast(r.ok ? 'ok' : 'err', r.message ?? (r.ok ? 'ok' : 'failed')); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
async function doRemove(name: string) {
  if (!confirm(`Uninstall ${name}?`)) return;
  try { await mcpAPI.remove(name); ui.toast('ok', 'uninstalled'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
async function doInstall(name: string) {
  // For tier-1 curated entries with no required args, install directly.
  try {
    await mcpAPI.install(name);
    ui.toast('ok', `${name} installing`);
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}
</script>

<template>
  <div class="view">
    <ViewHeader title="MCP servers" :subtitle="`${servers.length} installed · ${registry.length} in registry`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
      </template>
    </ViewHeader>
    <SubTabBar v-model="tab" :tabs="tabs" />

    <div v-if="tab === 'installed'" class="split">
      <div class="col-list scroll">
        <div v-if="err" class="error">⨯ {{ err }}</div>
        <table class="table">
          <thead><tr><th>name</th><th style="width: 110px">status</th><th style="width: 60px">tools</th></tr></thead>
          <tbody>
            <tr v-for="s in servers" :key="s.name" class="row-clickable" :class="{ active: selected?.name === s.name }" @click="selected = s">
              <td>{{ s.name }}</td>
              <td><span class="pill" :class="statusClass(s.status)">{{ s.status }}</span></td>
              <td class="mono xs">{{ s.tools.length }}</td>
            </tr>
            <tr v-if="servers.length === 0 && !loading">
              <td colspan="3" class="empty-row">No MCP servers installed.</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="col-detail scroll">
        <div v-if="!selected" class="dim center">Pick an installed server.</div>
        <div v-else>
          <header class="d-head">
            <strong>{{ selected.name }}</strong>
            <span class="pill" :class="statusClass(selected.status)">{{ selected.status }}</span>
            <span class="dim xs">{{ selected.connectedAt ? `connected ${timeAgo(selected.connectedAt)} ago` : 'not connected' }}</span>
            <span class="spacer" />
            <button class="btn ghost sm" @click="doReconnect(selected.name)">reconnect</button>
            <button class="btn ghost sm" @click="doHealth(selected.name)">health</button>
            <button class="btn danger sm" @click="doRemove(selected.name)">uninstall</button>
            <button class="btn ghost sm" @click="ui.showRaw(selected.name, selected)">raw</button>
          </header>
          <p v-if="selected.lastError" class="error">⨯ {{ selected.lastError }}</p>
          <p class="dim small">
            calls: {{ selected.totalCalls }} · failures: {{ selected.totalFailures }} · consec fails: {{ selected.consecutiveFailures }}
          </p>
          <h4>tools ({{ selected.tools.length }})</h4>
          <table class="table">
            <thead><tr><th>name</th><th>description</th></tr></thead>
            <tbody>
              <tr v-for="t in selected.tools" :key="t.name">
                <td class="mono">{{ t.name }}</td>
                <td class="dim small">{{ t.description ?? '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div v-else class="scroll">
      <table class="table">
        <thead><tr><th style="width: 200px">name</th><th>description</th><th style="width: 90px">category</th><th style="width: 80px">runtime</th><th style="width: 80px">tier</th><th style="width: 100px"></th></tr></thead>
        <tbody>
          <tr v-for="r in registry" :key="r.name">
            <td><strong>{{ r.name }}</strong></td>
            <td class="dim">{{ r.description }}</td>
            <td><span v-if="r.category" class="pill info">{{ r.category }}</span></td>
            <td class="mono xs">{{ r.runtime }}</td>
            <td><span v-if="r.tier" class="pill info">T{{ r.tier }}</span></td>
            <td>
              <button class="btn primary sm" @click="doInstall(r.name)" :disabled="(r.requiredArgs ?? []).length > 0 || (r.requiredEnv ?? []).length > 0">
                install
              </button>
              <button class="btn ghost sm" @click="ui.showRaw(r.name, r)">raw</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.split { flex: 1; display: flex; min-height: 0; }
.col-list { width: 340px; border-right: 1px solid var(--border-1); flex-shrink: 0; }
.col-detail { flex: 1; padding: var(--sp-3); }
.scroll { overflow: auto; padding: var(--sp-2); }
.col-detail h4 { margin: var(--sp-3) 0 var(--sp-1); font-size: var(--fs-xs); text-transform: uppercase; }
.row-clickable { cursor: pointer; }
.row-clickable:hover td { background: var(--bg-hover); }
.row-clickable.active td { background: var(--bg-active); }
.d-head { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: var(--sp-2); flex-wrap: wrap; }
.spacer { flex: 1; }
.center { padding: var(--sp-6); text-align: center; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); margin: var(--sp-2) 0; }
</style>
