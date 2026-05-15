<script setup lang="ts">
/**
 * Function detail — subtabs:
 *   Definition  → code + parameters of the active version
 *   Versions    → version history (accepted / rejected)
 *   Pending     → pending accept/reject (D11 dual-version model)
 *   Run         → invoke :run with JSON args, see result
 *   Executions  → D22 execution log
 */
import { onMounted, ref, watch, computed } from 'vue';
import { useRouter } from 'vue-router';
import { fnAPI } from '@/api/functions';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import SubTabBar from '@/components/common/SubTabBar.vue';
import { timeAgo, duration, pretty, shortID, statusClass } from '@/utils/format';
import type { Function as Fn, FunctionVersion, ExecutionRow } from '@/types/domain';

const props = defineProps<{ id: string }>();
const router = useRouter();
const ui = useUIStore();

const fn = ref<Fn | null>(null);
const activeVersion = ref<FunctionVersion | null>(null);
const versions = ref<FunctionVersion[]>([]);
const pending = ref<FunctionVersion | null>(null);
const executions = ref<ExecutionRow[]>([]);
const loading = ref(false);
const subtab = ref<'def' | 'versions' | 'pending' | 'run' | 'execs'>('def');

/* Run panel state */
const runArgs = ref('{}');
const runResult = ref<unknown>(null);
const runErr = ref<string | null>(null);
const runBusy = ref(false);

async function refresh() {
  loading.value = true;
  try {
    fn.value = await fnAPI.get(props.id);
    if (fn.value.activeVersionId) {
      activeVersion.value = await fnAPI.getVersion(props.id, fn.value.activeVersionId);
    } else {
      activeVersion.value = null;
    }
    const vs = await fnAPI.versions(props.id);
    versions.value = vs.items;
    try {
      pending.value = await fnAPI.pending(props.id);
    } catch {
      pending.value = null;
    }
    try {
      const r = await fnAPI.executions(props.id);
      executions.value = r.executions ?? [];
    } catch {
      executions.value = [];
    }
  } catch (e) {
    ui.toast('err', `加载函数失败: ${(e as Error).message}`);
  } finally {
    loading.value = false;
  }
}

watch(() => props.id, refresh, { immediate: true });

const tabs = computed(() => [
  { id: 'def', label: 'Definition' },
  { id: 'versions', label: 'Versions', badge: versions.value.length },
  { id: 'pending', label: 'Pending', badge: pending.value ? '●' : 0 },
  { id: 'run', label: 'Run' },
  { id: 'execs', label: 'Executions', badge: executions.value.length },
]);

async function doRun() {
  runBusy.value = true;
  runErr.value = null;
  try {
    const args = JSON.parse(runArgs.value);
    runResult.value = await fnAPI.run(props.id, args);
  } catch (e) {
    runErr.value = (e as Error).message;
  } finally {
    runBusy.value = false;
  }
}

