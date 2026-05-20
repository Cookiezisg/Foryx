// PaneResize — vertical drag handle between two panes. While dragging,
// listens window-level mousemove/mouseup and pushes pointer-events:none
// on body to prevent iframes/canvas swallowing events.
//
// PaneResize —— 双 pane 中间垂直拖动条。拖动时在 window 监听 mouse；body
// 加 pointer-events:none 防 iframe/canvas 抢事件。

import { useEffect, useState } from "react";

export function PaneResize({ onDrag }) {
  const [dragging, setDragging] = useState(false);

  useEffect(() => {
    if (!dragging) return;
    const onMove = (e) => onDrag(e.clientX);
    const onUp = () => setDragging(false);
    document.body.style.userSelect = "none";
    document.body.style.cursor = "col-resize";
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
      document.body.style.userSelect = "";
      document.body.style.cursor = "";
    };
  }, [dragging, onDrag]);

  // height:100% — boilerplate places PaneResize as a flex sibling between
  // two pane-wraps so it stretches naturally. We render it inside an
  // absolutely-positioned wrapper instead (PaneResizeBetween in
  // AppShell), so we explicit-height it to fill the wrapper's top:0+bot:0.
  //
  // height:100%——boilerplate 把 PaneResize 作为两个 pane-wrap 的 flex 兄弟
  // 自然撑满；我们放在 absolute 包装里需要显式高度才能撑开。
  return (
    <div
      className={"pane-resize" + (dragging ? " is-dragging" : "")}
      onMouseDown={() => setDragging(true)}
      role="separator"
      aria-orientation="vertical"
      style={{ height: "100%" }}
    />
  );
}
