// useDisplayName — local-only user display name kept in localStorage.
// Multiple instances stay in sync via a tiny in-module event bus.
//
// useDisplayName —— 本地单用户的显示名,走 localStorage;多个实例
// 通过模块内事件总线同步,避免不同组件读到不同值。

import { useEffect, useState } from "react";

const KEY = "forgify.user.displayName";
const listeners = new Set();

function read() {
  try { return localStorage.getItem(KEY) || ""; }
  catch { return ""; }
}

function write(value) {
  try {
    localStorage.setItem(KEY, value || "");
    listeners.forEach((fn) => fn(value || ""));
  } catch {
    // ignore quota / SSR
  }
}

export function useDisplayName() {
  const [value, setValue] = useState(read);

  useEffect(() => {
    const fn = (v) => setValue(v);
    listeners.add(fn);
    return () => listeners.delete(fn);
  }, []);

  return [value, write];
}
