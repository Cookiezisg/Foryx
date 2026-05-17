/* eslint-disable react/prop-types */
// Config view + Observe view (basic) + Onboarding splash

const { useState: useCfgState } = React;

function ConfigView() {
  const [tab, setTab] = useCfgState("keys");

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Settings /> 设置</div>
          <div className="page-subtitle">凭证、模型、沙箱、外观</div>
        </div>
      </div>

      <div className="page-tabs">
        {[
          ["keys", "API Keys"],
          ["models", "Model"],
          ["sandbox", "Sandbox"],
          ["appearance", "外观"],
          ["data", "数据"],
        ].map(([k, l]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>{l}</button>
        ))}
      </div>

      <div className="page-body">
        {tab === "keys" && (
          <>
            <table className="t">
              <thead>
                <tr>
                  <th style={{ paddingLeft: 0 }}>Provider</th>
                  <th>名称</th>
                  <th>Key</th>
                  <th>状态</th>
                  <th>最近使用</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {Forgify.apiKeys.map(k => (
                  <tr key={k.id}>
                    <td>
                      <div className="cell-flex">
                        <div style={{ width: 22, height: 22, borderRadius: 4, background: "var(--bg-elev-2)", border: "1px solid var(--border-soft)", display: "grid", placeItems: "center", fontFamily: "var(--font-mono)", fontSize: 10, fontWeight: 700, color: "var(--fg-muted)" }}>
                          {k.provider.slice(0, 2).toUpperCase()}
                        </div>
                        <span className="cell-strong">{k.provider}</span>
                      </div>
                    </td>
                    <td>{k.displayName}</td>
                    <td><span className="cell-mono">{k.masked}</span></td>
                    <td>{k.verified ? <span className="badge success"><span className="dot" />verified</span> : <span className="badge"><span className="dot" />unverified</span>}</td>
                    <td><span style={{ fontSize: 12, color: "var(--fg-muted)" }}>{relTime(k.lastUsed)}</span></td>
                    <td className="col-tight">
                      <button className="icon-btn"><Icon.Eye /></button>
                      <button className="icon-btn"><Icon.Trash /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div style={{ padding: 16, borderTop: "1px solid var(--border-soft)" }}>
              <button className="btn btn-sm btn-accent"><Icon.Plus /> 添加 Provider</button>
            </div>
          </>
        )}

        {tab === "models" && (
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            {Forgify.modelConfigs.map(c => (
              <div key={c.scenario} className="card" style={{ flexDirection: "row", alignItems: "center", gap: 14, cursor: "default" }}>
                <div style={{ width: 90, fontFamily: "var(--font-mono)", fontSize: 12, fontWeight: 600, color: "var(--fg-muted)", textTransform: "uppercase", letterSpacing: "0.05em" }}>
                  {c.scenario}
                </div>
                <div style={{ flex: 1 }}>
                  <div className="card-title" style={{ fontFamily: "var(--font-mono)", fontSize: 14 }}>{c.modelId}</div>
                  <div className="card-desc" style={{ marginTop: 2 }}>via <strong>{c.provider}</strong></div>
                </div>
                <button className="btn btn-sm btn-ghost">切换</button>
              </div>
            ))}
          </div>
        )}

        {tab === "sandbox" && (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <div className="card" style={{ cursor: "default" }}>
              <div className="card-head">
                <div className="card-title">mise runtime</div>
                <span className="badge success"><span className="dot" />已安装</span>
              </div>
              <div className="aside-kv">
                <div className="k">mise</div><div className="v">2025.09.14 · embedded</div>
                <div className="k">python</div><div className="v">3.12.13 · installed</div>
                <div className="k">node</div><div className="v">22.6.0 · installed</div>
                <div className="k">rust</div><div className="v">1.81.0 · installed</div>
                <div className="k">总大小</div><div className="v">3.4 GB · 14 venv</div>
              </div>
            </div>
            <div className="card" style={{ cursor: "default" }}>
              <div className="card-head">
                <div className="card-title">活跃 env</div>
                <span className="badge muted">14 active · 2 GC 候选</span>
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 12, fontFamily: "var(--font-mono)" }}>
                <div className="cell-flex" style={{ color: "var(--fg-muted)" }}><span>fnenv_a1b2c3 · python 3.12 · 142 MB · function · used 12m ago</span></div>
                <div className="cell-flex" style={{ color: "var(--fg-muted)" }}><span>hdenv_d4e5f6 · python 3.12 · 218 MB · handler  · used 1m ago</span></div>
                <div className="cell-flex" style={{ color: "var(--fg-muted)" }}><span>skenv_g7h8i9 · node 22 · 89 MB · skill · used 4h ago</span></div>
              </div>
            </div>
          </div>
        )}

        {tab === "appearance" && (
          <div style={{ maxWidth: 520, display: "flex", flexDirection: "column", gap: 14 }}>
            <div className="cfg-row">
              <div className="cfg-label">主题</div>
              <div className="cfg-value" style={{ gap: 6 }}>
                <button className="btn btn-sm">系统</button>
                <button className="btn btn-sm">明</button>
                <button className="btn btn-sm">暗</button>
              </div>
            </div>
            <div className="cfg-row">
              <div className="cfg-label">Accent 色</div>
              <div className="cfg-value" style={{ gap: 8 }}>
                {[["#d97757", "Claude"], ["#2383e2", "Blue"], ["#37352f", "Ink"], ["#0f7b6c", "Green"], ["#6940a5", "Purple"]].map(([c, l]) => (
                  <div key={c} title={l} style={{ width: 24, height: 24, borderRadius: "50%", background: c, border: "2px solid var(--border)", cursor: "pointer" }} />
                ))}
              </div>
            </div>
            <div className="cfg-row">
              <div className="cfg-label">密度</div>
              <div className="cfg-value" style={{ gap: 6 }}>
                <button className="btn btn-sm">紧凑</button>
                <button className="btn btn-sm">适中</button>
                <button className="btn btn-sm">舒展</button>
              </div>
            </div>
            <div className="cfg-row">
              <div className="cfg-label">侧栏</div>
              <div className="cfg-value">
                <span style={{ fontSize: 12, color: "var(--fg-muted)" }}>⌘B 折叠 / 展开</span>
              </div>
            </div>
            <div style={{ fontSize: 12, color: "var(--fg-faint)", marginTop: 8 }}>
              快速切换在侧栏底部的齿轮图标里
            </div>
          </div>
        )}

        {tab === "data" && (
          <div className="aside-kv" style={{ gridTemplateColumns: "180px 1fr", gap: "12px 24px", fontSize: 13 }}>
            <div className="k">数据目录</div>     <div className="v">~/.forgify/</div>
            <div className="k">SQLite</div>      <div className="v">forgify.db · 42.8 MB</div>
            <div className="k">附件</div>        <div className="v">attachments/ · 218 MB · 312 个文件</div>
            <div className="k">沙箱</div>        <div className="v">sandbox/ · 3.4 GB · 14 envs</div>
            <div className="k">日志</div>        <div className="v">logs/ · 184 MB · 滚动 7 天</div>
            <div className="k">备份</div>        <div className="v">从未备份 · <a>立即备份</a></div>
          </div>
        )}
      </div>
    </div>
  );
}

