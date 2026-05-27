import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON, patchJSON, delJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView, RelTime } from "@/ui";
import type { Memory, CreateMemoryBody, UpdateMemoryBody } from "@frontend/entities/memory/model/types";

type MemoryType = "all" | "user" | "feedback" | "project" | "reference";
const TABS: MemoryType[] = ["all", "user", "feedback", "project", "reference"];

const emptyCreate: CreateMemoryBody = { name: "", type: "user", description: "", content: "", pinned: false, source: "user" };

export function Memory() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<MemoryType>("all");
  const [selected, setSelected] = useState<Memory | null>(null);
  const [editPatch, setEditPatch] = useState<UpdateMemoryBody>({});
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState<CreateMemoryBody>(emptyCreate);

  const { data, isLoading, isError } = useQuery<Memory[]>({
    queryKey: qk.memories(tab === "all" ? undefined : tab),
    queryFn: () => getJSON<Memory[]>(tab === "all" ? "/api/v1/memories" : `/api/v1/memories?type=${tab}`),
  });

  const create = useMutation({
    mutationFn: (body: CreateMemoryBody) => postJSON("/api/v1/memories", body),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["memories"] }); setShowCreate(false); setCreateForm(emptyCreate); },
  });

  const update = useMutation({
    mutationFn: ({ name, patch }: { name: string; patch: UpdateMemoryBody }) =>
      patchJSON(`/api/v1/memories/${encodeURIComponent(name)}`, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["memories"] }),
  });

  const pin = useMutation({
    mutationFn: ({ name, pinned }: { name: string; pinned: boolean }) =>
      patchJSON(`/api/v1/memories/${encodeURIComponent(name)}`, { pinned }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["memories"] }),
  });

  const del = useMutation({
    mutationFn: (name: string) => delJSON(`/api/v1/memories/${encodeURIComponent(name)}`),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["memories"] }); if (selected?.name === del.variables) setSelected(null); },
  });

  if (isLoading) return <EmptyView>loading…</EmptyView>;
  if (isError) return <EmptyView>error loading memories</EmptyView>;

  const pinned = (data ?? []).filter((m) => m.pinned);
  const unpinned = (data ?? []).filter((m) => !m.pinned);

  return (
    <div style={{ display: "flex", height: "100%", overflow: "hidden" }}>
      {/* Left: list */}
      <div style={{ width: 300, borderRight: "1px solid var(--border)", display: "flex", flexDirection: "column", overflow: "hidden" }}>
        {/* Tabs */}
        <div style={{ display: "flex", borderBottom: "1px solid var(--border)", overflowX: "auto" }}>
          {TABS.map((t) => (
            <button key={t} onClick={() => setTab(t)} style={{
              padding: "6px 12px", fontSize: 11, border: "none", cursor: "pointer",
              borderBottom: tab === t ? "2px solid var(--accent)" : "2px solid transparent",
              background: "transparent", fontWeight: tab === t ? 600 : undefined,
            }}>{t}</button>
          ))}
        </div>
        <div style={{ padding: "4px 8px", display: "flex", justifyContent: "flex-end" }}>
          <button onClick={() => setShowCreate((v) => !v)} style={{ fontSize: 11, padding: "2px 8px", cursor: "pointer", border: "1px solid var(--border)", borderRadius: 3, background: "var(--accent)", color: "var(--accent-fg)" }}>
            + new
          </button>
        </div>
        {showCreate && (
          <div style={{ padding: 8, borderBottom: "1px solid var(--border)", display: "flex", flexDirection: "column", gap: 4 }}>
            <input placeholder="name" value={createForm.name} onChange={(e) => setCreateForm((f) => ({ ...f, name: e.target.value }))} style={inp} />
            <input placeholder="description" value={createForm.description} onChange={(e) => setCreateForm((f) => ({ ...f, description: e.target.value }))} style={inp} />
            <textarea placeholder="content" value={createForm.content} onChange={(e) => setCreateForm((f) => ({ ...f, content: e.target.value }))} style={{ ...inp, height: 60, resize: "vertical" }} />
            <select value={createForm.type} onChange={(e) => setCreateForm((f) => ({ ...f, type: e.target.value as CreateMemoryBody["type"] }))} style={inp}>
              {(["user", "feedback", "project", "reference"] as const).map((t) => <option key={t}>{t}</option>)}
            </select>
            <button onClick={() => create.mutate(createForm)} disabled={create.isPending || !createForm.name} style={{ fontSize: 11, padding: "3px 8px", cursor: "pointer", background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 3 }}>
              {create.isPending ? "saving…" : "Create"}
            </button>
          </div>
        )}
        <div style={{ flex: 1, overflow: "auto" }}>
          {(data ?? []).length === 0 && <EmptyView>no memories</EmptyView>}
          {pinned.length > 0 && (
            <>
              <div style={{ padding: "4px 12px", fontSize: 10, fontWeight: 600, color: "var(--fg-muted)", textTransform: "uppercase" }}>Pinned</div>
              {pinned.map((m) => <MemoryRow key={m.name} m={m} selected={selected?.name === m.name} onSelect={setSelected} onPin={(name, p) => pin.mutate({ name, pinned: p })} />)}
            </>
          )}
          {unpinned.length > 0 && (
            <>
              {pinned.length > 0 && <div style={{ padding: "4px 12px", fontSize: 10, fontWeight: 600, color: "var(--fg-muted)", textTransform: "uppercase" }}>Others</div>}
              {unpinned.map((m) => <MemoryRow key={m.name} m={m} selected={selected?.name === m.name} onSelect={setSelected} onPin={(name, p) => pin.mutate({ name, pinned: p })} />)}
            </>
          )}
        </div>
      </div>

      {/* Right: detail */}
      <div style={{ flex: 1, overflow: "auto", padding: 16 }}>
        {!selected && <EmptyView>select a memory</EmptyView>}
        {selected && (
          <>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
              <h3 style={{ margin: 0, fontSize: 15 }}>{selected.name}</h3>
              <div style={{ display: "flex", gap: 8 }}>
                <button onClick={() => update.mutate({ name: selected.name, patch: editPatch })} disabled={update.isPending} style={{ fontSize: 12, padding: "3px 10px", cursor: "pointer", background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 3 }}>
                  {update.isPending ? "saving…" : "Save"}
                </button>
                <button onClick={() => { if (confirm(`Delete memory "${selected.name}"?`)) del.mutate(selected.name); }} style={{ fontSize: 12, padding: "3px 10px", cursor: "pointer", background: "var(--bg-elev)", border: "1px solid var(--border)", borderRadius: 3, color: "var(--status-error)" }}>
                  Delete
                </button>
              </div>
            </div>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12, marginBottom: 12 }}>
              <tbody>
                <tr><td style={labelCell}>source</td><td>{selected.source}</td></tr>
                <tr><td style={labelCell}>type</td><td>
                  <select defaultValue={selected.type} onChange={(e) => setEditPatch((p) => ({ ...p, type: e.target.value as UpdateMemoryBody["type"] }))} style={{ fontSize: 12, padding: "2px 4px" }}>
                    {(["user", "feedback", "project", "reference"] as const).map((t) => <option key={t}>{t}</option>)}
                  </select>
                </td></tr>
                <tr><td style={labelCell}>pinned</td><td>{String(selected.pinned)}</td></tr>
                <tr><td style={labelCell}>accessCount</td><td>{selected.accessCount}</td></tr>
                <tr><td style={labelCell}>createdAt</td><td><RelTime ts={selected.createdAt} /></td></tr>
                <tr><td style={labelCell}>updatedAt</td><td><RelTime ts={selected.updatedAt} /></td></tr>
              </tbody>
            </table>
            <label style={{ fontSize: 11, fontWeight: 600, display: "block", marginBottom: 4, color: "var(--fg-muted)" }}>Description</label>
            <input
              defaultValue={selected.description}
              onChange={(e) => setEditPatch((p) => ({ ...p, description: e.target.value }))}
              style={{ width: "100%", padding: "4px 8px", fontSize: 12, border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)", marginBottom: 12 }}
            />
            <label style={{ fontSize: 11, fontWeight: 600, display: "block", marginBottom: 4, color: "var(--fg-muted)" }}>Content</label>
            <textarea
              defaultValue={selected.content}
              onChange={(e) => setEditPatch((p) => ({ ...p, content: e.target.value }))}
              style={{ width: "100%", minHeight: 200, padding: "6px 8px", fontSize: 12, border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)", fontFamily: "var(--mono)", resize: "vertical" }}
            />
          </>
        )}
      </div>
    </div>
  );
}

function MemoryRow({ m, selected, onSelect, onPin }: { m: Memory; selected: boolean; onSelect: (m: Memory) => void; onPin: (name: string, pinned: boolean) => void }) {
  return (
    <div
      onClick={() => onSelect(m)}
      style={{ padding: "8px 12px", cursor: "pointer", borderBottom: "1px solid var(--border-soft)", background: selected ? "var(--bg-elev)" : undefined, display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 12, fontWeight: 500, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{m.name}</div>
        <div className="muted" style={{ fontSize: 11 }}>{m.type}</div>
      </div>
      <button onClick={(e) => { e.stopPropagation(); onPin(m.name, !m.pinned); }} title={m.pinned ? "unpin" : "pin"} style={{ fontSize: 14, background: "none", border: "none", cursor: "pointer", color: m.pinned ? "var(--accent)" : "var(--fg-muted)" }}>
        {m.pinned ? "★" : "☆"}
      </button>
    </div>
  );
}

const inp: React.CSSProperties = { padding: "4px 6px", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)", fontSize: 12, width: "100%", boxSizing: "border-box" };
const labelCell: React.CSSProperties = { padding: "4px 8px 4px 0", fontWeight: 600, color: "var(--fg-muted)", width: 120, verticalAlign: "top" };
