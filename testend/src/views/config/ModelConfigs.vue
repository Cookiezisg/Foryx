<script setup lang="ts">
/**
 * Model configs — per-scenario provider+model picks (idempotent upsert).
 * Backend currently exposes 2 scenarios: `chat` and `web_summary`. Each
 * scenario maps to one (provider, modelId). Provider must match an existing
 * api-key on this user's account.
 */
import { onMounted, ref, computed } from 'vue';
import { modelAPI, apikeyAPI } from '@/api/resources';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo } from '@/utils/format';
import type { ModelConfig, APIKey } from '@/types/domain';

const ui = useUIStore();
const items = ref<ModelConfig[]>([]);
const apiKeys = ref<APIKey[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

/** Backend's hardcoded scenario whitelist. Stays small — extend when backend adds. */
const SCENARIOS = ['chat', 'web_summary'] as const;

/** Per-scenario edit drafts. */
const drafts = ref<Record<string, { provider: string; modelId: string }>>({});

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    items.value = await modelAPI.list();
    apiKeys.value = await apikeyAPI.list();
    for (const s of SCENARIOS) {
      const cur = items.value.find((m) => m.scenario === s);
      drafts.value[s] = drafts.value[s] ?? {
        provider: cur?.provider ?? '',
        modelId: cur?.modelId ?? '',
      };
    }
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

const availableProviders = computed(() => {
  const seen = new Set<string>();
  for (const k of apiKeys.value) seen.add(k.provider);
  return Array.from(seen).sort();
});

function modelsForProvider(p: string): string[] {
  const k = apiKeys.value.find((x) => x.provider === p);
  return k?.modelsFound ?? [];
}

async function save(scenario: string) {
  const d = drafts.value[scenario];
  try {
    await modelAPI.upsert(scenario, { provider: d.provider, modelId: d.modelId });
    ui.toast('ok', `${scenario} 已保存`);
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

function existing(scenario: string): ModelConfig | undefined {
  return items.value.find((m) => m.scenario === scenario);
}
</script>

<template>
  <div class="view">
    <ViewHeader title="Model configs" :subtitle="`${SCENARIOS.length} scenarios · ${items.length} configured`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <article v-for="s in SCENARIOS" :key="s" class="card">
        <header class="head">
          <strong>{{ s }}</strong>
          <span v-if="existing(s)" class="pill ok">configured</span>
          <span v-else class="pill warn">unset</span>
          <span class="spacer" />
          <button class="btn ghost sm" v-if="existing(s)" @click="ui.showRaw(s, existing(s))">raw</button>
        </header>
        <div class="grid">
          <label class="field-label">provider</label>
          <select v-model="drafts[s].provider">
            <option value="" disabled>— pick from your api-keys —</option>
            <option v-for="p in availableProviders" :key="p" :value="p">{{ p }}</option>
          </select>

          <label class="field-label">modelId</label>
          <select v-model="drafts[s].modelId">
            <option value="" disabled>— pick discovered model —</option>
            <option v-for="m in modelsForProvider(drafts[s].provider)" :key="m" :value="m">{{ m }}</option>
          </select>
        </div>

        <div class="row-actions">
          <button class="btn primary sm" :disabled="!drafts[s].provider || !drafts[s].modelId" @click="save(s)">Save</button>
          <span v-if="existing(s)" class="dim xs">updated {{ timeAgo(existing(s)!.updatedAt) }}</span>
        </div>
      </article>

      <p v-if="availableProviders.length === 0" class="dim">
        No api keys yet. Add one via <strong>Config › API Keys</strong> first.
      </p>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-3); }
.card { border: 1px solid var(--border-1); border-radius: var(--radius-md); padding: var(--sp-3); background: var(--bg-1); }
.head { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: var(--sp-2); }
.spacer { flex: 1; }
.grid { display: grid; grid-template-columns: 140px 1fr; gap: var(--sp-1) var(--sp-2); align-items: center; }
.grid select { width: 100%; }
.field-label { font-size: var(--fs-xs); color: var(--fg-2); justify-self: end; text-align: right; }
.row-actions { display: flex; gap: var(--sp-2); align-items: center; margin-top: var(--sp-2); }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
