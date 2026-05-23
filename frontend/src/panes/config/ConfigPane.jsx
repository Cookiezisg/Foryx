// ConfigPane — 5 tabs: API Keys / Model / Sandbox / 外观 / 数据.
//
// ConfigPane —— API Keys / Model / Sandbox / 外观 / 数据。

import { useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { Badge } from "../../components/primitives/Badge.jsx";
import { RelTime } from "../../components/shared/RelTime.jsx";
import {
  useApiKeys, useProviders, useCreateApiKey, useDeleteApiKey, useTestApiKey,
  useModelConfigs, useUpsertModelConfig,
} from "../../api/config.js";
import { useSettings } from "../../store/settings.js";
import { useUIStore } from "../../store/ui.js";

const TABS = [
  ["keys",       "API Keys"],
  ["models",     "Model"],
  ["sandbox",    "Sandbox"],
  ["appearance", "外观"],
  ["data",       "数据"],
];

export function ConfigPane() {
  const [tab, setTab] = useState("keys");
  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Settings /> 设置</div>
          <div className="page-subtitle">凭证 / 模型 / 沙箱 / 外观 / 数据</div>
        </div>
      </div>
      <div className="page-tabs">
        {TABS.map(([k, l]) => (
          <button key={k} className={"page-tab" + (tab === k ? " is-active" : "")} onClick={() => setTab(k)}>
            {l}
          </button>
        ))}
      </div>
      <div className="page-body">
        {tab === "keys"       && <ApiKeysTab />}
        {tab === "models"     && <ModelsTab />}
        {tab === "sandbox"    && <SandboxTab />}
        {tab === "appearance" && <AppearanceTab />}
        {tab === "data"       && <DataTab />}
      </div>
    </div>
  );
}

