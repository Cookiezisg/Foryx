/* eslint-disable react/prop-types */
// Execute view — overview with workflow heatmap + drill-in non-linear DAG + Gantt

const { useState: useExecState, useMemo: useExecMemo } = React;

// ── Status helpers ───────────────────────────────────────────────────────
const STATUS_COLOR = {
  ok: "var(--status-success)",
  fail: "var(--status-error)",
  running: "var(--accent)",
  wait: "var(--status-warn)",
  pending: "var(--fg-faint)",
  skip: "var(--fg-faint)",
  cancelled: "var(--fg-muted)"
};

function flowStatusBadge(s) {
  switch (s) {
    case "running":return <span className="badge streaming"><span className="dot" />运行中</span>;
    case "completed":return <span className="badge success"><span className="dot" />完成</span>;
    case "failed":return <span className="badge error"><span className="dot" />失败</span>;
    case "waiting_approval":return <span className="badge warn"><span className="dot" />待批准</span>;
    case "paused":return <span className="badge info"><span className="dot" />已暂停</span>;
    case "cancelled":return <span className="badge"><span className="dot" />已取消</span>;
    default:return <span className="badge">{s}</span>;
  }
}

// ── KPI strip ────────────────────────────────────────────────────────────
function KpiStrip() {
  const k = (label, value, sub, trend) =>
  <div className="kpi">
      <div className="kpi-label">{label}</div>
      <div className="kpi-value">{value}</div>
      <div className="kpi-sub">{sub}</div>
      <svg viewBox="0 0 200 28" preserveAspectRatio="none" className="kpi-spark">
        <polyline points={trend} fill="none" stroke="var(--accent)" strokeWidth="1.5" strokeLinejoin="round" />
      </svg>
    </div>;

  return (
    <div className="kpi-strip">
      {k("今日运行", "47", "+6 vs 昨天", "0,18 14,16 28,14 42,18 56,12 70,10 84,14 98,8 112,12 126,6 140,9 154,4 168,7 182,3 196,5")}
      {k("成功率", "94%", "近 24h · 3 个失败", "0,4 14,8 28,4 42,12 56,5 70,9 84,3 98,8 112,4 126,6 140,3 154,7 168,2 182,4 196,2")}
      {k("中位耗时", "1.8s", "p95 24s · p99 41s", "0,12 14,16 28,8 42,14 56,10 70,12 84,6 98,11 112,5 126,9 140,4 154,7 168,3 182,5 196,3")}
      {k("待批准", "1", "weekly-training · 28 分钟前", "0,2 14,3 28,2 42,4 56,2 70,3 84,5 98,3 112,4 126,5 140,3 154,4 168,2 182,3 196,3")}
    </div>);

}

// ── Workflow run heatmap ─────────────────────────────────────────────────
function WorkflowHeatmap({ onPick, query, selected }) {
  const rows = Object.entries(Forgify.workflowHistory).filter(([wf]) =>
  !query || wf.toLowerCase().includes(query.toLowerCase())
  );
  return (
    <div className="hm-wrap">
      <div className="hm-head">
        <div className="hm-head-label">最近 30 次运行 · {rows.length} 个 workflow</div>
        <div className="hm-head-legend">
          <span><i style={{ background: STATUS_COLOR.ok }} /> 完成</span>
          <span><i style={{ background: STATUS_COLOR.fail }} /> 失败</span>
          <span><i style={{ background: STATUS_COLOR.running }} /> 运行中</span>
          <span><i style={{ background: STATUS_COLOR.wait }} /> 待批准</span>
        </div>
      </div>
      <div className="hm-grid" style={{ maxHeight: 200, overflowY: "auto" }}>
        {rows.length === 0 ?
        <div className="empty" style={{ padding: 24 }}>
            <div className="sub">没匹配的 workflow</div>
          </div> :
        rows.map(([wf, hist]) => {
          const cells = [...hist].reverse();
          const successCt = hist.filter((s) => s === "ok").length;
          const failCt = hist.filter((s) => s === "fail").length;
          return (
            <div key={wf} className={"hm-row" + (selected === wf ? " is-selected" : "")}>
              <button className="hm-row-name" onClick={() => onPick(wf)}>{wf}</button>
              <div className="hm-cells">
                {cells.map((s, i) =>
                <span
                  key={i}
                  className={"hm-cell hm-" + s}
                  style={{ background: STATUS_COLOR[s] || "var(--bg-elev-2)" }}
                  title={`${i === 0 ? "刚才" : i + " 次前"} · ${s}`} />

                )}
              </div>
              <div className="hm-row-stats">
                <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>
                  {successCt}/{hist.length}
                </span>
                {failCt > 0 &&
                <span className="cell-mono" style={{ color: "var(--status-error)" }}>
                    · {failCt} 失败
                  </span>
                }
              </div>
            </div>);

        })}
      </div>
    </div>);

}

