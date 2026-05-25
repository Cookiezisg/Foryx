// StatusBadge — trinity entity status (ready / pending / draft / failed).
// pending/draft show an AI sparkle marker (boilerplate `.forge-ai-mark`).

import { useTranslation } from "react-i18next";
import { Icon } from "../primitives/Icon.jsx";
import { Badge } from "../primitives/Badge.jsx";

export function StatusBadge({ status }) {
  const { t } = useTranslation("misc");
  if (status === "ready") return <Badge kind="success">{t("statusBadge.ready")}</Badge>;
  if (status === "failed") return <Badge kind="error">{t("statusBadge.failed")}</Badge>;

  if (status === "pending" || status === "draft") {
    const kind = status === "pending" ? "warn" : "info";
    const label = status === "pending" ? t("statusBadge.pending") : t("statusBadge.draft");
    return (
      <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
        <Badge kind={kind}>{label}</Badge>
        <span className="forge-ai-mark" title={t("statusBadge.aiGenerated")}>
          <Icon.Sparkles />
          <span>AI</span>
        </span>
      </span>
    );
  }

  return <Badge>{status}</Badge>;
}
