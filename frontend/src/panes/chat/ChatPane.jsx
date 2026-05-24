// ChatPane — header + scrolling thread + composer. The chat store
// (SSE-driven) is the source of truth for the rendered tree; REST
// history hydrates it on first mount or conv switch.
//
// ChatPane —— 头部 + 滚动消息流 + composer。chat store 是真实 tree；
// 切换/初次进入时由 REST 历史 hydrate。

import { useEffect, useRef } from "react";
import { useConversation, useConversationMessages, useSendMessage, useCancelStream } from "../../api/conversations.js";
import { useApiKeys, useModelConfigs } from "../../api/config.js";
import { useChatStore } from "../../store/chat.js";
import { useUIStore } from "../../store/ui.js";
import { ChatHeader } from "./ChatHeader.jsx";
import { MessageView } from "./MessageView.jsx";
import { Composer } from "./Composer.jsx";
import { NoApiKeyGate } from "./NoApiKeyGate.jsx";
import { NoModelGate } from "./NoModelGate.jsx";
import { Icon } from "../../components/primitives/Icon.jsx";

// Stable empty array — zustand selectors must return cached references
// (warns "getSnapshot should be cached" + infinite loop otherwise when
// the conv hasn't been hydrated yet).
const EMPTY_IDS = Object.freeze([]);

export function ChatPane({ onClose }) {
  const activeConv = useUIStore((s) => s.activeConv);
  const { data: conv } = useConversation(activeConv);
  const { data: historyMessages, isLoading: histLoading } = useConversationMessages(activeConv);
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

  const send = useSendMessage(activeConv);
  const cancel = useCancelStream(activeConv);
  const pushToast = useUIStore((s) => s.pushToast);

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
    return <NoApiKeyGate />;
  }

  // chat scenario gate: keys exist but no model picked for chat. Onboarding's
  // testKey may have failed (no model-config written) or user skipped key
  // step and later added one via Config without configuring a model.
  //
  // chat scenario gate:有 key 但没配 chat 模型 —— onboarding 的 testKey 失败
  // (没写 model-config),或用户跳过 onboarding 后单独加 key 没配模型。
  const hasChatModel = modelConfigs.some((c) => c.scenario === "chat");
  if (!cfgLoading && !hasChatModel) {
    return <NoModelGate />;
  }

  if (!activeConv) {
    return <EmptyConvPlaceholder />;
  }

  const onSend = ({ content, attachments, mentions }) => {
    const body = { content };
    if (attachments?.length) body.attachments = attachments.map((a) => ({ fileName: a.name, sizeBytes: a.size }));
    if (mentions?.length) body.mentions = mentions.map((m) => m.id);
    send.mutate(body, {
      onError: (err) => pushToast({ kind: "error", title: "发送失败", desc: err.message }),
    });
  };

  const onCancel = () => {
    cancel.mutate(undefined, {
      onError: (err) => pushToast({ kind: "warn", title: "取消失败", desc: err.message }),
    });
  };

  return (
    <div className="chat">
      <ChatHeader conv={conv} onClose={onClose} />
      <div className="chat-stream" ref={streamRef}>
        <div className="chat-stream-inner">
          <div className="day-divider">今天 · {new Date().toLocaleDateString("zh-CN")}</div>
          {topMsgIds.length === 0 && !histLoading && (
            <EmptyConvHero conv={conv} />
          )}
          {topMsgIds.map((id) => (
            <MessageView key={id} convId={activeConv} msgId={id} />
          ))}
        </div>
      </div>
      <Composer
        disabled={send.isPending}
        isStreaming={isStreaming}
        onSend={onSend}
        onCancel={onCancel}
      />
    </div>
  );
}

function EmptyConvPlaceholder() {
  return (
    <div className="empty-shell">
      <div className="empty-shell-card">
        <div className="empty-shell-logo">
          <Icon.MessageSquare />
        </div>
        <div>
          <div className="empty-shell-title">还没选中对话</div>
          <div className="empty-shell-sub">从左边挑一个,或点 <Icon.Plus style={{ display: "inline", verticalAlign: "-2px", width: 12, height: 12 }} /> 开一段新的。</div>
        </div>
      </div>
    </div>
  );
}

function EmptyConvHero({ conv }) {
  return (
    <div style={{ padding: "48px 0", textAlign: "center", color: "var(--fg-muted)" }}>
      <Icon.Sparkles className="icon" style={{ width: 18, height: 18, color: "var(--accent)" }} />
      <div style={{ marginTop: 10, fontSize: 14, fontWeight: 500, color: "var(--fg-strong)" }}>
        {conv?.title || "新对话"}
      </div>
      <div style={{ marginTop: 6, fontSize: 12 }}>
        说说你想干啥。<code>@</code> 引用 function、handler、workflow、skill、文档。
      </div>
    </div>
  );
}