// ── FlowRuns table with mini-bars ────────────────────────────────────────
function ProgressMini({ done, total, status }) {
  const color = status === "failed" ? "var(--status-error)" : status === "waiting_approval" ? "var(--status-warn)" : status === "running" ? "var(--accent)" : "var(--status-success)";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <div className="progress-bar" style={{ width: 80 }}>
        <div style={{ width: done / total * 100 + "%", background: color }} />
      </div>
      <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-faint)", whiteSpace: "nowrap" }}>{done}/{total}</span>
    </div>);

}

function FlowRunsTable({ onOpen, filter, query, statusFilter }) {
  const list = Forgify.flowruns.filter((f) => {
    if (filter && f.workflow !== filter) return false;
    if (statusFilter && statusFilter !== "all" && f.status !== statusFilter) return false;
    if (query) {
      const q = query.toLowerCase();
      if (!(f.workflow.toLowerCase().includes(q) || f.id.toLowerCase().includes(q) || (f.trigger || "").toLowerCase().includes(q))) return false;
    }
    return true;
  });
  if (list.length === 0) {
    return <div className="empty" style={{ padding: 32 }}><div className="title">没有匹配的运行</div><div className="sub">改一下筛选条件</div></div>;
  }
  return (
    <table className="t">
      <thead>
        <tr>
          <th style={{ paddingLeft: 32 }}>Workflow</th>
          <th>状态</th>
          <th>节点</th>
          <th>触发</th>
          <th>开始</th>
          <th>耗时</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {list.map((fr) =>
        <tr key={fr.id} onClick={() => onOpen(fr)}>
            <td style={{ paddingLeft: 32 }}>
              <div>
                <div className="cell-strong">{fr.workflow}</div>
                <div className="cell-mono" style={{ marginTop: 2 }}>{fr.id}</div>
              </div>
            </td>
            <td>{flowStatusBadge(fr.status)}</td>
            <td><ProgressMini done={fr.nodes.done} total={fr.nodes.total} status={fr.status} /></td>
            <td><span className="cell-mono" style={{ fontSize: 11 }}>{fr.trigger}</span></td>
            <td><span style={{ fontSize: 12, color: "var(--fg-muted)" }}>{relTime(fr.startedAt)}</span></td>
            <td><span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-muted)" }}>{fmtDuration(fr.durationMs)}</span></td>
            <td className="col-tight"><button className="icon-btn"><Icon.MoreHorizontal /></button></td>
          </tr>
        )}
      </tbody>
    </table>);

}

// ── FlowRun detail — DAG + node inspector + Gantt ────────────────────────
function NodeStatusIcon({ status }) {
  if (status === "ok") return <Icon.Check style={{ width: 12, height: 12, color: "var(--status-success)" }} />;
  if (status === "fail") return <Icon.X style={{ width: 12, height: 12, color: "var(--status-error)" }} />;
  if (status === "running") return <span className="spinner" style={{ width: 12, height: 12, borderColor: "color-mix(in srgb, var(--accent) 30%, transparent)", borderTopColor: "var(--accent)" }} />;
  if (status === "wait") return <Icon.Clock style={{ width: 12, height: 12, color: "var(--status-warn)" }} />;
  if (status === "skip" || status === "pending") return <span style={{ width: 8, height: 8, borderRadius: "50%", border: "1.5px dashed var(--fg-faint)" }} />;
  return null;
}

