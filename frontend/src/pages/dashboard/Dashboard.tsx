// Dashboard — Gemini-style welcome page. Single centered greeting + pill
// input + optional smart context strip. Enter submits the first message
// (creates conv → sends → switches to chat pane).
//
// Dashboard —— Gemini-style 欢迎页。openPane/onSetActiveConv 由 AppShell
// 经 props 传入，pages 层零 app 依赖。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { RelTime } from "../../shared/ui/RelTime.tsx";
import { useToastStore } from "@shared/ui/toastStore";
import { useConversations, useCreateConversation } from "@entities/conversation";
import { useDisplayName } from "@entities/user";
import { apiFetch } from "@shared/api";
import { WelcomeInput } from "./ui/WelcomeInput.tsx";
import { useGreeting } from "./lib/useGreeting";
import { useContextStrip } from "./lib/useContextStrip";

function ContextStrip({ strip, onJump }: { strip: any; onJump: (pane: string, convId?: string) => void }) {
  const { t } = useTranslation("dashboard");
  if (!strip) return null;
  if (strip.kind === "waiting") {
    return (
      <div className="wel-strip">
        <span className="wel-strip-dot" style={{ background: "var(--status-warn)" }} />
        <span dangerouslySetInnerHTML={{ __html: t("contextStrip.waiting", { count: strip.payload.count }) }} />{" · "}
        <button className="wel-strip-link" onClick={() => onJump("execute")}>{strip.payload.flowName}</button>
      </div>
    );
  }
  if (strip.kind === "failed") {
    return (
      <div className="wel-strip">
        <span className="wel-strip-dot" style={{ background: "var(--status-error)" }} />
        <span dangerouslySetInnerHTML={{ __html: t("contextStrip.failed", { count: strip.payload.count }) }} />{" · "}
        <button className="wel-strip-link" onClick={() => onJump("execute")}>{t("contextStrip.failedLink")}</button>
      </div>
    );
  }
  if (strip.kind === "running") {
    return (
      <div className="wel-strip">
        <span className="wel-strip-dot" style={{ background: "var(--status-info)" }} />
        <span dangerouslySetInnerHTML={{ __html: t("contextStrip.running", { count: strip.payload.count }) }} />{" · "}
        {t("contextStrip.runningLinkPrefix")} <RelTime ts={strip.payload.latestStartedAt} />{" "}{t("contextStrip.runningLinkSuffix")}
      </div>
    );
  }
  if (strip.kind === "recent") {
    return (
      <div className="wel-strip">
        <span className="wel-strip-dot" style={{ background: "var(--fg-faint)" }} />
        <span>{t("contextStrip.recent")} · <button className="wel-strip-link" onClick={() => onJump("chat", strip.payload.convId)}>{strip.payload.convTitle}</button> · <RelTime ts={strip.payload.updatedAt} /></span>
      </div>
    );
  }
  return null;
}

interface DashboardProps {
  onOpenPane: (pane: string, id?: string) => void;
  onSetActiveConv: (id: string | null) => void;
}

export function Dashboard({ onOpenPane, onSetActiveConv }: DashboardProps) {
  const pushToast = useToastStore((s) => s.pushToast);

  const { data: conversations = [] } = useConversations();
  const [displayName] = useDisplayName() as [string, any];
  const create = useCreateConversation();

  const hasRecentConv = conversations.some(
    (c) => c.updatedAt && Date.now() - new Date(c.updatedAt).getTime() < 24 * 60 * 60 * 1000
  );
  const greeting = useGreeting({ hasRecentConv, displayName });
  const strip = useContextStrip();

  const { t } = useTranslation("dashboard");
  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async (text: string) => {
    setSubmitting(true);
    try {
      const created = await create.mutateAsync({});
      if (created?.id) {
        onSetActiveConv(created.id);
        onOpenPane("chat");
        await apiFetch(`/conversations/${created.id}/messages`, { method: "POST", body: { content: text } });
      }
    } catch (err) {
      pushToast({ kind: "error", title: t("sendFailed"), desc: (err as any)?.message });
    } finally {
      setSubmitting(false);
    }
  };

  const onJump = (pane: string, convId?: string) => {
    if (convId) onSetActiveConv(convId);
    onOpenPane(pane);
  };

  return (
    <div className="wel">
      <div className="wel-greet">{greeting}</div>
      <WelcomeInput onSubmit={onSubmit} isSubmitting={submitting} />
      <ContextStrip strip={strip} onJump={onJump} />
    </div>
  );
}
