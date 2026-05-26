// NoApiKeyGate — first-run friendly empty state shown when no API key
// is configured. Click opens the settings modal via injected callback.
//
// NoApiKeyGate —— 没有任何 API key 时显示的首次运行引导；onOpenSettings 由
// ChatPage 从 AppShell 向下传入，pages 层零 app 依赖。

import { useTranslation } from "react-i18next";
import { Icon } from "../../../components/primitives/Icon.jsx";
import { Button } from "../../../components/primitives/Button.jsx";

export function NoApiKeyGate({ onOpenSettings }) {
  const { t } = useTranslation("conv");
  return (
    <div className="empty-shell">
      <div className="empty-shell-card">
        <div className="empty-shell-logo" style={{ background: "var(--status-warn)" }}>
          <Icon.KeyRound />
        </div>
        <div>
          <div className="empty-shell-title">{t("noApiKey.title")}</div>
          <div className="empty-shell-sub">
            {t("noApiKey.sub")} <code style={{ fontFamily: "var(--font-mono)" }}>~/.forgify/</code>
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <Button size="sm" variant="accent" onClick={onOpenSettings}>
            <Icon.Plus /> {t("noApiKey.action")}
          </Button>
        </div>
      </div>
    </div>
  );
}
