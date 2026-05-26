// CommandPalette (⌘K) — fuzzy search across panes / conversations /
// forge entities. Keyboard-first: arrow up/down navigate, Enter selects,
// Esc closes. Mouse hover updates active index for smoother feel.
//
// CommandPalette —— ⌘K 调起的命令板；箭头键导航 / Enter 选 / Esc 关。

import React, { useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion } from "framer-motion";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { Kbd } from "@shared/ui/Kbd";
import { useConversations } from "@entities/conversation";
import { useFunctions } from "@entities/function";
import { useHandlers } from "@entities/handler";
import { useWorkflows } from "@entities/workflow";
import { useFlowRuns } from "@entities/flowrun";
import { scaleIn, fadeIn } from "@shared/lib/motion";

const NAV_ITEMS_DEF = [
  { icon: Icon.MessageSquare, labelKey: "nav.chat",      desc: "",                              target: "chat" },
  { icon: Icon.Hammer,        labelKey: "nav.forge",     desc: "function / handler / workflow", target: "forge" },
  { icon: Icon.Play,          labelKey: "nav.execute",   desc: "flowrun · trigger",             target: "execute" },
  { icon: Icon.FileText,      labelKey: "nav.documents", desc: "",                              target: "documents" },
  { icon: Icon.Sparkles,      label: "Skills",           desc: "",                              target: "skills" },
  { icon: Icon.Server,        label: "MCP",              desc: "",                              target: "mcp" },
  { icon: Icon.Brain,         label: "Memory",           desc: "",                              target: "memory" },
  { icon: Icon.Settings,      labelKey: "cmdk.settings", desc: "",                              overlay: "settings" },
];

interface CommandPaletteProps {
  open: boolean;
  onClose: () => void;
  onOpenPane: (pane: string) => void;
  onOpenEntity: (pane: string, id: string) => void;
  onSetActiveConv: (id: string | null) => void;
  onOpenSettings: () => void;
}

export function CommandPalette({ open, onClose, onOpenPane, onOpenEntity, onSetActiveConv, onOpenSettings }: CommandPaletteProps) {
  const { t } = useTranslation("sidebar");

  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);

  const { data: conversations = [] } = useConversations();
  const { data: functions = [] } = useFunctions();
  const { data: handlers = [] } = useHandlers();
  const { data: workflows = [] } = useWorkflows();
  const { data: flowruns = [] } = useFlowRuns();

  const items = useMemo(() => {
    const navGroup = t("cmdk.navGroup");
    const a = [];
    for (const nav of NAV_ITEMS_DEF) {
      const label = nav.labelKey ? t(nav.labelKey) : nav.label;
      const action = nav.overlay === "settings"
        ? () => onOpenSettings()
        : () => onOpenPane(nav.target);
      a.push({ ...nav, group: navGroup, label, action });
    }
    for (const c of conversations.slice(0, 6)) {
      a.push({
        group: t("cmdk.convGroup"),
        icon: Icon.MessageSquare,
        label: c.title || t("cmdk.unnamed"),
        desc: c.id,
        action: () => { onSetActiveConv(c.id); onOpenPane("chat"); },
      });
    }
    for (const f of (functions as any[]).slice(0, 5)) {
      a.push({
        group: "Function",
        icon: Icon.Code,
        label: f.name,
        desc: f.desc || f.description || f.id,
        action: () => onOpenEntity("forge", f.id),
      });
    }
    for (const h of (handlers as any[]).slice(0, 5)) {
      a.push({
        group: "Handler",
        icon: Icon.Server,
        label: h.name,
        desc: h.desc || h.description || h.id,
        action: () => onOpenEntity("forge", h.id),
      });
    }
    for (const w of (workflows as any[]).slice(0, 5)) {
      a.push({
        group: "Workflow",
        icon: Icon.Workflow,
        label: w.name,
        desc: w.desc || w.description || w.id,
        action: () => onOpenEntity("forge", w.id),
      });
    }
    for (const f of (flowruns as any[]).slice(0, 5)) {
      a.push({
        group: "FlowRun",
        icon: Icon.Play,
        label: f.workflow || f.workflowId,
        desc: f.id,
        action: () => onOpenEntity("execute", f.id),
      });
    }
    return a;
  }, [conversations, functions, handlers, workflows, flowruns, onOpenPane, onOpenEntity, onSetActiveConv, onOpenSettings, t]);

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
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") { onClose(); return; }
      if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(a + 1, filtered.length - 1)); }
      if (e.key === "ArrowUp")   { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
      if (e.key === "Enter") {
        const it = filtered[active];
        if (it) { it.action?.(); onClose(); }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, filtered, active, onClose]);

  let cursor = 0;

  return (
    <AnimatePresence>
      {open && (
        <motion.div className="overlay" {...(fadeIn as any)} onClick={() => onClose()}>
          <motion.div className="cmdk" {...(scaleIn as any)} onClick={(e) => e.stopPropagation()}>
            <div className="cmdk-input-wrap">
              <Icon.Search className="icon" />
              <input
                className="cmdk-input"
                autoFocus
                placeholder={t("cmdk.placeholder")}
                value={q}
                onChange={(e) => { setQ(e.target.value); setActive(0); }}
              />
              <Kbd>ESC</Kbd>
            </div>
            <div className="cmdk-list">
              {groups.length === 0 && (
                <div style={{ padding: "32px 16px", textAlign: "center", color: "var(--fg-faint)", fontSize: 13 }}>
                  {t("cmdk.noResults")}
                </div>
              )}
              {groups.map(([gname, gitems]) => (
                <div key={gname}>
                  <div className="cmdk-group-label">{gname}</div>
                  {gitems.map((it: any) => {
                    const idx = cursor++;
                    const IconC = it.icon || Icon.ChevronRight;
                    return (
                      <div
                        key={idx}
                        className={"cmdk-row" + (idx === active ? " is-active" : "")}
                        onMouseEnter={() => setActive(idx)}
                        onClick={() => { it.action?.(); onClose(); }}
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
              <div className="hint"><Kbd>↑</Kbd><Kbd>↓</Kbd> {t("cmdk.footerNav")} · <Kbd>↵</Kbd> {t("cmdk.footerOpen")} · <Kbd>esc</Kbd> {t("cmdk.footerClose")}</div>
              <div className="hint">{t("cmdk.footerLocal")}</div>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
