// MessageView — one message (user or assistant). Meta row + body + opt
// attachments. msg-actions hide by default and fade in on hover.
//
// MessageView —— 单条消息：meta + body + 附件；actions hover 才显示。

import { memo } from "react";
import { useChatStore } from "../../store/chat.js";
import { Icon } from "../../components/primitives/Icon.jsx";
import { RelTime } from "../../components/shared/RelTime.jsx";
import { Badge } from "../../components/primitives/Badge.jsx";
import { BlockList } from "./BlockRenderer.jsx";

export const MessageView = memo(function MessageView({ convId, msgId }) {
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
          <span>{isUser ? "你" : message.model || "Forgify"}</span>
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
          <Badge kind="streaming">在写</Badge>
        )}
        {message.status === "error" && (
          <Badge kind="error">出错了</Badge>
        )}
        <div style={{ flex: 1 }} />
        <div className="msg-actions">
          {/* Only "Copy" is wired today; backend has no :fork / :regenerate
              / :edit endpoints yet, so we don't surface dead buttons. */}
          <button
            className="msg-action"
            title="复制全文"
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
// Returns { hint, action? } where hint 是要给用户看的话。
//
// 常见 provider 报错 → 一句人话。
function friendlyError(code, message) {
  const m = (message || "").toLowerCase();
  if (m.includes("insufficient balance") || m.includes("insufficient_quota")) {
    return { hint: "LLM 厂商余额不足。去 provider 后台充值,或在设置里换一个 provider。", kind: "余额" };
  }
  if (m.includes("invalid_api_key") || m.includes("401") || m.includes("unauthorized")) {
    return { hint: "API key 无效或已过期。在设置里重新填一次。", kind: "key" };
  }
  if (m.includes("rate limit") || m.includes("rate_limit") || m.includes("429")) {
    return { hint: "请求太密。等几秒再试,或换一个 provider。", kind: "限流" };
  }
  if (m.includes("model_not_found") || m.includes("model not found") || m.includes("404")) {
    return { hint: "这个模型 ID 在 provider 那边没有。在设置里换一个有效的模型。", kind: "模型" };
  }
  if (m.includes("context length") || m.includes("context_length") || m.includes("too many tokens")) {
    return { hint: "上下文太长了。开一段新对话,或让 agent 自动压缩。", kind: "上下文" };
  }
  if (code === "MODEL_NOT_CONFIGURED" || code === "API_KEY_PROVIDER_NOT_FOUND") {
    return { hint: "模型或 API key 没配好。去设置看一眼。", kind: "配置" };
  }
  if (code === "LLM_PROVIDER_ERROR" || code === "LLM_STREAM_ERROR") {
    return { hint: "LLM 厂商返回了错误。看下面的原文。", kind: "provider" };
  }
  return null;
}

function ErrorCard({ code, message, provider, model }) {
  const friendly = friendlyError(code, message);
  return (
    <div className="msg-error">
      <div className="msg-error-head">
        <Icon.AlertCircle style={{ width: 13, height: 13, color: "var(--status-error)" }} />
        <span style={{ color: "var(--status-error)", fontWeight: 500 }}>
          {friendly?.kind ? friendly.kind : "出错"}
        </span>
        {(provider || model) && (
          <span style={{ color: "var(--fg-faint)", fontFamily: "var(--font-mono)", fontSize: 11 }}>
            {[provider, model].filter(Boolean).join(" / ")}
          </span>
        )}
      </div>
      {friendly?.hint && (
        <div className="msg-error-hint">{friendly.hint}</div>
      )}
      <details className="msg-error-raw">
        <summary>原文</summary>
        <pre>
          {code && <span style={{ color: "var(--fg-faint)" }}>{code + "\n"}</span>}
          {message || "(无信息)"}
        </pre>
      </details>
    </div>
  );
}
