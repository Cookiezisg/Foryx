import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, putJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView } from "@/ui";
import type { ModelConfig, Provider, Scenario } from "@frontend/entities/model-config/model/types";

interface RowState {
  provider: string;
  modelId: string;
}

export function ModelConfigs() {
  const qc = useQueryClient();
  const [edits, setEdits] = useState<Record<string, RowState>>({});

  const configs = useQuery<ModelConfig[]>({
    queryKey: qk.modelConfigs(),
    queryFn: () => getJSON<ModelConfig[]>("/api/v1/model-configs"),
  });
  const providers = useQuery<Provider[]>({
    queryKey: qk.providers(),
    queryFn: () => getJSON<Provider[]>("/api/v1/providers"),
  });
  const scenarios = useQuery<Scenario[]>({
    queryKey: qk.scenarios(),
    queryFn: () => getJSON<Scenario[]>("/api/v1/scenarios"),
  });

  // Seed edits from loaded configs
  useEffect(() => {
    if (!configs.data) return;
    const init: Record<string, RowState> = {};
    for (const c of configs.data) {
      init[c.scenario] = { provider: c.provider, modelId: c.modelId };
    }
    setEdits(init);
  }, [configs.data]);

  const save = useMutation({
    mutationFn: ({ scenario, body }: { scenario: string; body: RowState }) =>
      putJSON(`/api/v1/model-configs/${scenario}`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.modelConfigs() }),
  });

  if (configs.isLoading || scenarios.isLoading) return <EmptyView>loading…</EmptyView>;
  if (configs.isError) return <EmptyView>error loading model configs</EmptyView>;

  const providerList = providers.data ?? [];
  const scenarioList = scenarios.data ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden" }}>
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)" }}>
        <strong style={{ fontSize: 13 }}>Model Configs</strong>
        <span className="muted" style={{ marginLeft: 8, fontSize: 11 }}>{scenarioList.length} scenarios</span>
      </div>
      <div style={{ flex: 1, overflow: "auto" }}>
        {scenarioList.length === 0 && <EmptyView>no scenarios defined</EmptyView>}
        <table className="dt" style={{ width: "100%" }}>
          <thead>
            <tr>
              <th>Scenario</th>
              <th>Provider</th>
              <th>Model ID</th>
              <th>Action</th>
            </tr>
          </thead>
          <tbody>
            {scenarioList.map(({ name: scenario }) => {
              const row = edits[scenario] ?? { provider: "", modelId: "" };
              return (
                <tr key={scenario}>
                  <td style={{ fontFamily: "var(--mono)", fontSize: 12 }}>{scenario}</td>
                  <td>
                    <select
                      value={row.provider}
                      onChange={(e) => setEdits((s) => ({ ...s, [scenario]: { ...row, provider: e.target.value } }))}
                      style={{ padding: "3px 6px", fontSize: 12, border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)" }}
                    >
                      <option value="">— none —</option>
                      {providerList.map((p) => (
                        <option key={p.name} value={p.name}>{p.displayName ?? p.name}</option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <input
                      value={row.modelId}
                      onChange={(e) => setEdits((s) => ({ ...s, [scenario]: { ...row, modelId: e.target.value } }))}
                      placeholder="e.g. gpt-4o"
                      style={{ padding: "3px 6px", fontSize: 12, border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)", width: 180 }}
                    />
                  </td>
                  <td>
                    <button
                      onClick={() => save.mutate({ scenario, body: row })}
                      disabled={save.isPending || !row.provider || !row.modelId}
                      style={{ padding: "3px 10px", fontSize: 11, background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 3, cursor: "pointer" }}
                    >
                      Save
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
