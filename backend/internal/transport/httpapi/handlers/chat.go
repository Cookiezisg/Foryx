package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// ChatHandler serves attachment + message HTTP endpoints.
//
// ChatHandler 提供附件 + 消息 HTTP 端点。
type ChatHandler struct {
	svc *chatapp.Service
	log *zap.Logger
}

func NewChatHandler(svc *chatapp.Service, log *zap.Logger) *ChatHandler {
	return &ChatHandler{svc: svc, log: log}
}

func (h *ChatHandler) Register(mux Registrar) {
	mux.HandleFunc("POST /api/v1/attachments", h.UploadAttachment)
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", h.SendMessage)
	mux.HandleFunc("DELETE /api/v1/conversations/{id}/stream", h.CancelStream)
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", h.ListMessages)
	mux.HandleFunc("GET /api/v1/conversations/{id}/export", h.Export)
	mux.HandleFunc("GET /api/v1/conversations/{id}/llm-trace", h.LLMTrace)
}

func (h *ChatHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(chatdomain.MaxAttachmentBytes); err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("handlers.UploadAttachment: parseMultipart: %w (%v)", chatdomain.ErrAttachmentTooLarge, err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("handlers.UploadAttachment: missing file field: %w", chatdomain.ErrAttachmentParseFailed))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("handlers.UploadAttachment: read failed: %w (%v)", chatdomain.ErrAttachmentParseFailed, err))
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	att, err := h.svc.UploadAttachment(r.Context(), data, mimeType, header.Filename)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Created(w, att)
}

type sendMessageRequest struct {
	Content       string                       `json:"content"`
	AttachmentIDs []string                     `json:"attachmentIds"`
	Mentions      []mentiondomain.MentionInput `json:"mentions"`
}

func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req sendMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	msgID, err := h.svc.Send(r.Context(), id, chatapp.SendInput{
		Content:       req.Content,
		AttachmentIDs: req.AttachmentIDs,
		Mentions:      req.Mentions,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Success(w, http.StatusAccepted, map[string]string{"messageId": msgID})
}

func (h *ChatHandler) CancelStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Cancel(r.Context(), id); err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.NoContent(w)
}

func (h *ChatHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := paginationpkg.Parse(r)
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	items, next, err := h.svc.ListMessages(r.Context(), id, chatdomain.ListFilter{
		Cursor: p.Cursor,
		Limit:  p.Limit,
	})
	if err != nil {
		responsehttpapi.FromDomainError(w, h.log, err)
		return
	}
	responsehttpapi.Paged(w, items, next, next != "")
}

// Export serializes a conversation + all messages + blocks to markdown or JSON
// (§4.4). Format defaults to md; json dumps the typed entities verbatim.
//
// Export 把对话 + 全部 messages + blocks 序列化为 md 或 json(§4.4)。
// format 默认 md;json 直接 dump entity。
func (h *ChatHandler) Export(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "json" {
		responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
			"format must be 'md' or 'json'", nil)
		return
	}

	// Collect ALL messages (walk pages).
	//
	// 收所有 message(分页走完)。
	var all []*chatdomain.Message
	cursor := ""
	for {
		page, next, err := h.svc.ListMessages(r.Context(), id, chatdomain.ListFilter{
			Cursor: cursor,
			Limit:  200,
		})
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		cursor = next
	}

	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="conversation-%s.json"`, id))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conversationId": id,
			"exportedAt":     time.Now().UTC().Format(time.RFC3339),
			"messages":       all,
		})
		return
	}

	// Markdown rendering — human readable.
	//
	// markdown 渲染——人类可读。
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Conversation %s\n\n", id)
	fmt.Fprintf(&sb, "_Exported at %s · %d messages_\n\n---\n\n",
		time.Now().UTC().Format(time.RFC3339), len(all))
	for _, m := range all {
		fmt.Fprintf(&sb, "## %s · %s\n\n", strings.ToUpper(m.Role), m.CreatedAt.Format(time.RFC3339))
		if m.Status != chatdomain.StatusCompleted {
			fmt.Fprintf(&sb, "_status: %s", m.Status)
			if m.StopReason != "" {
				fmt.Fprintf(&sb, " · stopReason: %s", m.StopReason)
			}
			if m.ErrorCode != "" {
				fmt.Fprintf(&sb, " · error: %s — %s", m.ErrorCode, m.ErrorMessage)
			}
			sb.WriteString("_\n\n")
		}
		for _, b := range m.Blocks {
			switch b.Type {
			case eventlogdomain.BlockTypeText:
				sb.WriteString(b.Content)
				sb.WriteString("\n\n")
			case eventlogdomain.BlockTypeReasoning:
				fmt.Fprintf(&sb, "> 💭 _reasoning_\n>\n> %s\n\n",
					strings.ReplaceAll(b.Content, "\n", "\n> "))
			case eventlogdomain.BlockTypeToolCall:
				fmt.Fprintf(&sb, "**🔧 tool_call:** `%s` (id=%s)\n\n```json\n%s\n```\n\n",
					attrsString(b.Attrs, "name"), b.ID, b.Content)
			case eventlogdomain.BlockTypeToolResult:
				fmt.Fprintf(&sb, "**↩ tool_result:** _parent %s_\n\n```\n%s\n```\n\n",
					b.ParentBlockID, b.Content)
			case eventlogdomain.BlockTypeProgress:
				fmt.Fprintf(&sb, "_progress:_ %s\n\n", b.Content)
			case eventlogdomain.BlockTypeMessage:
				fmt.Fprintf(&sb, "_sub-message %s:_\n%s\n\n", b.ID, b.Content)
			default:
				fmt.Fprintf(&sb, "_%s block_\n\n```\n%s\n```\n\n", b.Type, b.Content)
			}
		}
		if m.InputTokens > 0 || m.OutputTokens > 0 {
			fmt.Fprintf(&sb, "_tokens: in=%d out=%d_\n\n", m.InputTokens, m.OutputTokens)
		}
		sb.WriteString("---\n\n")
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="conversation-%s.md"`, id))
	_, _ = w.Write([]byte(sb.String()))
}

