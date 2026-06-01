import { useParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { useUIStore } from "@/stores/ui";
import { EmptyView, StatusBadge, RelTime, Pill } from "@/ui";
import type { FlowRun, FlowRunNode } from "@frontend/entities/flowrun/model/types";

export function FlowRunDetail() {
  const { id } = useParams<{ id: string }>();
  const ui = useUIStore();
  const qc = useQueryClient();
  const { data: run } = useQuery({
    queryKey: qk.flowrun(id ?? ""),
    queryFn: () => getJSON<FlowRun>(`/api/v1/flowruns/${id}`),
    enabled: !!id,
  });
  const { data: nodes = [] } = useQuery({
    queryKey: qk.flowrunNodes(id ?? ""),
    queryFn: () => getJSON<FlowRunNode[]>(`/api/v1/flowruns/${id}/nodes`),
    enabled: !!id,
  });
  // Durable approval endpoint: POST /flowruns/{id}/approvals/{nodeId} with {decision, reason}.
  // decision values: "approved" | "rejected" (not the old "approve"/"reject").
  const approve = useMutation({
    mutationFn: ({ nodeId, decision }: { nodeId: string; decision: "approved" | "rejected" }) =>
      postJSON(`/api/v1/flowruns/${id}/approvals/${nodeId}`, { decision, reason: "" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.flowrunNodes(id ?? "") });
      qc.invalidateQueries({ queryKey: qk.flowrun(id ?? "") });
    },
  });
  if (!id || !run) return <EmptyView>loading…</EmptyView>;

  return (
    <div style={{ height: "100%", overflow: "auto", padding: 12 }}>
      <div style={{ display: "flex", gap: 8, alignItems: "baseline", marginBottom: 4 }}>
        <h2 style={{ margin: 0 }}>FlowRun</h2>
        <span className="mono muted" style={{ fontSize: 11 }}>{run.id}</span>
        <StatusBadge status={run.status} />
        {run.dryRun && <Pill kind="info">dry</Pill>}
      </div>
      <dl className="mono" style={{ fontSize: 12 }}>
        <dt>workflowId</dt><dd>{run.workflowId} (version {run.versionId})</dd>
        <dt>triggerKind</dt><dd>{run.triggerKind}</dd>
        <dt>startedAt</dt><dd><RelTime ts={run.startedAt} /></dd>
        <dt>endedAt</dt><dd>{run.endedAt ? <RelTime ts={run.endedAt} /> : "—"}</dd>
        <dt>elapsed</dt><dd>{run.elapsedMs}ms</dd>
      </dl>

      {run.pausedState && (
        <div style={{ marginTop: 12, padding: 12, background: "var(--accent-soft)", borderRadius: 4 }}>
          <strong>Paused — persisted state (RehydrateOnBoot)</strong>
          <pre className="raw-json" style={{ marginTop: 6 }}>{JSON.stringify(run.pausedState, null, 2)}</pre>
        </div>
      )}

      <h3>Nodes ({nodes.length})</h3>
      <table className="dt">
        <thead>
          <tr><th>nodeId</th><th>type</th><th>status</th><th>elapsed</th><th>attempts</th><th>conv</th><th></th></tr>
        </thead>
        <tbody>
          {nodes.map((n) => {
            // In the durable model the flowrun status is awaiting_signal when ANY approval is parked.
            // A parked approval node's own FlowRunNode.status can be "running" or "pending"; there is
            // no guaranteed "waiting_approval" node status. Show the approve/reject buttons for all
            // approval-type nodes when the run is awaiting_signal — simple and correct for a dev tool.
            const isPendingApproval = n.nodeType === "approval" && (run?.status === "awaiting_signal" || run?.status === "paused");
            return (
              <tr key={n.id}>
                <td className="mono" style={{ fontSize: 10 }}>{n.nodeId}</td>
                <td><code>{n.nodeType}</code></td>
                <td><StatusBadge status={n.status} /></td>
                <td className="muted">{n.elapsedMs ?? "—"}ms</td>
                <td>{n.attempts}</td>
                <td>{n.conversationId ? <a href={`#/current/wire?conv=${n.conversationId}`} className="mono" style={{ fontSize: 10, color: "var(--accent)" }}>{n.conversationId.slice(-8)}</a> : "—"}</td>
                <td>
                  {isPendingApproval ? (
                    <span>
                      <button onClick={() => approve.mutate({ nodeId: n.nodeId, decision: "approved" })} style={{
                        padding: "2px 8px", fontSize: 10, background: "var(--status-success)", color: "white",
                        border: "none", borderRadius: 3, cursor: "pointer", marginRight: 4,
                      }}>approved</button>
                      <button onClick={() => approve.mutate({ nodeId: n.nodeId, decision: "rejected" })} style={{
                        padding: "2px 8px", fontSize: 10, background: "var(--status-error)", color: "white",
                        border: "none", borderRadius: 3, cursor: "pointer",
                      }}>rejected</button>
                    </span>
                  ) : (
                    <button onClick={() => ui.showRaw(n.nodeId, n)} className="muted" style={{
                      background: "none", border: "1px solid var(--border)",
                      padding: "1px 6px", borderRadius: 3, fontSize: 10, cursor: "pointer",
                    }}>raw</button>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
