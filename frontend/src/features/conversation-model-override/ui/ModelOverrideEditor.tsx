// ModelOverrideEditor — KeyModelPicker + ThinkingControl + save/clear in a
// floating popover over the ChatHeader. Save sets the full ModelRef (including
// thinking); Clear sends null so backend falls back to dialogue default.
// Changing the model/key resets thinking — a budget valid for one model is
// meaningless for another.
//
// 弹出式编辑器;KeyModelPicker + ThinkingControl + 保存/清除。Save 写完整 ModelRef
// (含 thinking);Clear 写 null。换模型时重置 thinking。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@shared/ui/Button";
import type { ModelRef } from "@entities/conversation";
import { useModelCapabilities, capabilityFor, ThinkingControl } from "@entities/model-config";
import type { ThinkingSpec } from "@entities/model-config";
import { useApiKeys } from "@entities/apikey";
import { KeyModelPicker } from "@features/settings";
import { useConvModelOverride } from "../model/useConvModelOverride";

interface Props {
  conversationId: string;
  current: ModelRef | null;
  onClose: () => void;
}

export function ModelOverrideEditor({ conversationId, current, onClose }: Props) {
  const { t } = useTranslation(["conv", "common"]);
  const setOverride = useConvModelOverride();
  const [pending, setPending] = useState<ModelRef | null>(current);

  const { data: caps = [] } = useModelCapabilities();
  const { data: keys = [] } = useApiKeys();

  // Derive the provider from the selected key so capabilityFor can look up
  // the right capability row — provider is implicit in ApiKey, not in ModelRef.
  const provider = keys.find((k) => k.id === pending?.apiKeyId)?.provider ?? "";
  const capability = pending ? capabilityFor(caps, provider, pending.modelId) : undefined;

  const handlePickerChange = (v: { apiKeyId: string; modelId: string }) => {
    // Resetting thinking when model/key changes: a budget/effort valid for one
    // model is semantically wrong for another.
    setPending({ apiKeyId: v.apiKeyId, modelId: v.modelId });
  };

  const handleThinkingChange = (t: ThinkingSpec | undefined) => {
    if (!pending) return;
    const next: ModelRef = { ...pending };
    if (t) { next.thinking = t; } else { delete next.thinking; }
    setPending(next);
  };

  const save = async () => {
    if (!pending) return;
    await setOverride.mutateAsync({ conversationId, override: pending });
    onClose();
  };
  const clear = async () => {
    await setOverride.mutateAsync({ conversationId, override: null });
    onClose();
  };

  return (
    <div className="model-override-editor">
      <div className="moe-title">{t("conv:modelOverride.title")}</div>
      <KeyModelPicker value={pending} onChange={handlePickerChange} />
      <ThinkingControl
        capability={capability}
        value={pending?.thinking}
        onChange={handleThinkingChange}
        disabled={setOverride.isPending}
      />
      <div className="moe-actions">
        <Button variant="ghost" size="sm" onClick={clear} disabled={setOverride.isPending}>
          {t("conv:modelOverride.clear")}
        </Button>
        <Button variant="accent" size="sm" onClick={save} disabled={!pending || setOverride.isPending}>
          {t("common:save")}
        </Button>
      </div>
    </div>
  );
}
