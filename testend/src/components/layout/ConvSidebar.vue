<script setup lang="ts">
/**
 * ConvSidebar — col1.
 *   - List existing conversations (sorted by updatedAt desc)
 *   - "+ new" button
 *   - filter input
 *   - right-click menu (rename / set system prompt / duplicate / delete)
 *   - rail mode (40px icons-only) for expanded layout
 */
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useConvStore } from '@/stores/conv';
import { useUIStore } from '@/stores/ui';
import { timeAgo } from '@/utils/format';

defineProps<{ rail?: boolean }>();

const { t } = useI18n();
const conv = useConvStore();
const ui = useUIStore();

const showFilter = ref(false);

onMounted(() => {
  if (!conv.list.length) conv.refresh();
});

const filtered = computed(() => {
  const q = conv.filter.trim().toLowerCase();
  if (!q) return conv.list;
  return conv.list.filter(
    (c) => (c.title || '').toLowerCase().includes(q) || c.id.toLowerCase().includes(q),
  );
});

async function createNew() {
  await conv.create('');
}

/* context menu state */
const ctxMenu = ref<{ open: boolean; x: number; y: number; id: string }>({
  open: false,
  x: 0,
  y: 0,
  id: '',
});

function openCtx(e: MouseEvent, id: string) {
  e.preventDefault();
  ctxMenu.value = { open: true, x: e.clientX, y: e.clientY, id };
  document.addEventListener('click', closeCtx, { once: true });
}

function closeCtx() {
  ctxMenu.value.open = false;
}

async function doRename(id: string) {
  closeCtx();
  const cur = conv.list.find((c) => c.id === id);
  const next = window.prompt(t('convs.renamePrompt'), cur?.title || '');
  if (next !== null && next !== cur?.title) await conv.rename(id, next);
}

async function doSysPrompt(id: string) {
  closeCtx();
  const cur = conv.list.find((c) => c.id === id);
  const next = window.prompt(t('convs.systemPromptDialog'), cur?.systemPrompt || '');
  if (next !== null) await conv.setSystemPrompt(id, next);
}

async function doDuplicate(id: string) {
  closeCtx();
  await conv.duplicate(id);
}

async function doToggleArchive(id: string) {
  closeCtx();
  const cur = conv.list.find((c) => c.id === id);
  if (!cur) return;
  await conv.setArchived(id, !cur.archived);
}

async function doTogglePin(id: string) {
  closeCtx();
  const cur = conv.list.find((c) => c.id === id);
  if (!cur) return;
  await conv.setPinned(id, !cur.pinned);
}

async function doDelete(id: string) {
  closeCtx();
  if (!window.confirm(t('convs.deleteConfirm'))) return;
  await conv.remove(id);
}
</script>

<template>
  <aside class="convs" :class="{ rail }">
    <header class="convs-header">
      <template v-if="!rail">
        <strong class="convs-title">{{ conv.showArchived ? t('convs.archived') : t('convs.title') }}</strong>
        <button
          class="btn ghost icon"
          :class="{ 'archived-on': conv.showArchived }"
          @click="conv.toggleShowArchived()"
          :title="conv.showArchived ? t('convs.showActive') : t('convs.showArchived')"
        >📁</button>
        <button class="btn ghost icon" @click="showFilter = !showFilter" :title="t('convs.showFilter')">⌕</button>
        <button class="btn ghost icon primary-look" @click="createNew" :title="t('convs.newButton')">＋</button>
      </template>
      <template v-else>
        <button class="btn ghost icon" @click="ui.toggleExpanded()" title="restore 4-column">⤡</button>
        <button class="btn ghost icon" @click="createNew" title="new conversation">＋</button>
      </template>
    </header>

    <input
      v-if="!rail && showFilter"
      class="convs-filter"
      :placeholder="t('convs.filterPlaceholder')"
      v-model="conv.filter"
      autofocus
    />

    <div class="convs-list scroll" v-if="!rail">
      <div v-if="conv.loading" class="empty">
        <span class="dim">{{ t('convs.loadingHint') }}</span>
      </div>
      <div v-else-if="filtered.length === 0" class="empty">
        <span class="empty-title">{{ conv.list.length === 0 ? t('convs.emptyTitle') : t('convs.noMatch') }}</span>
        <button class="btn primary" @click="createNew">{{ t('convs.startFirst') }}</button>
      </div>
      <div
        v-for="c in filtered"
        :key="c.id"
        class="conv-item"
        :class="{ selected: c.id === conv.selectedId, pinned: c.pinned }"
        @click="conv.select(c.id)"
        @contextmenu="openCtx($event, c.id)"
        :title="`${c.title || t('common.untitled')} · ${c.id}`"
      >
        <div class="conv-line">
          <span v-if="c.pinned" class="pin-mark" title="pinned">📌</span>
          <span class="conv-title ellipsis">{{ c.title || t('common.untitled') }}</span>
          <span class="conv-time">{{ timeAgo(c.updatedAt) }}</span>
        </div>
        <div class="conv-id mono dim">{{ c.id.slice(0, 18) }}</div>
      </div>
    </div>

    <div v-else class="rail-list">
      <button
        v-for="c in conv.list.slice(0, 8)"
        :key="c.id"
        class="rail-conv"
        :class="{ selected: c.id === conv.selectedId }"
        @click="conv.select(c.id)"
        :title="c.title || c.id"
      >
        ●
      </button>
    </div>

    <!-- context menu -->
    <div
      v-if="ctxMenu.open"
      class="ctx-menu"
      :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
      @click.stop
    >
      <button class="ctx-item" @click="doTogglePin(ctxMenu.id)">
        {{ conv.list.find((c) => c.id === ctxMenu.id)?.pinned ? t('convs.ctxUnpin') : t('convs.ctxPin') }}
      </button>
      <button class="ctx-item" @click="doRename(ctxMenu.id)">{{ t('convs.ctxRename') }}</button>
      <button class="ctx-item" @click="doSysPrompt(ctxMenu.id)">{{ t('convs.ctxSysPrompt') }}</button>
      <button class="ctx-item" @click="doDuplicate(ctxMenu.id)">{{ t('convs.ctxDuplicate') }}</button>
      <button class="ctx-item" @click="doToggleArchive(ctxMenu.id)">
        {{ conv.list.find((c) => c.id === ctxMenu.id)?.archived ? t('convs.ctxUnarchive') : t('convs.ctxArchive') }}
      </button>
      <div class="ctx-sep" />
      <button class="ctx-item danger" @click="doDelete(ctxMenu.id)">{{ t('convs.ctxDelete') }}</button>
    </div>
  </aside>
