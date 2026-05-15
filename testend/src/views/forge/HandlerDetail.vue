<script setup lang="ts">
/**
 * Handler detail — subtabs:
 *   Definition  → init/shutdown bodies, methods, init-args schema
 *   Versions    → version history
 *   Pending     → pending diff + accept/reject
 *   Config      → init-args config get/post/clear
 *   Call        → invoke a method
 *   Calls (log) → D22 handler_calls table for this handler
 */
import { onMounted, ref, watch, computed } from 'vue';
import { useRouter } from 'vue-router';
import { hdAPI } from '@/api/handlers';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import SubTabBar from '@/components/common/SubTabBar.vue';
import { timeAgo, duration, pretty, shortID, statusClass } from '@/utils/format';
import type { Handler, HandlerVersion, ExecutionRow } from '@/types/domain';

const props = defineProps<{ id: string }>();
const router = useRouter();
const ui = useUIStore();

const hd = ref<Handler | null>(null);
const activeVersion = ref<HandlerVersion | null>(null);
const versions = ref<HandlerVersion[]>([]);
const pending = ref<HandlerVersion | null>(null);
const calls = ref<ExecutionRow[]>([]);
const config = ref<{ configState: string; config: Record<string, unknown> } | null>(null);
const loading = ref(false);
const subtab = ref<'def' | 'versions' | 'pending' | 'config' | 'call' | 'calls'>('def');

const configDraft = ref('{}');
const callDraft = ref({ method: '', args: '{}' });
const callResult = ref<unknown>(null);
const callErr = ref<string | null>(null);
const callBusy = ref(false);

async function refresh() {
  loading.value = true;
  try {
    hd.value = await hdAPI.get(props.id);
    if (hd.value.activeVersionId) {
      activeVersion.value = await hdAPI.getVersion(props.id, hd.value.activeVersionId);
    }
    const vs = await hdAPI.versions(props.id);
    versions.value = vs.items;
    try {
      pending.value = await hdAPI.pending(props.id);
    } catch {
      pending.value = null;
    }
    try {
      config.value = await hdAPI.getConfig(props.id);
      configDraft.value = pretty(config.value?.config ?? {});
    } catch {
      config.value = null;
    }
    try {
      const r = await hdAPI.calls(props.id);
      calls.value = r.calls ?? [];
    } catch {
      calls.value = [];
    }
  } catch (e) {
    ui.toast('err', `加载 handler 失败: ${(e as Error).message}`);
  } finally {
    loading.value = false;
  }
}

watch(() => props.id, refresh, { immediate: true });

const tabs = computed(() => [
  { id: 'def', label: 'Definition' },
  { id: 'versions', label: 'Versions', badge: versions.value.length },
  { id: 'pending', label: 'Pending', badge: pending.value ? '●' : 0 },
  { id: 'config', label: `Config (${config.value?.configState ?? '?'})` },
  { id: 'call', label: 'Call' },
  { id: 'calls', label: 'Call log', badge: calls.value.length },
]);

