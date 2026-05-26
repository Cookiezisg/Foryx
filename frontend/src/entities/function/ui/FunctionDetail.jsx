// FunctionDetail — current version full view + other-version split diff
// + VersionRail right rail. Pending versions surface Accept/Revert at
// the head.
//
// FunctionDetail —— 当前版本完整视图 + 其他版本分屏 diff + 右侧 VersionRail。

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@/components/primitives/Icon.jsx";
import { Button } from "@/components/primitives/Button.jsx";
import { KindChip } from "@shared/ui/KindChip.jsx";
import { StatusBadge } from "@shared/ui/StatusBadge.jsx";
import { EntityRelMeta } from "@/widgets/entity-rel-meta/EntityRelMeta.jsx";
import { VersionRail, SplitDiff, CodeView } from "@/widgets/version-rail/VersionRail.jsx";
import { AskAiTrigger } from "@/widgets/ask-ai-trigger/AskAiTrigger.jsx";
import { RunDrawer } from "@entities/flowrun/ui/RunDrawer.jsx";
import { useFunction, useFunctionVersions } from "@/api/forge.js";
import { useForgeProgress } from "@shared/model";
import { useForgeReview } from "@features/forge-review";

export function FunctionDetail({ forge, onBack }) {
  const { t } = useTranslation(["forge", "common"]);
  const { data: fn = forge } = useFunction(forge.id);
  const { data: versions = [] } = useFunctionVersions(forge.id);
  const { accept: onAccept, revert: onRevert } = useForgeReview("function", forge.id, fn.name);
  const forgeProgress = useForgeProgress((s) => s.active[`function:${forge.id}`]);

  const currentV = versions.find((v) => v.state === "current") || versions[0];
  const pendingV = versions.find((v) => v.state === "pending");

  const [selectedId, setSelectedId] = useState(null);
  const [runOpen, setRunOpen] = useState(false);
  const effectiveSelected = selectedId || pendingV?.id || currentV?.id;
  const selectedV = versions.find((v) => v.id === effectiveSelected) || currentV;
  const isViewingCurrent = selectedV?.id === currentV?.id;

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <Button size="xs" variant="ghost" onClick={onBack}>
              <Icon.ChevronRight style={{ transform: "rotate(180deg)" }} /> {t("common:back")}
            </Button>
            <span>·</span>
            <KindChip kind="function" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge.id}</span>
            {forgeProgress && forgeProgress.status === "running" && (
              <span className="badge streaming"><span className="dot" />{t("detail.forging")}</span>
            )}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{fn.name}</div>
            {!pendingV && <StatusBadge status={fn.status || "ready"} />}
          </div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{fn.desc || fn.description || ""}</span>
            <EntityRelMeta entityId={fn.id} kind="function" />
          </div>
        </div>
        <div className="page-actions">
          {pendingV ? (
            <>
              <Button size="sm" variant="danger" onClick={onRevert}>
                <Icon.X /> {t("detail.revert")}
              </Button>
              <Button size="sm" variant="accent" onClick={onAccept}>
                <Icon.Check /> {t("detail.accept")}
              </Button>
            </>
          ) : (
            <>
              <Button size="sm" onClick={() => setRunOpen(true)}><Icon.Play /> {t("function.runBtn")}</Button>
              <AskAiTrigger
                kind="function"
                entityId={fn.id}
                context={`Function · ${fn.name}`}
                suggestions={t("function.aiSuggestions", { returnObjects: true })}
              />
            </>
          )}
        </div>
      </div>

      <div className="vr-shell">
        <div className="vr-main">
          {isViewingCurrent
            ? <FunctionFullView v={selectedV} fn={fn} />
            : <FunctionDiffView currentV={currentV} otherV={selectedV} pendingV={pendingV} />
          }
        </div>
        <VersionRail
          versions={versions}
          currentId={currentV?.id}
          pendingId={pendingV?.id}
          selectedId={effectiveSelected}
          onSelect={setSelectedId}
          onAccept={onAccept}
          onRevert={onRevert}
        />
      </div>
      <RunDrawer open={runOpen} onClose={() => setRunOpen(false)} kind="function" entity={fn} />
    </div>
  );
}

function FieldRow({ label, value }) {
  return (
    <div className="fn-field-row">
      <div className="fn-field-label">{label}</div>
      <div className="fn-field-value">{value}</div>
    </div>
  );
}

