<script setup lang="ts">
/**
 * Tools registry — all system tools the LLM agent can call. Sourced from
 * `GET /dev/tools` (raw list with {name, desc}). Allows direct invoke via
 * `POST /dev/invoke` for smoke testing without going through the agent.
 */
import { onMounted, ref, computed } from 'vue';
import { devAPI } from '@/api/dev';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { duration, pretty } from '@/utils/format';

const ui = useUIStore();

interface ToolRow {
  name: string;
  desc: string;
}

const tools = ref<ToolRow[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);
const filter = ref('');

const selected = ref<string | null>(null);
const argsDraft = ref('{}');
const runResult = ref<{ ok: boolean; output: string; elapsedMs: number; error?: string } | null>(null);
const runBusy = ref(false);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    tools.value = await devAPI.tools();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase();
  if (!q) return tools.value;
  return tools.value.filter((t) => t.name.toLowerCase().includes(q) || t.desc.toLowerCase().includes(q));
});

async function doInvoke() {
  if (!selected.value) return;
  runBusy.value = true;
  runResult.value = null;
  try {
    runResult.value = await devAPI.invoke(selected.value, argsDraft.value);
  } catch (e) {
    runResult.value = { ok: false, output: '', elapsedMs: 0, error: (e as Error).message };
  } finally {
    runBusy.value = false;
  }
}
</script>

<template>
  <div class="view">
    <ViewHeader title="Tools registry" :subtitle="`${tools.length} system tools wired to agent`">
      <template #actions>
        <input v-model="filter" placeholder="filter…" class="sm" />
        <button class="btn ghost sm" :disabled="loading" @click="refresh">{{ loading ? '...' : 'refresh' }}</button>
      </template>
    </ViewHeader>
    <div class="split">
      <div class="col-list scroll">
        <div v-if="err" class="error">⨯ {{ err }}</div>
        <ul class="tool-list">
          <li
            v-for="t in filtered"
            :key="t.name"
            class="tool-item"
            :class="{ active: selected === t.name }"
            @click="(selected = t.name) && (argsDraft = '{}') && (runResult = null)"
          >
            <div class="tool-name mono">{{ t.name }}</div>
            <div class="tool-desc dim small">{{ t.desc }}</div>
          </li>
        </ul>
      </div>
      <div class="col-detail scroll">
        <div v-if="!selected" class="dim center">Select a tool to invoke.</div>
        <div v-else class="invoker">
          <h3>{{ selected }}</h3>
          <label class="field-label">args (JSON)</label>
          <textarea v-model="argsDraft" class="mono" rows="8" />
          <div class="row-actions">
            <button class="btn primary" :disabled="runBusy" @click="doInvoke">{{ runBusy ? '...' : 'Invoke' }}</button>
            <button class="btn ghost" @click="ui.showRaw(selected, { name: selected, args: argsDraft, result: runResult })">raw</button>
          </div>
          <div v-if="runResult" class="result">
            <div class="result-head">
              <span class="pill" :class="runResult.ok ? 'ok' : 'err'">{{ runResult.ok ? 'ok' : 'err' }}</span>
              <span class="mono xs dim">{{ duration(runResult.elapsedMs) }}</span>
            </div>
            <div v-if="runResult.error" class="error">{{ runResult.error }}</div>
            <pre class="code-block mono">{{ pretty(runResult.output) }}</pre>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.split { flex: 1; display: flex; min-height: 0; }
.col-list {
  width: 320px;
  border-right: 1px solid var(--border-1);
  flex-shrink: 0;
}
.col-detail { flex: 1; padding: var(--sp-3); }
.scroll { overflow: auto; }
.tool-list { list-style: none; margin: 0; padding: 0; }
.tool-item {
  padding: var(--sp-2);
  border-bottom: 1px solid var(--border-1);
  cursor: pointer;
}
.tool-item:hover { background: var(--bg-hover); }
.tool-item.active { background: var(--bg-active); border-left: 2px solid var(--accent); }
.tool-name { font-weight: 600; font-size: var(--fs-sm); }
.tool-desc { margin-top: 2px; }
.center { padding: var(--sp-6); text-align: center; }
.field-label { font-size: var(--fs-xs); text-transform: uppercase; color: var(--fg-2); }
.row-actions { display: flex; gap: var(--sp-2); margin: var(--sp-2) 0; }
.result { margin-top: var(--sp-3); }
.result-head { display: flex; gap: var(--sp-2); align-items: center; margin-bottom: var(--sp-1); }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
textarea { font-family: var(--font-mono); width: 100%; }
</style>
