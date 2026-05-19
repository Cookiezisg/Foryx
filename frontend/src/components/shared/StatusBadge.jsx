// StatusBadge — trinity entity status (ready / pending / draft / failed).
// pending/draft show an AI sparkle marker (boilerplate `.forge-ai-mark`).

import { Icon } from "../primitives/Icon.jsx";
import { Badge } from "../primitives/Badge.jsx";

export function StatusBadge({ status }) {
  if (status === "ready") return <Badge kind="success">ready</Badge>;
  if (status === "failed") return <Badge kind="error">failed</Badge>;

  if (status === "pending" || status === "draft") {
    const kind = status === "pending" ? "warn" : "info";
    return (
      <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
        <Badge kind={kind}>{status}</Badge>
        <span className="forge-ai-mark" title="由 AI 锻造产生">
          <Icon.Sparkles />
          <span>AI</span>
        </span>
      </span>
    );
  }

  return <Badge>{status}</Badge>;
}
