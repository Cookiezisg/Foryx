// ChatPage — header + scrolling thread + composer. The chat store
// (SSE-driven) is the source of truth for the rendered tree; REST
// history hydrates it on first mount or conv switch.
//
// ChatPage —— 头部 + 滚动消息流 + composer。chat store 是真实 tree；
// 切换/初次进入时由 REST 历史 hydrate。

import { useEffect, useRef } from "react";
import { Trans, useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useConversation, useConversationMessages } from "../../api/conversations.js";
import { useApiKeys, useModelConfigs } from "../../api/config.js";
import { useChatStore } from "../../store/chat.js";
import { qk } from "../../api/client.js";
import { useSendMessageFlow } from "../../features/send-message/index.ts";
import { ChatHeader } from "./ui/ChatHeader.jsx";
import { MessageView } from "./ui/MessageView.jsx";
import { Composer } from "@features/send-message";
import { NoApiKeyGate } from "./ui/NoApiKeyGate.jsx";
import { NoModelGate } from "./ui/NoModelGate.jsx";
import { Icon } from "../../components/primitives/Icon.jsx";

// Stable empty array — zustand selectors must return cached references
// (warns "getSnapshot should be cached" + infinite loop otherwise when
// the conv hasn't been hydrated yet).
const EMPTY_IDS = Object.freeze([]);

export function ChatPage({ activeConv, onSetActiveConv, onClose, onOpenSettings }) {
  const { t, i18n } = useTranslation("conv");
  const qc = useQueryClient();
  const { data: conv, error: convError } = useConversation(activeConv);
  const { data: historyMessages, isLoading: histLoading } = useConversationMessages(activeConv);

  // Self-heal upfront: if backend says this conv doesn't exist for the
  // current user (stale activeConv after delete or cross-user residue),
  // bounce to the picker before the user even tries to send.
  //
  // 上来就自愈:GET 对话本身就 404,直接弹回 picker,避免用户先打半天字
  // 再被发送失败。
  useEffect(() => {
    if (convError?.code === "CONVERSATION_NOT_FOUND") {
      onSetActiveConv(null);
      qc.invalidateQueries({ queryKey: qk.conversations() });
    }
  }, [convError, onSetActiveConv, qc]);
  const { data: apiKeys = [], isLoading: keysLoading } = useApiKeys();
  const { data: modelConfigs = [], isLoading: cfgLoading } = useModelConfigs();

  const hydrateConv = useChatStore((s) => s.hydrateConv);
  const ensureConv = useChatStore((s) => s.ensureConv);
  const topMsgIds = useChatStore((s) => {
    if (!activeConv) return EMPTY_IDS;
    return s.convs[activeConv]?.topMsgIds || EMPTY_IDS;
  });

  // Detect whether any message in this conv is currently streaming.
  const isStreaming = useChatStore((s) => {
    const c = activeConv && s.convs[activeConv];
    if (!c) return false;
    for (const m of c.messages.values()) {
      if (m.status === "streaming") return true;
    }
    return false;
  });

  const { submit, cancelStream, isPending } = useSendMessageFlow(activeConv, {
    onConvGone: () => {
      onSetActiveConv(null);
      qc.invalidateQueries({ queryKey: qk.conversations() });
    },
  });

  // Hydrate chat store from REST history when conv id changes / history loads.
  useEffect(() => {
    if (!activeConv) return;
    ensureConv(activeConv);
    if (historyMessages && Array.isArray(historyMessages)) {
      hydrateConv(activeConv, historyMessages);
    }
  }, [activeConv, historyMessages, hydrateConv, ensureConv]);

  // Auto-scroll to bottom whenever message tree changes (double-rAF so
  // freshly mounted blocks have their layout done before measuring).
  const streamRef = useRef(null);
  useEffect(() => {
    let r2 = null;
    const r1 = requestAnimationFrame(() => {
      r2 = requestAnimationFrame(() => {
        if (streamRef.current) {
          streamRef.current.scrollTop = streamRef.current.scrollHeight;
        }
      });
    });
    return () => { cancelAnimationFrame(r1); if (r2) cancelAnimationFrame(r2); };
  }, [topMsgIds.length]);

  if (!keysLoading && apiKeys.length === 0) {
    return <NoApiKeyGate onOpenSettings={onOpenSettings} />;
  }

  // chat scenario gate: keys exist but no model picked for chat. Onboarding's
  // testKey may have failed (no model-config written) or user skipped key
  // step and later added one via Config without configuring a model.
  //
  // chat scenario gate:有 key 但没配 chat 模型 —— onboarding 的 testKey 失败
  // (没写 model-config),或用户跳过 onboarding 后单独加 key 没配模型。
  const hasChatModel = modelConfigs.some((c) => c.scenario === "chat");
  if (!cfgLoading && !hasChatModel) {
    return <NoModelGate onOpenSettings={onOpenSettings} />;
  }

  if (!activeConv) {
    return <EmptyConvPlaceholder />;
  }

  const onSend = (payload) => submit(payload);
  const onCancel = () => cancelStream();

  return (
    <div className="chat">
      <ChatHeader conv={conv} onClose={onClose} />
      <div className="chat-stream" ref={streamRef}>
        <div className="chat-stream-inner">
          <div className="day-divider">{t("dayDivider")} · {new Date().toLocaleDateString(i18n.language === "zh" ? "zh-CN" : "en-US", { month: "long", day: "numeric", weekday: "short" })}</div>
          {topMsgIds.length === 0 && !histLoading && (
            <EmptyConvHero conv={conv} />
          )}
          {topMsgIds.map((id) => (
            <MessageView key={id} convId={activeConv} msgId={id} />
          ))}
        </div>
      </div>
      <Composer
        disabled={isPending}
        isStreaming={isStreaming}
        onSend={onSend}
        onCancel={onCancel}
      />
    </div>
  );
}

function EmptyConvPlaceholder() {
  const { t } = useTranslation("conv");
  return (
    <div className="empty-shell">
      <div className="empty-shell-card">
        <div className="empty-shell-logo">
          <Icon.MessageSquare />
        </div>
        <div>
          <div className="empty-shell-title">{t("emptyConv.noConvTitle")}</div>
          <div className="empty-shell-sub">
            <Trans i18nKey="emptyConv.noConvSub" ns="conv">
              <Icon.Plus style={{ display: "inline", verticalAlign: "-2px", width: 12, height: 12 }} />
            </Trans>
          </div>
        </div>
      </div>
    </div>
  );
}

function EmptyConvHero({ conv }) {
  const { t } = useTranslation("conv");
  return (
    <div style={{ padding: "48px 0", textAlign: "center", color: "var(--fg-muted)" }}>
      <Icon.Sparkles className="icon" style={{ width: 18, height: 18, color: "var(--accent)" }} />
      <div style={{ marginTop: 10, fontSize: 14, fontWeight: 500, color: "var(--fg-strong)" }}>
        {conv?.title || t("emptyConv.newConvTitle")}
      </div>
      <div style={{ marginTop: 6, fontSize: 12 }}>
        <Trans i18nKey="emptyConv.newConvSub" ns="conv">
          <code>@</code>
        </Trans>
      </div>
    </div>
  );
}
