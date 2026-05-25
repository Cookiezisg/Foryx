// VersionRail — collapsible right-rail version picker shared by
// Function / Handler / Workflow detail. Surfaces pending (warn),
// current (success), deployed (accent) state. Pending banner offers
// quick Accept / Revert / Diff actions.
//
// VersionRail —— trinity 三种详情共用的右侧版本栏；pending 显著高亮。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "../primitives/Icon.jsx";

export function VersionRail({
  versions,
  currentId, pendingId, deployedId,
  selectedId, onSelect,
  onAccept, onRevert, onRollback, onDeploy,
  showDeploy,
}) {
  const { t } = useTranslation("misc");
  const [collapsed, setCollapsed] = useState(false);
  const pending = versions.find((v) => v.id === pendingId);

  return (
    <aside className={"vr-rail" + (collapsed ? " is-collapsed" : "")}>
      <div className="vr-rail-head">
        <button className="vr-collapse" onClick={() => setCollapsed((c) => !c)} title={collapsed ? t("versionRail.expandTitle") : t("versionRail.collapseTitle")}>
          <Icon.GitBranch />
          {!collapsed && <span>{t("versionRail.versionCount", { count: versions.length })}</span>}
        </button>
      </div>

      {!collapsed && pending && (
        <div className="vr-pending-banner">
          <div className="vr-pending-head">
            <Icon.Sparkles style={{ width: 12, height: 12, color: "var(--status-warn)" }} />
            <span>{t("versionRail.pendingBanner")}</span>
          </div>
          <div className="vr-pending-summary">{pending.summary || pending.description || t("versionRail.noSummary")}</div>
          <div className="vr-pending-actions">
            <button className="btn btn-xs btn-danger" onClick={onRevert}>{t("versionRail.revert")}</button>
            <button className="btn btn-xs" onClick={() => onSelect?.(pendingId)}>{t("versionRail.viewDiff")}</button>
            <button className="btn btn-xs btn-accent" onClick={onAccept}>{t("versionRail.accept")}</button>
          </div>
        </div>
      )}

      {!collapsed && (
        <div className="vr-list">
          {versions.map((v) => (
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
          {versions.map((v) => (
            <button
              key={v.id}
              className={"vr-collapsed-dot" + (v.id === selectedId ? " is-selected" : "")}
              title={(v.label || v.versionLabel || "") + (v.summary ? " · " + v.summary : "")}
              onClick={() => onSelect?.(v.id)}
            >
              <span
                className="vr-dot"
                style={{
                  background: v.id === pendingId ? "var(--status-warn)"
                    : v.id === currentId ? "var(--status-success)"
                    : v.id === deployedId ? "var(--accent)"
                    : "var(--fg-faint)",
                }}
              />
              <span className="vr-num-small">{v.label || v.versionLabel}</span>
            </button>
          ))}
        </div>
      )}

      {!collapsed && showDeploy && deployedId && deployedId !== currentId && (
        <div className="vr-deploy-bar">
          <div style={{ fontSize: 11, color: "var(--fg-muted)" }}>
            {t("versionRail.deployBar", { deployedId, currentId })}
          </div>
          <button className="btn btn-xs btn-accent" onClick={onDeploy}>
            <Icon.Play /> {t("versionRail.deploy")}
          </button>
        </div>
      )}
    </aside>
  );
}

function VersionRow({ v, isCurrent, isPending, isDeployed, isSelected, onClick }) {
  const { t, i18n } = useTranslation("misc");
  const locale = i18n.language === "zh" ? "zh-CN" : "en-US";
  const dotColor = isPending ? "var(--status-warn)"
    : isCurrent ? "var(--status-success)"
    : isDeployed ? "var(--accent)"
    : "var(--fg-faint)";
  return (
    <button
      className={"vr-row" + (isSelected ? " is-selected" : "")}
      onClick={onClick}
    >
      <span className="vr-dot" style={{ background: dotColor }} />
      <div className="vr-meta">
        <div className="vr-head">
          <span className="vr-num">{v.label || v.versionLabel || v.id?.slice(-8)}</span>
          {isPending && <span className="vr-badge vr-pending"><Icon.Sparkles /> {t("versionRail.badgePending")}</span>}
          {isCurrent && <span className="vr-badge vr-current">{t("versionRail.badgeCurrent")}</span>}
          {isDeployed && <span className="vr-badge vr-deployed">{t("versionRail.badgeDeployed")}</span>}
        </div>
        <div className="vr-summary">
          {v.summary || v.description || <span style={{ color: "var(--fg-faint)" }}>{t("versionRail.noSummary")}</span>}
        </div>
        <div className="vr-foot">
          <span className="vr-author">{v.author || t("versionRail.youFallback")}</span>
          {(v.at || v.createdAt) && (
            <>
              <span className="vr-sep">·</span>
              <span className="vr-time">{v.at || new Date(v.createdAt).toLocaleString(locale, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}</span>
            </>
          )}
        </div>
      </div>
    </button>
  );
}

// SplitDiff — line-level LCS diff, side-by-side render.
//
// SplitDiff —— LCS 行级 diff，左右并排渲染。
export function SplitDiff({ leftLabel, rightLabel, leftSrc, rightSrc }) {
  const rows = computeSplitDiff(leftSrc || "", rightSrc || "");
  const adds = rows.filter((r) => r.op === "add").length;
  const dels = rows.filter((r) => r.op === "del").length;
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

function computeSplitDiff(a, b) {
  const al = a.split("\n"), bl = b.split("\n");
  const m = al.length, n = bl.length;
  const dp = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
  for (let i = m - 1; i >= 0; i--) {
    for (let j = n - 1; j >= 0; j--) {
      if (al[i] === bl[j]) dp[i][j] = dp[i + 1][j + 1] + 1;
      else dp[i][j] = Math.max(dp[i + 1][j], dp[i][j + 1]);
    }
  }
  const rows = [];
  let i = 0, j = 0;
  while (i < m && j < n) {
    if (al[i] === bl[j]) { rows.push({ leftN: i + 1, rightN: j + 1, left: al[i], right: bl[j], op: "eq" }); i++; j++; }
    else if (dp[i + 1][j] >= dp[i][j + 1]) { rows.push({ leftN: i + 1, rightN: null, left: al[i], right: "", op: "del" }); i++; }
    else { rows.push({ leftN: null, rightN: j + 1, left: "", right: bl[j], op: "add" }); j++; }
  }
  while (i < m) { rows.push({ leftN: i + 1, rightN: null, left: al[i], right: "", op: "del" }); i++; }
  while (j < n) { rows.push({ leftN: null, rightN: j + 1, left: "", right: bl[j], op: "add" }); j++; }
  return rows;
}

// CodeView — single-source code render with Python-ish syntax highlight.
// Tokenises with a simple state machine so quoted strings (single or
// double, including embedded code-looking tokens) don't get split.
//
// CodeView —— 单源代码视图；用 state machine 分词，避免 quote 内字符串
// 被误识别为关键字 (PRD §16 boilerplate CodeView regex bug 修复)。
const KEYWORDS = new Set([
  "def", "return", "for", "in", "if", "else", "elif", "from", "import", "class",
  "is", "not", "None", "True", "False", "and", "or", "lambda", "with", "as",
  "try", "except", "finally", "raise", "while", "break", "continue", "pass",
  "yield", "global", "nonlocal", "assert",
]);
const BUILTINS = new Set([
  "len", "sum", "range", "list", "dict", "tuple", "set", "str", "int", "float",
  "print", "open", "abs", "min", "max", "enumerate", "zip", "map", "filter",
  "sorted", "reversed", "isinstance", "type",
]);

function tokenisePython(line) {
  const out = [];
  let i = 0;
  const n = line.length;

  while (i < n) {
    const ch = line[i];

    if (ch === "#") {
      out.push({ k: "com", t: line.slice(i) });
      return out;
    }

    if (ch === "'" || ch === '"') {
      let j = i + 1;
      while (j < n) {
        if (line[j] === "\\" && j + 1 < n) { j += 2; continue; }
        if (line[j] === ch) { j++; break; }
        j++;
      }
      out.push({ k: "str", t: line.slice(i, j) });
      i = j;
      continue;
    }

    if (/\s/.test(ch)) {
      let j = i;
      while (j < n && /\s/.test(line[j])) j++;
      out.push({ k: "ws", t: line.slice(i, j) });
      i = j;
      continue;
    }

    if (/[A-Za-z_]/.test(ch)) {
      let j = i;
      while (j < n && /[A-Za-z0-9_]/.test(line[j])) j++;
      const word = line.slice(i, j);
      if (KEYWORDS.has(word))      out.push({ k: "kw", t: word });
      else if (BUILTINS.has(word)) out.push({ k: "bi", t: word });
      else                          out.push({ k: "id", t: word });
      i = j;
      continue;
    }

    if (/\d/.test(ch)) {
      let j = i;
      while (j < n && /[\d.]/.test(line[j])) j++;
      out.push({ k: "num", t: line.slice(i, j) });
      i = j;
      continue;
    }

    out.push({ k: "punc", t: ch });
    i++;
  }

  return out;
}

export function CodeView({ src, lang = "python" }) {
  const lines = (src || "").split("\n");
  return (
    <pre className="codeview">
      {lines.map((line, i) => (
        <div key={i} className="codeview-row">
          <span className="codeview-ln">{i + 1}</span>
          <span className="codeview-line">
            {tokenisePython(line).map((tok, j) => {
              if (tok.k === "str") return <span key={j} className="tok-str">{tok.t}</span>;
              if (tok.k === "com") return <span key={j} className="tok-com">{tok.t}</span>;
              if (tok.k === "kw")  return <span key={j} className="tok-kw">{tok.t}</span>;
              if (tok.k === "bi")  return <span key={j} className="tok-bi">{tok.t}</span>;
              if (tok.k === "num") return <span key={j} className="tok-num">{tok.t}</span>;
              return <span key={j}>{tok.t}</span>;
            })}
          </span>
        </div>
      ))}
    </pre>
  );
}
