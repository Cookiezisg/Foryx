// CommandPalette (⌘K) — fuzzy search across panes / conversations /
// forge entities. Keyboard-first: arrow up/down navigate, Enter selects,
// Esc closes. Mouse hover updates active index for smoother feel.
//
// CommandPalette —— ⌘K 调起的命令板；箭头键导航 / Enter 选 / Esc 关。

import { useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { Icon } from "../primitives/Icon.jsx";
import { Kbd } from "../primitives/Kbd.jsx";
import { useUIStore } from "../../store/ui.js";
import { useConversations } from "../../api/conversations.js";
import { useFunctions, useHandlers, useWorkflows } from "../../api/forge.js";
import { useFlowRuns } from "../../api/flowruns.js";
import { scaleIn, fadeIn } from "../../motion/tokens.js";

const NAV_ITEMS = [
  { group: "导航", icon: Icon.MessageSquare, label: "对话",   desc: "",   target: "chat" },
  { group: "导航", icon: Icon.Hammer,        label: "锻造",   desc: "function / handler / workflow", target: "forge" },
  { group: "导航", icon: Icon.Play,          label: "执行",   desc: "flowrun · trigger", target: "execute" },
  { group: "导航", icon: Icon.FileText,      label: "文档",   desc: "", target: "documents" },
  { group: "导航", icon: Icon.Sparkles,      label: "Skills", desc: "",  target: "skills" },
  { group: "导航", icon: Icon.Server,        label: "MCP",    desc: "", target: "mcp" },
  { group: "导航", icon: Icon.Brain,         label: "Memory", desc: "", target: "memory" },
  { group: "导航", icon: Icon.Settings,      label: "设置",   desc: "", target: "config" },
];

export function CommandPalette() {
  const open = useUIStore((s) => s.cmdkOpen);
  const setOpen = useUIStore((s) => s.setCmdkOpen);
  const openPane = useUIStore((s) => s.openPane);
  const openEntity = useUIStore((s) => s.openEntity);
  const setActiveConv = useUIStore((s) => s.setActiveConv);

  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);

  const { data: conversations = [] } = useConversations();
  const { data: functions = [] } = useFunctions();
  const { data: handlers = [] } = useHandlers();
  const { data: workflows = [] } = useWorkflows();
  const { data: flowruns = [] } = useFlowRuns();

  const items = useMemo(() => {
    const a = [];
    for (const nav of NAV_ITEMS) {
      a.push({ ...nav, action: () => openPane(nav.target) });
    }
    for (const c of conversations.slice(0, 6)) {
      a.push({
        group: "对话",
        icon: Icon.MessageSquare,
        label: c.title || "(未命名)",
        desc: c.id,
        action: () => { setActiveConv(c.id); openPane("chat"); },
      });
    }
    for (const f of functions.slice(0, 5)) {
      a.push({
        group: "Function",
        icon: Icon.Code,
        label: f.name,
        desc: f.desc || f.description || f.id,
        action: () => openEntity("forge", f.id),
      });
    }
    for (const h of handlers.slice(0, 5)) {
      a.push({
        group: "Handler",
        icon: Icon.Server,
        label: h.name,
        desc: h.desc || h.description || h.id,
        action: () => openEntity("forge", h.id),
      });
    }
    for (const w of workflows.slice(0, 5)) {
      a.push({
        group: "Workflow",
        icon: Icon.Workflow,
        label: w.name,
        desc: w.desc || w.description || w.id,
        action: () => openEntity("forge", w.id),
      });
    }
    for (const f of flowruns.slice(0, 5)) {
      a.push({
        group: "FlowRun",
        icon: Icon.Play,
        label: f.workflow || f.workflowId,
        desc: f.id,
        action: () => openEntity("execute", f.id),
      });
    }
    return a;
  }, [conversations, functions, handlers, workflows, flowruns, openPane, openEntity, setActiveConv]);

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return items;
    return items.filter((i) => (i.label + " " + (i.desc || "")).toLowerCase().includes(s));
  }, [q, items]);

  const groups = useMemo(() => {
    const m = new Map();
    filtered.forEach((it) => {
      if (!m.has(it.group)) m.set(it.group, []);
      m.get(it.group).push(it);
    });
    return [...m.entries()];
  }, [filtered]);

  useEffect(() => {
    if (!open) return;
    setQ("");
    setActive(0);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e) => {
      if (e.key === "Escape") { setOpen(false); return; }
      if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(a + 1, filtered.length - 1)); }
      if (e.key === "ArrowUp")   { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
      if (e.key === "Enter") {
        const it = filtered[active];
        if (it) { it.action?.(); setOpen(false); }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, filtered, active, setOpen]);

  let cursor = 0;

  return (
    <AnimatePresence>
      {open && (
        <motion.div className="overlay" {...fadeIn} onClick={() => setOpen(false)}>
          <motion.div className="cmdk" {...scaleIn} onClick={(e) => e.stopPropagation()}>
            <div className="cmdk-input-wrap">
              <Icon.Search className="icon" />
              <input
                className="cmdk-input"
                autoFocus
                placeholder="找点什么"
                value={q}
                onChange={(e) => { setQ(e.target.value); setActive(0); }}
              />
              <Kbd>ESC</Kbd>
            </div>
            <div className="cmdk-list">
              {groups.length === 0 && (
                <div style={{ padding: "32px 16px", textAlign: "center", color: "var(--fg-faint)", fontSize: 13 }}>
                  没有匹配。
                </div>
              )}
              {groups.map(([gname, gitems]) => (
                <div key={gname}>
                  <div className="cmdk-group-label">{gname}</div>
                  {gitems.map((it) => {
                    const idx = cursor++;
                    const IconC = it.icon || Icon.ChevronRight;
                    return (
                      <div
                        key={idx}
                        className={"cmdk-row" + (idx === active ? " is-active" : "")}
                        onMouseEnter={() => setActive(idx)}
                        onClick={() => { it.action?.(); setOpen(false); }}
                      >
                        <div className="icon-wrap"><IconC /></div>
                        <div className="label">
                          <span>{it.label}</span>
                          {it.desc && <span className="desc">— {it.desc}</span>}
                        </div>
                        <div className="meta">
                          {it.shortcut && <Kbd>{it.shortcut}</Kbd>}
                          <Icon.CornerDownLeft style={{ width: 11, height: 11, opacity: idx === active ? 1 : 0 }} />
                        </div>
                      </div>
                    );
                  })}
                </div>
              ))}
            </div>
            <div className="cmdk-footer">
              <div className="hint"><Kbd>↑</Kbd><Kbd>↓</Kbd> 选 · <Kbd>↵</Kbd> 打开 · <Kbd>esc</Kbd> 关</div>
              <div className="hint">Forgify · 本地</div>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
