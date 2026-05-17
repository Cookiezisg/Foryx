/**
 * Users store — list of local profiles + active user.
 *
 * Active user is persisted to localStorage via `setActiveUserID` so the next
 * page load picks the same profile. Switching reloads the page to clear all
 * in-memory state from other stores.
 */

import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import { usersAPI, type User } from '@/api/users';
import { getActiveUserID, setActiveUserID } from '@/api/client';
import { setLocale as setVueI18nLocale, type SupportedLocale } from '@/i18n';
import { useUIStore } from './ui';

export const useUsersStore = defineStore('users', () => {
  const ui = useUIStore();

  const list = ref<User[]>([]);
  const activeId = ref<string | null>(getActiveUserID());
  const loading = ref(false);

  const active = computed<User | null>(() =>
    list.value.find((u) => u.id === activeId.value) ?? null,
  );

  async function refresh() {
    loading.value = true;
    try {
      list.value = await usersAPI.list();
      // If no active or active is stale, pick most-recently-used (or first).
      if (!activeId.value || !list.value.find((u) => u.id === activeId.value)) {
        const pick =
          [...list.value].sort((a, b) =>
            (b.lastUsedAt ?? b.createdAt).localeCompare(a.lastUsedAt ?? a.createdAt),
          )[0] ?? null;
        if (pick) {
          activeId.value = pick.id;
          setActiveUserID(pick.id);
        }
      }
      // Sync UI locale with active user's preference. If user has no language
      // set, leave vue-i18n at whatever localStorage chose at boot.
      // 给 vue-i18n 同步活跃 user 的语言偏好；user 无设置则保留 boot 时 localStorage 决策。
      const act = list.value.find((u) => u.id === activeId.value);
      if (act?.language) {
        setVueI18nLocale(act.language as SupportedLocale);
      }
    } catch (e) {
      ui.toast('err', `Load users failed: ${(e as Error).message}`);
    } finally {
      loading.value = false;
    }
  }

  async function create(in_: { username: string; displayName?: string; avatarColor?: string }) {
    const u = await usersAPI.create(in_);
    list.value.push(u);
    return u;
  }

  async function remove(id: string) {
    await usersAPI.remove(id);
    list.value = list.value.filter((u) => u.id !== id);
    if (activeId.value === id) {
      activeId.value = list.value[0]?.id ?? null;
      setActiveUserID(activeId.value);
    }
  }

  async function switchTo(id: string) {
    if (id === activeId.value) return;
    try {
      await usersAPI.activate(id);
    } catch {
      // best-effort: touch lastUsedAt; don't block switch on failure.
      // best-effort：尽量更新 lastUsedAt；失败不挡切换。
    }
    setActiveUserID(id);
    activeId.value = id;
    // Reload to drop all in-memory state from other stores (conv, chat, etc.).
    // 整页 reload 把其他 store 的内存态清干净（conv / chat / 等）。
    window.location.reload();
  }

  async function setLanguage(id: string, lang: SupportedLocale) {
    try {
      const u = await usersAPI.update(id, { language: lang });
      const i = list.value.findIndex((x) => x.id === id);
      if (i >= 0) list.value[i] = u;
      if (id === activeId.value) {
        // Active user's language change applies immediately to vue-i18n.
        // 当前 active user 改语言：立即应用到 vue-i18n。
        setVueI18nLocale(lang);
      }
      return u;
    } catch (e) {
      ui.toast('err', `Update language failed: ${(e as Error).message}`);
      throw e;
    }
  }

  return { list, active, activeId, loading, refresh, create, remove, switchTo, setLanguage };
});
