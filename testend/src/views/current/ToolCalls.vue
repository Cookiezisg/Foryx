<script setup lang="ts">
/**
 * Tool calls — every tool_call block in the selected conversation.
 * Sourced from chat store blocksByConv; filters to type=tool_call.
 * Cross-references each call with its sibling tool_result child.
 */
import { computed } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useChatStore } from '@/stores/chat';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { duration, pretty, truncate } from '@/utils/format';
import type { Block } from '@/types/domain';

const conv = useConvStore();
const chat = useChatStore();
const ui = useUIStore();

const calls = computed<Array<Block & { _result?: Block }>>(() => {
  if (!conv.selectedId) return [];
  const map = chat.blocksByConv[conv.selectedId] ?? {};
  const list = Object.values(map).filter((b) => b.type === 'tool_call');
  for (const c of list) {
    (c as Block & { _result?: Block })._result = Object.values(map).find(
      (x) => x.type === 'tool_result' && x.parentBlockId === c.id,
    );
  }
  return list.sort((a, b) => a.seq - b.seq);
});

function attrs(b: Block) {
  return (b.attrs ?? {}) as Record<string, unknown>;
}
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Tool calls" hint="选个对话先" />
  </div>
  <div v-else class="view">
    <ViewHeader title="Tool calls" :subtitle="`conv ${conv.selectedId} · ${calls.length} calls`" />
    <div class="scroll">
      <table class="table">
        <thead>
          <tr>
            <th style="width: 70px">seq</th>
            <th style="width: 160px">tool</th>
            <th>summary</th>
            <th style="width: 90px">status</th>
            <th style="width: 60px">grp</th>
            <th style="width: 80px">elapsed</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in calls" :key="c.id">
            <td class="mono xs dim">{{ c.seq }}</td>
            <td class="mono">{{ attrs(c).toolName ?? '?' }}</td>
            <td class="ellipsis-cell">{{ attrs(c).summary ?? '—' }}</td>
            <td><span class="pill" :class="c.status">{{ c.status }}</span></td>
            <td class="mono xs">{{ attrs(c).executionGroup ?? '—' }}</td>
            <td class="mono xs">{{ duration(Number(attrs(c).elapsedMs)) }}</td>
            <td>
              <button class="btn ghost sm" @click="ui.showRaw(`tool_call ${attrs(c).toolName}`, { call: c, result: c._result })">
                raw
              </button>
            </td>
          </tr>
          <tr v-if="calls.length === 0">
            <td colspan="7" class="empty-row">No tool calls in this conversation yet.</td>
          </tr>
        </tbody>
      </table>

      <!-- Detail panel: expand one row's args + result -->
      <div v-for="c in calls" :key="`detail-${c.id}`" class="detail-card">
        <header class="detail-head">
          <span class="mono">{{ attrs(c).toolName }}</span>
          <span class="dim small">{{ c.id }}</span>
          <span class="pill" :class="c.status">{{ c.status }}</span>
          <span v-if="attrs(c).destructive" class="pill warn">⚠ destructive</span>
        </header>
        <details v-if="c.content" class="detail-section">
          <summary class="dim">args</summary>
          <pre class="code-block mono">{{ pretty(c.content) }}</pre>
        </details>
        <details v-if="c._result" class="detail-section">
          <summary class="dim">result</summary>
          <pre class="code-block mono">{{ truncate(c._result.content, 2000) }}</pre>
        </details>
      </div>
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
  padding: 0 var(--sp-3) var(--sp-3);
}
.ellipsis-cell {
  max-width: 400px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty-row {
  text-align: center;
  color: var(--fg-3);
  padding: var(--sp-6) 0;
  font-style: italic;
}
.detail-card {
  margin-top: var(--sp-3);
  border: 1px solid var(--border-1);
  border-radius: var(--radius-sm);
  padding: var(--sp-2);
}
.detail-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: var(--sp-2);
  flex-wrap: wrap;
}
.detail-section {
  margin: var(--sp-1) 0;
}
.detail-section summary {
  cursor: pointer;
  padding: 4px 0;
  font-size: var(--fs-xs);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
</style>
