// P3.C: Documents view — Notion-style click+edit tree.
// Drag-reorder deferred to future phase (P3.C ships click+edit; drag-reorder TBD).
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON, patchJSON, delJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView } from "@/ui";
import { MonacoEditor } from "@/ui/MonacoEditor";
import type { DocTreeNode, Document, CreateDocumentBody, UpdateDocumentPatch } from "@frontend/entities/document/model/types";

export function Documents() {
  const qc = useQueryClient();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [editPatch, setEditPatch] = useState<UpdateDocumentPatch>({});
  const [showCreate, setShowCreate] = useState(false);
  const [createName, setCreateName] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  const tree = useQuery<DocTreeNode[]>({
    queryKey: qk.documentsTree(),
    queryFn: () => getJSON<DocTreeNode[]>("/api/v1/documents/tree"),
  });

  const detail = useQuery<Document>({
    queryKey: qk.document(selectedId ?? ""),
    queryFn: () => getJSON<Document>(`/api/v1/documents/${selectedId}`),
    enabled: !!selectedId,
  });

  const create = useMutation({
    mutationFn: (body: CreateDocumentBody) => postJSON("/api/v1/documents", body),
    onSuccess: () => { qc.invalidateQueries({ queryKey: qk.documentsTree() }); setShowCreate(false); setCreateName(""); },
  });

  const save = useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: UpdateDocumentPatch }) =>
      patchJSON(`/api/v1/documents/${id}`, patch),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: qk.documentsTree() });
      qc.invalidateQueries({ queryKey: qk.document(id) });
    },
  });

  const del = useMutation({
    mutationFn: (id: string) => delJSON(`/api/v1/documents/${id}`),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: qk.documentsTree() });
      if (selectedId === id) setSelectedId(null);
    },
  });

  const toggleCollapse = (id: string) => setCollapsed((s) => {
    const n = new Set(s);
    n.has(id) ? n.delete(id) : n.add(id);
    return n;
  });

  if (tree.isLoading) return <EmptyView>loading…</EmptyView>;
  if (tree.isError) return <EmptyView>error loading document tree</EmptyView>;

  const nodes = tree.data ?? [];

  // Build child map for tree rendering
  const childrenOf = (parentId: string | null) =>
    nodes.filter((n) => n.parentId === parentId).sort((a, b) => a.position - b.position);

  function renderNode(node: DocTreeNode, depth: number): React.ReactNode {
    const children = childrenOf(node.id);
    const isOpen = !collapsed.has(node.id);
    return (
      <div key={node.id}>
        <div
          onClick={() => setSelectedId(node.id)}
          style={{
            padding: `4px 8px 4px ${12 + depth * 16}px`,
            cursor: "pointer", display: "flex", alignItems: "center", gap: 4,
            borderBottom: "1px solid var(--border-soft)",
            background: selectedId === node.id ? "var(--bg-elev)" : undefined,
            fontSize: 13,
          }}
        >
          {children.length > 0 && (
            <span onClick={(e) => { e.stopPropagation(); toggleCollapse(node.id); }} style={{ cursor: "pointer", color: "var(--fg-muted)", fontSize: 10, width: 12 }}>
              {isOpen ? "▼" : "▶"}
            </span>
          )}
          {children.length === 0 && <span style={{ width: 12 }} />}
          <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{node.name}</span>
          <span className="muted" style={{ fontSize: 10 }}>{(node.sizeBytes / 1024).toFixed(1)}K</span>
        </div>
        {isOpen && children.map((child) => renderNode(child, depth + 1))}
      </div>
    );
  }

  const roots = childrenOf(null);
  const doc = detail.data;

  return (
    <div style={{ display: "flex", height: "100%", overflow: "hidden" }}>
      {/* Left tree */}
      <div style={{ width: 280, borderRight: "1px solid var(--border)", display: "flex", flexDirection: "column", overflow: "hidden" }}>
        <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", gap: 8 }}>
          <strong style={{ fontSize: 13 }}>Documents</strong>
          <span className="muted" style={{ fontSize: 11 }}>{nodes.length}</span>
          <button onClick={() => setShowCreate((v) => !v)} style={{ marginLeft: "auto", fontSize: 11, padding: "2px 8px", cursor: "pointer", background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 3 }}>
            + new
          </button>
        </div>
        {showCreate && (
          <div style={{ padding: 8, borderBottom: "1px solid var(--border)", display: "flex", gap: 4 }}>
            <input
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
              placeholder="name"
              style={{ flex: 1, padding: "4px 6px", fontSize: 12, border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)" }}
              autoFocus
            />
            <button onClick={() => create.mutate({ name: createName })} disabled={create.isPending || !createName.trim()} style={{ fontSize: 11, padding: "3px 8px", cursor: "pointer", background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 3 }}>
              {create.isPending ? "…" : "Create"}
            </button>
          </div>
        )}
        <div style={{ flex: 1, overflow: "auto" }}>
          {roots.length === 0 && <EmptyView>no documents yet</EmptyView>}
          {roots.map((n) => renderNode(n, 0))}
        </div>
      </div>

      {/* Right editor */}
      <div style={{ flex: 1, overflow: "hidden", display: "flex", flexDirection: "column" }}>
        {!selectedId && <EmptyView>select a document</EmptyView>}
        {selectedId && detail.isLoading && <EmptyView>loading…</EmptyView>}
        {selectedId && detail.isError && <EmptyView>error loading document</EmptyView>}
        {doc && (
          <>
            <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)", display: "flex", gap: 8, alignItems: "center" }}>
              <input
                defaultValue={doc.name}
                onChange={(e) => setEditPatch((p) => ({ ...p, name: e.target.value }))}
                style={{ fontSize: 14, fontWeight: 500, flex: 1, padding: "3px 6px", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)" }}
              />
              <button onClick={() => save.mutate({ id: doc.id, patch: editPatch })} disabled={save.isPending} style={{ fontSize: 12, padding: "3px 10px", cursor: "pointer", background: "var(--accent)", color: "var(--accent-fg)", border: "none", borderRadius: 3 }}>
                {save.isPending ? "saving…" : "Save"}
              </button>
              <button onClick={() => { if (confirm(`Delete "${doc.name}"?`)) del.mutate(doc.id); }} style={{ fontSize: 12, padding: "3px 10px", cursor: "pointer", border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-elev)", color: "var(--status-error)" }}>
                Delete
              </button>
            </div>
            <div style={{ padding: "4px 12px", borderBottom: "1px solid var(--border)", display: "flex", gap: 8, alignItems: "center" }}>
              <span className="muted" style={{ fontSize: 11 }}>description:</span>
              <input
                defaultValue={doc.description}
                onChange={(e) => setEditPatch((p) => ({ ...p, description: e.target.value }))}
                style={{ flex: 1, padding: "2px 6px", fontSize: 12, border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)" }}
                placeholder="description…"
              />
              <span className="muted" style={{ fontSize: 11 }}>tags:</span>
              <input
                defaultValue={doc.tags?.join(", ")}
                onChange={(e) => setEditPatch((p) => ({ ...p, tags: e.target.value.split(",").map((t) => t.trim()).filter(Boolean) }))}
                style={{ flex: 1, padding: "2px 6px", fontSize: 12, border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-paper)" }}
                placeholder="tag1, tag2…"
              />
            </div>
            <div style={{ flex: 1, overflow: "hidden" }}>
              <MonacoEditor
                value={doc.content}
                onChange={(v) => setEditPatch((p) => ({ ...p, content: v }))}
                language="markdown"
                height="100%"
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}
