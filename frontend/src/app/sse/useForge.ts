// useForge — subscribes to /api/v1/forge. 4 events: started / op_applied
// / env_attempt / completed. Writes into useForgeProgress (shared/model)
// so detail views and forge list can read it without reversing FSD layers.
//
// useForge —— /api/v1/forge 4 个事件；写入 shared/model/forgeProgress；
// 详情页与列表行顺向读 shared，不反向依赖 app/sse。

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createSSE } from "@shared/api/sse";
import { useSessionStore } from "@entities/session";
import { qk } from "@shared/api/queryKeys";
import { useForgeProgress } from "@shared/model";

export { useForgeProgress };

const scopeKey = (scope: any) => `${scope?.kind}:${scope?.id}`;

export function useForge() {
  const qc = useQueryClient();
  const [status, setStatus] = useState("connecting");
  const activeUserId = useSessionStore((s) => s.currentUserId);

  useEffect(() => {
    const store = useForgeProgress.getState();

    const ctrl = createSSE({
      path: "/forge",
      eventHandlers: {
        forge_started: (e: any) => {
          const key = scopeKey(e.scope);
          store.put(key, {
            scope: e.scope,
            operation: e.operation,
            conversationId: e.conversationId,
            toolCallId: e.toolCallId,
            ops: [],
            envAttempts: [],
            status: "running",
          });
        },
        forge_op_applied: (e: any) => {
          const key = scopeKey(e.scope);
          const cur = useForgeProgress.getState().active[key];
          if (!cur) return;
          store.put(key, { ...cur, ops: [...cur.ops, { index: e.index, op: e.op }] });
        },
        forge_env_attempt: (e: any) => {
          const key = scopeKey(e.scope);
          const cur = useForgeProgress.getState().active[key];
          if (!cur) return;
          store.put(key, {
            ...cur,
            envAttempts: [
              ...cur.envAttempts,
              { attempt: e.attempt, status: e.status, stage: e.stage, detail: e.detail, error: e.error },
            ],
          });
        },
        forge_completed: (e: any) => {
          const key = scopeKey(e.scope);
          const cur = useForgeProgress.getState().active[key];
          store.put(key, {
            ...(cur || { scope: e.scope }),
            status: e.status,
            versionId: e.versionId,
            envStatus: e.envStatus,
            attemptsUsed: e.attemptsUsed,
            error: e.error,
            finishedAt: Date.now(),
          });
          // refresh entity caches once forging finishes
          if (e.scope?.kind && e.scope?.id) {
            const kind = e.scope.kind;
            const id = e.scope.id;
            if (kind === "function") {
              qc.invalidateQueries({ queryKey: qk.functions() });
              qc.invalidateQueries({ queryKey: qk.function(id) });
              qc.invalidateQueries({ queryKey: qk.functionVersions(id) });
            } else if (kind === "handler") {
              qc.invalidateQueries({ queryKey: qk.handlers() });
              qc.invalidateQueries({ queryKey: qk.handler(id) });
              qc.invalidateQueries({ queryKey: qk.handlerVersions(id) });
            } else if (kind === "workflow") {
              qc.invalidateQueries({ queryKey: qk.workflows() });
              qc.invalidateQueries({ queryKey: qk.workflow(id) });
              qc.invalidateQueries({ queryKey: qk.workflowVersions(id) });
            }
          }
        },
      },
      onStatus: setStatus,
    });
    return () => ctrl.close();
  }, [qc, activeUserId]);

  return status;
}