async function saveConfig() {
  try {
    const parsed = JSON.parse(configDraft.value);
    await hdAPI.updateConfig(props.id, parsed);
    ui.toast('ok', 'config 已保存');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function clearConfig() {
  if (!confirm('清空 handler config?')) return;
  try {
    await hdAPI.clearConfig(props.id);
    ui.toast('ok', 'config 已清空');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function doCall() {
  callBusy.value = true;
  callErr.value = null;
  try {
    const args = JSON.parse(callDraft.value.args);
    callResult.value = await hdAPI.call(props.id, callDraft.value.method, args);
  } catch (e) {
    callErr.value = (e as Error).message;
  } finally {
    callBusy.value = false;
  }
}

async function doAccept() {
  try { await hdAPI.acceptPending(props.id); ui.toast('ok', '已接受'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
async function doReject() {
  try { await hdAPI.rejectPending(props.id); ui.toast('ok', '已拒绝'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
async function doRevert(v: number | undefined) {
  if (v === undefined) return;
  if (!confirm(`Revert to v${v}?`)) return;
  try { await hdAPI.revert(props.id, v); ui.toast('ok', `已回退 v${v}`); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
</script>

<template>
  <div class="view">
    <ViewHeader v-if="hd" :title="hd.name" :subtitle="`${shortID(hd.id, 12)} · ${hd.description || '(no description)'}`">
      <template #actions>
        <span v-if="hd.envStatus" class="pill" :class="statusClass(hd.envStatus)">env: {{ hd.envStatus }}</span>
        <button class="btn ghost sm" @click="ui.showRaw(hd.name, hd)">raw</button>
        <button class="btn ghost sm" @click="router.back()">← back</button>
      </template>
    </ViewHeader>
    <ViewHeader v-else title="loading..." subtitle="" />

    <SubTabBar v-model="subtab" :tabs="tabs" />

    <div class="scroll">
      <!-- DEFINITION -->
      <section v-if="subtab === 'def' && activeVersion" class="section">
        <div class="meta-row">
          <span class="dim xs">active v{{ activeVersion.version }}</span>
          <span class="pill" :class="statusClass(activeVersion.status)">{{ activeVersion.status }}</span>
        </div>
        <h4>imports</h4>
        <pre class="code-block mono">{{ activeVersion.imports || '(none)' }}</pre>
        <h4>init body</h4>
        <pre class="code-block mono">{{ activeVersion.initBody || '(empty)' }}</pre>
        <h4>shutdown body</h4>
        <pre class="code-block mono">{{ activeVersion.shutdownBody || '(empty)' }}</pre>
        <h4>methods</h4>
        <pre class="code-block mono">{{ pretty(activeVersion.methods) }}</pre>
        <h4>init args schema</h4>
        <pre class="code-block mono">{{ pretty((activeVersion as any).initArgsSchema ?? []) }}</pre>
        <h4>dependencies</h4>
        <pre class="code-block mono">{{ pretty(activeVersion.dependencies) }}</pre>
      </section>

      <!-- VERSIONS -->
      <section v-if="subtab === 'versions'" class="section">
        <table class="table">
          <thead><tr><th>v</th><th>status</th><th>methods</th><th>env</th><th>created</th><th></th></tr></thead>
          <tbody>
            <tr v-for="v in versions" :key="v.id">
              <td class="mono">{{ v.version ?? '?' }}</td>
              <td><span class="pill" :class="statusClass(v.status)">{{ v.status }}</span></td>
              <td class="mono xs">{{ (v.methods ?? []).map((m: any) => m.name).join(', ') || '—' }}</td>
              <td><span v-if="v.envStatus" class="pill" :class="statusClass(v.envStatus)">{{ v.envStatus }}</span></td>
              <td class="dim xs">{{ timeAgo(v.createdAt) }}</td>
              <td>
                <button v-if="v.status === 'accepted' && v.id !== hd?.activeVersionId" class="btn ghost sm" @click="doRevert(v.version)">revert</button>
                <button class="btn ghost sm" @click="ui.showRaw(`v${v.version}`, v)">raw</button>
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- PENDING -->
      <section v-if="subtab === 'pending'" class="section">
        <div v-if="pending">
          <div class="meta-row">
            <span class="pill warn">pending</span>
            <button class="btn primary sm" @click="doAccept">Accept</button>
            <button class="btn danger sm" @click="doReject">Reject</button>
          </div>
          <h4>init body</h4>
          <pre class="code-block mono">{{ pending.initBody }}</pre>
          <h4>methods</h4>
          <pre class="code-block mono">{{ pretty(pending.methods) }}</pre>
        </div>
        <div v-else class="dim">No pending version.</div>
      </section>

      <!-- CONFIG -->
      <section v-if="subtab === 'config'" class="section">
        <p class="dim small">
          configState: <span class="mono">{{ config?.configState ?? 'unknown' }}</span>
        </p>
        <textarea v-model="configDraft" class="mono" rows="8" />
        <div class="row-actions">
          <button class="btn primary" @click="saveConfig">Save config</button>
          <button class="btn danger" @click="clearConfig">Clear</button>
        </div>
      </section>

      <!-- CALL -->
      <section v-if="subtab === 'call'" class="section">
        <label class="field-label">method</label>
        <select v-model="callDraft.method">
          <option value="" disabled>选择 method</option>
          <option v-for="m in (activeVersion?.methods ?? [])" :key="m.name" :value="m.name">{{ m.name }}</option>
        </select>
        <label class="field-label">args (JSON)</label>
        <textarea v-model="callDraft.args" class="mono" rows="6" />
        <div class="row-actions">
          <button class="btn primary" :disabled="callBusy || !callDraft.method" @click="doCall">{{ callBusy ? '...' : 'Call' }}</button>
        </div>
        <div v-if="callErr" class="error">{{ callErr }}</div>
        <div v-if="callResult !== null">
          <h4>result</h4>
          <pre class="code-block mono">{{ pretty(callResult) }}</pre>
        </div>
      </section>

      <!-- CALLS LOG -->
      <section v-if="subtab === 'calls'" class="section">
        <table class="table">
          <thead><tr><th>id</th><th>method</th><th>status</th><th>elapsed</th><th>at</th><th></th></tr></thead>
          <tbody>
            <tr v-for="c in calls" :key="c.id">
              <td class="mono xs">{{ shortID(c.id, 10) }}</td>
              <td class="mono xs">{{ (c as any).methodName ?? (c as any).method ?? '—' }}</td>
              <td><span class="pill" :class="statusClass(c.status)">{{ c.status }}</span></td>
              <td class="mono xs">{{ duration(c.elapsedMs) }}</td>
              <td class="dim xs">{{ timeAgo(c.startedAt) }}</td>
              <td><button class="btn ghost sm" @click="ui.showRaw(c.id, c)">raw</button></td>
            </tr>
            <tr v-if="calls.length === 0"><td colspan="6" class="empty-row">No calls.</td></tr>
          </tbody>
        </table>
      </section>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); }
.section { display: flex; flex-direction: column; gap: var(--sp-2); margin-bottom: var(--sp-4); }
.section h4 { margin: var(--sp-2) 0 0; font-size: var(--fs-xs); text-transform: uppercase; letter-spacing: 0.05em; color: var(--fg-2); }
.meta-row { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; padding-bottom: var(--sp-2); border-bottom: 1px solid var(--border-1); }
.row-actions { display: flex; gap: var(--sp-2); }
.field-label { font-size: var(--fs-xs); text-transform: uppercase; color: var(--fg-2); }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-4) 0; font-style: italic; }
textarea { font-family: var(--font-mono); min-height: 100px; resize: vertical; }
</style>