// ── API Keys ──────────────────────────────────────────────────────────
function ApiKeysTab() {
  const { data: keys = [] } = useApiKeys();
  const del = useDeleteApiKey();
  const test = useTestApiKey();
  const pushToast = useUIStore((s) => s.pushToast);
  const [showAdd, setShowAdd] = useState(false);

  return (
    <>
      <table className="t">
        <thead>
          <tr>
            <th style={{ paddingLeft: 16 }}>Provider</th>
            <th>名称</th>
            <th>Key</th>
            <th>状态</th>
            <th>最近</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {keys.length === 0 && (
            <tr><td colSpan={6} style={{ padding: 24, textAlign: "center", color: "var(--fg-faint)" }}>
              还没添加任何 key — 点右下角添加。
            </td></tr>
          )}
          {keys.map((k) => (
            <tr key={k.id}>
              <td style={{ paddingLeft: 16 }}>
                <div className="cell-flex">
                  <div style={{
                    width: 22, height: 22, borderRadius: 4,
                    background: "var(--bg-elev-2)", border: "1px solid var(--border-soft)",
                    display: "grid", placeItems: "center",
                    fontFamily: "var(--font-mono)", fontSize: 10, fontWeight: 700, color: "var(--fg-muted)",
                  }}>
                    {(k.provider || "?").slice(0, 2).toUpperCase()}
                  </div>
                  <span className="cell-strong">{k.provider}</span>
                </div>
              </td>
              <td>{k.displayName || k.name || "—"}</td>
              <td><span className="cell-mono">{k.masked || k.maskedKey || "•••"}</span></td>
              <td>
                {k.verified || k.testStatus === "ok"
                  ? <Badge kind="success">verified</Badge>
                  : <Badge>unverified</Badge>}
              </td>
              <td><span style={{ fontSize: 12, color: "var(--fg-muted)" }}>
                {k.lastUsed ? <RelTime ts={k.lastUsed} /> : "—"}
              </span></td>
              <td className="col-tight">
                <button
                  className="icon-btn"
                  title="测试连通"
                  onClick={() => test.mutate(k.id, {
                    onSuccess: () => pushToast({ kind: "success", title: "测试通过", desc: k.provider }),
                    onError: (e) => pushToast({ kind: "error", title: "测试失败", desc: e.message }),
                  })}
                >
                  <Icon.Refresh />
                </button>
                <button
                  className="icon-btn"
                  title="删除"
                  onClick={() => {
                    if (confirm(`确认删除 ${k.provider} · ${k.displayName || ""}?`)) {
                      del.mutate(k.id);
                    }
                  }}
                >
                  <Icon.Trash />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div style={{ padding: 16, borderTop: "1px solid var(--border-soft)" }}>
        <Button size="sm" variant="accent" onClick={() => setShowAdd(true)}>
          <Icon.Plus /> 添加 Provider
        </Button>
      </div>
      {showAdd && <AddKeyDrawer onClose={() => setShowAdd(false)} />}
    </>
  );
}

function AddKeyDrawer({ onClose }) {
  const { data: providers = [] } = useProviders();
  const create = useCreateApiKey();
  const test = useTestApiKey();
  const pushToast = useUIStore((s) => s.pushToast);

  const llmProviders = providers.filter((p) => p.category === "llm");

  const [provider, setProvider] = useState(llmProviders[0]?.name || "deepseek");
  const [displayName, setDisplayName] = useState("");
  const [secret, setSecret] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const submit = async () => {
    if (!provider || !secret) {
      pushToast({ kind: "warn", title: "Provider 和 Key 必填" });
      return;
    }
    setSubmitting(true);
    try {
      const body = { provider, displayName: displayName || provider, key: secret };
      if (baseUrl) body.baseUrl = baseUrl;
      const created = await create.mutateAsync(body);
      if (created?.id) {
        test.mutate(created.id, {
          onSuccess: () => pushToast({ kind: "success", title: "Key 已加入并测试通过" }),
          onError: (e) => pushToast({ kind: "warn", title: "Key 已加入但测试失败", desc: e.message }),
        });
      }
      onClose();
    } catch (e) {
      pushToast({ kind: "error", title: "添加失败", desc: e.message });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="drawer-wrap is-open">
      <div className="drawer-scrim" onClick={onClose} />
      <div className="drawer" style={{ width: 420 }}>
        <div className="drawer-head">
          <div className="drawer-title">添加 API Key</div>
          <button className="icon-btn" onClick={onClose}><Icon.X /></button>
        </div>
        <div style={{ padding: 16, display: "flex", flexDirection: "column", gap: 12 }}>
          <div>
            <div className="cfg-label">Provider</div>
            <select className="cfg-input" value={provider} onChange={(e) => setProvider(e.target.value)}>
              {(llmProviders.length ? llmProviders : providers).map((p) => (
                <option key={p.name} value={p.name}>{p.displayName || p.name}</option>
              ))}
            </select>
          </div>
          <div>
            <div className="cfg-label">显示名 (可选)</div>
            <input className="cfg-input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder={provider} />
          </div>
          <div>
            <div className="cfg-label">Key</div>
            <input
              className="cfg-input"
              type="password"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              placeholder="sk-…"
              autoFocus
            />
          </div>
          <div>
            <div className="cfg-label">Base URL (可选)</div>
            <input className="cfg-input" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="留空走默认端点" />
          </div>
          <div style={{ display: "flex", gap: 8, justifyContent: "flex-end", paddingTop: 8 }}>
            <Button size="sm" variant="ghost" onClick={onClose}>取消</Button>
            <Button size="sm" variant="accent" loading={submitting} disabled={submitting} onClick={submit}>
              <Icon.Check /> 测试并保存
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ── Model Configs ─────────────────────────────────────────────────────
function ModelsTab() {
  const { data: configs = [] } = useModelConfigs();
  const { data: keys = [] } = useApiKeys();
  const upsert = useUpsertModelConfig();
  const pushToast = useUIStore((s) => s.pushToast);
  const [editing, setEditing] = useState(null);

  const scenarios = configs.length > 0 ? configs.map((c) => c.scenario) : ["chat", "auto_title", "web_summary", "intent", "compaction"];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
      {scenarios.map((scenario) => {
        const cfg = configs.find((c) => c.scenario === scenario) || { scenario };
        return (
          <div key={scenario} className="card" style={{ flexDirection: "row", alignItems: "center", gap: 14, cursor: "default" }}>
            <div style={{
              width: 130, fontFamily: "var(--font-mono)", fontSize: 12, fontWeight: 600,
              color: "var(--fg-muted)", textTransform: "uppercase", letterSpacing: "0.05em",
            }}>
              {scenario}
            </div>
            <div style={{ flex: 1 }}>
              <div className="card-title" style={{ fontFamily: "var(--font-mono)", fontSize: 14 }}>
                {cfg.modelId || cfg.modelID || <span style={{ color: "var(--fg-faint)" }}>未配置</span>}
              </div>
              <div className="card-desc" style={{ marginTop: 2 }}>
                via <strong>{cfg.provider || "?"}</strong>
              </div>
            </div>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setEditing({ scenario, provider: cfg.provider || keys[0]?.provider || "deepseek", modelId: cfg.modelId || cfg.modelID || "" })}
            >
              {cfg.modelId ? "切换" : "配置"}
            </Button>
          </div>
        );
      })}

      {editing && (
        <div className="drawer-wrap is-open">
          <div className="drawer-scrim" onClick={() => setEditing(null)} />
          <div className="drawer" style={{ width: 380 }}>
            <div className="drawer-head">
              <div className="drawer-title">配置 {editing.scenario}</div>
              <button className="icon-btn" onClick={() => setEditing(null)}><Icon.X /></button>
            </div>
            <div style={{ padding: 16, display: "flex", flexDirection: "column", gap: 12 }}>
              <div>
                <div className="cfg-label">Provider</div>
                <select
                  className="cfg-input"
                  value={editing.provider}
                  onChange={(e) => setEditing({ ...editing, provider: e.target.value })}
                >
                  {keys.map((k) => (
                    <option key={k.id} value={k.provider}>{k.provider}{k.displayName ? ` (${k.displayName})` : ""}</option>
                  ))}
                </select>
                {keys.length === 0 && (
                  <div style={{ fontSize: 11, color: "var(--status-warn)", marginTop: 4 }}>
                    先去 API Keys tab 添加一个 key
                  </div>
                )}
              </div>
              <div>
                <div className="cfg-label">Model ID</div>
                <input
                  className="cfg-input"
                  value={editing.modelId}
                  onChange={(e) => setEditing({ ...editing, modelId: e.target.value })}
                  placeholder="例如 deepseek-chat / claude-sonnet-4-6"
                />
                {(() => {
                  const k = keys.find((kk) => kk.provider === editing.provider);
                  const found = k?.modelsFound || [];
                  if (!found.length) return null;
                  return (
                    <div style={{ marginTop: 6, display: "flex", flexWrap: "wrap", gap: 4 }}>
                      {found.slice(0, 10).map((m) => (
                        <button
                          key={m}
                          className="ask-ai-pop-sug"
                          onClick={() => setEditing({ ...editing, modelId: m })}
                        >
                          {m}
                        </button>
                      ))}
                    </div>
                  );
                })()}
              </div>
              <div style={{ display: "flex", gap: 8, justifyContent: "flex-end", paddingTop: 8 }}>
                <Button size="sm" variant="ghost" onClick={() => setEditing(null)}>取消</Button>
                <Button
                  size="sm"
                  variant="accent"
                  onClick={() => {
                    upsert.mutate({ scenario: editing.scenario, provider: editing.provider, modelId: editing.modelId }, {
                      onSuccess: () => {
                        pushToast({ kind: "success", title: "已保存", desc: editing.scenario });
                        setEditing(null);
                      },
                      onError: (e) => pushToast({ kind: "error", title: "保存失败", desc: e.message }),
                    });
                  }}
                >
                  保存
                </Button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ── Sandbox ───────────────────────────────────────────────────────────
function SandboxTab() {
  return (
    <div style={{ padding: 24, color: "var(--fg-muted)" }}>
      <div className="card" style={{ cursor: "default" }}>
        <div className="card-head">
          <div className="card-title">mise runtime</div>
          <Badge kind="success">embedded</Badge>
        </div>
        <div style={{ fontSize: 12, color: "var(--fg-faint)", marginTop: 6 }}>
          二进制随后端打包；python / node 等 runtime 按需 install。详情见 sandbox.md。
        </div>
      </div>
    </div>
  );
}

// ── Appearance ────────────────────────────────────────────────────────
function AppearanceTab() {
  const settings = useSettings();
  const Row = ({ label, children }) => (
    <div style={{ display: "flex", alignItems: "center", gap: 16, padding: "12px 0", borderBottom: "1px solid var(--border-soft)" }}>
      <div style={{ width: 120, fontSize: 13, color: "var(--fg-muted)" }}>{label}</div>
      <div style={{ flex: 1, display: "flex", gap: 6, flexWrap: "wrap" }}>{children}</div>
    </div>
  );
  const Pill = ({ active, onClick, children }) => (
    <button className={"btn btn-xs" + (active ? " btn-primary" : " btn-ghost")} onClick={onClick}>
      {children}
    </button>
  );
  return (
    <div style={{ padding: "12px 24px", maxWidth: 720 }}>
      <Row label="主题">
        {["system", "light", "dark"].map((v) => (
          <Pill key={v} active={settings.theme === v} onClick={() => settings.set({ theme: v })}>{v}</Pill>
        ))}
      </Row>
      <Row label="Accent">
        {[["claude", "#d97757"], ["blue", "#2383e2"], ["ink", "#37352f"], ["green", "#0f7b6c"], ["purple", "#6940a5"]].map(([k, c]) => (
          <button
            key={k}
            className={"settings-pop-swatch" + (settings.accent === k ? " is-active" : "")}
            style={{ background: c, width: 24, height: 24, borderRadius: 6, border: "2px solid var(--border-soft)", cursor: "pointer" }}
            title={k}
            onClick={() => settings.set({ accent: k })}
          />
        ))}
      </Row>
      <Row label="密度">
        {["compact", "cozy", "comfortable"].map((v) => (
          <Pill key={v} active={settings.density === v} onClick={() => settings.set({ density: v })}>{v}</Pill>
        ))}
      </Row>
      <Row label="语言">
        {[["zh", "中文"], ["en", "English"]].map(([v, l]) => (
          <Pill key={v} active={settings.lang === v} onClick={() => settings.set({ lang: v })}>{l}</Pill>
        ))}
      </Row>
      <Row label="reasoning">
        {["collapsed", "expanded"].map((v) => (
          <Pill key={v} active={settings.reasoningDefault === v} onClick={() => settings.set({ reasoningDefault: v })}>{v}</Pill>
        ))}
      </Row>
    </div>
  );
}

// ── Data ──────────────────────────────────────────────────────────────
function DataTab() {
  return (
    <div style={{ padding: 24 }}>
      <div className="card" style={{ cursor: "default" }}>
        <div className="card-head">
          <div className="card-title">数据目录</div>
        </div>
        <div style={{ fontSize: 12, color: "var(--fg-muted)", fontFamily: "var(--font-mono)" }}>
          ~/.forgify/
        </div>
        <div style={{ fontSize: 11, color: "var(--fg-faint)", marginTop: 6 }}>
          所有数据本地存储；包括 SQLite DB、加密 API keys、Skill 库、Memory、Workflow flowrun 历史。
        </div>
      </div>
    </div>
  );
}
