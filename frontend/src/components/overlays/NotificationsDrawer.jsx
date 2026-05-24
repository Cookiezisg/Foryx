// NotificationsDrawer — right-side slide-in drawer listing notifications
// from REST snapshot (initial) plus any new ones in the local SSE buffer.
//
// NotificationsDrawer —— 右滑抽屉；REST 快照 + 本地 SSE 累积。

import { AnimatePresence, motion } from "framer-motion";
import { Icon } from "../primitives/Icon.jsx";
import { Badge } from "../primitives/Badge.jsx";
import { RelTime } from "../shared/RelTime.jsx";
import { useUIStore } from "../../store/ui.js";
import { useSSEHealth } from "../../sse/SSEProvider.jsx";
import { useNotificationsSnapshot } from "../../api/notifications.js";

const TYPE_TO_PANE = {
  conversation: "chat",
  function: "forge",
  handler: "forge",
  workflow: "forge",
  flowrun: "execute",
  mcp_server: "mcp",
  skill: "skills",
  memory: "memory",
  todo: "execute",
  ask: "chat",
};

const TYPE_TO_ICON = {
  conversation: Icon.MessageSquare,
  function: Icon.Code,
  handler: Icon.Server,
  workflow: Icon.Workflow,
  flowrun: Icon.Play,
  mcp_server: Icon.Server,
  skill: Icon.Sparkles,
  memory: Icon.Brain,
  todo: Icon.ListChecks,
  ask: Icon.HelpCircle,
  catalog: Icon.Folder,
  sandbox_env: Icon.Boxes,
  compaction: Icon.Archive,
};

export function NotificationsDrawer() {
  const open = useUIStore((s) => s.notifsOpen);
  const setOpen = useUIStore((s) => s.setNotifsOpen);
  const openPane = useUIStore((s) => s.openPane);
  const openEntity = useUIStore((s) => s.openEntity);
  const setActiveConv = useUIStore((s) => s.setActiveConv);

  const { unread, clearUnread } = useSSEHealth();
  const { data: snapshot = [] } = useNotificationsSnapshot(50);

  const onClick = (n) => {
    const pane = TYPE_TO_PANE[n.type];
    if (!pane) return;
    if (n.type === "conversation" && n.id) {
      setActiveConv(n.id);
      openPane("chat");
    } else if (pane && n.id) {
      openEntity(pane, n.id);
    } else {
      openPane(pane);
    }
    setOpen(false);
  };

  const onClose = () => { setOpen(false); clearUnread(); };

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="drawer-wrap is-open"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.18 }}
        >
          <div className="drawer-scrim" onClick={onClose} />
          <motion.div
            className="drawer"
            initial={{ x: 360 }}
            animate={{ x: 0 }}
            exit={{ x: 360 }}
            transition={{ duration: 0.24, ease: [0.2, 0.8, 0.2, 1] }}
          >
            <div className="drawer-head">
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <Icon.Bell style={{ width: 14, height: 14, color: "var(--fg-muted)" }} />
                <div className="drawer-title">通知</div>
                {unread > 0 && <Badge kind="muted">{unread} 未读</Badge>}
              </div>
              <div style={{ display: "flex", gap: 4 }}>
                <button className="btn btn-xs btn-ghost" onClick={clearUnread}>全部已读</button>
                <button className="icon-btn" onClick={onClose} title="关闭"><Icon.X /></button>
              </div>
            </div>
            <div className="drawer-list">
              {snapshot.length === 0 && (
                <div style={{ padding: 32, textAlign: "center", color: "var(--fg-faint)", fontSize: 12 }}>
                  这里很安静。
                </div>
              )}
              {snapshot.map((n) => {
                const I = TYPE_TO_ICON[n.type] || Icon.Bell;
                return (
                  <div key={n.seq} className="notif" onClick={() => onClick(n)}>
                    <div className="icon-wrap"><I /></div>
                    <div className="meta">
                      <div className="row">
                        <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-muted)" }}>
                          {n.type}
                        </span>
                        {n.data?.action && (
                          <span style={{ marginLeft: 6, color: "var(--fg-faint)" }}>{n.data.action}</span>
                        )}
                      </div>
                      <div className="desc">
                        {n.id || n.conversationId || ""}
                      </div>
                      <div className="time">
                        <RelTime ts={n.createdAt || n.at} />
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
