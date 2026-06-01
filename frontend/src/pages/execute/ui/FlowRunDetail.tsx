// FlowRunDetail — header + DAG + node inspector + Gantt timeline.
// Triage panel + run-diff panel surface as inline collapsibles above
// the DAG when toggled.
//
// FlowRunDetail —— 头部 + DAG + 节点 inspector + Gantt；triage / diff
// 面板 inline 折叠。

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { Badge } from "@shared/ui/Badge";
import { RelTime } from "@shared/ui/RelTime.tsx";
import { EntityRelMeta } from "@/widgets/entity-rel-meta/EntityRelMeta.tsx";
import { BottomSheet } from "@shared/ui/BottomSheet.tsx";
import { ApprovalBanner } from "./ApprovalBanner.tsx";
import type { FlowRun, FlowRunNode, FlowRunStatus, FlowRunNodeStatus } from "@entities/flowrun";
import {
  useFlowRun, useFlowRunNodes, useCancelFlowRun, useApproveNode,
  useRejectNode, useTriageFlowRun,
} from "@entities/flowrun";
import { useToastStore } from "@shared/ui/toastStore";

const STATUS_KIND: Record<string, string> = {
  running: "streaming",
  completed: "success",
  failed: "error",
  waiting_approval: "warn",
  paused: "info",
  cancelled: "muted",
};

// Runtime node shape — FlowRunNode plus display-only fields computed/aliased by the API.
// Status is widened to include legacy variant values ("ok", "fail", "skip", "completed").
interface NodeRuntime extends Omit<FlowRunNode, "status"> {
  status: FlowRunNodeStatus | "ok" | "completed" | "fail" | "skip" | "waiting" | "wait";
  x?: number;
  y?: number;
  kind?: string;
  label?: string;
  durationMs?: number;
  startedMs?: number;
  dependsOn?: string[];
  parents?: string[];
  log?: Array<{ time?: string; level?: string; msg?: string }>;
  error?: string;
}

// Runtime flowrun shape — FlowRun plus server-aliased fields.
interface FlowRunRuntime extends FlowRun {
  workflow?: string;
  trigger?: string;
}

function StatusBadge({ status }: { status: FlowRunStatus | string }) {
  const { t } = useTranslation("execute");
  const label = t(`status.${status === "waiting_approval" ? "waitingApproval" : status}`, status);
  const kind = (STATUS_KIND[status] ?? "muted") as "success" | "error" | "warn" | "info" | "streaming" | "muted";
  return <Badge kind={kind}>{label}</Badge>;
}

function fmtDuration(ms: number | null | undefined): string {
  if (ms == null) return "—";
  if (ms < 1000) return ms + "ms";
  if (ms < 60_000) return (ms / 1000).toFixed(1) + "s";
  const m = Math.floor(ms / 60_000);
  const s = Math.round((ms % 60_000) / 1000);
  return `${m}m ${s}s`;
}

interface FlowRunDetailProps {
  runId: string;
  onBack: () => void;
  onOpenChat?: (convId: string) => void;
}

