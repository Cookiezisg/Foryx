// AppShell — sidebar | main grid. main hosts 0/1/2 panes side-by-side
// with a draggable resize handle between them. Below 1000px main width,
// "narrow" mode collapses to a single visible pane with a bottom tab
// switcher. Pane mount/unmount animates via Framer Motion.
//
// AppShell —— sidebar | main 网格。main 装 0/1/2 个 pane（可拖宽中线）。
// main 宽 < 1000px → narrow 模式只显示一个 pane + 底部 tab 切换。
// pane 进出动画走 Framer Motion。

import { useCallback, useEffect, useRef } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Sidebar } from "./Sidebar.jsx";
import { PaneFrame, PANE_META } from "./PaneFrame.jsx";
import { PaneResize } from "./PaneResize.jsx";
import { NarrowSwitch } from "./NarrowSwitch.jsx";
import { Dashboard } from "../../panes/dashboard/Dashboard.jsx";
import { PlaceholderPane } from "../../panes/PlaceholderPane.jsx";
import { ChatPane } from "../../panes/chat/ChatPane.jsx";
import { ForgePane } from "../../panes/forge/ForgePane.jsx";
import { ExecutePane } from "../../panes/execute/ExecutePane.jsx";
import { ConfigPane } from "../../panes/config/ConfigPane.jsx";
import { SkillsPane } from "../../panes/library/SkillsPane.jsx";
import { McpPane } from "../../panes/library/McpPane.jsx";
import { MemoryPane } from "../../panes/library/MemoryPane.jsx";
import { DocumentsPane } from "../../panes/library/DocumentsPane.jsx";
import { ObservePane } from "../../panes/observe/ObservePane.jsx";
import { CommandPalette } from "../overlays/CommandPalette.jsx";
import { NotificationsDrawer } from "../overlays/NotificationsDrawer.jsx";
import { AskUserModal } from "../overlays/AskUserModal.jsx";
import { ToastTray } from "../overlays/ToastTray.jsx";
import { SettingsPopover } from "../overlays/SettingsPopover.jsx";
import { SettingsModal } from "../overlays/SettingsModal.jsx";
import { useUIStore } from "../../store/ui.js";
import { useKeyboardShortcuts } from "../../hooks/useKeyboardShortcuts.js";
import { easeOut } from "../../motion/tokens.js";

function renderPaneBody(kind, onClose) {
  switch (kind) {
    case "chat":      return <ChatPane onClose={onClose} />;
    case "forge":     return <ForgePane />;
    case "execute":   return <ExecutePane />;
    case "documents": return <DocumentsPane />;
    case "skills":    return <SkillsPane />;
    case "mcp":       return <McpPane />;
    case "memory":    return <MemoryPane />;
    case "observe":   return <ObservePane />;
    case "config":    return <ConfigPane />;
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

  // Memoise so PaneResize's useEffect deps don't churn. Without this,
  // every leftPct update triggers a fresh onPaneDrag → PaneResize
  // re-attaches its window mousemove/mouseup listeners mid-drag →
  // mousemove events fired during the brief detach window are lost,
  // causing perceived stutter / completely dropped drags in fast
  // motion. setLeftPct is stable from zustand; mainRef is a stable ref.
  //
  // 必须 memoise；否则每次 leftPct 变就给 PaneResize 一个新 onDrag，
  // useEffect 反复 attach/detach 监听 → mousemove 丢事件，拖动卡顿。
  const onPaneDrag = useCallback((clientX) => {
    if (!mainRef.current) return;
    const r = mainRef.current.getBoundingClientRect();
    const pct = ((clientX - r.left) / r.width) * 100;
    setLeftPct(pct);
  }, [setLeftPct]);

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

              // position:relative is required so PaneResizeBetween's
              // absolute-positioned strip anchors to THIS pane's wrap
              // (otherwise it falls back to viewport and ends up 0-height
              // pinned to the right edge — drag becomes impossible).
              //
              // position:relative 必须；否则 PaneResizeBetween 的 absolute
              // 找不到祖先，挂到 viewport（贴右下、高度 0、拖不动）。
              const baseStyle = { display: "flex", flexDirection: "column", position: "relative" };
              const style = isTwoPane
                ? idx === 0
                  ? { ...baseStyle, flex: `0 0 calc(${leftPct}% - 2px)` }
                  : { ...baseStyle, flex: "1 1 auto" }
                : { ...baseStyle, flex: "1 1 auto" };

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

      <CommandPalette />
      <NotificationsDrawer />
      <AskUserModal />
      <SettingsPopover />
      <SettingsModal />
      <ToastTray />
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
