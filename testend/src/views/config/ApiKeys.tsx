import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON, patchJSON, delJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView, StatusBadge, RelTime } from "@/ui";
import type { ApiKey } from "@frontend/entities/apikey/model/types";

const API_FORMATS = ["openai", "anthropic", "google", "ollama"];

interface CreateForm {
  provider: string;
  displayName: string;
  key: string;
  baseUrl: string;
  apiFormat: string;
}

const defaultForm: CreateForm = { provider: "", displayName: "", key: "", baseUrl: "", apiFormat: "openai" };

export function ApiKeys() {
  const qc = useQueryClient();
  const [form, setForm] = useState<CreateForm>(defaultForm);
  const [showCreate, setShowCreate] = useState(false);

  const { data, isLoading, isError } = useQuery<ApiKey[]>({
    queryKey: qk.apikeys(),
    queryFn: () => getJSON<ApiKey[]>("/api/v1/api-keys"),
  });

  const create = useMutation({
    mutationFn: (body: CreateForm) => postJSON("/api/v1/api-keys", body),
    onSuccess: () => { qc.invalidateQueries({ queryKey: qk.apikeys() }); setForm(defaultForm); setShowCreate(false); },
  });

  const test = useMutation({
    mutationFn: (id: string) => postJSON(`/api/v1/api-keys/${id}:test`),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.apikeys() }),
  });

  const setDefault = useMutation({
    mutationFn: (id: string) => patchJSON(`/api/v1/api-keys/${id}`, { isDefault: true }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.apikeys() }),
  });

  const del = useMutation({
    mutationFn: (id: string) => delJSON(`/api/v1/api-keys/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.apikeys() }),
  });

  if (isLoading) return <EmptyView>loading…</EmptyView>;
  if (isError) return <EmptyView>error loading api keys</EmptyView>;

  // Group by category (provider) for is_default radio
  const byProvider = (data ?? []).reduce<Record<string, ApiKey[]>>((acc, k) => {
    (acc[k.provider] ??= []).push(k);
    return acc;
  }, {});

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden" }}>
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", display: "flex", gap: 8, alignItems: "center" }}>
        <strong style={{ fontSize: 13 }}>API Keys</strong>
        <button onClick={() => setShowCreate((v) => !v)} style={{
          marginLeft: "auto", padding: "3px 10px", fontSize: 12,
          background: "var(--accent)", color: "var(--accent-fg)",
          border: "none", borderRadius: 4, cursor: "pointer",
        }}>+ Add</button>
      </div>

      {showCreate && (
        <div style={{ padding: 12, borderBottom: "1px solid var(--border)", display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
          <input placeholder="provider" value={form.provider} onChange={(e) => setForm((f) => ({ ...f, provider: e.target.value }))} style={inp} />
          <input placeholder="displayName" value={form.displayName} onChange={(e) => setForm((f) => ({ ...f, displayName: e.target.value }))} style={inp} />
          <input placeholder="key (secret)" type="password" value={form.key} onChange={(e) => setForm((f) => ({ ...f, key: e.target.value }))} style={inp} />
          <input placeholder="baseUrl (optional)" value={form.baseUrl} onChange={(e) => setForm((f) => ({ ...f, baseUrl: e.target.value }))} style={{ ...inp, width: 180 }} />
          <select value={form.apiFormat} onChange={(e) => setForm((f) => ({ ...f, apiFormat: e.target.value }))} style={inp}>
            {API_FORMATS.map((f) => <option key={f}>{f}</option>)}
          </select>
          <button onClick={() => create.mutate(form)} disabled={create.isPending || !form.provider || !form.key} style={btn}>
            {create.isPending ? "saving…" : "Create"}
          </button>
          <button onClick={() => setShowCreate(false)} style={{ ...btn, background: "var(--bg-elev)", color: "var(--fg)" }}>Cancel</button>
        </div>
      )}

      <div style={{ flex: 1, overflow: "auto" }}>
        {(data ?? []).length === 0 && <EmptyView>no api keys yet</EmptyView>}
        {Object.entries(byProvider).map(([prov, keys]) => (
          <div key={prov}>
            <div style={{ padding: "6px 12px", fontSize: 11, fontWeight: 600, background: "var(--bg-elev)", color: "var(--fg-muted)", textTransform: "uppercase", letterSpacing: 1 }}>
              {prov}
            </div>
            <table className="dt" style={{ width: "100%" }}>
              <thead>
                <tr>
                  <th>Default</th><th>Display Name</th><th>Key</th><th>Format</th>
                  <th>Status</th><th>Tested</th><th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {keys.map((k) => (
                  <tr key={k.id}>
                    <td style={{ textAlign: "center" }}>
                      <input type="radio" name={`default-${prov}`} checked={k.isDefault} onChange={() => setDefault.mutate(k.id)} title="set as default" />
                    </td>
                    <td>{k.displayName}</td>
                    <td style={{ fontFamily: "var(--mono)", fontSize: 11 }}>{k.keyMasked}</td>
                    <td>{k.apiFormat}</td>
                    <td><StatusBadge status={k.testStatus} /></td>
                    <td>{k.lastTestedAt ? <RelTime ts={k.lastTestedAt} /> : <span className="muted">—</span>}</td>
                    <td style={{ display: "flex", gap: 4 }}>
                      <button onClick={() => test.mutate(k.id)} disabled={test.isPending} style={smallBtn}>test</button>
                      <button onClick={() => { if (confirm(`Delete ${k.displayName}?`)) del.mutate(k.id); }} style={{ ...smallBtn, color: "var(--status-error)" }}>del</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      </div>
    </div>
  );
}

const inp: React.CSSProperties = {
  padding: "4px 8px", border: "1px solid var(--border)", borderRadius: 3,
  background: "var(--bg-paper)", fontSize: 12, width: 140,
};
const btn: React.CSSProperties = {
  padding: "4px 12px", background: "var(--accent)", color: "var(--accent-fg)",
  border: "none", borderRadius: 4, cursor: "pointer", fontSize: 12,
};
const smallBtn: React.CSSProperties = {
  padding: "2px 8px", fontSize: 11, background: "var(--bg-elev)",
  border: "1px solid var(--border)", borderRadius: 3, cursor: "pointer",
};
