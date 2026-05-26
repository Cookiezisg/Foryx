// AskUserModal — surfaces backend AskUserQuestion tool calls. Opens
// automatically when pendingAsk is set (set by the notifications SSE
// dispatch for type="ask"). Submit POSTs the answer to the
// pending-questions :resolve endpoint.
//
// AskUserModal —— 后端 AskUserQuestion 工具触发；提交走 :resolve 端点。
// open/pending/onClose 由 AppShell 经 props 传入，零 app 依赖。

import { useEffect, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { AnimatePresence, motion } from "framer-motion";
import { Icon } from "@/components/primitives/Icon.jsx";
import { Button } from "@/components/primitives/Button.jsx";
import { useAskUserAnswer } from "@features/ask-user";
import { scaleIn, fadeIn } from "@/motion/tokens.js";

export function AskUserModal({ pending, askOpen, onClose }) {
  const { t } = useTranslation(["conv", "common"]);

  const isOpen = askOpen || !!pending;

  const { submitting, submit } = useAskUserAnswer({ pending, onClose });

  const [selected, setSelected] = useState(null);

  useEffect(() => { setSelected(null); }, [pending?.id]);

  useEffect(() => {
    if (!isOpen) return;
    const onKey = (e) => {
      if (e.key === "Escape") { onClose(); return; }
      const n = parseInt(e.key, 10);
      if (n >= 1 && n <= 9 && pending?.options?.[n - 1]) {
        setSelected(pending.options[n - 1].id || pending.options[n - 1].value);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [isOpen, pending, onClose]);

  if (!pending) {
    // No pending question — show "no questions" empty state if manually opened
    return (
      <AnimatePresence>
        {askOpen && (
          <motion.div className="overlay" {...fadeIn} onClick={onClose}>
            <motion.div className="ask-card" {...scaleIn} onClick={(e) => e.stopPropagation()}>
              <div className="ask-head">
                <div className="icon-wrap"><Icon.HelpCircle /></div>
                <div className="meta">
                  <div className="label">{t("ask.emptyLabel")}</div>
                  <div className="title">{t("ask.emptyTitle")}</div>
                </div>
                <button className="icon-btn" onClick={onClose} style={{ marginLeft: "auto" }} title={t("common:close")}>
                  <Icon.X />
                </button>
              </div>
              <div className="ask-body">
                <div className="ask-question">
                  {t("ask.emptyBody")}
                </div>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    );
  }

  const options = pending.options || [];

  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div className="overlay" {...fadeIn} onClick={onClose}>
          <motion.div className="ask-card" {...scaleIn} onClick={(e) => e.stopPropagation()}>
            <div className="ask-head">
              <div className="icon-wrap"><Icon.HelpCircle /></div>
              <div className="meta">
                <div className="label">{t("ask.waitingLabel")}</div>
                <div className="title">{pending.question || t("ask.defaultQuestion")}</div>
              </div>
              <button className="icon-btn" onClick={onClose} style={{ marginLeft: "auto" }}>
                <Icon.X />
              </button>
            </div>
            <div className="ask-body">
              {pending.context && <div className="ask-question">{pending.context}</div>}
              <div className="ask-options">
                {options.length === 0 && (
                  <div style={{ padding: 16, color: "var(--fg-faint)", fontSize: 12 }}>
                    {t("ask.noOptionsHint")}
                  </div>
                )}
                {options.map((o, i) => (
                  <div
                    key={o.id || i}
                    className={"ask-option" + (selected === (o.id || o.value) ? " is-selected" : "")}
                    onClick={() => setSelected(o.id || o.value)}
                  >
                    <div className="key">{i + 1}</div>
                    <div className="text">{o.text || o.label}<span className="sub">{o.sub || ""}</span></div>
                    <Icon.Check className="check" />
                  </div>
                ))}
              </div>
            </div>
            <div className="ask-footer">
              <div className="hint">
                <Trans i18nKey="ask.footerHint" ns="conv">
                  <Icon.CornerDownLeft style={{ width: 11, height: 11 }} />
                </Trans>
              </div>
              <div className="actions">
                <Button size="sm" variant="ghost" onClick={onClose}>{t("ask.laterBtn")}</Button>
                <Button size="sm" variant="accent" disabled={!selected || submitting} loading={submitting} onClick={() => submit(selected)}>
                  <Icon.Check /> {t("ask.submitBtn")}
                </Button>
              </div>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
