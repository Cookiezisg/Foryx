// ToastTray — bottom-right stack. Toasts animate in (slide up + fade)
// and out via Framer Motion AnimatePresence + layout transitions.
//
// ToastTray —— 右下角；Framer Motion AnimatePresence + layout 动画。

import { useTranslation } from "react-i18next";
import { AnimatePresence, motion } from "framer-motion";
import { Icon } from "../primitives/Icon.jsx";
import { useUIStore } from "../../store/ui.js";

export function ToastTray() {
  const { t } = useTranslation("toast");
  const toasts = useUIStore((s) => s.toasts);
  const dismiss = useUIStore((s) => s.dismissToast);

  return (
    <div className="toast-tray">
      <AnimatePresence initial={false}>
        {toasts.map((toast) => (
          <motion.div
            key={toast.id}
            layout
            initial={{ opacity: 0, y: 16, scale: 0.97 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 4, scale: 0.96 }}
            transition={{ duration: 0.22, ease: [0.2, 0.8, 0.2, 1] }}
            className={"toast" + (toast.kind ? " is-" + toast.kind : "")}
          >
            <div className="toast-icon">
              {toast.kind === "error" ? <Icon.AlertCircle />
               : toast.kind === "warn" ? <Icon.AlertCircle />
               : <Icon.CheckCircle />}
            </div>
            <div className="toast-body">
              {toast.title && <div className="toast-title">{toast.title}</div>}
              {toast.desc && <div className="toast-desc">{toast.desc}</div>}
            </div>
            {toast.undo && (
              <button className="btn btn-xs btn-ghost" onClick={() => { toast.undo(); dismiss(toast.id); }}>
                <Icon.Refresh /> {t("notifications.undoLabel")}
              </button>
            )}
            <button className="icon-btn" onClick={() => dismiss(toast.id)}><Icon.X /></button>
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}
