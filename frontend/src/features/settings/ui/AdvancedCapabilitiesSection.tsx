// AdvancedCapabilitiesSection — tune operational ceilings (agent steps, output,
// timeouts, tool results) backed by settings.json. Collapsed by default; values
// hot-reload. Component holds no business logic — it reads useLimits and writes
// useUpdateLimits (= S6 / 前端铁律).
//
// AdvancedCapabilitiesSection —— 调运行上限（agent 步数 / 输出 / 超时 / 工具结果），
// 存 settings.json，默认折叠，改完热重载。组件零业务逻辑，只 useLimits/useUpdateLimits。

import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { useToastStore } from "@shared/ui/toastStore";
import { useLimits, useUpdateLimits, DEFAULT_LIMITS, type Limits } from "@entities/settings";

export function AdvancedCapabilitiesSection({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  const { t } = useTranslation("settings");
  const { data: limits } = useLimits();
  const update = useUpdateLimits();
  const pushToast = useToastStore((s) => s.pushToast);
  const [draft, setDraft] = useState<Limits | null>(null);

  useEffect(() => {
    if (limits) setDraft(limits);
  }, [limits]);

  const dirty = !!draft && !!limits && JSON.stringify(draft) !== JSON.stringify(limits);

  const setGroup = <G extends keyof Limits>(g: G, patch: Partial<Limits[G]>) =>
    setDraft((d) => (d ? ({ ...d, [g]: { ...d[g], ...patch } } as Limits) : d));

  const save = () => {
    if (!draft) return;
    update.mutate(draft, {
      onSuccess: () => pushToast({ kind: "success", title: t("advanced.saved") }),
      onError: (e) => pushToast({ kind: "error", title: t("advanced.saveFail"), desc: e.message }),
    });
  };

  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.Settings className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">
            {t("advanced.title")}
            <span className="set-sec-opt-tag">{t("advanced.optional")}</span>
          </div>
          <div className="set-sec-t2">{t("advanced.sub")}</div>
        </div>
        <Icon.ChevronRight className={"set-sec-chev icon" + (open ? " is-open" : "")} />
      </button>
      {open && draft && (
        <div className="set-sec-p">
          <div className="set-sec-empty">{t("advanced.warning")}</div>

          <Group title={t("advanced.groupAgent")}>
            <NumField label={t("advanced.maxSteps")} hint={t("advanced.zeroUnlimited")} value={draft.agent.maxSteps} onChange={(v) => setGroup("agent", { maxSteps: v })} />
            <NumField label={t("advanced.maxTurnDurationSec")} hint={t("advanced.zeroUnlimited")} value={draft.agent.maxTurnDurationSec} onChange={(v) => setGroup("agent", { maxTurnDurationSec: v })} />
            <NumField label={t("advanced.subagentTimeoutSec")} value={draft.agent.subagentTimeoutSec} onChange={(v) => setGroup("agent", { subagentTimeoutSec: v })} />
            <NumField label={t("advanced.subagentMaxTurns")} value={draft.agent.subagentMaxTurns} onChange={(v) => setGroup("agent", { subagentMaxTurns: v })} />
          </Group>

          <Group title={t("advanced.groupOutput")}>
            <NumField label={t("advanced.unknownModelMaxTokens")} value={draft.output.unknownModelMaxTokens} onChange={(v) => setGroup("output", { unknownModelMaxTokens: v })} />
          </Group>

          <Group title={t("advanced.groupContext")}>
            <NumField label={t("advanced.softRatio")} step={0.05} value={draft.context.softRatio} onChange={(v) => setGroup("context", { softRatio: v })} />
            <NumField label={t("advanced.hardRatio")} step={0.05} value={draft.context.hardRatio} onChange={(v) => setGroup("context", { hardRatio: v })} />
          </Group>

          <Group title={t("advanced.groupTimeout")}>
            <NumField label={t("advanced.llmIdleSec")} value={draft.timeout.llmIdleSec} onChange={(v) => setGroup("timeout", { llmIdleSec: v })} />
            <NumField label={t("advanced.mcpCallSec")} value={draft.timeout.mcpCallSec} onChange={(v) => setGroup("timeout", { mcpCallSec: v })} />
            <NumField label={t("advanced.bashDefaultTimeoutSec")} value={draft.timeout.bashDefaultTimeoutSec} onChange={(v) => setGroup("timeout", { bashDefaultTimeoutSec: v })} />
          </Group>

          <Group title={t("advanced.groupTools")}>
            <NumField label={t("advanced.searchTopN")} value={draft.tools.searchTopN} onChange={(v) => setGroup("tools", { searchTopN: v })} />
            <NumField label={t("advanced.readDefaultLines")} value={draft.tools.readDefaultLines} onChange={(v) => setGroup("tools", { readDefaultLines: v })} />
            <NumField label={t("advanced.bashOutputCapKB")} value={draft.tools.bashOutputCapKB} onChange={(v) => setGroup("tools", { bashOutputCapKB: v })} />
          </Group>

          <Group title={t("advanced.groupWorkflow")}>
            <NumField label={t("advanced.agentNodeMaxTurns")} value={draft.workflow.agentNodeMaxTurns} onChange={(v) => setGroup("workflow", { agentNodeMaxTurns: v })} />
            <NumField label={t("advanced.agentNodeMaxTurnsHard")} value={draft.workflow.agentNodeMaxTurnsHard} onChange={(v) => setGroup("workflow", { agentNodeMaxTurnsHard: v })} />
          </Group>

          <div className="set-ap-actions">
            <Button variant="ghost" size="sm" onClick={() => setDraft(DEFAULT_LIMITS)} disabled={update.isPending}>
              {t("advanced.restoreDefaults")}
            </Button>
            <Button variant="accent" size="sm" onClick={save} disabled={!dirty || update.isPending} loading={update.isPending}>
              {t("common:save")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div style={{ marginTop: 12 }}>
      <div className="set-sec-t1" style={{ marginBottom: 4, fontSize: 12, color: "var(--fg-muted)" }}>{title}</div>
      {children}
    </div>
  );
}

function NumField({ label, value, onChange, step, hint }: { label: string; value: number; onChange: (v: number) => void; step?: number; hint?: string }) {
  return (
    <label className="set-drow" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8, padding: "4px 0" }}>
      <span className="set-dk">
        {label}
        {hint && <span className="set-sec-t2"> · {hint}</span>}
      </span>
      <input
        type="number"
        step={step ?? 1}
        className="set-acct-add-input"
        style={{ width: 120, textAlign: "right" }}
        value={value}
        onChange={(e) => {
          const n = e.target.valueAsNumber;
          onChange(Number.isNaN(n) ? 0 : n);
        }}
      />
    </label>
  );
}
