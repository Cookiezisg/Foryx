// useEventLog — single global subscription to /api/v1/eventlog. Dispatches
// the 5 message/block events to the chat store. Sidebar footer status dot
// reflects the connection state.
//
// useEventLog —— /api/v1/eventlog 单例订阅；5 个事件分发到 chat store；
// sidebar 状态点反映连接状态。activeUserId 变化时 tear-down 旧 EventSource
// 重连——否则 Onboarding 切到新账号后还在收旧 user 的事件，新发消息不渲染。

import { useEffect, useState } from "react";
import { createSSE } from "./shared.js";
import { useChatStore } from "../store/chat.js";
import { useSessionStore } from "@entities/session";

export function useEventLog() {
  const [status, setStatus] = useState("connecting");
  const activeUserId = useSessionStore((s) => s.currentUserId);

  useEffect(() => {
    const ch = useChatStore.getState();

    const handlers = {
      message_start: (e) => {
        if (!e?.conversationId) return;
        ch.ensureConv(e.conversationId);
        ch.onMessageStart(e.conversationId, e);
      },
      message_stop: (e) => e?.conversationId && ch.onMessageStop(e.conversationId, e),
      block_start:  (e) => {
        if (!e?.conversationId) return;
        ch.ensureConv(e.conversationId);
        ch.onBlockStart(e.conversationId, e);
      },
      block_delta:  (e) => e?.conversationId && ch.onBlockDelta(e.conversationId, e),
      block_stop:   (e) => e?.conversationId && ch.onBlockStop(e.conversationId, e),
    };

    const ctrl = createSSE({
      path: "/eventlog",
      eventHandlers: handlers,
      onStatus: setStatus,
    });
    return () => ctrl.close();
  }, [activeUserId]);

  return status;
}
