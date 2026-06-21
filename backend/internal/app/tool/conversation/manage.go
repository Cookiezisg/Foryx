package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	conversationapp "github.com/sunweilin/anselm/backend/internal/app/conversation"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	conversationdomain "github.com/sunweilin/anselm/backend/internal/domain/conversation"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

var _ toolapp.Tool = (*ManageConversation)(nil)

// Manager is the narrow slice of the conversation Service the manage tool needs — a DIP port
// (mirrors the trace tool's Reader) so the test can inject a fake without a real repo.
// *conversationapp.Service satisfies it; archive/pin already exist there (no new app method).
//
// Manager 是 manage 工具所需 conversation Service 的窄切面——DIP 端口（镜像 trace 工具的 Reader），
// 使测试无需真 repo 即可注入 fake。*conversationapp.Service 满足之；archive/pin 那里已存在（无新 app 方法）。
type Manager interface {
	Update(ctx context.Context, id string, in conversationapp.UpdateInput) (*conversationdomain.Conversation, error)
}

// ManageConversation is manage_conversation: archive/pin THIS conversation from the chat seat.
// The capability already exists over HTTP PATCH; the LLM just had no tool for it, so when asked
// to "archive/compact this conversation" the agent fabricated a UI button. Two truths this fixes:
// (1) the agent can now actually archive/pin the current thread; (2) the description states that
// compaction is AUTOMATIC (no manual action / no button) so the agent stops inventing one.
//
// ManageConversation 即 manage_conversation：从 chat 席归档/置顶本对话。该能力 HTTP PATCH 已有、
// LLM 没工具够着，故被问「归档/压缩本对话」时 agent 编造 UI 按钮。修两点：①agent 现真能归档/置顶
// 当前对话；②描述声明 compaction 是自动的（无手动动作/无按钮），使 agent 不再臆造。
type ManageConversation struct{ mgr Manager }

func (t *ManageConversation) Name() string { return "manage_conversation" }

func (t *ManageConversation) Description() string {
	return "Archive, pin, or rename THIS current conversation (archive | unarchive | pin | unpin | rename — rename needs a `title`). " +
		"IMPORTANT: archiving the thread you are CURRENTLY chatting in is effectively moot — sending ANY further message to an archived thread AUTOMATICALLY unarchives it; if the user wants the open conversation kept archived, tell them it stays archived only once they stop messaging it (don't silently let the next message undo it). " +
		"Compaction/summarization happens AUTOMATICALLY when the thread nears the model's context window — there is no manual compact/summarize action and no UI button for it; never tell the user to click one, and never invent a UI gesture for rename either (use this tool's rename action)."
}

func (t *ManageConversation) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["action"],
		"properties": {
			"action": {"type": "string", "enum": ["archive", "unarchive", "pin", "unpin", "rename"], "description": "What to do to this conversation."},
			"title": {"type": "string", "description": "New title — REQUIRED when action is 'rename', ignored otherwise."}
		}
	}`)
}

type manageArgs struct {
	Action string `json:"action"`
	Title  string `json:"title"`
}

// ValidateInput rejects malformed JSON and any action outside the enum.
//
// ValidateInput 拒绝坏 JSON 与枚举外的 action。
func (t *ManageConversation) ValidateInput(args json.RawMessage) error {
	var a manageArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("manage_conversation: bad args: %w", err)
	}
	switch a.Action {
	case "archive", "unarchive", "pin", "unpin":
		return nil
	case "rename":
		if strings.TrimSpace(a.Title) == "" {
			return fmt.Errorf("manage_conversation: rename requires a non-empty title")
		}
		return nil
	default:
		return fmt.Errorf("manage_conversation: action must be one of archive/unarchive/pin/unpin/rename, got %q", a.Action)
	}
}

// Execute maps the action to an archive/pin PATCH on the conversation in ctx. No conversation in
// ctx degrades to a tool-result string (mirrors get_subagent_trace) so the LLM can adjust rather
// than seeing a hard wiring error.
//
// Execute 把 action 映射成对 ctx 内对话的 archive/pin PATCH。ctx 无对话时降级为 tool-result 串
// （镜像 get_subagent_trace），使 LLM 可调整而非见硬接线错。
func (t *ManageConversation) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a manageArgs
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("manage_conversation: bad args: %w", err)
	}

	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok {
		return "manage_conversation is only available inside a conversation (no conversationId in context).", nil
	}

	var in conversationapp.UpdateInput
	switch a.Action {
	case "archive":
		v := true
		in.Archived = &v
	case "unarchive":
		v := false
		in.Archived = &v
	case "pin":
		v := true
		in.Pinned = &v
	case "unpin":
		v := false
		in.Pinned = &v
	case "rename":
		// UpdateInput.Title already exists + Service.Update applies it (the HTTP PATCH renames fine);
		// the tool was just asymmetric, so the agent fabricated a UI gesture for rename (F107, the F38 class).
		// Trim + reject whitespace-only at the write point too (not just ValidateInput) so a "   " title
		// can't slip a blank rename through, and a padded title is stored clean.
		// 写入点也 trim + 拒纯空白（非仅 ValidateInput），使 "   " 无法溜进空改名、带空格的标题存得干净。
		title := strings.TrimSpace(a.Title)
		if title == "" {
			return "", fmt.Errorf("manage_conversation: rename requires a non-empty title")
		}
		in.Title = &title
	default:
		return "", fmt.Errorf("manage_conversation: action must be one of archive/unarchive/pin/unpin/rename, got %q", a.Action)
	}

	c, err := t.mgr.Update(ctx, convID, in)
	if err != nil {
		return "", fmt.Errorf("manage_conversation: %w", err)
	}
	return toolapp.ToJSON(map[string]any{
		"conversationId": convID,
		"action":         a.Action,
		"title":          c.Title,
		"archived":       c.Archived,
		"pinned":         c.Pinned,
	}), nil
}
