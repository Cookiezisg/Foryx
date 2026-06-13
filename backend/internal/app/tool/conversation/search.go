// Package conversation gives the LLM a recall window into past conversations — the one
// content kind the omni search indexes that LLM tools previously could not reach. It
// returns snippets + ids only, never full transcripts: recall is a pointer, not a
// context dump.
//
// Package conversation 给 LLM 一扇回忆历史对话的窗——综搜已索引、但 LLM 工具此前唯一够不
// 着的内容类。只返 snippet + id、绝不返全文：回忆是指针、不是上下文倾倒。
package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	searchapp "github.com/sunweilin/forgify/backend/internal/app/search"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
)

// ConversationTools constructs the conversation tool group (lazy).
//
// ConversationTools 构造 conversation 工具组（懒加载）。
func ConversationTools(engine *searchapp.Service) []toolapp.Tool {
	return []toolapp.Tool{&SearchConversations{engine: engine}}
}

const (
	defaultLimit = 8
	maxLimit     = 20
)

type SearchConversations struct{ engine *searchapp.Service }

func (t *SearchConversations) Name() string { return "search_conversations" }

func (t *SearchConversations) Description() string {
	return "Search past conversation history by content (hybrid lexical + semantic). Use it when the user refers to something discussed earlier (\"the plan we talked about\"). Returns per hit: conversationId, title, a snippet of the matching message and its messageId — snippets only, never full transcripts."
}

func (t *SearchConversations) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["query"],
		"properties": {
			"query": {"type": "string", "description": "What to look for in past conversations."},
			"limit": {"type": "integer", "description": "Max hits (1-20, default 8)."}
		}
	}`)
}

func (t *SearchConversations) ValidateInput(args json.RawMessage) error {
	var a struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("search_conversations: bad args: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return searchdomain.ErrQueryRequired
	}
	return nil
}

func (t *SearchConversations) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_conversations: bad args: %w", err)
	}
	limit := args.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	page, err := t.engine.Search(ctx, &searchdomain.Query{
		Q:               args.Query,
		Types:           []searchdomain.EntityType{searchdomain.TypeConversation},
		IncludeArchived: true,
		Limit:           limit,
	})
	if err != nil {
		return "", fmt.Errorf("search_conversations: %w", err)
	}
	type hit struct {
		ConversationID string `json:"conversationId"`
		Title          string `json:"title,omitempty"`
		Snippet        string `json:"snippet,omitempty"`
		MessageID      string `json:"messageId,omitempty"` // the matching message (search anchor). 命中消息（检索锚点）。
		MatchedChunks  int    `json:"matchedChunks,omitempty"`
	}
	hits := make([]hit, 0, len(page.Hits))
	for _, h := range page.Hits {
		hits = append(hits, hit{
			ConversationID: h.EntityID,
			Title:          h.Name,
			Snippet:        h.Snippet,
			MessageID:      h.Anchor,
			MatchedChunks:  h.MatchedChunks,
		})
	}
	return toolapp.ToJSON(map[string]any{"hits": hits, "total": page.Total}), nil
}
