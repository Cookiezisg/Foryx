// useForge — subscribes to /api/v1/forge. 4 events: started / op_applied
// / env_attempt / completed. Populates a zustand store keyed by
// "{kind}:{id}" so detail views and forge list can render live progress.
//
// useForge —— /api/v1/forge 4 个事件，按 scope key 写入 store；
// 详情页与列表行的进度由此驱动。

import { useEffect, useState } from "react";
import { create } from "zustand";
import { useQueryClient } from "@tanstack/react-query";
import { createSSE } from "./shared.js";
import { useSessionStore } from "@entities/session";
import { qk } from "../api/client.js";

export const useForgeProgress = create((set, get) => ({
  // Map<scopeKey, ForgeProgress>
  active: {},

  put(scopeKey, value) {
    set((s) => ({ active: { ...s.active, [scopeKey]: value } }));
  },
  clear(scopeKey) {
    set((s) => {
      const next = { ...s.active };
      delete next[scopeKey];
      return { active: next };
    });
  },
}));

const scopeKey = (scope) => `${scope?.kind}:${scope?.id}`;

export function useForge() {
  const qc = useQueryClient();
  const [status, setStatus] = useState("connecting");
  const activeUserId = useSessionStore((s) => s.currentUserId);

  useEffect(() => {
    const store = useForgeProgress.getState();

    const ctrl = createSSE({
      path: "/forge",
      eventHandlers: {
        forge_started: (e) => {
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
        forge_op_applied: (e) => {
          const key = scopeKey(e.scope);
          const cur = useForgeProgress.getState().active[key];
          if (!cur) return;
          store.put(key, { ...cur, ops: [...cur.ops, { index: e.index, op: e.op }] });
        },
        forge_env_attempt: (e) => {
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
        forge_completed: (e) => {
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
