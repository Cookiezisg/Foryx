/**
 * UI store — column widths, expand mode, dialogs, toasts.
 *
 * Persists col widths to localStorage so a refresh keeps the operator's
 * layout. Other UI state (open menus, current dialog) is ephemeral.
 */

import { defineStore } from 'pinia';
import { ref, computed, watch } from 'vue';

const LS_KEY = 'forgify-testend:ui:v2';

interface Persisted {
  colConv: number;
  colChat: number;
  colNav: number;
  expanded: boolean;
}

function loadPersisted(): Persisted {
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) return JSON.parse(raw) as Persisted;
  } catch {
    /* ignore */
  }
  return { colConv: 200, colChat: 520, colNav: 220, expanded: false };
}

interface Toast {
  id: number;
  kind: 'ok' | 'err' | 'info';
  message: string;
  ttlMs: number;
}

export const useUIStore = defineStore('ui', () => {
  const persisted = ref<Persisted>(loadPersisted());
  const colConv = computed({ get: () => persisted.value.colConv, set: (v) => (persisted.value.colConv = v) });
  const colChat = computed({ get: () => persisted.value.colChat, set: (v) => (persisted.value.colChat = v) });
  const colNav = computed({ get: () => persisted.value.colNav, set: (v) => (persisted.value.colNav = v) });
  const expanded = computed({ get: () => persisted.value.expanded, set: (v) => (persisted.value.expanded = v) });

  watch(persisted, () => {
    try {
      localStorage.setItem(LS_KEY, JSON.stringify(persisted.value));
    } catch {
      /* quota? ignore */
    }
  }, { deep: true });

  function resetColumns() {
    persisted.value = { colConv: 200, colChat: 520, colNav: 220, expanded: false };
  }

  function toggleExpanded() {
    persisted.value.expanded = !persisted.value.expanded;
  }

  /* toasts */
  const toasts = ref<Toast[]>([]);
  let toastSeq = 0;

  function toast(kind: Toast['kind'], message: string, ttlMs = 4000) {
    const id = ++toastSeq;
    toasts.value.push({ id, kind, message, ttlMs });
    setTimeout(() => {
      toasts.value = toasts.value.filter((t) => t.id !== id);
    }, ttlMs);
  }

  function dismissToast(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id);
  }

  /* command palette */
  const palette = ref(false);
  function openPalette() { palette.value = true; }
  function closePalette() { palette.value = false; }

  /* raw json modal */
  const rawJson = ref<{ open: boolean; title: string; data: unknown }>({
    open: false,
    title: '',
    data: null,
  });
  function showRaw(title: string, data: unknown) {
    rawJson.value = { open: true, title, data };
  }
  function closeRaw() {
    rawJson.value.open = false;
  }

  return {
    colConv, colChat, colNav, expanded,
    resetColumns, toggleExpanded,
    toasts, toast, dismissToast,
    palette, openPalette, closePalette,
    rawJson, showRaw, closeRaw,
  };
});
