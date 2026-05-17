/* eslint-disable react/prop-types */
// Function detail — VersionRail (right) + main area.
//   - Default selected = pending (if exists), else current.
//   - Selecting current  → view full info (desc / schema / code).
//   - Selecting any other version → split diff (current ⇆ that) of each changed field.

const { useState: useFnState } = React;

// ── Python-flavored code view (reused by FullView + DiffView) ──────────
function CodeView({ src }) {
  const KEYS = new Set(["def", "return", "for", "in", "if", "else", "elif", "from", "import", "class", "is", "not", "None", "True", "False", "and", "or", "lambda", "with", "as"]);
  const BUILTINS = new Set(["len", "sum", "range", "list", "dict", "tuple", "set", "str", "int", "float", "print"]);
  const lines = (src || "").split("\n");
  return (
    <pre className="codeview">
      {lines.map((line, i) => (
        <div key={i} className="codeview-row">
          <span className="codeview-ln">{i + 1}</span>
          <span className="codeview-line">
            {line.split(/(\s+|[(),:.\[\]'"])/g).map((tok, j) => {
              if (tok.startsWith("'") || tok.startsWith('"')) return <span key={j} className="tok-str">{tok}</span>;
              if (tok.startsWith("#")) return <span key={j} className="tok-com">{tok}</span>;
              if (KEYS.has(tok)) return <span key={j} className="tok-kw">{tok}</span>;
              if (BUILTINS.has(tok)) return <span key={j} className="tok-bi">{tok}</span>;
              if (/^\d+(\.\d+)?$/.test(tok)) return <span key={j} className="tok-num">{tok}</span>;
              return <span key={j}>{tok}</span>;
            })}
          </span>
        </div>
      ))}
    </pre>
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

function FunctionFullView({ v, runs, sandbox }) {
  return (
    <div className="fn-view">
      <h3 className="section-label" style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
        {v.label}
        {v.state === "current" && <span className="vr-badge vr-current">current</span>}
        {v.state === "pending" && <span className="vr-badge vr-pending"><Icon.Sparkles /> pending</span>}
        {v.state === "archived" && <span className="vr-badge" style={{ background: "var(--bg-elev-2)", color: "var(--fg-muted)" }}>archived</span>}
      </h3>

      <FieldRow label="说明" value={<div style={{ lineHeight: 1.6 }}>{v.description || <span style={{ color: "var(--fg-faint)" }}>无说明</span>}</div>} />
      <FieldRow label="输入" value={<code style={{ fontFamily: "var(--font-mono)" }}>{v.schema?.inputs}</code>} />
      <FieldRow label="输出" value={<code style={{ fontFamily: "var(--font-mono)" }}>{v.schema?.outputs}</code>} />
      <FieldRow label="运行环境" value={<code style={{ fontFamily: "var(--font-mono)" }}>{sandbox}</code>} />

      <h4 className="section-label" style={{ marginTop: 20 }}>代码</h4>
      <CodeView src={v.code} />

      {runs && runs.length > 0 && (
        <>
          <h4 className="section-label" style={{ marginTop: 20 }}>最近试跑</h4>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {runs.map((r, i) => (
              <div key={i} style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--fg-muted)" }}>
                <span style={{ width: 6, height: 6, borderRadius: "50%", background: r.status === "ok" ? "var(--status-success)" : "var(--status-error)" }} />
                <span style={{ flex: 1, fontFamily: "var(--font-mono)" }}>{r.at} · {r.input}</span>
                <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-faint)" }}>{r.duration}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function FunctionDiffView({ currentV, otherV, pendingV }) {
  const isPending = otherV.id === pendingV?.id;
  const descChanged = (currentV.description || "") !== (otherV.description || "");
  const inputsChanged = currentV.schema?.inputs !== otherV.schema?.inputs;
  const outputsChanged = currentV.schema?.outputs !== otherV.schema?.outputs;
  const codeChanged = (currentV.code || "") !== (otherV.code || "");

  const changedCount = [descChanged, inputsChanged, outputsChanged, codeChanged].filter(Boolean).length;

  return (
    <div className="fn-view">
      <h3 className="section-label" style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
        Diff · {currentV.label} ⇆ {otherV.label}
        {isPending && <span className="vr-badge vr-pending"><Icon.Sparkles /> pending</span>}
        <span style={{ color: "var(--fg-faint)", fontWeight: 400, textTransform: "none", letterSpacing: 0 }}>· {changedCount} 处变更</span>
      </h3>

      {!descChanged && !inputsChanged && !outputsChanged && !codeChanged && (
        <div style={{ padding: 24, color: "var(--fg-faint)", textAlign: "center" }}>两个版本内容完全一致</div>
      )}

      {descChanged && (
        <div className="fn-diff-section">
          <div className="fn-diff-section-label">说明</div>
          <div className="fn-diff-2col">
            <div className="fn-diff-side">
              <div className="fn-diff-side-head">A · {currentV.label} (current)</div>
              <div className="fn-diff-prose">{currentV.description || <span style={{ color: "var(--fg-faint)" }}>(空)</span>}</div>
            </div>
            <div className="fn-diff-side">
              <div className="fn-diff-side-head">B · {otherV.label}{isPending ? " (pending)" : ""}</div>
              <div className="fn-diff-prose">{otherV.description || <span style={{ color: "var(--fg-faint)" }}>(空)</span>}</div>
            </div>
          </div>
        </div>
      )}

      {(inputsChanged || outputsChanged) && (
        <div className="fn-diff-section">
          <div className="fn-diff-section-label">契约</div>
          <div className="fn-diff-2col">
            <div className="fn-diff-side">
              <div className="fn-diff-side-head">A · {currentV.label}</div>
              <div className="fn-diff-kv"><span className="k">输入</span><code>{currentV.schema?.inputs}</code></div>
              <div className="fn-diff-kv"><span className="k">输出</span><code>{currentV.schema?.outputs}</code></div>
            </div>
            <div className="fn-diff-side">
              <div className="fn-diff-side-head">B · {otherV.label}</div>
              <div className="fn-diff-kv"><span className="k">输入</span><code className={inputsChanged ? "is-diff" : ""}>{otherV.schema?.inputs}</code></div>
              <div className="fn-diff-kv"><span className="k">输出</span><code className={outputsChanged ? "is-diff" : ""}>{otherV.schema?.outputs}</code></div>
            </div>
          </div>
        </div>
      )}

      {codeChanged && (
        <div className="fn-diff-section">
          <div className="fn-diff-section-label">代码</div>
          <SplitDiff
            leftLabel={currentV.label + " · current"}
            rightLabel={otherV.label + (isPending ? " · pending" : " · archived")}
            leftSrc={currentV.code || ""}
            rightSrc={otherV.code || ""}
          />
        </div>
      )}
    </div>
  );
}

function FunctionDetail({ forge, onBack }) {
  const detail = Forgify.functionDetails[forge.id] || Forgify.functionDetails.fn_aggregate_week;
  const versions = detail.versions;
  const currentV = versions.find(v => v.state === "current") || versions[0];
  const pendingV = versions.find(v => v.state === "pending");
  const [selectedId, setSelectedId] = useFnState(pendingV ? pendingV.id : currentV.id);

  const selectedV = versions.find(v => v.id === selectedId) || currentV;
  const isViewingCurrent = selectedV.id === currentV.id;

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <button onClick={onBack} className="btn btn-xs btn-ghost">← 返回</button>
            <span>·</span>
            <KindChip kind="function" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{forge.id}</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{forge.name}</div>
          </div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{forge.desc}</span>
            <EntityRelMeta entityId={forge.id} />
          </div>
        </div>
        <div className="page-actions">
          <button className="btn btn-sm"><Icon.Play /> 试跑</button>
          <AskAiTrigger
            context={"Function · " + forge.name + " " + currentV.label}
            suggestions={["把超时改成 60 秒", "失败重试 3 次", "加单元测试"]}
          />
          <button className="btn btn-sm"><Icon.MoreHorizontal /></button>
        </div>
      </div>

      <div className="vr-shell">
        <div className="vr-main">
          {isViewingCurrent
            ? <FunctionFullView v={currentV} runs={detail.runs} sandbox={detail.sandbox} />
            : <FunctionDiffView currentV={currentV} otherV={selectedV} pendingV={pendingV} />}
        </div>

        <VersionRail
          versions={versions}
          currentId={currentV.id}
          pendingId={pendingV?.id || null}
          selectedId={selectedId}
          onSelect={setSelectedId}
          onAccept={() => window.Shell?.toast({ kind: "success", title: "已 Accept", desc: pendingV?.label, undo: () => {} })}
          onRevert={() => window.Shell?.toast({ kind: "warn", title: "已 Revert pending", desc: pendingV?.label, undo: () => {} })}
        />
      </div>
    </div>
  );
}

window.FunctionDetail = FunctionDetail;
