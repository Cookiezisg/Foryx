// history.go — LLM history construction from DB messages. Used at the start
// of each user turn to seed loop.Run. The blocks→LLM-wire converter is
// shared with the in-loop extender via loop.BlocksToAssistantLLM.
//
// history.go — 从 DB 消息构建 LLM 历史，每个用户回合开头调一次给 loop.Run
// 喂种子。blocks→LLM-wire 转换器与循环内扩展器共享 loop.BlocksToAssistantLLM。
package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	chatinfra "github.com/sunweilin/forgify/backend/internal/infra/chat"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// maxHistoryMessages is the maximum number of past messages loaded per LLM call.
// Older messages beyond this limit are silently dropped.
//
// maxHistoryMessages 是每次 LLM 调用加载的历史消息上限，超出的旧消息静默丢弃。
const maxHistoryMessages = 200

// buildHistory loads completed messages from the DB and returns them as LLM
// wire messages. currentUserMsgID is excluded from the main scan and appended
// last, ensuring the LLM always sees the triggering message at the end
// regardless of created_at ordering (prevents the fast-send race condition).
//
// buildHistory 从 DB 加载已完成消息并转为 LLM 协议格式。
// currentUserMsgID 排除在主扫描外并追加到末尾，保证 LLM 以该消息作为待回复轮次，
// 不受快速连发时 created_at 竞态的影响。
func (s *Service) buildHistory(ctx context.Context, convID, currentUserMsgID string) ([]llminfra.LLMMessage, error) {
	rows, _, err := s.repo.ListMessagesByConversation(ctx, convID, chatdomain.ListFilter{Limit: maxHistoryMessages})
	if err != nil {
		return nil, fmt.Errorf("chat.Service.buildHistory: %w", err)
	}

	var out []llminfra.LLMMessage
	var currentUserMsg *chatdomain.Message

	for _, m := range rows {
		if m.Status == chatdomain.StatusStreaming || m.Status == chatdomain.StatusPending {
			continue
		}
		if m.ID == currentUserMsgID {
			currentUserMsg = m
			continue
		}
		msgs, err := s.blocksToLLM(ctx, m)
		if err != nil {
			return nil, fmt.Errorf("chat.Service.buildHistory: message %q: %w", m.ID, err)
		}
		out = append(out, msgs...)
	}

	if currentUserMsg != nil {
		msg, err := s.buildUserLLMMessage(ctx, currentUserMsg)
		if err != nil {
			return nil, fmt.Errorf("chat.Service.buildHistory: current user msg %q: %w", currentUserMsgID, err)
		}
		out = append(out, msg)
	}
	return out, nil
}

// blocksToLLM converts one persisted Message to LLM wire messages.
// Delegates assistant blocks to loop.BlocksToAssistantLLM (single source of
// truth shared with the in-loop extender).
//
// blocksToLLM 把一条已持久化的 Message 转为 LLM 协议消息。assistant blocks
// 委托给 loop.BlocksToAssistantLLM（与循环内扩展器共享的事实源）。
func (s *Service) blocksToLLM(ctx context.Context, m *chatdomain.Message) ([]llminfra.LLMMessage, error) {
	switch m.Role {
	case chatdomain.RoleUser:
		msg, err := s.buildUserLLMMessage(ctx, m)
		if err != nil {
			return nil, fmt.Errorf("chat.Service.blocksToLLM: %w", err)
		}
		return []llminfra.LLMMessage{msg}, nil
	case chatdomain.RoleAssistant:
		return loopapp.BlocksToAssistantLLM(s.log, m.Blocks)
	}
	// Unknown role — log + drop. Returning nil keeps the caller's loop
	// going; the message just doesn't land in LLM context. Without the
	// log this drop is invisible (DB rows where Role is a future
	// reserved value, schema drift, etc. would silently shrink history).
	//
	// 未知 role——log + drop。返 nil 让调用方循环继续，消息在 LLM
	// context 里消失。没有 log 时这种丢弃无可追（DB row Role 是未来保留
	// 值 / schema 漂移会让历史悄悄变短）。
	s.log.Warn("chat.Service.blocksToLLM: unknown role dropped from history",
		zap.String("message_id", m.ID), zap.String("role", m.Role))
	return nil, nil
}

