import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getPage } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView, StatusBadge, RelTime, Pill } from "@/ui";
import type { FlowRun } from "@frontend/entities/flowrun/model/types";

// awaiting_signal = durable approval park; paused = legacy (kept for backwards display compat).
const STATUSES = ["", "running", "awaiting_signal", "paused", "completed", "failed", "cancelled"];

export function FlowRuns() {
  const [status, setStatus] = useState("");
  const [workflowId, setWorkflowId] = useState("");
  const { data } = useQuery({
    queryKey: qk.flowruns({ status, workflowId }),
    queryFn: () => getPage<FlowRun>("/api/v1/flowruns", {
      status: status || undefined,
      workflowId: workflowId || undefined,
      limit: 50,
    }),
  });
  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div style={{ padding: 8, display: "flex", gap: 8, borderBottom: "1px solid var(--border)" }}>
        <select value={status} onChange={(e) => setStatus(e.target.value)} style={{
          padding: "4px 8px", border: "1px solid var(--border)", borderRadius: 3, fontSize: 12,
        }}>
          {STATUSES.map((s) => <option key={s} value={s}>{s || "(any status)"}</option>)}
        </select>
        <input value={workflowId} onChange={(e) => setWorkflowId(e.target.value)} placeholder="workflowId filter…" style={{
          flex: 1, padding: "4px 8px", border: "1px solid var(--border)", borderRadius: 3, fontSize: 12,
        }} />
      </div>
      <div style={{ flex: 1, overflow: "auto" }}>
        {!data || data.data.length === 0 ? (
          <EmptyView>no flowruns match</EmptyView>
        ) : (
          <table className="dt">
            <thead>
              <tr><th>id</th><th>workflow</th><th>trigger</th><th>status</th><th>started</th><th>elapsed</th><th>dryRun</th></tr>
            </thead>
            <tbody>
              {data.data.map((r) => (
                <tr key={r.id}>
                  <td><Link to={`/execute/flowruns/${r.id}`} className="mono" style={{ color: "var(--accent)", fontSize: 10 }}>{r.id.slice(-12)}</Link></td>
                  <td className="mono" style={{ fontSize: 10 }}>{r.workflowId}</td>
                  <td className="muted">{r.triggerKind}</td>
                  <td><StatusBadge status={r.status} /></td>
                  <td className="muted"><RelTime ts={r.startedAt} /></td>
                  <td className="muted">{r.elapsedMs ?? "—"}ms</td>
                  <td>{r.dryRun ? <Pill kind="info">dry</Pill> : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
