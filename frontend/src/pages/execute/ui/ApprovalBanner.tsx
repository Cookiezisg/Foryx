// ApprovalBanner — sticky banner at the top of FlowRunDetail listing the run's
// parked approval nodes. Data comes from the approvals inbox (GET /approvals,
// backend 17 §9 projection) filtered by runId — NOT from node status: the
// journal parks the node, the interpreter never emits a "waiting_approval"
// node status. Each row approves/rejects via POST /flowruns/{runId}/approvals.
//
// ApprovalBanner —— flowrun 顶部 sticky banner;数据来自 approvals inbox(按 runId
// 过滤 parked 行),不是节点状态。每行 approve/reject。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { motion, AnimatePresence } from "framer-motion";
import { Icon } from "@shared/ui/Icon";
import { Button } from "@shared/ui/Button";
import { useApprovalInbox, useApproveNode, useRejectNode, type Approval } from "@entities/flowrun";
import { useToastStore } from "@shared/ui/toastStore";
import { slideDown } from "@shared/lib/motion";
import type { MotionProps } from "framer-motion";

interface ApprovalBannerProps {
  runId: string;
}

export function ApprovalBanner({ runId }: ApprovalBannerProps) {
  const { t } = useTranslation("execute");
  const { data: approvals } = useApprovalInbox();
  const pending = (approvals || []).filter(
    (a) => a.flowrunId === runId && a.status === "parked"
  );
  if (pending.length === 0) return null;

  return (
    <motion.div className="approval-banner" {...(slideDown as MotionProps)}>
      <div className="approval-banner-head">
        <Icon.Pause style={{ width: 14, height: 14, color: "var(--status-warn)" }} />
        <strong>{t("approval.banner.title")}</strong>
        <span style={{ color: "var(--fg-muted)" }}>{t("approval.banner.pendingCount", { count: pending.length })}</span>
      </div>
      <div className="approval-banner-list">
        {pending.map((a) => (
          <ApprovalRow key={a.nodeId} runId={runId} approval={a} />
        ))}
      </div>
    </motion.div>
  );
}

function ApprovalRow({ runId, approval }: { runId: string; approval: Approval }) {
  const { t } = useTranslation("execute");
  const approve = useApproveNode();
  const reject = useRejectNode();
  const pushToast = useToastStore((s) => s.pushToast);
  const [reasonOpen, setReasonOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [decided, setDecided] = useState<string | null>(null);

  const label = approval.prompt || approval.nodeId;

  const onApprove = () => {
    approve.mutate(
      { runId, nodeId: approval.nodeId, decision: "approved", reason },
      {
        onSuccess: () => { setDecided("approved"); pushToast({ kind: "success", title: t("approval.row.toast.approveSuccess"), desc: label }); },
        onError: (e) => pushToast({ kind: "error", title: t("approval.row.toast.approveFail"), desc: e.message }),
      }
    );
  };
  const onReject = () => {
    reject.mutate(
      { runId, nodeId: approval.nodeId, reason },
      {
        onSuccess: () => { setDecided("rejected"); pushToast({ kind: "warn", title: t("approval.row.toast.rejectSuccess"), desc: label }); },
        onError: (e) => pushToast({ kind: "error", title: t("approval.row.toast.rejectFail"), desc: e.message }),
      }
    );
  };

  if (decided) {
    return (
      <div className={"approval-row is-" + decided}>
        <Icon.Check style={{ width: 12, height: 12 }} />
        <span className="cell-mono">{label}</span>
        <span style={{ color: "var(--fg-muted)" }}>{t(`approval.row.decided.${decided}` as Parameters<typeof t>[0])}</span>
        {reason && <span style={{ color: "var(--fg-faint)", fontSize: 11 }}>· {reason}</span>}
      </div>
    );
  }

  const busy = approve.isPending || reject.isPending;

  return (
    <div className="approval-row">
      <div className="approval-row-head">
        <Icon.Clock style={{ width: 12, height: 12, color: "var(--status-warn)" }} />
        <span className="cell-mono">{label}</span>
        <div style={{ flex: 1 }} />
        {approval.allowReason && (
          <Button size="xs" variant="ghost" onClick={() => setReasonOpen((o) => !o)}>
            {reasonOpen ? t("approval.row.collapseReason") : t("approval.row.addReason")}
          </Button>
        )}
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
