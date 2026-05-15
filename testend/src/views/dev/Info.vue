<script setup lang="ts">
/**
 * Info — backend metadata at boot (port, data dirs, build id, etc.) +
 * `/dev/forgify-home` directory listing for inspecting persisted state.
 */
import { onMounted, ref } from 'vue';
import { devAPI } from '@/api/dev';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { useUIStore } from '@/stores/ui';
import { bytes, timestamp, pretty } from '@/utils/format';

const ui = useUIStore();
const info = ref<Awaited<ReturnType<typeof devAPI.info>> | null>(null);
const home = ref<Awaited<ReturnType<typeof devAPI.forgifyHome>> | null>(null);
const err = ref<string | null>(null);

async function refresh() {
  err.value = null;
  try {
    info.value = await devAPI.info();
    home.value = await devAPI.forgifyHome();
  } catch (e) {
    err.value = (e as Error).message;
  }
}

onMounted(refresh);
</script>

<template>
  <div class="view">
    <ViewHeader title="Info" subtitle="backend boot metadata + forgify home">
      <template #actions>
        <button class="btn ghost sm" @click="refresh">refresh</button>
        <button class="btn ghost sm" @click="ui.showRaw('info+home', { info, home })">raw</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <section v-if="info" class="section">
        <h4>backend</h4>
        <div class="kv">
          <div class="k">port</div><div class="v mono">{{ info.port }}</div>
          <div class="k">home</div><div class="v mono">{{ info.home }}</div>
          <div class="k">forgify home</div><div class="v mono">{{ info.forgifyHome }}</div>
          <div class="k">integration dir</div><div class="v mono">{{ info.integrationDir }}</div>
          <div class="k">collections dir</div><div class="v mono">{{ info.collectionsDir }}</div>
          <div class="k">mcp config</div><div class="v mono">{{ info.mcpConfigPath }}</div>
          <div class="k">skills dir</div><div class="v mono">{{ info.skillsDir }}</div>
          <div class="k">catalog cache</div><div class="v mono">{{ info.catalogCachePath }}</div>
          <div v-if="info.goVersion" class="k">go</div><div v-if="info.goVersion" class="v mono">{{ info.goVersion }}</div>
          <div v-if="info.startedAt" class="k">started</div><div v-if="info.startedAt" class="v">{{ timestamp(info.startedAt) }}</div>
          <div v-if="info.buildID" class="k">build id</div><div v-if="info.buildID" class="v mono">{{ info.buildID }}</div>
        </div>
      </section>

      <section v-if="info?.tableCounts" class="section">
        <h4>table row counts</h4>
        <pre class="code-block mono">{{ pretty(info.tableCounts) }}</pre>
      </section>

      <section v-if="home?.tree" class="section">
        <h4>forgify-home tree ({{ home.path }})</h4>
        <table class="table sm">
          <thead><tr><th>name</th><th style="width: 100px">size</th><th style="width: 140px">modified</th></tr></thead>
          <tbody>
            <tr v-for="e in home.tree" :key="e.name">
              <td>
                <span v-if="e.isDir" class="dim">▸</span>
                <span class="mono">{{ e.name }}</span>
              </td>
              <td class="mono xs">{{ e.isDir ? '—' : bytes(e.size) }}</td>
              <td class="dim xs">{{ timestamp(e.modified) }}</td>
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
.section { margin-bottom: var(--sp-4); }
.section h4 { margin: var(--sp-2) 0 var(--sp-1); font-size: var(--fs-xs); text-transform: uppercase; letter-spacing: 0.05em; color: var(--fg-2); }
.kv { display: grid; grid-template-columns: 160px 1fr; row-gap: 2px; column-gap: var(--sp-2); }
.k { color: var(--fg-3); font-size: var(--fs-xs); }
.v { font-size: var(--fs-sm); word-break: break-all; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