function dagEdgePath(a, b) {
  const sx = a.x + 92,sy = a.y + 60;
  const ex = b.x + 92,ey = b.y;
  const dy = Math.max(30, (ey - sy) / 2);
  return `M ${sx} ${sy} C ${sx} ${sy + dy}, ${ex} ${ey - dy}, ${ex} ${ey}`;
}

function FlowRunDag({ detail, selected, onSelect }) {
  const byId = Object.fromEntries(detail.nodes.map((n) => [n.id, n]));
  return (
    <div className="fr-dag">
      <svg className="fr-dag-edges" width="100%" height="100%">
        <defs>
          <marker id="fr-arr" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M0 0 L10 5 L0 10 z" fill="var(--border-strong)" />
          </marker>
        </defs>
        {detail.edges.map((e, i) => {
          const a = byId[e.from],b = byId[e.to];
          if (!a || !b) return null;
          const fromOk = a.status === "ok";
          return (
            <path
              key={i}
              d={dagEdgePath(a, b)}
              fill="none"
              stroke={fromOk ? "var(--border-strong)" : "var(--border)"}
              strokeWidth="1.4"
              strokeDasharray={fromOk ? "" : "4 4"}
              markerEnd="url(#fr-arr)"
              opacity={fromOk ? 1 : 0.6} />);


        })}
      </svg>
      {detail.nodes.map((n) =>
      <div
        key={n.id}
        className={"fr-dag-node fr-status-" + n.status + (selected === n.id ? " is-selected" : "")}
        style={{ left: n.x, top: n.y }}
        onClick={() => onSelect(n.id)}>
        
          <div className="fr-dag-node-head">
            <NodeStatusIcon status={n.status} />
            <KindChip kind={n.kind === "trigger" ? "function" : n.kind === "handler" ? "handler" : n.kind === "condition" ? "function" : "function"} />
          </div>
          <div className="fr-dag-node-title">{n.label}</div>
          <div className="fr-dag-node-sub">
            {n.durationMs != null ? fmtDuration(n.durationMs) : n.status === "running" ? "运行中…" : n.status === "pending" ? "等待" : "—"}
          </div>
        </div>
      )}
    </div>);

}

function NodeInspector({ node, nodeDetail }) {
  if (!node) {
    return (
      <div className="fr-inspector">
        <div className="empty" style={{ padding: "32px 16px" }}>
          <Icon.Filter className="icon" />
          <div className="title">点节点查看细节</div>
          <div className="sub">input · output · log · 重试</div>
        </div>
      </div>);

  }
  return (
    <div className="fr-inspector">
      <div className="fr-inspector-head">
        <div className="fr-inspector-title">
          <NodeStatusIcon status={node.status} />
          <span>{node.label}</span>
        </div>
        <div className="fr-inspector-meta">
          <KindChip kind={node.kind === "trigger" ? "function" : node.kind === "handler" ? "handler" : "function"} />
          {node.durationMs != null && <span className="cell-mono">{fmtDuration(node.durationMs)}</span>}
          {nodeDetail?.retries > 0 && <span className="badge warn"><span className="dot" />{nodeDetail.retries} 次重试</span>}
        </div>
        {node.error && <div className="fr-inspector-error">{node.error}</div>}
      </div>

      <div className="fr-inspector-body">
        {nodeDetail?.input != null &&
        <div className="fr-section">
            <div className="fr-section-label">Input</div>
            <pre className="code-block" style={{ fontSize: 11 }}>{JSON.stringify(nodeDetail.input, null, 2)}</pre>
          </div>
        }
        {nodeDetail?.output != null &&
        <div className="fr-section">
            <div className="fr-section-label">Output</div>
            <pre className="code-block" style={{ fontSize: 11 }}>{JSON.stringify(nodeDetail.output, null, 2)}</pre>
          </div>
        }
        {nodeDetail?.log &&
        <div className="fr-section">
            <div className="fr-section-label">Log</div>
            <div className="fr-log">
              {nodeDetail.log.map((l, i) =>
            <div key={i} className={"fr-log-row level-" + l.level}>
                  <span className="fr-log-time">{l.time}</span>
                  <span className="fr-log-level">{l.level}</span>
                  <span className="fr-log-msg">{l.msg}</span>
                </div>
            )}
            </div>
          </div>
        }
        {!nodeDetail &&
        <div className="empty" style={{ padding: 20 }}>
            <div className="sub" style={{ color: "var(--fg-faint)" }}>该节点没有产生 input/output（被跳过或还未运行）</div>
          </div>
        }
      </div>
    </div>);

}

