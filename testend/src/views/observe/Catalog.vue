<script setup lang="ts">
/**
 * Catalog — the live "Available capabilities" summary the main agent reads
 * to know what it can call. Refresh triggers a rebuild (forces source
 * re-scan); the bg poll keeps it up-to-date as the trinity catalog evolves.
 */
import { onMounted, ref } from 'vue';
import { useCatalogStore } from '@/stores/catalog';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timestamp, pretty } from '@/utils/format';

const cat = useCatalogStore();
const ui = useUIStore();
const refreshing = ref(false);

async function force() {
  refreshing.value = true;
  try {
    await cat.forceRebuild();
    ui.toast('ok', 'catalog 已重建');
  } catch (e) {
    ui.toast('err', (e as Error).message);
  } finally {
    refreshing.value = false;
  }
}

async function pull() {
  await cat.refresh();
}

onMounted(pull);
</script>

<template>
  <div class="view">
    <ViewHeader
      title="Catalog"
      :subtitle="cat.current ? `v${cat.current.version} · generated ${timestamp(cat.current.generatedAt)}` : 'no catalog yet'"
    >
      <template #actions>
        <button class="btn ghost sm" @click="pull">pull</button>
        <button class="btn primary sm" :disabled="refreshing" @click="force">{{ refreshing ? '...' : 'force rebuild' }}</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <section v-if="cat.current" class="section">
        <div class="meta-row">
          <span class="pill info">v{{ cat.current.version }}</span>
          <span class="dim small">fingerprint {{ cat.current.fingerprint.slice(0, 12) }}…</span>
          <button class="btn ghost sm" @click="ui.showRaw('catalog', cat.current)">raw</button>
        </div>

        <h4>summary (the prompt the agent sees)</h4>
        <pre class="code-block mono">{{ cat.current.summary }}</pre>

        <h4>coverage</h4>
        <pre class="code-block mono">{{ pretty(cat.current.coverage) }}</pre>

        <h4>source timestamps</h4>
        <pre class="code-block mono">{{ pretty(cat.current.sourcesAt) }}</pre>
      </section>
      <div v-else class="dim center">No catalog loaded yet. Click "pull".</div>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); }
.section { display: flex; flex-direction: column; gap: var(--sp-2); margin-bottom: var(--sp-4); }
.section h4 { margin: var(--sp-2) 0 0; font-size: var(--fs-xs); text-transform: uppercase; letter-spacing: 0.05em; color: var(--fg-2); }
.meta-row { display: flex; align-items: center; gap: var(--sp-2); padding-bottom: var(--sp-2); border-bottom: 1px solid var(--border-1); }
.center { padding: var(--sp-6); text-align: center; }
</style>
