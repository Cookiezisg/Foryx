// useDisplayName — the active user's display name, read from the backend User
// entity (set during onboarding via createUser, editable in Settings). NOT a
// separate localStorage field: that duplicated the name in a disconnected
// place and showed "?" even for properly-onboarded users. Falls back to
// username, then "".
//
// useDisplayName —— 当前激活 user 的显示名,来自后端 User 实体(onboarding
// createUser 写入,设置里可改),不再用孤立的 localStorage 字段(那会让
// 走完引导的用户也显示 "?")。取不到 displayName 退到 username,再退到 ""。

import { useSessionStore } from "@entities/session/@x/user";
import { useUsers, useUpdateUser } from "@entities/user";

export function useDisplayName() {
  const activeUserId = useSessionStore((s) => s.currentUserId);
  const { data: users = [] } = useUsers();
  const update = useUpdateUser();

  const user = users.find((u) => u.id === activeUserId) || null;
  const value = user?.displayName || user?.username || "";

  // Persist to the backend User; useUpdateUser invalidates /users so the
  // footer / greeting refresh. No-op when empty or unchanged.
  const setValue = (next: string) => {
    const trimmed = (next || "").trim();
    if (activeUserId && trimmed && trimmed !== value) {
      update.mutate({ id: activeUserId, patch: { displayName: trimmed } });
    }
  };

  return [value, setValue];
}
