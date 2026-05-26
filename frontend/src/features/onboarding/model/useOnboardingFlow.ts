// Orchestrates the 6-step onboarding wizard: user creation, provider/key
// selection, model verification, search key, finish. Extracted verbatim from
// Onboarding.jsx so the component only handles rendering.
//
// 封装 6 步首启向导编排;Onboarding.jsx 只负责渲染,不再含业务决策。

import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import i18n from "@shared/lib/i18n/index.js";
import { useCreateUser } from "@entities/user";
import { useCreateApiKey, useTestApiKey, useDeleteApiKey } from "@entities/apikey";
import { useProviders, useUpsertModelConfig } from "@entities/model-config";
// TODO(阶段4): onboarding-strings 迁入 shared/config 后修正 import
// eslint-disable-next-line boundaries/dependencies
import { ACCENTS, PROVIDER_DEFAULT_MODEL } from "../../../components/overlays/onboarding-strings.js";
import { useSessionStore } from "@entities/session";
import { useSettingsStore } from "@entities/settings";
import { useToastStore } from "@shared/ui/toastStore";

const STEP_KEYS = ["welcome", "workspace", "appearance", "model", "search", "done"] as const;
type StepKey = typeof STEP_KEYS[number];

export interface OnboardingFlowState {
  // Step navigation
  step: number;
  stepKey: StepKey;
  busy: boolean;
  next: (onFinish?: () => void) => void;
  back: () => void;
  canNext: () => boolean;

  // workspace
  name: string;
  setName: (v: string) => void;
  createdUserId: string | null;

  // model
  provider: string;
  pickProvider: (n: string) => void;
  apiKey: string;
  onKeyChange: (v: string) => void;
  verify: () => void;
  verifying: boolean;
  verified: boolean;
  verifyError: string;
  models: string[];
  modelId: string;
  setModelId: (v: string) => void;
  llmProviders: Array<{ name: string; category: string; displayName: string; defaultBaseUrl: string }>;

  // search
  searchProvider: string;
  setSearchProvider: (v: string) => void;
  searchKey: string;
  setSearchKey: (v: string) => void;
  searchProviders: Array<{ name: string; category: string; displayName: string; defaultBaseUrl: string }>;

  // display helper
  providerDisplay: (n: string) => string;

  // journey desc
  jdesc: (key: string, fallback: string) => string;

  // advance without run() wrapper — used by skip button to avoid busy flicker
  advance: () => void;

  // finish
  finish: (onFinish?: () => void) => void;
}

export function useOnboardingFlow(): OnboardingFlowState {
  const { t } = useTranslation("onboarding");
  const prefs = useSettingsStore();
  const qc = useQueryClient();
  const pushToast = useToastStore((s) => s.pushToast);

  // Sync prefs.lang → i18n when changed inside the wizard (before App mounts).
  useEffect(() => { i18n.changeLanguage(prefs.lang); }, [prefs.lang]);

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
  const [createdUserId, setCreatedUserId] = useState<string | null>(null);
  // model
  const [provider, setProvider] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [createdKeyId, setCreatedKeyId] = useState<string | null>(null);
  const [createdKeyText, setCreatedKeyText] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [verified, setVerified] = useState(false);
  const [verifyError, setVerifyError] = useState("");
  const [models, setModels] = useState<string[]>([]);
  const [modelId, setModelId] = useState("");
  // search
  const [searchProvider, setSearchProvider] = useState("");
  const [searchKey, setSearchKey] = useState("");

  const llmProviders = (providers as Array<{ name: string; category: string; displayName: string; defaultBaseUrl: string }>)
    .filter((p) => p.category === "llm" && p.name !== "mock" && p.name !== "custom");
  const searchProviders = (providers as Array<{ name: string; category: string; displayName: string; defaultBaseUrl: string }>)
    .filter((p) => p.category === "search");
  const stepKey = STEP_KEYS[step] as StepKey;
  const providerDisplay = (n: string) => (providers as Array<{ name: string; displayName: string }>).find((p) => p.name === n)?.displayName || n;

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    try { await fn(); }
    catch (err: unknown) { pushToast({ kind: "error", title: t("toast.opFail"), desc: (err as Error).message }); }
    finally { setBusy(false); }
  };

  const ensureUser = async () => {
    if (createdUserId) return;
    const user = await createUser.mutateAsync({
      username: name.trim().toLowerCase().replace(/\s+/g, "-"),
      displayName: name.trim(),
      avatarColor: ACCENTS.find(([k]: [string, string]) => k === prefs.accent)?.[1] || "#d97757",
    });
    setCreatedUserId(user.id);
    useSessionStore.getState().setCurrentUser(user.id);
  };

  // Switching provider drops key/model state; best-effort delete the orphaned
  // key created for the previous provider.
  const pickProvider = (n: string) => {
    if (createdKeyId) deleteKey.mutate(createdKeyId);
    setProvider(n);
    setApiKey(""); setCreatedKeyId(null); setCreatedKeyText("");
    setVerified(false); setModels([]); setModelId(""); setVerifyError("");
  };

  const onKeyChange = (v: string) => {
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
      const res = await testKey.mutateAsync(keyId!);
      const found = (res as { modelsFound?: string[] })?.modelsFound || [];
      const opts = found.length ? found : ((PROVIDER_DEFAULT_MODEL as Record<string, string>)[provider] ? [(PROVIDER_DEFAULT_MODEL as Record<string, string>)[provider]] : []);
      setModels(opts);
      setModelId(opts[0] || "");
      setVerified(true);
      pushToast({ kind: "success", title: t("toast.keyVerified") });
    } catch {
      setVerified(false);
      setVerifyError(t("model.verifyFail"));
    } finally {
      setVerifying(false);
    }
  });

  const finish = (onFinish?: () => void) => {
    useSessionStore.getState().setStatus("ready");
    qc.invalidateQueries();
    pushToast({ kind: "success", title: t("toast.welcome"), desc: name.trim() });
    onFinish?.();
  };

  const advance = () => setStep((s) => Math.min(STEP_KEYS.length - 1, s + 1));
  const back = () => setStep((s) => Math.max(0, s - 1));

  // handleNext takes onFinish so the "done" step can call back to the component.
  const handleNext = (onFinish?: () => void) => {
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
      case "done": return finish(onFinish);
      default: return advance();
    }
  };

  const canNext = () => (stepKey === "workspace" ? name.trim().length > 0 : true);

  // journey desc shows captured values once a step is behind us.
  const jdesc = (key: string, fallback: string) => {
    if (key === "workspace" && createdUserId) return name.trim();
    if (key === "appearance" && step > 2) return `${prefs.accent} · ${prefs.lang === "zh" ? "中文" : "EN"}`;
    if (key === "model" && step > 3) return verified ? providerDisplay(provider) : t("done.none");
    if (key === "search" && step > 4) return searchProvider ? providerDisplay(searchProvider) : t("done.none");
    return fallback;
  };

  return {
    step,
    stepKey,
    busy,
    next: handleNext,
    back,
    canNext,
    name,
    setName,
    createdUserId,
    provider,
    pickProvider,
    apiKey,
    onKeyChange,
    verify,
    verifying,
    verified,
    verifyError,
    models,
    modelId,
    setModelId,
    llmProviders,
    searchProvider,
    setSearchProvider,
    searchKey,
    setSearchKey,
    searchProviders,
    providerDisplay,
    jdesc,
    advance,
    finish,
  };
}
