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
import { useTranslation } from "react-i18next";
import { Sidebar } from "@/widgets/sidebar/Sidebar.jsx";
import { CommandPalette } from "@/widgets/command-palette/CommandPalette.jsx";
import { NotificationsDrawer } from "@/widgets/notifications-drawer/NotificationsDrawer.jsx";
import { ToastTray } from "@/widgets/toaster/ToastTray.jsx";
import { PaneFrame, PANE_META } from "./shell/PaneFrame.jsx";
import { PaneResize } from "./shell/PaneResize.jsx";
import { NarrowSwitch } from "./shell/NarrowSwitch.jsx";
import { Dashboard } from "@/pages/dashboard/Dashboard.jsx";
import { ChatPage } from "@/pages/chat/ChatPage.jsx";
import { ForgePage } from "@/pages/forge/ForgePage.jsx";
import { ExecutePage } from "@/pages/execute/ExecutePage.jsx";
import { SkillsPage } from "@/pages/library/SkillsPage.jsx";
import { McpPage } from "@/pages/library/McpPage.jsx";
import { MemoryPage } from "@/pages/library/MemoryPage.jsx";
import { DocumentsPage } from "@/pages/library/DocumentsPage.jsx";
import { ObservePage } from "@/pages/observe/ui/ObservePage.jsx";
import { AskUserModal } from "@/components/overlays/AskUserModal.jsx";
import { SettingsModal } from "@/components/overlays/SettingsModal.jsx";
import { usePaneStore, useSidebarStore, useOverlayStore } from "@app/model";
import { setNavigator } from "@shared/lib/navigation";
import { useKeyboardShortcuts } from "./lib/useKeyboardShortcuts.js";
import { easeOut } from "@/motion/tokens.js";

function renderPaneBody(kind, onClose, pageProps) {
  switch (kind) {
    case "chat":      return <ChatPage onClose={onClose} {...pageProps.chat} />;
    case "forge":     return <ForgePage {...pageProps.forge} />;
    case "execute":   return <ExecutePage {...pageProps.execute} />;
    case "documents": return <DocumentsPage {...pageProps.documents} />;
    case "skills":    return <SkillsPage />;
    case "mcp":       return <McpPage />;
    case "memory":    return <MemoryPage />;
    case "observe":   return <ObservePage />;
    default:          return null;
  }
}

