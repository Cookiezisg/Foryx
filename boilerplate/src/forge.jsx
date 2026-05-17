/* eslint-disable react/prop-types */
// Forge view: tabs (functions / handlers / workflows) + table + detail w/ pending diff

const { useState: useForgeState } = React;

function relTime(iso) {
  const diff = (Date.now() - new Date(iso).getTime()) / 1000;
  if (diff < 60) return Math.floor(diff) + " 秒前";
  if (diff < 3600) return Math.floor(diff / 60) + " 分钟前";
  if (diff < 86400) return Math.floor(diff / 3600) + " 小时前";
  return Math.floor(diff / 86400) + " 天前";
}

function KindChip({ kind }) {
  const map = {
    function: { cls: "fn", label: "Function" },
    handler: { cls: "hd", label: "Handler" },
    workflow: { cls: "wf", label: "Workflow" },
    skill: { cls: "sk", label: "Skill" },
    mcp: { cls: "mcp", label: "MCP" }
  };
  const m = map[kind] || { cls: "fn", label: kind };
  return <span className={"kind-chip " + m.cls}>{m.label}</span>;
}

function StatusBadge({ s }) {
  if (s === "ready") return <span className="badge success"><span className="dot" />ready</span>;
  if (s === "pending") return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
      <span className="badge warn"><span className="dot" />pending</span>
      <span className="forge-ai-mark" title="由 AI 锻造产生"><Icon.Sparkles /> AI</span>
    </span>);

  if (s === "draft") return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
      <span className="badge info"><span className="dot" />draft</span>
      <span className="forge-ai-mark" title="由 AI 锻造产生"><Icon.Sparkles /> AI</span>
    </span>);

  if (s === "failed") return <span className="badge error"><span className="dot" />failed</span>;
  return <span className="badge">{s}</span>;
}

