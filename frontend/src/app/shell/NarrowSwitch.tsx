// NarrowSwitch — bottom tab switcher visible only when narrow=true and 2
// panes are open. Tap a tab to make that pane visible (the other hides).
//
// NarrowSwitch —— narrow=true 且开 2 个 pane 时显示的底部 tab 条。

import { useTranslation } from "react-i18next";
import { PANE_META } from "./PaneFrame.tsx";
import { usePaneStore } from "@app/model";

export function NarrowSwitch() {
  const { t } = useTranslation("sidebar");
  // TODO(4b): pages props 化后移除 feature-tmp→app 过渡反向引用
  const openPanes = usePaneStore((s) => s.openPanes);
  const narrow = usePaneStore((s) => s.narrow);
  const activeNarrowPane = usePaneStore((s) => s.activeNarrowPane);
  const setActiveNarrowPane = usePaneStore((s) => s.setActiveNarrowPane);

  if (!narrow || openPanes.length < 2) return null;

  const paneLabel = (k: string) => {
    const meta = (PANE_META as Record<string, { icon: string; labelKey?: string; label?: string }>)[k];
    if (!meta) return k;
    return meta.labelKey ? t(meta.labelKey) : (meta.label || k);
  };

  return (
    <div className="narrow-switch">
      {openPanes.map((k) => (
        <button
          key={k}
          className={"narrow-switch-btn" + (activeNarrowPane === k ? " is-active" : "")}
          onClick={() => setActiveNarrowPane(k)}
        >
          {paneLabel(k)}
        </button>
      ))}
    </div>
  );
}
