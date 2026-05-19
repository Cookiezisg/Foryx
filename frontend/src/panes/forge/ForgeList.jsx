// ForgeList — unified table over function/handler/workflow. Three
// REST hooks run in parallel; results merge into one rows array sorted
// by updatedAt desc. Tab filters by kind. Click row → opens detail.
//
// ForgeList —— trinity 三域合并表；tabs 按类型过滤；行点击进 detail。

import { useMemo, useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { KindChip } from "../../components/shared/KindChip.jsx";
import { StatusBadge } from "../../components/shared/StatusBadge.jsx";
import { RelTime } from "../../components/shared/RelTime.jsx";
import { ActionMenu } from "../../components/shared/ActionMenu.jsx";
import { useFunctions, useHandlers, useWorkflows, useDeleteFunction } from "../../api/forge.js";
import { useForgeProgress } from "../../sse/useForge.js";

const TABS = [
  { key: "all",       label: "全部" },
  { key: "function",  label: "Functions" },
  { key: "handler",   label: "Handlers" },
  { key: "workflow",  label: "Workflows" },
];

export function ForgeList({ onOpen }) {
  const [tab, setTab] = useState("all");
  const [q, setQ] = useState("");
  const [selected, setSelected] = useState(new Set());

  const { data: functions = [] } = useFunctions();
  const { data: handlers = [] } = useHandlers();
  const { data: workflows = [] } = useWorkflows();
  const activeForge = useForgeProgress((s) => s.active);
  const deleteFn = useDeleteFunction();

  const rows = useMemo(() => {
    const fns = functions.map((x) => ({ ...x, kind: "function" }));
    const hds = handlers.map((x) => ({ ...x, kind: "handler" }));
    const wfs = workflows.map((x) => ({ ...x, kind: "workflow" }));
    const all = [...fns, ...hds, ...wfs];
    return all.sort((a, b) => {
      const ta = new Date(a.updatedAt || a.createdAt || 0).getTime();
      const tb = new Date(b.updatedAt || b.createdAt || 0).getTime();
      return tb - ta;
    });
  }, [functions, handlers, workflows]);

  const filtered = useMemo(() => {
    const ql = q.toLowerCase();
    return rows.filter((r) => {
      if (tab !== "all" && r.kind !== tab) return false;
      if (!ql) return true;
      return (r.name || "").toLowerCase().includes(ql)
        || (r.desc || r.description || "").toLowerCase().includes(ql);
    });
  }, [rows, tab, q]);

  const counts = {
    all: rows.length,
    function: rows.filter((r) => r.kind === "function").length,
    handler: rows.filter((r) => r.kind === "handler").length,
    workflow: rows.filter((r) => r.kind === "workflow").length,
  };

  const toggle = (id) => {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id); else next.add(id);
    setSelected(next);
  };
  const clearSel = () => setSelected(new Set());

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Hammer /> 锻造</div>
          <div className="page-subtitle">Function / Handler / Workflow</div>
        </div>
        <div className="page-actions">
          <Button size="sm"><Icon.Inbox /> 导入</Button>
          <Button size="sm" variant="accent"><Icon.Plus /> 新建</Button>
        </div>
      </div>

      <div className="page-tabs">
        {TABS.map(({ key, label }) => (
          <button
            key={key}
            className={"page-tab" + (tab === key ? " is-active" : "")}
            onClick={() => setTab(key)}
          >
            {label}
            <span className="count">{counts[key] ?? 0}</span>
          </button>
        ))}
      </div>

      <div className="page-toolbar">
        <div className="search-input">
          <Icon.Search className="icon" />
          <input
            placeholder="搜索 forge 名称 / 描述…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <div style={{ flex: 1 }} />
        <span style={{ fontSize: 11, color: "var(--fg-faint)", fontFamily: "var(--font-mono)" }}>
          {filtered.length} 条 · 按"最近更新"
        </span>
      </div>

      {selected.size > 0 && (
        <div className="batch-bar">
          <span className="batch-bar-count">已选 {selected.size} 项</span>
          <Button size="xs" variant="ghost" onClick={clearSel}>取消选择</Button>
          <div className="batch-bar-buttons">
            <Button size="xs" variant="danger">
              <Icon.Trash /> 删除
            </Button>
          </div>
        </div>
      )}

      <div className="page-body" style={{ padding: 0 }}>
        {filtered.length === 0 ? (
          <div className="empty" style={{ padding: 48 }}>
            <Icon.Hammer className="icon" />
            <div className="title">还没有 {tab === "all" ? "" : tab} 锻造产物</div>
            <div className="sub">在对话里告诉 AI："帮我做一个 X 工具"</div>
          </div>
        ) : (
          <table className="t">
            <thead>
              <tr>
                <th style={{ paddingLeft: 16, width: 32 }}>
                  <span className={"row-select" + (selected.size === filtered.length && filtered.length > 0 ? " is-checked" : "")}
                        onClick={() => selected.size === filtered.length ? clearSel() : setSelected(new Set(filtered.map((f) => f.id)))}>
                    {selected.size === filtered.length && filtered.length > 0 && <Icon.Check />}
                  </span>
                </th>
                <th>名称</th>
                <th>类型</th>
                <th>版本</th>
                <th>状态</th>
                <th>最近更新</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {filtered.map((f) => {
                const scopeKey = `${f.kind}:${f.id}`;
                const inProgress = activeForge[scopeKey];
                return (
                  <tr
                    key={f.id}
                    className={selected.has(f.id) ? "is-selected" : ""}
                  >
                    <td style={{ paddingLeft: 16 }} onClick={(e) => { e.stopPropagation(); toggle(f.id); }}>
                      <span className={"row-select" + (selected.has(f.id) ? " is-checked" : "")}>
                        {selected.has(f.id) && <Icon.Check />}
                      </span>
                    </td>
                    <td onClick={() => onOpen(f)}>
                      <div className="cell-flex">
                        <div style={{ width: 24, height: 24, borderRadius: 5, background: "var(--bg-elev-2)", border: "1px solid var(--border-soft)", display: "grid", placeItems: "center", color: "var(--fg-muted)" }}>
                          {f.kind === "function" && <Icon.Code style={{ width: 13, height: 13 }} />}
                          {f.kind === "handler" && <Icon.Server style={{ width: 13, height: 13 }} />}
                          {f.kind === "workflow" && <Icon.Workflow style={{ width: 13, height: 13 }} />}
                        </div>
                        <div>
                          <div className="cell-strong">{f.name}</div>
                          <div style={{ fontSize: 11, color: "var(--fg-muted)", marginTop: 1 }}>
                            {f.desc || f.description || ""}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td onClick={() => onOpen(f)}><KindChip kind={f.kind} /></td>
                    <td onClick={() => onOpen(f)}>
                      <span className="cell-mono">{f.versionLabel || f.version || "—"}</span>
                    </td>
                    <td onClick={() => onOpen(f)}>
                      {inProgress
                        ? <span className="badge streaming"><span className="dot" />锻造中</span>
                        : <StatusBadge status={f.status || "ready"} />}
                    </td>
                    <td onClick={() => onOpen(f)}>
                      {(f.updatedAt || f.createdAt) ? <RelTime ts={f.updatedAt || f.createdAt} /> : "—"}
                    </td>
                    <td className="col-tight">
                      <ActionMenu
                        items={[
                          { label: f.kind === "handler" ? "试调用" : "试跑", icon: Icon.Play, onClick: () => onOpen(f) },
                          { label: "查看版本历史", icon: Icon.GitBranch, onClick: () => onOpen(f) },
                          "divider",
                          { label: "删除", icon: Icon.Trash, danger: true, onClick: () => {
                            if (f.kind === "function") deleteFn.mutate(f.id);
                          }},
                        ]}
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
