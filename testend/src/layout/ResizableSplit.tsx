import { useEffect, useRef, useState, type ReactNode } from "react";

export function ResizableSplit({
  leftWidth, minLeft = 100, maxLeft = 1000, onResize,
  left, right,
}: {
  leftWidth: number;
  minLeft?: number;
  maxLeft?: number;
  onResize: (w: number) => void;
  left: ReactNode;
  right: ReactNode;
}) {
  const [dragging, setDragging] = useState(false);
  const dragStartX = useRef(0);
  const dragStartWidth = useRef(leftWidth);

  useEffect(() => {
    if (!dragging) return;
    const onMove = (e: MouseEvent) => {
      const dx = e.clientX - dragStartX.current;
      const w = Math.max(minLeft, Math.min(maxLeft, dragStartWidth.current + dx));
      onResize(w);
    };
    const onUp = () => setDragging(false);
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
  }, [dragging, minLeft, maxLeft, onResize]);

  return (
    <div style={{ display: "flex", flex: 1, minWidth: 0, height: "100%" }}>
      <div style={{ width: leftWidth, flexShrink: 0, overflow: "hidden" }}>{left}</div>
      <div
        onMouseDown={(e) => { dragStartX.current = e.clientX; dragStartWidth.current = leftWidth; setDragging(true); }}
        style={{ width: 4, cursor: "col-resize", background: dragging ? "var(--accent)" : "var(--border)", flexShrink: 0 }}
      />
      <div style={{ flex: 1, minWidth: 0, overflow: "hidden" }}>{right}</div>
    </div>
  );
}
