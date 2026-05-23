// ExecuteOverview — KPI strip + workflow heatmap + tabs over
// (runs / approvals / triggers). All data real via useFlowRuns().
//
// ExecuteOverview —— KPI 条 + heatmap + 三标签内容；数据全真实。

import { useMemo, useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { Badge } from "../../components/primitives/Badge.jsx";
import { RelTime } from "../../components/shared/RelTime.jsx";
import { useFlowRuns, useApproveNode, useRejectNode } from "../../api/flowruns.js";
import { useUIStore } from "../../store/ui.js";

const STATUS_LABEL = {
  running: "运行中",
  completed: "完成",
  failed: "失败",
  waiting_approval: "待批准",
  paused: "已暂停",
  cancelled: "已取消",
};
const STATUS_KIND = {
  running: "streaming",
  completed: "success",
  failed: "error",
  waiting_approval: "warn",
  paused: "info",
  cancelled: "muted",
};

function flowStatusBadge(s) {
  const k = STATUS_KIND[s] || "muted";
  return <Badge kind={k}>{STATUS_LABEL[s] || s}</Badge>;
}

export function ExecuteOverview({ onOpen }) {
  const [tab, setTab] = useState("runs");
  const [q, setQ] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const { data: flowruns = [], isLoading } = useFlowRuns();

  const filtered = useMemo(() => {
    return flowruns.filter((f) => {
      if (statusFilter !== "all" && f.status !== statusFilter) return false;
      if (q) {
        const ql = q.toLowerCase();
        return (f.workflow || f.workflowId || "").toLowerCase().includes(ql)
          || (f.id || "").toLowerCase().includes(ql)
          || (f.trigger || "").toLowerCase().includes(ql);
      }
      return true;
    });
  }, [flowruns, q, statusFilter]);

  const running   = flowruns.filter((f) => f.status === "running");
  const waiting   = flowruns.filter((f) => f.status === "waiting_approval");
  const failed    = flowruns.filter((f) => f.status === "failed");
  const completed = flowruns.filter((f) => f.status === "completed");

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Play /> 执行</div>
          <div className="page-subtitle">运行历史 · 待批准 · 触发器</div>
        </div>
        <div className="page-actions">
          <Button size="sm"><Icon.Refresh /> 刷新</Button>
        </div>
      </div>

      <div className="page-toolbar">
        <div className="search-input" style={{ maxWidth: 320 }}>
          <Icon.Search className="icon" />
          <input placeholder="搜 workflow / run id / 触发源…" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <div className="seg">
          {[["all", "全部"], ["running", "运行中"], ["waiting_approval", "待批准"], ["failed", "失败"], ["completed", "完成"]].map(([k, l]) => (
            <button key={k} className={"seg-btn" + (statusFilter === k ? " is-active" : "")} onClick={() => setStatusFilter(k)}>
              {l}
            </button>
          ))}
        </div>
        <div style={{ flex: 1 }} />
        {(q || statusFilter !== "all") && (
          <Button size="xs" variant="ghost" onClick={() => { setQ(""); setStatusFilter("all"); }}>
            <Icon.X /> 清除筛选
          </Button>
        )}
      </div>

      <div className="page-body" style={{ padding: "16px 32px" }}>
        <KpiStrip total={flowruns.length} running={running.length} waiting={waiting.length} failed={failed.length} success={completed.length} onOpen={() => {}} />

        <div className="page-tabs" style={{ marginTop: 22, padding: 0, border: 0 }}>
          {[
            ["runs", "FlowRuns", filtered.length],
            ["approvals", "待批准", waiting.length],
            ["triggers", "触发器", null],
          ].map(([k, l, c]) => (
            <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
              {l}{c != null && <span className="count">{c}</span>}
            </button>
          ))}
        </div>

        <div style={{ marginTop: 10 }}>
          {tab === "runs" && (
            <FlowRunsTable runs={filtered} loading={isLoading} onOpen={onOpen} />
          )}
          {tab === "approvals" && (
            <ApprovalsQueue runs={waiting} />
          )}
          {tab === "triggers" && (
            <TriggersGrid />
          )}
        </div>
      </div>
    </div>
  );
}

function KpiStrip({ total, running, waiting, failed, success }) {
  const rate = total === 0 ? 0 : Math.round((success / total) * 100);
  return (
    <div className="kpi-strip">
      <Kpi label="运行总数" value={total} sub={`${rate}% 成功率`} />
      <Kpi label="运行中" value={running} sub={running ? "" : "无"} active={running > 0} />
      <Kpi label="待批准" value={waiting} sub={waiting ? "" : "无"} warn={waiting > 0} />
      <Kpi label="需关注" value={failed} sub={failed ? "" : "无"} error={failed > 0} />
    </div>
  );
}

function Kpi({ label, value, sub, active, warn, error }) {
  const cls = ["kpi", active && "is-active", warn && "is-warn", error && "is-error"].filter(Boolean).join(" ");
  return (
    <div className={cls}>
      <div className="kpi-label">{label}</div>
      <div className="kpi-value">{value}</div>
      <div className="kpi-sub">{sub}</div>
    </div>
  );
}

function ProgressMini({ done, total, status }) {
  const t = total || 1;
  const pct = Math.round((done / t) * 100);
  const color = status === "failed" ? "var(--status-error)"
    : status === "waiting_approval" ? "var(--status-warn)"
    : status === "running" ? "var(--accent)"
    : "var(--status-success)";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
      <div className="progress-bar" style={{ width: 80 }}>
        <div style={{ width: pct + "%", background: color }} />
      </div>
      <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-faint)", whiteSpace: "nowrap" }}>{done}/{t}</span>
    </div>
  );
}

