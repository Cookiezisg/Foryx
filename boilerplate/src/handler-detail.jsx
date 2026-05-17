/* eslint-disable react/prop-types */
// Handler detail — VersionRail + main area
//   Current → full view (class / config / call history)
//   Other   → diff view (description / methods / config changes)

const { useState: useHdState } = React;

// ── Full view (selected version is current/prod) ────────────────────────
function HandlerFullView({ v, callStats, recentCalls }) {
  const [tab, setTab] = useHdState("class");
  const [selectedMethod, setSelectedMethod] = useHdState(v.methods[0]);
  const successRate = callStats.ok / (callStats.ok + callStats.fail) * 100;

  // ensure method exists in this version
  const method = v.methods.find(m => m.name === selectedMethod.name) || v.methods[0];

  return (
    <>
      <div className="page-tabs">
        {[["class", "Class"], ["config", "Config"], ["calls", "Call 历史"]].map(([k, l]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>{l}</button>
        ))}
      </div>

      {tab === "class" && (
        <div className="hd-class">
          <aside className="hd-methods">
            <div className="hd-class-name">
              <Icon.Boxes style={{ width: 14, height: 14, marginRight: 6 }} />
              class
            </div>
            {v.methods.map(m => (
              <button key={m.name} className={"hd-method" + (method.name === m.name ? " is-active" : "")} onClick={() => setSelectedMethod(m)}>
                <span style={{ color: "var(--fg-faint)", fontFamily: "var(--font-mono)", fontSize: 10 }}>fn</span>
                <span className="cell-mono">{m.name}</span>
              </button>
            ))}
          </aside>
          <main className="hd-method-detail">
            <div className="hd-method-sig">
              <span style={{ color: "var(--fg-faint)", fontFamily: "var(--font-mono)" }}>def</span>{" "}
              <span style={{ color: "var(--accent)", fontFamily: "var(--font-mono)", fontWeight: 600 }}>{method.name}</span>
              <span style={{ fontFamily: "var(--font-mono)", color: "var(--fg-body)" }}>{method.sig}</span>
            </div>
            <div className="hd-method-desc">{method.desc}</div>
          </main>
        </div>
      )}

      {tab === "config" && (
        <div style={{ padding: "20px 32px", display: "flex", flexDirection: "column", gap: 12, maxWidth: 600 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12, color: "var(--fg-muted)" }}>
            <Icon.KeyRound style={{ width: 13, height: 13 }} /> Encrypted with AES-GCM · 仅本地存储
          </div>
          {Object.entries(v.config).map(([k, val]) => (
            <div key={k} className="cfg-row">
              <div className="cfg-label">
                {k}
                {val.secret && <span className="badge muted" style={{ marginLeft: 6 }}>secret</span>}
              </div>
              <div className="cfg-value">
                <input type="text" className="cfg-input" value={val.value} readOnly />
                {val.masked && <button className="icon-btn"><Icon.Eye /></button>}
                <button className="icon-btn"><Icon.Copy /></button>
              </div>
            </div>
          ))}
        </div>
      )}

      {tab === "calls" && (
        <div style={{ padding: "16px 32px" }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 12, marginBottom: 16 }}>
            <div className="stat-card"><div className="stat-label">成功率</div><div className="stat-value">{successRate.toFixed(1)}%</div><div className="stat-sub">{callStats.ok} / {callStats.ok + callStats.fail}</div></div>
            <div className="stat-card"><div className="stat-label">p50</div><div className="stat-value">{callStats.p50}<small>ms</small></div></div>
            <div className="stat-card"><div className="stat-label">p95</div><div className="stat-value">{callStats.p95}<small>ms</small></div></div>
            <div className="stat-card"><div className="stat-label">p99</div><div className="stat-value">{callStats.p99}<small>ms</small></div></div>
          </div>
          <table className="t">
            <thead><tr><th style={{ paddingLeft: 0 }}>时间</th><th>方法</th><th>状态</th><th>耗时</th><th>错误</th></tr></thead>
            <tbody>
              {recentCalls.map((c, i) => (
                <tr key={i}>
                  <td className="cell-mono" style={{ fontSize: 12, color: "var(--fg-muted)" }}>{c.at}</td>
                  <td><span className="cell-mono" style={{ color: "var(--accent)" }}>{c.method}</span></td>
                  <td>{c.status === "ok" ? <span className="badge success"><span className="dot" />ok</span> : <span className="badge error"><span className="dot" />fail</span>}</td>
                  <td className="cell-mono">{c.ms}ms</td>
                  <td><span style={{ color: c.status === "fail" ? "var(--status-error)" : "var(--fg-faint)", fontFamily: "var(--font-mono)", fontSize: 11 }}>{c.error || ""}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

function MethodDiffCard({ change, isPending, currentV, otherV }) {
  const [open, setOpen] = useHdState(false);
  const c = change;
  const Ic = c.kind === "added" ? Icon.Plus : c.kind === "removed" ? Icon.X : Icon.Wrench;

  // Compute body change check
  const bodyChanged = c.kind === "changed" && c.a?.body !== c.b?.body;
  const expandable = c.kind === "added" || c.kind === "removed" || bodyChanged;

  return (
    <div className={"hd-method-diff hd-method-diff-" + c.kind}>
      <button
        className="hd-method-diff-head-btn"
        onClick={() => expandable && setOpen(o => !o)}
        style={{ cursor: expandable ? "pointer" : "default" }}
      >
        {expandable && <Icon.ChevronRight className="chev" style={{ transform: open ? "rotate(90deg)" : "none", color: "var(--fg-muted)", width: 12, height: 12 }} />}
        <span className={"vr-badge vr-cmp-" + c.kind}>
          {c.kind === "added" ? "新增" : c.kind === "removed" ? "删除" : "修改"}
        </span>
        <code style={{ fontFamily: "var(--font-mono)", color: "var(--accent)", fontSize: 13 }}>{c.node?.name || c.b?.name || c.a?.name}</code>
        {!expandable && c.kind === "changed" && (
          <span style={{ fontSize: 11, color: "var(--fg-faint)", marginLeft: 6 }}>仅签名 / 描述变化</span>
        )}
      </button>

      {/* Always-visible signature comparison */}
      {c.kind === "removed" && (
        <div className="hd-method-diff-side">
          <div className="hd-method-diff-label">移除</div>
          <code className="hd-method-diff-sig">{c.a.sig}</code>
          <div className="hd-method-diff-desc">{c.a.desc}</div>
        </div>
      )}
      {c.kind === "added" && (
        <div className="hd-method-diff-side">
          <div className="hd-method-diff-label">新增</div>
          <code className="hd-method-diff-sig">{c.b.sig}</code>
          <div className="hd-method-diff-desc">{c.b.desc}</div>
        </div>
      )}
      {c.kind === "changed" && (
        <div className="hd-method-diff-2col">
          <div className="hd-method-diff-side">
            <div className="hd-method-diff-label">A · current</div>
            <code className="hd-method-diff-sig">{c.a.sig}</code>
            <div className="hd-method-diff-desc">{c.a.desc}</div>
          </div>
          <div className="hd-method-diff-side">
            <div className="hd-method-diff-label">B{isPending ? " · pending" : ""}</div>
            <code className="hd-method-diff-sig">{c.b.sig}</code>
            <div className="hd-method-diff-desc">{c.b.desc}</div>
          </div>
        </div>
      )}

      {open && c.kind === "added" && c.b.body && (
        <div className="hd-method-body-pane">
          <div className="hd-method-body-head">代码 · 新增</div>
          <CodeView src={c.b.body} />
        </div>
      )}
      {open && c.kind === "removed" && c.a.body && (
        <div className="hd-method-body-pane">
          <div className="hd-method-body-head">代码 · 已移除</div>
          <CodeView src={c.a.body} />
        </div>
      )}
      {open && c.kind === "changed" && bodyChanged && (
        <div className="hd-method-body-pane">
          <SplitDiff
            leftLabel="A · current"
            rightLabel={"B" + (isPending ? " · pending" : "")}
            leftSrc={c.a.body || ""}
            rightSrc={c.b.body || ""}
          />
        </div>
      )}
    </div>
  );
}

// ── Diff view ───────────────────────────────────────────────────────────
function HandlerDiffView({ currentV, otherV, pendingV }) {
  const isPending = otherV.id === pendingV?.id;
  const descChanged = (currentV.description || "") !== (otherV.description || "");

  // Methods: compute added / removed / changed-signature
  const curMethods = new Map(currentV.methods.map(m => [m.name, m]));
  const othMethods = new Map(otherV.methods.map(m => [m.name, m]));
  const allNames = [...new Set([...curMethods.keys(), ...othMethods.keys()])];
  const methodChanges = allNames.map(name => {
    const a = curMethods.get(name);
    const b = othMethods.get(name);
    if (!a) return { name, kind: "added", a: null, b };
    if (!b) return { name, kind: "removed", a, b: null };
    if (a.sig !== b.sig || a.desc !== b.desc || a.body !== b.body) return { name, kind: "changed", a, b };
    return { name, kind: "same", a, b };
  }).filter(c => c.kind !== "same");

  // Config: added / removed / value changed
  const curCfg = currentV.config || {};
  const othCfg = otherV.config || {};
  const allKeys = [...new Set([...Object.keys(curCfg), ...Object.keys(othCfg)])];
  const cfgChanges = allKeys.map(k => {
    const a = curCfg[k];
    const b = othCfg[k];
    if (!a) return { key: k, kind: "added", a: null, b };
    if (!b) return { key: k, kind: "removed", a, b: null };
    if (a.value !== b.value || a.secret !== b.secret) return { key: k, kind: "changed", a, b };
    return { key: k, kind: "same", a, b };
  }).filter(c => c.kind !== "same");

  const totalChanges = (descChanged ? 1 : 0) + methodChanges.length + cfgChanges.length;

  return (
    <div className="fn-view" style={{ padding: "20px 28px" }}>
      <h3 className="section-label" style={{ marginTop: 0, display: "flex", alignItems: "center", gap: 8 }}>
        Diff · {currentV.label} ⇆ {otherV.label}
        {isPending && <span className="vr-badge vr-pending"><Icon.Sparkles /> pending</span>}
        <span style={{ color: "var(--fg-faint)", fontWeight: 400, textTransform: "none", letterSpacing: 0 }}>· {totalChanges} 处变更</span>
      </h3>

      {totalChanges === 0 && (
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

      {methodChanges.length > 0 && (
        <div className="fn-diff-section">
          <div className="fn-diff-section-label">方法变更 · {methodChanges.length}</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {methodChanges.map((c, i) => (
              <MethodDiffCard key={i} change={c} isPending={isPending} currentV={currentV} otherV={otherV} />
            ))}
          </div>
        </div>
      )}

      {cfgChanges.length > 0 && (
        <div className="fn-diff-section">
          <div className="fn-diff-section-label">Config 变更 · {cfgChanges.length}</div>
          <table className="t" style={{ fontSize: 12 }}>
            <thead><tr><th>字段</th><th>A · {currentV.label}</th><th>B · {otherV.label}</th><th>变化</th></tr></thead>
            <tbody>
              {cfgChanges.map((c, i) => (
                <tr key={i}>
                  <td className="cell-mono">{c.key}</td>
                  <td><code style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{c.a?.value || "—"}</code></td>
                  <td><code style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{c.b?.value || "—"}</code></td>
                  <td>
                    {c.kind === "added"   && <span className="vr-badge vr-cmp-added">新增</span>}
                    {c.kind === "removed" && <span className="vr-badge vr-cmp-removed">删除</span>}
                    {c.kind === "changed" && <span className="vr-badge vr-cmp-changed">修改</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ── Detail page ─────────────────────────────────────────────────────────
function HandlerDetail({ forge, onBack }) {
  const detail = Forgify.handlerDetails[forge.id] || Forgify.handlerDetails.hd_notion_001;
  const versions = detail.versions;
  const currentV = versions.find(v => v.state === "current") || versions[0];
  const pendingV = versions.find(v => v.state === "pending");
  const [selectedId, setSelectedId] = useHdState(pendingV ? pendingV.id : currentV.id);

  const selectedV = versions.find(v => v.id === selectedId) || currentV;
  const isViewingCurrent = selectedV.id === currentV.id;

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <button onClick={onBack} className="btn btn-xs btn-ghost">← 返回</button>
            <span>·</span>
            <KindChip kind="handler" />
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
          <button className="btn btn-sm"><Icon.Play /> 试调用</button>
          <AskAiTrigger context={"Handler · " + forge.name} suggestions={["给 upsert_row 加批量写", "把 search 改成游标分页"]} />
        </div>
      </div>

      <div className="vr-shell">
        <div className="vr-main" style={{ padding: 0 }}>
          {isViewingCurrent
            ? <HandlerFullView v={currentV} callStats={detail.callStats} recentCalls={detail.recentCalls} />
            : <HandlerDiffView currentV={currentV} otherV={selectedV} pendingV={pendingV} />}
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

window.HandlerDetail = HandlerDetail;
