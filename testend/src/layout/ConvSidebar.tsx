import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { useConvStore } from "@/stores/conv";
import { useUIStore } from "@/stores/ui";
import type { Conversation } from "@frontend/entities/conversation/model/types";

function RelTime({ ts }: { ts: string | undefined }) {
  if (!ts) return <span className="muted">—</span>;
  const d = new Date(ts).getTime();
  if (!Number.isFinite(d)) return <span className="muted">—</span>;
  const diff = (Date.now() - d) / 1000;
  let s = "刚刚";
  if (diff > 60 && diff < 3600) s = `${Math.floor(diff / 60)}分钟前`;
  else if (diff < 86400) s = `${Math.floor(diff / 3600)}小时前`;
  else if (diff < 86400 * 30) s = `${Math.floor(diff / 86400)}天前`;
  else s = new Date(d).toLocaleDateString();
  return <span title={new Date(d).toLocaleString()}>{s}</span>;
}

export function ConvSidebar() {
  const { activeId, filter, setActive, setFilter, showArchived, setShowArchived } = useConvStore();
  const ui = useUIStore();
  const qc = useQueryClient();
  const { data: convs = [], isError } = useQuery({
    queryKey: qk.conversations({ archived: showArchived }),
    queryFn: () => getJSON<Conversation[]>(`/api/v1/conversations${showArchived ? "?archived=true" : ""}`),
  });
  const create = useMutation({
    mutationFn: () => postJSON<Conversation>("/api/v1/conversations", { title: "(new)" }),
    onSuccess: (c) => { qc.invalidateQueries({ queryKey: qk.conversations() }); setActive(c.id); },
  });

  const filtered = convs.filter((c) => !filter || c.title.toLowerCase().includes(filter.toLowerCase()));

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%", background: "var(--bg-sidebar)" }}>
      <div style={{ padding: 8, display: "flex", gap: 6 }}>
        <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="filter…" style={{
          flex: 1, padding: "4px 8px", border: "1px solid var(--border)",
          borderRadius: 3, background: "var(--bg-paper)", color: "var(--fg-body)", fontSize: 12,
        }} />
        <button onClick={() => create.mutate()} style={{
          padding: "4px 8px", background: "var(--accent)", color: "var(--accent-fg)",
          border: "none", borderRadius: 3, cursor: "pointer", fontSize: 12,
        }}>+</button>
      </div>
      <label style={{ display: "flex", alignItems: "center", gap: 4, padding: "0 8px 6px", fontSize: 11, color: "var(--fg-muted)" }}>
        <input type="checkbox" checked={showArchived} onChange={(e) => setShowArchived(e.target.checked)} /> archived
      </label>
      <div style={{ flex: 1, overflowY: "auto" }}>
        {isError && <div className="empty">load error</div>}
        {!isError && filtered.length === 0 && <div className="empty">no conversations</div>}
        {filtered.map((c) => (
          <div key={c.id} onClick={() => setActive(c.id)}
            onContextMenu={(e) => { e.preventDefault(); ui.showRaw(c.title, c); }}
            style={{
              padding: "6px 10px", cursor: "pointer",
              borderLeft: "2px solid transparent",
              borderLeftColor: activeId === c.id ? "var(--accent)" : "transparent",
              background: activeId === c.id ? "var(--bg-elev)" : undefined,
              fontSize: 12,
            }}>
            <div style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              {c.title || "(untitled)"}
            </div>
            <div className="muted" style={{ fontSize: 10 }}><RelTime ts={c.updatedAt} /></div>
          </div>
        ))}
      </div>
    </div>
  );
}
