// UI state — pane system, active conversation, baseUrl, narrow mode,
// per-pane focused entity, overlay flags. Owned by zustand because it is
// session-scoped state that drives most of the shell.
//
// UI 状态 —— pane 系统、活跃会话、baseUrl、narrow 模式、各 pane 待 focus
// 实体、overlay 开关。zustand 管 —— 会话级状态，驱动 shell。

import { create } from "zustand";

const MAX_PANES = 2;

function readBool(key, fallback) {
  try {
    const v = localStorage.getItem(key);
    if (v === null) return fallback;
    return v === "1";
  } catch { return fallback; }
}
function writeBool(key, value) {
  try { localStorage.setItem(key, value ? "1" : "0"); } catch {}
}

export const useUIStore = create((set, get) => ({
  baseUrl: null,
  openPanes: ["chat"],
  activeConv: null,
  activeFlowRun: null,
  activeDocument: null,
  leftPct: 50,
  collapsed:      readBool("sidebar.collapsed",      false),
  toolsExpanded:  readBool("sidebar.toolsExpanded",  true),
  recentExpanded: readBool("sidebar.recentExpanded", true),
  narrow: false,
  activeNarrowPane: null,
  focusEntity: {},

  // overlays
  cmdkOpen: false,
  notifsOpen: false,
  askOpen: false,
  settingsPopOpen: false,
  pendingAsk: null,

  toasts: [],

  setBaseUrl: (url) => set({ baseUrl: url }),

  setActiveConv: (id) => set({ activeConv: id }),
  setActiveFlowRun: (id) => set({ activeFlowRun: id }),
  setActiveDocument: (id) => set({ activeDocument: id }),

  togglePane: (k) =>
    set((s) => {
      if (s.openPanes.includes(k)) {
        const next = s.openPanes.filter((x) => x !== k);
        const nextActive = s.activeNarrowPane === k ? next[next.length - 1] || null : s.activeNarrowPane;
        return { openPanes: next, activeNarrowPane: nextActive };
      }
      if (s.openPanes.length >= MAX_PANES) {
        return { openPanes: [s.openPanes[1], k], activeNarrowPane: k };
      }
      return { openPanes: [...s.openPanes, k], activeNarrowPane: k };
    }),

  openPane: (k) =>
    set((s) => {
      if (s.openPanes.includes(k)) return { activeNarrowPane: k };
      if (s.openPanes.length >= MAX_PANES) {
        return { openPanes: [s.openPanes[1], k], activeNarrowPane: k };
      }
      return { openPanes: [...s.openPanes, k], activeNarrowPane: k };
    }),

  closePane: (k) =>
    set((s) => {
      const next = s.openPanes.filter((x) => x !== k);
      const nextActive = s.activeNarrowPane === k ? next[next.length - 1] || null : s.activeNarrowPane;
      return { openPanes: next, activeNarrowPane: nextActive };
    }),

  openEntity: (pane, id) =>
    set((s) => {
      const focus = { ...s.focusEntity, [pane]: id };
      const open = s.openPanes.includes(pane) ? s.openPanes
        : s.openPanes.length >= MAX_PANES
          ? [s.openPanes[1], pane]
          : [...s.openPanes, pane];
      return { openPanes: open, focusEntity: focus, activeNarrowPane: pane };
    }),

  consumeFocusEntity: (pane) => {
    const id = get().focusEntity[pane];
    if (!id) return null;
    set((s) => {
      const next = { ...s.focusEntity };
      delete next[pane];
      return { focusEntity: next };
    });
    return id;
  },

  setLeftPct: (n) => set({ leftPct: Math.max(20, Math.min(80, n)) }),

  setCollapsed: (b) => {
    const next = typeof b === "function" ? b(get().collapsed) : !!b;
    writeBool("sidebar.collapsed", next);
    set({ collapsed: next });
  },

  setToolsExpanded: (b) => {
    const next = typeof b === "function" ? b(get().toolsExpanded) : !!b;
    writeBool("sidebar.toolsExpanded", next);
    set({ toolsExpanded: next });
  },

  setRecentExpanded: (b) => {
    const next = typeof b === "function" ? b(get().recentExpanded) : !!b;
    writeBool("sidebar.recentExpanded", next);
    set({ recentExpanded: next });
  },

  setNarrow: (b) => set({ narrow: !!b }),

  setActiveNarrowPane: (k) => set({ activeNarrowPane: k }),

  setCmdkOpen: (b) => set({ cmdkOpen: !!b }),
  setNotifsOpen: (b) => set({ notifsOpen: !!b }),
  setAskOpen: (b) => set({ askOpen: !!b }),
  setSettingsPopOpen: (b) => set({ settingsPopOpen: !!b }),
  setPendingAsk: (v) => set({ pendingAsk: v }),

  pushToast: (t) => {
    const id = Math.random().toString(36).slice(2, 9);
    const toast = { id, ...t };
    set((s) => ({ toasts: [...s.toasts, toast] }));
    if (t.duration !== 0) {
      setTimeout(() => {
        set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) }));
      }, t.duration || 5000);
    }
    return id;
  },
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) })),
}));
