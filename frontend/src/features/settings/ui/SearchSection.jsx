// SearchSection — search-key management inside SettingsModal.
// 搜索默认 is the backend isDefault flag on the key itself (not a model-config).
// Backend enforces single-default per category: setting one clears siblings.
//
// 搜索密钥管理。搜索默认 = key 上的 isDefault 标记(非 model-config)。
// 后端保证同 category 单默认;设置一个即服务端清除其他。

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Icon } from "@/components/primitives/Icon.jsx";
import { Button } from "@/components/primitives/Button.jsx";
import { useToastStore } from "@shared/ui/toastStore";
import {
  useApiKeys, useProviders, useCreateApiKey,
  useTestApiKey, useDeleteApiKey, useUpdateApiKey,
} from "@/api/config.js";
import { apiFetch, qk } from "@/api/client.js";
import { SEARCH_HINTS } from "@shared/lib/onboarding-strings.js";
import { ProviderGrid } from "./ProviderGrid.jsx";
import { KeyVerifyField } from "./KeyVerifyField.jsx";

export function SearchSection({ open, onToggle }) {
  const { t } = useTranslation("settings");
  const { data: providers = [] } = useProviders();
  const { data: allKeys = [] } = useApiKeys();

  const searchProviders = providers.filter((p) => p.category === "search");
  const searchNames = new Set(searchProviders.map((p) => p.name));
  const keys = allKeys.filter((k) => searchNames.has(k.provider));
  const defaultKey = keys.find((k) => k.isDefault);

  const providerDisplay = (n) => providers.find((p) => p.name === n)?.displayName || n;

  const sub = defaultKey
    ? t("search.subWithDefault", { provider: providerDisplay(defaultKey.provider) })
    : keys.length > 0
    ? t("search.subNoDefault", { count: keys.length })
    : t("search.subEmpty");

  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.Search className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">
            {t("search.title")}
            <span className="set-sec-opt-tag">{t("search.optional")}</span>
          </div>
          <div className="set-sec-t2">{sub}</div>
        </div>
        <Icon.ChevronRight className={"set-sec-chev icon" + (open ? " is-open" : "")} />
      </button>
      {open && (
        <div className="set-sec-p">
          <KeyList
            keys={keys}
            providers={searchProviders}
            defaultKey={defaultKey}
            providerDisplay={providerDisplay}
          />
        </div>
      )}
    </div>
  );
}