function ForgeList({ onOpen }) {
  const [tab, setTab] = useForgeState("all");
  const [selected, setSelected] = useForgeState(new Set());
  const list = Forgify.forges.filter((f) => tab === "all" ? true : f.kind === tab.replace(/s$/, ""));
  const counts = {
    all: Forgify.forges.length,
    functions: Forgify.forges.filter((f) => f.kind === "function").length,
    handlers: Forgify.forges.filter((f) => f.kind === "handler").length,
    workflows: Forgify.forges.filter((f) => f.kind === "workflow").length
  };
  const toggle = (id) => setSelected((s) => {
    const n = new Set(s);
    if (n.has(id)) n.delete(id);else n.add(id);
    return n;
  });
  const clear = () => setSelected(new Set());
  const selectedCount = selected.size;

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Hammer /> 锻造</div>
          <div className="page-subtitle">Function / Handler / Workflow</div>
        </div>
        <div className="page-actions">
          <button className="btn btn-sm"><Icon.Refresh /> 同步</button>
          <button className="btn btn-sm"><Icon.Inbox /> 导入</button>
          <button className="btn btn-sm btn-accent"><Icon.Plus /> 新建</button>
        </div>
      </div>

      <div className="page-tabs">
        {[
        ["all", "全部"],
        ["functions", "Functions"],
        ["handlers", "Handlers"],
        ["workflows", "Workflows"]].
        map(([k, l]) =>
        <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
            {l}
            <span className="count">{counts[k]}</span>
          </button>
        )}
      </div>

      <div className="page-toolbar">
        <div className="search-input">
          <Icon.Search className="icon" />
          <input placeholder="搜索 forge…" />
        </div>
        <button className="btn btn-sm btn-ghost"><Icon.Filter />筛选</button>
        <div style={{ flex: 1 }} />
        <span style={{ fontSize: 11, color: "var(--fg-faint)", fontFamily: "var(--font-mono)" }}>
          {list.length} 条 · 按"最近更新"排序
        </span>
      </div>

      {selectedCount > 0 &&
      <div className="batch-bar">
          <span className="batch-bar-count">已选 {selectedCount} 项</span>
          <button className="btn btn-xs btn-ghost" onClick={clear}>取消选择</button>
          <div className="batch-bar-buttons">
            <button className="btn btn-xs"><Icon.Play /> 批量试跑</button>
            <button className="btn btn-xs"><Icon.Inbox /> 导出</button>
            <button className="btn btn-xs"><Icon.Pin /> 归档</button>
            <button className="btn btn-xs btn-danger"><Icon.Trash /> 删除</button>
            <button className="btn btn-xs btn-accent"><Icon.Check /> 批量 Accept pending</button>
          </div>
        </div>
      }

      <div className="page-body" style={{ padding: 0 }}>
        <table className="t">
          <thead>
            <tr>
              <th style={{ paddingLeft: 16, width: 32 }}>
                <span className={"row-select" + (selectedCount === list.length && list.length > 0 ? " is-checked" : "")}
                onClick={() => selectedCount === list.length ? clear() : setSelected(new Set(list.map((f) => f.id)))}>
                  {selectedCount === list.length && list.length > 0 && <Icon.Check />}
                </span>
              </th>
              <th>名称</th>
              <th>类型</th>
              <th>版本</th>
              <th>运行次数</th>
              <th>状态</th>
              <th>最近更新</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {list.map((f) =>
            <tr key={f.id} className={selected.has(f.id) ? "is-selected" : ""}>
                <td style={{ paddingLeft: 16 }} onClick={(e) => {e.stopPropagation();toggle(f.id);}}>
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
                      <div style={{ fontSize: 11, color: "var(--fg-muted)", marginTop: 1 }}>{f.desc}</div>
                    </div>
                  </div>
                </td>
                <td onClick={() => onOpen(f)}><KindChip kind={f.kind} /></td>
                <td onClick={() => onOpen(f)}><span className="cell-mono">{f.version}</span></td>
                <td onClick={() => onOpen(f)}><span className="cell-mono">{f.runs.toLocaleString()}</span></td>
                <td onClick={() => onOpen(f)}><StatusBadge s={f.status} /></td>
                <td onClick={() => onOpen(f)}><RelTime ts={f.updatedAt} /></td>
                <td className="col-tight">
                  <ActionMenu items={[
                { label: f.kind === "handler" ? "试调用" : "试跑", icon: Icon.Play },
                { label: "查看版本历史", icon: Icon.GitBranch },
                { label: "复制", icon: Icon.Copy, shortcut: "⌘D" },
                "divider",
                { label: "导出", icon: Icon.Inbox },
                { label: "归档", icon: Icon.Folder },
                { label: "删除", icon: Icon.Trash, danger: true, shortcut: "⌫" }]
                } />
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>);

}

function ForgeDetail({ forge, onBack }) {
  const isPending = forge.status === "pending" || forge.status === "draft";

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 8 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <button onClick={onBack} className="btn btn-xs btn-ghost"><Icon.ChevronRight style={{ transform: "rotate(180deg)" }} /> 返回</button>
            <span>·</span>
            <KindChip kind={forge.kind} />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge.id}</span>
          </div>
          <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{forge.name}</div>
          <div className="page-subtitle">{forge.desc}</div>
        </div>
        <div className="page-actions">
          {isPending &&
          <>
              <button className="btn btn-sm btn-danger"><Icon.X /> Revert</button>
              <button className="btn btn-sm"><Icon.Refresh /> Iterate</button>
              <button className="btn btn-sm btn-accent"><Icon.Check /> Accept · v{(parseInt(forge.version.slice(1)) || 0) + 1}</button>
            </>
          }
          {!isPending &&
          <>
              <button className="btn btn-sm"><Icon.Play /> 试跑</button>
              <button className="btn btn-sm"><Icon.GitBranch /> 历史</button>
              <button className="btn btn-sm"><Icon.MoreHorizontal /></button>
            </>
          }
        </div>
      </div>

      <div className="split">
        <div className="pane-main">
          {isPending &&
          <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "10px 14px", background: "oklch(from var(--status-warn) l c h / 0.08)", border: "1px solid oklch(from var(--status-warn) l c h / 0.20)", borderRadius: 8, marginBottom: 16 }}>
              <Icon.AlertCircle style={{ color: "var(--status-warn)", width: 16, height: 16 }} />
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 13, color: "var(--fg-strong)", fontWeight: 500 }}>这是一个 pending 版本</div>
                <div style={{ fontSize: 11, color: "var(--fg-muted)" }}>由对话 <a href="#">CSV → Notion 同步脚本</a> 锻造产生。Accept 才会成为最新版。</div>
              </div>
              <span className="badge muted">v{forge.version} → v{(parseInt(forge.version.slice(1)) || 0) + 1}</span>
            </div>
          }

          <h3 style={{ fontSize: 13, color: "var(--fg-faint)", textTransform: "uppercase", letterSpacing: "0.06em", margin: "6px 0 10px 0", fontWeight: 600 }}>
            Pending Changes
          </h3>

          <div className="diff">
            <div className="diff-head">
              <span>aggregate_week.py</span>
              <div className="stats">
                <span className="add">+10</span>
                <span className="del">-1</span>
              </div>
            </div>
            <div className="diff-body">
              {Forgify.pendingDiff.map((row, i) =>
              <div key={i} className={"diff-row " + (row.type === "hunk" ? "hunk" : row.type)}>
                  {row.type === "hunk" ?
                <div className="code" style={{ gridColumn: "1 / -1" }}>{row.text}</div> :

                <>
                      <div className="ln">{row.type === "add" ? "+" : row.type === "del" ? "-" : " "}</div>
                      <div className="ln">{i}</div>
                      <div className="code">{row.code}</div>
                    </>
                }
                </div>
              )}
            </div>
          </div>

          <h3 style={{ fontSize: 13, color: "var(--fg-faint)", textTransform: "uppercase", letterSpacing: "0.06em", margin: "22px 0 10px 0", fontWeight: 600 }}>
            Schema
          </h3>
          <pre className="code-block" style={{ fontSize: 12 }}>{`inputs:
  activities: Activity[]      # from fn_strava_002

outputs:
  by_week: dict[isoweek, {
    avg_pace_s_per_km: float
    total_climb_m:     float
    avg_hr:            float
    count:             int
  }]`}</pre>
        </div>

        <aside className="pane-aside">
          <div className="aside-section">
            <div className="aside-label">基本信息</div>
            <div className="aside-kv">
              <div className="k">ID</div><div className="v">{forge.id}</div>
              <div className="k">类型</div><div className="v"><KindChip kind={forge.kind} /></div>
              <div className="k">版本</div><div className="v">{forge.version}</div>
              <div className="k">状态</div><div className="v"><StatusBadge s={forge.status} /></div>
              <div className="k">运行次数</div><div className="v">{forge.runs.toLocaleString()}</div>
              <div className="k">最近更新</div><div className="v">{relTime(forge.updatedAt)}</div>
            </div>
          </div>

          <div className="aside-section">
            <div className="aside-label">依赖</div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <div className="cell-flex">
                <KindChip kind="function" />
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>fn_strava_002</span>
              </div>
              <div className="cell-flex">
                <KindChip kind="handler" />
                <span style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>hd_notion_001</span>
              </div>
            </div>
          </div>

          <div className="aside-section">
            <div className="aside-label">沙箱</div>
            <div className="aside-kv">
              <div className="k">Runtime</div><div className="v">python 3.12.13</div>
              <div className="k">Env</div><div className="v">fnenv_a1b2…</div>
              <div className="k">大小</div><div className="v">142 MB</div>
            </div>
          </div>

          <div className="aside-section">
            <div className="aside-label">近期试跑</div>
            <div style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 12 }}>
              {["3 分钟前 · ok", "12 分钟前 · ok", "1 小时前 · fail · NameError"].map((s, i) =>
              <div key={i} className="cell-flex" style={{ color: "var(--fg-muted)", fontFamily: "var(--font-mono)", fontSize: 11 }}>
                  <span className="dot" style={{ width: 6, height: 6, borderRadius: "50%", background: s.includes("fail") ? "var(--status-error)" : "var(--status-success)" }} />
                  <span style={{ flex: 1 }}>{s}</span>
                </div>
              )}
            </div>
          </div>
        </aside>
      </div>
    </div>);

}

function ForgeView({ onOpenDetail }) {
  const [open, setOpen] = useForgeState(null);
  React.useEffect(() => {
    const id = window.Shell?.focusEntity?.forge;
    if (id) {
      const f = Forgify.forges.find((x) => x.id === id);
      if (f) setOpen(f);
    }
  }, []);
  if (open) {
    if (open.kind === "function") return <FunctionDetail forge={open} onBack={() => setOpen(null)} />;
    if (open.kind === "handler") return <HandlerDetail forge={open} onBack={() => setOpen(null)} />;
    if (open.kind === "workflow") return <WorkflowView forge={open} onBack={() => setOpen(null)} />;
    return <ForgeDetail forge={open} onBack={() => setOpen(null)} />;
  }
  return <ForgeList onOpen={setOpen} />;
}

window.ForgeView = ForgeView;
window.KindChip = KindChip;
window.StatusBadge = StatusBadge;
window.relTime = relTime;