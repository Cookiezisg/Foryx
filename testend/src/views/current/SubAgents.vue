<script setup lang="ts">
/**
 * Sub-agents — messages in the selected conversation where
 * `attrs.kind === "subagent_run"`. Per the unified schema (Plan 03),
 * sub-runs are stored as Message rows nested under a tool_call block;
 * each sub-run's transcript is the block tree of that message.
 */
import { computed } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useChatStore } from '@/stores/chat';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { timeAgo, statusClass, shortID } from '@/utils/format';

const conv = useConvStore();
const chat = useChatStore();
const ui = useUIStore();

const runs = computed(() => {
  if (!conv.selectedId) return [];
  return (chat.messagesByConv[conv.selectedId] ?? []).filter((m) => {
    const a = (m.attrs ?? {}) as Record<string, unknown>;
    return a.kind === 'subagent_run';
  });
});
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Sub-agents" hint="选个对话先" />
  </div>
  <div v-else class="view">
    <ViewHeader title="Sub-agents" :subtitle="`conv ${conv.selectedId} · ${runs.length} sub-runs`" />
    <div class="scroll">
      <table class="table">
        <thead>
          <tr>
            <th style="width: 200px">id</th>
            <th>type</th>
            <th style="width: 100px">status</th>
            <th style="width: 120px">parent block</th>
            <th style="width: 100px">started</th>
            <th style="width: 90px">tokens</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in runs" :key="r.id">
            <td class="mono xs">{{ shortID(r.id, 10) }}</td>
            <td class="mono">{{ (r.attrs as any)?.subagentType ?? (r.attrs as any)?.agentType ?? '?' }}</td>
            <td><span class="pill" :class="statusClass(r.status)">{{ r.status }}</span></td>
            <td class="mono xs">{{ shortID((r.attrs as any)?.parentBlockId, 8) }}</td>
            <td class="dim xs">{{ timeAgo(r.createdAt) }}</td>
            <td class="mono xs">
              <span v-if="r.inputTokens || r.outputTokens">
                {{ r.inputTokens ?? 0 }}↑ {{ r.outputTokens ?? 0 }}↓
              </span>
              <span v-else class="dim">—</span>
            </td>
            <td><button class="btn ghost sm" @click="ui.showRaw(`subagent ${r.id}`, r)">raw</button></td>
          </tr>
          <tr v-if="runs.length === 0">
            <td colspan="7" class="empty-row">No sub-runs spawned in this conversation.</td>
          </tr>
        </tbody>
      </table>
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
.empty-row {
  text-align: center;
  color: var(--fg-3);
  padding: var(--sp-6) 0;
  font-style: italic;
}
</style>
