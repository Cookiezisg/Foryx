import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { postJSON, patchJSON, delJSON } from "@/api/devClient";
import { useUsersStore } from "@/stores/users";
import { EmptyView, RelTime } from "@/ui";
import type { User, CreateUserBody } from "@frontend/entities/user/model/types";

const defaultCreate: CreateUserBody = { username: "", displayName: "", avatarColor: "#6366f1", language: "en" };

export function Profile() {
  const { list, activeId, refresh, setActive } = useUsersStore();
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState<CreateUserBody>(defaultCreate);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editPatch, setEditPatch] = useState<{ displayName?: string; avatarColor?: string; language?: string }>({});
  const [confirmDel, setConfirmDel] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: (body: CreateUserBody) => postJSON<User>("/api/v1/users", body),
    onSuccess: () => { refresh(); qc.invalidateQueries({ queryKey: ["users"] }); setShowCreate(false); setCreateForm(defaultCreate); },
  });

  const update = useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: typeof editPatch }) =>
      patchJSON<User>(`/api/v1/users/${id}`, patch),
    onSuccess: () => { refresh(); setEditingId(null); setEditPatch({}); },
  });

  const del = useMutation({
    mutationFn: (id: string) => delJSON(`/api/v1/users/${id}`),
    onSuccess: () => { refresh(); setConfirmDel(null); },
  });

  if (list.length === 0) {
    return (
      <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden" }}>
        <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)" }}>
          <strong style={{ fontSize: 13 }}>Profile</strong>
          <button onClick={() => setShowCreate((v) => !v)} style={{ marginLeft: 12, fontSize: 12, padding: "3px 10px", background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 4, cursor: "pointer" }}>+ New User</button>
        </div>
        {showCreate && <CreateForm form={createForm} setForm={setCreateForm} onSubmit={() => create.mutate(createForm)} isPending={create.isPending} onCancel={() => setShowCreate(false)} />}
        <EmptyView>no users yet</EmptyView>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", overflow: "hidden" }}>
      <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center" }}>
        <strong style={{ fontSize: 13 }}>Profile</strong>
        <button onClick={() => setShowCreate((v) => !v)} style={{ marginLeft: "auto", fontSize: 12, padding: "3px 10px", background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 4, cursor: "pointer" }}>
          + New User
        </button>
      </div>

      {showCreate && <CreateForm form={createForm} setForm={setCreateForm} onSubmit={() => create.mutate(createForm)} isPending={create.isPending} onCancel={() => setShowCreate(false)} />}

      <div style={{ flex: 1, overflow: "auto" }}>
        <table className="dt" style={{ width: "100%" }}>
          <thead>
            <tr>
              <th>Active</th><th>Username</th><th>Display Name</th><th>Color</th>
              <th>Language</th><th>Last Used</th><th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {list.map((u) => (
              <>
                <tr key={u.id} style={{ background: activeId === u.id ? "var(--bg-elev)" : undefined }}>
                  <td style={{ textAlign: "center" }}>
                    {activeId === u.id
                      ? <span style={{ fontSize: 12, color: "var(--accent)", fontWeight: 700 }}>✓</span>
                      : <button onClick={() => setActive(u.id)} style={{ fontSize: 11, padding: "2px 8px", cursor: "pointer", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-elev)" }}>switch</button>
                    }
                  </td>
                  <td style={{ fontFamily: "var(--mono)", fontSize: 12 }}>{u.username}</td>
                  <td>{u.displayName || <span className="muted">—</span>}</td>
                  <td>
                    <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
                      <div style={{ width: 16, height: 16, borderRadius: "50%", background: u.avatarColor, border: "1px solid var(--border)" }} />
                      <span style={{ fontSize: 11, fontFamily: "var(--mono)" }}>{u.avatarColor}</span>
                    </div>
                  </td>
                  <td>{u.language || <span className="muted">—</span>}</td>
                  <td>{u.lastUsedAt ? <RelTime ts={u.lastUsedAt} /> : <span className="muted">—</span>}</td>
                  <td style={{ display: "flex", gap: 4 }}>
                    <button onClick={() => { setEditingId(u.id); setEditPatch({}); }} style={smallBtn}>edit</button>
                    <button onClick={() => setConfirmDel(u.id)} style={{ ...smallBtn, color: "var(--status-error)" }}>del</button>
                  </td>
                </tr>
                {editingId === u.id && (
                  <tr key={`${u.id}-edit`}>
                    <td colSpan={7} style={{ padding: 12, background: "var(--bg-elev)", borderBottom: "1px solid var(--border)" }}>
                      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
                        <input placeholder="displayName" defaultValue={u.displayName} onChange={(e) => setEditPatch((p) => ({ ...p, displayName: e.target.value }))} style={inp} />
                        <input placeholder="avatarColor" defaultValue={u.avatarColor} onChange={(e) => setEditPatch((p) => ({ ...p, avatarColor: e.target.value }))} style={{ ...inp, width: 120 }} />
                        <input placeholder="language" defaultValue={u.language} onChange={(e) => setEditPatch((p) => ({ ...p, language: e.target.value }))} style={{ ...inp, width: 80 }} />
                        <button onClick={() => update.mutate({ id: u.id, patch: editPatch })} disabled={update.isPending} style={saveBtn}>{update.isPending ? "saving…" : "Save"}</button>
                        <button onClick={() => setEditingId(null)} style={{ ...saveBtn, background: "var(--bg-paper)", color: "var(--fg)", border: "1px solid var(--border)" }}>Cancel</button>
                      </div>
                    </td>
                  </tr>
                )}
                {confirmDel === u.id && (
                  <tr key={`${u.id}-del`}>
                    <td colSpan={7} style={{ padding: 10, background: "#fef2f2", borderBottom: "1px solid var(--border)" }}>
                      <span style={{ fontSize: 12, marginRight: 12 }}>Delete user <strong>{u.username}</strong>?</span>
                      <button onClick={() => del.mutate(u.id)} disabled={del.isPending} style={{ ...saveBtn, background: "var(--status-error)", color: "#fff" }}>
                        {del.isPending ? "deleting…" : "Confirm"}
                      </button>
                      <button onClick={() => setConfirmDel(null)} style={{ ...saveBtn, marginLeft: 6, background: "var(--bg-paper)", color: "var(--fg)", border: "1px solid var(--border)" }}>Cancel</button>
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

function CreateForm({ form, setForm, onSubmit, isPending, onCancel }: {
  form: CreateUserBody;
  setForm: (f: CreateUserBody) => void;
  onSubmit: () => void;
  isPending: boolean;
  onCancel: () => void;
}) {
  return (
    <div style={{ padding: 12, borderBottom: "1px solid var(--border)", display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end", background: "var(--bg-elev)" }}>
      <input placeholder="username *" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} style={inp} />
      <input placeholder="displayName" value={form.displayName ?? ""} onChange={(e) => setForm({ ...form, displayName: e.target.value })} style={inp} />
      <input placeholder="avatarColor" value={form.avatarColor ?? ""} onChange={(e) => setForm({ ...form, avatarColor: e.target.value })} style={{ ...inp, width: 120 }} />
      <input placeholder="language" value={form.language ?? ""} onChange={(e) => setForm({ ...form, language: e.target.value })} style={{ ...inp, width: 80 }} />
      <button onClick={onSubmit} disabled={isPending || !form.username} style={saveBtn}>{isPending ? "creating…" : "Create"}</button>
      <button onClick={onCancel} style={{ ...saveBtn, background: "var(--bg-paper)", color: "var(--fg)", border: "1px solid var(--border)" }}>Cancel</button>
    </div>
  );
}

const inp: React.CSSProperties = { padding: "4px 8px", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)", fontSize: 12, width: 160 };
const saveBtn: React.CSSProperties = { padding: "4px 12px", background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 3, cursor: "pointer", fontSize: 12 };
const smallBtn: React.CSSProperties = { padding: "2px 8px", fontSize: 11, background: "var(--bg-elev)", border: "1px solid var(--border)", borderRadius: 3, cursor: "pointer" };
