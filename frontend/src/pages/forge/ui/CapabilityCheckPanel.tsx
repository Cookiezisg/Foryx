// CapabilityCheckPanel — inline expandable panel under WorkflowDetail
// header. Triggers POST /workflows/{id}:capability-check and renders the
// list of required capabilities + whether each is satisfied.
//
// CapabilityCheckPanel —— 工作流详情头部下折叠面板；按需触发能力检查并
// 渲染结果（每项 capability + 是否 ready）。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { motion, AnimatePresence } from "framer-motion";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { useCapabilityCheck, CapabilityCheckResult } from "@entities/workflow";
import { useToastStore } from "@shared/ui/toastStore";

interface CapabilityCheckPanelProps {
  workflowId: string;
}

export function CapabilityCheckPanel({ workflowId }: CapabilityCheckPanelProps) {
  const { t } = useTranslation("forge");
  const [open, setOpen] = useState(false);
  const [result, setResult] = useState<CapabilityCheckResult | null>(null);
  const check = useCapabilityCheck();
  const pushToast = useToastStore((s) => s.pushToast);

  const run = async () => {
    setOpen(true);
    try {
      const r = await check.mutateAsync(workflowId);
      setResult(r);
    } catch (e) {
      pushToast({ kind: "error", title: t("capability.checkFail"), desc: (e as any)?.message });
    }
  };

  return (
    <>
      <Button size="sm" onClick={run} disabled={check.isPending}>
        {check.isPending ? <><span className="spinner" /> {t("capability.checking")}</> : <><Icon.Eye /> {t("capability.checkBtn")}</>}
      </Button>
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0, y: -6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -4 }}
            className="cap-panel"
          >
            <div className="cap-panel-head">
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <Icon.Eye />
                <strong>{t("capability.panelTitle")}</strong>
                {result?.ok && <span className="badge success">{t("capability.allReady")}</span>}
                {result && !result.ok && <span className="badge warn">{t("capability.missingCount", { count: (result.issues || []).length })}</span>}
              </div>
              <button className="icon-btn" onClick={() => setOpen(false)}><Icon.X /></button>
            </div>
            <div className="cap-panel-body">
              {!result && check.isPending && <div className="empty"><div className="sub">{t("capability.loading")}</div></div>}
              {result && (
                <CapabilityResult result={result} />
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  );
}

function CapabilityResult({ result }: { result: any }) {
  const { t } = useTranslation("forge");
  const items = result.items || result.capabilities || [];
  if (items.length === 0) {
    return (
      <div className="empty" style={{ padding: 12 }}>
        <div className="sub">{t("capability.noCapabilitiesNeeded")}</div>
      </div>
    );
  }
  return (
    <div className="cap-list">
      {items.map((it: any, i: number) => {
        const ok = it.ready ?? it.satisfied ?? (!it.missing);
        return (
          <div key={i} className={"cap-row" + (ok ? " is-ok" : " is-missing")}>
            {ok
              ? <Icon.Check style={{ color: "var(--status-success)", width: 13, height: 13 }} />
              : <Icon.AlertCircle style={{ color: "var(--status-error)", width: 13, height: 13 }} />}
            <span className="cell-mono">{it.kind || it.type || "capability"}</span>
            <span style={{ flex: 1 }}>{it.name || it.id || it.label}</span>
            {it.reason && <span style={{ color: "var(--fg-muted)", fontSize: 11 }}>{it.reason}</span>}
          </div>
        );
      })}
    </div>
  );
}