export function FlowRunDetail({ runId, onBack, onOpenChat }: FlowRunDetailProps) {
  const { t } = useTranslation("execute");
  const { data: fr } = useFlowRun(runId);
  const { data: nodes = [] } = useFlowRunNodes(runId);
  const cancel = useCancelFlowRun();
  const triage = useTriageFlowRun();
  const pushToast = useToastStore((s) => s.pushToast);

  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const shellRef = useRef<HTMLDivElement>(null);

  if (!fr) return <div className="empty" style={{ padding: 48 }}><div className="sub">{t("detail.loading")}</div></div>;

  const frAny = fr as FlowRunRuntime;
  const nodesAny = nodes as NodeRuntime[];
  const okCount   = nodesAny.filter((n) => n.status === "ok"      || n.status === "completed").length;
  const failCount = nodesAny.filter((n) => n.status === "fail"    || n.status === "failed").length;
  const skipCount = nodesAny.filter((n) => n.status === "skip"    || n.status === "pending").length;
  const selected = nodesAny.find((n) => n.id === selectedNodeId) || nodesAny[0];

  const onTriage = async () => {
    try {
      const res = await triage.mutateAsync(runId);
      const cid = (res as Record<string, unknown> | null)?.conversationId as string | undefined;
      if (cid) {
        onOpenChat?.(cid);
        pushToast({ kind: "success", title: t("detail.toast.triageSuccess") });
      }
    } catch (e) {
      pushToast({ kind: "error", title: t("detail.toast.triageFail"), desc: e instanceof Error ? e.message : undefined });
    }
  };

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <Button size="xs" variant="ghost" onClick={onBack}>{t("detail.back")}</Button>
            <span>·</span>
            <span className="cell-mono">{fr.id}</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{frAny.workflow || fr.workflowId}</div>
            <StatusBadge status={fr.status} />
          </div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>
              {t("detail.triggeredBy")} <code style={{ fontFamily: "var(--font-mono)" }}>{frAny.trigger || frAny.triggerKind || "?"}</code> · <RelTime ts={frAny.startedAt} />
            </span>
            <span style={{ color: "var(--status-success)" }}> · {okCount} ok</span>
            {failCount > 0 && <span style={{ color: "var(--status-error)" }}> · {failCount} fail</span>}
            {skipCount > 0 && <span style={{ color: "var(--fg-faint)" }}> · {skipCount} skip</span>}
            <EntityRelMeta entityId={fr.id} kind="flowrun" />
          </div>
        </div>
        <div className="page-actions">
          {fr.status === "running" && (
            <Button size="sm" variant="danger" onClick={() => cancel.mutate(runId)}>
              <Icon.StopCircle /> {t("detail.cancelBtn")}
            </Button>
          )}
          {fr.status === "failed" && (
            <Button size="sm" onClick={onTriage}>
              <Icon.Sparkles /> {t("detail.triageBtn")}
            </Button>
          )}
          <Button size="sm"><Icon.Refresh /> {t("detail.rerunBtn")}</Button>
        </div>
      </div>

      <ApprovalBanner runId={runId} />

      <div className="fr-shell" ref={shellRef}>
        <FlowRunDag nodes={nodesAny} selected={selected?.id} onSelect={setSelectedNodeId} />
        <BottomSheet
          open={!!selected}
          onClose={() => setSelectedNodeId(null)}
          title={selected ? (selected.label || selected.id) : ""}
          height={340}
          anchorRef={shellRef}
        >
          {selected && <NodeInspectorBody node={selected} fr={frAny} />}
        </BottomSheet>
      </div>
      <GanttTimeline nodes={nodesAny} />
    </div>
  );
}

function nodeStatusIcon(status: string | undefined) {
  if (status === "ok" || status === "completed") return <Icon.Check style={{ width: 12, height: 12, color: "var(--status-success)" }} />;
  if (status === "fail" || status === "failed") return <Icon.X style={{ width: 12, height: 12, color: "var(--status-error)" }} />;
  if (status === "running") return <span className="spinner" style={{ width: 12, height: 12, borderColor: "color-mix(in srgb, var(--accent) 30%, transparent)", borderTopColor: "var(--accent)" }} />;
  if (status === "waiting" || status === "wait") return <Icon.Clock style={{ width: 12, height: 12, color: "var(--status-warn)" }} />;
  return <span style={{ width: 8, height: 8, borderRadius: "50%", border: "1.5px dashed var(--fg-faint)" }} />;
}

function FlowRunDag({ nodes, selected, onSelect }: { nodes: NodeRuntime[]; selected: string | undefined; onSelect: (id: string) => void }) {
  const { t } = useTranslation("execute");
  if (!nodes || nodes.length === 0) {
    return <div className="empty" style={{ padding: 32, flex: 1 }}><div className="sub">{t("detail.dag.empty")}</div></div>;
  }
  // Lay out nodes by their layer (if absent, simple stack).
  const positioned = nodes.map((n, i) => ({
    ...n,
    x: typeof n.x === "number" ? n.x : 220 * (i % 4),
    y: typeof n.y === "number" ? n.y : 100 * Math.floor(i / 4),
  }));
  const byId: Record<string, typeof positioned[number]> = Object.fromEntries(positioned.map((n) => [n.id, n]));
  const edges = nodes.flatMap((n) => (n.dependsOn || n.parents || []).map((from: string) => ({ from, to: n.id })));

  return (
    <div className="fr-dag">
      <svg className="fr-dag-edges" width="100%" height="100%">
        <defs>
          <marker id="fr-arr" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M0 0 L10 5 L0 10 z" fill="var(--border-strong)" />
          </marker>
        </defs>
        {edges.map((e, i) => {
          const a = byId[e.from], b = byId[e.to];
          if (!a || !b) return null;
          const sx = a.x + 92, sy = a.y + 60;
          const ex = b.x + 92, ey = b.y;
          const dy = Math.max(30, (ey - sy) / 2);
          const d = `M ${sx} ${sy} C ${sx} ${sy + dy}, ${ex} ${ey - dy}, ${ex} ${ey}`;
          return <path key={i} d={d} fill="none" stroke="var(--border-strong)" strokeWidth="1.4" markerEnd="url(#fr-arr)" />;
        })}
      </svg>
      {positioned.map((n) => (
        <div
          key={n.id}
          className={"fr-dag-node fr-status-" + (n.status || "pending") + (selected === n.id ? " is-selected" : "")}
          style={{ left: n.x, top: n.y }}
          onClick={() => onSelect(n.id)}
          title={n.id}
        >
          <div className="fr-dag-node-head">
            {nodeStatusIcon(n.status)}
            <span className="cell-mono" style={{ fontSize: 10, color: "var(--fg-muted)" }}>{n.kind || "?"}</span>
          </div>
          <div className="fr-dag-node-title">{n.label || n.id}</div>
          <div className="fr-dag-node-sub">
            {n.durationMs != null ? fmtDuration(n.durationMs) : n.status === "running" ? t("detail.dag.nodeRunning") : n.status === "pending" ? t("detail.dag.nodeWaiting") : "—"}
          </div>
        </div>
      ))}
    </div>
  );
}

