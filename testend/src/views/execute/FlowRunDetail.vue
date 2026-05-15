<script setup lang="ts">
/**
 * FlowRun detail — node-by-node timeline + paused state + cancel/approve.
 * Sourced from `/api/v1/flowruns/{id}` and `/api/v1/flowruns/{id}/nodes`.
 */
import { onMounted, ref, watch, computed } from 'vue';
import { useRouter } from 'vue-router';
import { flowrunAPI } from '@/api/flowruns';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import SubTabBar from '@/components/common/SubTabBar.vue';
import { timeAgo, duration, pretty, shortID, statusClass } from '@/utils/format';
import type { FlowRun, FlowRunNode } from '@/types/domain';

const props = defineProps<{ id: string }>();
const router = useRouter();
const ui = useUIStore();

const run = ref<FlowRun | null>(null);
const nodes = ref<FlowRunNode[]>([]);
const subtab = ref<'timeline' | 'output' | 'paused' | 'input'>('timeline');

const approveDraft = ref('{}');

async function refresh() {
  try {
    run.value = await flowrunAPI.get(props.id);
    const r = await flowrunAPI.nodes(props.id, { limit: 200 });
    nodes.value = r.items;
  } catch (e) {
    ui.toast('err', `加载 flowrun 失败: ${(e as Error).message}`);
  }
}

watch(() => props.id, refresh, { immediate: true });

const tabs = computed(() => [
  { id: 'timeline', label: 'Timeline', badge: nodes.value.length },
  { id: 'output', label: 'Output' },
  { id: 'paused', label: 'Paused state', badge: run.value?.status === 'paused' ? '●' : 0 },
  { id: 'input', label: 'Trigger input' },
]);

async function doCancel() {
  if (!confirm(`Cancel flowrun ${shortID(props.id, 10)}?`)) return;
  try {
    await flowrunAPI.cancel(props.id);
    ui.toast('ok', '已取消');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function approveNode(nodeId: string, decision: 'approved' | 'rejected') {
  try {
    await flowrunAPI.approve(props.id, nodeId, decision, decision === 'rejected' ? approveDraft.value : undefined);
    ui.toast('ok', `${decision}`);
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}
</script>

<template>
  <div class="view">
    <ViewHeader v-if="run" :title="`FlowRun ${shortID(run.id, 10)}`" :subtitle="`workflow ${shortID(run.workflowId, 10)} · ${run.triggerKind}`">
      <template #actions>
        <span class="pill" :class="statusClass(run.status)">{{ run.status }}</span>
        <span class="mono xs dim">{{ duration(run.elapsedMs) }}</span>
        <button v-if="run.status === 'running' || run.status === 'paused'" class="btn danger sm" @click="doCancel">Cancel</button>
        <button class="btn ghost sm" @click="ui.showRaw(run.id, run)">raw</button>
        <button class="btn ghost sm" @click="router.back()">← back</button>
      </template>
    </ViewHeader>

    <SubTabBar v-model="subtab" :tabs="tabs" />

    <div class="scroll">
      <!-- TIMELINE -->
      <section v-if="subtab === 'timeline'" class="section">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 200px">node</th>
              <th style="width: 80px">type</th>
              <th style="width: 90px">status</th>
              <th style="width: 80px">elapsed</th>
              <th style="width: 100px">started</th>
              <th>error</th>
              <th style="width: 100px"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="n in nodes" :key="n.id">
              <td class="mono xs">{{ n.nodeId }}</td>
              <td><span class="pill info">{{ n.nodeType }}</span></td>
              <td><span class="pill" :class="statusClass(n.status)">{{ n.status }}</span></td>
              <td class="mono xs">{{ duration(n.elapsedMs) }}</td>
              <td class="dim xs">{{ timeAgo(n.startedAt) }}</td>
              <td class="ellipsis-cell">{{ n.errorMessage || '—' }}</td>
              <td>
                <template v-if="run?.status === 'paused' && run.pausedState?.nodeId === n.nodeId">
                  <button class="btn primary sm" @click="approveNode(n.nodeId, 'approved')">approve</button>
                  <button class="btn danger sm" @click="approveNode(n.nodeId, 'rejected')">reject</button>
                </template>
                <button class="btn ghost sm" @click="ui.showRaw(n.nodeId, n)">raw</button>
              </td>
            </tr>
            <tr v-if="nodes.length === 0">
              <td colspan="7" class="empty-row">No node entries yet.</td>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- OUTPUT -->
      <section v-if="subtab === 'output'" class="section">
        <pre v-if="run?.output !== undefined" class="code-block mono">{{ pretty(run.output) }}</pre>
        <div v-else class="dim center">No output yet (run still active or didn't produce a final value).</div>
        <div v-if="run?.errorMessage" class="error">{{ run.errorCode }}: {{ run.errorMessage }}</div>
      </section>

      <!-- PAUSED STATE -->
      <section v-if="subtab === 'paused'" class="section">
        <div v-if="run?.pausedState">
          <h4>at node</h4>
          <pre class="code-block mono">{{ run.pausedState.nodeId }}</pre>
          <h4>variables</h4>
          <pre class="code-block mono">{{ pretty(run.pausedState.variables) }}</pre>
          <h4>outputs accumulated</h4>
          <pre class="code-block mono">{{ pretty(run.pausedState.outputs) }}</pre>
          <h4>position trail</h4>
          <pre class="code-block mono">{{ pretty(run.pausedState.position) }}</pre>
        </div>
        <div v-else class="dim center">Run is not paused.</div>
      </section>

      <!-- INPUT -->
      <section v-if="subtab === 'input'" class="section">
        <pre class="code-block mono">{{ pretty(run?.triggerInput ?? {}) }}</pre>
      </section>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); }
.section { margin-bottom: var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-2); }
.section h4 { margin: var(--sp-2) 0 0; font-size: var(--fs-xs); text-transform: uppercase; letter-spacing: 0.05em; color: var(--fg-2); }
.ellipsis-cell { max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--fs-xs); }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-4) 0; font-style: italic; }
.center { padding: var(--sp-6); text-align: center; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
