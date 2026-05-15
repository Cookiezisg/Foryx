<script setup lang="ts">
/**
 * Mock LLM — push scripted responses to the mock backend (TE-4b) so a chat
 * conversation gets canned replies instead of hitting the real provider.
 * Useful for deterministic e2e tests + tool-call rehearsal.
 *
 * Endpoints (all only available when --dev):
 *   POST   /dev/mock-llm/scripts       push a list of scripts
 *   GET    /dev/mock-llm/queue         see what's queued
 *   DELETE /dev/mock-llm/scripts       clear queue
 *   GET    /dev/mock-llm/last-prompt   inspect the last prompt the agent sent
 */
import { onMounted, ref } from 'vue';
import { devAPI } from '@/api/dev';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { pretty } from '@/utils/format';

const ui = useUIStore();

const draft = ref(`[
  { "kind": "text", "content": "Hello from mock LLM" }
]`);
const queue = ref<{ scripts: unknown[]; count: number } | null>(null);
const lastPrompt = ref<unknown>(null);
const busy = ref(false);
const err = ref<string | null>(null);

async function refresh() {
  err.value = null;
  try {
    queue.value = await devAPI.mockLLMQueue();
    lastPrompt.value = await devAPI.mockLLMLastPrompt();
  } catch (e) {
    err.value = (e as Error).message;
  }
}

async function push() {
  busy.value = true;
  err.value = null;
  try {
    const scripts = JSON.parse(draft.value);
    await devAPI.mockLLMPush(Array.isArray(scripts) ? scripts : [scripts]);
    ui.toast('ok', 'pushed');
    await refresh();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}

async function clear() {
  if (!confirm('清空 mock LLM 队列?')) return;
  try {
    await devAPI.mockLLMClear();
    ui.toast('ok', 'cleared');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

onMounted(refresh);
</script>

<template>
  <div class="view">
    <ViewHeader title="Mock LLM" subtitle="dev-only canned replies for testing">
      <template #actions>
        <button class="btn ghost sm" @click="refresh">refresh</button>
        <button class="btn danger sm" @click="clear">clear queue</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>

      <section class="section">
        <h4>queue ({{ queue?.count ?? 0 }} scripts)</h4>
        <pre class="code-block mono">{{ pretty(queue?.scripts ?? []) }}</pre>
      </section>

      <section class="section">
        <h4>push new scripts (JSON array)</h4>
        <textarea v-model="draft" rows="10" class="mono" />
        <div class="row-actions">
          <button class="btn primary" :disabled="busy" @click="push">{{ busy ? '...' : 'Push' }}</button>
        </div>
      </section>

      <section class="section">
        <h4>last prompt (what the agent sent last)</h4>
        <pre class="code-block mono">{{ pretty(lastPrompt) }}</pre>
      </section>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); }
.section { margin-bottom: var(--sp-4); display: flex; flex-direction: column; gap: var(--sp-2); }
.section h4 { margin: 0; font-size: var(--fs-xs); text-transform: uppercase; letter-spacing: 0.05em; color: var(--fg-2); }
.row-actions { display: flex; gap: var(--sp-2); }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
textarea { font-family: var(--font-mono); resize: vertical; }
</style>
