import { useUsersStore } from "@/stores/users";

export function UserPicker() {
  const { list, setActive } = useUsersStore();
  if (list.length === 0) {
    return (
      <div style={{
        position: "fixed", inset: 0, background: "rgba(0,0,0,0.4)",
        display: "flex", alignItems: "center", justifyContent: "center", zIndex: 90,
      }}>
        <div style={{
          background: "var(--bg-paper)", padding: 20, borderRadius: 8,
          border: "1px solid var(--border)", maxWidth: 400,
        }}>
          <h3 style={{ margin: "0 0 8px" }}>No user yet</h3>
          <p className="muted" style={{ margin: 0 }}>
            Backend has no users; create one via the main frontend's onboarding flow, then refresh testend.
          </p>
        </div>
      </div>
    );
  }
  return (
    <div style={{
      position: "fixed", inset: 0, background: "rgba(0,0,0,0.4)",
      display: "flex", alignItems: "center", justifyContent: "center", zIndex: 90,
    }}>
      <div style={{
        background: "var(--bg-paper)", padding: 20, borderRadius: 8,
        border: "1px solid var(--border)", minWidth: 320,
      }}>
        <h3 style={{ margin: "0 0 12px" }}>Pick a profile</h3>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {list.map((u) => (
            <button key={u.id} onClick={() => setActive(u.id)} style={{
              padding: "8px 12px", background: "var(--bg-elev)",
              border: "1px solid var(--border)", borderRadius: 4, cursor: "pointer", textAlign: "left",
            }}>
              <strong>{u.displayName || u.username}</strong>
              <span className="muted mono" style={{ marginLeft: 8, fontSize: 11 }}>{u.id}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