async function doAccept() {
  try {
    await fnAPI.acceptPending(props.id);
    ui.toast('ok', '已接受');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function doReject() {
  try {
    await fnAPI.rejectPending(props.id);
    ui.toast('ok', '已拒绝');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function doRevert(v: number | undefined) {
  if (v === undefined) return;
  if (!confirm(`Revert function to version ${v}?`)) return;
  try {
    await fnAPI.revert(props.id, v);
    ui.toast('ok', `已回退到 v${v}`);
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}
</script>

<template>
  <div class="view">
    <ViewHeader v-if="fn" :title="fn.name" :subtitle="`${shortID(fn.id, 12)} · ${fn.description || '(no description)'}`">
      <template #actions>
        <span v-if="fn.envStatus" class="pill" :class="statusClass(fn.envStatus)">env: {{ fn.envStatus }}</span>
        <button class="btn ghost sm" @click="ui.showRaw(fn.name, fn)">raw</button>
        <button class="btn ghost sm" @click="router.back()">← back</button>
      </template>
    </ViewHeader>
    <ViewHeader v-else title="loading..." subtitle="" />

    <SubTabBar v-model="subtab" :tabs="tabs" />

    <div class="scroll">
      <!-- DEFINITION -->
      <section v-if="subtab === 'def' && activeVersion" class="section">
        <div class="meta-row">
          <span class="dim xs">active version:</span>
          <span class="mono">v{{ activeVersion.version }}</span>
          <span class="pill" :class="statusClass(activeVersion.status)">{{ activeVersion.status }}</span>
          <span v-if="activeVersion.envStatus" class="pill" :class="statusClass(activeVersion.envStatus)">env: {{ activeVersion.envStatus }}</span>
          <span class="dim xs">created {{ timeAgo(activeVersion.createdAt) }}</span>
        </div>
        <h4>parameters</h4>
        <pre class="code-block mono">{{ pretty(activeVersion.parameters) }}</pre>
        <h4>dependencies</h4>
        <pre class="code-block mono">{{ pretty(activeVersion.dependencies) }}</pre>
        <h4>code</h4>
        <pre class="code-block mono">{{ activeVersion.code }}</pre>
        <div v-if="activeVersion.envError" class="error">env error: {{ activeVersion.envError }}</div>
      </section>

      <!-- VERSIONS -->
      <section v-if="subtab === 'versions'" class="section">
        <table class="table">
          <thead>
            <tr><th>v</th><th>status</th><th>changeReason</th><th>env</th><th>created</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="v in versions" :key="v.id">
              <td class="mono">{{ v.version ?? '?' }}</td>
              <td><span class="pill" :class="statusClass(v.status)">{{ v.status }}</span></td>
              <td>{{ v.changeReason || '—' }}</td>
              <td><span v-if="v.envStatus" class="pill" :class="statusClass(v.envStatus)">{{ v.envStatus }}</span></td>
              <td class="dim xs">{{ timeAgo(v.createdAt) }}</td>
              <td>
                <button v-if="v.status === 'accepted' && v.id !== fn?.activeVersionId" class="btn ghost sm" @click="doRevert(v.version)">revert</button>
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
            <span class="dim xs">changeReason: {{ pending.changeReason || '(none)' }}</span>
            <button class="btn primary sm" @click="doAccept">Accept</button>
            <button class="btn danger sm" @click="doReject">Reject</button>
          </div>
          <h4>code</h4>
          <pre class="code-block mono">{{ pending.code }}</pre>
          <h4>parameters</h4>
          <pre class="code-block mono">{{ pretty(pending.parameters) }}</pre>
        </div>
        <div v-else class="dim">No pending version. Use chat agent's <code>edit_function</code> tool to propose one.</div>
      </section>

      <!-- RUN -->
      <section v-if="subtab === 'run'" class="section">
        <p class="dim small">Body is the args JSON forwarded to <code>:run</code> action.</p>
        <textarea v-model="runArgs" rows="6" class="mono" />
        <div class="run-actions">
          <button class="btn primary" :disabled="runBusy" @click="doRun">{{ runBusy ? '...' : 'Run' }}</button>
        </div>
        <div v-if="runErr" class="error">{{ runErr }}</div>
        <div v-if="runResult !== null">
          <h4>result</h4>
          <pre class="code-block mono">{{ pretty(runResult) }}</pre>
        </div>
      </section>

      <!-- EXECUTIONS -->
      <section v-if="subtab === 'execs'" class="section">
        <table class="table">
          <thead>
            <tr><th>id</th><th>status</th><th>elapsed</th><th>trigger</th><th>at</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-for="e in executions" :key="e.id">
              <td class="mono xs">{{ shortID(e.id, 10) }}</td>
              <td><span class="pill" :class="statusClass(e.status)">{{ e.status }}</span></td>
              <td class="mono xs">{{ duration(e.elapsedMs) }}</td>
              <td class="mono xs">{{ e.trigger ?? '—' }}</td>
              <td class="dim xs">{{ timeAgo(e.startedAt) }}</td>
              <td><button class="btn ghost sm" @click="ui.showRaw(e.id, e)">raw</button></td>
            </tr>
            <tr v-if="executions.length === 0">
              <td colspan="6" class="empty-row">No executions recorded.</td>
            </tr>
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
.run-actions { display: flex; gap: var(--sp-2); }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); font-size: var(--fs-sm); }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-4) 0; font-style: italic; }
textarea { font-family: var(--font-mono); min-height: 100px; resize: vertical; }
</style>
