<script setup lang="ts">
/**
 * Processes — background Bash sessions spawned by the Bash tool. Useful
 * during dev to see what long-lived commands the agent has running.
 */
import { onMounted, ref } from 'vue';
import { devAPI } from '@/api/dev';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { timeAgo, shortID, statusClass } from '@/utils/format';

interface Proc {
  id: string;
  command: string;
  cwd: string;
  startedAt: string;
  status: string;
  exitCode?: number;
}

const ui = useUIStore();
const procs = ref<Proc[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    const r = await devAPI.bashProcesses();
    procs.value = (r as { processes?: Proc[] }).processes ?? (r as unknown as Proc[]);
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);
</script>

<template>
  <div class="view">
    <ViewHeader title="Processes" :subtitle="`${procs.length} Bash bg sessions`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <table class="table">
        <thead>
          <tr>
            <th style="width: 180px">id</th>
            <th style="width: 100px">status</th>
            <th>command</th>
            <th>cwd</th>
            <th style="width: 80px">exit</th>
            <th style="width: 110px">started</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in procs" :key="p.id">
            <td class="mono xs">{{ shortID(p.id, 10) }}</td>
            <td><span class="pill" :class="statusClass(p.status)">{{ p.status }}</span></td>
            <td class="mono xs ellipsis-cell">{{ p.command }}</td>
            <td class="mono xs ellipsis-cell">{{ p.cwd }}</td>
            <td class="mono xs">{{ p.exitCode ?? '—' }}</td>
            <td class="dim xs">{{ timeAgo(p.startedAt) }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(p.id, p)">raw</button></td>
          </tr>
          <tr v-if="procs.length === 0 && !loading">
            <td colspan="7" class="empty-row">No background Bash processes running.</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: 0 var(--sp-3) var(--sp-3); }
.ellipsis-cell { max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-row { text-align: center; color: var(--fg-3); padding: var(--sp-6) 0; font-style: italic; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