function NodeInspectorBody({ node, fr }: { node: NodeRuntime; fr: FlowRunRuntime }) {
  const { t } = useTranslation("execute");
  return (
    <div className="fr-inspector-content">
      <div className="fr-inspector-meta-row">
        {nodeStatusIcon(node.status)}
        {node.kind && <span className="kind-chip fn">{node.kind}</span>}
        {node.durationMs != null && <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-muted)" }}>{fmtDuration(node.durationMs)}</span>}
        {node.error && <span className="fr-inspector-error">{node.error}</span>}
      </div>
      <div className="fr-inspector-body">
        {node.input != null && (
          <div className="fr-section">
            <div className="fr-section-label">Input</div>
            <pre className="code-block" style={{ fontSize: 11 }}>{prettyJSON(node.input)}</pre>
          </div>
        )}
        {node.output != null && (
          <div className="fr-section">
            <div className="fr-section-label">Output</div>
            <pre className="code-block" style={{ fontSize: 11 }}>{prettyJSON(node.output)}</pre>
          </div>
        )}
        {Array.isArray(node.log) && node.log.length > 0 && (
          <div className="fr-section">
            <div className="fr-section-label">Log</div>
            <div className="fr-log">
              {node.log.map((l, i) => (
                <div key={i} className={"fr-log-row level-" + (l.level || "info")}>
                  <span className="fr-log-time">{l.time}</span>
                  <span className="fr-log-level">{l.level || "info"}</span>
                  <span className="fr-log-msg">{l.msg}</span>
                </div>
              ))}
            </div>
          </div>
        )}
        {node.input == null && node.output == null && (!node.log || node.log.length === 0) && (
          <div className="empty" style={{ padding: 20 }}>
            <div className="sub" style={{ color: "var(--fg-faint)" }}>{t("detail.inspector.empty")}</div>
          </div>
        )}
      </div>
    </div>
  );
}

function prettyJSON(v: unknown): string {
  try { return JSON.stringify(v, null, 2); } catch { return String(v); }
}

function GanttTimeline({ nodes }: { nodes: NodeRuntime[] }) {
  const { t } = useTranslation("execute");
  if (!nodes || nodes.length === 0) return null;
  const total = Math.max(...nodes.map((n) => (n.startedMs ?? 0) + (n.durationMs ?? 0)), 1);
  return (
    <div className="fr-gantt">
      <div className="fr-gantt-head">
        <span className="fr-gantt-title">{t("detail.gantt.title")}</span>
        <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>
          {t("detail.gantt.totalDuration", { duration: fmtDuration(total) })}
        </span>
      </div>
      <div className="fr-gantt-body">
        {nodes.map((n) => {
          const start = n.startedMs ?? 0;
          const dur = n.durationMs ?? 0;
          const left = (start / total * 100).toFixed(1) + "%";
          const width = Math.max(0.5, (dur / total) * 100).toFixed(1) + "%";
          const color = n.status === "fail" || n.status === "failed" ? "var(--status-error)"
            : n.status === "running" ? "var(--accent)"
            : n.status === "ok" || n.status === "completed" ? "var(--status-success)"
            : "var(--fg-faint)";
          return (
            <div key={n.id} className="fr-gantt-row">
              <div className="fr-gantt-label">{n.label || n.id}</div>
              <div className="fr-gantt-track">
                {n.startedMs != null
                  ? <div className={"fr-gantt-bar status-" + (n.status || "pending")} style={{ left, width, background: color }} />
                  : <div className="fr-gantt-pending">{t("detail.gantt.notRun")}</div>}
              </div>
              <div className="fr-gantt-dur cell-mono">{dur ? fmtDuration(dur) : "—"}</div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
