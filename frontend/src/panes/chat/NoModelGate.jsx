// NoModelGate — shown when API keys exist but no model is configured for
// the chat scenario (typically after onboarding skipped or test failed).
// Mirrors NoApiKeyGate's shape so the empty states feel consistent.
//
// NoModelGate —— 有 key 但 chat scenario 未配模型时显示;形态对齐 NoApiKeyGate
// 保持空状态一致(常见于 onboarding 跳过或 testKey 失败两条路径)。

import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { useUIStore } from "../../store/ui.js";

export function NoModelGate() {
  const openPane = useUIStore((s) => s.openPane);
  return (
    <div className="empty-shell">
      <div className="empty-shell-card">
        <div className="empty-shell-logo" style={{ background: "var(--status-warn)" }}>
          <Icon.Sparkles />
        </div>
        <div>
          <div className="empty-shell-title">挑一个模型</div>
          <div className="empty-shell-sub">
            钥匙在了。挑一个用来聊天的模型,就能开始。
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <Button size="sm" variant="accent" onClick={() => openPane("config")}>
            <Icon.ArrowRight /> 去挑一个
          </Button>
        </div>
      </div>
    </div>
  );
}
