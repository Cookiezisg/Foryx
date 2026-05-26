// NotificationsDrawer — right-side slide-in drawer listing notifications
// from REST snapshot (initial) plus any new ones in the local SSE buffer.
// Also hosts the 待办 tab that shows pending AskUser questions inline.
//
// NotificationsDrawer —— 右滑抽屉；待办（Help/Ask）+ 通知两 tab。

import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { AnimatePresence, motion } from "framer-motion";
import { Icon } from "@shared/ui/Icon";
import { Badge } from "@shared/ui/Badge";
import { Button } from "@shared/ui/Button";
import { RelTime } from "../../shared/ui/RelTime.tsx";
import { useToastStore } from "@shared/ui/toastStore";
import { useNotificationsSnapshot } from "./useNotificationsSnapshot";
import { apiFetch } from "@shared/api/httpClient";

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
  sandbox_env: Icon.Boxes,
  compaction: Icon.Archive,
};

function TodoTab({ pendingAsk, setPendingAsk, pushToast }: { pendingAsk: any; setPendingAsk: (v: any) => void; pushToast: (t: any) => void }) {
  const { t } = useTranslation("misc");
  const [selected, setSelected] = useState(null);
  const [submitting, setSubmitting] = useState(false);

  if (!pendingAsk) {
    return (
      <div style={{ padding: 32, textAlign: "center", color: "var(--fg-faint)", fontSize: 12 }}>
        {t("notificationsDrawer.emptyTodo")}
      </div>
    );
  }

  const options = pendingAsk.options || [];

  const submit = async () => {
    if (!selected) return;
    setSubmitting(true);
    try {
      await apiFetch(
        `/conversations/${pendingAsk.conversationId}/pending-questions/${pendingAsk.toolCallId}:resolve`,
        { method: "POST", body: { answer: selected } }
      );
      pushToast({ kind: "success", title: t("toast:notifications.answerSubmitted") });
      setPendingAsk(null);
    } catch (err) {
      pushToast({ kind: "error", title: t("toast:notifications.submitFailed"), desc: (err as any)?.message });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="todo-tab-body">
      <div className="ask-head" style={{ padding: "12px 16px", borderBottom: "1px solid var(--border-soft)" }}>
        <div className="icon-wrap"><Icon.HelpCircle /></div>
        <div className="meta">
          <div className="label">{t("notificationsDrawer.askHeadLabel")}</div>
          <div className="title">{pendingAsk.question || t("notificationsDrawer.askHeadFallback")}</div>
        </div>
      </div>
      <div className="ask-body" style={{ padding: "12px 16px" }}>
        {pendingAsk.context && <div className="ask-question">{pendingAsk.context}</div>}
        <div className="ask-options">
          {options.length === 0 && (
            <div style={{ padding: 16, color: "var(--fg-faint)", fontSize: 12 }}>
              {t("notificationsDrawer.askNoOptions")}
            </div>
          )}
          {options.map((o: any, i: number) => (
            <div
              key={o.id || i}
              className={"ask-option" + (selected === (o.id || o.value) ? " is-selected" : "")}
              onClick={() => setSelected(o.id || o.value)}
            >
              <div className="key">{i + 1}</div>
              <div className="text">{o.text || o.label}<span className="sub">{o.sub || ""}</span></div>
              <Icon.Check className="check" />
            </div>
          ))}
        </div>
      </div>
      <div className="ask-footer" style={{ padding: "8px 16px" }}>
        <div className="hint">{t("notificationsDrawer.askHint")} <Icon.CornerDownLeft style={{ width: 11, height: 11 }} /></div>
        <div className="actions">
          <Button size="sm" variant="accent" disabled={!selected || submitting} loading={submitting} onClick={submit}>
            <Icon.Check /> {t("notificationsDrawer.submitLabel")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function NotifsTab({ snapshot, onClick }: { snapshot: any[]; onClick: (n: any) => void }) {
  const { t } = useTranslation("misc");
  if (snapshot.length === 0) {
    return (
      <div style={{ padding: 32, textAlign: "center", color: "var(--fg-faint)", fontSize: 12 }}>
        {t("notificationsDrawer.emptyNotifs")}
      </div>
    );
  }
  return (
    <>
      {snapshot.map((n: any) => {
        const I = (TYPE_TO_ICON as Record<string, React.ComponentType<any>>)[n.type] || Icon.Bell;
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
    </>
  );
}

interface NotificationsDrawerProps {
  open: boolean;
  onClose: () => void;
  onOpenPane: (pane: string) => void;
  onOpenEntity: (pane: string, id: string) => void;
  onSetActiveConv: (id: string | null) => void;
  pendingAsk?: any;
  onSetPendingAsk: (ask: any) => void;
  unread?: number;
  clearUnread?: () => void;
}

export function NotificationsDrawer({ open, onClose, onOpenPane, onOpenEntity, onSetActiveConv, pendingAsk, onSetPendingAsk, unread = 0, clearUnread = () => {} }: NotificationsDrawerProps) {
  const { t } = useTranslation("misc");
  const pushToast = useToastStore((s) => s.pushToast);

  const { data: snapshot = [] } = useNotificationsSnapshot(50);

  const [tab, setTab] = useState(pendingAsk ? "todo" : "notifs");

  const handleNotifClick = (n: any) => {
    const pane = (TYPE_TO_PANE as Record<string, string>)[n.type];
    if (!pane) return;
    if (n.type === "conversation" && n.id) {
      onSetActiveConv(n.id);
      onOpenPane("chat");
    } else if (pane && n.id) {
      onOpenEntity(pane, n.id);
    } else {
      onOpenPane(pane);
    }
    onClose();
  };

  const handleClose = () => { onClose(); clearUnread(); };

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
          <div className="drawer-scrim" onClick={handleClose} />
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
                <div className="drawer-title">{t("notificationsDrawer.title")}</div>
                {unread > 0 && <Badge kind="muted">{t("notificationsDrawer.unread", { count: unread })}</Badge>}
              </div>
              <div style={{ display: "flex", gap: 4 }}>
                <button className="btn btn-xs btn-ghost" onClick={clearUnread}>{t("notificationsDrawer.markAllRead")}</button>
                <button className="icon-btn" onClick={handleClose} title={t("notificationsDrawer.closeTitle")}><Icon.X /></button>
              </div>
            </div>
            <div className="notif-tabs">
              <button
                className={"notif-tab" + (tab === "todo" ? " is-active" : "")}
                onClick={() => setTab("todo")}
              >
                {pendingAsk ? t("notificationsDrawer.tabTodoPending") : t("notificationsDrawer.tabTodo")}
              </button>
              <button
                className={"notif-tab" + (tab === "notifs" ? " is-active" : "")}
                onClick={() => setTab("notifs")}
              >
                {t("notificationsDrawer.tabNotifs")}
              </button>
            </div>
            <div className="drawer-list">
              {tab === "todo" ? (
                <TodoTab pendingAsk={pendingAsk} setPendingAsk={onSetPendingAsk} pushToast={pushToast} />
              ) : (
                <NotifsTab snapshot={snapshot} onClick={handleNotifClick} />
              )}
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
