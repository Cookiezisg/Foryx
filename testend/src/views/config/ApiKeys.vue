<script setup lang="ts">
/**
 * API Keys — provider credentials. Create / delete / live-probe via :test.
 * Provider whitelist is sourced from /api/v1/providers; baseUrl is only
 * exposed for providers that require/allow one (per baseUrlRequired).
 */
import { onMounted, ref, computed } from 'vue';
import { apikeyAPI, providerAPI } from '@/api/resources';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, statusClass, shortID } from '@/utils/format';
import type { APIKey } from '@/types/domain';
import type { ProviderMeta } from '@/api/resources';

const ui = useUIStore();
const items = ref<APIKey[]>([]);
const providers = ref<ProviderMeta[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

const draft = ref({ provider: '', displayName: '', baseUrl: '', apiFormat: '', key: '' });

const showAdd = ref(false);

const llmProviders = computed(() => providers.value.filter((p) => p.category === 'llm'));
const otherProviders = computed(() => providers.value.filter((p) => p.category !== 'llm'));
const selectedProvider = computed(() =>
  providers.value.find((p) => p.name === draft.value.provider),
);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    items.value = await apikeyAPI.list();
    if (providers.value.length === 0) providers.value = await providerAPI.list();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

async function create() {
  try {
    await apikeyAPI.create({
      provider: draft.value.provider,
      displayName: draft.value.displayName || undefined,
      baseUrl: draft.value.baseUrl || undefined,
      apiFormat: draft.value.apiFormat || undefined,
      key: draft.value.key,
    });
    ui.toast('ok', 'api key 已创建');
    draft.value = { provider: '', displayName: '', baseUrl: '', apiFormat: '', key: '' };
    showAdd.value = false;
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function doTest(id: string) {
  try {
    const r = await apikeyAPI.test(id);
    ui.toast(r.ok ? 'ok' : 'err', r.message ?? (r.ok ? 'ok' : 'failed'));
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}

async function doDelete(id: string) {
  if (!confirm(`删除 api key ${shortID(id, 10)}?`)) return;
  try {
    await apikeyAPI.remove(id);
    ui.toast('ok', 'deleted');
    await refresh();
  } catch (e) {
    ui.toast('err', (e as Error).message);
  }
}
</script>

<template>
  <div class="view">
    <ViewHeader title="API Keys" :subtitle="`${items.length} keys configured`">
      <template #actions>
        <button class="btn primary sm" @click="showAdd = !showAdd">{{ showAdd ? '取消' : '+ Add' }}</button>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
      </template>
    </ViewHeader>

    <section v-if="showAdd" class="add-form">
      <h4>Add API key</h4>
      <label class="field-label">provider</label>
      <select v-model="draft.provider">
        <option value="" disabled>— select —</option>
        <optgroup label="LLM">
          <option v-for="p in llmProviders" :key="p.name" :value="p.name">{{ p.displayName }}</option>
        </optgroup>
        <optgroup label="Other">
          <option v-for="p in otherProviders" :key="p.name" :value="p.name">{{ p.displayName }}</option>
        </optgroup>
      </select>

      <label class="field-label">display name (optional)</label>
      <input v-model="draft.displayName" />

      <label v-if="selectedProvider?.baseUrlRequired || draft.baseUrl" class="field-label">
        base URL <span v-if="selectedProvider?.defaultBaseUrl" class="dim small">(default: {{ selectedProvider.defaultBaseUrl }})</span>
      </label>
      <input v-if="selectedProvider?.baseUrlRequired || draft.baseUrl" v-model="draft.baseUrl" :placeholder="selectedProvider?.defaultBaseUrl ?? ''" />

      <label class="field-label">apiFormat (optional)</label>
      <input v-model="draft.apiFormat" placeholder="openai / anthropic (auto-detected)" />

      <label class="field-label">key</label>
      <input v-model="draft.key" type="password" placeholder="sk-..." />

      <button class="btn primary" @click="create" :disabled="!draft.provider || !draft.key">Create</button>
    </section>

    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <table class="table">
        <thead>
          <tr>
            <th style="width: 140px">provider</th>
            <th>display</th>
            <th style="width: 180px">key</th>
            <th style="width: 80px">test</th>
            <th>models</th>
            <th style="width: 100px">tested</th>
            <th style="width: 160px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="k in items" :key="k.id">
            <td><span class="pill info">{{ k.provider }}</span></td>
            <td>
              <div>{{ k.displayName || '(no name)' }}</div>
              <div class="dim xs mono">{{ shortID(k.id, 10) }}</div>
            </td>
            <td class="mono xs">{{ k.keyMasked }}</td>
            <td><span class="pill" :class="statusClass(k.testStatus)">{{ k.testStatus || '?' }}</span></td>
            <td class="dim small">{{ (k.modelsFound ?? []).join(', ') || '—' }}</td>
            <td class="dim xs">{{ k.lastTestedAt ? timeAgo(k.lastTestedAt) : '—' }}</td>
            <td>
              <button class="btn ghost sm" @click="doTest(k.id)">test</button>
              <button class="btn danger sm" @click="doDelete(k.id)">delete</button>
              <button class="btn ghost sm" @click="ui.showRaw(k.displayName ?? k.id, k)">raw</button>
            </td>
          </tr>
          <tr v-if="!loading && items.length === 0">
            <td colspan="7" class="empty-row">No API keys configured. Click + Add.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.add-form {
  background: var(--bg-1);
  border-bottom: 1px solid var(--border-1);
  padding: var(--sp-3);
  display: grid;
  grid-template-columns: 140px 1fr;
  gap: var(--sp-1) var(--sp-2);
  align-items: center;
}
.add-form h4 { grid-column: 1 / -1; margin: 0 0 var(--sp-2); }
.add-form input, .add-form select { width: 100%; }
.add-form button { grid-column: 2; justify-self: start; margin-top: var(--sp-2); }
.field-label { font-size: var(--fs-xs); color: var(--fg-2); justify-self: end; text-align: right; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
