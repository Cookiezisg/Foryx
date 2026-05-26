// ExecuteOverview — KPI strip + workflow heatmap + tabs over
// (runs / approvals / triggers). All data real via useFlowRuns().
//
// ExecuteOverview —— KPI 条 + heatmap + 三标签内容；数据全真实。

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { Badge } from "@shared/ui/Badge";
import { RelTime } from "../../../shared/ui/RelTime.tsx";
import { useFlowRuns, useApproveNode, useRejectNode } from "@entities/flowrun";
import { useToastStore } from "@shared/ui/toastStore";

const STATUS_KIND = {
  running: "streaming",
  completed: "success",
  failed: "error",
  waiting_approval: "warn",
  paused: "info",
  cancelled: "muted",
};

function FlowStatusBadge({ status }: { status: any }) {
  const { t } = useTranslation("execute");
  const k = (STATUS_KIND as Record<string, string>)[status] || "muted";
  const label = t(`status.${
    status === "waiting_approval" ? "waitingApproval" : status
  }`, status);
  return <Badge kind={k as any}>{label as any}</Badge>;
}

interface ExecuteOverviewProps {
  onOpen: (id: string) => void;
}

export function ExecuteOverview({ onOpen }: ExecuteOverviewProps) {
  const { t } = useTranslation("execute");
  const [tab, setTab] = useState("runs");
  const [q, setQ] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const { data: flowrunsRaw = [], isLoading } = useFlowRuns();
  const flowruns = flowrunsRaw as any[];

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
          <div className="page-title"><Icon.Play /> {t("overview.title")}</div>
          <div className="page-subtitle">{t("overview.subtitle")}</div>
        </div>
        <div className="page-actions">
          <Button size="sm"><Icon.Refresh /> {t("overview.refreshBtn")}</Button>
        </div>
      </div>

      <div className="page-toolbar">
        <div className="search-input" style={{ maxWidth: 320 }}>
          <Icon.Search className="icon" />
          <input placeholder={t("overview.searchPlaceholder")} value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <div className="seg">
          {[
            ["all",              t("overview.statusFilter.all")],
            ["running",          t("overview.statusFilter.running")],
            ["waiting_approval", t("overview.statusFilter.waitingApproval")],
            ["failed",           t("overview.statusFilter.failed")],
            ["completed",        t("overview.statusFilter.completed")],
          ].map(([k, l]) => (
            <button key={k} className={"seg-btn" + (statusFilter === k ? " is-active" : "")} onClick={() => setStatusFilter(k as string)}>
              {l}
            </button>
          ))}
        </div>
        <div style={{ flex: 1 }} />
        {(q || statusFilter !== "all") && (
          <Button size="xs" variant="ghost" onClick={() => { setQ(""); setStatusFilter("all"); }}>
            <Icon.X /> {t("overview.clearFilter")}
          </Button>
        )}
      </div>

      <div className="page-body" style={{ padding: "16px 32px" }}>
        <KpiStrip total={flowruns.length} running={running.length} waiting={waiting.length} failed={failed.length} success={completed.length} onOpen={() => {}} />

        <div className="page-tabs" style={{ marginTop: 22, padding: 0, border: 0 }}>
          {[
            ["runs",      t("overview.tabs.runs"),      filtered.length],
            ["approvals", t("overview.tabs.approvals"), waiting.length],
            ["triggers",  t("overview.tabs.triggers"),  null],
          ].map(([k, l, c]) => (
            <button key={k as string} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k as string)}>
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

function KpiStrip({ total, running, waiting, failed, success, onOpen }: any) {
  const { t } = useTranslation("execute");
  const rate = total === 0 ? 0 : Math.round((success / total) * 100);
  return (
    <div className="kpi-strip">
      <Kpi label={t("overview.kpi.total")}          value={total}   sub={t("overview.kpi.successRate", { rate })} />
      <Kpi label={t("overview.kpi.running")}        value={running} sub={running ? "" : t("overview.kpi.none")} active={running > 0} />
      <Kpi label={t("overview.kpi.waiting")}        value={waiting} sub={waiting ? "" : t("overview.kpi.none")} warn={waiting > 0} />
      <Kpi label={t("overview.kpi.needsAttention")} value={failed}  sub={failed  ? "" : t("overview.kpi.none")} error={failed > 0} />
    </div>
  );
}

function Kpi({ label, value, sub, active, warn, error }: any) {
  const cls = ["kpi", active && "is-active", warn && "is-warn", error && "is-error"].filter(Boolean).join(" ");
  return (
    <div className={cls}>
      <div className="kpi-label">{label}</div>
      <div className="kpi-value">{value}</div>
      <div className="kpi-sub">{sub}</div>
    </div>
  );
}

function ProgressMini({ done, total, status }: { done: any; total: any; status: any }) {
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

function fmtDuration(ms: any) {
  if (ms == null) return "—";
  if (ms < 1000) return ms + "ms";
  if (ms < 60_000) return (ms / 1000).toFixed(1) + "s";
  return Math.round(ms / 1000) + "s";
}

function FlowRunsTable({ runs, loading, onOpen }: { runs: any[]; loading: boolean; onOpen: (fr: any) => void }) {
  const { t } = useTranslation("execute");
  if (loading) return <div className="empty" style={{ padding: 32 }}><div className="sub">{t("overview.runs.loading")}</div></div>;
  if (runs.length === 0) {
    return (
      <div className="empty" style={{ padding: 32 }}>
        <Icon.Play className="icon" />
        <div className="title">{t("overview.runs.empty.title")}</div>
        <div className="sub">{t("overview.runs.empty.sub")}</div>
      </div>
    );
  }
  return (
    <table className="t">
      <thead>
        <tr>
          <th style={{ paddingLeft: 32 }}>Workflow</th>
          <th>{t("overview.runs.cols.status")}</th>
          <th>{t("overview.runs.cols.nodes")}</th>
          <th>{t("overview.runs.cols.trigger")}</th>
          <th>{t("overview.runs.cols.startedAt")}</th>
          <th>{t("overview.runs.cols.duration")}</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {runs.map((fr: any) => (
          <tr key={fr.id} onClick={() => onOpen(fr)}>
            <td style={{ paddingLeft: 32 }}>
              <div>
                <div className="cell-strong">{fr.workflow || fr.workflowId}</div>
                <div className="cell-mono" style={{ marginTop: 2 }}>{fr.id}</div>
              </div>
            </td>
            <td><FlowStatusBadge status={fr.status} /></td>
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

function ApprovalsQueue({ runs }: { runs: any[] }) {
  const { t } = useTranslation("execute");
  const pushToast = useToastStore((s) => s.pushToast);
  const approve = useApproveNode();
  const reject = useRejectNode();

  if (runs.length === 0) {
    return (
      <div className="empty">
        <Icon.CheckCircle className="icon" />
        <div className="title">{t("overview.approvals.empty.title")}</div>
        <div className="sub">{t("overview.approvals.empty.sub")}</div>
      </div>
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: 8 }}>
      {runs.map((fr: any) => (
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
                  {fr.id} · {t("overview.approvals.nodeCount", { done: fr.nodes?.done ?? 0, total: fr.nodes?.total ?? 0 })}
                </div>
              </div>
            </div>
            <FlowStatusBadge status={fr.status} />
          </div>
          <div className="card-foot">
            <span>{t("overview.approvals.triggeredBy", { trigger: fr.trigger || fr.triggerKind || "?" })} · <RelTime ts={fr.startedAt} /></span>
            <div style={{ display: "flex", gap: 6 }}>
              <Button size="xs" variant="danger" onClick={() => reject.mutate({ runId: fr.id, nodeId: fr.pausedNodeId || "" })}>
                <Icon.X /> {t("overview.approvals.rejectBtn")}
              </Button>
              <Button size="xs" variant="accent" onClick={() => {
                approve.mutate({ runId: fr.id, nodeId: fr.pausedNodeId || "" }, {
                  onSuccess: () => pushToast({ kind: "success", title: t("overview.approvals.toast.approveSuccess") }),
                  onError: (e) => pushToast({ kind: "error", title: t("overview.approvals.toast.approveFail"), desc: e.message }),
                });
              }}>
                <Icon.Check /> {t("overview.approvals.approveBtn")}
              </Button>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function TriggersGrid() {
  const { t } = useTranslation("execute");
  // Backend trigger listing endpoint is internal-only. Show static info
  // about supported trigger kinds for now.
  const triggers = [
    { kind: "cron",     icon: Icon.Clock,  label: t("overview.triggers.cron.label"),    desc: t("overview.triggers.cron.desc") },
    { kind: "fsnotify", icon: Icon.Folder, label: t("overview.triggers.fsnotify.label"), desc: t("overview.triggers.fsnotify.desc") },
    { kind: "webhook",  icon: Icon.Globe,  label: "Webhook",                             desc: t("overview.triggers.webhook.desc") },
    { kind: "manual",   icon: Icon.Play,   label: t("overview.triggers.manual.label"),   desc: t("overview.triggers.manual.desc") },
  ];
  return (
    <div className="card-grid" style={{ marginTop: 10 }}>
      {triggers.map((trigger) => {
        const I = trigger.icon;
        return (
          <div key={trigger.kind} className="card" style={{ cursor: "default" }}>
            <div className="card-head">
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <div style={{
                  width: 26, height: 26, borderRadius: 5,
                  background: "var(--bg-elev-2)", border: "1px solid var(--border-soft)",
                  display: "grid", placeItems: "center", color: "var(--fg-muted)",
                }}>
                  <I style={{ width: 13, height: 13 }} />
                </div>
                <div className="cell-mono" style={{ fontSize: 12, color: "var(--fg-strong)" }}>{trigger.label}</div>
              </div>
              <Badge kind="success">available</Badge>
            </div>
            <div className="card-desc">{trigger.desc}</div>
          </div>
        );
      })}
    </div>
  );
}
