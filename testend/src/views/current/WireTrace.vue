<script setup lang="ts">
/**
 * Wire trace — every LLM call (request + stream events + final text)
 * captured during this server uptime for the selected conversation.
 * Source: `GET /dev/llm-trace` (TE-5a; only set when `--dev` boots the
 * server and llmFactory.SetTracer was called).
 */
import { computed, onMounted, ref } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useUIStore } from '@/stores/ui';
import { devAPI } from '@/api/dev';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { duration, timestamp, pretty } from '@/utils/format';

const conv = useConvStore();
const ui = useUIStore();

interface Trace {
  traceId: string;
  conversationId?: string;
  scenario?: string;
  model?: string;
  startedAt: string;
  elapsedMs: number;
  request?: unknown;
  events?: unknown[];
  finalText?: string;
  error?: string;
}

const all = ref<Trace[]>([]);
const loading = ref(false);
const err = ref<string | null>(null);

async function refresh() {
  loading.value = true;
  err.value = null;
  try {
    all.value = await devAPI.llmTrace();
  } catch (e) {
    err.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
}

onMounted(refresh);

const traces = computed(() => {
  if (!conv.selectedId) return [];
  return all.value
    .filter((t) => t.conversationId === conv.selectedId)
    .sort((a, b) => b.startedAt.localeCompare(a.startedAt));
});
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Wire trace" hint="选个对话先" />
  </div>
  <div v-else class="view">
    <ViewHeader title="Wire trace" :subtitle="`conv ${conv.selectedId} · ${traces.length} LLM calls captured`">
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">{{ loading ? '...' : 'refresh' }}</button>
      </template>
    </ViewHeader>
    <div class="scroll">
      <div v-if="err" class="error">⨯ {{ err }}</div>
      <div v-if="!loading && traces.length === 0" class="empty">
        <div class="empty-title">No LLM calls traced</div>
        <div class="empty-hint">
          Trace recorder is enabled only when the backend is started with <code>--dev</code>.
          Send a message in chat and refresh.
        </div>
      </div>

      <article v-for="t in traces" :key="t.traceId" class="trace-card">
        <header class="trace-head">
          <span class="mono">{{ t.scenario ?? '?' }} · {{ t.model ?? '?' }}</span>
          <span class="dim xs">{{ timestamp(t.startedAt) }}</span>
          <span class="pill ok">{{ duration(t.elapsedMs) }}</span>
          <span v-if="t.error" class="pill err">error</span>
          <span class="spacer" />
          <button class="btn ghost sm" @click="ui.showRaw(`trace ${t.traceId}`, t)">raw</button>
        </header>

        <details>
          <summary class="dim">request ({{ JSON.stringify(t.request)?.length ?? 0 }} chars)</summary>
          <pre class="code-block mono">{{ pretty(t.request) }}</pre>
        </details>

        <details>
          <summary class="dim">events ({{ (t.events ?? []).length }})</summary>
          <pre class="code-block mono">{{ pretty(t.events) }}</pre>
        </details>

        <details v-if="t.finalText">
          <summary class="dim">final text</summary>
          <pre class="code-block mono">{{ t.finalText }}</pre>
        </details>

        <div v-if="t.error" class="error">{{ t.error }}</div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.view-pad {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.scroll {
  flex: 1;
  overflow: auto;
  padding: var(--sp-3);
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.trace-card {
  border: 1px solid var(--border-1);
  border-radius: var(--radius-md);
  padding: var(--sp-3);
  background: var(--bg-1);
}
.trace-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: var(--sp-2);
  flex-wrap: wrap;
}
.spacer { flex: 1; }
.empty {
  text-align: center;
  padding: var(--sp-6) 0;
}
.empty-title {
  font-weight: 600;
  margin-bottom: var(--sp-1);
}
.empty-hint code {
  background: var(--bg-2);
  padding: 1px 6px;
  border-radius: 4px;
}
.error {
  background: var(--status-err-bg);
  color: var(--status-err);
  padding: var(--sp-2);
  border-radius: var(--radius-sm);
  margin-top: var(--sp-2);
  font-size: var(--fs-sm);
}
details {
  margin-top: var(--sp-2);
}
details summary {
  cursor: pointer;
  font-size: var(--fs-xs);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  padding: 4px 0;
}
</style>
