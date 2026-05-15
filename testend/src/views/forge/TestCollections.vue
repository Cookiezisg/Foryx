<script setup lang="ts">
/**
 * Test collections — YAML scenarios loaded from --collections-dir. Each
 * collection is a sequence of HTTP steps that can be replayed. Currently
 * read-only viewer; a step runner can be added later.
 */
import { onMounted, ref } from 'vue';
import { devAPI } from '@/api/dev';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import { pretty } from '@/utils/format';

const ui = useUIStore();

interface Step {
  name: string;
  method: string;
  path: string;
  body?: Record<string, unknown>;
  expect?: { status: number };
  capture?: Record<string, string>;
}
interface Collection {
  name: string;
  description: string;
  steps: Step[];
}

const collections = ref<Collection[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    collections.value = await devAPI.collections();
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
    <ViewHeader title="Test collections" :subtitle="`${collections.length} YAML files loaded`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">{{ loading ? '...' : 'refresh' }}</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <div v-if="!loading && collections.length === 0" class="empty">
        <div class="empty-title">No collections found</div>
        <div class="empty-hint">
          Drop YAML scenarios into the directory pointed at by <code>--collections-dir</code>.
        </div>
      </div>
      <article v-for="c in collections" :key="c.name" class="col-card">
        <header class="col-head">
          <strong>{{ c.name }}</strong>
          <span class="dim small">{{ c.description }}</span>
          <span class="spacer" />
          <span class="pill info">{{ c.steps.length }} steps</span>
          <button class="btn ghost sm" @click="ui.showRaw(c.name, c)">raw</button>
        </header>
        <table class="table sm">
          <thead><tr><th style="width: 60px">#</th><th style="width: 80px">method</th><th>path</th><th>expect</th><th>capture</th></tr></thead>
          <tbody>
            <tr v-for="(s, i) in c.steps" :key="i">
              <td class="mono xs">{{ i + 1 }}</td>
              <td class="mono">{{ s.method }}</td>
              <td class="mono ellipsis-cell">{{ s.path }}</td>
              <td class="mono xs">{{ s.expect?.status ?? '—' }}</td>
              <td class="mono xs">{{ Object.keys(s.capture ?? {}).join(', ') || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </article>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-3); }
.col-card { border: 1px solid var(--border-1); border-radius: var(--radius-md); padding: var(--sp-2); background: var(--bg-1); }
.col-head { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: var(--sp-2); }
.spacer { flex: 1; }
.ellipsis-cell { max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty { text-align: center; padding: var(--sp-6) 0; }
.empty-title { font-weight: 600; margin-bottom: var(--sp-1); }
.empty-hint code { background: var(--bg-2); padding: 1px 6px; border-radius: 4px; }
.error { background: var(--status-err-bg); color: var(--status-err); padding: var(--sp-2); border-radius: var(--radius-sm); }
</style>
