<script setup lang="ts">
/**
 * Metrics — runtime stats (uptime, goroutines, memory, GC, DB size).
 * Auto-refreshes every 3s while the view is mounted.
 */
import { onMounted, onUnmounted, ref } from 'vue';
import { devAPI } from '@/api/dev';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { bytes, duration } from '@/utils/format';

interface Runtime {
  uptimeSec: number;
  numGoroutine: number;
  memAllocBytes: number;
  memSysBytes: number;
  numGC: number;
  dbSizeBytes?: number;
}

const runtime = ref<Runtime | null>(null);
const err = ref<string | null>(null);
let timer: ReturnType<typeof setInterval> | null = null;

async function refresh() {
  try {
    runtime.value = await devAPI.runtime();
    err.value = null;
  } catch (e) {
    err.value = (e as Error).message;
  }
}

onMounted(() => {
  refresh();
  timer = setInterval(refresh, 3000);
});
onUnmounted(() => {
  if (timer) clearInterval(timer);
});
</script>

<template>
  <div class="view">
    <ViewHeader title="Metrics" subtitle="live runtime stats (3s poll)">
      <template #actions>
        <button class="btn ghost sm" @click="refresh">refresh</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <div v-if="runtime" class="grid">
        <div class="card">
          <div class="card-label">uptime</div>
          <div class="card-value mono">{{ duration(runtime.uptimeSec * 1000) }}</div>
        </div>
        <div class="card">
          <div class="card-label">goroutines</div>
          <div class="card-value mono">{{ runtime.numGoroutine }}</div>
        </div>
        <div class="card">
          <div class="card-label">mem alloc</div>
          <div class="card-value mono">{{ bytes(runtime.memAllocBytes) }}</div>
        </div>
        <div class="card">
          <div class="card-label">mem sys</div>
          <div class="card-value mono">{{ bytes(runtime.memSysBytes) }}</div>
        </div>
        <div class="card">
          <div class="card-label">gc cycles</div>
          <div class="card-value mono">{{ runtime.numGC }}</div>
        </div>
        <div class="card" v-if="runtime.dbSizeBytes !== undefined">
          <div class="card-label">db size</div>
          <div class="card-value mono">{{ bytes(runtime.dbSizeBytes) }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: var(--sp-3); }
.card { background: var(--bg-1); border: 1px solid var(--border-1); border-radius: var(--radius-md); padding: var(--sp-3); }
.card-label { color: var(--fg-3); font-size: var(--fs-xs); text-transform: uppercase; letter-spacing: 0.05em; }
.card-value { font-size: var(--fs-xl); font-weight: 600; margin-top: 4px; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); margin-bottom: var(--sp-2); }
</style>
