// ActionMenu — dropdown menu via floating-ui (auto-positioned, portal'd).
// items: [{ label, icon?, danger?, shortcut?, onClick } | "divider"]
//
// Trigger renders as an icon-btn with MoreHorizontal by default; pass a
// custom `renderTrigger` to override. The popover is rendered in a portal
// at the document root and dismisses on outside click + Escape.
//
// ActionMenu —— 用 floating-ui 自动定位 + portal 的下拉菜单；外部点击 /
// Escape 关闭。

import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  useFloating, autoUpdate, offset, flip, shift,
  useDismiss, useClick, useInteractions, useRole, FloatingPortal,
  type Placement,
} from "@floating-ui/react";
import { Icon } from "@shared/ui/Icon";

interface ActionMenuItem {
  label: string;
  icon?: React.ComponentType<{ className?: string; size?: number }>;
  danger?: boolean;
  shortcut?: string;
  onClick?: () => void;
}

interface ActionMenuProps {
  items: (ActionMenuItem | "divider")[];
  renderTrigger?: (props: React.HTMLAttributes<HTMLElement> & { ref?: any }) => React.ReactNode; // floating-ui ref callback is untyped
  placement?: Placement;
}

export function ActionMenu({ items, renderTrigger, placement = "bottom-end" }: ActionMenuProps) {
  const { t } = useTranslation("misc");
  const [open, setOpen] = useState(false);

  const { refs, floatingStyles, context } = useFloating({
    open,
    onOpenChange: setOpen,
    placement,
    middleware: [offset(6), flip({ padding: 8 }), shift({ padding: 8 })],
    whileElementsMounted: autoUpdate,
  });

  const click = useClick(context);
  const dismiss = useDismiss(context);
  const role = useRole(context, { role: "menu" });
  const { getReferenceProps, getFloatingProps } = useInteractions([click, dismiss, role]);

  const trigger = renderTrigger ? (
    renderTrigger({ ref: refs.setReference, ...getReferenceProps() })
  ) : (
    <button
      ref={refs.setReference}
      className="icon-btn"
      title={t("actionMenu.more")}
      {...getReferenceProps()}
    >
      <Icon.MoreHorizontal />
    </button>
  );

  return (
    <>
      {trigger}
      {open && (
        <FloatingPortal>
          {/* Outer = positioning only (Floating UI owns its transform). Inner
              = the visible menu + entrance animation. Splitting them stops the
              keyframe transform from overriding the positioning transform,
              which made the menu animate at (0,0) then snap into place. */}
          <div
            ref={refs.setFloating}
            style={floatingStyles}
            className="action-menu-pos"
            {...getFloatingProps()}
          >
            <div className="action-menu">
              {items.map((it, i) => {
                if (it === "divider") return <div key={i} className="action-menu-divider" />;
                const IconC = it.icon;
                return (
                  <button
                    key={i}
                    className={"action-menu-item" + (it.danger ? " is-danger" : "")}
                    onClick={() => {
                      setOpen(false);
                      it.onClick?.();
                    }}
                  >
                    {IconC && <IconC className="icon" />}
                    <span className="label">{it.label}</span>
                    {it.shortcut && <kbd>{it.shortcut}</kbd>}
                  </button>
                );
              })}
            </div>
          </div>
        </FloatingPortal>
      )}
    </>
  );
}
