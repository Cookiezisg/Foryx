// Sidebar — Gemini-style left rail. Expanded 260px / collapsed 64px.
// Top logo morphs to PanelLeftClose/PanelLeftOpen on hover (no extra row).
// "工具" + "最近" are collapsible via SidebarSection. Footer shows avatar
// (with red-dot badge for combined Help+Bell unread) and reveals a ⚙
// settings button on hover.
//
// Sidebar —— Gemini-style 左栏。展开 260 / 收起 64。顶部 logo hover 变
// panel-toggle 切换收起;"工具" / "最近" 段可折叠。footer 头像角红 dot
// 是 Help+Bell 合并未读,hover 整行浮出 ⚙ 入设置。

import { motion } from "framer-motion";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { useConversations, useCreateConversation } from "@entities/conversation";
import { useDisplayName } from "@entities/user";
import { ChatListItem } from "./ChatListItem.tsx";
import { SidebarSection } from "./SidebarSection.tsx";

const SPRING = { type: "spring", stiffness: 280, damping: 28 };

function ForgifyLogo({ size = 22 }) {
  // Anvil + spark mark. Stroke matches Lucide outline weight.
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} fill="none"
      stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 2v3" /><path d="M5 5l2 2" /><path d="M19 5l-2 2" />
      <path d="M4 12h4l2-3l4 6l2-3h4" />
      <path d="M5 17h14" /><path d="M7 21l1-4" /><path d="M17 21l-1-4" />
    </svg>
  );
}

function NavItem({ icon: I, label, active, primary, onClick, collapsed }: { icon: any; label: string; active?: boolean; primary?: boolean; onClick?: () => void; collapsed?: boolean }) {
  const cls =
    "sb-item" +
    (active  ? " is-active"  : "") +
    (primary ? " is-primary" : "");
  return (
    <button type="button" className={cls} onClick={onClick} title={collapsed ? label : undefined}>
      <span className="ic-slot"><I size={18} strokeWidth={2} className="ic" /></span>
      {!collapsed && <span className="label">{label}</span>}
    </button>
  );
}

interface SidebarProps {
  openPanes: string[];
  activeConv: string | null;
  collapsed: boolean;
  toolsExpanded: boolean;
  recentExpanded: boolean;
  archivedExpanded: boolean;
  sseHealth?: any;
  onTogglePane: (pane: string) => void;
  onOpenPane: (pane: string) => void;
  onSetActiveConv: (id: string | null) => void;
  onSetCollapsed: (v: boolean) => void;
  onSetToolsExpanded: (v: boolean) => void;
  onSetRecentExpanded: (v: boolean) => void;
  onSetArchivedExpanded: (v: boolean) => void;
  onOpenCmdk: () => void;
  onOpenNotifs: () => void;
  onOpenSettings: () => void;
}