</template>

<style scoped>
.convs {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-1);
  border-right: 1px solid var(--border-1);
  min-width: 0;
}

.convs-header {
  display: flex;
  align-items: center;
  padding: var(--sp-2) var(--sp-2) var(--sp-2) var(--sp-3);
  gap: var(--sp-1);
  border-bottom: 1px solid var(--border-1);
}

.convs.rail .convs-header {
  flex-direction: column;
  padding: var(--sp-2) 0;
  gap: var(--sp-2);
}

.convs-title {
  flex: 1;
  font-size: var(--fs-xs);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--fg-2);
}

.primary-look {
  color: var(--accent);
}

.archived-on {
  color: var(--accent);
  background: var(--accent-bg);
}

.convs-filter {
  margin: var(--sp-2);
  padding: var(--sp-1) var(--sp-2);
  font-size: var(--fs-sm);
}

.convs-list {
  flex: 1;
  overflow-y: auto;
  padding: 4px;
}

.conv-item {
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  border: 1px solid transparent;
  margin-bottom: 2px;
}

.conv-item:hover {
  background: var(--bg-hover);
}

.conv-item.selected {
  background: var(--bg-active);
  border-color: var(--border-2);
}

.conv-item.pinned {
  background: color-mix(in srgb, var(--accent-bg) 50%, transparent);
}

.pin-mark {
  font-size: 10px;
  margin-right: 4px;
  flex-shrink: 0;
}

.conv-line {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--sp-2);
}

.conv-title {
  font-size: var(--fs-sm);
  font-weight: 500;
}

.conv-time {
  font-size: 10px;
  color: var(--fg-3);
  white-space: nowrap;
  flex-shrink: 0;
}

.conv-id {
  font-size: 10px;
  color: var(--fg-3);
  margin-top: 1px;
}

.rail-list {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--sp-2) 0;
  gap: 4px;
  overflow-y: auto;
}

.rail-conv {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: transparent;
  color: var(--fg-3);
  font-size: 14px;
}

.rail-conv:hover {
  background: var(--bg-hover);
}

.rail-conv.selected {
  color: var(--accent);
  background: var(--accent-bg);
}

.ctx-menu {
  position: fixed;
  background: var(--bg-2);
  border: 1px solid var(--border-2);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-2);
  padding: 4px;
  z-index: 100;
  min-width: 160px;
}

.ctx-item {
  display: block;
  width: 100%;
  text-align: left;
  padding: 6px 10px;
  font-size: var(--fs-sm);
  border-radius: var(--radius-sm);
  color: var(--fg-1);
  background: transparent;
}

.ctx-item:hover {
  background: var(--bg-hover);
}

.ctx-item.danger {
  color: var(--status-err);
}

.ctx-sep {
  height: 1px;
  background: var(--border-1);
  margin: 4px 0;
}

.empty {
  padding: var(--sp-4);
  flex-direction: column;
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
</style>