// LLMTrace returns per-assistant-message LLM call metadata (§4.6) derived
// from messages — no separate persistence. Model / tokens / stopReason /
// elapsed timestamps are already on each Message row.
//
// LLMTrace 从 messages 表派生每个 assistant message 的 LLM 调用元数据(§4.6),
// 无新持久化。模型/token/stopReason/时间戳已在 Message 行上。
func (h *ChatHandler) LLMTrace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var all []*chatdomain.Message
	cursor := ""
	for {
		page, next, err := h.svc.ListMessages(r.Context(), id, chatdomain.ListFilter{
			Cursor: cursor,
			Limit:  200,
		})
		if err != nil {
			responsehttpapi.FromDomainError(w, h.log, err)
			return
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		cursor = next
	}

	type trace struct {
		MessageID    string    `json:"messageId"`
		Role         string    `json:"role"`
		Provider     string    `json:"provider,omitempty"`
		ModelID      string    `json:"modelId,omitempty"`
		InputTokens  int       `json:"inputTokens"`
		OutputTokens int       `json:"outputTokens"`
		Status       string    `json:"status"`
		StopReason   string    `json:"stopReason,omitempty"`
		ErrorCode    string    `json:"errorCode,omitempty"`
		ErrorMessage string    `json:"errorMessage,omitempty"`
		CreatedAt    time.Time `json:"createdAt"`
		UpdatedAt    time.Time `json:"updatedAt,omitempty"`
		ElapsedMs    int64     `json:"elapsedMs,omitempty"`
	}
	out := make([]trace, 0)
	totalIn, totalOut := 0, 0
	for _, m := range all {
		if m.Role != "assistant" {
			continue
		}
		t := trace{
			MessageID:    m.ID,
			Role:         m.Role,
			Provider:     m.Provider,
			ModelID:      m.ModelID,
			InputTokens:  m.InputTokens,
			OutputTokens: m.OutputTokens,
			Status:       m.Status,
			StopReason:   m.StopReason,
			ErrorCode:    m.ErrorCode,
			ErrorMessage: m.ErrorMessage,
			CreatedAt:    m.CreatedAt,
		}
		if !m.UpdatedAt.IsZero() {
			t.UpdatedAt = m.UpdatedAt
			t.ElapsedMs = m.UpdatedAt.Sub(m.CreatedAt).Milliseconds()
		}
		out = append(out, t)
		totalIn += m.InputTokens
		totalOut += m.OutputTokens
	}
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"conversationId": id,
		"calls":          out,
		"totals": map[string]int{
			"inputTokens":  totalIn,
			"outputTokens": totalOut,
			"calls":        len(out),
		},
	})
}

func attrsString(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	v, _ := attrs[key].(string)
	return v
}
