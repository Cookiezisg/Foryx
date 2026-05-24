// NoApiKeyGate — first-run friendly empty state shown when no API key
// is configured. Click leads into Config pane.
//
// NoApiKeyGate —— 没有任何 API key 时显示的首次运行引导；点击进 Config pane。

import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { useUIStore } from "../../store/ui.js";

export function NoApiKeyGate() {
  const openPane = useUIStore((s) => s.openPane);
  return (
    <div className="empty-shell">
      <div className="empty-shell-card">
        <div className="empty-shell-logo" style={{ background: "var(--status-warn)" }}>
          <Icon.KeyRound />
        </div>
        <div>
          <div className="empty-shell-title">先配一把钥匙</div>
          <div className="empty-shell-sub">
            一把 LLM 的 API key。只存在这台电脑,放在 <code style={{ fontFamily: "var(--font-mono)" }}>~/.forgify/</code>。
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <Button size="sm" variant="accent" onClick={() => openPane("config")}>
            <Icon.Plus /> 去配置
          </Button>
        </div>
      </div>
    </div>
  );
}
