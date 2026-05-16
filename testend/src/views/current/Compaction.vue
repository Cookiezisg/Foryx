<script setup lang="ts">
/**
 * Compaction (current conv) — inspect the conversation-level token
 * compactor state (V1.2 §1 final-sweep):
 *
 *   • current `conversation.summary` (read-only)
 *   • all block context_role projection (hot / warm / cold / archived)
 *   • all compaction blocks (with their attrs)
 *
 * Compaction view 仅展示，不直接编辑 —— manager 由 chat runner 自动驱动；
 * 想强制压缩可点 "Force compact"（未来 POST /:compact 接入后）。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useUIStore } from '@/stores/ui';
import { convAPI } from '@/api/conversations';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { timeAgo, timestamp } from '@/utils/format';
import type { Block, Conversation } from '@/types/domain';

const conv = useConvStore();
const ui = useUIStore();

const blocks = ref<Block[]>([]);
const fullConv = ref<Conversation | null>(null);
const loading = ref(false);

const byRole = computed(() => {
  const groups: Record<string, Block[]> = { hot: [], warm: [], cold: [], archived: [] };
  for (const b of blocks.value) {
    const r = b.contextRole ?? 'hot';
    if (!groups[r]) groups[r] = [];
    groups[r].push(b);
  }
  return groups;
});

const compactionBlocks = computed(() =>
  blocks.value.filter((b) => b.type === 'compaction'),
);

let refreshTimer: number | undefined;

async function refresh() {
  if (!conv.selectedId) return;
  loading.value = true;
  try {
    fullConv.value = await convAPI.get(conv.selectedId);
    // No /blocks endpoint — flatten blocks out of every message in the conv.
    // 无 /blocks 端点——从每条 message 的 blocks 里平铺。
    const page = await convAPI.messages(conv.selectedId, 500);
    const all: Block[] = [];
    for (const m of page.items) {
      if (m.blocks) all.push(...m.blocks);
    }
    all.sort((a, b) => a.seq - b.seq);
    blocks.value = all;
  } catch (e) {
    ui.toast('err', `加载失败: ${(e as Error).message}`);
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  refresh();
  // soft polling every 5s — compaction is bursty, no SSE event yet
  refreshTimer = window.setInterval(refresh, 5000);
});

onUnmounted(() => {
  if (refreshTimer !== undefined) window.clearInterval(refreshTimer);
});

watch(() => conv.selectedId, refresh);

function roleClass(role?: string) {
  return `pill role-${role ?? 'hot'}`;
}
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Compaction" hint="Select a conversation first." />
  </div>
  <div v-else class="view">
    <ViewHeader
      title="Compaction"
      :subtitle="`conv ${conv.selectedId} · ${blocks.length} blocks · ${compactionBlocks.length} compaction events`"
    >
      <template #actions>
        <button class="btn ghost sm" :disabled="loading" @click="refresh">refresh</button>
      </template>
    </ViewHeader>

    <div class="scroll">
      <!-- Summary card -->
      <section class="card">
        <h4>Running summary</h4>
        <div v-if="fullConv?.summary" class="summary-body">
          <pre class="mono small">{{ fullConv.summary }}</pre>
          <div class="dim xs">
            covers blocks up to seq {{ fullConv.summaryCoversUpToSeq }}
          </div>
        </div>
        <div v-else class="dim small">
          No compaction has happened yet for this conversation. Compaction
          fires automatically when token usage crosses the hard threshold
          (default 85% of model's usable input).
        </div>
      </section>

      <!-- Role breakdown -->
      <section class="card">
        <h4>Block roles</h4>
        <div class="role-grid">
          <div
            v-for="(items, role) in byRole"
            :key="role"
            class="role-cell"
          >
            <span :class="roleClass(role)">{{ role }}</span>
            <span class="dim small">{{ items.length }}</span>
          </div>
        </div>
        <p class="dim xs note">
          hot = full content; warm = first 200 bytes + truncation marker;
          cold = metadata-only placeholder; archived = dropped (covered by summary).
        </p>
      </section>

      <!-- Compaction blocks list -->
      <section class="card">
        <h4>Compaction events ({{ compactionBlocks.length }})</h4>
        <table v-if="compactionBlocks.length > 0" class="table">
          <thead>
            <tr>
              <th style="width: 80px">seq</th>
              <th style="width: 120px">covers</th>
              <th style="width: 100px">archived</th>
              <th>summary preview</th>
              <th style="width: 120px">created</th>
              <th style="width: 60px"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="b in compactionBlocks" :key="b.id">
              <td class="mono xs">{{ b.seq }}</td>
              <td class="mono xs">{{ b.attrs?.coversFromSeq }} → {{ b.attrs?.coversToSeq }}</td>
              <td class="mono xs">{{ b.attrs?.blocksArchived ?? '?' }}</td>
              <td class="dim small ellipsis">{{ b.content?.slice(0, 200) ?? '' }}</td>
              <td class="dim xs">{{ timeAgo(b.createdAt) }}</td>
              <td>
                <button class="btn ghost sm" @click="ui.showRaw(`compaction ${b.id}`, b)">raw</button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-else class="dim small">No compaction blocks yet.</div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; flex-direction: column; height: 100%; }
.view-pad { flex: 1; display: flex; align-items: center; justify-content: center; }
.scroll { flex: 1; overflow: auto; padding: var(--sp-3); display: flex; flex-direction: column; gap: var(--sp-3); }
.card {
  background: var(--bg-1);
  border: 1px solid var(--border-1);
  border-radius: var(--radius-md);
  padding: var(--sp-3);
}
.card h4 { margin: 0 0 var(--sp-2); }
.summary-body { display: flex; flex-direction: column; gap: var(--sp-1); }
.summary-body pre { white-space: pre-wrap; word-wrap: break-word; background: var(--bg-2); padding: var(--sp-2); border-radius: var(--radius-sm); margin: 0; }
.role-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: var(--sp-3); }
.role-cell { display: flex; flex-direction: column; align-items: center; gap: var(--sp-1); }
.role-cell .small { font-size: var(--fs-lg); }
.note { margin-top: var(--sp-2); }
.pill.role-hot { background: #10b981; color: white; }
.pill.role-warm { background: #f59e0b; color: #1f2937; }
.pill.role-cold { background: #3b82f6; color: white; }
.pill.role-archived { background: var(--bg-2); color: var(--fg-3); }
.ellipsis { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 360px; }
</style>
