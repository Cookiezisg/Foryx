// ApiKeysSection — key-centric LLM key management inside SettingsModal.
// 对话默认 (chat default) is a model-config row, NOT a key flag: a key is the
// default iff model-config.chat.provider === key.provider. Promoting a key
// upserts that one chat row (which implicitly replaces the prior default).
//
// 按 key 组织的 LLM 密钥管理。对话默认 = model-config 的 chat 行,不是 key
// 上的标记;升级某 key 即 upsert chat 行(隐式顶掉旧默认)。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { useToastStore } from "@shared/ui/toastStore";
import { useApiKeys, useCreateApiKey, useTestApiKey, useDeleteApiKey } from "@entities/apikey";
import { useProviders, useModelConfigs, useUpsertModelConfig } from "@entities/model-config";
import { LLM_HINTS, PROVIDER_DEFAULT_MODEL } from "@shared/lib/onboarding-strings";
import { ProviderGrid } from "./ProviderGrid.tsx";
import { KeyVerifyField } from "./KeyVerifyField.tsx";
import { ModelSelect } from "./ModelSelect.tsx";

export function ApiKeysSection({ open, onToggle }) {
  const { t } = useTranslation("settings");
  const { data: providers = [] } = useProviders();
  const { data: allKeys = [] } = useApiKeys();
  const { data: modelConfigs = [] } = useModelConfigs();

  const llmProviders = providers.filter(
    (p) => p.category === "llm" && p.name !== "mock" && p.name !== "custom",
  );
  const llmNames = new Set(llmProviders.map((p) => p.name));
  const keys = allKeys.filter((k) => llmNames.has(k.provider));
  const chatConfig = modelConfigs.find((m) => m.scenario === "chat");

  const providerDisplay = (n) => providers.find((p) => p.name === n)?.displayName || n;
  const sub = keys.length
    ? t("apiKeys.subWithDefault", { count: keys.length, provider: chatConfig ? providerDisplay(chatConfig.provider) : t("apiKeys.subNotSet") })
    : t("apiKeys.subEmpty");

  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.KeyRound className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">API Keys</div>
          <div className="set-sec-t2">{sub}</div>
        </div>
        <Icon.ChevronRight className={"set-sec-chev icon" + (open ? " is-open" : "")} />
      </button>
      {open && (
        <div className="set-sec-p">
          <KeyList
            keys={keys}
            providers={llmProviders}
            chatConfig={chatConfig}
            providerDisplay={providerDisplay}
          />
        </div>
      )}
    </div>
  );
}

function KeyList({ keys, providers, chatConfig, providerDisplay }) {
  const { t } = useTranslation("settings");
  const [openKey, setOpenKey] = useState(null);
  const [adding, setAdding] = useState(false);

  const toggleKey = (id) => setOpenKey((p) => (p === id ? null : id));

  return (
    <>
      {keys.length === 0 && !adding && (
        <div className="set-sec-empty">{t("apiKeys.emptyList")}</div>
      )}
      <div className="set-klist">
        {keys.map((key) => (
          <KeyItem
            key={key.id}
            apiKey={key}
            isDefault={chatConfig?.provider === key.provider}
            chatConfig={chatConfig}
            displayName={providerDisplay(key.provider)}
            open={openKey === key.id}
            onToggle={() => toggleKey(key.id)}
          />
        ))}
      </div>

      {adding ? (
        <AddPanel
          providers={providers}
          configured={keys.map((k) => k.provider)}
          hasChatDefault={!!chatConfig}
          providerDisplay={providerDisplay}
          onDone={() => setAdding(false)}
        />
      ) : (
        <button className="set-addbtn" onClick={() => setAdding(true)}>
          <Icon.Plus /> API Key
        </button>
      )}
    </>
  );
}

