// HandlerDetail — current version multi-tab (Class / Config / Calls) +
// diff view + VersionRail.
//
// HandlerDetail —— Class / Config / Calls 多标签 + diff + VersionRail。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@/components/primitives/Icon.jsx";
import { Button } from "@/components/primitives/Button.jsx";
import { KindChip } from "@shared/ui/KindChip.jsx";
import { StatusBadge } from "@shared/ui/StatusBadge.jsx";
import { EntityRelMeta } from "@/widgets/entity-rel-meta/EntityRelMeta.jsx";
import { VersionRail, SplitDiff, CodeView } from "@/widgets/version-rail/VersionRail.jsx";
import { AskAiTrigger } from "@/widgets/ask-ai-trigger/AskAiTrigger.jsx";
import { PaneCollapseToggle } from "@shared/ui/PaneCollapseToggle.jsx";
import { RunDrawer } from "@entities/flowrun/ui/RunDrawer.jsx";
import {
  useHandler, useHandlerVersions, useHandlerConfig,
} from "@/api/forge.js";
import { useForgeProgress } from "@shared/model";
import { useCollapsible } from "@/hooks/useCollapsible.js";
import { useForgeReview } from "@features/forge-review";

export function HandlerDetail({ forge, onBack }) {
  const { t } = useTranslation(["forge", "common"]);
  const { data: hd = forge } = useHandler(forge.id);
  const { data: versions = [] } = useHandlerVersions(forge.id);
  const { accept: onAccept, reject: onReject } = useForgeReview("handler", forge.id);
  const progress = useForgeProgress((s) => s.active[`handler:${forge.id}`]);

  const currentV = versions.find((v) => v.state === "current") || versions[0];
  const pendingV = versions.find((v) => v.state === "pending");

  const [selectedId, setSelectedId] = useState(null);
  const [runOpen, setRunOpen] = useState(false);
  const currentMethods = currentV?.methods || [];
  const effectiveSelected = selectedId || pendingV?.id || currentV?.id;
  const selectedV = versions.find((v) => v.id === effectiveSelected) || currentV;
  const isViewingCurrent = selectedV?.id === currentV?.id;

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <Button size="xs" variant="ghost" onClick={onBack}>← {t("common:back")}</Button>
            <span>·</span>
            <KindChip kind="handler" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge.id}</span>
            {progress && progress.status === "running" && (
              <span className="badge streaming"><span className="dot" />{t("detail.forging")}</span>
            )}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{hd.name}</div>
            {!pendingV && <StatusBadge status={hd.status || "ready"} />}
          </div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{hd.desc || hd.description || ""}</span>
            <EntityRelMeta entityId={hd.id} kind="handler" />
          </div>
        </div>
        <div className="page-actions">
          {pendingV ? (
            <>
              <Button size="sm" variant="danger" onClick={onReject}>
                <Icon.X /> {t("detail.revert")}
              </Button>
              <Button size="sm" variant="accent" onClick={onAccept}>
                <Icon.Check /> {t("detail.accept")}
              </Button>
            </>
          ) : (
            <Button size="sm" onClick={() => setRunOpen(true)}><Icon.Play /> {t("handler.runBtn")}</Button>
          )}
          <AskAiTrigger
            kind="handler"
            entityId={hd.id}
            context={`Handler · ${hd.name}`}
            suggestions={t("handler.aiSuggestions", { returnObjects: true })}
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
          onAccept={onAccept}
          onRevert={onReject}
        />
      </div>
      <RunDrawer
        open={runOpen}
        onClose={() => setRunOpen(false)}
        kind="handler"
        entity={{ ...hd, methods: currentMethods }}
      />
    </div>
  );
}

function HandlerFullView({ v, hd }) {
  const { t } = useTranslation("forge");
  const [tab, setTab] = useState("class");
  const { data: config } = useHandlerConfig(hd.id);
  const [methodsOpen, toggleMethods] = useCollapsible("handler-methods", true);

  if (!v) return null;
  const methods = v.methods || [];
  const [selectedMethod, setSelectedMethod] = useState(methods[0]);
  const method = methods.find((m) => m.name === selectedMethod?.name) || methods[0];

  return (
    <>
      <div className="page-tabs">
        {[["class", "Class"], ["config", "Config"], ["calls", t("handler.tabs.calls")]].map(([k, l]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
            {l}
          </button>
        ))}
      </div>

      {tab === "class" && (
        <div className={"hd-class pane-collapse-host" + (methodsOpen ? "" : " is-methods-collapsed")}>
          {methodsOpen && (
            <aside className="hd-methods">
              <div className="hd-class-name">
                <Icon.Boxes style={{ width: 14, height: 14, marginRight: 6 }} />
                class
                <button className="icon-btn" title={t("handler.collapseMethodList")} onClick={toggleMethods} style={{ marginLeft: "auto" }}>
                  <Icon.ChevronRight style={{ transform: "rotate(180deg)" }} />
                </button>
              </div>
              {methods.length === 0 && (
                <div style={{ padding: 16, fontSize: 12, color: "var(--fg-faint)" }}>{t("handler.noMethods")}</div>
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
          )}
          {!methodsOpen && <PaneCollapseToggle onClick={toggleMethods} title={t("handler.expandMethodList")} />}
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
            <Icon.KeyRound style={{ width: 13, height: 13 }} /> {t("handler.config.encrypted")}
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
            : <div className="empty" style={{ padding: 18 }}><div className="sub">{t("handler.noConfig")}</div></div>}
        </div>
      )}

      {tab === "calls" && (
        <div style={{ padding: "16px 32px" }}>
          <div className="empty" style={{ padding: 18 }}>
            <Icon.ListChecks className="icon" />
            <div className="title">{t("handler.tabs.calls")}</div>
            <div className="sub">{t("handler.tabs.callsPlaceholder")}</div>
          </div>
        </div>
      )}
    </>
  );
}

function HandlerDiffView({ currentV, otherV, pendingV }) {
  const { t } = useTranslation("forge");
  if (!currentV || !otherV) return <div className="empty"><div className="sub">{t("handler.noVersionForDiff")}</div></div>;
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
        <span style={{ color: "var(--fg-faint)", fontWeight: 400 }}>· {t("handler.methodChanges", { count: changes.length })}</span>
      </h3>
      {changes.length === 0 && <div style={{ padding: 24, color: "var(--fg-faint)", textAlign: "center" }}>{t("handler.methodsIdentical")}</div>}
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {changes.map((c, i) => (
          <div key={i} className={"hd-method-diff hd-method-diff-" + c.kind}>
            <div className="hd-method-diff-head-btn" style={{ cursor: "default" }}>
              <span className={"vr-badge vr-cmp-" + c.kind}>
                {c.kind === "added" ? t("handler.changeKind.added") : c.kind === "removed" ? t("handler.changeKind.removed") : t("handler.changeKind.changed")}
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
