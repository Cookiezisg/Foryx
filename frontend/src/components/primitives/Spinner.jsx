// Spinner — CSS-rotated ring. The `.spinner` rule already exists in the
// boilerplate (keyframes spin + 1.5px border). Use size 12 or 16.

export function Spinner({ size = 12, color = "currentColor", className = "" }) {
  const style = { width: size, height: size, borderTopColor: color };
  return <span className={`spinner ${className}`} style={style} />;
}
