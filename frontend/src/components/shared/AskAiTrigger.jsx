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
import { motion, AnimatePresence } from "framer-motion";
import { Icon } from "../primitives/Icon.jsx";
import { useIterateForge } from "../../api/forge.js";
import { useUIStore } from "../../store/ui.js";
import { slideUp } from "../../motion/tokens.js";

export function AskAiTrigger({ kind, entityId, context, suggestions = [] }) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const ta = useRef(null);

  const iterate = useIterateForge();
  const pushToast = useUIStore((s) => s.pushToast);
  const setActiveConv = useUIStore((s) => s.setActiveConv);
  const openPane = useUIStore((s) => s.openPane);

  useEffect(() => {
    if (open) setTimeout(() => ta.current?.focus(), 50);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e) => { if (e.key === "Escape") setOpen(false); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  const submit = async (prompt) => {
    setOpen(false);
    setText("");
    try {
      const res = await iterate.mutateAsync({ kind, id: entityId, prompt });
      const cid = res?.conversationId || res?.id;
      if (cid) {
        setActiveConv(cid);
        openPane("chat");
      } else {
        pushToast({ kind: "warn", title: "iterate 返回为空", desc: "无法跳转到对话" });
      }
    } catch (err) {
      pushToast({ kind: "error", title: "iterate 失败", desc: err.message });
    }
  };

  return (
    <>
      <button
        className="btn btn-sm ask-ai-btn"
        onClick={() => setOpen((o) => !o)}
        title="让 AI 迭代这个实体"
      >
        <Icon.Sparkles /> AI · 迭代
      </button>

      <AnimatePresence>
        {open && (
          <motion.div
            className="ask-ai-pop"
            {...slideUp}
            transition={{ duration: 0.18, ease: [0.2, 0.8, 0.2, 1] }}
          >
            <div className="ask-ai-pop-head">
              <div className="ask-ai-pop-context">
                <Icon.Sparkles style={{ width: 12, height: 12, color: "var(--accent)" }} />
                <span>{context || "迭代这个实体"}</span>
              </div>
              <button className="icon-btn" onClick={() => setOpen(false)} title="关闭">
                <Icon.X />
              </button>
            </div>
            <textarea
              ref={ta}
              className="ask-ai-pop-input"
              placeholder="告诉 AI 你想改什么……（Enter 提交，Shift+Enter 换行）"
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