function GanttTimeline({ detail }) {
  const total = Math.max(...detail.nodes.map((n) => (n.startedMs ?? 0) + (n.durationMs ?? 0))) || 1;
  return (
    <div className="fr-gantt">
      <div className="fr-gantt-head">
        <span className="fr-gantt-title">时间线</span>
        <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>总耗时 {fmtDuration(total)} · 0ms 起点</span>
      </div>
      <div className="fr-gantt-body">
        {detail.nodes.map((n) => {
          const start = n.startedMs ?? 0;
          const dur = n.durationMs ?? 0;
          const left = (start / total * 100).toFixed(1) + "%";
          const width = Math.max(0.5, dur / total * 100).toFixed(1) + "%";
          const color = n.status === "fail" ? "var(--status-error)" : n.status === "running" ? "var(--accent)" : n.status === "ok" ? "var(--status-success)" : "var(--fg-faint)";
          return (
            <div key={n.id} className="fr-gantt-row">
              <div className="fr-gantt-label">{n.label}</div>
              <div className="fr-gantt-track">
                {n.startedMs != null ?
                <div
                  className={"fr-gantt-bar status-" + n.status}
                  style={{ left, width, background: color }}
                  title={`${start}ms → ${start + dur}ms`} /> :


                <div className="fr-gantt-pending">未运行</div>
                }
              </div>
              <div className="fr-gantt-dur cell-mono">
                {dur ? fmtDuration(dur) : "—"}
              </div>
            </div>);

        })}
      </div>
    </div>);

}

function FlowRunDetail({ fr, onBack }) {
  const detail = Forgify.flowrunDetails[fr.id] || Forgify.flowrunDetails.fr_8d2a;
  const [selected, setSelected] = useExecState(detail.selectedNode || detail.nodes[0]?.id);
  const [showTriage, setShowTriage] = useExecState(fr.status === "failed");
  const [showDiff, setShowDiff] = useExecState(false);
  const node = detail.nodes.find((n) => n.id === selected);
  const nodeDetail = detail.nodeDetails?.[selected];

  const okCount = detail.nodes.filter((n) => n.status === "ok").length;
  const failCount = detail.nodes.filter((n) => n.status === "fail").length;
  const skipCount = detail.nodes.filter((n) => n.status === "skip" || n.status === "pending").length;
  const failedNode = detail.nodes.find((n) => n.status === "fail");

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <button onClick={onBack} className="btn btn-xs btn-ghost">← 返回</button>
            <span>·</span>
            <span className="cell-mono">{fr.id}</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{fr.workflow}</div>
            {flowStatusBadge(fr.status)}
          </div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>由 <code style={{ fontFamily: "var(--font-mono)" }}>{fr.trigger}</code> 触发 · <RelTime ts={fr.startedAt} /></span>
            <span style={{ color: "var(--status-success)" }}> · {okCount} ok</span>
            {failCount > 0 && <span style={{ color: "var(--status-error)" }}> · {failCount} fail</span>}
            {skipCount > 0 && <span style={{ color: "var(--fg-faint)" }}> · {skipCount} skip</span>}
            <EntityRelMeta entityId={fr.id} />
          </div>
        </div>
        <div className="page-actions">
          <button className="btn btn-sm" onClick={() => setShowDiff(true)}><Icon.GitBranch /> 与历史 diff</button>
          {fr.status === "running" && <button className="btn btn-sm btn-danger"><Icon.StopCircle /> 取消</button>}
          {fr.status === "waiting_approval" && <button className="btn btn-sm btn-accent"><Icon.Check /> 批准并继续</button>}
          {fr.status === "failed" && <>
            <button className="btn btn-sm" onClick={() => setShowTriage((s) => !s)}>
              <Icon.Sparkles /> AI 排查
            </button>
            <button className="btn btn-sm"><Icon.Refresh /> 重跑</button>
          </>}
          {fr.status !== "failed" && <button className="btn btn-sm"><Icon.Refresh /> 重跑</button>}
        </div>
      </div>

      {showTriage && fr.status === "failed" && failedNode &&
      <TriagePanel fr={fr} failedNode={failedNode} nodeDetail={detail.nodeDetails?.[failedNode.id]} onClose={() => setShowTriage(false)} />
      }
      {showDiff &&
      <RunDiffPanel fr={fr} onClose={() => setShowDiff(false)} />
      }

      <div className="fr-shell">
        <FlowRunDag detail={detail} selected={selected} onSelect={setSelected} />
        <NodeInspector node={node} nodeDetail={nodeDetail} />
      </div>
      <GanttTimeline detail={detail} />
    </div>);

}

