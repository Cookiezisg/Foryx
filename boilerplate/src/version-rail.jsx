/* eslint-disable react/prop-types */
// Unified version management UI for Function / Handler / Workflow.
//
//   <VersionRail
//     versions={[...]}            sorted newest-first
//     currentVersion={"v3"}
//     pendingVersion={"v4"|null}  null when no pending
//     deployedVersion={"v2"|null} only for workflow; null otherwise
//     compareLeft={"v3"}          // base of diff
//     compareRight={"v4"}         // tip of diff
//     onSelect, onCompare, onAccept, onRevert, onRollback, onDeploy
//     showDeploy={bool}           // true only for workflow
//   />

const { useState: useVrState, useRef: useVrRef, useEffect: useVrEffect } = React;

// ── Version row ─────────────────────────────────────────────────────────
function VersionRow({ v, isCurrent, isPending, isDeployed, isCompareLeft, isCompareRight, isSelected, onClick, onMore }) {
  const dotColor = isPending ? "var(--status-warn)"
    : isCurrent ? "var(--status-success)"
    : isDeployed ? "var(--accent)"
    : "var(--fg-faint)";
  return (
    <button
      className={"vr-row" + (isSelected ? " is-selected" : "") + (isCompareLeft || isCompareRight ? " is-compare" : "")}
      onClick={onClick}
    >
      <span className="vr-dot" style={{ background: dotColor }} />
      <div className="vr-meta">
        <div className="vr-head">
          <span className="vr-num">{v.label}</span>
          {isPending && <span className="vr-badge vr-pending"><Icon.Sparkles /> AI · pending</span>}
          {isCurrent && <span className="vr-badge vr-current">current</span>}
          {isDeployed && <span className="vr-badge vr-deployed">deployed</span>}
          {(isCompareLeft || isCompareRight) && <span className="vr-badge vr-compare">{isCompareLeft ? "A" : "B"}</span>}
        </div>
        <div className="vr-summary">{v.summary || <span style={{ color: "var(--fg-faint)" }}>无说明</span>}</div>
        <div className="vr-foot">
          <span className="vr-author">{v.author}</span>
          <span className="vr-sep">·</span>
          <span className="vr-time">{v.at}</span>
        </div>
      </div>
    </button>
  );
}

// ── Compare picker ──────────────────────────────────────────────────────
function ComparePicker({ versions, leftId, rightId, onChange }) {
  return (
    <div className="vr-compare-picker">
      <select value={leftId} onChange={e => onChange(e.target.value, rightId)} className="vr-cmp-select">
        {versions.map(v => <option key={v.id} value={v.id}>{v.label}{v.summary ? " · " + v.summary.slice(0, 22) : ""}</option>)}
      </select>
      <span className="vr-cmp-arrow">⇆</span>
      <select value={rightId} onChange={e => onChange(leftId, e.target.value)} className="vr-cmp-select">
        {versions.map(v => <option key={v.id} value={v.id}>{v.label}{v.summary ? " · " + v.summary.slice(0, 22) : ""}</option>)}
      </select>
    </div>
  );
}

