// AskAiTrigger — "AI · 迭代" button shared by Function/Handler/Workflow
// detail headers. Opens a fixed bottom-right popover with a textarea
// and suggestion chips; submitting calls /<kind>s/{id}:iterate which
// returns a conversationId, and we jump to chat pane.
//
// Visual: matches boilerplate's `.ask-ai-btn` + `.ask-ai-pop` (fixed
// bottom-right, slide-in animation) per the §3.1 design tokens.
//
// AskAiTrigger —— forge 详情头部按钮；fixed 弹层 + suggestion chips；
// :iterate 返 conversationId → 跳 chat。

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { motion, AnimatePresence } from "framer-motion";
import { Icon } from "@shared/ui/Icon";
import { useForgeIterate } from "@features/forge-iterate";
import { navigate } from "@shared/lib/navigation";
import { slideUp } from "@shared/lib/motion";

interface AskAiTriggerProps {
  kind: string;
  entityId: string;
  context?: string;
  suggestions?: string[];
}

export function AskAiTrigger({ kind, entityId, context, suggestions = [] }: AskAiTriggerProps) {
  const { t } = useTranslation("misc");
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const ta = useRef(null);

  const { submit: iterateSubmit } = useForgeIterate();

  useEffect(() => {
    if (open) setTimeout(() => ta.current?.focus(), 50);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  const submit = async (prompt: string) => {
    setOpen(false);
    setText("");
    const cid = await iterateSubmit(kind, entityId, prompt);
    if (cid) {
      navigate.openConv(cid);
    }
  };

  return (
    <>
      <button
        className="btn btn-sm ask-ai-btn"
        onClick={() => setOpen((o) => !o)}
        title={t("askAi.btnTitle")}
      >
        <Icon.Sparkles /> {t("askAi.btnLabel")}
      </button>

      <AnimatePresence>
        {open && (
          <motion.div
            className="ask-ai-pop"
            {...(slideUp as any)}
            transition={{ duration: 0.18, ease: [0.2, 0.8, 0.2, 1] }}
          >
            <div className="ask-ai-pop-head">
              <div className="ask-ai-pop-context">
                <Icon.Sparkles style={{ width: 12, height: 12, color: "var(--accent)" }} />
                <span>{context || t("askAi.defaultContext")}</span>
              </div>
              <button className="icon-btn" onClick={() => setOpen(false)} title={t("askAi.closeTitle")}>
                <Icon.X />
              </button>
            </div>
            <textarea
              ref={ta}
              className="ask-ai-pop-input"
              placeholder={t("askAi.placeholder")}
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  submit(text.trim());
                }
              }}
              rows={3}
            />
            {suggestions.length > 0 && (
              <div className="ask-ai-pop-sugs">
                {suggestions.map((s, i) => (
                  <button key={i} className="ask-ai-pop-sug" onClick={() => submit(s)}>
                    {s}
                  </button>
                ))}
              </div>
            )}
          </motion.div>
        )}
      </AnimatePresence>
    </>
  );
}
