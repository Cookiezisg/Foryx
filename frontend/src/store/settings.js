// Settings — persisted user preferences (theme/accent/density/lang/etc).
// Written to localStorage via zustand persist; applied to
// documentElement.dataset.* so CSS variant rules switch in real time.
//
// 用户偏好；localStorage 持久化；变更写 documentElement.dataset.* 触发 CSS
// 变体规则。

import { create } from "zustand";
import { persist } from "zustand/middleware";

const DEFAULTS = {
  theme: "system",       // "system" | "light" | "dark"
  accent: "claude",      // "claude" | "blue" | "ink" | "green" | "purple"
  density: "cozy",       // "compact" | "cozy" | "comfortable"
  lang: "zh",            // "zh" | "en"
  reasoningDefault: "collapsed", // "collapsed" | "expanded"
  leftPct: 50,           // saved pane split
  activeUserId: null,    // local profile id; null → backend default local-user
  onboarded: false,      // first-run wizard completed flag
};

export const useSettings = create(
  persist(
    (set) => ({
      ...DEFAULTS,
      set: (patch) => set((s) => ({ ...s, ...patch })),
      reset: () => set(DEFAULTS),
    }),
    { name: "forgify-settings", version: 1 }
  )
);

// resolveTheme — collapses "system" to "light"/"dark" using prefers-color-scheme.
export function resolveTheme(theme) {
  if (theme !== "system") return theme;
  if (typeof window === "undefined") return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

// applyTheme — write theme/accent/density data-attrs to <html>.
// Idempotent; safe to call on every settings change.
export function applyTheme(settings) {
  const root = document.documentElement;
  root.dataset.theme = resolveTheme(settings.theme);
  root.dataset.accent = settings.accent;
  root.dataset.density = settings.density;
  root.dataset.lang = settings.lang;
}