function KeyList({ keys, providers, defaultKey, providerDisplay }) {
  const { t } = useTranslation("settings");
  const [openKey, setOpenKey] = useState(null);
  const [adding, setAdding] = useState(false);

  const toggleKey = (id) => setOpenKey((p) => (p === id ? null : id));

  return (
    <>
      {keys.length === 0 && !adding && (
        <div className="set-sec-empty">{t("search.emptyList")}</div>
      )}
      <div className="set-klist">
        {keys.map((key) => (
          <KeyItem
            key={key.id}
            apiKey={key}
            isDefault={key.isDefault === true}
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
          hasSearchDefault={!!defaultKey}
          providerDisplay={providerDisplay}
          onDone={() => setAdding(false)}
        />
      ) : (
        <button className="set-addbtn" onClick={() => setAdding(true)}>
          <Icon.Plus /> {t("search.addSearchService")}
        </button>
      )}
    </>
  );
}

function KeyItem({ apiKey, isDefault, displayName, open, onToggle }) {
  const { t } = useTranslation("settings");
  const pushToast = useToastStore((s) => s.pushToast);
  const testKey = useTestApiKey();
  const deleteKey = useDeleteApiKey();
  const updateKey = useUpdateApiKey(apiKey.id);

  const verified = apiKey.testStatus === "ok";
  const hint = SEARCH_HINTS[apiKey.provider] || { abbr: apiKey.provider.slice(0, 2).toUpperCase(), color: "#6b6459" };

  const setDefault = () => {
    updateKey.mutate(
      { isDefault: true },
      { onSuccess: () => pushToast({ kind: "success", title: t("search.setDefaultSuccess", { provider: displayName }) }) },
    );
  };

  const unsetDefault = () => {
    updateKey.mutate(
      { isDefault: false },
      { onSuccess: () => pushToast({ kind: "success", title: t("search.unsetDefaultSuccess") }) },
    );
  };

  const retest = () => testKey.mutate(apiKey.id, {
    onSuccess: (res) => pushToast(
      res?.ok
        ? { kind: "success", title: t("search.retestSuccess") }
        : { kind: "error", title: t("search.retestFail") },
    ),
    onError: (e) => pushToast({ kind: "error", title: t("search.retestError"), desc: e.message }),
  });

  const remove = () => {
    if (!window.confirm(t("search.deleteConfirm", { provider: displayName }))) return;
    deleteKey.mutate(apiKey.id, {
      onSuccess: () => pushToast({ kind: "success", title: t("search.deleteSuccess") }),
      onError: (e) => pushToast({ kind: "error", title: t("search.deleteFail"), desc: e.message }),
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
        {isDefault && <span className="set-badge is-default">{t("search.searchDefault")}</span>}
        {verified && <span className="set-badge is-ok">{t("search.verified")}</span>}
        <Icon.ChevronRight className="set-kchev icon" />
      </div>
      {open && (
        <div className="set-kdetail">
          <div className="set-drow">
            <div className="set-dk">{t("search.usage")}</div>
            <div className="set-seg">
              <button
                className={"set-seg-opt" + (isDefault ? " is-on" : "")}
                onClick={setDefault}
                disabled={isDefault}
              >
                {t("search.usageDefault")}
              </button>
              <button
                className={"set-seg-opt" + (!isDefault ? " is-on" : "")}
                onClick={unsetDefault}
                disabled={!isDefault}
              >
                {t("search.usageBackup")}
              </button>
            </div>
          </div>
          <div className="set-dact">
            <button className="set-link" onClick={retest} disabled={testKey.isPending}>
              {verified ? t("search.retest") : t("search.test")}
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

// AddPanel — inline verify flow: create key → :test → cleanup-on-switch/cancel.
// No ModelSelect for search keys. If no search default exists yet, first key
// is auto-promoted to isDefault on save (best-effort via direct PATCH).
//
// 内联验证流;无模型选择。首个搜索 key 保存时尽力设为搜索默认。
function AddPanel({ providers, configured, hasSearchDefault, providerDisplay, onDone }) {
  const { t } = useTranslation("settings");
  const pushToast = useToastStore((s) => s.pushToast);
  const qc = useQueryClient();
  const createKey = useCreateApiKey();
  const testKey = useTestApiKey();
  const deleteKey = useDeleteApiKey();

  const [provider, setProvider] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [createdKeyId, setCreatedKeyId] = useState(null);
  const [createdKeyText, setCreatedKeyText] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [verified, setVerified] = useState(false);
  const [verifyError, setVerifyError] = useState("");
  const [saving, setSaving] = useState(false);

  const display = providerDisplay(provider);

  const pickProvider = (n) => {
    if (createdKeyId) deleteKey.mutate(createdKeyId);
    setProvider(n);
    setApiKey(""); setCreatedKeyId(null); setCreatedKeyText("");
    setVerified(false); setVerifyError("");
  };

  const onKeyChange = (v) => {
    setApiKey(v);
    setVerifyError("");
    if (verified) setVerified(false);
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
          provider, key: apiKey, displayName: `${provider}`,
        });
        keyId = k.id; setCreatedKeyId(k.id); setCreatedKeyText(apiKey);
      }
      await testKey.mutateAsync(keyId);
      setVerified(true);
      pushToast({ kind: "success", title: t("search.addPanel.verifySuccess") });
    } catch {
      setVerified(false);
      setVerifyError(t("search.addPanel.verifyFail"));
    } finally {
      setVerifying(false);
    }
  };

  const reset = () => {
    setProvider(""); setApiKey(""); setCreatedKeyId(null); setCreatedKeyText("");
    setVerified(false); setVerifyError("");
  };

  const cancel = () => {
    if (createdKeyId) deleteKey.mutate(createdKeyId);
    reset();
    onDone();
  };

  const save = async () => {
    setSaving(true);
    try {
      // Auto-promote first search key to default (best-effort; non-fatal if it
      // fails, user can always click 搜索默认 in the key detail).
      //
      // 首个 key 尽力设默认；失败不阻断 onDone。
      if (!hasSearchDefault && createdKeyId) {
        try {
          await apiFetch(`/api-keys/${createdKeyId}`, { method: "PATCH", body: { isDefault: true } });
          await qc.invalidateQueries({ queryKey: qk.apikeys() });
        } catch {
          // Non-fatal: user can set default manually from key detail.
        }
      }
      reset();
      onDone();
    } catch (e) {
      pushToast({ kind: "error", title: t("search.addPanel.saveFail"), desc: e.message });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="set-addpanel">
      <div className="set-ap-head">
        <div className="set-ap-t">{t("search.addPanel.title")}</div>
        <button className="set-ap-x" onClick={cancel}><Icon.X /></button>
      </div>
      <div className="set-ap-body">
        <ProviderGrid
          providers={providers}
          hints={SEARCH_HINTS}
          configured={configured}
          selected={provider}
          onPick={pickProvider}
        />
        <div className="set-ap-scrollnote">{t("search.addPanel.scrollNote")}</div>

        {provider && (
          <div className="set-ap-fields">
            <div className="set-ap-field">
              <KeyVerifyField
                label={`${display} API Key`}
                value={apiKey}
                onChange={onKeyChange}
                onVerify={verify}
                verifying={verifying}
                verified={verified}
                error={verifyError}
                verifyLabel={t("search.addPanel.verifyLabel")}
                verifyingLabel={t("search.addPanel.verifyingLabel")}
                verifiedLabel={t("search.addPanel.verifiedLabel")}
                placeholder={t("search.addPanel.placeholder")}
              />
            </div>
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