function FunctionFullView({ v, fn }) {
  const { t } = useTranslation("forge");
  if (!v) return <div className="empty" style={{ padding: 32 }}><div className="sub">{t("function.noVersion")}</div></div>;
  return (
    <div className="fn-view">
      <h3 className="section-label" style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
        {v.label || v.versionLabel || v.id}
        {v.state === "current" && <span className="vr-badge vr-current">{t("detail.badgeCurrent")}</span>}
        {v.state === "pending" && <span className="vr-badge vr-pending"><Icon.Sparkles /> {t("detail.badgePending")}</span>}
      </h3>
      <FieldRow label={t("function.fieldLabel.description")} value={
        <div style={{ lineHeight: 1.6 }}>
          {v.description || fn.description || <span style={{ color: "var(--fg-faint)" }}>—</span>}
        </div>
      } />
      {(v.schema?.inputs || v.inputs) && (
        <FieldRow label={t("function.fieldLabel.inputs")} value={
          <code style={{ fontFamily: "var(--font-mono)" }}>{v.schema?.inputs || JSON.stringify(v.inputs)}</code>
        } />
      )}
      {(v.schema?.outputs || v.outputs) && (
        <FieldRow label={t("function.fieldLabel.outputs")} value={
          <code style={{ fontFamily: "var(--font-mono)" }}>{v.schema?.outputs || JSON.stringify(v.outputs)}</code>
        } />
      )}
      {(v.runtime || v.sandbox) && (
        <FieldRow label={t("function.fieldLabel.runtime")} value={
          <code style={{ fontFamily: "var(--font-mono)" }}>{v.runtime || v.sandbox}</code>
        } />
      )}

      <h4 className="section-label" style={{ marginTop: 20 }}>{t("function.fieldLabel.code")}</h4>
      {v.code
        ? <CodeView src={v.code} />
        : <div className="empty" style={{ padding: 18 }}><div className="sub">{t("function.noCode")}</div></div>}
    </div>
  );
}

function FunctionDiffView({ currentV, otherV, pendingV }) {
  const { t } = useTranslation("forge");
  const isPending = otherV?.id === pendingV?.id;
  const descChanged = (currentV?.description || "") !== (otherV?.description || "");
  const inA = currentV?.schema?.inputs, inB = otherV?.schema?.inputs;
  const outA = currentV?.schema?.outputs, outB = otherV?.schema?.outputs;
  const inputsChanged = inA !== inB;
  const outputsChanged = outA !== outB;
  const codeChanged = (currentV?.code || "") !== (otherV?.code || "");

  const total = [descChanged, inputsChanged, outputsChanged, codeChanged].filter(Boolean).length;

  if (!otherV || !currentV) {
    return <div className="empty" style={{ padding: 32 }}><div className="sub">{t("function.noVersionForDiff")}</div></div>;
  }

  return (
    <div className="fn-view">
      <h3 className="section-label" style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
        Diff · {currentV.label || "current"} ⇆ {otherV.label || otherV.id}
        {isPending && <span className="vr-badge vr-pending"><Icon.Sparkles /> pending</span>}
        <span style={{ color: "var(--fg-faint)", fontWeight: 400, textTransform: "none", letterSpacing: 0 }}>
          · {t("function.changes", { count: total })}
        </span>
      </h3>

      {total === 0 && (
        <div style={{ padding: 24, color: "var(--fg-faint)", textAlign: "center" }}>{t("function.identical")}</div>
      )}

      {descChanged && (
        <DiffSection label={t("function.fieldLabel.description")} leftLabel={currentV.label} rightLabel={otherV.label}>
          <div className="fn-diff-2col">
            <div className="fn-diff-side">
              <div className="fn-diff-prose">{currentV.description || <span style={{ color: "var(--fg-faint)" }}>{t("function.fieldLabel.empty")}</span>}</div>
            </div>
            <div className="fn-diff-side">
              <div className="fn-diff-prose">{otherV.description || <span style={{ color: "var(--fg-faint)" }}>{t("function.fieldLabel.empty")}</span>}</div>
            </div>
          </div>
        </DiffSection>
      )}

      {(inputsChanged || outputsChanged) && (
        <DiffSection label={t("function.fieldLabel.contract")}>
          <div className="fn-diff-2col">
            <div className="fn-diff-side">
              <div className="fn-diff-kv"><span className="k">{t("function.fieldLabel.inputs")}</span><code>{inA}</code></div>
              <div className="fn-diff-kv"><span className="k">{t("function.fieldLabel.outputs")}</span><code>{outA}</code></div>
            </div>
            <div className="fn-diff-side">
              <div className="fn-diff-kv"><span className="k">{t("function.fieldLabel.inputs")}</span><code className={inputsChanged ? "is-diff" : ""}>{inB}</code></div>
              <div className="fn-diff-kv"><span className="k">{t("function.fieldLabel.outputs")}</span><code className={outputsChanged ? "is-diff" : ""}>{outB}</code></div>
            </div>
          </div>
        </DiffSection>
      )}

      {codeChanged && (
        <DiffSection label={t("function.fieldLabel.code")}>
          <SplitDiff
            leftLabel={currentV.label + " · current"}
            rightLabel={otherV.label + (isPending ? " · pending" : "")}
            leftSrc={currentV.code || ""}
            rightSrc={otherV.code || ""}
          />
        </DiffSection>
      )}
    </div>
  );
}

function DiffSection({ label, children }) {
  return (
    <div className="fn-diff-section">
      <div className="fn-diff-section-label">{label}</div>
      {children}
    </div>
  );
}
