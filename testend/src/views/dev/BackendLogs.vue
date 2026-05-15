<script setup lang="ts">
/**
 * Backend logs — tail of zap logs broadcast over `/dev/logs` SSE. Newest
 * first; level filter + free-text search. Plain EventSource (this stream
 * is separate from the 3 main SSE streams to avoid mixing channels).
 */
import { onMounted, onUnmounted, ref, computed } from 'vue';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timestamp } from '@/utils/format';

interface LogEntry {
  level: string;
  msg: string;
  raw: string;
  at: number;
}

const ui = useUIStore();
const lines = ref<LogEntry[]>([]);
const MAX = 2000;
const levelFilter = ref<string>('');
const search = ref<string>('');
const connected = ref(false);
const lastError = ref<string | null>(null);

let es: EventSource | null = null;

function start() {
  if (es) return;
  es = new EventSource('/dev/logs');
  es.addEventListener('log', (ev) => {
    const data = (ev as MessageEvent).data as string;
    let level = '';
    let msg = data;
    try {
      const parsed = JSON.parse(data);
      level = parsed.level ?? '';
      msg = parsed.msg ?? data;
    } catch {
      // dev log line is sometimes a raw zap-encoded string — keep as-is
    }
    lines.value.unshift({ level, msg, raw: data, at: Date.now() });
    if (lines.value.length > MAX) lines.value.length = MAX;
  });
  es.onopen = () => { connected.value = true; lastError.value = null; };
  es.onerror = () => {
    connected.value = false;
    lastError.value = 'connection error; will auto-reconnect';
  };
}

function stop() {
  if (es) { es.close(); es = null; connected.value = false; }
}

onMounted(start);
onUnmounted(stop);

const filtered = computed(() => {
  let xs = lines.value;
  if (levelFilter.value) xs = xs.filter((l) => l.level === levelFilter.value);
  const q = search.value.trim().toLowerCase();
  if (q) xs = xs.filter((l) => l.msg.toLowerCase().includes(q) || l.raw.toLowerCase().includes(q));
  return xs;
});

function levelCls(level: string): string {
  switch (level.toLowerCase()) {
    case 'error': case 'fatal': return 'err';
    case 'warn': return 'warn';
    case 'info': return 'info';
    case 'debug': return 'pending';
    default: return '';
  }
}
</script>

<template>
  <div class="view">
    <ViewHeader
      title="Backend logs"
      :subtitle="`${filtered.length} of ${lines.length} captured · ${connected ? 'connected' : 'disconnected'}`"
    >
      <template #actions>
        <select v-model="levelFilter" class="sm">
          <option value="">all levels</option>
          <option value="debug">debug</option>
          <option value="info">info</option>
          <option value="warn">warn</option>
          <option value="error">error</option>
          <option value="fatal">fatal</option>
        </select>
        <input v-model="search" placeholder="search…" class="sm" />
        <button class="btn ghost sm" @click="lines.length = 0">clear</button>
      </template>
    </ViewHeader>
    <div v-if="lastError" class="warn-row">{{ lastError }}</div>
    <div class="scroll">
      <table class="table sm mono">
        <thead><tr><th style="width: 110px">time</th><th style="width: 70px">level</th><th>message</th><th style="width: 50px"></th></tr></thead>
        <tbody>
          <tr v-for="l in filtered" :key="l.at + l.raw">
            <td class="dim xs">{{ timestamp(l.at) }}</td>
            <td><span class="pill" :class="levelCls(l.level)">{{ l.level || '—' }}</span></td>
            <td class="msg-cell">{{ l.msg }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(l.level, l.raw)">raw</button></td>
          </tr>
          <tr v-if="filtered.length === 0">
            <td colspan="4" class="empty-row">No logs match.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.warn-row { padding: var(--sp-1) var(--sp-3); background: var(--status-warn-bg); color: var(--status-warn); font-size: var(--fs-xs); }
.msg-cell { font-size: var(--fs-xs); max-width: 800px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-4) 0; font-style: italic; }
</style>
