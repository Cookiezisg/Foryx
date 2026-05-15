<script setup lang="ts">
/**
 * Errors — recent error / warn-level lines from the backend log SSE.
 * Subscribes to `/dev/logs` (same channel as Backend logs view) but only
 * keeps entries with level=error|fatal|warn for quick triage.
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
const MAX = 500;
const connected = ref(false);
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
      level = (parsed.level ?? '').toLowerCase();
      msg = parsed.msg ?? data;
    } catch { /* keep raw */ }
    if (!['error', 'fatal', 'warn'].includes(level)) return;
    lines.value.unshift({ level, msg, raw: data, at: Date.now() });
    if (lines.value.length > MAX) lines.value.length = MAX;
  });
  es.onopen = () => { connected.value = true; };
  es.onerror = () => { connected.value = false; };
}

function stop() {
  if (es) { es.close(); es = null; }
}

onMounted(start);
onUnmounted(stop);

const errCount = computed(() => lines.value.filter((l) => ['error', 'fatal'].includes(l.level)).length);
const warnCount = computed(() => lines.value.filter((l) => l.level === 'warn').length);
</script>

<template>
  <div class="view">
    <ViewHeader
      title="Errors"
      :subtitle="`${errCount} errors · ${warnCount} warns · ${connected ? 'live' : 'disconnected'}`"
    >
      <template #actions>
        <button class="btn ghost sm" @click="lines.length = 0">clear</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <table class="table sm mono">
        <thead><tr><th style="width: 110px">time</th><th style="width: 70px">level</th><th>message</th><th style="width: 50px"></th></tr></thead>
        <tbody>
          <tr v-for="l in lines" :key="l.at + l.raw" :class="l.level">
            <td class="dim xs">{{ timestamp(l.at) }}</td>
            <td><span class="pill" :class="l.level === 'warn' ? 'warn' : 'err'">{{ l.level }}</span></td>
            <td class="msg-cell">{{ l.msg }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(l.level, l.raw)">raw</button></td>
          </tr>
          <tr v-if="lines.length === 0">
            <td colspan="4" class="empty-row">No errors or warnings captured yet this session — that's good.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.msg-cell { font-size: var(--fs-xs); max-width: 800px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-4) 0; font-style: italic; }
tr.error td, tr.fatal td { color: var(--status-err); }
</style>
