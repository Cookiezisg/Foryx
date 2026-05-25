// SearchSection — search-key management inside SettingsModal.
// 搜索默认 is the backend isDefault flag on the key itself (not a model-config).
// Backend enforces single-default per category: setting one clears siblings.
//
// 搜索密钥管理。搜索默认 = key 上的 isDefault 标记(非 model-config)。
// 后端保证同 category 单默认;设置一个即服务端清除其他。

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Icon } from "../primitives/Icon.jsx";
import { Button } from "../primitives/Button.jsx";
import { useUIStore } from "../../store/ui.js";
import {
  useApiKeys, useProviders, useCreateApiKey,
  useTestApiKey, useDeleteApiKey, useUpdateApiKey,
} from "../../api/config.js";
import { apiFetch, qk } from "../../api/client.js";
import { SEARCH_HINTS } from "../overlays/onboarding-strings.js";
import { ProviderGrid } from "./ProviderGrid.jsx";
import { KeyVerifyField } from "./KeyVerifyField.jsx";

export function SearchSection({ open, onToggle }) {
  const { data: providers = [] } = useProviders();
  const { data: allKeys = [] } = useApiKeys();

  const searchProviders = providers.filter((p) => p.category === "search");
  const searchNames = new Set(searchProviders.map((p) => p.name));
  const keys = allKeys.filter((k) => searchNames.has(k.provider));
  const defaultKey = keys.find((k) => k.isDefault);

  const providerDisplay = (n) => providers.find((p) => p.name === n)?.displayName || n;

  const sub = defaultKey
    ? `${providerDisplay(defaultKey.provider)} · 搜索默认`
    : keys.length > 0
    ? `${keys.length} 个搜索服务 · 未设默认`
    : "未配置";

  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.Search className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">
            网络搜索
            <span className="set-sec-opt-tag">可选</span>
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
  const [openKey, setOpenKey] = useState(null);
  const [adding, setAdding] = useState(false);

  const toggleKey = (id) => setOpenKey((p) => (p === id ? null : id));

  return (
    <>
      {keys.length === 0 && !adding && (
        <div className="set-sec-empty">还没有搜索密钥</div>
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
          <Icon.Plus /> 搜索服务
        </button>
      )}
    </>
  );
}

function KeyItem({ apiKey, isDefault, displayName, open, onToggle }) {
  const pushToast = useUIStore((s) => s.pushToast);
  const testKey = useTestApiKey();
  const deleteKey = useDeleteApiKey();
  const updateKey = useUpdateApiKey(apiKey.id);

  const verified = apiKey.testStatus === "ok";
  const hint = SEARCH_HINTS[apiKey.provider] || { abbr: apiKey.provider.slice(0, 2).toUpperCase(), color: "#6b6459" };

  const setDefault = () => {
    updateKey.mutate(
      { isDefault: true },
      { onSuccess: () => pushToast({ kind: "success", title: `搜索默认 → ${displayName}` }) },
    );
  };

  const unsetDefault = () => {
    updateKey.mutate(
      { isDefault: false },
      { onSuccess: () => pushToast({ kind: "success", title: "已改为仅备用" }) },
    );
  };

  const retest = () => testKey.mutate(apiKey.id, {
    onSuccess: (res) => pushToast(
      res?.ok
        ? { kind: "success", title: "API Key 已验证" }
        : { kind: "error", title: "验证未通过" },
    ),
    onError: (e) => pushToast({ kind: "error", title: "验证失败", desc: e.message }),
  });

  const remove = () => {
    if (!window.confirm(`删除 ${displayName} 的 API Key?`)) return;
    deleteKey.mutate(apiKey.id, {
      onSuccess: () => pushToast({ kind: "success", title: "已删除" }),
      onError: (e) => pushToast({ kind: "error", title: "删除失败", desc: e.message }),
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
        {isDefault && <span className="set-badge is-default">搜索默认</span>}
        {verified && <span className="set-badge is-ok">已验证</span>}
        <Icon.ChevronRight className="set-kchev icon" />
      </div>
      {open && (
        <div className="set-kdetail">
          <div className="set-drow">
            <div className="set-dk">用途</div>
            <div className="set-seg">
              <button
                className={"set-seg-opt" + (isDefault ? " is-on" : "")}
                onClick={setDefault}
                disabled={isDefault}
              >
                搜索默认
              </button>
              <button
                className={"set-seg-opt" + (!isDefault ? " is-on" : "")}
                onClick={unsetDefault}
                disabled={!isDefault}
              >
                仅备用
              </button>
            </div>
          </div>
          <div className="set-dact">
            <button className="set-link" onClick={retest} disabled={testKey.isPending}>
              {verified ? "重新验证" : "验证"}
            </button>
            <button className="set-link is-danger" onClick={remove} disabled={deleteKey.isPending}>
              删除
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
  const pushToast = useUIStore((s) => s.pushToast);
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
      pushToast({ kind: "success", title: "API Key 已验证" });
    } catch {
      setVerified(false);
      setVerifyError("验证未通过 —— 请检查 API Key 是否正确");
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
      pushToast({ kind: "error", title: "保存失败", desc: e.message });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="set-addpanel">
      <div className="set-ap-head">
        <div className="set-ap-t">添加搜索服务</div>
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
        <div className="set-ap-scrollnote">可滚动查看全部服务商 · 右上角 ✓ = 已存 Key</div>

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
                verifyLabel="验证"
                verifyingLabel="验证中…"
                verifiedLabel="已验证"
                placeholder="填入 API Key…"
              />
            </div>
          </div>
        )}

        <div className="set-ap-actions">
          <Button variant="ghost" size="sm" onClick={cancel} disabled={saving}>取消</Button>
          <Button variant="accent" size="sm" onClick={save} disabled={!verified || saving} loading={saving}>
            保存
          </Button>
        </div>
      </div>
    </div>
  );
}
