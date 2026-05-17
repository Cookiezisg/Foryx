/* eslint-disable react/prop-types */
// MCP view — server list + detailed server card

const { useState: useMcpState } = React;

function HealthSparkline({ data }) {
  const w = 220, h = 28;
  const step = w / Math.max(1, data.length - 1);
  return (
    <svg width={w} height={h} className="mcp-sparkline">
      <rect x="0" y="0" width={w} height={h} fill="var(--bg-elev-2)" rx="4" />
      {data.map((v, i) => (
        <rect
          key={i}
          x={i * step}
          y={v === 1 ? 4 : 16}
          width={Math.max(1, step - 1)}
          height={v === 1 ? h - 8 : h - 20}
          fill={v === 1 ? "var(--status-success)" : "var(--status-warn)"}
          opacity={v === 1 ? 0.8 : 1}
          rx="1"
        />
      ))}
    </svg>
  );
}

function McpDetail({ server, onBack }) {
  const detail = Forgify.mcpDetails[server.id] || {};
  const [tab, setTab] = useMcpState("tools");

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <button onClick={onBack} className="btn btn-xs btn-ghost">← 返回</button>
            <span>·</span>
            <KindChip kind="mcp" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{server.id}</span>
          </div>
          <div className="page-title" style={{ fontFamily: "var(--font-mono)" }}>{server.name}</div>
          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            {server.status === "healthy" ? (
              <span className="badge success"><span className="dot" />healthy</span>
            ) : (
              <span className="badge warn"><span className="dot" />degraded</span>
            )}
            <span style={{ marginLeft: 4 }}>{detail.tools?.length || 0} tools 已注册 · 最近心跳 {relTime(server.lastSeen)}</span>
            <EntityRelMeta entityId={server.id} />
          </div>
        </div>
        <div className="page-actions">
          <button className="btn btn-sm"><Icon.Refresh /> 重连</button>
          <button className="btn btn-sm btn-danger"><Icon.StopCircle /> 停用</button>
        </div>
      </div>

      <div className="page-body">
        {/* Top row: command + health */}
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
          <div className="card" style={{ cursor: "default" }}>
            <div className="card-head">
              <div className="card-title">启动命令</div>
              <button className="btn btn-xs btn-ghost"><Icon.Copy /></button>
            </div>
            <pre className="code-block" style={{ fontSize: 11, lineHeight: 1.6 }}>{detail.command} {(detail.args || []).join(" ")}</pre>
            <div className="aside-label" style={{ marginTop: 8 }}>环境变量</div>
            <div className="aside-kv" style={{ gap: "4px 12px" }}>
              {Object.entries(detail.env || {}).map(([k, v]) => (
                <React.Fragment key={k}>
                  <div className="k">{k}</div>
                  <div className="v">
                    {v.masked} {v.required && <span style={{ color: "var(--status-error)", marginLeft: 4 }}>required</span>}
                  </div>
                </React.Fragment>
              ))}
            </div>
          </div>

          <div className="card" style={{ cursor: "default" }}>
            <div className="card-head">
              <div className="card-title">24 小时健康</div>
              <span className="cell-mono" style={{ fontSize: 11, color: "var(--fg-faint)" }}>
                {(detail.health || []).filter(x => x === 1).length}/{(detail.health || []).length} 小时正常
              </span>
            </div>
            <HealthSparkline data={detail.health || []} />
            <div className="aside-label" style={{ marginTop: 8 }}>失败策略</div>
            <div style={{ fontSize: 12, color: "var(--fg-muted)" }}>
              连续 3 次心跳超时 → 进 degraded · 下次调用时自动重连
            </div>
          </div>
        </div>

        <div className="page-tabs" style={{ marginTop: 16, padding: 0, border: 0 }}>
          {[
            ["tools", "Tools", (detail.tools || []).length],
            ["log", "安装日志", (detail.installLog || []).length],
            ["raw", "Raw JSON"],
          ].map(([k, l, c]) => (
            <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
              {l}{c != null && <span className="count">{c}</span>}
            </button>
          ))}
        </div>

        {tab === "tools" && (
          <table className="t" style={{ marginTop: 8 }}>
            <thead>
              <tr>
                <th style={{ paddingLeft: 0 }}>名称</th>
                <th>描述</th>
                <th>动作</th>
              </tr>
            </thead>
            <tbody>
              {(detail.tools || []).map(t => (
                <tr key={t.name}>
                  <td><span className="cell-mono" style={{ color: "var(--accent)" }}>{t.name}</span></td>
                  <td>{t.desc}</td>
                  <td className="col-tight">
                    <button className="btn btn-xs btn-ghost"><Icon.Play /> 试调用</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {tab === "log" && (
          <pre className="code-block" style={{ marginTop: 8, maxHeight: 400, overflowY: "auto" }}>
            {(detail.installLog || []).map((l, i) =>
              `[${l.time}] ${l.level.toUpperCase().padEnd(5)} ${l.msg}\n`
            ).join("")}
          </pre>
        )}

        {tab === "raw" && (
          <pre className="code-block" style={{ marginTop: 8, fontSize: 11 }}>{JSON.stringify({ ...server, ...detail }, null, 2)}</pre>
        )}
      </div>
    </div>
  );
}

// Mcp view — list + open by focusEntity
function McpView() {
  const [open, setOpen] = useMcpState(null);
  useMcpState; // hook order
  React.useEffect(() => {
    const id = window.Shell?.focusEntity?.mcp;
    if (id) {
      const m = Forgify.mcpServers.find(x => x.id === id);
      if (m) setOpen(m);
    }
  }, []);
  if (open) return <McpDetail server={open} onBack={() => setOpen(null)} />;

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Server /> MCP Servers</div>
          <div className="page-subtitle">MCP server</div>
        </div>
        <div className="page-actions">
          <button className="btn btn-sm"><Icon.Inbox /> 从市场添加</button>
          <button className="btn btn-sm btn-accent"><Icon.Plus /> 手动配置</button>
        </div>
      </div>

      <div className="page-body">
        <div className="card-grid">
          {Forgify.mcpServers.map(m => {
            const detail = Forgify.mcpDetails[m.id] || {};
            return (
              <div key={m.id} className="card" onClick={() => setOpen(m)}>
                <div className="card-head">
                  <div style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
                    <div style={{ width: 28, height: 28, borderRadius: 6, background: "var(--bg-elev-2)", border: "1px solid var(--border-soft)", display: "grid", placeItems: "center", color: "var(--fg-muted)" }}>
                      <Icon.Server style={{ width: 14, height: 14 }} />
                    </div>
                    <div style={{ minWidth: 0 }}>
                      <div className="card-title">{m.name}</div>
                      <div className="cell-mono" style={{ fontSize: 10, color: "var(--fg-faint)" }}>{m.id}</div>
                    </div>
                  </div>
                  {m.status === "healthy" ? (
                    <span className="badge success"><span className="dot" />healthy</span>
                  ) : (
                    <span className="badge warn"><span className="dot" />degraded</span>
                  )}
                </div>
                <div style={{ marginTop: 4 }}>
                  <HealthSparkline data={detail.health || Array(24).fill(1)} />
                </div>
                <div className="card-foot">
                  <span>{detail.tools?.length || m.tools} tools · 最近 {relTime(m.lastSeen)}</span>
                  <ActionMenu items={[
                    { label: "重连", icon: Icon.Refresh },
                    { label: "编辑配置", icon: Icon.Settings },
                    { label: "查看日志", icon: Icon.Terminal },
                    "divider",
                    { label: "停用", icon: Icon.StopCircle },
                    { label: "删除", icon: Icon.Trash, danger: true },
                  ]} />
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

window.McpView = McpView;
