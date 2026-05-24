// RunDrawer — input form for function :run, handler :call, workflow :trigger.
// Single component handles all three kinds so the UX stays consistent and
// every invoke surface (forge list, detail page) can open it the same way.
//
// RunDrawer —— function/handler/workflow 三种触发入口的统一表单 drawer。

import { useEffect, useMemo, useRef, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Icon } from "../primitives/Icon.jsx";
import { Button } from "../primitives/Button.jsx";
import { useRunFunction, useCallHandler, useRunWorkflow } from "../../api/forge.js";
import { useUIStore } from "../../store/ui.js";
import { slideRight, scrim } from "../../motion/tokens.js";

function safeParse(text) {
  const t = text.trim();
  if (!t) return [{}, null];
  try { return [JSON.parse(t), null]; }
  catch (e) { return [null, e.message]; }
}

export function RunDrawer({ open, onClose, kind, entity }) {
  const run = useRunFunction();
  const call = useCallHandler();
  const trig = useRunWorkflow();
  const pushToast = useUIStore((s) => s.pushToast);
  const setActiveConv = useUIStore((s) => s.setActiveConv);
  const openPane = useUIStore((s) => s.openPane);

  const [body, setBody] = useState("{\n  \n}");
  const [method, setMethod] = useState("");
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);
  const ta = useRef(null);

  useEffect(() => {
    if (!open) return;
    setResult(null); setError(null);
    setBody("{\n  \n}");
    if (kind === "handler") {
      const methods = entity?.methods || entity?.currentVersion?.methods || [];
      setMethod(methods[0]?.name || "");
    }
    setTimeout(() => ta.current?.focus(), 80);
  }, [open, kind, entity?.id]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  const submit = async () => {
    const [parsed, perr] = safeParse(body);
    if (perr) { setError("JSON 不对 · " + perr); return; }
    setError(null); setResult(null);
    try {
      let res;
      if (kind === "function") {
        res = await run.mutateAsync({ id: entity.id, inputs: parsed });
      } else if (kind === "handler") {
        if (!method) { setError("挑一个方法"); return; }
        res = await call.mutateAsync({ id: entity.id, method, args: parsed });
      } else if (kind === "workflow") {
        res = await trig.mutateAsync({ id: entity.id, input: parsed });
        const runId = res?.flowRunId || res?.id || res?.runId;
        pushToast({ kind: "success", title: "已触发", desc: runId || "flowrun 创建" });
        if (runId) {
          openPane("execute");
          useUIStore.getState().setActiveFlowRun?.(runId);
        }
      }
      setResult(res);
    } catch (e) {
      setError(e.message || String(e));
    }
  };

  const busy = run.isPending || call.isPending || trig.isPending;
  const methods = kind === "handler"
    ? (entity?.methods || entity?.currentVersion?.methods || [])
    : [];

  const title =
    kind === "function" ? "跑 function" :
    kind === "handler"  ? "调 handler" :
                          "触发 workflow";

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div className="overlay-scrim" {...scrim} onClick={onClose} />
          <motion.aside
            className="drawer drawer-right run-drawer"
            {...slideRight}
            onClick={(e) => e.stopPropagation()}
          >
            <header className="drawer-head">
              <div className="drawer-title">
                <Icon.Play /> {title}
              </div>
              <button className="icon-btn" onClick={onClose} title="关闭"><Icon.X /></button>
            </header>

            <div className="drawer-body" style={{ display: "flex", flexDirection: "column", gap: 14 }}>
              <div style={{ fontSize: 12, color: "var(--fg-muted)" }}>
                <span style={{ fontFamily: "var(--font-mono)", color: "var(--accent)" }}>{entity?.id}</span>
                {entity?.name && <> · {entity.name}</>}
              </div>

              {kind === "handler" && (
                <div>
                  <label className="drawer-label">方法</label>
                  {methods.length === 0 ? (
                    <div className="empty" style={{ padding: 12 }}>
                      <div className="sub">当前版本没有方法</div>
                    </div>
                  ) : (
                    <select
                      className="cfg-input"
                      style={{ fontFamily: "var(--font-mono)" }}
                      value={method}
                      onChange={(e) => setMethod(e.target.value)}
                      aria-label="方法"
                    >
                      {methods.map((m) => (
                        <option key={m.name} value={m.name}>
                          {m.name}{m.sig || m.signature ? " " + (m.sig || m.signature) : ""}
                        </option>
                      ))}
                    </select>
                  )}
                </div>
              )}

              <div>
                <label className="drawer-label">
                  {kind === "function" ? "inputs (JSON)" : kind === "handler" ? "args (JSON)" : "input (JSON)"}
                </label>
                <textarea
                  ref={ta}
                  className="run-drawer-input"
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                  spellCheck={false}
                  rows={10}
                />
                {error && <div className="run-drawer-error">{error}</div>}
              </div>

              {result != null && (
                <div>
                  <label className="drawer-label">结果</label>
                  <pre className="code-block run-drawer-result">{JSON.stringify(result, null, 2)}</pre>
                </div>
              )}
            </div>

            <footer className="drawer-foot">
              <span style={{ fontSize: 11, color: "var(--fg-faint)" }}>
                Esc 关闭 · Cmd+Enter 提交
              </span>
              <div style={{ flex: 1 }} />
              <Button size="sm" variant="ghost" onClick={onClose}>取消</Button>
              <Button size="sm" variant="accent" onClick={submit} disabled={busy}>
                {busy ? <><span className="spinner" /> 在跑</> : <><Icon.Play /> 提交</>}
              </Button>
            </footer>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
}
