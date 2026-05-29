// ModelCapOverrideEditor — advanced escape hatch for when the static capability
// catalog is stale or wrong for a specific model. Rendered as a subtle link
// that expands inline; intentionally unobtrusive since it's rarely needed.
//
// 高级逃生舱：静态目录与实际能力不符时手动覆盖 thinkingShape / contextWindow /
// maxOutput。以折叠链接形式嵌入 ScenarioCard，不占视觉主位。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Select } from "@shared/ui/Select";
import { Button } from "@shared/ui/Button";
import {
  useSetModelCapabilityOverride,
  useClearModelCapabilityOverride,
  type ModelCapability,
  type ThinkingShape,
} from "@entities/model-config";

interface Props {
  provider: string;
  modelId: string;
  current: ModelCapability | undefined;
}

const THINKING_SHAPES: ThinkingShape[] = ["none", "effort", "budget", "toggle"];

export function ModelCapOverrideEditor({ provider, modelId, current }: Props) {
  const { t } = useTranslation("settings");
  const [open, setOpen] = useState(false);
  const [shape, setShape] = useState<ThinkingShape>(current?.thinkingShape ?? "none");
  const [window, setWindow] = useState<string>(String(current?.contextWindow ?? ""));
  const [output, setOutput] = useState<string>(String(current?.maxOutput ?? ""));

  const setOverride = useSetModelCapabilityOverride();
  const clearOverride = useClearModelCapabilityOverride();

  const handleOpen = () => {
    setShape(current?.thinkingShape ?? "none");
    setWindow(String(current?.contextWindow ?? ""));
    setOutput(String(current?.maxOutput ?? ""));
    setOpen(true);
  };

  const handleSave = () => {
    const body: { thinkingShape?: ThinkingShape; contextWindow?: number; maxOutput?: number } = {
      thinkingShape: shape,
    };
    const w = parseInt(window, 10);
    const o = parseInt(output, 10);
    if (!isNaN(w) && w > 0) body.contextWindow = w;
    if (!isNaN(o) && o > 0) body.maxOutput = o;
    setOverride.mutate({ provider, modelId, ...body });
    setOpen(false);
  };

  const handleRestore = () => {
    clearOverride.mutate({ provider, modelId });
    setOpen(false);
  };

  if (!open) {
    return (
      <button type="button" className="set-cap-link" onClick={handleOpen}>
        {t("capOverride.trigger")}
      </button>
    );
  }

  return (
    <div className="set-cap-editor">
      <div className="set-cap-row">
        <div className="onb-klabel">{t("capOverride.thinkingShapeLabel")}</div>
        <Select
          options={THINKING_SHAPES.map((s) => ({
            value: s,
            label: t(`capOverride.shapes.${s}`),
          }))}
          value={shape}
          onChange={(v) => setShape(v as ThinkingShape)}
          ariaLabel={t("capOverride.thinkingShapeLabel")}
        />
      </div>
      <div className="set-cap-row">
        <div className="onb-klabel">{t("capOverride.contextWindowLabel")}</div>
        <input
          type="number"
          className="onb-input"
          style={{ height: 34, fontSize: "var(--fs-13)", padding: "0 10px" }}
          value={window}
          onChange={(e) => setWindow(e.target.value)}
          aria-label={t("capOverride.contextWindowLabel")}
        />
      </div>
      <div className="set-cap-row">
        <div className="onb-klabel">{t("capOverride.maxOutputLabel")}</div>
        <input
          type="number"
          className="onb-input"
          style={{ height: 34, fontSize: "var(--fs-13)", padding: "0 10px" }}
          value={output}
          onChange={(e) => setOutput(e.target.value)}
          aria-label={t("capOverride.maxOutputLabel")}
        />
      </div>
      <div className="set-cap-actions">
        <Button size="sm" variant="primary" onClick={handleSave} loading={setOverride.isPending}>
          {t("capOverride.save")}
        </Button>
        <Button size="sm" variant="ghost" onClick={handleRestore} loading={clearOverride.isPending}>
          {t("capOverride.restore")}
        </Button>
        <button type="button" className="set-cap-link" onClick={() => setOpen(false)}>
          {t("common:cancel")}
        </button>
      </div>
    </div>
  );
}
