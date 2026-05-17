/* eslint-disable react/prop-types */
// Skills view — list + detail with SKILL.md frontmatter + body preview

const { useState: useSkState } = React;

function SkillRow({ skill, active, onClick }) {
  return (
    <div className="sk-row-wrap">
      <button className={"sk-row" + (active ? " is-active" : "")} onClick={onClick}>
        <div className="sk-row-icon">
          <Icon.Sparkles />
        </div>
        <div className="sk-row-meta">
          <div className="sk-row-name">{skill.name}</div>
          <div className="sk-row-desc">{skill.description}</div>
        </div>
        <span className="badge muted">{skill.source}</span>
      </button>
      <ActionMenu items={[
        { label: "打开 SKILL.md 文件夹", icon: Icon.Folder },
        { label: "复制路径", icon: Icon.Copy },
        { label: "重新扫描", icon: Icon.Refresh },
        "divider",
        { label: "禁用", icon: Icon.EyeOff },
        { label: "删除", icon: Icon.Trash, danger: true },
      ]} />
    </div>
  );
}

function FrontmatterField({ k, v }) {
  if (Array.isArray(v)) {
    return (
      <div className="fm-field">
        <div className="fm-key">{k}</div>
        <div className="fm-value">
          {v.map((x, i) => <span key={i} className="kind-chip" style={{ marginRight: 4 }}>{x}</span>)}
        </div>
      </div>
    );
  }
  return (
    <div className="fm-field">
      <div className="fm-key">{k}</div>
      <div className="fm-value">{v}</div>
    </div>
  );
}

function highlightPlaceholders(body) {
  // Highlight $1, $ARGUMENTS, ${CLAUDE_X} placeholders
  const parts = [];
  const re = /(\$\d+|\$ARGUMENTS|\$\{CLAUDE_[A-Z_]+\}|\{\{\s*[^}]+\s*\}\})/g;
  let last = 0;
  body.replace(re, (m, _g, idx) => {
    if (idx > last) parts.push(body.slice(last, idx));
    parts.push(<mark key={idx} className="sk-placeholder">{m}</mark>);
    last = idx + m.length;
    return m;
  });
  if (last < body.length) parts.push(body.slice(last));
  return parts;
}

function SkillDetail({ skill, onBack }) {
  const detail = Forgify.skillBodies[skill.id];
  const fm = detail?.frontmatter || { name: skill.name, description: skill.description, scope: skill.source };
  const body = detail?.body || "";

  return (
    <div className="page">
      <div className="page-header" style={{ paddingTop: 18 }}>
        <div className="page-header-text" style={{ gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "var(--fg-muted)" }}>
            <button onClick={onBack} className="btn btn-xs btn-ghost">← 返回</button>
            <span>·</span>
            <KindChip kind="skill" />
            <span className="cell-mono" style={{ color: "var(--fg-faint)" }}>{skill.id}</span>
          </div>

          <div className="page-subtitle" style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
            <span>{fm.description || skill.description}</span>
            <EntityRelMeta entityId={skill.id} />
          </div>
        </div>
        <div className="page-actions">
          <span className="badge success"><span className="dot" />active</span>
          <button className="btn btn-sm"><Icon.Folder /> 打开文件夹</button>
          <button className="btn btn-sm"><Icon.Play /> 试运行</button>
          <button className="btn btn-sm btn-accent"><Icon.Sparkles /> 提示 agent 使用</button>
        </div>
      </div>

      <div className="split">
        <div className="pane-main">
          <div className="sk-body">
            <h2 style={{ display: "flex", alignItems: "center", gap: 8 }}>
              SKILL.md
              <span className="badge muted" style={{ fontSize: 10 }}>预览</span>
            </h2>
            <pre className="sk-source">{body && <span>{highlightPlaceholders(body)}</span>}</pre>
          </div>
        </div>
        <aside className="pane-aside">
          <div className="aside-section">
            <div className="aside-label">Frontmatter</div>
            <div className="fm">
              {Object.entries(fm).map(([k, v]) => <FrontmatterField key={k} k={k} v={v} />)}
            </div>
          </div>

          <div className="aside-section">
            <div className="aside-label">占位符</div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6, fontSize: 12 }}>
              <div className="cell-flex"><code style={{ fontFamily: "var(--font-mono)", color: "var(--accent)" }}>$1</code> 第一个位置参数</div>
              <div className="cell-flex"><code style={{ fontFamily: "var(--font-mono)", color: "var(--accent)" }}>$ARGUMENTS</code> 全部位置参数</div>
              <div className="cell-flex"><code style={{ fontFamily: "var(--font-mono)", color: "var(--accent)" }}>${"{CLAUDE_*}"}</code> 环境变量</div>
            </div>
          </div>

          <div className="aside-section">
            <div className="aside-label">最近调用</div>
            <div style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 12, fontFamily: "var(--font-mono)" }}>
              <div className="cell-flex" style={{ color: "var(--fg-muted)" }}>
                <span className="dot" style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--status-success)" }} />
                <span>4 分钟前 · 240ms</span>
              </div>
              <div className="cell-flex" style={{ color: "var(--fg-muted)" }}>
                <span className="dot" style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--status-success)" }} />
                <span>3 小时前 · 219ms</span>
              </div>
              <div className="cell-flex" style={{ color: "var(--fg-muted)" }}>
                <span className="dot" style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--status-error)" }} />
                <span>2 天前 · 1.4s · IOError</span>
              </div>
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}

function SkillsView() {
  const [open, setOpen] = useSkState(null);
  React.useEffect(() => {
    const id = window.Shell?.focusEntity?.skills;
    if (id) {
      const s = Forgify.skills.find(x => x.id === id);
      if (s) setOpen(s);
    }
  }, []);
  if (open) return <SkillDetail skill={open} onBack={() => setOpen(null)} />;

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Sparkles /> Skills</div>
          <div className="page-subtitle">SKILL.md 模板</div>
        </div>
        <div className="page-actions">
          <button className="btn btn-sm"><Icon.Folder /> 打开 skills 目录</button>
          <button className="btn btn-sm btn-accent"><Icon.Plus /> 新建</button>
        </div>
      </div>
      <div className="page-toolbar">
        <div className="search-input">
          <Icon.Search className="icon" />
          <input placeholder="搜索 skill…" />
        </div>
      </div>
      <div className="page-body" style={{ padding: 0 }}>
        <div className="sk-list">
          {Forgify.skills.map(s => (
            <SkillRow key={s.id} skill={s} active={false} onClick={() => setOpen(s)} />
          ))}
        </div>
      </div>
    </div>
  );
}

window.SkillsView = SkillsView;