// ── Version rail (collapsible) ──────────────────────────────────────────
function VersionRail({
  versions,
  currentId, pendingId, deployedId,
  selectedId, onSelect,
  onAccept, onRevert, onRollback, onDeploy,
  showDeploy,
}) {
  const [collapsed, setCollapsed] = useVrState(false);
  const pending = versions.find(v => v.id === pendingId);

  return (
    <aside className={"vr-rail" + (collapsed ? " is-collapsed" : "")}>
      <div className="vr-rail-head">
        <button className="vr-collapse" onClick={() => setCollapsed(c => !c)} title={collapsed ? "展开" : "收起"}>
          <Icon.GitBranch />
          {!collapsed && <span>版本 · {versions.length}</span>}
        </button>
      </div>

      {!collapsed && pending && (
        <div className="vr-pending-banner">
          <div className="vr-pending-head">
            <Icon.Sparkles style={{ width: 12, height: 12, color: "var(--status-warn)" }} />
            <span>有 1 个 pending 待处理</span>
          </div>
          <div className="vr-pending-summary">{pending.summary || "(无说明)"}</div>
          <div className="vr-pending-actions">
            <button className="btn btn-xs btn-danger" onClick={onRevert}>Revert</button>
            <button className="btn btn-xs" onClick={() => onSelect?.(pendingId)}>查看 diff</button>
            <button className="btn btn-xs btn-accent" onClick={onAccept}>Accept</button>
          </div>
        </div>
      )}

      {!collapsed && (
        <div className="vr-list">
          {versions.map(v => (
            <VersionRow
              key={v.id}
              v={v}
              isCurrent={v.id === currentId}
              isPending={v.id === pendingId}
              isDeployed={v.id === deployedId}
              isSelected={v.id === selectedId}
              onClick={() => onSelect?.(v.id)}
            />
          ))}
        </div>
      )}

      {collapsed && (
        <div className="vr-collapsed-list">
          {versions.map(v => (
            <button
              key={v.id}
              className={"vr-collapsed-dot" + (v.id === selectedId ? " is-selected" : "")}
              title={v.label + (v.summary ? " · " + v.summary : "")}
              onClick={() => onSelect?.(v.id)}
            >
              <span
                className="vr-dot"
                style={{
                  background: v.id === pendingId ? "var(--status-warn)"
                    : v.id === currentId ? "var(--status-success)"
                    : v.id === deployedId ? "var(--accent)"
                    : "var(--fg-faint)"
                }}
              />
              <span className="vr-num-small">{v.label}</span>
            </button>
          ))}
        </div>
      )}

      {!collapsed && showDeploy && deployedId !== currentId && (
        <div className="vr-deploy-bar">
          <div style={{ fontSize: 11, color: "var(--fg-muted)" }}>
            生产中：{deployedId} · 当前编辑：{currentId}
          </div>
          <button className="btn btn-xs btn-accent" onClick={onDeploy}>
            <Icon.Play /> 部署 {currentId}
          </button>
        </div>
      )}
    </aside>
  );
}

// ── Split-view text diff (for Function/Handler code) ────────────────────
//   Takes left/right source strings, computes line-level diff, renders side-by-side.
function splitDiff(a, b) {
  const al = a.split("\n");
  const bl = b.split("\n");
  // LCS-based diff (simple, O(n*m))
  const m = al.length, n = bl.length;
  const dp = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      if (al[i] === bl[j]) dp[i][j] = dp[i + 1][j + 1] + 1;
      else dp[i][j] = Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  // Walk to produce rows: { left, right, leftN, rightN, op: 'eq'|'del'|'add' }
  const rows = [];
  let i = 0, j = 0;
  while (i < m && j < n) {
    if (al[i] === bl[j]) {
      rows.push({ leftN: i + 1, rightN: j + 1, left: al[i], right: bl[j], op: "eq" });
      i++; j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      rows.push({ leftN: i + 1, rightN: null, left: al[i], right: "", op: "del" });
      i++;
    } else {
      rows.push({ leftN: null, rightN: j + 1, left: "", right: bl[j], op: "add" });
      j++;
    }
  }
  while (i < m) { rows.push({ leftN: i + 1, rightN: null, left: al[i], right: "", op: "del" }); i++; }
  while (j < n) { rows.push({ leftN: null, rightN: j + 1, left: "", right: bl[j], op: "add" }); j++; }
  return rows;
}

function SplitDiff({ leftLabel, rightLabel, leftSrc, rightSrc }) {
  const rows = splitDiff(leftSrc || "", rightSrc || "");
  const adds = rows.filter(r => r.op === "add").length;
  const dels = rows.filter(r => r.op === "del").length;
  return (
    <div className="split-diff">
      <div className="split-diff-head">
        <div className="split-diff-side"><span className="vr-badge vr-current">A</span> {leftLabel}</div>
        <div className="split-diff-stats">
          <span className="vr-add">+{adds}</span>
          <span className="vr-del">−{dels}</span>
        </div>
        <div className="split-diff-side"><span className="vr-badge vr-pending"><Icon.Sparkles /> B</span> {rightLabel}</div>
      </div>
      <div className="split-diff-body">
        {rows.map((r, i) => (
          <div key={i} className={"sd-row sd-" + r.op}>
            <div className="sd-ln">{r.leftN || ""}</div>
            <div className="sd-code sd-left">{r.left}</div>
            <div className="sd-ln">{r.rightN || ""}</div>
            <div className="sd-code sd-right">{r.right}</div>
          </div>
        ))}
      </div>
    </div>
  );
}

window.VersionRail = VersionRail;
window.SplitDiff = SplitDiff;
window.splitDiff = splitDiff;
