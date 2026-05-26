import React, { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "./Icon.tsx";

// BottomSheet — full-width panel sliding up from the bottom of an
// anchor container. Built for wide content (logs, step output) that
// reads better horizontally than the right-rail format.
//
// BottomSheet —— 从底部升起的全宽面板；适合横向更舒服的内容
// （日志、step output）。
//
// Same anchorRef contract as FloatingInspector for click-outside.

interface BottomSheetProps {
  open: boolean;
  onClose?: () => void;
  title?: React.ReactNode;
  children?: React.ReactNode;
  height?: number;
  anchorRef?: React.RefObject<Element>;
}

export function BottomSheet({
  open, onClose, title, children, height = 340, anchorRef,
}: BottomSheetProps) {
  const { t } = useTranslation("misc");
  const sheetRef = useRef(null);

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
      if (!sheetRef.current) return;
      if (sheetRef.current.contains(e.target as Node)) return;
      onClose?.();
    };
    container.addEventListener("pointerdown", onPointerDown);
    return () => container.removeEventListener("pointerdown", onPointerDown);
  }, [open, onClose, anchorRef]);

  if (!open) return null;

  return (
    <div
      ref={sheetRef}
      className="bottom-sheet"
      style={{ height }}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <div className="bottom-sheet-head">
        <span className="bottom-sheet-title">{title}</span>
        <button className="icon-btn" onClick={onClose} title={t("bottomSheet.closeTitle")} aria-label={t("bottomSheet.closeAria")}>
          <Icon.X />
        </button>
      </div>
      <div className="bottom-sheet-body">{children}</div>
    </div>
  );
}