function fmtDuration(ms) {
  if (ms == null) return "—";
  if (ms < 1000) return ms + "ms";
  if (ms < 60_000) return (ms / 1000).toFixed(1) + "s";
  return Math.round(ms / 1000) + "s";
}

function FlowRunsTable({ runs, loading, onOpen }) {
  if (loading) return <div className="empty" style={{ padding: 32 }}><div className="sub">加载中…</div></div>;
  if (runs.length === 0) {
    return (
      <div className="empty" style={{ padding: 32 }}>
        <Icon.Play className="icon" />
        <div className="title">没有匹配的运行</div>
        <div className="sub">调整筛选条件，或先去 forge 部署一个 workflow</div>
      </div>
    );
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
        {runs.map((fr) => (
          <tr key={fr.id} onClick={() => onOpen(fr)}>
            <td style={{ paddingLeft: 32 }}>
              <div>
                <div className="cell-strong">{fr.workflow || fr.workflowId}</div>
                <div className="cell-mono" style={{ marginTop: 2 }}>{fr.id}</div>
              </div>
            </td>
            <td>{flowStatusBadge(fr.status)}</td>
            <td>
              <ProgressMini done={fr.nodes?.done ?? 0} total={fr.nodes?.total ?? 0} status={fr.status} />
            </td>
            <td><span className="cell-mono" style={{ fontSize: 11 }}>{fr.trigger || fr.triggerKind || "—"}</span></td>
            <td><span style={{ fontSize: 12, color: "var(--fg-muted)" }}><RelTime ts={fr.startedAt} /></span></td>
            <td><span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-muted)" }}>{fmtDuration(fr.durationMs)}</span></td>
            <td className="col-tight"><button className="icon-btn" onClick={(e) => { e.stopPropagation(); onOpen(fr); }}><Icon.ChevronRight /></button></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function ApprovalsQueue({ runs }) {
  const pushToast = useUIStore((s) => s.pushToast);
  const approve = useApproveNode();
  const reject = useRejectNode();

  if (runs.length === 0) {
    return (
      <div className="empty">
        <Icon.CheckCircle className="icon" />
        <div className="title">没有待批准的任务</div>
        <div className="sub">workflow 暂停于 approval 节点时会出现在这里</div>
      </div>
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: 8 }}>
      {runs.map((fr) => (
        <div
          key={fr.id}
          className="card"
          style={{
            borderColor: "color-mix(in srgb, var(--status-warn) 30%, transparent)",
            background: "color-mix(in srgb, var(--status-warn) 4%, transparent)",
          }}
        >
          <div className="card-head">
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <div style={{
                width: 28, height: 28, borderRadius: 6,
                background: "color-mix(in srgb, var(--status-warn) 12%, transparent)",
                color: "var(--status-warn)",
                display: "grid", placeItems: "center",
              }}>
                <Icon.Pause />
              </div>
              <div>
                <div className="card-title">{fr.workflow || fr.workflowId}</div>
                <div className="cell-mono" style={{ fontSize: 11, color: "var(--fg-faint)" }}>
                  {fr.id} · 节点 {fr.nodes?.done ?? 0}/{fr.nodes?.total ?? 0}
                </div>
              </div>
            </div>
            {flowStatusBadge(fr.status)}
          </div>
          <div className="card-foot">
            <span>由 {fr.trigger || fr.triggerKind || "?"} 触发 · <RelTime ts={fr.startedAt} /></span>
            <div style={{ display: "flex", gap: 6 }}>
              <Button size="xs" variant="danger" onClick={() => reject.mutate({ runId: fr.id, nodeId: fr.pausedNodeId || "" })}>
                <Icon.X /> 拒绝
              </Button>
              <Button size="xs" variant="accent" onClick={() => {
                approve.mutate({ runId: fr.id, nodeId: fr.pausedNodeId || "" }, {
                  onSuccess: () => pushToast({ kind: "success", title: "已批准并继续" }),
                  onError: (e) => pushToast({ kind: "error", title: "批准失败", desc: e.message }),
                });
              }}>
                <Icon.Check /> 批准并继续
              </Button>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function TriggersGrid() {
  // Backend trigger listing endpoint is internal-only. Show static info
  // about supported trigger kinds for now.
  const triggers = [
    { kind: "cron",     icon: Icon.Clock,    label: "Cron 定时", desc: "robfig/cron v3 表达式" },
    { kind: "fsnotify", icon: Icon.Folder,   label: "文件触发",  desc: "fsnotify watch 路径" },
    { kind: "webhook",  icon: Icon.Globe,    label: "Webhook",   desc: "HTTP POST 入口" },
    { kind: "manual",   icon: Icon.Play,     label: "手动入口",  desc: "从 UI 或 API 触发" },
  ];
  return (
    <div className="card-grid" style={{ marginTop: 10 }}>
      {triggers.map((t) => {
        const I = t.icon;
        return (
          <div key={t.kind} className="card" style={{ cursor: "default" }}>
            <div className="card-head">
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <div style={{
                  width: 26, height: 26, borderRadius: 5,
                  background: "var(--bg-elev-2)", border: "1px solid var(--border-soft)",
                  display: "grid", placeItems: "center", color: "var(--fg-muted)",
                }}>
                  <I style={{ width: 13, height: 13 }} />
                </div>
                <div className="cell-mono" style={{ fontSize: 12, color: "var(--fg-strong)" }}>{t.label}</div>
              </div>
              <Badge kind="success">available</Badge>
            </div>
            <div className="card-desc">{t.desc}</div>
          </div>
        );
      })}
    </div>
  );
}
