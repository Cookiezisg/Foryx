<script setup lang="ts">
/**
 * Workflow detail — subtabs:
 *   Graph     → cytoscape DAG render of the active version
 *   Definition→ raw graph JSON + variables list
 *   Versions  → history
 *   Pending   → accept/reject pending version
 *   Triggers  → active triggers (cron/fsnotify/webhook/manual)
 *   Trigger   → manual one-shot trigger with input JSON
 */
import { onMounted, ref, watch, computed } from 'vue';
import { useRouter } from 'vue-router';
import { wfAPI } from '@/api/workflows';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import SubTabBar from '@/components/common/SubTabBar.vue';
import GraphView from '@/components/forge/GraphView.vue';
import { pretty, shortID, statusClass, timeAgo } from '@/utils/format';
import type { Workflow, WorkflowVersion, Graph, TriggerState } from '@/types/domain';

const props = defineProps<{ id: string }>();
const router = useRouter();
const ui = useUIStore();

const wf = ref<Workflow | null>(null);
const activeVersion = ref<WorkflowVersion | null>(null);
const activeGraph = ref<Graph | null>(null);
const versions = ref<WorkflowVersion[]>([]);
const pending = ref<WorkflowVersion | null>(null);
const triggers = ref<TriggerState[]>([]);
const loading = ref(false);
const subtab = ref<'graph' | 'def' | 'versions' | 'pending' | 'triggers' | 'trigger'>('graph');

const triggerInput = ref('{}');
const triggerErr = ref<string | null>(null);
const triggerBusy = ref(false);

function parseGraph(jsonOrObj: string | Graph | undefined | null): Graph | null {
  if (!jsonOrObj) return null;
  if (typeof jsonOrObj === 'string') {
    try {
      return JSON.parse(jsonOrObj) as Graph;
    } catch {
      return null;
    }
  }
  return jsonOrObj;
}

async function refresh() {
  loading.value = true;
  try {
    wf.value = await wfAPI.get(props.id);
    if (wf.value.activeVersionId) {
      activeVersion.value = await wfAPI.getVersion(props.id, wf.value.activeVersionId);
      activeGraph.value = parseGraph(activeVersion.value.graphParsed ?? activeVersion.value.graph);
    }
    const vs = await wfAPI.versions(props.id);
    versions.value = vs.items;
    try {
      pending.value = await wfAPI.pending(props.id);
    } catch {
      pending.value = null;
    }
    try {
      triggers.value = await wfAPI.triggerStates(props.id);
    } catch {
      triggers.value = [];
    }
  } catch (e) {
    ui.toast('err', `加载 workflow 失败: ${(e as Error).message}`);
  } finally {
    loading.value = false;
  }
}

watch(() => props.id, refresh, { immediate: true });

const tabs = computed(() => [
  { id: 'graph', label: 'Graph' },
  { id: 'def', label: 'Definition' },
  { id: 'versions', label: 'Versions', badge: versions.value.length },
  { id: 'pending', label: 'Pending', badge: pending.value ? '●' : 0 },
  { id: 'triggers', label: 'Triggers', badge: triggers.value.length },
  { id: 'trigger', label: 'Trigger' },
]);

async function doTrigger() {
  triggerBusy.value = true;
  triggerErr.value = null;
  try {
    const input = JSON.parse(triggerInput.value);
    const r = await wfAPI.trigger(props.id, input);
    ui.toast('ok', `已触发 run ${shortID(r.runId, 8)}`);
  } catch (e) {
    triggerErr.value = (e as Error).message;
  } finally {
    triggerBusy.value = false;
  }
}

