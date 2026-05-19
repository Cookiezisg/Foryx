// HandlerDetail — current version multi-tab (Class / Config / Calls) +
// diff view + VersionRail.
//
// HandlerDetail —— Class / Config / Calls 多标签 + diff + VersionRail。

import { useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { KindChip } from "../../components/shared/KindChip.jsx";
import { StatusBadge } from "../../components/shared/StatusBadge.jsx";
import { EntityRelMeta } from "../../components/shared/EntityRelMeta.jsx";
import { VersionRail, SplitDiff, CodeView } from "../../components/shared/VersionRail.jsx";
import { AskAiTrigger } from "../../components/shared/AskAiTrigger.jsx";
import {
  useHandler, useHandlerVersions, useHandlerConfig, useAcceptHandler,
} from "../../api/forge.js";
import { useForgeProgress } from "../../sse/useForge.js";
import { useUIStore } from "../../store/ui.js";

export function HandlerDetail({ forge, onBack }) {
  const { data: hd = forge } = useHandler(forge.id);
  const { data: versions = [] } = useHandlerVersions(forge.id);
  const pushToast = useUIStore((s) => s.pushToast);
  const accept = useAcceptHandler();
  const progress = useForgeProgress((s) => s.active[`handler:${forge.id}`]);

  const currentV = versions.find((v) => v.state === "current") || versions[0];
  const pendingV = versions.find((v) => v.state === "pending");

  const [selectedId, setSelectedId] = useState(null);
  const effectiveSelected = selectedId || pendingV?.id || currentV?.id;
  const selectedV = versions.find((v) => v.id === effectiveSelected) || currentV;
  const isViewingCurrent = selectedV?.id === currentV?.id;

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <Button size="xs" variant="ghost" onClick={onBack}>← 返回</Button>
            <span>·</span>
            <KindChip kind="handler" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge.id}</span>
            {progress && progress.status === "running" && (
              <span className="badge streaming"><span className="dot" />锻造中</span>
            )}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{hd.name}</div>
            {!pendingV && <StatusBadge status={hd.status || "ready"} />}
          </div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{hd.desc || hd.description || ""}</span>
            <EntityRelMeta entityId={hd.id} />
          </div>
        </div>
        <div className="page-actions">
          {pendingV
            ? <Button size="sm" variant="accent" onClick={() => accept.mutate(forge.id, {
                onSuccess: () => pushToast({ kind: "success", title: "Accepted" }),
              })}>
                <Icon.Check /> Accept
              </Button>
            : <Button size="sm"><Icon.Play /> 试调用</Button>}
          <AskAiTrigger
            kind="handler"
            entityId={hd.id}
            context={`Handler · ${hd.name}`}
            suggestions={["给方法加批量写", "把同步改成异步"]}
          />
        </div>
      </div>

      <div className="vr-shell">
        <div className="vr-main" style={{ padding: 0 }}>
          {isViewingCurrent
            ? <HandlerFullView v={selectedV} hd={hd} />
            : <HandlerDiffView currentV={currentV} otherV={selectedV} pendingV={pendingV} />}
        </div>
        <VersionRail
          versions={versions}
          currentId={currentV?.id}
          pendingId={pendingV?.id}
          selectedId={effectiveSelected}
          onSelect={setSelectedId}
          onAccept={() => accept.mutate(forge.id)}
          onRevert={() => {}}
        />
      </div>
    </div>
  );
}

