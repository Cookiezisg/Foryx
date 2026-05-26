import { useCallback, useEffect, useState } from "react";

// useCollapsible — persisted open/closed state for a named UI region.
// Survives reload via localStorage; collapses default to user's previous
// preference, falling back to `defaultOpen` for first-run.
//
// useCollapsible —— 命名 UI 区域的折叠状态，localStorage 持久化。
// 优先用用户上次的选择，无记录时回退 defaultOpen。

const PREFIX = "forgify-collapse-";

export function useCollapsible(key: string, defaultOpen = true) {
  const storageKey = PREFIX + key;
  const [open, setOpen] = useState(() => {
    try {
      const raw = localStorage.getItem(storageKey);
      if (raw === null) return defaultOpen;
      return raw === "1";
    } catch { return defaultOpen; }
  });

  useEffect(() => {
    try { localStorage.setItem(storageKey, open ? "1" : "0"); } catch {}
  }, [storageKey, open]);

  const toggle = useCallback(() => setOpen((o) => !o), []);
  return [open, toggle, setOpen];
}
