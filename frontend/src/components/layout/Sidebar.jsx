// Sidebar — left navigation rail. Real conversation list via
// useConversations(); SSE notifications invalidate this query.
//
// Sidebar —— 左侧导航；真实对话列表；SSE 通知触发 invalidation。

import { useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Icon } from "../primitives/Icon.jsx";
import { Kbd } from "../primitives/Kbd.jsx";
import { useUIStore } from "../../store/ui.js";
import { useConversations, useCreateConversation } from "../../api/conversations.js";
import { ChatListItem } from "./ChatListItem.jsx";
import { useSSEHealth } from "../../sse/SSEProvider.jsx";
import { spring, easeOut } from "../../motion/tokens.js";

const SSE_DOT_COLOR = {
  ok:      "var(--status-success)",
  warn:    "var(--status-warn)",
  err:     "var(--status-error)",
  unknown: "var(--fg-faint)",
};
const SSE_DOT_TITLE = {
  ok:      "在线",
  warn:    "连接中",
  err:     "离线",
  unknown: "—",
};

function NavItem({ icon: I, label, active, onClick, badge }) {
  return (
    <button
      className={"nav-item" + (active ? " is-active" : "")}
      onClick={onClick}
    >
      {I && <I className="icon" />}
      <span className="label">{label}</span>
      {badge != null && <span className="badge">{badge}</span>}
    </button>
  );
}

