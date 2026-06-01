// useFlowrunTicker — maintains a per-node visual state map driven by the flowrun
// notifications tick stream + eventlog (08 §5, CANON-X4).
//
// The backend emits ephemeral "flowrun" notifications with action="tick" + nodeId + status
// on each node transition. This hook maps those into a nodeId→status record for canvas
// animation. On reconnect or missed ticks, it re-fetches the full trace from the REST
// API to catch up (journal is the durable truth).
//
// useFlowrunTicker —— 维护 nodeId → 视觉状态映射；tick 实时驱动，重连从 trace REST 补全。

import { useCallback, useEffect, useRef, useState } from "react";
import { apiFetch, pickList } from "@shared/api";
import type { TraceEntry } from "@entities/flowrun";

export type NodeTickStatus = "pending" | "running" | "ok" | "failed" | "awaiting_signal" | "skipped";

export interface NodeTick {
  nodeId: string;
  status: NodeTickStatus;
  iterationKey?: number;
}

// useFlowrunTicker subscribes to the SSE notification channel for a specific flowrun
// and returns a nodeId→NodeTick map. It also provides a refetch function for reconnect
// full-pull via GET /flowruns/{id}/trace.
//
// Parameters:
//   runId — the flowrun ID to track; null = hook is inactive
//   sseData — the latest notification payload from useNotifications (caller passes it)
export function useFlowrunTicker(runId: string | null) {
  const [nodeStates, setNodeStates] = useState<Record<string, NodeTick>>({});
  const fetchedRef = useRef(false);

  // Full-pull from trace API: used on mount and reconnect.
  const fetchFullTrace = useCallback(async () => {
    if (!runId) return;
    try {
      const raw = await apiFetch(`/flowruns/${runId}/trace`);
      const entries = pickList<TraceEntry>(raw);
      // Derive node states from the journal trace (last event per node wins).
      const map: Record<string, NodeTick> = {};
      for (const e of entries) {
        let status: NodeTickStatus = "pending";
        switch (e.type) {
          case "node_started":   status = "running"; break;
          case "node_completed": status = "ok";      break;
          case "node_failed":    status = "failed";  break;
          case "signal_awaited": status = "awaiting_signal"; break;
          case "branch_taken":   status = "ok";      break;
          default:               continue;
        }
        map[e.nodeId] = { nodeId: e.nodeId, status, iterationKey: e.iterationKey };
      }
      setNodeStates(map);
      fetchedRef.current = true;
    } catch {
      // ignore — canvas will show last-known state
    }
  }, [runId]);

  // Initial full-pull when runId changes.
  useEffect(() => {
    fetchedRef.current = false;
    if (!runId) { setNodeStates({}); return; }
    fetchFullTrace();
  }, [runId, fetchFullTrace]);

  // Apply a single tick update from the SSE notification.
  const applyTick = useCallback((nodeId: string, status: NodeTickStatus, iterationKey?: number) => {
    setNodeStates((prev) => ({
      ...prev,
      [nodeId]: { nodeId, status, iterationKey: iterationKey ?? 0 },
    }));
  }, []);

  return { nodeStates, applyTick, refetch: fetchFullTrace };
}
