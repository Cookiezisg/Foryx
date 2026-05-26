// useNotifications — subscribes to /api/v1/notifications. SSE event name
// is fixed "notification"; payload .type drives dispatch into TanStack
// Query invalidation. Maintains a local unread counter that
// NotificationsDrawer clears on open.
//
// useNotifications —— SSE event name 固定 "notification"，按 payload.type
// 分派 invalidate；本地维护未读计数。

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createSSE } from "./shared.js";
import { useOverlayStore } from "@app/model";
import { useSessionStore } from "@entities/session";
import { qk } from "../api/client.js";

// type -> list of query keys to invalidate when this entity changes.
const TYPE_TO_INVALIDATIONS = {
  conversation: (id) => [qk.conversations(), qk.conversation(id)],
  function:     (id) => [qk.functions(), qk.function(id), qk.functionVersions(id)],
  handler:      (id) => [qk.handlers(), qk.handler(id), qk.handlerVersions(id), qk.handlerConfig(id)],
  workflow:     (id) => [qk.workflows(), qk.workflow(id), qk.workflowVersions(id)],
  flowrun:      (id) => [qk.flowruns(), qk.flowrun(id), qk.flowrunNodes(id)],
  mcp_server:   () => [qk.mcpServers()],
  skill:        () => [qk.skills()],
  memory:       () => [["memories"]],
  todo:         () => [],
  sandbox_env:  () => [],
  compaction:   (id) => [qk.conversation(id)],
};

export function useNotifications() {
  const qc = useQueryClient();
  const [status, setStatus] = useState("connecting");
  const [unread, setUnread] = useState(0);
  // TODO(4b): pages props 化后移除 shared-tmp→app 过渡引用
  const setPendingAsk = useOverlayStore((s) => s.setPendingAsk);
  const activeUserId = useSessionStore((s) => s.currentUserId);

  useEffect(() => {
    const ctrl = createSSE({
      path: "/notifications",
      eventHandlers: {
        notification: (payload) => {
          if (!payload) return;
          const { type, id, data, conversationId } = payload;

          if (type === "ask") {
            const action = data?.action;
            if (!action || action === "pending") {
              setPendingAsk({ id, conversationId, ...data });
            } else if (action === "resolved") {
              setPendingAsk(null);
            }
            return;
          }

          const factory = TYPE_TO_INVALIDATIONS[type];
          if (factory) {
            const keys = factory(id);
            for (const key of keys) qc.invalidateQueries({ queryKey: key });
          }
          setUnread((n) => n + 1);
        },
      },
      onStatus: setStatus,
    });
    return () => ctrl.close();
  }, [qc, setPendingAsk, activeUserId]);

  return { status, unread, clearUnread: () => setUnread(0) };
}