// ── AI Triage panel (inline failure deep-dive) ──────────────────────────
function TriagePanel({ fr, failedNode, nodeDetail, onClose }) {
  return (
    <div className="triage">
      <div className="triage-head">
        <div className="triage-title">
          <Icon.Sparkles style={{ width: 14, height: 14, color: "var(--accent)" }} /> AI 排查报告
        </div>
        <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-faint)" }}>由 deepseek-chat 生成 · 1.4s · 412 tokens</span>
        <div style={{ flex: 1 }} />
        <button className="icon-btn" onClick={onClose}><Icon.X /></button>
      </div>
      <div className="triage-body">
        <div className="triage-section">
          <div className="triage-section-label">问题</div>
          <p>
            <code style={{ fontFamily: "var(--font-mono)" }}>{failedNode.label}</code> 连续 3 次重试都遇到 <strong>HTTP 429 rate limited</strong>。
            对比这个 workflow 最近 30 次运行，<strong>有 4 次失败都是 Airtable 限流</strong>，集中在 07:00-08:00 区间——和 cron 触发的早晨高峰吻合。
          </p>
        </div>
        <div className="triage-section">
          <div className="triage-section-label">根因</div>
          <p>
            Airtable 的 free tier 限流是 5 req/sec/base。<code style={{ fontFamily: "var(--font-mono)" }}>{failedNode.label}</code> 是同步阻塞写，没有 backoff 策略，3 次重试间隔太密（1.2s / 2.4s），赶不上限流恢复窗口。
          </p>
        </div>
        <div className="triage-section">
          <div className="triage-section-label">建议修复</div>
          <div className="triage-fixes">
            <div className="triage-fix">
              <div className="triage-fix-icon"><Icon.Wrench /></div>
              <div>
                <div className="triage-fix-title">把 retry 改成指数退避 + 抖动</div>
                <div className="triage-fix-sub">2s → 5s → 13s，加 ±20% jitter。改 <code style={{ fontFamily: "var(--font-mono)" }}>handler.config.retry</code></div>
              </div>
              <button className="btn btn-xs btn-accent">Accept 修改</button>
            </div>
            <div className="triage-fix">
              <div className="triage-fix-icon"><Icon.Clock /></div>
              <div>
                <div className="triage-fix-title">把 cron 错峰到 07:35</div>
                <div className="triage-fix-sub">绕开你其他几个 workflow 在 07:00-07:30 的 burst</div>
              </div>
              <button className="btn btn-xs btn-accent">Accept 修改</button>
            </div>
            <div className="triage-fix">
              <div className="triage-fix-icon"><Icon.Layers /></div>
              <div>
                <div className="triage-fix-title">加一个 rate-limit handler 包一层</div>
                <div className="triage-fix-sub">新建 <code style={{ fontFamily: "var(--font-mono)" }}>hd_rate_limited_writer</code>，包装现在的 writer，做客户端限流</div>
              </div>
              <button className="btn btn-xs">让 AI 锻造</button>
            </div>
          </div>
        </div>
        <div className="triage-section">
          <div className="triage-section-label">下一步</div>
          <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
            <button className="btn btn-xs"><Icon.Refresh /> 用相同输入重跑</button>
            <button className="btn btn-xs"><Icon.Wrench /> 改输入后重跑</button>
            <button className="btn btn-xs"><Icon.Play /> 从 <code style={{ fontFamily: "var(--font-mono)", fontSize: 11 }}>{failedNode.label}</code> 继续</button>
            <button className="btn btn-xs btn-ghost"><Icon.Pin /> 加到 memory · 提醒下次注意</button>
          </div>
        </div>
      </div>
    </div>);

}

