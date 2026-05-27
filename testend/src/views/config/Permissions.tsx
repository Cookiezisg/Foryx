// Permissions view — read-only tables for rules + hooks.
// Inline editing is deferred (P3.C ships display-only; CRUD TBD).
import { useQuery } from "@tanstack/react-query";
import { getJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView } from "@/ui";

interface PermissionRule {
  mode: "allow" | "ask" | "deny";
  pattern: string;
  scope?: string;
}

interface PermissionHook {
  event: string;
  command: string;
  match?: string;
}

interface PermissionsResponse {
  rules: PermissionRule[];
  hooks: PermissionHook[];
}

export function Permissions() {
  const { data, isLoading, isError } = useQuery<PermissionsResponse>({
    queryKey: qk.permissions(),
    queryFn: () => getJSON<PermissionsResponse>("/api/v1/permissions"),
  });

  if (isLoading) return <EmptyView>loading…</EmptyView>;
  if (isError) return <EmptyView>endpoint not implemented yet — /api/v1/permissions not available</EmptyView>;

  const rules = data?.rules ?? [];
  const hooks = data?.hooks ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "auto" }}>
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)" }}>
        <strong style={{ fontSize: 13 }}>Permissions</strong>
        <span className="muted" style={{ marginLeft: 8, fontSize: 11 }}>(read-only display; editing TBD)</span>
      </div>

      {/* Rules */}
      <div style={{ padding: "6px 12px", fontWeight: 600, fontSize: 12, background: "var(--bg-elev)", borderBottom: "1px solid var(--border)" }}>
        Rules ({rules.length})
      </div>
      {rules.length === 0 ? (
        <EmptyView>no rules configured</EmptyView>
      ) : (
        <table className="dt" style={{ width: "100%" }}>
          <thead>
            <tr><th>Mode</th><th>Pattern</th><th>Scope</th></tr>
          </thead>
          <tbody>
            {rules.map((r, i) => (
              <tr key={i}>
                <td>
                  <span style={{
                    fontSize: 11, fontWeight: 600, padding: "2px 6px", borderRadius: 3,
                    background: r.mode === "allow" ? "var(--status-success-bg)" : r.mode === "deny" ? "var(--status-error-bg)" : "var(--status-warn-bg)",
                    color: r.mode === "allow" ? "var(--status-success)" : r.mode === "deny" ? "var(--status-error)" : "var(--status-warn)",
                  }}>{r.mode}</span>
                </td>
                <td style={{ fontFamily: "var(--mono)", fontSize: 12 }}>{r.pattern}</td>
                <td className="muted">{r.scope ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {/* Hooks */}
      <div style={{ padding: "6px 12px", fontWeight: 600, fontSize: 12, background: "var(--bg-elev)", borderBottom: "1px solid var(--border)", marginTop: 16 }}>
        Hooks ({hooks.length})
      </div>
      {hooks.length === 0 ? (
        <EmptyView>no hooks configured</EmptyView>
      ) : (
        <table className="dt" style={{ width: "100%" }}>
          <thead>
            <tr><th>Event</th><th>Command</th><th>Match</th></tr>
          </thead>
          <tbody>
            {hooks.map((h, i) => (
              <tr key={i}>
                <td style={{ fontFamily: "var(--mono)", fontSize: 12 }}>{h.event}</td>
                <td style={{ fontFamily: "var(--mono)", fontSize: 12 }}>{h.command}</td>
                <td className="muted">{h.match ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
