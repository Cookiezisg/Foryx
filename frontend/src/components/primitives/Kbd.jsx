// Kbd — keyboard shortcut chip. boilerplate `kbd` element is already styled
// in base.css; this is just a convenience React wrapper.

export function Kbd({ children, className = "" }) {
  return <kbd className={className}>{children}</kbd>;
}
