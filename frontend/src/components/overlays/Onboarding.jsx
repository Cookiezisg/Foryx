// Onboarding — toB 6-step first-run wizard (split stage + journey).
//   1 Welcome    — what Forgify is.
//   2 Workspace  — create the first local user (POST /users), sets activeUserId.
//   3 Appearance — accent + language + theme; all write straight to settings (live).
//   4 Model      — provider + key → verify (:test) → pick model → POST /model-configs.
//   5 Search     — optional search provider + key (POST /api-keys, category=search).
//   6 Done       — recap + enter.
//
// Workspace creates the user EARLY so steps 4/5 can write user-scoped keys;
// App's onboarding latch keeps this mounted afterwards. finish() flips
// settings.onboarded.
//
// 6 步 toB 首启向导。workspace 步即建 user(后续步才能写 user 作用域的 key);
// App 的 latch 保证建 user 后不被卸载。finish 置 onboarded。

import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { useQueryClient } from "@tanstack/react-query";
import { Icon } from "../primitives/Icon.jsx";
import { Button } from "../primitives/Button.jsx";
import { useSettings } from "../../store/settings.js";
import { useUIStore } from "../../store/ui.js";
import { useCreateUser } from "../../api/users.js";
import { useProviders, useCreateApiKey, useTestApiKey, useUpsertModelConfig, useDeleteApiKey } from "../../api/config.js";
import { STRINGS, ACCENTS, LLM_HINTS, SEARCH_HINTS, PROVIDER_DEFAULT_MODEL } from "./onboarding-strings.js";
import { ProviderGrid } from "../config/ProviderGrid.jsx";
import { KeyVerifyField } from "../config/KeyVerifyField.jsx";
import { ModelSelect } from "../config/ModelSelect.jsx";

const STEP_KEYS = ["welcome", "workspace", "appearance", "model", "search", "done"];
const ANVIL = (
  <svg viewBox="0 0 24 24"><path d="M12 2v3" /><path d="M5 5l2 2" /><path d="M19 5l-2 2" /><path d="M4 12h4l2-3l4 6l2-3h4" /><path d="M5 17h14" /><path d="M7 21l1-4" /><path d="M17 21l-1-4" /></svg>
);

