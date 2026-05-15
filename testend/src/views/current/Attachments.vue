<script setup lang="ts">
/**
 * Attachments — files uploaded to this conversation. Derived from message
 * `attrs.attachments` arrays (the convention for the user-message attach
 * flow, per chat.go `AttachmentRef`). Each entry: {id, filename, sizeBytes,
 * contentType}.
 *
 * The backend has no GET attachments list endpoint — uploads happen via
 * `POST /api/v1/attachments` and the references end up nested in the
 * spawning user message's attrs. So this view aggregates from in-memory
 * messages already loaded by chat.ts.
 */
import { computed } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useChatStore } from '@/stores/chat';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { bytes, timeAgo, shortID } from '@/utils/format';

const conv = useConvStore();
const chat = useChatStore();
const ui = useUIStore();

interface AttachmentRow {
  id: string;
  filename: string;
  sizeBytes?: number;
  contentType?: string;
  messageId: string;
  createdAt: string;
}

const rows = computed<AttachmentRow[]>(() => {
  if (!conv.selectedId) return [];
  const out: AttachmentRow[] = [];
  for (const m of chat.messagesByConv[conv.selectedId] ?? []) {
    const a = (m.attrs ?? {}) as Record<string, unknown>;
    const list = (a.attachments as Array<Record<string, unknown>>) ?? [];
    for (const att of list) {
      out.push({
        id: String(att.id ?? ''),
        filename: String(att.filename ?? '?'),
        sizeBytes: typeof att.sizeBytes === 'number' ? att.sizeBytes : undefined,
        contentType: att.contentType as string | undefined,
        messageId: m.id,
        createdAt: m.createdAt,
      });
    }
  }
  return out;
});
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Attachments" hint="选个对话先" />
  </div>
  <div v-else class="view">
    <ViewHeader title="Attachments" :subtitle="`conv ${conv.selectedId} · ${rows.length} files`" />
    <div class="scroll">
      <table class="table">
        <thead>
          <tr>
            <th style="width: 200px">id</th>
            <th>filename</th>
            <th style="width: 80px">size</th>
            <th style="width: 160px">type</th>
            <th style="width: 200px">message</th>
            <th style="width: 100px">uploaded</th>
            <th style="width: 60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="`${r.id}-${r.messageId}`">
            <td class="mono xs">{{ shortID(r.id, 10) }}</td>
            <td>{{ r.filename }}</td>
            <td class="mono xs">{{ bytes(r.sizeBytes) }}</td>
            <td class="mono xs">{{ r.contentType ?? '—' }}</td>
            <td class="mono xs">{{ shortID(r.messageId, 8) }}</td>
            <td class="dim xs">{{ timeAgo(r.createdAt) }}</td>
            <td><button class="btn ghost sm" @click="ui.showRaw(r.filename, r)">raw</button></td>
          </tr>
          <tr v-if="rows.length === 0">
            <td colspan="7" class="empty-row">No attachments uploaded to this conversation yet.</td>
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
