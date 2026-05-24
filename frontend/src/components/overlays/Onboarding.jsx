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

import { useEffect, useState } from "react";
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
  { key: "intro",    title: "你好",      desc: "看一眼" },
  { key: "account",  title: "工作空间",  desc: "起个名字" },
  { key: "look",     title: "外观",      desc: "挑个色调" },
  { key: "provider", title: "钥匙",      desc: "可以稍后" },
  { key: "done",     title: "就位",      desc: "开始" },
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

  // mock = dev-only fake LLM; custom = catch-all (needs baseUrl + apiFormat
  // that don't fit onboarding's lightweight form). Keep them off the grid.
  //
  // mock 是 dev 假 LLM,custom 要 baseUrl+apiFormat 不适合 onboarding 轻量表单。
  const llmProviders = providers.filter(
    (p) => p.category === "llm" && p.name !== "mock" && p.name !== "custom",
  );

  // Live preview of accent on the wizard itself.
  useEffect(() => {
    applyTheme({ ...settings, accent });
  }, [accent]); // eslint-disable-line react-hooks/exhaustive-deps

  const canAdvance = () => {
    switch (STEPS[step].key) {
      case "intro":    return true;
      case "account":  return name.trim().length > 0;
      case "look":     return true;
      case "provider":
        // Provider must be picked. ollama is local + needs no key. Other
        // providers need both provider + apiKey. Skip is via explicit button,
        // not silent advance.
        if (!provider) return false;
        if (provider === "ollama") return true;
        return apiKey.trim().length > 0;
      case "done":     return true;
    }
  };

  // skipProvider — explicit "no key right now" path. Clears state and jumps
  // to the done step without trying to write a key. Settings is updated in
  // finish() like the normal path.
  //
  // skipProvider —— 显式 "稍后配" 路径;清状态跳到 done。
  const skipProvider = () => {
    setProvider("");
    setApiKey("");
    setStep((s) => Math.min(STEPS.length - 1, s + 1));
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

      // 2. Key path: create → test → write model-config from real modelsFound.
      //    Anthropic's tester intentionally doesn't list models — fall back
      //    to a curated default ONLY when modelsFound is empty. If the test
      //    itself fails (401 / network / etc.), don't write model-config
      //    and let NoModelGate guide the user later.
      //
      //    For ollama: key is empty by design; treat the same way (test pings
      //    /api/tags, returns models if local server is up).
      //
      // key 路径:create → test → 拿 modelsFound[0] 写 model-config。
      // Anthropic 不返 models,留 fallback;test 失败干脆不写,后面 NoModelGate
      // 接力。ollama 无 key 但 test 走 /api/tags,逻辑同上。
      const hasKeyish = (apiKey || provider === "ollama") && provider;
      if (hasKeyish) {
        const k = await createKey.mutateAsync({
          provider,
          key: apiKey || "ollama-no-key",
          displayName: `${provider} (onboarding)`,
        });
        try {
          const testResult = await testKey.mutateAsync(k.id);
          const modelId = (testResult?.modelsFound?.[0]) || PROVIDER_DEFAULT_MODEL[provider];
          if (modelId) {
            await setModel.mutateAsync({ scenario: "chat", provider, modelId });
          }
        } catch (testErr) {
          // Test 失败保留 key,但不写 model-config;NoModelGate 会引导。
          pushToast({ kind: "warn", title: "Key 已存但验证未通过", desc: testErr.message });
        }
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
                <div className="onb-brand-sub">v1.2</div>
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
              数据存在 <code>~/.forgify/</code>。
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
                  onSkip={skipProvider}
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
                {step === STEPS.length - 1 ? "开始" : step === 0 ? "开始" : "继续"}
                <Icon.ArrowRight />
              </Button>
            </div>
          </div>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
}

// PROVIDER_DEFAULT_MODEL — fallback model IDs used ONLY when the apikey
// connectivity test couldn't return modelsFound (e.g., Anthropic's ping
// doesn't list models). For everyone else, finish() prefers modelsFound[0].
//
// Values must be real model IDs — wrong defaults cause silent "configured
// but won't run" states (the bug we just fixed: "deepseek-v4-flash" wasn't
// real). When in doubt, omit the provider here; chat will fall through to
// NoModelGate and the user picks manually.
//
// 仅当 apikey test 没返 modelsFound 时使用的兜底 modelID;其它情况优先
// modelsFound[0]。值必须真实可用 —— 错的会让 "看似配好但跑不起来"。
const PROVIDER_DEFAULT_MODEL = {
  anthropic: "claude-sonnet-4-6",
};

// ── Steps ──────────────────────────────────────────────────────────

function IntroStep() {
  return (
    <>
      <div className="onb-head">
        <div className="onb-title">你好</div>
        <div className="onb-sub">
          这是 Forgify。一个住在你电脑上的 agent。
          你说一句话,它做事;事情沉淀成你能反复用的工具。
        </div>
      </div>
      <div className="onb-bullet-list">
        <Bullet icon={Icon.MessageSquare} title="先对话"
                desc="说你想做什么。agent 自己挑工具、写代码、跑工作流。" />
        <Bullet icon={Icon.Hammer} title="再沉淀"
                desc="agent 帮你造 Function / Handler / Workflow,带版本,可回滚。" />
        <Bullet icon={Icon.Server} title="都在本地"
                desc="数据放在 ~/.forgify/,不上传,不需要登录。" />
      </div>
    </>
  );
}

function AccountStep({ name, setName, accent }) {
  const color = ACCENTS.find(([k]) => k === accent)?.[1] || "#4f46e5";
  return (
    <>
      <div className="onb-head">
        <div className="onb-title">起个名字</div>
        <div className="onb-sub">工作空间的名字。后续可以再加、再切换。</div>
      </div>
      <div className="onb-avatar-row">
        <div className="onb-avatar" style={{ background: color }}>
          {name.trim().slice(0, 1).toUpperCase() || "?"}
        </div>
        <div className="onb-field" style={{ flex: 1 }}>
          <div className="onb-label">名字</div>
          <input
            className="onb-input onb-input-lg"
            placeholder="例如 私人 / 工作 / 写作"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
          />
          <div className="onb-hint">显示在 sidebar 底部。切换时只切这一个空间的数据。</div>
        </div>
      </div>
    </>
  );
}

function LookStep({ accent, setAccent }) {
  return (
    <>
      <div className="onb-head">
        <div className="onb-title">挑个色调</div>
        <div className="onb-sub">点睛色。后续在设置里可改。</div>
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

function ProviderStep({ providers, provider, setProvider, apiKey, setApiKey, onSkip }) {
  const selectedMeta = providers.find((p) => p.name === provider);
  const isOllama = provider === "ollama";

  return (
    <>
      <div className="onb-head">
        <div className="onb-title">配一把钥匙</div>
        <div className="onb-sub">给一个 LLM 厂商,填它的 API key。也可以稍后再配。</div>
      </div>
      <div className="onb-provider-grid">
        {providers.map((p) => {
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
                <span className="onb-pdesc">{p.defaultBaseUrl || (p.name === "ollama" ? "本地 · 无需 key" : "")}</span>
              </span>
            </button>
          );
        })}
      </div>

      {provider && !isOllama && (
        <div className="onb-field" style={{ marginTop: 16 }}>
          <div className="onb-label">
            {selectedMeta?.displayName || provider} 的 API Key
          </div>
          <input
            className="onb-input"
            type="password"
            placeholder="sk-…"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            autoFocus
          />
          <div className="onb-hint">key 经 AES-GCM 加密放在 ~/.forgify/,不上传。</div>
        </div>
      )}

      {isOllama && (
        <div style={{
          marginTop: 16, padding: 14, borderRadius: 6,
          background: "var(--accent-soft)", fontSize: 12,
          color: "var(--fg-muted)", lineHeight: 1.55,
        }}>
          Ollama 是本地推理,不需要 API key。确保 <code style={{ fontFamily: "var(--font-mono)" }}>ollama serve</code> 已启动。
        </div>
      )}

      <div style={{ marginTop: 18 }}>
        <button
          className="btn btn-ghost"
          style={{ padding: "6px 10px", fontSize: 12 }}
          onClick={onSkip}
          type="button"
        >
          稍后再配 →
        </button>
      </div>
    </>
  );
}

function DoneStep({ name, accent, provider, hasKey }) {
  return (
    <div style={{ textAlign: "center", paddingTop: 16 }}>
      <div className="onb-done-mark"><Icon.Check /></div>
      <div className="onb-title">好了</div>
      <div className="onb-sub" style={{ marginTop: 8 }}>
        点"开始",和 agent 说第一句话。
      </div>
      <div className="onb-done-grid">
        <DoneCard label="工作空间" value={name} />
        <DoneCard label="色调" value={accent} />
        <DoneCard label="LLM" value={hasKey ? provider : "稍后再配"} />
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
