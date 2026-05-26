// Root component — boot-state machine (onboarding/booting/ready), theme
// propagation, SSE bootstrap, AppShell.
//
// 根组件 —— 启动状态机；theme dataset 同步;挂 SSE;渲染 AppShell。

import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { AppShell } from "./components/layout/AppShell.jsx";
import { Onboarding } from "./components/overlays/Onboarding.jsx";
import { SSEProvider } from "./sse/SSEProvider.jsx";
import { useSettingsStore, applyTheme } from "@entities/settings";
import { useSessionStore } from "@entities/session";
import { useSessionBootstrap } from "@app/model";
import i18n from "@shared/lib/i18n";
import { useChatStore } from "./store/chat.js";
import { usePaneStore } from "@app/model";

export default function App() {
  const status = useSessionBootstrap();
  const prefs = useSettingsStore();
  const qc = useQueryClient();
  const prevUid = useRef(useSessionStore.getState().currentUserId);

  useEffect(() => {
    applyTheme(prefs);
  }, [prefs.theme, prefs.accent, prefs.density, prefs.lang]);

  useEffect(() => {
    i18n.changeLanguage(prefs.lang);
  }, [prefs.lang]);

  useEffect(() => {
    if (prefs.theme !== "system") return;
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const fn = () => applyTheme(prefs);
    mql.addEventListener?.("change", fn);
    return () => mql.removeEventListener?.("change", fn);
  }, [prefs.theme]);

  // Account switch: drop old user's chat tree, invalidate every REST cache,
  // clear cross-user pane state (stale activeConv would 404 on send).
  // Subscribes to session store so currentUserId change triggers cleanup.
  //
  // 切账号：清 chat store + 失效所有 query + 清 cross-user 残留 pane 状态。
  const currentUserId = useSessionStore((s) => s.currentUserId);
  useEffect(() => {
    if (prevUid.current === currentUserId) return;
    prevUid.current = currentUserId;
    useChatStore.getState().resetAll();
    const pane = usePaneStore.getState();
    pane.setActiveConv(null);
    pane.setActiveFlowRun(null);
    pane.setActiveDocument(null);
    qc.invalidateQueries();
  }, [currentUserId, qc]);

  if (status === "onboarding") {
    return (
      <SSEProvider>
        <Onboarding />
      </SSEProvider>
    );
  }
  if (status === "loading") {
    return <SSEProvider><div className="app-booting" /></SSEProvider>;
  }
  return (
    <SSEProvider>
      <AppShell />
    </SSEProvider>
  );
}
