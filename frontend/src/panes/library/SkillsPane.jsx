// SkillsPane — list of SKILL.md cards. Click a card opens a drawer with
// frontmatter + body markdown.
//
// SkillsPane —— Skill 列表 + 详情 Drawer。

import { useMemo, useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { useSkills } from "../../api/library.js";

export function SkillsPane() {
  const { data: skills = [], isLoading } = useSkills();
  const [q, setQ] = useState("");
  const [openSkill, setOpenSkill] = useState(null);

  const filtered = useMemo(() => {
    if (!q) return skills;
    const ql = q.toLowerCase();
    return skills.filter((s) =>
      (s.name || "").toLowerCase().includes(ql)
      || (s.description || "").toLowerCase().includes(ql)
    );
  }, [skills, q]);

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Sparkles /> Skills</div>
          <div className="page-subtitle">SKILL.md 模板库</div>
        </div>
        <div className="page-actions">
          <Button size="sm"><Icon.Inbox /> 导入</Button>
        </div>
      </div>

      <div className="page-toolbar">
        <div className="search-input" style={{ maxWidth: 320 }}>
          <Icon.Search className="icon" />
          <input placeholder="搜技能名 / 描述…" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <div style={{ flex: 1 }} />
        <span style={{ fontSize: 11, color: "var(--fg-faint)", fontFamily: "var(--font-mono)" }}>
          {filtered.length} 项
        </span>
      </div>

      <div className="page-body" style={{ padding: 24 }}>
        {isLoading ? <div className="empty"><div className="sub">加载中…</div></div>
          : filtered.length === 0 ? (
            <div className="empty">
              <Icon.Sparkles className="icon" />
              <div className="title">还没有 Skill</div>
              <div className="sub">把 SKILL.md 文件放到 ~/.forgify/skills/ 即可被自动发现</div>
            </div>
          ) : (
            <div className="card-grid">
              {filtered.map((s) => (
                <div key={s.id || s.name} className="card" onClick={() => setOpenSkill(s)}>
                  <div className="card-head">
                    <div className="card-title">{s.name}</div>
                    {s.activated && <span className="badge success"><span className="dot" />已激活</span>}
                  </div>
                  <div className="card-desc" style={{ display: "-webkit-box", WebkitLineClamp: 3, WebkitBoxOrient: "vertical", overflow: "hidden" }}>
                    {s.description}
                  </div>
                  {s.tags?.length > 0 && (
                    <div style={{ display: "flex", gap: 4, flexWrap: "wrap", marginTop: 8 }}>
                      {s.tags.slice(0, 5).map((t) => (
                        <span key={t} className="badge muted">{t}</span>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
      </div>

      {openSkill && <SkillDrawer skill={openSkill} onClose={() => setOpenSkill(null)} />}
    </div>
  );
}

function SkillDrawer({ skill, onClose }) {
  return (
    <div className="drawer-wrap is-open">
      <div className="drawer-scrim" onClick={onClose} />
      <div className="drawer" style={{ width: 560 }}>
        <div className="drawer-head">
          <div className="drawer-title">{skill.name}</div>
          <button className="icon-btn" onClick={onClose}><Icon.X /></button>
        </div>
        <div style={{ padding: 20, overflowY: "auto" }}>
          <div style={{ fontSize: 13, color: "var(--fg-muted)", marginBottom: 16 }}>
            {skill.description}
          </div>
          {skill.body && (
            <pre style={{
              whiteSpace: "pre-wrap", fontFamily: "var(--font-sans)", fontSize: 13, lineHeight: 1.6,
              color: "var(--fg-body)",
            }}>{skill.body}</pre>
          )}
        </div>
      </div>
    </div>
  );
}
