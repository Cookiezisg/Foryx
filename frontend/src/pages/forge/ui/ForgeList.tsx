// ForgeList — unified table over function/handler/workflow. Three
// REST hooks run in parallel; results merge into one rows array sorted
// by updatedAt desc. Tab filters by kind. Click row → opens detail.
//
// ForgeList —— trinity 三域合并表；tabs 按类型过滤；行点击进 detail。

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { KindChip } from "../../../shared/ui/KindChip.tsx";
import { StatusBadge } from "../../../shared/ui/StatusBadge.tsx";
import { RelTime } from "../../../shared/ui/RelTime.tsx";
import { ActionMenu } from "../../../widgets/action-menu/ActionMenu.tsx";
import { useFunctions, useDeleteFunction } from "@entities/function";
import { useHandlers, useDeleteHandler } from "@entities/handler";
import { useWorkflows, useDeleteWorkflow } from "@entities/workflow";
import { useForgeProgress } from "@shared/model";
import { useToastStore } from "@shared/ui/toastStore";
import { RunDrawer } from "./RunDrawer.tsx";
import { useForgeBatchDelete } from "@features/forge-review";

interface ForgeListProps {
  onOpen: (entity: any) => void;
  onOpenExecute?: (id: string) => void;
}

export function ForgeList({ onOpen, onOpenExecute }: ForgeListProps) {
  const { t } = useTranslation(["forge", "common"]);
  const [tab, setTab] = useState("all");
  const [q, setQ] = useState("");
  const [selected, setSelected] = useState(new Set());

  const TABS = [
    { key: "all",       label: t("list.tabs.all") },
    { key: "function",  label: "Functions" },
    { key: "handler",   label: "Handlers" },
    { key: "workflow",  label: "Workflows" },
  ];

  const { data: functions = [] } = useFunctions();
  const { data: handlers = [] } = useHandlers();
  const { data: workflows = [] } = useWorkflows();
  const activeForge = useForgeProgress((s) => s.active);
  const deleteFn = useDeleteFunction();
  const deleteHd = useDeleteHandler();
  const deleteWf = useDeleteWorkflow();
  const pushToast = useToastStore((s) => s.pushToast);
  const { batchDelete } = useForgeBatchDelete();
  const [runTarget, setRunTarget] = useState(null);

  const rows: any[] = useMemo(() => {
    const fns = (functions as any[]).map((x) => ({ ...x, kind: "function" }));
    const hds = (handlers as any[]).map((x) => ({ ...x, kind: "handler" }));
    const wfs = (workflows as any[]).map((x) => ({ ...x, kind: "workflow" }));
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

  const toggle = (id: string) => {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id); else next.add(id);
    setSelected(next);
  };
  const clearSel = () => setSelected(new Set());

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Hammer /> {t("list.title")}</div>
          <div className="page-subtitle">Function / Handler / Workflow</div>
        </div>
        <div className="page-actions">
          <Button size="sm" onClick={() => pushToast({
            kind: "info",
            title: t("list.toast.newTitle"),
            desc: t("list.toast.newDesc"),
            duration: 6000,
          })}>
            <Icon.Plus /> {t("list.newBtn")}
          </Button>
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
            <span className="count">{(counts as Record<string, number>)[key] ?? 0}</span>
          </button>
        ))}
      </div>

      <div className="page-toolbar">
        <div className="search-input">
          <Icon.Search className="icon" />
          <input
            placeholder={t("list.searchPlaceholder")}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <div style={{ flex: 1 }} />
        <span style={{ fontSize: 11, color: "var(--fg-faint)", fontFamily: "var(--font-mono)" }}>
          {t("list.countInfo", { count: filtered.length })}
        </span>
      </div>

      {selected.size > 0 && (
        <div className="batch-bar">
          <span className="batch-bar-count">{t("list.batch.selected", { count: selected.size })}</span>
          <Button size="xs" variant="ghost" onClick={clearSel}>{t("list.batch.clearSel")}</Button>
          <div className="batch-bar-buttons">
            <Button size="xs" variant="danger" onClick={() => {
              const picked = filtered.filter((f) => selected.has(f.id));
              batchDelete(picked, clearSel);
            }}>
              <Icon.Trash /> {t("common:delete")}
            </Button>
          </div>
        </div>
      )}

      <div className="page-body" style={{ padding: 0 }}>
        {filtered.length === 0 ? (
          <div className="empty" style={{ padding: 48 }}>
            <Icon.Hammer className="icon" />
            <div className="title">{t("list.empty.title", { kindPart: tab === "all" ? "" : t("list.kindNames." + tab) + " " })}</div>
            <div className="sub">{t("list.empty.sub")}</div>
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
                <th>{t("list.table.name")}</th>
                <th>{t("list.table.kind")}</th>
                <th>{t("list.table.version")}</th>
                <th>{t("list.table.status")}</th>
                <th>{t("list.table.updatedAt")}</th>
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
                        ? <span className="badge streaming"><span className="dot" />{t("list.forging")}</span>
                        : <StatusBadge status={f.status || "ready"} />}
                    </td>
                    <td onClick={() => onOpen(f)}>
                      {(f.updatedAt || f.createdAt) ? <RelTime ts={f.updatedAt || f.createdAt} /> : "—"}
                    </td>
                    <td className="col-tight">
                      <ActionMenu
                        items={[
                          { label: f.kind === "handler" ? t("list.actionMenu.testCall") : f.kind === "workflow" ? t("list.actionMenu.trigger") : t("list.actionMenu.testRun"),
                            icon: Icon.Play,
                            onClick: () => setRunTarget(f) },
                          { label: t("list.actionMenu.viewDetail"), icon: Icon.GitBranch, onClick: () => onOpen(f) },
                          "divider",
                          { label: t("common:delete"), icon: Icon.Trash, danger: true, onClick: () => {
                            if (!confirm(t("list.deleteConfirm", { kind: f.kind, name: f.name }))) return;
                            const m = f.kind === "function" ? deleteFn
                                   : f.kind === "handler"  ? deleteHd
                                   :                          deleteWf;
                            m.mutate(f.id, {
                              onSuccess: () => pushToast({ kind: "success", title: t("list.toast.deleteSuccess"), desc: f.name }),
                              onError: (e) => pushToast({ kind: "error", title: t("list.toast.deleteFail"), desc: e.message }),
                            });
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
      <RunDrawer
        open={!!runTarget}
        onClose={() => setRunTarget(null)}
        kind={runTarget?.kind}
        entity={runTarget || {}}
        onOpenExecute={onOpenExecute}
      />
    </div>
  );
}
