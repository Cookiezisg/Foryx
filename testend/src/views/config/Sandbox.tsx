import { useQuery } from "@tanstack/react-query";
import { getJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView, StatusBadge, RelTime } from "@/ui";

interface SandboxRuntime {
  id: string;
  language: string;
  version: string;
  installedAt: string;
  status: string;
}

interface SandboxOwner {
  kind: "function" | "handler" | "mcp" | "skill" | "conversation";
  id: string;
}

interface SandboxEnv {
  id: string;
  runtimeId: string;
  owner: SandboxOwner;
  envStatus: string;
  sizeBytes: number;
  lastUsedAt: string | null;
  lruRank?: number;
}

export function Sandbox() {
  const runtimes = useQuery<SandboxRuntime[]>({
    queryKey: qk.sandboxRuntimes(),
    queryFn: () => getJSON<SandboxRuntime[]>("/api/v1/sandbox/runtimes"),
  });

  const envs = useQuery<SandboxEnv[]>({
    queryKey: qk.sandboxEnvs(),
    queryFn: () => getJSON<SandboxEnv[]>("/api/v1/sandbox/envs"),
  });

  const isLoading = runtimes.isLoading || envs.isLoading;
  if (isLoading) return <EmptyView>loading…</EmptyView>;

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "auto" }}>
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)" }}>
        <strong style={{ fontSize: 13 }}>Sandbox</strong>
      </div>

      {/* Runtimes */}
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", fontWeight: 600, fontSize: 12, background: "var(--bg-elev)" }}>
        Runtimes ({runtimes.data?.length ?? 0})
      </div>
      {runtimes.isError && <EmptyView>error loading runtimes</EmptyView>}
      {!runtimes.isError && (runtimes.data?.length ?? 0) === 0 && <EmptyView>no runtimes installed</EmptyView>}
      {(runtimes.data?.length ?? 0) > 0 && (
        <table className="dt" style={{ width: "100%" }}>
          <thead>
            <tr><th>ID</th><th>Language</th><th>Version</th><th>Status</th><th>Installed</th></tr>
          </thead>
          <tbody>
            {runtimes.data!.map((r) => (
              <tr key={r.id}>
                <td style={{ fontFamily: "var(--mono)", fontSize: 10 }}>{r.id}</td>
                <td>{r.language}</td>
                <td style={{ fontFamily: "var(--mono)", fontSize: 11 }}>{r.version}</td>
                <td><StatusBadge status={r.status} /></td>
                <td><RelTime ts={r.installedAt} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {/* Envs */}
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", fontWeight: 600, fontSize: 12, background: "var(--bg-elev)", marginTop: 16 }}>
        Envs ({envs.data?.length ?? 0})
      </div>
      {envs.isError && <EmptyView>error loading sandbox envs</EmptyView>}
      {!envs.isError && (envs.data?.length ?? 0) === 0 && <EmptyView>no sandbox envs</EmptyView>}
      {(envs.data?.length ?? 0) > 0 && (
        <table className="dt" style={{ width: "100%" }}>
          <thead>
            <tr><th>ID</th><th>Owner Kind</th><th>Owner ID</th><th>Runtime</th><th>Status</th><th>Size</th><th>Last Used</th><th>LRU</th></tr>
          </thead>
          <tbody>
            {envs.data!.map((e) => (
              <tr key={e.id}>
                <td style={{ fontFamily: "var(--mono)", fontSize: 10 }}>{e.id}</td>
                <td>{e.owner.kind}</td>
                <td style={{ fontFamily: "var(--mono)", fontSize: 10 }}>{e.owner.id}</td>
                <td style={{ fontFamily: "var(--mono)", fontSize: 10 }}>{e.runtimeId}</td>
                <td><StatusBadge status={e.envStatus} /></td>
                <td style={{ fontSize: 11 }}>{(e.sizeBytes / 1024).toFixed(1)} KB</td>
                <td>{e.lastUsedAt ? <RelTime ts={e.lastUsedAt} /> : <span className="muted">—</span>}</td>
                <td className="muted">{e.lruRank ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
