// Onboarding — toB 6-step first-run wizard (split stage + journey).
//   1 Welcome    — what Forgify is.
//   2 Workspace  — create the first local user (POST /users), sets activeUserId.
//   3 Appearance — accent + language + theme; all write straight to settings (live).
//   4 Model      — provider + key → verify (:test) → pick model → POST /model-configs.
//   5 Search     — optional search provider + key (POST /api-keys, category=search).
//   6 Done       — recap + enter.
//
// 6 步 toB 首启向导。业务编排已迁入 useOnboardingFlow;本组件只负责渲染。

import { useTranslation } from "react-i18next";
import { motion, AnimatePresence } from "framer-motion";
import { Icon } from "@/components/primitives/Icon.jsx";
import { Button } from "@/components/primitives/Button.jsx";
import { useSettingsStore } from "@entities/settings";
import { ACCENTS, LLM_HINTS, SEARCH_HINTS } from "./onboarding-strings.js";
import { ProviderGrid } from "@features/settings/ui/ProviderGrid.jsx";
import { KeyVerifyField } from "@features/settings/ui/KeyVerifyField.jsx";
import { ModelSelect } from "@features/settings/ui/ModelSelect.jsx";
import { useOnboardingFlow } from "@features/onboarding";

const STEP_KEYS = ["welcome", "workspace", "appearance", "model", "search", "done"];
const ANVIL = (
  <svg viewBox="0 0 24 24"><path d="M12 2v3" /><path d="M5 5l2 2" /><path d="M19 5l-2 2" /><path d="M4 12h4l2-3l4 6l2-3h4" /><path d="M5 17h14" /><path d="M7 21l1-4" /><path d="M17 21l-1-4" /></svg>
);

