// PaneFrame — the chrome shared by every non-chat pane: a thin top bar
// with crumbs + close. Chat skips the bar (its own header is taller).
//
// PaneFrame —— 非 chat pane 的统一外壳：薄顶栏 + 面包屑 + 关闭。
// chat 有自己的 header，不走 pane-bar。

import React from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";

export const PANE_META = {
  chat:      { icon: "MessageSquare", labelKey: "pane.chat" },
  forge:     { icon: "Hammer",        labelKey: "pane.forge" },
  execute:   { icon: "Play",          labelKey: "pane.execute" },
  documents: { icon: "FileText",      labelKey: "pane.documents" },
  skills:    { icon: "Sparkles",      label: "Skills" },
  mcp:       { icon: "Server",        label: "MCP" },
  memory:    { icon: "Brain",         label: "Memory" },
  observe:   { icon: "Activity",      labelKey: "pane.observe" },
};

export function PaneFrame({ kind, onClose, crumbs, children }: { kind: string; onClose: () => void; crumbs?: string[]; children?: React.ReactNode }) {
  const { t } = useTranslation("sidebar");
  const meta = (PANE_META as Record<string, { icon: string; labelKey?: string; label?: string }>)[kind] || { icon: "Square", label: kind };
  const I = (Icon as Record<string, React.ComponentType<{ className?: string }>>)[meta.icon] || Icon.MoreHorizontal;

  const metaLabel = meta.labelKey ? t(meta.labelKey) : (meta.label || kind);

  if (kind === "chat") {
    return (
      <div className="pane" data-kind={kind}>
        <div className="pane-body">{children}</div>
      </div>
    );
  }

  const cs = crumbs && crumbs.length > 0 ? crumbs : [metaLabel];

  return (
    <div className="pane" data-kind={kind}>
      <div className="pane-bar">
        <div className="pane-crumbs">
          <I className="icon" />
          <span className={cs.length === 1 ? "cur" : ""}>{cs[0]}</span>
          {cs.slice(1).map((c: string, i: number) => (
            <span key={i} style={{ display: "contents" }}>
              <Icon.ChevronRight className="sep" />
              <span className={i === cs.length - 2 ? "cur" : ""}>{c}</span>
            </span>
          ))}
        </div>
        <div className="pane-actions">
          <button className="icon-btn" title={t("paneBar.more")}><Icon.MoreHorizontal /></button>
          <button className="icon-btn" title={t("common:close")} onClick={onClose}><Icon.X /></button>
        </div>
      </div>
      <div className="pane-body">{children}</div>
    </div>
  );
}