// ── Observe view (relations graph + usage) ──────────────────────────────
function ObserveView() {
  const [tab, setTab] = useCfgState("relations");
  const metric = (label, value, sub) => (
    <div className="card" style={{ cursor: "default" }}>
      <div style={{ fontSize: 11, color: "var(--fg-faint)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 600 }}>{label}</div>
      <div style={{ fontSize: 26, fontWeight: 600, color: "var(--fg-strong)", letterSpacing: "-0.02em", fontFamily: "var(--font-mono)" }}>{value}</div>
      <div className="card-desc">{sub}</div>
    </div>
  );
  return (
    <div className="page" style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0 }}>
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Activity /> 洞察</div>
          <div className="page-subtitle">实体关系 · 用量</div>
        </div>
      </div>
      <div className="page-tabs">
        {[["relations", "关系图"], ["usage", "用量"]].map(([k, l]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>{l}</button>
        ))}
      </div>
      {tab === "relations" && (
        <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
          <RelGraph />
        </div>
      )}
      {tab === "usage" && (
        <div className="page-body">
          <div className="card-grid" style={{ gridTemplateColumns: "repeat(4, 1fr)" }}>
            {metric("消息", "342", "本周 +12% vs 上周")}
            {metric("Tool calls", "1,284", "并行率 38%")}
            {metric("Token 用量", "1.2M", "≈ ¥ 4.18 / 周")}
            {metric("FlowRun 成功率", "94%", "6 个 workflow 活跃")}
          </div>

          <h3 className="section-label" style={{ marginTop: 24 }}>
            活动热力图 · 过去一年
          </h3>
          <ContribHeatmap />
        </div>
      )}
    </div>
  );
}

