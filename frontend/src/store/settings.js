// Session store — persisted session-local state (onboarded / leftPct).
// activeUserId has migrated to entities/session (useSessionStore). Preferences
// (theme/accent/density/lang/reasoningDefault) live in entities/settings.
//
// 会话持久化；activeUserId 已迁 entities/session；偏好字段已迁 entities/settings。

import { create } from "zustand";
import { persist } from "zustand/middleware";

const DEFAULTS = {
  onboarded: false,      // first-run wizard completed flag
  leftPct: 50,           // saved pane split (read by ui.js; kept here for legacy persist)
};

export const useSettings = create(
  persist(
    (set) => ({
      ...DEFAULTS,
      set: (patch) => set((s) => ({ ...s, ...patch })),
      reset: () => set(DEFAULTS),
    }),
    { name: "forgify-ui", version: 1 }
  )
);
