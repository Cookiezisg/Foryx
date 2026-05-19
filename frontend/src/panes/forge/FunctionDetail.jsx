// FunctionDetail — current version full view + other-version split diff
// + VersionRail right rail. Pending versions surface Accept/Revert at
// the head.
//
// FunctionDetail —— 当前版本完整视图 + 其他版本分屏 diff + 右侧 VersionRail。

import { useMemo, useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { KindChip } from "../../components/shared/KindChip.jsx";
import { StatusBadge } from "../../components/shared/StatusBadge.jsx";
import { EntityRelMeta } from "../../components/shared/EntityRelMeta.jsx";
import { VersionRail, SplitDiff, CodeView } from "../../components/shared/VersionRail.jsx";
import { AskAiTrigger } from "../../components/shared/AskAiTrigger.jsx";
import { useFunction, useFunctionVersions, useAcceptFunction, useRevertFunction } from "../../api/forge.js";
import { useForgeProgress } from "../../sse/useForge.js";
import { useUIStore } from "../../store/ui.js";

export function FunctionDetail({ forge, onBack }) {
  const { data: fn = forge } = useFunction(forge.id);
  const { data: versions = [] } = useFunctionVersions(forge.id);
  const pushToast = useUIStore((s) => s.pushToast);
  const accept = useAcceptFunction();
  const revert = useRevertFunction();
  const forgeProgress = useForgeProgress((s) => s.active[`function:${forge.id}`]);

  const currentV = versions.find((v) => v.state === "current") || versions[0];
  const pendingV = versions.find((v) => v.state === "pending");

  const [selectedId, setSelectedId] = useState(null);
  const effectiveSelected = selectedId || pendingV?.id || currentV?.id;
  const selectedV = versions.find((v) => v.id === effectiveSelected) || currentV;
  const isViewingCurrent = selectedV?.id === currentV?.id;

  const onAccept = () => {
    accept.mutate(forge.id, {
      onSuccess: () => pushToast({ kind: "success", title: "Accepted", desc: fn.name }),
      onError: (e) => pushToast({ kind: "error", title: "Accept 失败", desc: e.message }),
    });
  };
  const onRevert = () => {
    revert.mutate(forge.id, {
      onSuccess: () => pushToast({ kind: "warn", title: "Reverted pending", desc: fn.name }),
      onError: (e) => pushToast({ kind: "error", title: "Revert 失败", desc: e.message }),
    });
  };

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <Button size="xs" variant="ghost" onClick={onBack}>
              <Icon.ChevronRight style={{ transform: "rotate(180deg)" }} /> 返回
            </Button>
            <span>·</span>
            <KindChip kind="function" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge.id}</span>
            {forgeProgress && forgeProgress.status === "running" && (
              <span className="badge streaming"><span className="dot" />锻造中</span>
            )}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{fn.name}</div>
            {!pendingV && <StatusBadge status={fn.status || "ready"} />}
          </div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{fn.desc || fn.description || ""}</span>
            <EntityRelMeta entityId={fn.id} />
          </div>
        </div>
        <div className="page-actions">
          {pendingV ? (
            <>
              <Button size="sm" variant="danger" onClick={onRevert}>
                <Icon.X /> Revert
              </Button>
              <Button size="sm" variant="accent" onClick={onAccept}>
                <Icon.Check /> Accept
              </Button>
            </>
          ) : (
            <>
              <Button size="sm"><Icon.Play /> 试跑</Button>
              <AskAiTrigger
                kind="function"
                entityId={fn.id}
                context={`Function · ${fn.name}`}
                suggestions={["把超时改成 60 秒", "失败重试 3 次", "加单元测试"]}
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
  if (!v) return <div className="empty" style={{ padding: 32 }}><div className="sub">没有可显示的版本</div></div>;
  return (
    <div className="fn-view">
      <h3 className="section-label" style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
        {v.label || v.versionLabel || v.id}
        {v.state === "current" && <span className="vr-badge vr-current">current</span>}
        {v.state === "pending" && <span className="vr-badge vr-pending"><Icon.Sparkles /> pending</span>}
      </h3>
      <FieldRow label="说明" value={
        <div style={{ lineHeight: 1.6 }}>
          {v.description || fn.description || <span style={{ color: "var(--fg-faint)" }}>无说明</span>}
        </div>
      } />
      {(v.schema?.inputs || v.inputs) && (
        <FieldRow label="输入" value={
          <code style={{ fontFamily: "var(--font-mono)" }}>{v.schema?.inputs || JSON.stringify(v.inputs)}</code>
        } />
      )}
      {(v.schema?.outputs || v.outputs) && (
        <FieldRow label="输出" value={
          <code style={{ fontFamily: "var(--font-mono)" }}>{v.schema?.outputs || JSON.stringify(v.outputs)}</code>
        } />
      )}
      {(v.runtime || v.sandbox) && (
        <FieldRow label="运行环境" value={
          <code style={{ fontFamily: "var(--font-mono)" }}>{v.runtime || v.sandbox}</code>
        } />
      )}

      <h4 className="section-label" style={{ marginTop: 20 }}>代码</h4>
      {v.code
        ? <CodeView src={v.code} />
        : <div className="empty" style={{ padding: 18 }}><div className="sub">该版本没有代码</div></div>}
    </div>
  );
}

function FunctionDiffView({ currentV, otherV, pendingV }) {
  const isPending = otherV?.id === pendingV?.id;
  const descChanged = (currentV?.description || "") !== (otherV?.description || "");
  const inA = currentV?.schema?.inputs, inB = otherV?.schema?.inputs;
  const outA = currentV?.schema?.outputs, outB = otherV?.schema?.outputs;
  const inputsChanged = inA !== inB;
  const outputsChanged = outA !== outB;
  const codeChanged = (currentV?.code || "") !== (otherV?.code || "");

  const total = [descChanged, inputsChanged, outputsChanged, codeChanged].filter(Boolean).length;

  if (!otherV || !currentV) {
    return <div className="empty" style={{ padding: 32 }}><div className="sub">缺少版本数据无法对比</div></div>;
  }

  return (
    <div className="fn-view">
      <h3 className="section-label" style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
        Diff · {currentV.label || "current"} ⇆ {otherV.label || otherV.id}
        {isPending && <span className="vr-badge vr-pending"><Icon.Sparkles /> pending</span>}
        <span style={{ color: "var(--fg-faint)", fontWeight: 400, textTransform: "none", letterSpacing: 0 }}>
          · {total} 处变更
        </span>
      </h3>

      {total === 0 && (
        <div style={{ padding: 24, color: "var(--fg-faint)", textAlign: "center" }}>两个版本内容完全一致</div>
      )}

      {descChanged && (
        <DiffSection label="说明" leftLabel={currentV.label} rightLabel={otherV.label}>
          <div className="fn-diff-2col">
            <div className="fn-diff-side">
              <div className="fn-diff-prose">{currentV.description || <span style={{ color: "var(--fg-faint)" }}>(空)</span>}</div>
            </div>
            <div className="fn-diff-side">
              <div className="fn-diff-prose">{otherV.description || <span style={{ color: "var(--fg-faint)" }}>(空)</span>}</div>
            </div>
          </div>
        </DiffSection>
      )}

      {(inputsChanged || outputsChanged) && (
        <DiffSection label="契约">
          <div className="fn-diff-2col">
            <div className="fn-diff-side">
              <div className="fn-diff-kv"><span className="k">输入</span><code>{inA}</code></div>
              <div className="fn-diff-kv"><span className="k">输出</span><code>{outA}</code></div>
            </div>
            <div className="fn-diff-side">
              <div className="fn-diff-kv"><span className="k">输入</span><code className={inputsChanged ? "is-diff" : ""}>{inB}</code></div>
              <div className="fn-diff-kv"><span className="k">输出</span><code className={outputsChanged ? "is-diff" : ""}>{outB}</code></div>
            </div>
          </div>
        </DiffSection>
      )}

      {codeChanged && (
        <DiffSection label="代码">
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
