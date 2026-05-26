// MessageView — one message (user or assistant). Meta row + body + opt
// attachments. msg-actions hide by default and fade in on hover.
//
// MessageView —— 单条消息：meta + body + 附件；actions hover 才显示。

import { memo } from "react";
import { useTranslation } from "react-i18next";
import { useChatStore } from "../../../store/chat.js";
import { Icon } from "../../../components/primitives/Icon.jsx";
import { RelTime } from "../../../shared/ui/RelTime.jsx";
import { Badge } from "../../../components/primitives/Badge.jsx";
import { BlockList } from "./BlockRenderer.jsx";

export const MessageView = memo(function MessageView({ convId, msgId }) {
  const { t } = useTranslation("conv");
  const message = useChatStore((s) => s.convs[convId]?.messages.get(msgId));
  if (!message) return null;

  const isUser = message.role === "user";
  const provider = (message.model || "").split("-")[0]?.toUpperCase().slice(0, 2) || (isUser ? "Y" : "AI");

  return (
    <div className={"msg role-" + message.role} data-comment-anchor={message.id}>
      <div className="msg-meta">
        <div className="msg-author">
          <div className={"msg-author-avatar " + (isUser ? "user" : "ai")}>
            {isUser ? "Y" : provider}
          </div>
          <span>{isUser ? t("message.you") : message.model || "Forgify"}</span>
        </div>
        <span>·</span>
        <RelTime ts={message.createdAt} />
        {!isUser && message.inputTokens != null && (
          <>
            <span>·</span>
            <span className="msg-tokens">
              <span style={{ color: "var(--fg-muted)" }}>{message.inputTokens.toLocaleString()}</span>
              <span className="sep"> ↓ ↑ </span>
              <span style={{ color: "var(--fg-muted)" }}>{message.outputTokens?.toLocaleString() ?? "—"}</span>
            </span>
          </>
        )}
        {message.status === "streaming" && (
          <Badge kind="streaming">{t("message.badgeStreaming")}</Badge>
        )}
        {message.status === "error" && (
          <Badge kind="error">{t("message.badgeError")}</Badge>
        )}
        <div style={{ flex: 1 }} />
        <div className="msg-actions">
          {/* Only "Copy" is wired today; backend has no :fork / :regenerate
              / :edit endpoints yet, so we don't surface dead buttons. */}
          <button
            className="msg-action"
            title={t("message.copyFullText")}
            onClick={() => navigator.clipboard?.writeText(extractText(convId, message)).catch(() => {})}
          >
            <Icon.Copy />
          </button>
        </div>
      </div>
      <div className="msg-body">
        <BlockList convId={convId} blockIds={message.blocks} defaultOpenTools={false} />
        {message.status === "error" && (message.errorMessage || message.errorCode) && (
          <ErrorCard
            code={message.errorCode}
            message={message.errorMessage}
            provider={message.provider}
            model={message.model}
          />
        )}
        {message.attachments?.length > 0 && (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginTop: 12 }}>
            {message.attachments.map((a) => (
              <div className="attached-pill" key={a.id || a.fileName}>
                {(a.mimeType || "").startsWith("image/")
                  ? <Icon.Image className="file-icon" />
                  : <Icon.File className="file-icon" />}
                <span>{a.fileName}</span>
                {typeof a.sizeBytes === "number" && (
                  <span className="file-meta">{Math.round(a.sizeBytes / 1024)}kb</span>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
});

function extractText(convId, message) {
  const store = useChatStore.getState();
  const blocks = store.convs[convId]?.blocks;
  if (!blocks) return "";
  return message.blocks
    .map((bid) => blocks.get(bid))
    .filter((b) => b && b.type === "text")
    .map((b) => b.content)
    .join("\n\n");
}

// friendlyError — turns common provider errors into one calm line.
// Returns { hintKey, kindKey } where keys map to conv.error.* translations.
//
// 常见 provider 报错 → 翻译 key。
function friendlyError(code, message) {
  const m = (message || "").toLowerCase();
  if (m.includes("insufficient balance") || m.includes("insufficient_quota")) {
    return { hintKey: "error.insufficientBalance", kindKey: "error.kindBalance" };
  }
  if (m.includes("invalid_api_key") || m.includes("401") || m.includes("unauthorized")) {
    return { hintKey: "error.invalidApiKey", kindKey: "error.kindKey" };
  }
  if (m.includes("rate limit") || m.includes("rate_limit") || m.includes("429")) {
    return { hintKey: "error.rateLimit", kindKey: "error.kindRateLimit" };
  }
  if (m.includes("model_not_found") || m.includes("model not found") || m.includes("404")) {
    return { hintKey: "error.modelNotFound", kindKey: "error.kindModel" };
  }
  if (m.includes("context length") || m.includes("context_length") || m.includes("too many tokens")) {
    return { hintKey: "error.contextLength", kindKey: "error.kindContext" };
  }
  if (code === "MODEL_NOT_CONFIGURED" || code === "API_KEY_PROVIDER_NOT_FOUND") {
    return { hintKey: "error.notConfigured", kindKey: "error.kindConfig" };
  }
  if (code === "LLM_PROVIDER_ERROR" || code === "LLM_STREAM_ERROR") {
    return { hintKey: "error.providerError", kindKey: "error.kindProvider" };
  }
  return null;
}

function ErrorCard({ code, message, provider, model }) {
  const { t } = useTranslation("conv");
  const friendly = friendlyError(code, message);
  return (
    <div className="msg-error">
      <div className="msg-error-head">
        <Icon.AlertCircle style={{ width: 13, height: 13, color: "var(--status-error)" }} />
        <span style={{ color: "var(--status-error)", fontWeight: 500 }}>
          {friendly?.kindKey ? t(friendly.kindKey) : t("error.kindGeneric")}
        </span>
        {(provider || model) && (
          <span style={{ color: "var(--fg-faint)", fontFamily: "var(--font-mono)", fontSize: 11 }}>
            {[provider, model].filter(Boolean).join(" / ")}
          </span>
        )}
      </div>
      {friendly?.hintKey && (
        <div className="msg-error-hint">{t(friendly.hintKey)}</div>
      )}
      <details className="msg-error-raw">
        <summary>{t("error.rawSummary")}</summary>
        <pre>
          {code && <span style={{ color: "var(--fg-faint)" }}>{code + "\n"}</span>}
          {message || t("error.noMessage")}
        </pre>
      </details>
    </div>
  );
}
