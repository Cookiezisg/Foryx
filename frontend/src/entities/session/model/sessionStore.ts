import { create } from "zustand";
import { persist } from "zustand/middleware";

export interface SessionState {
  currentUserId: string | null;
  status: "loading" | "onboarding" | "ready";
  setCurrentUser(id: string | null): void;
  setStatus(s: SessionState["status"]): void;
}

export const useSessionStore = create<SessionState>()(
  persist(
    (set) => ({
      currentUserId: null as string | null,
      status: "loading" as SessionState["status"],
      setCurrentUser: (id) => set({ currentUserId: id }),
      setStatus: (status) => set({ status }),
    }),
    { name: "forgify-session", partialize: (s) => ({ currentUserId: s.currentUserId }) },
  ),
);
