// ApprovalBanner — sticky banner shown at top of FlowRunDetail when one
// or more nodes are waiting_approval. Each row has approve/reject with
// optional reason. Hits POST /flowruns/{id}/approvals/{nodeId}.
//
// ApprovalBanner —— flowrun 顶部 sticky banner；每个 waiting_approval 节点
// 接 approve/reject + 可选 reason。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { motion, AnimatePresence } from "framer-motion";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { useApproveNode, useRejectNode } from "@entities/flowrun";
import { useToastStore } from "@shared/ui/toastStore";
import { slideDown } from "@shared/lib/motion";

interface ApprovalBannerProps {
  runId: string;
  nodes: any[];
}

export function ApprovalBanner({ runId, nodes }: ApprovalBannerProps) {
  const { t } = useTranslation("execute");
  const pending = (nodes || []).filter((n) =>
    n.status === "waiting_approval" || n.status === "waiting" || n.status === "wait"
  );
  if (pending.length === 0) return null;

  return (
    <motion.div className="approval-banner" {...(slideDown as any)}>
      <div className="approval-banner-head">
        <Icon.Pause style={{ width: 14, height: 14, color: "var(--status-warn)" }} />
        <strong>{t("approval.banner.title")}</strong>
        <span style={{ color: "var(--fg-muted)" }}>{t("approval.banner.pendingCount", { count: pending.length })}</span>
      </div>
      <div className="approval-banner-list">
        {pending.map((n) => (
          <ApprovalRow key={n.id} runId={runId} node={n} />
        ))}
      </div>
    </motion.div>
  );
}

function ApprovalRow({ runId, node }: { runId: string; node: any }) {
  const { t } = useTranslation("execute");
  const approve = useApproveNode();
  const reject = useRejectNode();
  const pushToast = useToastStore((s) => s.pushToast);
  const [reasonOpen, setReasonOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [decided, setDecided] = useState(null);

  const onApprove = () => {
    approve.mutate(
      { runId, nodeId: node.id, decision: "approve", reason },
      {
        onSuccess: () => { setDecided("approved"); pushToast({ kind: "success", title: t("approval.row.toast.approveSuccess"), desc: node.label || node.id }); },
        onError: (e) => pushToast({ kind: "error", title: t("approval.row.toast.approveFail"), desc: e.message }),
      }
    );
  };
  const onReject = () => {
    reject.mutate(
      { runId, nodeId: node.id, reason },
      {
        onSuccess: () => { setDecided("rejected"); pushToast({ kind: "warn", title: t("approval.row.toast.rejectSuccess"), desc: node.label || node.id }); },
        onError: (e) => pushToast({ kind: "error", title: t("approval.row.toast.rejectFail"), desc: e.message }),
      }
    );
  };

  if (decided) {
    return (
      <div className={"approval-row is-" + decided}>
        <Icon.Check style={{ width: 12, height: 12 }} />
        <span className="cell-mono">{node.label || node.id}</span>
        <span style={{ color: "var(--fg-muted)" }}>{t(`approval.row.decided.${decided}`)}</span>
        {reason && <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>· {reason}</span>}
      </div>
    );
  }

  const busy = approve.isPending || reject.isPending;

  return (
    <div className="approval-row">
      <div className="approval-row-head">
        <Icon.Clock style={{ width: 12, height: 12, color: "var(--status-warn)" }} />
        <span className="cell-mono">{node.label || node.id}</span>
        {node.kind && <span className="kind-chip">{node.kind}</span>}
        <div style={{ flex: 1 }} />
        <Button size="xs" variant="ghost" onClick={() => setReasonOpen((o) => !o)}>
          {reasonOpen ? t("approval.row.collapseReason") : t("approval.row.addReason")}
        </Button>
        <Button size="xs" variant="danger" onClick={onReject} disabled={busy}>
          <Icon.X /> {t("approval.row.rejectBtn")}
        </Button>
        <Button size="xs" variant="accent" onClick={onApprove} disabled={busy}>
          <Icon.Check /> {t("approval.row.approveBtn")}
        </Button>
      </div>
      <AnimatePresence>
        {reasonOpen && (
          <motion.input
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            className="cfg-input"
            style={{ marginTop: 6 }}
            placeholder={t("approval.row.reasonPlaceholder")}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        )}
      </AnimatePresence>
    </div>
  );
}
