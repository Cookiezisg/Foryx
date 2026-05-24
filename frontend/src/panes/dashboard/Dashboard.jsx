// Dashboard — Gemini-style welcome page. Single centered greeting + pill
// input + optional smart context strip. Enter submits the first message
// (creates conv → sends → switches to chat pane).
//
// Dashboard —— Gemini-style 欢迎页。居中问候 + pill 输入 + 可选智能条;
// Enter 串行新建 conv + 发首条消息 + 切到 chat pane。

import { useState } from "react";
import { RelTime } from "../../components/shared/RelTime.jsx";
import { useUIStore } from "../../store/ui.js";
import { useConversations, useCreateConversation } from "../../api/conversations.js";
import { useDisplayName } from "../../hooks/useDisplayName.js";
import { WelcomeInput } from "./WelcomeInput.jsx";
import { useGreeting } from "./useGreeting.js";
import { useContextStrip } from "./useContextStrip.js";

function ContextStrip({ strip, onJump }) {
  if (!strip) return null;
  if (strip.kind === "waiting") {
    return (
      <div className="wel-strip">
        <span className="wel-strip-dot" style={{ background: "var(--status-warn)" }} />
        <span><strong>{strip.payload.count} 个流程等你确认</strong> · <button className="wel-strip-link" onClick={() => onJump("execute")}>{strip.payload.flowName}</button></span>
      </div>
    );
  }
  if (strip.kind === "failed") {
    return (
      <div className="wel-strip">
        <span className="wel-strip-dot" style={{ background: "var(--status-error)" }} />
        <span><strong>{strip.payload.count} 个流程卡住了</strong> · <button className="wel-strip-link" onClick={() => onJump("execute")}>查看</button></span>
      </div>
    );
  }
  if (strip.kind === "running") {
    return (
      <div className="wel-strip">
        <span className="wel-strip-dot" style={{ background: "var(--status-info)" }} />
        <span><strong>{strip.payload.count} 个流程在跑</strong> · 最近一次 <RelTime ts={strip.payload.latestStartedAt} /> 启动</span>
      </div>
    );
  }
  if (strip.kind === "recent") {
    return (
      <div className="wel-strip">
        <span className="wel-strip-dot" style={{ background: "var(--fg-faint)" }} />
        <span>继续 · <button className="wel-strip-link" onClick={() => onJump("chat", strip.payload.convId)}>{strip.payload.convTitle}</button> · <RelTime ts={strip.payload.updatedAt} /></span>
      </div>
    );
  }
  return null;
}

async function sendMessageDirect(convId, text) {
  // useSendMessage ties convId at hook-call time; we don't have the new id
  // until mid-onSubmit, so POST directly. ChatPane will re-fetch
  // ["conversation", convId, "messages"] when it mounts.
  const res = await fetch(`/api/v1/conversations/${convId}/messages`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body?.error?.message || res.statusText);
  }
}

export function Dashboard() {
  const openPane      = useUIStore((s) => s.openPane);
  const setActiveConv = useUIStore((s) => s.setActiveConv);

  const { data: conversations = [] } = useConversations();
  const [displayName] = useDisplayName();
  const create = useCreateConversation();

  const hasRecentConv = conversations.some(
    (c) => c.updatedAt && Date.now() - new Date(c.updatedAt).getTime() < 24 * 60 * 60 * 1000
  );
  const greeting = useGreeting({ hasRecentConv, displayName });
  const strip = useContextStrip();

  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async (text) => {
    setSubmitting(true);
    try {
      const created = await create.mutateAsync({});
      if (created?.id) {
        setActiveConv(created.id);
        openPane("chat");
        await sendMessageDirect(created.id, text);
      }
    } finally {
      setSubmitting(false);
    }
  };

  const onJump = (pane, convId) => {
    if (convId) setActiveConv(convId);
    openPane(pane);
  };

  return (
    <div className="wel">
      <div className="wel-greet">{greeting}</div>
      <WelcomeInput onSubmit={onSubmit} isSubmitting={submitting} />
      <ContextStrip strip={strip} onJump={onJump} />
    </div>
  );
}
