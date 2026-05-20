// Onboarding — 5-step first-run wizard.
//   0 Welcome — what Forgify is.
//   1 Account — create the first local profile (username + avatar color).
//   2 Look    — pick accent.
//   3 API Key — pick provider + key (skippable).
//   4 Done    — recap + 进入应用.
//
// Persists into ~/.forgify/ via real REST: POST /users → creates profile,
// POST /api-keys → stores key. Frontend marks settings.onboarded=true on
// finish so this never re-appears.
//
// Onboarding —— 5 步首次启动向导；进 Forgify 真后端写 user / api-key；
// 完成后 settings.onboarded=true 永不再现。

import { useEffect, useMemo, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { useQueryClient } from "@tanstack/react-query";
import { Icon } from "../primitives/Icon.jsx";
import { Button } from "../primitives/Button.jsx";
import { useSettings, applyTheme } from "../../store/settings.js";
import { useUIStore } from "../../store/ui.js";
import { useCreateUser, useUsers } from "../../api/users.js";
import { useProviders, useCreateApiKey, useTestApiKey, useUpsertModelConfig } from "../../api/config.js";

const ACCENTS = [
  ["claude", "#d97757", "Claude 橙"],
  ["blue",   "#2383e2", "Notion 蓝"],
  ["ink",    "#37352f", "墨"],
  ["green",  "#0f7b6c", "森林绿"],
  ["purple", "#6940a5", "紫"],
];
const PROVIDER_HINTS = {
  deepseek:  { abbr: "DS", color: "#4D6BFE" },
  anthropic: { abbr: "AN", color: "#D97757" },
  openai:    { abbr: "OA", color: "#10A37F" },
  qwen:      { abbr: "QW", color: "#615CED" },
  moonshot:  { abbr: "MS", color: "#37352F" },
  zhipu:     { abbr: "ZP", color: "#3870E0" },
  ollama:    { abbr: "OL", color: "#37352F" },
  google:    { abbr: "GO", color: "#4285F4" },
};

const STEPS = [
  { key: "intro",    title: "欢迎",      desc: "了解一下" },
  { key: "account",  title: "创建账号",  desc: "本地账号，仅在你这台机器" },
  { key: "look",     title: "外观",      desc: "主题色 · 后续可改" },
  { key: "provider", title: "API Key",   desc: "至少配一个（可跳过）" },
  { key: "done",     title: "就绪",      desc: "进入应用" },
];

export function Onboarding({ onFinish }) {
  const settings = useSettings();
  const qc = useQueryClient();
  const pushToast = useUIStore((s) => s.pushToast);
  const { data: existingUsers = [] } = useUsers();
  const createUser = useCreateUser();
  const { data: providers = [] } = useProviders();
  const createKey = useCreateApiKey();
  const testKey = useTestApiKey();
  const setModel = useUpsertModelConfig();

  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);

  // State the user collects via the wizard:
  const [name, setName] = useState("");
  const [accent, setAccent] = useState(settings.accent);
  const [provider, setProvider] = useState("");
  const [apiKey, setApiKey] = useState("");

  const llmProviders = providers.filter((p) => p.category === "llm");
  useEffect(() => {
    if (!provider && llmProviders.length > 0) setProvider(llmProviders[0].name);
  }, [provider, llmProviders]);

  // Live preview of accent on the wizard itself.
  useEffect(() => {
    applyTheme({ ...settings, accent });
  }, [accent]); // eslint-disable-line react-hooks/exhaustive-deps

  const canAdvance = () => {
    switch (STEPS[step].key) {
      case "intro":    return true;
      case "account":  return name.trim().length > 0;
      case "look":     return true;
      case "provider": return true; // skippable
      case "done":     return true;
    }
  };

  const finish = async () => {
    setBusy(true);
    try {
      // 1. Create the local user profile (idempotent: skip if "name" matches an existing username).
      const existing = existingUsers.find((u) => u.username === name.trim().toLowerCase());
      let user = existing;
      if (!user) {
        user = await createUser.mutateAsync({
          username: name.trim().toLowerCase().replace(/\s+/g, "-"),
          displayName: name.trim(),
          avatarColor: ACCENTS.find(([k]) => k === accent)?.[1] || "#4f46e5",
        });
      }
      settings.set({ activeUserId: user.id, accent, onboarded: true });

      // 2. If a key was provided, write it then test + set chat scenario.
      if (apiKey && provider) {
        // After settings.activeUserId is set, apiFetch sends X-Forgify-User-ID,
        // so the key + model-config land under THIS user.
        const k = await createKey.mutateAsync({
          provider, key: apiKey, displayName: `${provider} (onboarding)`,
        });
        testKey.mutate(k.id);
        await setModel.mutateAsync({ scenario: "chat", provider, modelId: pickDefaultModel(provider) });
      }

      qc.invalidateQueries();
      pushToast({ kind: "success", title: "欢迎使用 Forgify", desc: name.trim() });
      onFinish?.();
    } catch (err) {
      pushToast({ kind: "error", title: "初始化失败", desc: err.message });
    } finally {
      setBusy(false);
    }
  };

  const next = () => {
    if (step < STEPS.length - 1) setStep((s) => s + 1);
    else finish();
  };
  const prev = () => setStep((s) => Math.max(0, s - 1));

  // Keyboard: Enter advances, Esc nothing (user must complete or use button).
  useEffect(() => {
    const onKey = (e) => {
      if (e.key === "Enter" && !e.shiftKey && canAdvance() && !busy) {
        if (document.activeElement?.tagName === "INPUT") {
          // Enter on input also advances; default form behavior.
        }
        next();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [step, busy, name, accent, provider, apiKey]); // eslint-disable-line

  return (
    <AnimatePresence>
      <motion.div
        className="onb-stage-overlay"
        initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
      >
        <motion.div
          className="onb-stage"
          initial={{ opacity: 0, scale: 0.97, y: 8 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          transition={{ duration: 0.32, ease: [0.2, 0.8, 0.2, 1] }}
        >
          <aside className="onb-rail">
            <div className="onb-brand">
              <div className="onb-mark">F</div>
              <div>
                <div className="onb-brand-name">Forgify</div>
                <div className="onb-brand-sub">v1.2 · 本地优先</div>
              </div>
            </div>
            <div className="onb-steps">
              {STEPS.map((s, i) => (
                <div
                  key={s.key}
                  className={"onb-step" + (i === step ? " is-active" : "") + (i < step ? " is-done" : "")}
                >
                  <div className="onb-step-num">{i < step ? <Icon.Check /> : i + 1}</div>
                  <div className="onb-step-text">
                    <div className="onb-step-title">{s.title}</div>
                    <div className="onb-step-desc">{s.desc}</div>
                  </div>
                </div>
              ))}
            </div>
            <div className="onb-rail-footer">
              数据存在 <code>~/.forgify/</code>。不上传任何服务器，不需要登录。
            </div>
          </aside>

          <div className="onb-pane">
            <div className="onb-content">
              {STEPS[step].key === "intro" && <IntroStep />}
              {STEPS[step].key === "account" && <AccountStep name={name} setName={setName} accent={accent} />}
              {STEPS[step].key === "look" && <LookStep accent={accent} setAccent={setAccent} />}
              {STEPS[step].key === "provider" && (
                <ProviderStep
                  providers={llmProviders}
                  provider={provider} setProvider={setProvider}
                  apiKey={apiKey} setApiKey={setApiKey}
                />
              )}
              {STEPS[step].key === "done" && (
                <DoneStep name={name} accent={accent} provider={provider} hasKey={!!apiKey} />
              )}
            </div>

            <div className="onb-actions">
              <div className="onb-progress">
                步骤 {step + 1} / {STEPS.length}
                <div className="onb-progress-bar">
                  <div style={{ width: `${((step + 1) / STEPS.length) * 100}%` }} />
                </div>
              </div>
              {step > 0 && (
                <Button variant="ghost" size="sm" onClick={prev} disabled={busy}>
                  ← 上一步
                </Button>
              )}
              <Button
                variant="accent"
                size="sm"
                onClick={next}
                disabled={!canAdvance() || busy}
                loading={busy}
              >
                {step === STEPS.length - 1 ? "进入应用" : step === 0 ? "开始" : "继续"}
                <Icon.ArrowRight />
              </Button>
            </div>
          </div>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
}

function pickDefaultModel(provider) {
  switch (provider) {
    case "deepseek":  return "deepseek-v4-flash";
    case "anthropic": return "claude-sonnet-4-6";
    case "openai":    return "gpt-4o-mini";
    case "qwen":      return "qwen-plus";
    case "moonshot":  return "moonshot-v1-32k";
    case "zhipu":     return "glm-4";
    case "ollama":    return "llama3.2";
    case "google":    return "gemini-1.5-pro";
    default:          return provider;
  }
}

// ── Steps ──────────────────────────────────────────────────────────

function IntroStep() {
  return (
    <>
      <div className="onb-head">
        <div className="onb-title">欢迎使用 Forgify</div>
        <div className="onb-sub">
          本地优先的 Agentic Workflow Platform。
          对话 · 锻造工具 · 编排工作流 · 接 MCP · 全部跑在你这台机器。
        </div>
      </div>
      <div className="onb-bullet-list">
        <Bullet icon={Icon.MessageSquare} title="对话即工作"
                desc="自然语言告诉 Agent 你要做什么，它会自己挑工具、调 MCP、写函数、跑工作流" />
        <Bullet icon={Icon.Hammer} title="锻造工具"
                desc="让 AI 给你写新的 Function / Handler / Workflow，加版本管理，可回滚" />
        <Bullet icon={Icon.Server} title="本地优先"
                desc="数据全部在 ~/.forgify/，不上传任何云端，不需要账号登录" />
      </div>
    </>
  );
}

function AccountStep({ name, setName, accent }) {
  const color = ACCENTS.find(([k]) => k === accent)?.[1] || "#4f46e5";
  return (
    <>
      <div className="onb-head">
        <div className="onb-title">创建本地账号</div>
        <div className="onb-sub">不是注册账号 —— 只是给这台机器上的多个 profile 起个名。后续可以加更多。</div>
      </div>
      <div className="onb-avatar-row">
        <div className="onb-avatar" style={{ background: color }}>
          {name.trim().slice(0, 1).toUpperCase() || "?"}
        </div>
        <div className="onb-field" style={{ flex: 1 }}>
          <div className="onb-label">你的名字</div>
          <input
            className="onb-input onb-input-lg"
            placeholder="例如 sun"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
          <div className="onb-hint">用作账号 username + 显示名</div>
        </div>
      </div>
    </>
  );
}

function LookStep({ accent, setAccent }) {
  return (
    <>
      <div className="onb-head">
        <div className="onb-title">选个主题色</div>
        <div className="onb-sub">这是你的 accent；点击切换看效果。后续可以在设置里改。</div>
      </div>
      <div className="onb-swatches">
        {ACCENTS.map(([k, c, label]) => (
          <button
            key={k}
            className={"onb-swatch" + (accent === k ? " is-active" : "")}
            style={{ background: c }}
            onClick={() => setAccent(k)}
            title={label}
          />
        ))}
      </div>
    </>
  );
}

function ProviderStep({ providers, provider, setProvider, apiKey, setApiKey }) {
  return (
    <>
      <div className="onb-head">
        <div className="onb-title">配一个 LLM</div>
        <div className="onb-sub">至少配一个 API Key，对话才能跑。也可以跳过，之后在设置里加。</div>
      </div>
      <div className="onb-provider-grid">
        {providers.slice(0, 8).map((p) => {
          const hint = PROVIDER_HINTS[p.name] || { abbr: p.name.slice(0, 2).toUpperCase(), color: "#37352f" };
          return (
            <button
              key={p.name}
              className={"onb-provider" + (provider === p.name ? " is-active" : "")}
              onClick={() => setProvider(p.name)}
            >
              <span className="onb-pchip" style={{ background: hint.color }}>{hint.abbr}</span>
              <span style={{ display: "flex", flexDirection: "column", textAlign: "left", minWidth: 0 }}>
                <span className="onb-pname">{p.displayName || p.name}</span>
                <span className="onb-pdesc">{p.defaultBaseUrl}</span>
              </span>
            </button>
          );
        })}
      </div>
      <div className="onb-field" style={{ marginTop: 16 }}>
        <div className="onb-label">API Key（可粘贴；留空跳过）</div>
        <input
          className="onb-input"
          type="password"
          placeholder="sk-…"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
        />
        <div className="onb-hint">key 经 AES-GCM 加密落地 ~/.forgify/，不上传</div>
      </div>
    </>
  );
}

function DoneStep({ name, accent, provider, hasKey }) {
  return (
    <div style={{ textAlign: "center", paddingTop: 16 }}>
      <div className="onb-done-mark"><Icon.Check /></div>
      <div className="onb-title">就绪</div>
      <div className="onb-sub" style={{ marginTop: 8 }}>
        你的本地 Forgify 配好了。点 "进入应用" 开始第一段对话。
      </div>
      <div className="onb-done-grid">
        <DoneCard label="账号" value={name} />
        <DoneCard label="主题色" value={accent} />
        <DoneCard label="LLM" value={hasKey ? provider : "稍后配"} />
      </div>
    </div>
  );
}

function DoneCard({ label, value }) {
  return (
    <div className="onb-done-card">
      <div className="onb-done-card-label">{label}</div>
      <div className="onb-done-card-value">{value}</div>
    </div>
  );
}

function Bullet({ icon: I, title, desc }) {
  return (
    <div className="onb-bullet">
      <I className="icon" />
      <div>
        <div className="onb-bullet-title">{title}</div>
        <div className="onb-bullet-desc">{desc}</div>
      </div>
    </div>
  );
}
