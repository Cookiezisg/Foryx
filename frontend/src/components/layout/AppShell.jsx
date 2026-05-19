// AppShell — sidebar | main grid. main hosts 0/1/2 panes side-by-side
// with a draggable resize handle between them. Below 1000px main width,
// "narrow" mode collapses to a single visible pane with a bottom tab
// switcher. Pane mount/unmount animates via Framer Motion.
//
// AppShell —— sidebar | main 网格。main 装 0/1/2 个 pane（可拖宽中线）。
// main 宽 < 1000px → narrow 模式只显示一个 pane + 底部 tab 切换。
// pane 进出动画走 Framer Motion。

import { useEffect, useRef } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Sidebar } from "./Sidebar.jsx";
import { PaneFrame, PANE_META } from "./PaneFrame.jsx";
import { PaneResize } from "./PaneResize.jsx";
import { NarrowSwitch } from "./NarrowSwitch.jsx";
import { Dashboard } from "../../panes/dashboard/Dashboard.jsx";
import { PlaceholderPane } from "../../panes/PlaceholderPane.jsx";
import { ChatPane } from "../../panes/chat/ChatPane.jsx";
import { ForgePane } from "../../panes/forge/ForgePane.jsx";
import { useUIStore } from "../../store/ui.js";
import { useKeyboardShortcuts } from "../../hooks/useKeyboardShortcuts.js";
import { easeOut } from "../../motion/tokens.js";

function renderPaneBody(kind, onClose) {
  switch (kind) {
    case "chat":      return <ChatPane onClose={onClose} />;
    case "forge":     return <ForgePane />;
    case "execute":   return <PlaceholderPane title="执行" phase="Phase 7" lead="FlowRuns / Approvals / Triggers" />;
    case "documents": return <PlaceholderPane title="文档" phase="Phase 9" lead="LLM-ranked attach (Phase 5 backend)" />;
    case "skills":    return <PlaceholderPane title="Skills" phase="Phase 9" lead="SKILL.md library" />;
    case "mcp":       return <PlaceholderPane title="MCP" phase="Phase 9" lead="MCP servers + health" />;
    case "memory":    return <PlaceholderPane title="Memory" phase="Phase 9" lead="user / feedback / project / reference" />;
    case "observe":   return <PlaceholderPane title="洞察" phase="Phase 5 backend" lead="metrics dashboard" />;
    case "config":    return <PlaceholderPane title="设置" phase="Phase 9" lead="API keys / model / sandbox / 外观 / 数据" />;
    default:          return null;
  }
}

export function AppShell() {
  const openPanes = useUIStore((s) => s.openPanes);
  const narrow = useUIStore((s) => s.narrow);
  const setNarrow = useUIStore((s) => s.setNarrow);
  const leftPct = useUIStore((s) => s.leftPct);
  const setLeftPct = useUIStore((s) => s.setLeftPct);
  const collapsed = useUIStore((s) => s.collapsed);
  const activeNarrowPane = useUIStore((s) => s.activeNarrowPane);
  const closePane = useUIStore((s) => s.closePane);
  const setActiveNarrowPane = useUIStore((s) => s.setActiveNarrowPane);

  const mainRef = useRef(null);

  useKeyboardShortcuts();

  useEffect(() => {
    if (!mainRef.current) return;
    const check = () => {
      const w = mainRef.current?.clientWidth ?? 0;
      setNarrow(w < 1000);
    };
    check();
    const ro = new ResizeObserver(check);
    ro.observe(mainRef.current);
    return () => ro.disconnect();
  }, [setNarrow]);

  useEffect(() => {
    if (narrow && openPanes.length === 2 && !activeNarrowPane) {
      setActiveNarrowPane(openPanes[openPanes.length - 1]);
    }
  }, [narrow, openPanes, activeNarrowPane, setActiveNarrowPane]);

  const onPaneDrag = (clientX) => {
    if (!mainRef.current) return;
    const r = mainRef.current.getBoundingClientRect();
    const pct = ((clientX - r.left) / r.width) * 100;
    setLeftPct(pct);
  };

  const isTwoPane = openPanes.length === 2 && !narrow;

  return (
    <div className={"app" + (collapsed ? " is-collapsed" : "") + (narrow ? " is-narrow" : "")}>
      <Sidebar />

      <main className="main" ref={mainRef}>
        {openPanes.length === 0 ? (
          <Dashboard />
        ) : (
          <AnimatePresence mode="popLayout" initial={false}>
            {openPanes.map((kind, idx) => {
              const hideInNarrow = narrow && openPanes.length === 2 && activeNarrowPane && activeNarrowPane !== kind;
              if (hideInNarrow) return null;

              const style = isTwoPane
                ? idx === 0
                  ? { flex: `0 0 calc(${leftPct}% - 2px)`, display: "flex", flexDirection: "column" }
                  : { flex: "1 1 auto", display: "flex", flexDirection: "column" }
                : { flex: "1 1 auto", display: "flex", flexDirection: "column" };

              return (
                <motion.div
                  key={kind}
                  layout
                  initial={{ opacity: 0, scale: 0.985 }}
                  animate={{ opacity: 1, scale: 1 }}
                  exit={{ opacity: 0, scale: 0.96 }}
                  transition={easeOut}
                  className={"pane-wrap" + (idx === 1 ? " is-secondary" : "")}
                  style={style}
                >
                  <PaneFrame
                    kind={kind}
                    onClose={() => closePane(kind)}
                    crumbs={[(PANE_META[kind] || { label: kind }).label]}
                  >
                    {renderPaneBody(kind, () => closePane(kind))}
                  </PaneFrame>
                  {isTwoPane && idx === 0 && (
                    <PaneResizeBetween key="resize-between" onDrag={onPaneDrag} />
                  )}
                </motion.div>
              );
            })}
          </AnimatePresence>
        )}
      </main>

      <NarrowSwitch />
    </div>
  );
}

// PaneResizeBetween — wraps PaneResize so it can be slotted inside the
// flex flow without breaking layout. The resize handle is a 4px vertical
// strip absolutely positioned at the right edge of the first pane.
//
// PaneResizeBetween —— 把 PaneResize 包成绝对定位条，贴在左 pane 右边。
function PaneResizeBetween({ onDrag }) {
  return (
    <div
      style={{
        position: "absolute",
        top: 0, right: -2, bottom: 0,
        width: 4, zIndex: 5,
      }}
    >
      <PaneResize onDrag={onDrag} />
    </div>
  );
}