// ── Run diff (compare two runs) ─────────────────────────────────────────
function RunDiffPanel({ fr, onClose }) {
  const candidates = Forgify.flowruns.filter((f) => f.workflow === fr.workflow && f.id !== fr.id);
  const [other, setOther] = useExecState(candidates[0]);
  if (!other) {
    return (
      <div className="triage">
        <div className="triage-head">
          <div className="triage-title">
            <Icon.GitBranch style={{ width: 14, height: 14 }} /> 与历史 diff
          </div>
          <div style={{ flex: 1 }} />
          <button className="icon-btn" onClick={onClose}><Icon.X /></button>
        </div>
        <div className="triage-body"><div className="empty" style={{ padding: 24 }}><div className="sub">没有同 workflow 的其他 run 可对比</div></div></div>
      </div>);

  }
  const a = Forgify.flowrunDetails[fr.id] || Forgify.flowrunDetails.fr_8d2a;
  const b = Forgify.flowrunDetails[other.id] || Forgify.flowrunDetails.fr_8d26;
  const allIds = Array.from(new Set([...a.nodes.map((n) => n.id), ...b.nodes.map((n) => n.id)]));
  return (
    <div className="triage">
      <div className="triage-head">
        <div className="triage-title">
          <Icon.GitBranch style={{ width: 14, height: 14 }} /> Run Diff
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12 }}>
          <span className="cell-mono" style={{ color: "var(--status-success)" }}>A · {fr.id}</span>
          <span style={{ color: "var(--fg-faint)" }}>vs</span>
          <select value={other.id} onChange={(e) => setOther(candidates.find((c) => c.id === e.target.value))} className="cfg-input" style={{ width: 240 }}>
            {candidates.map((c) => <option key={c.id} value={c.id}>{c.id} · {c.status} · {fmtDuration(c.durationMs)}</option>)}
          </select>
        </div>
        <div style={{ flex: 1 }} />
        <button className="icon-btn" onClick={onClose}><Icon.X /></button>
      </div>
      <div className="triage-body">
        <table className="t" style={{ marginTop: 0 }}>
          <thead>
            <tr>
              <th>节点</th>
              <th>A: {fr.id}</th>
              <th>B: {other.id}</th>
              <th>结果</th>
            </tr>
          </thead>
          <tbody>
            {allIds.map((id) => {
              const na = a.nodes.find((n) => n.id === id);
              const nb = b.nodes.find((n) => n.id === id);
              const sameStatus = na?.status === nb?.status;
              const sameDur = Math.abs((na?.durationMs || 0) - (nb?.durationMs || 0)) < 500;
              const verdict = !na ? "B only" : !nb ? "A only" : sameStatus && sameDur ? "same" : "diff";
              return (
                <tr key={id}>
                  <td className="cell-mono">{na?.label || nb?.label || id}</td>
                  <td>
                    {na ? <><NodeStatusIcon status={na.status} /><span className="cell-mono" style={{ marginLeft: 6 }}>{fmtDuration(na.durationMs || 0)}</span></> : <span style={{ color: "var(--fg-faint)" }}>—</span>}
                  </td>
                  <td>
                    {nb ? <><NodeStatusIcon status={nb.status} /><span className="cell-mono" style={{ marginLeft: 6 }}>{fmtDuration(nb.durationMs || 0)}</span></> : <span style={{ color: "var(--fg-faint)" }}>—</span>}
                  </td>
                  <td>
                    {verdict === "same" ? <span className="badge muted">一致</span> :
                    verdict === "diff" ? <span className="badge warn"><span className="dot" />有差异</span> :
                    <span className="badge info">单边</span>}
                  </td>
                </tr>);

            })}
          </tbody>
        </table>
      </div>
    </div>);

}