async function doAccept() {
  try { await wfAPI.acceptPending(props.id); ui.toast('ok', '已接受'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
async function doReject() {
  try { await wfAPI.rejectPending(props.id); ui.toast('ok', '已拒绝'); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
async function doRevert(v: number | undefined) {
  if (v === undefined) return;
  if (!confirm(`Revert workflow to v${v}?`)) return;
  try { await wfAPI.revert(props.id, v); ui.toast('ok', `已回退 v${v}`); await refresh(); }
  catch (e) { ui.toast('err', (e as Error).message); }
}
</script>

<template>
  <div class="view">
    <ViewHeader v-if="wf" :title="wf.name" :subtitle="`${shortID(wf.id, 12)} · ${wf.description || '(no description)'}`">
      <template #actions>
        <span class="pill" :class="wf.enabled ? 'ok' : ''">{{ wf.enabled ? 'enabled' : 'disabled' }}</span>
        <button class="btn ghost sm" @click="ui.showRaw(wf.name, wf)">raw</button>
        <button class="btn ghost sm" @click="router.back()">← back</button>
      </template>
    </ViewHeader>
    <ViewHeader v-else title="loading..." subtitle="" />

    <SubTabBar v-model="subtab" :tabs="tabs" />

    <div class="scroll">
      <!-- GRAPH -->
      <section v-if="subtab === 'graph'" class="section graph-wrap">
        <div v-if="!activeGraph" class="dim center">No graph available.</div>
        <GraphView v-else :graph="activeGraph" />
      </section>

      <!-- DEFINITION -->
      <section v-if="subtab === 'def'" class="section">
        <h4>variables</h4>
        <pre class="code-block mono">{{ pretty(activeGraph?.variables ?? []) }}</pre>
        <h4>nodes ({{ (activeGraph?.nodes ?? []).length }})</h4>
        <pre class="code-block mono">{{ pretty(activeGraph?.nodes) }}</pre>
        <h4>edges ({{ (activeGraph?.edges ?? []).length }})</h4>
        <pre class="code-block mono">{{ pretty(activeGraph?.edges) }}</pre>
      </section>

      <!-- VERSIONS -->
      <section v-if="subtab === 'versions'" class="section">
        <table class="table">
          <thead><tr><th>v</th><th>status</th><th>changeReason</th><th>created</th><th></th></tr></thead>
          <tbody>
            <tr v-for="v in versions" :key="v.id">
              <td class="mono">{{ v.version ?? '?' }}</td>
              <td><span class="pill" :class="statusClass(v.status)">{{ v.status }}</span></td>
              <td>{{ v.changeReason || '—' }}</td>
              <td class="dim xs">{{ timeAgo(v.createdAt) }}</td>
              <td>
                <button v-if="v.status === 'accepted' && v.id !== wf?.activeVersionId" class="btn ghost sm" @click="doRevert(v.version)">revert</button>
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
            <span class="dim xs">{{ pending.changeReason || '(no reason)' }}</span>
            <button class="btn primary sm" @click="doAccept">Accept</button>
            <button class="btn danger sm" @click="doReject">Reject</button>
          </div>
          <h4>graph (proposed)</h4>
          <pre class="code-block mono">{{ pending.graph }}</pre>
        </div>
        <div v-else class="dim">No pending version.</div>
      </section>

      <!-- TRIGGERS -->
      <section v-if="subtab === 'triggers'" class="section">
        <table class="table">
          <thead><tr><th>kind</th><th>spec</th><th>status</th><th>next fire</th><th>last fire</th></tr></thead>
          <tbody>
            <tr v-for="t in triggers" :key="`${t.kind}-${t.nodeId}`">
              <td><span class="pill info">{{ t.kind }}</span></td>
              <td class="mono ellipsis-cell">{{ t.nodeId }}</td>
              <td><span class="pill" :class="statusClass(t.status)">{{ t.status }}</span></td>
              <td class="dim xs">{{ t.nextFireAt ? timeAgo(t.nextFireAt) : '—' }}</td>
              <td class="dim xs">{{ t.lastFiredAt ? timeAgo(t.lastFiredAt) : '—' }}</td>
            </tr>
            <tr v-if="triggers.length === 0"><td colspan="5" class="empty-row">No triggers registered.</td></tr>
          </tbody>
        </table>
      </section>

      <!-- TRIGGER (manual) -->
      <section v-if="subtab === 'trigger'" class="section">
        <p class="dim small">Manually fire this workflow with a JSON input. Returns a flowrun id.</p>
        <textarea v-model="triggerInput" rows="6" class="mono" />
        <div class="row-actions">
          <button class="btn primary" :disabled="triggerBusy" @click="doTrigger">{{ triggerBusy ? '...' : 'Trigger' }}</button>
        </div>
        <div v-if="triggerErr" class="error">{{ triggerErr }}</div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); display: flex; flex-direction: column; }
.section { margin-bottom: var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-2); }
.section h4 { margin: var(--sp-2) 0 0; font-size: var(--fs-xs); text-transform: uppercase; letter-spacing: 0.05em; color: var(--fg-2); }
.graph-wrap { flex: 1; min-height: 500px; }
.meta-row { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; padding-bottom: var(--sp-2); border-bottom: 1px solid var(--border-1); }
.row-actions { display: flex; gap: var(--sp-2); }
.ellipsis-cell { max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-4) 0; font-style: italic; }
.center { padding: var(--sp-6); text-align: center; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
textarea { font-family: var(--font-mono); min-height: 100px; resize: vertical; }
</style>
