import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView, StatusBadge, RelTime } from "@/ui";

interface LLMHealthEntry {
  provider: string;
  status: string;
  lastChecked?: string;
  latencyMs?: number;
  errorCount?: number;
  errorCode?: string;
  keyId?: string;
}

export function LLMHealth() {
  const qc = useQueryClient();

  const { data, isLoading, isError } = useQuery<LLMHealthEntry[]>({
    queryKey: qk.llmHealth(),
    queryFn: () => getJSON<LLMHealthEntry[]>("/api/v1/llm/health"),
  });

  const refresh = useMutation({
    mutationFn: (keyId: string) => postJSON(`/api/v1/api-keys/${keyId}:test`),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.llmHealth() }),
  });

  if (isLoading) return <EmptyView>loading…</EmptyView>;
  if (isError) return <EmptyView>endpoint not implemented yet — /api/v1/llm/health not available</EmptyView>;
  if (!data || data.length === 0) return <EmptyView>no LLM health data</EmptyView>;

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden" }}>
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", gap: 8 }}>
        <strong style={{ fontSize: 13 }}>LLM Health</strong>
        <button
          onClick={() => qc.invalidateQueries({ queryKey: qk.llmHealth() })}
          style={{ marginLeft: "auto", fontSize: 11, padding: "2px 8px", cursor: "pointer", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-elev)" }}
        >
          refresh all
        </button>
      </div>
      <div style={{ flex: 1, overflow: "auto" }}>
        <table className="dt" style={{ width: "100%" }}>
          <thead>
            <tr>
              <th>Provider</th>
              <th>Status</th>
              <th>Last Checked</th>
              <th>Latency (ms)</th>
              <th>Error Count</th>
              <th>Error Code</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {data.map((entry) => (
              <tr key={entry.provider}>
                <td style={{ fontWeight: 500 }}>{entry.provider}</td>
                <td><StatusBadge status={entry.status ?? "unknown"} /></td>
                <td>{entry.lastChecked ? <RelTime ts={entry.lastChecked} /> : <span className="muted">—</span>}</td>
                <td style={{ fontFamily: "var(--mono)", fontSize: 12 }}>
                  {entry.latencyMs != null ? `${entry.latencyMs} ms` : <span className="muted">—</span>}
                </td>
                <td style={{ color: (entry.errorCount ?? 0) > 0 ? "var(--status-error)" : undefined }}>
                  {entry.errorCount ?? 0}
                </td>
                <td style={{ fontFamily: "var(--mono)", fontSize: 11 }}>
                  {entry.errorCode ?? <span className="muted">—</span>}
                </td>
                <td>
                  {entry.keyId ? (
                    <button
                      onClick={() => refresh.mutate(entry.keyId!)}
                      disabled={refresh.isPending}
                      style={{ fontSize: 11, padding: "2px 8px", cursor: "pointer", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-elev)" }}
                    >
                      test key
                    </button>
                  ) : (
                    <span className="muted" style={{ fontSize: 11 }}>—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
