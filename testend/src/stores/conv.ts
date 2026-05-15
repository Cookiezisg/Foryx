/**
 * Conversation store — list (sidebar), selected conv, CRUD ops.
 *
 * Chat message state lives in chat.ts; this store only handles the
 * sidebar list + which conv is selected.
 */

import { defineStore } from 'pinia';
import { ref } from 'vue';
import { convAPI } from '@/api/conversations';
import type { Conversation } from '@/types/domain';
import { useUIStore } from './ui';

export const useConvStore = defineStore('conv', () => {
  const ui = useUIStore();

  const list = ref<Conversation[]>([]);
  const loading = ref(false);
  const selectedId = ref<string | null>(null);
  const filter = ref('');

  async function refresh() {
    loading.value = true;
    try {
      const page = await convAPI.list(200);
      list.value = page.items.sort((a, b) =>
        (b.updatedAt || b.createdAt).localeCompare(a.updatedAt || a.createdAt),
      );
    } catch (e) {
      ui.toast('err', `加载对话列表失败: ${(e as Error).message}`);
    } finally {
      loading.value = false;
    }
  }

  async function create(title = '') {
    try {
      const conv = await convAPI.create(title);
      list.value.unshift(conv);
      selectedId.value = conv.id;
      return conv;
    } catch (e) {
      ui.toast('err', `新建对话失败: ${(e as Error).message}`);
      throw e;
    }
  }

  async function rename(id: string, title: string) {
    try {
      const conv = await convAPI.rename(id, title);
      const i = list.value.findIndex((c) => c.id === id);
      if (i >= 0) list.value[i] = conv;
    } catch (e) {
      ui.toast('err', `重命名失败: ${(e as Error).message}`);
    }
  }

  async function setSystemPrompt(id: string, systemPrompt: string) {
    try {
      const conv = await convAPI.setSystemPrompt(id, systemPrompt);
      const i = list.value.findIndex((c) => c.id === id);
      if (i >= 0) list.value[i] = conv;
      ui.toast('ok', 'system prompt 已保存');
    } catch (e) {
      ui.toast('err', `保存失败: ${(e as Error).message}`);
    }
  }

  async function remove(id: string) {
    try {
      await convAPI.remove(id);
      list.value = list.value.filter((c) => c.id !== id);
      if (selectedId.value === id) selectedId.value = null;
    } catch (e) {
      ui.toast('err', `删除失败: ${(e as Error).message}`);
    }
  }

  async function duplicate(id: string) {
    // Best-effort: fetch source, create new with copied title + system prompt.
    const src = list.value.find((c) => c.id === id);
    if (!src) return;
    const fresh = await create(`${src.title || '(untitled)'} (copy)`);
    if (src.systemPrompt) {
      await setSystemPrompt(fresh.id, src.systemPrompt);
    }
  }

  function select(id: string | null) {
    selectedId.value = id;
  }

  /** Bump updatedAt on a conv (in-memory) — called when a new message arrives. */
  function touchUpdated(id: string, ts?: string) {
    const i = list.value.findIndex((c) => c.id === id);
    if (i < 0) return;
    list.value[i].updatedAt = ts ?? new Date().toISOString();
    // Re-sort: move touched conv to top.
    const [item] = list.value.splice(i, 1);
    list.value.unshift(item);
  }

  function setTitle(id: string, title: string) {
    const i = list.value.findIndex((c) => c.id === id);
    if (i >= 0) list.value[i].title = title;
  }

  return {
    list, loading, selectedId, filter,
    refresh, create, rename, setSystemPrompt, remove, duplicate, select,
    touchUpdated, setTitle,
  };
});