// ── Approvals queue (kept from earlier) ──────────────────────────────────
function ApprovalsQueue() {
  const pending = Forgify.flowruns.filter((f) => f.status === "waiting_approval");
  if (pending.length === 0) {
    return (
      <div className="empty">
        <Icon.CheckCircle className="icon" />
        <div className="title">没有待批准的任务</div>
        <div className="sub">workflow 在 approval 节点暂停时会出现在这里</div>
      </div>);

  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: 8 }}>
      {pending.map((fr) =>
      <div key={fr.id} className="card" style={{ borderColor: "color-mix(in srgb, var(--status-warn) 30%, transparent)", background: "color-mix(in srgb, var(--status-warn) 4%, transparent)" }}>
          <div className="card-head">
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <div style={{ width: 28, height: 28, borderRadius: 6, background: "color-mix(in srgb, var(--status-warn) 12%, transparent)", color: "var(--status-warn)", display: "grid", placeItems: "center" }}>
                <Icon.Pause />
              </div>
              <div>
                <div className="card-title">{fr.workflow}</div>
                <div className="cell-mono" style={{ fontSize: 11, color: "var(--fg-faint)" }}>{fr.id} · 节点 {fr.nodes.done}/{fr.nodes.total}</div>
              </div>
            </div>
            {flowStatusBadge(fr.status)}
          </div>
          <div className="card-desc">
            等待人工确认：<strong>把本周训练汇总写入 Notion 草稿区</strong>。
            <pre className="code-block" style={{ marginTop: 8, fontSize: 11 }}>{`week:        2026-W20
avg_pace:    5'12"/km
total_climb: 412 m
avg_hr:      142 bpm
sessions:    4`}</pre>
          </div>
          <div className="card-foot">
            <span>由 cron · 30 7 * * 1 触发 · {relTime(fr.startedAt)}</span>
            <div style={{ display: "flex", gap: 6 }}>
              <button className="btn btn-xs btn-danger"><Icon.X /> 拒绝</button>
              <button className="btn btn-xs"><Icon.Pause /> 暂存</button>
              <button className="btn btn-xs btn-accent"><Icon.Check /> 批准并继续</button>
            </div>
          </div>
        </div>
      )}
    </div>);

}