function HandlerFullView({ v, hd }) {
  const [tab, setTab] = useState("class");
  const { data: config } = useHandlerConfig(hd.id);

  if (!v) return null;
  const methods = v.methods || [];
  const [selectedMethod, setSelectedMethod] = useState(methods[0]);
  const method = methods.find((m) => m.name === selectedMethod?.name) || methods[0];

  return (
    <>
      <div className="page-tabs">
        {[["class", "Class"], ["config", "Config"], ["calls", "Call 历史"]].map(([k, l]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
            {l}
          </button>
        ))}
      </div>

      {tab === "class" && (
        <div className="hd-class">
          <aside className="hd-methods">
            <div className="hd-class-name">
              <Icon.Boxes style={{ width: 14, height: 14, marginRight: 6 }} />
              class
            </div>
            {methods.length === 0 && (
              <div style={{ padding: 16, fontSize: 12, color: "var(--fg-faint)" }}>该版本没有方法</div>
            )}
            {methods.map((m) => (
              <button
                key={m.name}
                className={"hd-method" + (method?.name === m.name ? " is-active" : "")}
                onClick={() => setSelectedMethod(m)}
              >
                <span style={{ color: "var(--fg-faint)", fontFamily: "var(--font-mono)", fontSize: 10 }}>fn</span>
                <span className="cell-mono">{m.name}</span>
              </button>
            ))}
          </aside>
          <main className="hd-method-detail">
            {method && (
              <>
                <div className="hd-method-sig">
                  <span style={{ color: "var(--fg-faint)", fontFamily: "var(--font-mono)" }}>def</span>{" "}
                  <span style={{ color: "var(--accent)", fontFamily: "var(--font-mono)", fontWeight: 600 }}>{method.name}</span>
                  <span style={{ fontFamily: "var(--font-mono)", color: "var(--fg-body)" }}>{method.sig || method.signature || ""}</span>
                </div>
                {method.desc && <div className="hd-method-desc">{method.desc}</div>}
                {method.body && <CodeView src={method.body} />}
              </>
            )}
          </main>
        </div>
      )}

      {tab === "config" && (
        <div style={{ padding: "20px 32px", display: "flex", flexDirection: "column", gap: 12, maxWidth: 600 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--fg-muted)" }}>
            <Icon.KeyRound style={{ width: 13, height: 13 }} /> Encrypted with AES-GCM · 仅本地存储
          </div>
          {config && Object.keys(config).length > 0
            ? Object.entries(config).map(([k, val]) => (
                <div key={k} className="cfg-row">
                  <div className="cfg-label">
                    {k}
                    {val?.secret && <span className="badge muted" style={{ marginLeft: 6 }}>secret</span>}
                  </div>
                  <div className="cfg-value">
                    <input type="text" className="cfg-input" value={val?.value || ""} readOnly />
                    <button className="icon-btn"><Icon.Copy /></button>
                  </div>
                </div>
              ))
            : <div className="empty" style={{ padding: 18 }}><div className="sub">还没有配置项</div></div>}
        </div>
      )}

      {tab === "calls" && (
        <div style={{ padding: "16px 32px" }}>
          <div className="empty" style={{ padding: 18 }}>
            <Icon.ListChecks className="icon" />
            <div className="title">Call 历史</div>
            <div className="sub">通过 handler 调用后这里会有记录（KPI + 表格 Phase 11 接入）</div>
          </div>
        </div>
      )}
    </>
  );
}

function HandlerDiffView({ currentV, otherV, pendingV }) {
  if (!currentV || !otherV) return <div className="empty"><div className="sub">缺少版本</div></div>;
  const isPending = otherV.id === pendingV?.id;
  const curMethods = new Map((currentV.methods || []).map((m) => [m.name, m]));
  const othMethods = new Map((otherV.methods || []).map((m) => [m.name, m]));
  const allNames = [...new Set([...curMethods.keys(), ...othMethods.keys()])];
  const changes = allNames.map((name) => {
    const a = curMethods.get(name);
    const b = othMethods.get(name);
    if (!a) return { name, kind: "added", a: null, b };
    if (!b) return { name, kind: "removed", a, b: null };
    if (a.sig !== b.sig || a.desc !== b.desc || a.body !== b.body) return { name, kind: "changed", a, b };
    return { name, kind: "same" };
  }).filter((c) => c.kind !== "same");

  return (
    <div className="fn-view" style={{ padding: "20px 28px" }}>
      <h3 className="section-label" style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
        Diff · {currentV.label || "current"} ⇆ {otherV.label || otherV.id}
        {isPending && <span className="vr-badge vr-pending"><Icon.Sparkles /> pending</span>}
        <span style={{ color: "var(--fg-faint)", fontWeight: 400 }}>· {changes.length} 处方法变更</span>
      </h3>
      {changes.length === 0 && <div style={{ padding: 24, color: "var(--fg-faint)", textAlign: "center" }}>方法层面一致</div>}
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {changes.map((c, i) => (
          <div key={i} className={"hd-method-diff hd-method-diff-" + c.kind}>
            <div className="hd-method-diff-head-btn" style={{ cursor: "default" }}>
              <span className={"vr-badge vr-cmp-" + c.kind}>
                {c.kind === "added" ? "新增" : c.kind === "removed" ? "删除" : "修改"}
              </span>
              <code style={{ fontFamily: "var(--font-mono)", color: "var(--accent)" }}>{c.name}</code>
            </div>
            {c.kind === "changed" && c.a.body && c.b.body && c.a.body !== c.b.body && (
              <div className="hd-method-body-pane">
                <SplitDiff leftLabel="current" rightLabel={isPending ? "pending" : "other"} leftSrc={c.a.body} rightSrc={c.b.body} />
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
