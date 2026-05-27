import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView, StatusBadge, RelTime } from "@/ui";
import type { McpServer } from "@frontend/entities/mcp/model/types";

export function MCPServers() {
  const qc = useQueryClient();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const { data, isLoading, isError } = useQuery<McpServer[]>({
    queryKey: qk.mcpServers(),
    queryFn: () => getJSON<McpServer[]>("/api/v1/mcp-servers"),
  });

  const reconnect = useMutation({
    mutationFn: (name: string) => postJSON(`/api/v1/mcp-servers/${encodeURIComponent(name)}:reconnect`),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.mcpServers() }),
  });

  const toggleExpand = (name: string) => setExpanded((s) => {
    const next = new Set(s);
    next.has(name) ? next.delete(name) : next.add(name);
    return next;
  });

  if (isLoading) return <EmptyView>loading…</EmptyView>;
  if (isError) return <EmptyView>error loading MCP servers</EmptyView>;
  if (!data || data.length === 0) return <EmptyView>no MCP servers configured</EmptyView>;

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden" }}>
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)" }}>
        <strong style={{ fontSize: 13 }}>MCP Servers</strong>
        <span className="muted" style={{ marginLeft: 8, fontSize: 11 }}>{data.length} servers</span>
      </div>
      <div style={{ flex: 1, overflow: "auto" }}>
        <table className="dt" style={{ width: "100%" }}>
          <thead>
            <tr>
              <th>Name</th><th>Status</th><th>PID</th><th>Connected</th>
              <th>Failures</th><th>Total Calls</th><th>Tools</th><th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {data.map((srv) => (
              <>
                <tr key={srv.name}>
                  <td style={{ fontWeight: 500 }}>{srv.name}</td>
                  <td><StatusBadge status={srv.status} /></td>
                  <td className="muted" style={{ fontSize: 11 }}>{srv.pid ?? "—"}</td>
                  <td>{srv.connectedAt ? <RelTime ts={srv.connectedAt} /> : <span className="muted">—</span>}</td>
                  <td style={{ color: srv.consecutiveFailures > 0 ? "var(--status-error)" : undefined }}>
                    {srv.consecutiveFailures}
                  </td>
                  <td>{srv.totalCalls}</td>
                  <td>
                    <button
                      onClick={() => toggleExpand(srv.name)}
                      style={{ fontSize: 11, padding: "1px 6px", cursor: "pointer", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-elev)" }}
                    >
                      {srv.tools.length} {expanded.has(srv.name) ? "▲" : "▼"}
                    </button>
                  </td>
                  <td>
                    <button
                      onClick={() => reconnect.mutate(srv.name)}
                      disabled={reconnect.isPending}
                      style={{ fontSize: 11, padding: "2px 8px", cursor: "pointer", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-elev)" }}
                    >
                      reconnect
                    </button>
                  </td>
                </tr>
                {expanded.has(srv.name) && srv.tools.length > 0 && (
                  <tr key={`${srv.name}-tools`}>
                    <td colSpan={8} style={{ padding: "0 8px 8px 24px", background: "var(--bg-elev)" }}>
                      <ul style={{ margin: 0, padding: 0, listStyle: "none", fontSize: 11 }}>
                        {srv.tools.map((t) => (
                          <li key={t.name} style={{ padding: "2px 0" }}>
                            <strong>{t.name}</strong>
                            {t.description && <span className="muted" style={{ marginLeft: 8 }}>{t.description}</span>}
                          </li>
                        ))}
                      </ul>
                    </td>
                  </tr>
                )}
              </>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