export function Sidebar() {
  const openPanes = useUIStore((s) => s.openPanes);
  const togglePane = useUIStore((s) => s.togglePane);
  const openPane = useUIStore((s) => s.openPane);
  const setActiveConv = useUIStore((s) => s.setActiveConv);
  const collapsed = useUIStore((s) => s.collapsed);
  const setCmdkOpen = useUIStore((s) => s.setCmdkOpen);
  const setNotifsOpen = useUIStore((s) => s.setNotifsOpen);
  const setAskOpen = useUIStore((s) => s.setAskOpen);
  const setSettingsPopOpen = useUIStore((s) => s.setSettingsPopOpen);
  const settingsPopOpen = useUIStore((s) => s.settingsPopOpen);

  const { data: conversations = [], isLoading } = useConversations();
  const createConv = useCreateConversation();
  const sse = useSSEHealth();

  const [showArchived, setShowArchived] = useState(false);

  const pinned = conversations.filter((c) => c.pinned && !c.archived);
  const recent = conversations.filter((c) => !c.pinned && !c.archived);
  const archived = conversations.filter((c) => c.archived);

  const isOpen = (k) => openPanes.includes(k);

  const onNewConv = async () => {
    try {
      const created = await createConv.mutateAsync({});
      if (created?.id) {
        setActiveConv(created.id);
        if (!openPanes.includes("chat")) openPane("chat");
      }
    } catch (err) {
      console.error("create conv failed", err);
    }
  };

  return (
    <motion.aside
      className={"sidebar" + (collapsed ? " is-collapsed" : "")}
      animate={{ width: collapsed ? 56 : 248 }}
      transition={spring}
      style={{ overflow: "hidden" }}
    >
      <div className="sidebar-header">
        <div className="workspace-pill">
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="workspace-name">Forgify</div>
            {!collapsed && (
              <div style={{ fontSize: 10, color: "var(--fg-faint)" }}>local</div>
            )}
          </div>
          {!collapsed && <Icon.ChevronDown style={{ width: 12, height: 12, color: "var(--fg-faint)" }} />}
        </div>

        {!collapsed && (
          <button className="cmdk-trigger" onClick={() => setCmdkOpen(true)}>
            <Icon.Search className="icon" />
            <span className="label">搜索 或 跳转</span>
            <Kbd>⌘</Kbd>
            <Kbd>K</Kbd>
          </button>
        )}
      </div>

      <div className="nav-section">
        <div style={{ height: 4 }} />
        <NavItem icon={Icon.MessageSquare} label="对话"  active={isOpen("chat")}      onClick={() => togglePane("chat")} />
        <NavItem icon={Icon.Hammer}        label="锻造"  active={isOpen("forge")}     onClick={() => togglePane("forge")} />
        <NavItem icon={Icon.Play}          label="执行"  active={isOpen("execute")}   onClick={() => togglePane("execute")} />
        <NavItem icon={Icon.FileText}      label="文档"  active={isOpen("documents")} onClick={() => togglePane("documents")} />
        <NavItem icon={Icon.GitBranch}     label="洞察"  active={isOpen("observe")}   onClick={() => togglePane("observe")} />
      </div>

      <div className="nav-section">
        {!collapsed && <div className="nav-section-title"><span>资源库</span></div>}
        <NavItem icon={Icon.Sparkles}      label="Skills" active={isOpen("skills")} onClick={() => togglePane("skills")} />
        <NavItem icon={Icon.Server}        label="MCP"    active={isOpen("mcp")}    onClick={() => togglePane("mcp")} />
        <NavItem icon={Icon.Brain}         label="Memory" active={isOpen("memory")} onClick={() => togglePane("memory")} />
      </div>

      <div className="nav-section nav-conv-section" style={{ overflowY: "auto", flex: 1, paddingBottom: 12 }}>
        {!collapsed && pinned.length > 0 && (
          <div className="nav-section-title"><span>置顶</span></div>
        )}
        <AnimatePresence initial={false}>
          {!collapsed && pinned.map((c) => (
            <motion.div key={c.id} layout
              initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -4 }}
              transition={easeOut}>
              <ChatListItem conv={c} />
            </motion.div>
          ))}
        </AnimatePresence>

        {!collapsed && (
          <div className="nav-section-title" style={{ marginTop: pinned.length ? 6 : 0 }}>
            <span>最近对话</span>
            <button className="add-btn" title="新对话" onClick={onNewConv}>
              <Icon.Plus />
            </button>
          </div>
        )}
        <AnimatePresence initial={false}>
          {!collapsed && recent.map((c) => (
            <motion.div key={c.id} layout
              initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -4 }}
              transition={easeOut}>
              <ChatListItem conv={c} />
            </motion.div>
          ))}
        </AnimatePresence>

        {!collapsed && !isLoading && recent.length === 0 && pinned.length === 0 && (
          <div style={{ padding: "12px 10px", fontSize: 11, color: "var(--fg-faint)", textAlign: "center" }}>
            还没有对话 — 点 <Icon.Plus style={{ display: "inline", verticalAlign: "-2px", width: 11, height: 11 }} /> 开始一段
          </div>
        )}

        {!collapsed && archived.length > 0 && (
          <>
            <button
              className="nav-section-title nav-section-toggle"
              onClick={() => setShowArchived((s) => !s)}
              style={{ marginTop: 10 }}
            >
              <span>归档 · {archived.length}</span>
              <Icon.ChevronRight
                className="chev"
                style={{
                  width: 11, height: 11,
                  transform: showArchived ? "rotate(90deg)" : "none",
                  transition: "transform 120ms",
                }}
              />
            </button>
            <AnimatePresence initial={false}>
              {showArchived && archived.map((c) => (
                <motion.div key={c.id} layout
                  initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -4 }}
                  transition={easeOut}>
                  <ChatListItem conv={c} />
                </motion.div>
              ))}
            </AnimatePresence>
          </>
        )}
      </div>

      <div className="sidebar-footer">
        <div className="user-pill">
          <div className="user-avatar">S</div>
          {!collapsed && <div className="user-name">本地</div>}
          {!collapsed && (
            <>
              <span
                className="user-status"
                style={{ background: SSE_DOT_COLOR[sse.overall] }}
                title={SSE_DOT_TITLE[sse.overall] +
                  ` · eventlog ${sse.eventlog} · notifs ${sse.notifs} · forge ${sse.forge}`}
              />
              <div style={{ flex: 1 }} />
            </>
          )}
          <button className="icon-btn" onClick={() => setAskOpen(true)} title="agent 在等你回答">
            <Icon.HelpCircle />
          </button>
          <button
            className="icon-btn"
            onClick={() => { setNotifsOpen(true); sse.clearUnread(); }}
            title={sse.unread > 0 ? `通知 · ${sse.unread} 未读` : "通知"}
            style={{ position: "relative" }}
          >
            <Icon.Bell />
            {sse.unread > 0 && (
              <span style={{
                position: "absolute", top: 4, right: 4,
                width: 6, height: 6, borderRadius: "50%",
                background: "var(--accent)",
              }} />
            )}
          </button>
          <button
            className="icon-btn"
            onClick={() => setSettingsPopOpen(!settingsPopOpen)}
            title="账号 / 外观 / 完整设置"
          >
            <Icon.Settings />
          </button>
        </div>
      </div>
    </motion.aside>
  );
}