function ContribHeatmap() {
  const weeks = 53;
  const today = new Date();
  // Anchor: oldest cell is (weeks-1) weeks + 6 days ago, then we walk forward day by day.
  // Determine the Sunday of (today's week - (weeks-1) weeks).
  const dayOfWeek = today.getDay(); // 0 = Sun
  const start = new Date(today);
  start.setDate(today.getDate() - dayOfWeek - (weeks - 1) * 7);
  start.setHours(0, 0, 0, 0);

  const cells = [];
  for (let w = 0; w < weeks; w++) {
    const col = [];
    for (let d = 0; d < 7; d++) {
      const idx = w * 7 + d;
      const date = new Date(start);
      date.setDate(start.getDate() + idx);
      if (date > today) { col.push(null); continue; }
      const recency = w / weeks;
      const weekday = d > 0 && d < 6 ? 1 : 0.4;
      const noise = ((Math.sin(idx * 1.7) + Math.cos(idx * 0.7) + 2) / 4);
      const v = noise * weekday * (0.3 + recency * 0.7);
      let level = 0;
      if (v > 0.20) level = 1;
      if (v > 0.40) level = 2;
      if (v > 0.60) level = 3;
      if (v > 0.78) level = 4;
      col.push({ level, date });
    }
    cells.push(col);
  }

  // Month labels: place label at the column where a new month starts (only if it has >= 2 weeks visible)
  const monthLabels = [];
  let lastMonth = -1;
  for (let w = 0; w < weeks; w++) {
    const firstReal = cells[w].find(c => c);
    if (!firstReal) continue;
    const m = firstReal.date.getMonth();
    if (m !== lastMonth) {
      monthLabels.push({ w, label: (m + 1) + "月" });
      lastMonth = m;
    }
  }

  const totalActive = cells.flat().filter(c => c && c.level > 0).length;

  // GitHub-style: 10px square, 3px gap → column width 13
  const CELL = 10, GAP = 3, STEP = CELL + GAP;

  return (
    <div className="ct-heatmap">
      <div className="ct-summary">
        <span>过去一年 <b>{totalActive}</b> 天有活动</span>
      </div>
      <div className="ct-grid" style={{ "--ct-cell": CELL + "px", "--ct-gap": GAP + "px", "--ct-step": STEP + "px" }}>
        <div className="ct-day-col">
          <span style={{ visibility: "hidden" }}>Mon</span>
          <span>周一</span>
          <span style={{ visibility: "hidden" }}>Tue</span>
          <span>周三</span>
          <span style={{ visibility: "hidden" }}>Thu</span>
          <span>周五</span>
          <span style={{ visibility: "hidden" }}>Sat</span>
        </div>
        <div className="ct-right">
          <div className="ct-month-row" style={{ width: weeks * STEP }}>
            {monthLabels.map((m, i) => (
              <span key={i} className="ct-month" style={{ left: m.w * STEP }}>{m.label}</span>
            ))}
          </div>
          <div className="ct-cells">
            {cells.map((col, w) => (
              <div key={w} className="ct-col">
                {col.map((c, d) => (
                  c ? (
                    <div
                      key={d}
                      className={"ct-cell ct-l" + c.level}
                      title={c.date.toLocaleDateString("zh-CN", { year: "numeric", month: "short", day: "numeric" }) + " · " + (c.level === 0 ? "无活动" : c.level + " 级活动")}
                    />
                  ) : (
                    <div key={d} className="ct-cell ct-empty" />
                  )
                ))}
              </div>
            ))}
          </div>
        </div>
      </div>
      <div className="ct-legend">
        <span>少</span>
        <div className="ct-cell ct-l0" />
        <div className="ct-cell ct-l1" />
        <div className="ct-cell ct-l2" />
        <div className="ct-cell ct-l3" />
        <div className="ct-cell ct-l4" />
        <span>多</span>
      </div>
    </div>
  );
}

// ── Onboarding splash ────────────────────────────────────────────────────
function Onboarding({ onDismiss }) {
  const steps = [
    { num: "01", title: "数据目录", desc: "~/.forgify/ 已创建 · 42.8 MB · SQLite 已初始化", state: "ready", done: true },
    { num: "02", title: "添加 API Key", desc: "至少配置一个 provider (DeepSeek / Anthropic / Qwen / …)", state: "DEEPSEEK · ✓ 已验证", done: true },
    { num: "03", title: "Sandbox runtime", desc: "mise 嵌入版本 · 首次为 Python 装备运行环境", state: "55%   downloading python-3.12.13", active: true },
    { num: "04", title: "导入 Skills (可选)", desc: "把你的 SKILL.md 文件夹拖进来，agent 即可调用", state: "skipped" },
    { num: "05", title: "开始第一次对话", desc: "试试『帮我做一个工具来 …』", state: "next →" },
  ];
  return (
    <div className="onboarding">
      <div className="onboarding-card">
        <div className="onboarding-logo">FG</div>
        <div>
          <div className="onboarding-title">欢迎使用 Forgify</div>
          <div className="onboarding-sub">
            你的工具、对话、运行历史只存在这台电脑上。<br />
            先把环境准备好。
          </div>
        </div>

        <div className="onboarding-steps">
          {steps.map(s => (
            <div key={s.num} className={"onb-step" + (s.done ? " is-done" : s.active ? " is-active" : "")}>
              <div className="step-num">{s.done ? <Icon.Check style={{ width: 12, height: 12 }} /> : s.num}</div>
              <div className="step-text">
                <div className="step-title">{s.title}</div>
                <div className="step-desc">{s.desc}</div>
                {s.active && (
                  <div className="progress-bar" style={{ marginTop: 6 }}><div style={{ width: "55%" }} /></div>
                )}
              </div>
              <div className="step-state">{s.state}</div>
            </div>
          ))}
        </div>

        <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
          <button className="btn btn-sm btn-ghost" onClick={onDismiss}>稍后</button>
          <button className="btn btn-sm btn-accent" onClick={onDismiss}>进入应用 <Icon.ArrowRight /></button>
        </div>
      </div>
    </div>
  );
}

window.ConfigView = ConfigView;
window.ObserveView = ObserveView;
window.Onboarding = Onboarding;
