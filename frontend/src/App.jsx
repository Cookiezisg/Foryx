// Root component — first-run detection (Onboarding), theme propagation,
// SSE bootstrap, AppShell.
//
// 根组件 —— 首次启动 Onboarding；theme dataset 同步；挂 SSE；渲染 AppShell。

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AppShell } from "./components/layout/AppShell.jsx";
import { Onboarding } from "./components/overlays/Onboarding.jsx";
import { SSEProvider } from "./sse/SSEProvider.jsx";
import { useSettings, applyTheme } from "./store/settings.js";
import { apiFetch, qk, pickList } from "./api/client.js";

export default function App() {
  const settings = useSettings();
  const [forceShowOnboarding, setForceShowOnboarding] = useState(false);

  useEffect(() => {
    applyTheme(settings);
  }, [settings.theme, settings.accent, settings.density, settings.lang]);

  useEffect(() => {
    if (settings.theme !== "system") return;
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const fn = () => applyTheme(settings);
    mql.addEventListener?.("change", fn);
    return () => mql.removeEventListener?.("change", fn);
  }, [settings.theme]);

  // First-run detection — show Onboarding when settings.onboarded is
  // false AND the only user in the backend is the auto-created default.
  // (Backend always seeds a local-user on first boot, so 1 user with
  // username==="default" === fresh install.)
  //
  // 首次启动检测：onboarded=false 且后端只有自动建的 default user → 显示。
  const usersQ = useQuery({
    queryKey: qk.users(),
    queryFn: () => apiFetch("/users"),
    select: pickList,
  });
  const users = usersQ.data || [];
  const isFreshInstall = !settings.onboarded
    && users.length <= 1
    && (users[0]?.username === "default" || !users[0]);
  const showOnboarding = forceShowOnboarding || (isFreshInstall && !usersQ.isLoading);

  if (showOnboarding) {
    return (
      <SSEProvider>
        <Onboarding onFinish={() => setForceShowOnboarding(false)} />
      </SSEProvider>
    );
  }

  return (
    <SSEProvider>
      <AppShell />
    </SSEProvider>
  );
}
