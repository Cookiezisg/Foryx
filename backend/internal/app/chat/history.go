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

const maxHistoryMessages = 200

// buildHistory loads completed messages and appends currentUserMsgID last to dodge created_at races.
//
// buildHistory 加载已完成消息，currentUserMsgID 追加末尾以规避 created_at 竞态。
func (s *Service) buildHistory(ctx context.Context, convID, currentUserMsgID string) ([]llminfra.LLMMessage, error) {
	rows, _, err := s.repo.ListMessagesByConversation(ctx, convID, chatdomain.ListFilter{Limit: maxHistoryMessages})
	if err != nil {
		return nil, fmt.Errorf("chat.Service.buildHistory: %w", err)
	}

	var out []llminfra.LLMMessage

	// Wrap conv.Summary in <conversation_summary> so LLM treats it as compressed context.
	conv, err := s.convRepo.Get(ctx, convID)
	if err == nil && conv != nil && conv.Summary != "" {
		out = append(out, llminfra.LLMMessage{
			Role: llminfra.RoleAssistant,
			Content: "<conversation_summary>\n" + conv.Summary +
				"\n</conversation_summary>",
		})
	}

	var currentUserMsg *chatdomain.Message

	for _, m := range rows {
		if m.Status == chatdomain.StatusStreaming || m.Status == chatdomain.StatusPending {
			continue
		}
		if m.ID == currentUserMsgID {
			currentUserMsg = m
			continue
		}
		// contextmgr-produced system messages are already represented in conv.Summary above.
		if m.Role == "system" {
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

// blocksToLLM converts a persisted Message to LLM wire messages.
//
// blocksToLLM 把已持久化的 Message 转为 LLM 协议消息。
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
	// Unknown role: log + drop so silent history shrinkage from schema drift is visible.
	s.log.Warn("chat.Service.blocksToLLM: unknown role dropped from history",
		zap.String("message_id", m.ID), zap.String("role", m.Role))
	return nil, nil
}

// buildUserLLMMessage assembles a user LLM message from blocks + Message.Attrs attachments.
//
// buildUserLLMMessage 把 user blocks + Message.Attrs 附件汇成单条 LLM 消息；附件失败软跳过。
func (s *Service) buildUserLLMMessage(ctx context.Context, m *chatdomain.Message) (llminfra.LLMMessage, error) {
	msg := llminfra.LLMMessage{Role: llminfra.RoleUser}
	var parts []llminfra.ContentPart

	for _, b := range m.Blocks {
		if b.Type == eventlogdomain.BlockTypeText && b.Content != "" {
			parts = append(parts, llminfra.ContentPart{Type: "text", Text: b.Content})
		}
	}

	if len(m.Attrs) > 0 {
		if rawAttachments, ok := m.Attrs["attachments"]; ok {
			// Round-trip via JSON to coerce []any → []AttachmentRef.
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

// attachmentToPart turns an AttachmentRef into a ContentPart (image → base64 / doc → text).
//
// attachmentToPart 把 AttachmentRef 转为 ContentPart（图片 → base64，文档 → 内联文本）。
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
