// Badge — boilerplate `.badge` with kind + optional dot.
// kind: success | error | warn | info | streaming | muted | (none = neutral)
//
// streaming kind ships a pulse-dot (CSS @keyframes pulse-dot) — drives the
// "agent is working" surface.

export function Badge({ kind, dot = true, children, className = "", ...rest }) {
  const cls = ["badge", kind, className].filter(Boolean).join(" ");
  return (
    <span className={cls} {...rest}>
      {dot && kind && kind !== "muted" && <span className="dot" />}
      {children}
    </span>
  );
}
