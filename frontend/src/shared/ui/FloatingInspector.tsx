import React, { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "./Icon.tsx";

// FloatingInspector — non-modal popover anchored to a pane corner.
// Click-outside and Esc both close it; the canvas underneath stays
// fully interactive (pointer events pass through the backdrop).
//
// FloatingInspector —— 非 modal 浮层；Esc / 点外面 / X 三种方式关；
// 不挡画布交互。
//
// Props:
//   open        boolean
//   onClose     () => void
//   title       string — header label
//   side        "right" | "bottom-right"   default "right" (top-right corner)
//   width       number   default 340
//   anchorRef   React ref to the container element this popover lives in;
//               click-outside fires only for clicks inside that container,
//               so toolbar / sidebar clicks in other panes don't dismiss.

interface FloatingInspectorProps {
  open: boolean;
  onClose?: () => void;
  title?: React.ReactNode;
  children?: React.ReactNode;
  side?: "right" | "bottom-right";
  width?: number;
  anchorRef?: React.RefObject<Element>;
}

export function FloatingInspector({
  open, onClose, title, children,
  side = "right", width = 340, anchorRef,
}: FloatingInspectorProps) {
  const { t } = useTranslation("misc");
  const popRef = useRef(null);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose?.(); };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  useEffect(() => {
    if (!open || !anchorRef?.current) return;
    const container = anchorRef.current;
    const onPointerDown = (e: Event) => {
      if (!popRef.current) return;
      if (popRef.current.contains(e.target as Node)) return;
      onClose?.();
    };
    container.addEventListener("pointerdown", onPointerDown);
    return () => container.removeEventListener("pointerdown", onPointerDown);
  }, [open, onClose, anchorRef]);

  if (!open) return null;

  return (
    <div
      ref={popRef}
      className={"floating-inspector floating-inspector-" + side}
      style={{ width }}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <div className="floating-inspector-head">
        <span className="floating-inspector-title">{title}</span>
        <button className="icon-btn" onClick={onClose} title={t("floatingInspector.closeTitle")} aria-label={t("floatingInspector.closeAria")}>
          <Icon.X />
        </button>
      </div>
      <div className="floating-inspector-body">{children}</div>
    </div>
  );
}