export function Onboarding({ onFinish }) {
  const settings = useSettings();
  const qc = useQueryClient();
  const pushToast = useUIStore((s) => s.pushToast);
  const t = STRINGS[settings.lang] || STRINGS.zh;

  const createUser = useCreateUser();
  const { data: providers = [] } = useProviders();
  const createKey = useCreateApiKey();
  const testKey = useTestApiKey();
  const deleteKey = useDeleteApiKey();
  const upsertModel = useUpsertModelConfig();

  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);

  // workspace
  const [name, setName] = useState("");
  const [createdUserId, setCreatedUserId] = useState(null);
  // model
  const [provider, setProvider] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [createdKeyId, setCreatedKeyId] = useState(null);
  const [createdKeyText, setCreatedKeyText] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [verified, setVerified] = useState(false);
  const [verifyError, setVerifyError] = useState("");
  const [models, setModels] = useState([]);
  const [modelId, setModelId] = useState("");
  // search
  const [searchProvider, setSearchProvider] = useState("");
  const [searchKey, setSearchKey] = useState("");

  const llm = providers.filter((p) => p.category === "llm" && p.name !== "mock" && p.name !== "custom");
  const search = providers.filter((p) => p.category === "search");
  const stepKey = STEP_KEYS[step];
  const providerDisplay = (n) => providers.find((p) => p.name === n)?.displayName || n;

  const run = async (fn) => {
    setBusy(true);
    try { await fn(); }
    catch (err) { pushToast({ kind: "error", title: t.toast.opFail, desc: err.message }); }
    finally { setBusy(false); }
  };

  const ensureUser = async () => {
    if (createdUserId) return;
    const user = await createUser.mutateAsync({
      username: name.trim().toLowerCase().replace(/\s+/g, "-"),
      displayName: name.trim(),
      avatarColor: ACCENTS.find(([k]) => k === settings.accent)?.[1] || "#d97757",
    });
    setCreatedUserId(user.id);
    settings.set({ activeUserId: user.id });
  };

  // Switching provider drops key/model state; best-effort delete the orphaned
  // key created for the previous provider.
  const pickProvider = (n) => {
    if (createdKeyId) deleteKey.mutate(createdKeyId);
    setProvider(n);
    setApiKey(""); setCreatedKeyId(null); setCreatedKeyText("");
    setVerified(false); setModels([]); setModelId(""); setVerifyError("");
  };

  const onKeyChange = (v) => {
    setApiKey(v);
    setVerifyError("");
    if (verified) { setVerified(false); setModels([]); setModelId(""); }
  };

  const verify = () => run(async () => {
    setVerifying(true);
    setVerifyError("");
    try {
      let keyId = createdKeyId;
      if (keyId && createdKeyText !== apiKey) {
        deleteKey.mutate(keyId);
        keyId = null; setCreatedKeyId(null);
      }
      if (!keyId) {
        const k = await createKey.mutateAsync({
          provider, key: apiKey || "ollama-no-key", displayName: `${provider} (onboarding)`,
        });
        keyId = k.id; setCreatedKeyId(k.id); setCreatedKeyText(apiKey);
      }
      const res = await testKey.mutateAsync(keyId);
      const found = res?.modelsFound || [];
      const opts = found.length ? found : (PROVIDER_DEFAULT_MODEL[provider] ? [PROVIDER_DEFAULT_MODEL[provider]] : []);
      setModels(opts);
      setModelId(opts[0] || "");
      setVerified(true);
      pushToast({ kind: "success", title: t.toast.keyVerified });
    } catch {
      setVerified(false);
      setVerifyError(t.model.verifyFail);
    } finally {
      setVerifying(false);
    }
  });

  const finish = () => {
    settings.set({ onboarded: true });
    qc.invalidateQueries();
    pushToast({ kind: "success", title: t.toast.welcome, desc: name.trim() });
    onFinish?.();
  };

  const advance = () => setStep((s) => Math.min(STEP_KEYS.length - 1, s + 1));
  const back = () => setStep((s) => Math.max(0, s - 1));

  const handleNext = () => {
    switch (stepKey) {
      case "workspace": return run(async () => { await ensureUser(); advance(); });
      case "model": return run(async () => {
        if (verified && modelId) await upsertModel.mutateAsync({ scenario: "chat", provider, modelId });
        advance();
      });
      case "search": return run(async () => {
        if (searchProvider && searchKey.trim()) {
          await createKey.mutateAsync({ provider: searchProvider, key: searchKey.trim(), displayName: `${searchProvider} (onboarding)` });
        }
        advance();
      });
      case "done": return finish();
      default: return advance();
    }
  };

  const canNext = () => (stepKey === "workspace" ? name.trim().length > 0 : true);

  // journey desc shows captured values once a step is behind us.
  const jdesc = (key, fallback) => {
    if (key === "workspace" && createdUserId) return name.trim();
    if (key === "appearance" && step > 2) return `${settings.accent} · ${settings.lang === "zh" ? "中文" : "EN"}`;
    if (key === "model" && step > 3) return verified ? providerDisplay(provider) : t.done.none;
    if (key === "search" && step > 4) return searchProvider ? providerDisplay(searchProvider) : t.done.none;
    return fallback;
  };

  return (
    <AnimatePresence>
      <motion.div className="onb-overlay" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
        <motion.div
          className="onb-card"
          initial={{ opacity: 0, scale: 0.97, y: 8 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          transition={{ duration: 0.32, ease: [0.2, 0.8, 0.2, 1] }}
        >
          <aside className="onb-stage">
            <div className="onb-brand">
              <div className="onb-mark">{ANVIL}</div>
              <div>
                <div className="onb-brand-name">Forgify</div>
                <div className="onb-brand-sub">{t.brandSub}</div>
              </div>
            </div>
            <div className="onb-journey">
              {STEP_KEYS.map((key, i) => {
                const [jt, jd] = t.journey[key];
                const cls = "onb-jstep" + (i === step ? " is-active" : "") + (i < step ? " is-done" : "");
                return (
                  <div key={key} className={cls}>
                    {i < STEP_KEYS.length - 1 && <div className="onb-jline" />}
                    <div className="onb-jdot">{i < step ? <Icon.Check /> : i + 1}</div>
                    <div className="onb-jtext">
                      <div className="onb-jtitle">{jt}</div>
                      <div className="onb-jdesc">{jdesc(key, jd)}</div>
                    </div>
                  </div>
                );
              })}
            </div>
            <div className="onb-foot">
              <Icon.KeyRound />
              <span>{t.footer1} <code>~/.forgify/</code><br />{t.footer2}</span>
            </div>
          </aside>

          <section className={"onb-pane" + (stepKey === "done" ? " is-centered" : "")}>
            {stepKey === "welcome" && <Welcome t={t} />}
            {stepKey === "workspace" && <Workspace t={t} name={name} setName={setName} accent={settings.accent} />}
            {stepKey === "appearance" && <Appearance t={t} settings={settings} />}
            {stepKey === "model" && (
              <Model
                t={t} providers={llm} provider={provider} pickProvider={pickProvider}
                apiKey={apiKey} onKeyChange={onKeyChange} verify={verify}
                verifying={verifying} verified={verified} verifyError={verifyError}
                models={models} modelId={modelId} setModelId={setModelId}
              />
            )}
            {stepKey === "search" && (
              <Search
                t={t} providers={search} provider={searchProvider} setProvider={setSearchProvider}
                apiKey={searchKey} setApiKey={setSearchKey}
              />
            )}
            {stepKey === "done" && (
              <Done t={t} name={name} accent={settings.accent} provider={verified ? providerDisplay(provider) : null} search={searchProvider ? providerDisplay(searchProvider) : null} />
            )}

            <div className="onb-actions">
              {stepKey !== "done" && (
                <div className="onb-progress">
                  <div className="onb-progress-label">{t.stepWord} {step + 1} / {STEP_KEYS.length}</div>
                  <div className="onb-progress-track">
                    <div className="onb-progress-fill" style={{ width: `${((step + 1) / STEP_KEYS.length) * 100}%` }} />
                  </div>
                </div>
              )}
              {stepKey === "search" ? (
                <Button variant="ghost" size="sm" onClick={advance} disabled={busy}>{t.skip}</Button>
              ) : step > 0 && stepKey !== "done" ? (
                <Button variant="ghost" size="sm" onClick={back} disabled={busy}>← {t.back}</Button>
              ) : null}
              <Button variant="accent" size="sm" onClick={handleNext} disabled={!canNext() || busy} loading={busy}>
                {stepKey === "done" ? t.enter : stepKey === "welcome" ? t.start : t.next}
                <Icon.ArrowRight />
              </Button>
            </div>
          </section>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
}

function Welcome({ t }) {
  const icons = [Icon.MessageSquare, Icon.Wrench, Icon.Server];
  return (
    <>
      <div className="onb-kicker">{t.welcome.kicker}</div>
      <div className="onb-title">{t.welcome.title}</div>
      <div className="onb-sub">{t.welcome.sub}</div>
      <div className="onb-features">
        {t.welcome.features.map(([title, desc], i) => {
          const I = icons[i];
          return (
            <div className="onb-feature" key={i}>
              <div className="onb-fic"><I /></div>
              <div><div className="onb-ftitle">{title}</div><div className="onb-fdesc">{desc}</div></div>
            </div>
          );
        })}
      </div>
    </>
  );
}

function Workspace({ t, name, setName, accent }) {
  const color = ACCENTS.find(([k]) => k === accent)?.[1] || "#d97757";
  return (
    <>
      <div className="onb-kicker">{t.workspace.kicker}</div>
      <div className="onb-title">{t.workspace.title}</div>
      <div className="onb-sub">{t.workspace.sub}</div>
      <div className="onb-ws">
        <div className="onb-avatar" style={{ background: color }}>{name.trim().slice(0, 1).toUpperCase() || "W"}</div>
        <div className="onb-field">
          <div className="onb-label">{t.workspace.label}</div>
          <input className="onb-input" placeholder={t.workspace.placeholder} value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          <div className="onb-hint">{t.workspace.hint}</div>
        </div>
      </div>
    </>
  );
}

function Appearance({ t, settings }) {
  return (
    <>
      <div className="onb-kicker">{t.appearance.kicker}</div>
      <div className="onb-title">{t.appearance.title}</div>
      <div className="onb-sub">{t.appearance.sub}</div>

      <div className="onb-fg">
        <div className="onb-label">{t.appearance.accent}</div>
        <div className="onb-swatches">
          {ACCENTS.map(([k, c]) => (
            <button key={k} className={"onb-swatch" + (settings.accent === k ? " is-active" : "")} style={{ background: c }} onClick={() => settings.set({ accent: k })} />
          ))}
        </div>
      </div>

      <div className="onb-fg">
        <div className="onb-label">{t.appearance.language} <span className="onb-auto">{t.auto}</span></div>
        <div className="onb-seg">
          {[["zh", "中文"], ["en", "English"]].map(([k, label]) => (
            <button key={k} className={"onb-seg-opt" + (settings.lang === k ? " is-active" : "")} onClick={() => settings.set({ lang: k })}>{label}</button>
          ))}
        </div>
      </div>

      <div className="onb-fg">
        <div className="onb-label">{t.appearance.theme} <span className="onb-auto">{t.auto}</span></div>
        <div className="onb-seg">
          {[["light", t.appearance.themeOpts.light], ["dark", t.appearance.themeOpts.dark], ["system", t.appearance.themeOpts.system]].map(([k, label]) => (
            <button key={k} className={"onb-seg-opt" + (settings.theme === k ? " is-active" : "")} onClick={() => settings.set({ theme: k })}>{label}</button>
          ))}
        </div>
      </div>
    </>
  );
}

function Model({ t, providers, provider, pickProvider, apiKey, onKeyChange, verify, verifying, verified, verifyError, models, modelId, setModelId }) {
  const isOllama = provider === "ollama";
  const display = providers.find((p) => p.name === provider)?.displayName || provider;
  return (
    <>
      <div className="onb-kicker">{t.model.kicker}</div>
      <div className="onb-title">{t.model.title}</div>
      <div className="onb-sub">{t.model.sub}</div>
      <ProviderGrid providers={providers} hints={LLM_HINTS} selected={provider} onPick={pickProvider} />
      <div className="onb-scrollnote">{t.model.scrollNote}</div>

      {provider && (
        <>
          <div className="onb-twofield">
            <div className="onb-keyfield" style={{ flex: 1.3 }}>
              <KeyVerifyField
                label={isOllama ? display : t.model.keyLabel(display)}
                value={isOllama ? "localhost:11434" : apiKey}
                onChange={onKeyChange}
                onVerify={verify}
                verifying={verifying}
                verified={verified}
                error={verifyError}
                verifyLabel={t.model.verify}
                verifyingLabel={t.model.verifying}
                verifiedLabel={t.model.verified}
                placeholder={t.model.keyPlaceholder}
                readOnly={isOllama}
              />
            </div>
            {verified && models.length > 0 && (
              <div className="onb-keyfield" style={{ flex: 1 }}>
                <div className="onb-klabel">{t.model.modelLabel}</div>
                <ModelSelect models={models} value={modelId} onChange={setModelId} />
              </div>
            )}
          </div>
          {verified && models.length > 0 && <div className="onb-khint">{t.model.availHint(models)}</div>}
        </>
      )}
    </>
  );
}

function Search({ t, providers, provider, setProvider, apiKey, setApiKey }) {
  const display = providers.find((p) => p.name === provider)?.displayName || provider;
  return (
    <>
      <div className="onb-kicker">{t.search.kicker} <span style={{ color: "var(--fg-faint)", fontWeight: 500, textTransform: "none", letterSpacing: 0 }}>{t.search.optional}</span></div>
      <div className="onb-title">{t.search.title}</div>
      <div className="onb-sub">{t.search.sub}</div>
      <div className="onb-fg">
        <div className="onb-label">{t.search.providerLabel}</div>
        <ProviderGrid providers={providers} hints={SEARCH_HINTS} selected={provider} onPick={setProvider} tall />
      </div>
      {provider && (
        <div className="onb-keyfield">
          <div className="onb-klabel">{t.search.keyLabel(display)}</div>
          <div className="onb-kinput is-plain">
            <Icon.KeyRound />
            <input type="password" placeholder={t.search.keyPlaceholder} value={apiKey} onChange={(e) => setApiKey(e.target.value)} autoFocus />
          </div>
        </div>
      )}
    </>
  );
}

function Done({ t, name, accent, provider, search }) {
  const color = ACCENTS.find(([k]) => k === accent)?.[1] || "#d97757";
  return (
    <>
      <div className="onb-donemark"><Icon.Check /></div>
      <div className="onb-title">{t.done.title}</div>
      <div className="onb-sub">{t.done.sub}</div>
      <div className="onb-recap">
        <Recap label={t.done.recap.workspace} value={name} />
        <Recap label={t.done.recap.accent}><span className="onb-recap-dot" style={{ background: color }} /></Recap>
        <Recap label={t.done.recap.model} value={provider} muted={!provider} fallback={t.done.none} />
        <Recap label={t.done.recap.search} value={search} muted={!search} fallback={t.done.none} />
      </div>
    </>
  );
}

function Recap({ label, value, children, muted, fallback }) {
  return (
    <div className="onb-recap-card">
      <div className="onb-recap-label">{label}</div>
      <div className={"onb-recap-value" + (muted ? " is-muted" : "")}>{children || value || fallback}</div>
    </div>
  );
}
