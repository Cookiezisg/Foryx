package chat

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// LoadHistory composes the LLM message history the loop generates against: the conversation's
// compaction summary (if any) first, then every persisted turn oldest-first. User turns render
// to text (+ multimodal attachment parts gated by the model's capabilities); assistant turns
// project their block tree via loopapp.BlocksToAssistantLLM (hot/warm/cold). The in-flight
// assistant turn (this generation, opened with no blocks yet) is skipped.
//
// LoadHistory 组装 loop 据以生成的 LLM 消息历史：先对话压缩摘要（若有），再每个持久回合最旧在前。
// user 回合渲成文本（+ 按模型能力门控的多模态附件部件）；assistant 回合经 loopapp.BlocksToAssistantLLM
// 投影其 block 树（hot/warm/cold）。在飞的 assistant 回合（本次生成、开时暂无 block）被跳过。
func (h *chatHost) LoadHistory(ctx context.Context) ([]llminfra.LLMMessage, error) {
	thread, err := h.svc.messages.LoadThread(ctx, h.conversationID)
	if err != nil {
		return nil, fmt.Errorf("chatapp.LoadHistory: %w", err)
	}

	var out []llminfra.LLMMessage
	if h.summary != "" {
		// The compacted older history rides as a leading user-role context block (the original
		// blocks are archived; their content folded into conversation.summary).
		//
		// 被压缩的旧历史作为一条前置 user 角色上下文块（原 block 已 archived，内容并入 conversation.summary）。
		out = append(out, llminfra.LLMMessage{
			Role:    llminfra.RoleUser,
			Content: "<conversation_summary>\n" + h.summary + "\n</conversation_summary>",
		})
	}

	for _, m := range thread {
		// A subagent's sub-messages live in this conversation (persisted for the reload tree)
		// but are NOT part of the parent's LLM history — the parent only sees the spawning
		// tool_call + its tool_result (the subagent's final answer). Exclude them here.
		//
		// subagent 的 sub-message 落在本对话（为 reload 树持久化），但**不是**父的 LLM 历史——父只见
		// 派它的 tool_call + 其 tool_result（subagent 最终答案）。此处排除。
		if m.SubagentID != "" {
			continue
		}
		switch m.Role {
		case messagesdomain.RoleUser:
			out = append(out, h.userMessage(ctx, m))
		case messagesdomain.RoleAssistant:
			if m.ID == h.assistantMsgID {
				continue // the turn being generated right now — no blocks to replay yet
			}
			out = append(out, loopapp.BlocksToAssistantLLM(m.Blocks)...)
		}
	}
	return out, nil
}

// userMessage renders one persisted user turn to an LLM message: plain text when there are no
// attachments, otherwise a text part followed by the attachment renderer's multimodal parts
// (image_url / inline file / extracted text, gated by the model's capabilities). A render
// failure degrades to text-only — a turn never fails to load over a bad attachment.
//
// userMessage 把一个持久 user 回合渲成 LLM 消息：无附件时纯文本，否则一个 text 部件后接附件渲染器
// 的多模态部件（image_url / 内联 file / 抽取文本，按模型能力门控）。渲染失败降级为纯文本——回合
// 绝不因坏附件而加载失败。
func (h *chatHost) userMessage(ctx context.Context, m *messagesdomain.Message) llminfra.LLMMessage {
	text := userText(m)
	// Prepend the frozen @-mention snapshots so the referenced entities' content is inline.
	// 前置冻结的 @ mention 快照，使被引用实体内容内联。
	if mentions := renderMentions(m); mentions != "" {
		if text != "" {
			text = mentions + "\n\n" + text
		} else {
			text = mentions
		}
	}
	ids := attachmentIDsOf(m)
	if len(ids) == 0 || h.svc.deps.Attachments == nil {
		return llminfra.LLMMessage{Role: llminfra.RoleUser, Content: text}
	}

	parts, err := h.svc.deps.Attachments.ToContentParts(ctx, ids, h.caps)
	if err != nil {
		h.svc.log.Warn("chatapp.userMessage: attachment render failed; text only",
			zap.String("messageId", m.ID), zap.Error(err))
		return llminfra.LLMMessage{Role: llminfra.RoleUser, Content: text}
	}

	msg := llminfra.LLMMessage{Role: llminfra.RoleUser}
	if text != "" {
		msg.Parts = append(msg.Parts, llminfra.ContentPart{Type: llminfra.PartText, Text: text})
	}
	msg.Parts = append(msg.Parts, parts...)
	return msg
}

// userText concatenates a turn's text blocks (newline-joined). User turns carry only text blocks;
// reasoning / tool_* belong to assistant turns.
//
// userText 拼接一个回合的 text block（换行连接）。user 回合只有 text block；reasoning / tool_* 属
// assistant 回合。
func userText(m *messagesdomain.Message) string {
	var b strings.Builder
	for _, blk := range m.Blocks {
		if blk.Type == messagesdomain.BlockTypeText {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(blk.Content)
		}
	}
	return b.String()
}

// attachmentIDsOf reads the attachment ids Send snapshotted into Message.Attrs. A JSON round-trip
// (store persists Attrs as JSON) turns the []string into []any, so both forms are handled.
//
// attachmentIDsOf 读 Send 快照进 Message.Attrs 的附件 id。JSON 往返（store 把 Attrs 存为 JSON）把
// []string 变成 []any，故两种形态都处理。
func attachmentIDsOf(m *messagesdomain.Message) []string {
	raw, ok := m.Attrs[attrAttachments]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
