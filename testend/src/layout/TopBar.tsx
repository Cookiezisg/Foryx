import { useEffect, useState } from "react";
import { useUIStore } from "@/stores/ui";
import { useUsersStore } from "@/stores/users";
import { status as sseStatus } from "@/api/sse";

function useSSEStatuses() {
  const [, setTick] = useState(0);
  useEffect(() => {
    const i = setInterval(() => setTick((x) => x + 1), 1000);
    return () => clearInterval(i);
  }, []);
  return {
    el: sseStatus("eventlog"),
    nf: sseStatus("notifications"),
    fg: sseStatus("forge"),
  };
}

function Pill({ ok, label }: { ok: boolean; label: string }) {
  return <span className={`pill ${ok ? "success" : "error"}`}>{label}</span>;
}

export function TopBar() {
  const { expanded, setExpanded, openPalette } = useUIStore();
  const { list, activeId, setActive } = useUsersStore();
  const sse = useSSEStatuses();
  const active = list.find((u) => u.id === activeId);

  return (
    <div style={{
      height: 36, borderBottom: "1px solid var(--border)", padding: "0 12px",
      display: "flex", alignItems: "center", gap: 12, fontSize: 12,
    }}>
      <strong>Forgify Dev Console V3</strong>
      <span className="muted">/dev/</span>
      <span style={{ flex: 1 }} />
      <Pill ok={sse.el.connected} label="EL" />
      <Pill ok={sse.nf.connected} label="NF" />
      <Pill ok={sse.fg.connected} label="FG" />
      <select value={activeId ?? ""} onChange={(e) => setActive(e.target.value)} style={{
        background: "var(--bg-elev)", color: "var(--fg-body)",
        border: "1px solid var(--border)", borderRadius: 4, padding: "2px 6px", fontSize: 12,
      }}>
        {!active && <option value="">(no user)</option>}
        {list.map((u) => <option key={u.id} value={u.id}>{u.displayName || u.username}</option>)}
      </select>
      <button onClick={openPalette} className="muted" style={{
        background: "none", border: "none", cursor: "pointer", fontSize: 12,
      }}>
        <kbd style={{
          fontFamily: "var(--mono)", fontSize: 11, padding: "1px 5px",
          border: "1px solid var(--border)", borderRadius: 3, background: "var(--bg-elev)",
        }}>⌘K</kbd>
      </button>
      <button onClick={() => setExpanded(!expanded)} className="muted" style={{
        background: "none", border: "1px solid var(--border)",
        borderRadius: 4, cursor: "pointer", padding: "2px 8px", fontSize: 12,
      }}>
        {expanded ? "← shrink" : "expand →"}
      </button>
    </div>
  );
}
