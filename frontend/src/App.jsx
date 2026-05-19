// Phase 1 design-system showcase. Renders every primitive + shared
// component against the migrated tokens/CSS so visual regressions vs the
// boilerplate are obvious at a glance. Replaced by the real AppShell in
// Phase 2.
//
// Phase 1 设计系统展示页；Phase 2 进 AppShell 后被替换。

import { useEffect, useState } from "react";
import { getBaseUrl } from "./bridge/wails";
import { Icon } from "./components/primitives/Icon.jsx";
import { Button } from "./components/primitives/Button.jsx";
import { Badge } from "./components/primitives/Badge.jsx";
import { Spinner } from "./components/primitives/Spinner.jsx";
import { Kbd } from "./components/primitives/Kbd.jsx";
import { RelTime } from "./components/shared/RelTime.jsx";
import { KindChip } from "./components/shared/KindChip.jsx";
import { StatusBadge } from "./components/shared/StatusBadge.jsx";
import { ActionMenu } from "./components/shared/ActionMenu.jsx";

function Section({ title, children }) {
  return (
    <section style={{ marginBottom: 28 }}>
      <h3 style={{
        fontSize: 11, color: "var(--fg-faint)",
        textTransform: "uppercase", letterSpacing: "0.06em",
        margin: "0 0 12px 0", fontWeight: 600,
      }}>{title}</h3>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 12, alignItems: "center" }}>
        {children}
      </div>
    </section>
  );
}

export default function App() {
  const [health, setHealth] = useState("checking…");
  const [theme, setTheme] = useState("light");
  const [accent, setAccent] = useState("claude");

  useEffect(() => {
    const base = getBaseUrl();
    fetch(base + "/api/v1/health")
      .then((r) => r.json())
      .then((j) => setHealth(j.data?.status || "unknown"))
      .catch((e) => setHealth("error: " + e.message));
  }, []);

  useEffect(() => { document.documentElement.dataset.theme = theme; }, [theme]);
  useEffect(() => { document.documentElement.dataset.accent = accent; }, [accent]);

  return (
    <div style={{
      padding: "40px 60px", maxWidth: 1000, margin: "0 auto",
      overflow: "auto", height: "100vh",
    }}>
      <header style={{ marginBottom: 32 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <div style={{ fontSize: 22, fontWeight: 600, color: "var(--fg-strong)" }}>
            Forgify · Design System
          </div>
          <Badge kind={health === "ok" ? "success" : "error"}>backend {health}</Badge>
        </div>
        <div style={{ marginTop: 12, display: "flex", gap: 16, fontSize: 12, color: "var(--fg-muted)" }}>
          <label>
            theme:
            <select value={theme} onChange={(e) => setTheme(e.target.value)}
              style={{ marginLeft: 6, padding: "2px 6px" }}>
              <option value="light">light</option>
              <option value="dark">dark</option>
            </select>
          </label>
          <label>
            accent:
            <select value={accent} onChange={(e) => setAccent(e.target.value)}
              style={{ marginLeft: 6, padding: "2px 6px" }}>
              <option value="claude">claude</option>
              <option value="blue">blue</option>
              <option value="ink">ink</option>
              <option value="green">green</option>
              <option value="purple">purple</option>
            </select>
          </label>
        </div>
      </header>

      <Section title="Buttons">
        <Button>Default</Button>
        <Button variant="primary">Primary</Button>
        <Button variant="accent"><Icon.Plus />Accent</Button>
        <Button variant="ghost">Ghost</Button>
        <Button variant="danger"><Icon.Trash />Danger</Button>
        <Button size="sm">Small</Button>
        <Button size="xs">XS</Button>
        <Button loading>Loading</Button>
        <Button disabled>Disabled</Button>
      </Section>

      <Section title="Icon Buttons">
        <button className="icon-btn" title="Search"><Icon.Search /></button>
        <button className="icon-btn" title="Settings"><Icon.Settings /></button>
        <button className="icon-btn" title="Close"><Icon.X /></button>
        <button className="icon-btn" title="More"><Icon.MoreHorizontal /></button>
        <ActionMenu items={[
          { label: "重命名", icon: Icon.Edit, onClick: () => console.log("rename") },
          { label: "复制", icon: Icon.Copy, shortcut: "⌘D" },
          "divider",
          { label: "删除", icon: Icon.Trash, danger: true, shortcut: "⌫" },
        ]} />
      </Section>

      <Section title="Badges">
        <Badge>neutral</Badge>
        <Badge kind="success">success</Badge>
        <Badge kind="error">error</Badge>
        <Badge kind="warn">warn</Badge>
        <Badge kind="info">info</Badge>
        <Badge kind="streaming">streaming</Badge>
        <Badge kind="muted">v1.2.3</Badge>
      </Section>

      <Section title="Kind Chips">
        <KindChip kind="function" />
        <KindChip kind="handler" />
        <KindChip kind="workflow" />
        <KindChip kind="skill" />
        <KindChip kind="mcp" />
      </Section>

      <Section title="Status Badges">
        <StatusBadge status="ready" />
        <StatusBadge status="pending" />
        <StatusBadge status="draft" />
        <StatusBadge status="failed" />
      </Section>

      <Section title="Spinners & Loading">
        <Spinner size={12} />
        <Spinner size={16} />
        <Spinner size={20} color="var(--accent)" />
      </Section>

      <Section title="Keyboard Hints">
        <span>Open palette <Kbd>⌘</Kbd><Kbd>K</Kbd></span>
        <span>Toggle sidebar <Kbd>⌘</Kbd><Kbd>B</Kbd></span>
        <span>Send <Kbd>↵</Kbd></span>
        <span>Cancel <Kbd>esc</Kbd></span>
      </Section>

      <Section title="Relative Time">
        <RelTime ts={new Date()} />
        <RelTime ts={Date.now() - 30 * 1000} />
        <RelTime ts={Date.now() - 12 * 60 * 1000} />
        <RelTime ts={Date.now() - 4 * 60 * 60 * 1000} />
        <RelTime ts={Date.now() - 3 * 86400_000} />
        <RelTime ts="2025-11-04T08:30:00Z" />
      </Section>

      <Section title="Typography">
        <div style={{ width: "100%", display: "grid", gridTemplateColumns: "auto 1fr", gap: "8px 24px", alignItems: "baseline" }}>
          <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>fg-strong 14</span>
          <span style={{ color: "var(--fg-strong)", fontSize: 14 }}>The quick brown fox · 中文示例 · 1234567890</span>
          <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>fg-body 13</span>
          <span style={{ color: "var(--fg-body)", fontSize: 13 }}>The quick brown fox · 中文示例</span>
          <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>fg-muted 12</span>
          <span style={{ color: "var(--fg-muted)", fontSize: 12 }}>secondary copy · 辅助信息</span>
          <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>fg-faint 11</span>
          <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>caption · 注释级</span>
          <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>mono 12</span>
          <code style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>fn_a1b2c3d4 · const x = 42</code>
        </div>
      </Section>
    </div>
  );
}
