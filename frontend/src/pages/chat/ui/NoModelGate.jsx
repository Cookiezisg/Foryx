// NoModelGate — shown when API keys exist but no model is configured for
// the chat scenario. onOpenSettings injected by ChatPage from AppShell.
//
// NoModelGate —— 有 key 但 chat scenario 未配模型时显示；onOpenSettings
// 从 AppShell 经 ChatPage 传入，pages 层零 app 依赖。

import { useTranslation } from "react-i18next";
import { Icon } from "../../../components/primitives/Icon.jsx";
import { Button } from "../../../components/primitives/Button.jsx";

export function NoModelGate({ onOpenSettings }) {
  const { t } = useTranslation("conv");
  return (
    <div className="empty-shell">
      <div className="empty-shell-card">
        <div className="empty-shell-logo" style={{ background: "var(--status-warn)" }}>
          <Icon.Sparkles />
        </div>
        <div>
          <div className="empty-shell-title">{t("noModel.title")}</div>
          <div className="empty-shell-sub">
            {t("noModel.sub")}
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <Button size="sm" variant="accent" onClick={onOpenSettings}>
            <Icon.ArrowRight /> {t("noModel.action")}
          </Button>
        </div>
      </div>
    </div>
  );
}