function KeyItem({ apiKey, isDefault, chatConfig, displayName, open, onToggle }) {
  const { t } = useTranslation("settings");
  const pushToast = useToastStore((s) => s.pushToast);
  const testKey = useTestApiKey();
  const deleteKey = useDeleteApiKey();
  const upsertModel = useUpsertModelConfig();

  const models = apiKey.modelsFound || [];
  const [localModel, setLocalModel] = useState(models[0] || "");
  const verified = apiKey.testStatus === "ok";
  const hint = LLM_HINTS[apiKey.provider] || { abbr: apiKey.provider.slice(0, 2).toUpperCase(), color: "#6b6459" };

  const modelValue = isDefault ? chatConfig?.modelId || "" : localModel;
  const onModelChange = (v) => {
    if (isDefault) {
      upsertModel.mutate({ scenario: "chat", provider: apiKey.provider, modelId: v });
    } else {
      setLocalModel(v);
    }
  };

  const canPromote = !isDefault && (modelValue || models.length > 0);
  const promote = () => {
    if (!canPromote) return;
    upsertModel.mutate(
      { scenario: "chat", provider: apiKey.provider, modelId: modelValue || models[0] },
      { onSuccess: () => pushToast({ kind: "success", title: t("apiKeys.promoteSuccess", { provider: displayName }) }) },
    );
  };

  const retest = () => testKey.mutate(apiKey.id, {
    onSuccess: (res) => pushToast(
      res?.ok
        ? { kind: "success", title: t("apiKeys.retestSuccess") }
        : { kind: "error", title: t("apiKeys.retestFail") },
    ),
    onError: (e) => pushToast({ kind: "error", title: t("apiKeys.retestError"), desc: e.message }),
  });

  const remove = () => {
    if (!window.confirm(t("apiKeys.deleteConfirm", { provider: displayName }))) return;
    deleteKey.mutate(apiKey.id, {
      onSuccess: () => pushToast({ kind: "success", title: t("apiKeys.deleteSuccess") }),
      onError: (e) => pushToast({ kind: "error", title: t("apiKeys.deleteFail"), desc: e.message }),
    });
  };

  return (
    <div className={"set-kitem" + (isDefault ? " is-default" : "") + (open ? " is-open" : "")}>
      <div className="set-kitem-row" onClick={onToggle}>
        <span className="set-pchip" style={{ background: hint.color }}>{hint.abbr}</span>
        <div className="set-kitem-id">
          <div className="set-pn">{apiKey.displayName || displayName}</div>
          <div className="set-pk">{apiKey.keyMasked}</div>
        </div>
        {isDefault && chatConfig?.modelId && <span className="set-mtag">{chatConfig.modelId}</span>}
        {isDefault && <span className="set-badge is-default">{t("apiKeys.chatDefault")}</span>}
        {verified && <span className="set-badge is-ok">{t("apiKeys.verified")}</span>}
        <Icon.ChevronRight className="set-kchev icon" />
      </div>
      {open && (
        <div className="set-kdetail">
          {models.length > 0 && (
            <div className="set-drow">
              <div className="set-dk">{t("apiKeys.model")}</div>
              <ModelSelect models={models} value={modelValue} onChange={onModelChange} />
            </div>
          )}
          <div className="set-drow">
            <div className="set-dk">{t("apiKeys.usage")}</div>
            <div className="set-seg">
              <button
                className={"set-seg-opt" + (isDefault ? " is-on" : "")}
                onClick={promote}
                disabled={!isDefault && !canPromote}
              >
                {t("apiKeys.usageChatDefault")}
              </button>
              <button className={"set-seg-opt" + (isDefault ? "" : " is-on")} disabled={isDefault}>
                {t("apiKeys.usageBackup")}
              </button>
            </div>
          </div>
          <div className="set-dact">
            <button className="set-link" onClick={retest} disabled={testKey.isPending}>
              {verified ? t("apiKeys.retest") : t("apiKeys.test")}
            </button>
            <button className="set-link is-danger" onClick={remove} disabled={deleteKey.isPending}>
              {t("common:delete")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

// AddPanel — onboarding's verify flow inlined: create key → :test → modelsFound.
// Switching provider or cancelling best-effort deletes the orphan created key.
//
// AddPanel —— 内联引导页验证流;切换厂商/取消时尽力删除已创建的孤儿 key。
function AddPanel({ providers, configured, hasChatDefault, providerDisplay, onDone }) {
  const { t } = useTranslation("settings");
  const pushToast = useToastStore((s) => s.pushToast);
  const createKey = useCreateApiKey();
  const testKey = useTestApiKey();
  const deleteKey = useDeleteApiKey();
  const upsertModel = useUpsertModelConfig();

  const [provider, setProvider] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [createdKeyId, setCreatedKeyId] = useState(null);
  const [createdKeyText, setCreatedKeyText] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [verified, setVerified] = useState(false);
  const [verifyError, setVerifyError] = useState("");
  const [models, setModels] = useState([]);
  const [modelId, setModelId] = useState("");
  const [saving, setSaving] = useState(false);

  const isOllama = provider === "ollama";
  const display = providerDisplay(provider);

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

  const verify = async () => {
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
          provider, key: apiKey || "ollama-no-key", displayName: `${provider}`,
        });
        keyId = k.id; setCreatedKeyId(k.id); setCreatedKeyText(apiKey);
      }
      const res = await testKey.mutateAsync(keyId);
      const found = res?.modelsFound || [];
      const opts = found.length ? found : (PROVIDER_DEFAULT_MODEL[provider] ? [PROVIDER_DEFAULT_MODEL[provider]] : []);
      setModels(opts);
      setModelId(opts[0] || "");
      setVerified(true);
      pushToast({ kind: "success", title: t("apiKeys.addPanel.verifySuccess") });
    } catch {
      setVerified(false);
      setVerifyError(t("apiKeys.addPanel.verifyFail"));
    } finally {
      setVerifying(false);
    }
  };

  const reset = () => {
    setProvider(""); setApiKey(""); setCreatedKeyId(null); setCreatedKeyText("");
    setVerified(false); setModels([]); setModelId(""); setVerifyError("");
  };

  const cancel = () => {
    if (createdKeyId) deleteKey.mutate(createdKeyId);
    reset();
    onDone();
  };

  const save = async () => {
    setSaving(true);
    try {
      if (modelId && !hasChatDefault) {
        await upsertModel.mutateAsync({ scenario: "chat", provider, modelId });
      }
      reset();
      onDone();
    } catch (e) {
      pushToast({ kind: "error", title: t("apiKeys.addPanel.saveFail"), desc: e.message });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="set-addpanel">
      <div className="set-ap-head">
        <div className="set-ap-t">{t("apiKeys.addPanel.title")}</div>
        <button className="set-ap-x" onClick={cancel}><Icon.X /></button>
      </div>
      <div className="set-ap-body">
        <ProviderGrid
          providers={providers}
          hints={LLM_HINTS}
          configured={configured}
          selected={provider}
          onPick={pickProvider}
        />
        <div className="set-ap-scrollnote">{t("apiKeys.addPanel.scrollNote")}</div>

        {provider && (
          <div className="set-ap-fields">
            <div className="set-ap-field">
              <KeyVerifyField
                label={isOllama ? display : `${display} API Key`}
                value={isOllama ? "localhost:11434" : apiKey}
                onChange={onKeyChange}
                onVerify={verify}
                verifying={verifying}
                verified={verified}
                error={verifyError}
                verifyLabel={t("apiKeys.addPanel.verifyLabel")}
                verifyingLabel={t("apiKeys.addPanel.verifyingLabel")}
                verifiedLabel={t("apiKeys.addPanel.verifiedLabel")}
                placeholder="sk-…"
                readOnly={isOllama}
              />
            </div>
            {verified && models.length > 0 && (
              <div className="set-ap-field">
                <div className="onb-klabel">{t("apiKeys.addPanel.modelLabel")}</div>
                <ModelSelect models={models} value={modelId} onChange={setModelId} />
              </div>
            )}
          </div>
        )}

        <div className="set-ap-actions">
          <Button variant="ghost" size="sm" onClick={cancel} disabled={saving}>{t("common:cancel")}</Button>
          <Button variant="accent" size="sm" onClick={save} disabled={!verified || saving} loading={saving}>
            {t("common:save")}
          </Button>
        </div>
      </div>
    </div>
  );
}