export function AppShell() {
  const { t } = useTranslation("sidebar");
  const openPanes = usePaneStore((s) => s.openPanes);
  const narrow = usePaneStore((s) => s.narrow);
  const setNarrow = usePaneStore((s) => s.setNarrow);
  const leftPct = usePaneStore((s) => s.leftPct);
  const setLeftPct = usePaneStore((s) => s.setLeftPct);
  const collapsed = useSidebarStore((s) => s.collapsed);
  const activeNarrowPane = usePaneStore((s) => s.activeNarrowPane);
  const closePane = usePaneStore((s) => s.closePane);
  const setActiveNarrowPane = usePaneStore((s) => s.setActiveNarrowPane);

  // Sidebar state
  const activeConv = usePaneStore((s) => s.activeConv);
  const togglePane = usePaneStore((s) => s.togglePane);
  const openPane = usePaneStore((s) => s.openPane);
  const setActiveConv = usePaneStore((s) => s.setActiveConv);
  const toolsExpanded = useSidebarStore((s) => s.toolsExpanded);
  const recentExpanded = useSidebarStore((s) => s.recentExpanded);
  const archivedExpanded = useSidebarStore((s) => s.archivedExpanded);
  const setCollapsed = useSidebarStore((s) => s.setCollapsed);
  const setToolsExpanded = useSidebarStore((s) => s.setToolsExpanded);
  const setRecentExpanded = useSidebarStore((s) => s.setRecentExpanded);
  const setArchivedExpanded = useSidebarStore((s) => s.setArchivedExpanded);

  // Page-level entity state
  const focusEntity = usePaneStore((s) => s.focusEntity);
  const consumeFocusEntity = usePaneStore((s) => s.consumeFocusEntity);
  const activeDocument = usePaneStore((s) => s.activeDocument);
  const setActiveDocument = usePaneStore((s) => s.setActiveDocument);

  // Overlay state
  const cmdkOpen = useOverlayStore((s) => s.cmdkOpen);
  const notifsOpen = useOverlayStore((s) => s.notifsOpen);
  const pendingAsk = useOverlayStore((s) => s.pendingAsk);
  const setCmdkOpen = useOverlayStore((s) => s.setCmdkOpen);
  const setNotifsOpen = useOverlayStore((s) => s.setNotifsOpen);
  const setSettingsOpen = useOverlayStore((s) => s.setSettingsOpen);
  const setPendingAsk = useOverlayStore((s) => s.setPendingAsk);
  const openEntity = usePaneStore((s) => s.openEntity);

  const mainRef = useRef(null);

  useKeyboardShortcuts();

  useEffect(() => {
    setNavigator({
      openConv: (id) => {
        usePaneStore.getState().setActiveConv(id);
        usePaneStore.getState().openPane("chat");
      },
      openEntity: (pane, id) => usePaneStore.getState().openEntity(pane, id),
      openPane: (pane) => usePaneStore.getState().openPane(pane),
      setActiveDocument: (id) => {
        usePaneStore.getState().setActiveDocument(id);
        usePaneStore.getState().openPane("documents");
      },
    });
  }, []);

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

  const pageProps = {
    chat: { activeConv, onSetActiveConv: setActiveConv, onOpenSettings: () => setSettingsOpen(true) },
    forge: { focusEntity, onConsumeFocusEntity: consumeFocusEntity },
    execute: { focusEntity, onConsumeFocusEntity: consumeFocusEntity },
    documents: { activeDoc: activeDocument, onSetActiveDocument: setActiveDocument },
  };

  return (
    <div className={"app" + (collapsed ? " is-collapsed" : "") + (narrow ? " is-narrow" : "")}>
      <Sidebar
        openPanes={openPanes}
        activeConv={activeConv}
        collapsed={collapsed}
        toolsExpanded={toolsExpanded}
        recentExpanded={recentExpanded}
        archivedExpanded={archivedExpanded}
        onTogglePane={togglePane}
        onOpenPane={openPane}
        onSetActiveConv={setActiveConv}
        onSetCollapsed={setCollapsed}
        onSetToolsExpanded={setToolsExpanded}
        onSetRecentExpanded={setRecentExpanded}
        onSetArchivedExpanded={setArchivedExpanded}
        onOpenCmdk={() => setCmdkOpen(true)}
        onOpenNotifs={() => setNotifsOpen(true)}
        onOpenSettings={() => setSettingsOpen(true)}
      />

      <main className="main" ref={mainRef}>
        {openPanes.length === 0 ? (
          <Dashboard onOpenPane={openPane} onSetActiveConv={setActiveConv} />
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
                    crumbs={[(() => { const m = PANE_META[kind]; return m ? (m.labelKey ? t(m.labelKey) : (m.label || kind)) : kind; })()]}
                  >
                    {renderPaneBody(kind, () => closePane(kind), pageProps)}
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

      <CommandPalette
        open={cmdkOpen}
        onClose={() => setCmdkOpen(false)}
        onOpenPane={openPane}
        onOpenEntity={openEntity}
        onSetActiveConv={setActiveConv}
        onOpenSettings={() => setSettingsOpen(true)}
      />
      <NotificationsDrawer
        open={notifsOpen}
        onClose={() => setNotifsOpen(false)}
        onOpenPane={openPane}
        onOpenEntity={openEntity}
        onSetActiveConv={setActiveConv}
        pendingAsk={pendingAsk}
        onSetPendingAsk={setPendingAsk}
      />
      <AskUserModal />
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
