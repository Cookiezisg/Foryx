package document

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	documentapp "github.com/sunweilin/anselm/backend/internal/app/document"
	searchapp "github.com/sunweilin/anselm/backend/internal/app/search"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	searchdomain "github.com/sunweilin/anselm/backend/internal/domain/search"
)

// docHit is the unified slim shape both search paths render — the content engine fills
// id/name/snippet, the legacy name search fills id/name/path/description; omitempty keeps the
// JSON tight (one structured shape, no prose).
//
// docHit 是两条检索路径共用的 slim 形状——内容引擎填 id/name/snippet，原名字检索填
// id/name/path/description；omitempty 保持 JSON 紧凑（单一结构形状、无散文）。
type docHit struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
}

const searchDocumentsDescription = `Search documents by keyword over name / description / tags. Returns path + description per match so you can pick which to read. Prefer list_documents when you already know the folder.`

const searchDocumentsDefaultLimit = 10

var searchDocumentsSchema = json.RawMessage(`{
	"type": "object",
	"required": ["query"],
	"properties": {
		"query": {"type": "string"},
		"limit": {"type": "integer", "default": 10, "maximum": 50}
	}
}`)

// SearchDocuments implements the search_documents system tool.
//
// SearchDocuments 是 search_documents 系统工具的实现。
type SearchDocuments struct {
	svc     *documentapp.Service
	content *searchapp.Service // nil → legacy substring only. nil → 仅原子串路径。
}

func (t *SearchDocuments) Name() string                { return "search_documents" }
func (t *SearchDocuments) Description() string         { return searchDocumentsDescription }
func (t *SearchDocuments) Parameters() json.RawMessage { return searchDocumentsSchema }

func (t *SearchDocuments) ValidateInput(args json.RawMessage) error {
	var a struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("search_documents: bad args: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return ErrQueryRequired
	}
	if a.Limit < 0 || a.Limit > 50 {
		return fmt.Errorf("search_documents: limit must be 0..50, got %d", a.Limit)
	}
	return nil
}

func (t *SearchDocuments) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("search_documents: %w", err)
	}
	if a.Limit == 0 {
		a.Limit = searchDocumentsDefaultLimit
	}
	// Content engine first: full-text over names AND markdown bodies, heading
	// snippets included; engine errors fall back to the legacy name search.
	// 先走内容引擎：全文覆盖名字**及 markdown 正文**、附标题 snippet；引擎出错回退原名字检索。
	if t.content != nil {
		if page, err := t.content.Search(ctx, &searchdomain.Query{
			Q: a.Query, Types: []searchdomain.EntityType{searchdomain.TypeDocument}, IncludeArchived: true, Limit: a.Limit,
		}); err == nil {
			out := make([]docHit, 0, len(page.Hits))
			for _, h := range page.Hits {
				out = append(out, docHit{ID: h.EntityID, Name: h.Name, Snippet: h.Snippet})
			}
			// Disclose truncation (total + nextCursor/hasMore) so the LLM doesn't read `count` as the
			// full match count (F175-M4, sibling of the entity-search ContentSearch fix; shared helper).
			// 披露截断（total + nextCursor/hasMore），免 LLM 把 `count` 当全量匹配数（F175-M4，与实体搜 ContentSearch 同修；共用 helper）。
			return toolapp.ToJSON(toolapp.SlimPageResult(len(out), page.Total, page.NextCursor, "documents", out)), nil
		}
	}
	rows, err := t.svc.Search(ctx, a.Query, a.Limit)
	if err != nil {
		return "", err
	}
	out := make([]docHit, 0, len(rows))
	for _, d := range rows {
		out = append(out, docHit{ID: d.ID, Name: d.Name, Path: d.Path, Description: d.Description})
	}
	return toolapp.ToJSON(map[string]any{"count": len(out), "documents": out}), nil
}