// ── ExecuteView main ─────────────────────────────────────────────────────
function ExecuteView() {
  const [tab, setTab] = useExecState("runs");
  const [openRun, setOpenRun] = useExecState(null);
  const [filterWf, setFilterWf] = useExecState(null);
  const [query, setQuery] = useExecState("");
  const [statusFilter, setStatusFilter] = useExecState("all");

  if (openRun) return <FlowRunDetail fr={openRun} onBack={() => setOpenRun(null)} />;

  const filteredRuns = Forgify.flowruns.filter((f) => {
    if (filterWf && f.workflow !== filterWf) return false;
    if (statusFilter !== "all" && f.status !== statusFilter) return false;
    if (query) {
      const q = query.toLowerCase();
      if (!(f.workflow.toLowerCase().includes(q) || f.id.toLowerCase().includes(q) || (f.trigger || "").toLowerCase().includes(q))) return false;
    }
    return true;
  });

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Play /> 执行</div>
          <div className="page-subtitle">运行历史</div>
        </div>
        <div className="page-actions">
          <button className="btn btn-sm"><Icon.Refresh /> 刷新</button>
        </div>
      </div>

      <div className="page-toolbar">
        <div className="search-input" style={{ width: 320 }}>
          <Icon.Search className="icon" />
          <input
            placeholder="搜 workflow / run id / 触发源…"
            value={query}
            onChange={(e) => setQuery(e.target.value)} />
          
        </div>
        <div className="seg">
          {[
          ["all", "全部"],
          ["running", "运行中"],
          ["waiting_approval", "待批准"],
          ["failed", "失败"],
          ["completed", "完成"]].
          map(([k, l]) =>
          <button key={k} className={"seg-btn" + (statusFilter === k ? " is-active" : "")} onClick={() => setStatusFilter(k)}>{l}</button>
          )}
        </div>
        <div style={{ flex: 1 }} />
        {(filterWf || query || statusFilter !== "all") &&
        <button className="btn btn-xs btn-ghost" onClick={() => {setFilterWf(null);setQuery("");setStatusFilter("all");}}>
            <Icon.X /> 清除筛选
          </button>
        }
      </div>

      <div className="page-body" style={{ padding: "16px 32px", overflowY: "auto" }}>
        <KpiStrip />
        <h3 className="section-label" style={{ marginTop: 22 }}>
          {filterWf ? <>已过滤：<code style={{ fontFamily: "var(--font-mono)", color: "var(--accent)" }}>{filterWf}</code></> : "Workflow 历史"}
        </h3>
        <WorkflowHeatmap
          query={query}
          selected={filterWf}
          onPick={(wf) => setFilterWf(filterWf === wf ? null : wf)} />
        

        <div className="page-tabs" style={{ marginTop: 22, padding: 0, border: 0 }}>
          {[
          ["runs", "FlowRuns", filteredRuns.length],
          ["approvals", "待批准", Forgify.flowruns.filter((f) => f.status === "waiting_approval").length],
          ["triggers", "触发器", 4]].
          map(([k, l, c]) =>
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>{l}<span className="count">{c}</span></button>
          )}
        </div>

        <div style={{ marginTop: 10 }}>
          {tab === "runs" && <FlowRunsTable onOpen={setOpenRun} filter={filterWf} query={query} statusFilter={statusFilter} />}
          {tab === "approvals" && <ApprovalsQueue />}
          {tab === "triggers" &&
          <div className="card-grid" style={{ marginTop: 10 }}>
              {[
            { name: "30 7 * * 1", desc: "weekly-training-summary · 每周一 07:30", kind: "cron" },
            { name: "fsnotify ~/Downloads/invoices", desc: "invoice-intake · 新 PDF 触发", kind: "fsnotify" },
            { name: "manual:weekly-recap", desc: "Linear 周回顾 · 手动入口", kind: "manual" },
            { name: "webhook:strava-push", desc: "Strava push subscription", kind: "webhook" }].
            map((t, i) =>
            <div key={i} className="card">
                  <div className="card-head">
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <div style={{ width: 26, height: 26, borderRadius: 5, background: "var(--bg-elev-2)", border: "1px solid var(--border-soft)", display: "grid", placeItems: "center", color: "var(--fg-muted)" }}>
                        {t.kind === "cron" && <Icon.Clock style={{ width: 13, height: 13 }} />}
                        {t.kind === "fsnotify" && <Icon.Folder style={{ width: 13, height: 13 }} />}
                        {t.kind === "manual" && <Icon.Play style={{ width: 13, height: 13 }} />}
                        {t.kind === "webhook" && <Icon.Globe style={{ width: 13, height: 13 }} />}
                      </div>
                      <div className="cell-mono" style={{ fontSize: 12, color: "var(--fg-strong)" }}>{t.name}</div>
                    </div>
                    <span className="badge success"><span className="dot" />active</span>
                  </div>
                  <div className="card-desc">{t.desc}</div>
                </div>
            )}
            </div>
          }
        </div>
      </div>
    </div>);

}

window.ExecuteView = ExecuteView;
window.flowStatusBadge = flowStatusBadge;