export function Sidebar({
  openPanes,
  activeConv,
  collapsed,
  toolsExpanded,
  recentExpanded,
  archivedExpanded,
  sseHealth,
  onTogglePane,
  onOpenPane,
  onSetActiveConv,
  onSetCollapsed,
  onSetToolsExpanded,
  onSetRecentExpanded,
  onSetArchivedExpanded,
  onOpenCmdk,
  onOpenNotifs,
  onOpenSettings,
}: SidebarProps) {
  const { t } = useTranslation("sidebar");

  const { data: conversations = [] } = useConversations();
  const createConv = useCreateConversation();
  const sse = sseHealth ?? { overall: "unknown", unread: 0, clearUnread: () => {} };
  const [displayName] = useDisplayName() as [string, any];

  const pinned   = conversations.filter((c) => c.pinned   && !c.archived);
  const recent   = conversations.filter((c) => !c.pinned  && !c.archived);
  const archived = conversations.filter((c) => c.archived);

  const isOpen = (k: string) => openPanes.includes(k);

  const onNewConv = async () => {
    try {
      const created = await createConv.mutateAsync({});
      if (created?.id) {
        onSetActiveConv(created.id);
        if (!isOpen("chat")) onOpenPane("chat");
      }
    } catch (err) {
      console.error("create conv failed", err);
    }
  };

  const initial = (displayName?.[0] || "?").toUpperCase();
  const unread = sse.unread || 0;
  const sseDot = sse.overall === "err" || sse.overall === "warn"
    ? `var(--status-${sse.overall === "err" ? "error" : "warn"})` : null;

  return (
    <motion.aside
      className={"sidebar" + (collapsed ? " is-collapsed" : "")}
      animate={{ width: collapsed ? 64 : 260 }}
      transition={SPRING}
      style={{ overflow: "hidden" }}
    >
      <div className="sb-head">
        {collapsed ? (
          // Collapsed: the brand mark IS the expand toggle. Same .sb-logo-mark
          // element + position as expanded (zero drift); hover overlays the
          // PanelLeftOpen icon in place without moving anything.
          <button
            type="button"
            className="sb-logo-btn"
            onClick={() => onSetCollapsed(false)}
            title={t("toggle.expand")}
            aria-label="toggle sidebar"
          >
            <span className="sb-logo-mark"><ForgifyLogo /></span>
            <span className="sb-logo-toggle"><Icon.PanelLeftOpen size={20} strokeWidth={2} /></span>
          </button>
        ) : (
          <>
            <span className="sb-logo-mark"><ForgifyLogo /></span>
            <span className="sb-logo-name">Forgify</span>
            <button
              type="button"
              className="sb-collapse-btn"
              onClick={() => onSetCollapsed(true)}
              title={t("toggle.collapse")}
              aria-label="toggle sidebar"
            >
              <Icon.PanelLeftClose size={18} strokeWidth={2} />
            </button>
          </>
        )}
      </div>

      <NavItem icon={Icon.SquarePen} label={t("nav.newChat")}      primary onClick={onNewConv} collapsed={collapsed} />
      <NavItem icon={Icon.Search}    label={t("nav.searchOrJump")} onClick={() => onOpenCmdk()} collapsed={collapsed} />

      <div style={{ height: 8 }} />

      <NavItem icon={Icon.MessageSquare} label={t("nav.chat")}      active={isOpen("chat")}      onClick={() => onTogglePane("chat")}      collapsed={collapsed} />
      <NavItem icon={Icon.Hammer}        label={t("nav.forge")}     active={isOpen("forge")}     onClick={() => onTogglePane("forge")}     collapsed={collapsed} />
      <NavItem icon={Icon.Play}          label={t("nav.execute")}   active={isOpen("execute")}   onClick={() => onTogglePane("execute")}   collapsed={collapsed} />
      <NavItem icon={Icon.FileText}      label={t("nav.documents")} active={isOpen("documents")} onClick={() => onTogglePane("documents")} collapsed={collapsed} />

      <SidebarSection label={t("nav.tools")} expanded={toolsExpanded} onToggle={() => onSetToolsExpanded(!toolsExpanded)} collapsedSidebar={collapsed}>
        <NavItem icon={Icon.BarChart3} label={t("nav.insights")} active={isOpen("observe")} onClick={() => onTogglePane("observe")} collapsed={collapsed} />
        <NavItem icon={Icon.Sparkles}  label="Skills"             active={isOpen("skills")} onClick={() => onTogglePane("skills")}  collapsed={collapsed} />
        <NavItem icon={Icon.Plug}      label="MCP"                active={isOpen("mcp")}    onClick={() => onTogglePane("mcp")}     collapsed={collapsed} />
        <NavItem icon={Icon.Brain}     label="Memory"             active={isOpen("memory")} onClick={() => onTogglePane("memory")}  collapsed={collapsed} />
      </SidebarSection>

      {!collapsed && (
        <div className="sb-recent-wrap">
          <SidebarSection label={t("nav.recent")} expanded={recentExpanded} onToggle={() => onSetRecentExpanded(!recentExpanded)}>
            {pinned.map((c) => <ChatListItem key={c.id} conv={c} openPanes={openPanes} activeConv={activeConv} onSetActiveConv={onSetActiveConv} onOpenPane={onOpenPane} />)}
            {recent.map((c) => <ChatListItem key={c.id} conv={c} openPanes={openPanes} activeConv={activeConv} onSetActiveConv={onSetActiveConv} onOpenPane={onOpenPane} />)}
            {pinned.length === 0 && recent.length === 0 && (
              <div className="sb-empty">{t("empty")}</div>
            )}
          </SidebarSection>
          {archived.length > 0 && (
            <SidebarSection label={t("nav.archived", { count: archived.length })} expanded={archivedExpanded} onToggle={() => onSetArchivedExpanded(!archivedExpanded)}>
              {archived.map((c) => <ChatListItem key={c.id} conv={c} openPanes={openPanes} activeConv={activeConv} onSetActiveConv={onSetActiveConv} onOpenPane={onOpenPane} />)}
            </SidebarSection>
          )}
        </div>
      )}

      {collapsed && <div className="sb-foot-spacer" />}
      <div className="sb-foot">
        <button
          type="button"
          className="sb-avatar-slot"
          onClick={() => { onOpenNotifs(); sse.clearUnread?.(); }}
          title={unread > 0 ? t("footer.unreadNotifications", { count: unread }) : t("footer.notifications")}
        >
          <span className="sb-avatar">{initial}</span>
          {unread > 0 && <span className="sb-badge-dot" />}
          {sseDot && <span className="sb-sse-dot" style={{ background: sseDot }} />}
        </button>
        {!collapsed && <span className="sb-user">{displayName || ""}</span>}
        <button
          type="button"
          className="sb-gear-btn"
          onClick={() => onOpenSettings()}
          title={t("footer.settings")}
          aria-label="settings"
        >
          <Icon.Settings size={18} strokeWidth={2} />
        </button>
      </div>
    </motion.aside>
  );
}
