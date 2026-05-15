<script setup lang="ts">
/**
 * Approvals queue — flowruns paused at an `approval` node, awaiting human
 * decision. Sourced from `/api/v1/flowruns?status=paused` then filtered to
 * those whose `pausedState.nodeId` indicates an approval gate.
 *
 * Approving a node: `POST /api/v1/flowruns/{id}/approvals/{nodeId}` with
 * `{decision: "approved"|"rejected", reason?}` — runs the rest of the DAG.
 */
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { flowrunAPI } from '@/api/flowruns';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, shortID, statusClass, pretty } from '@/utils/format';
import type { FlowRun } from '@/types/domain';

const router = useRouter();
const ui = useUIStore();

const items = ref<FlowRun[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);
const reasonByRun = ref<Record<string, string>>({});

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    const page = await flowrunAPI.list({ status: 'paused', limit: 200 });
    items.value = page.items;
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

async function decide(run: FlowRun, decision: 'approved' | 'rejected') {
  const nodeId = run.pausedState?.nodeId;
  if (!nodeId) {
    ui.toast('err', '该 run 没有 pausedState.nodeId');
    return;
  }
  try {
    await flowrunAPI.approve(run.id, nodeId, decision, reasonByRun.value[run.id]);
    ui.toast('ok', `${decision}: ${shortID(run.id, 8)}`);
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

function openDetail(id: string) {
  router.push(`/execute/flowruns/${id}`);
}
</script>

<template>
  <div class="view">
    <ViewHeader title="Approvals queue" :subtitle="`${items.length} runs awaiting approval`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">{{ loading ? '...' : 'refresh' }}</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <div v-if="!loading && items.length === 0" class="empty">
        <div class="empty-title">Queue clear</div>
        <div class="empty-hint">No flowruns currently paused waiting for human approval.</div>
      </div>
      <article v-for="r in items" :key="r.id" class="approval-card">
        <header class="head">
          <span class="pill paused">paused @ {{ r.pausedState?.nodeId ?? 'unknown' }}</span>
          <span class="mono">{{ shortID(r.id, 12) }}</span>
          <span class="dim small">workflow {{ shortID(r.workflowId, 10) }} · {{ timeAgo(r.startedAt) }} ago</span>
          <span class="spacer" />
          <button class="btn ghost sm" @click="openDetail(r.id)">open</button>
          <button class="btn ghost sm" @click="ui.showRaw(r.id, r)">raw</button>
        </header>
        <details>
          <summary class="dim">accumulated variables + outputs</summary>
          <pre class="code-block mono">{{ pretty(r.pausedState ?? {}) }}</pre>
        </details>
        <div class="decide-row">
          <input
            placeholder="reason (optional, sent on reject)"
            v-model="reasonByRun[r.id]"
            class="reason-input"
          />
          <button class="btn primary" @click="decide(r, 'approved')">Approve</button>
          <button class="btn danger" @click="decide(r, 'rejected')">Reject</button>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-3); }
.approval-card { border: 1px solid var(--border-1); border-radius: var(--radius-md); padding: var(--sp-3); background: var(--bg-1); }
.head { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; margin-bottom: var(--sp-2); }
.spacer { flex: 1; }
.decide-row { display: flex; gap: var(--sp-2); margin-top: var(--sp-2); align-items: center; }
.reason-input { flex: 1; }
.empty { text-align: center; padding: var(--sp-6) 0; }
.empty-title { font-weight: 600; margin-bottom: var(--sp-1); }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
details summary { cursor: pointer; padding: 4px 0; font-size: var(--fs-xs); }
</style>