// buildUserLLMMessage converts a user message's blocks + attachments
// (from Message.Attrs) to a single LLM message. Text blocks become
// inline content; attachments (now stored in Attrs JSON, not blocks)
// resolve to ContentParts (image → base64, document → extracted text).
// Attachment failures are soft: logged and skipped.
//
// buildUserLLMMessage 把 user 消息的 blocks + Attachments（来自
// Message.Attrs）转为单条 LLM 消息。text block 变为内联 content；
// 附件（现存 Attrs JSON，非 block）解析为 ContentPart（图片 → base64，
// 文档 → 提取文本）。附件失败属于软失败：记录后跳过。
func (s *Service) buildUserLLMMessage(ctx context.Context, m *chatdomain.Message) (llminfra.LLMMessage, error) {
	msg := llminfra.LLMMessage{Role: llminfra.RoleUser}
	var parts []llminfra.ContentPart

	// Text from blocks (after event-log unification, user text lives in
	// blocks of type=text with raw Content).
	//
	// Text 从 blocks 取（事件日志统一后，用户文本在 type=text block 的
	// 裸 Content）。
	for _, b := range m.Blocks {
		if b.Type == eventlogdomain.BlockTypeText && b.Content != "" {
			parts = append(parts, llminfra.ContentPart{Type: "text", Text: b.Content})
		}
	}

	// Attachments from Message.Attrs (2026-05 changed from string to
	// map[string]any via GORM serializer:json). Read attachments slice
	// directly via map lookup + type-assert helper roundtrip.
	// 2026-05 Attrs 变 map[string]any (GORM serializer);先按 map 取键 +
	// json 中转一次转 typed slice。
	if len(m.Attrs) > 0 {
		if rawAttachments, ok := m.Attrs["attachments"]; ok {
			// Round-trip via JSON to convert []any → []AttachmentRef cleanly.
			// 走一遍 JSON 把 []any 转 []AttachmentRef。
			raw, err := json.Marshal(rawAttachments)
			if err == nil {
				var refs []chatdomain.AttachmentRef
				if err := json.Unmarshal(raw, &refs); err != nil {
					s.log.Warn("chat.Service.buildUserLLMMessage: malformed Message.Attrs attachments; dropped from LLM context",
						zap.String("message_id", m.ID), zap.Error(err))
				} else {
					for _, ref := range refs {
						part, err := s.attachmentToPart(ctx, ref)
						if err != nil {
							s.log.Warn("skipping attachment in LLM history", zap.Error(err))
							continue
						}
						parts = append(parts, *part)
					}
				}
			}
		}
	}

	if len(parts) == 1 && parts[0].Type == "text" {
		msg.Content = parts[0].Text
		return msg, nil
	}
	msg.Parts = parts
	return msg, nil
}

// attachmentToPart resolves an AttachmentRef to a ContentPart.
// Images → image_url (base64 data URL); documents → inlined text part.
//
// attachmentToPart 把 AttachmentRef 解析为 ContentPart。
// 图片 → image_url（base64 data URL）；文档 → 内联文本。
func (s *Service) attachmentToPart(ctx context.Context, ref chatdomain.AttachmentRef) (*llminfra.ContentPart, error) {
	att, err := s.repo.GetAttachment(ctx, ref.AttachmentID)
	if err != nil {
		return nil, fmt.Errorf("chat.Service.attachmentToPart: get attachment %q: %w", ref.AttachmentID, err)
	}

	if chatinfra.IsImage(att.MimeType) {
		data, err := readAndEncode(att.StoragePath)
		if err != nil {
			return nil, fmt.Errorf("chat.Service.attachmentToPart: encode image %q: %w", att.ID, err)
		}
		return &llminfra.ContentPart{
			Type:     "image_url",
			ImageURL: "data:" + att.MimeType + ";base64," + data,
		}, nil
	}

	text, err := chatinfra.Extract(att.StoragePath, att.MimeType)
	if err != nil {
		return nil, fmt.Errorf("chat.Service.attachmentToPart: extract %q: %w", att.ID, err)
	}
	return &llminfra.ContentPart{
		Type: "text",
		Text: fmt.Sprintf("\n\n[附件: %s]\n%s", att.FileName, text),
	}, nil
}