export function Onboarding({ onFinish }) {
  const settings = useSettingsStore();
  const { t } = useTranslation("onboarding");

  const {
    step, stepKey, busy,
    next, back, canNext,
    advance,
    name, setName,
    provider, pickProvider,
    apiKey, onKeyChange,
    verify, verifying, verified, verifyError,
    models, modelId, setModelId,
    llmProviders,
    searchProvider, setSearchProvider,
    searchKey, setSearchKey,
    searchProviders,
    providerDisplay,
    jdesc,
  } = useOnboardingFlow();

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
                <div className="onb-brand-sub">{t("brandSub")}</div>
              </div>
            </div>
            <div className="onb-journey">
              {STEP_KEYS.map((key, i) => {
                const [jt, jd] = t(`journey.${key}`, { returnObjects: true });
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
              <span>{t("footer1")} <code>~/.forgify/</code><br />{t("footer2")}</span>
            </div>
          </aside>

          <section className={"onb-pane" + (stepKey === "done" ? " is-centered" : "")}>
            {stepKey === "welcome" && <Welcome t={t} />}
            {stepKey === "workspace" && <Workspace t={t} name={name} setName={setName} accent={settings.accent} />}
            {stepKey === "appearance" && <Appearance t={t} settings={settings} />}
            {stepKey === "model" && (
              <Model
                t={t} providers={llmProviders} provider={provider} pickProvider={pickProvider}
                apiKey={apiKey} onKeyChange={onKeyChange} verify={verify}
                verifying={verifying} verified={verified} verifyError={verifyError}
                models={models} modelId={modelId} setModelId={setModelId}
              />
            )}
            {stepKey === "search" && (
              <Search
                t={t} providers={searchProviders} provider={searchProvider} setProvider={setSearchProvider}
                apiKey={searchKey} setApiKey={setSearchKey}
              />
            )}
            {stepKey === "done" && (
              <Done t={t} name={name} accent={settings.accent} provider={verified ? providerDisplay(provider) : null} search={searchProvider ? providerDisplay(searchProvider) : null} />
            )}

            <div className="onb-actions">
              {stepKey !== "done" && (
                <div className="onb-progress">
                  <div className="onb-progress-label">{t("stepWord")} {step + 1} / {STEP_KEYS.length}</div>
                  <div className="onb-progress-track">
                    <div className="onb-progress-fill" style={{ width: `${((step + 1) / STEP_KEYS.length) * 100}%` }} />
                  </div>
                </div>
              )}
              {stepKey === "search" ? (
                <Button variant="ghost" size="sm" onClick={advance} disabled={busy}>{t("skip")}</Button>
              ) : step > 0 && stepKey !== "done" ? (
                <Button variant="ghost" size="sm" onClick={back} disabled={busy}>← {t("back")}</Button>
              ) : null}
              <Button variant="accent" size="sm" onClick={() => next(onFinish)} disabled={!canNext() || busy} loading={busy}>
                {stepKey === "done" ? t("enter") : stepKey === "welcome" ? t("start") : t("next")}
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
      <div className="onb-kicker">{t("welcome.kicker")}</div>
      <div className="onb-title">{t("welcome.title")}</div>
      <div className="onb-sub">{t("welcome.sub")}</div>
      <div className="onb-features">
        {t("welcome.features", { returnObjects: true }).map(([title, desc], i) => {
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
      <div className="onb-kicker">{t("workspace.kicker")}</div>
      <div className="onb-title">{t("workspace.title")}</div>
      <div className="onb-sub">{t("workspace.sub")}</div>
      <div className="onb-ws">
        <div className="onb-avatar" style={{ background: color }}>{name.trim().slice(0, 1).toUpperCase() || "W"}</div>
        <div className="onb-field">
          <div className="onb-label">{t("workspace.label")}</div>
          <input className="onb-input" placeholder={t("workspace.placeholder")} value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          <div className="onb-hint">{t("workspace.hint")}</div>
        </div>
      </div>
    </>
  );
}

function Appearance({ t, settings }) {
  return (
    <>
      <div className="onb-kicker">{t("appearance.kicker")}</div>
      <div className="onb-title">{t("appearance.title")}</div>
      <div className="onb-sub">{t("appearance.sub")}</div>

      <div className="onb-fg">
        <div className="onb-label">{t("appearance.accent")}</div>
        <div className="onb-swatches">
          {ACCENTS.map(([k, c]) => (
            <button key={k} className={"onb-swatch" + (settings.accent === k ? " is-active" : "")} style={{ background: c }} onClick={() => settings.set({ accent: k })} />
          ))}
        </div>
      </div>

      <div className="onb-fg">
        <div className="onb-label">{t("appearance.language")} <span className="onb-auto">{t("auto")}</span></div>
        <div className="onb-seg">
          {[["zh", "中文"], ["en", "English"]].map(([k, label]) => (
            <button key={k} className={"onb-seg-opt" + (settings.lang === k ? " is-active" : "")} onClick={() => settings.set({ lang: k })}>{label}</button>
          ))}
        </div>
      </div>

      <div className="onb-fg">
        <div className="onb-label">{t("appearance.theme")} <span className="onb-auto">{t("auto")}</span></div>
        <div className="onb-seg">
          {[["light", t("appearance.themeOpts.light")], ["dark", t("appearance.themeOpts.dark")], ["system", t("appearance.themeOpts.system")]].map(([k, label]) => (
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
      <div className="onb-kicker">{t("model.kicker")}</div>
      <div className="onb-title">{t("model.title")}</div>
      <div className="onb-sub">{t("model.sub")}</div>
      <ProviderGrid providers={providers} hints={LLM_HINTS} selected={provider} onPick={pickProvider} />
      <div className="onb-scrollnote">{t("model.scrollNote")}</div>

      {provider && (
        <>
          <div className="onb-twofield">
            <div className="onb-keyfield" style={{ flex: 1.3 }}>
              <KeyVerifyField
                label={isOllama ? display : t("model.keyLabel", { provider: display })}
                value={isOllama ? "localhost:11434" : apiKey}
                onChange={onKeyChange}
                onVerify={verify}
                verifying={verifying}
                verified={verified}
                error={verifyError}
                verifyLabel={t("model.verify")}
                verifyingLabel={t("model.verifying")}
                verifiedLabel={t("model.verified")}
                placeholder={t("model.keyPlaceholder")}
                readOnly={isOllama}
              />
            </div>
            {verified && models.length > 0 && (
              <div className="onb-keyfield" style={{ flex: 1 }}>
                <div className="onb-klabel">{t("model.modelLabel")}</div>
                <ModelSelect models={models} value={modelId} onChange={setModelId} />
              </div>
            )}
          </div>
          {verified && models.length > 0 && <div className="onb-khint">{t("model.availHint", { list: models.join(" · ") })}</div>}
        </>
      )}
    </>
  );
}

function Search({ t, providers, provider, setProvider, apiKey, setApiKey }) {
  const display = providers.find((p) => p.name === provider)?.displayName || provider;
  return (
    <>
      <div className="onb-kicker">{t("search.kicker")} <span style={{ color: "var(--fg-faint)", fontWeight: 500, textTransform: "none", letterSpacing: 0 }}>{t("search.optional")}</span></div>
      <div className="onb-title">{t("search.title")}</div>
      <div className="onb-sub">{t("search.sub")}</div>
      <div className="onb-fg">
        <div className="onb-label">{t("search.providerLabel")}</div>
        <ProviderGrid providers={providers} hints={SEARCH_HINTS} selected={provider} onPick={setProvider} tall />
      </div>
      {provider && (
        <div className="onb-keyfield">
          <div className="onb-klabel">{t("search.keyLabel", { provider: display })}</div>
          <div className="onb-kinput is-plain">
            <Icon.KeyRound />
            <input type="password" placeholder={t("search.keyPlaceholder")} value={apiKey} onChange={(e) => setApiKey(e.target.value)} autoFocus />
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
      <div className="onb-title">{t("done.title")}</div>
      <div className="onb-sub">{t("done.sub")}</div>
      <div className="onb-recap">
        <Recap label={t("done.recap.workspace")} value={name} />
        <Recap label={t("done.recap.accent")}><span className="onb-recap-dot" style={{ background: color }} /></Recap>
        <Recap label={t("done.recap.model")} value={provider} muted={!provider} fallback={t("done.none")} />
        <Recap label={t("done.recap.search")} value={search} muted={!search} fallback={t("done.none")} />
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
