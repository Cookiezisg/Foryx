package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

const defaultTopK = 3

// ErrEmptyQuery: query missing or whitespace.
//
// ErrEmptyQuery：query 缺失或全空白。
var ErrEmptyQuery = errors.New("query is required and must be non-empty")

const searchSkillsDescription = `Search the user's installed skills (procedural workflows + allowed-tools bundles) for ones relevant to a task. Returns the top K candidates, each with name, description, and an isFork flag indicating whether activation will spawn an isolated subagent or run inline. Pair with activate_skill once you have a candidate.`

var searchSkillsSchema = json.RawMessage(`{
	"type": "object",
	"required": ["query"],
	"properties": {
		"query": {
			"type": "string",
			"description": "Natural-language description of the task or workflow you need (e.g. 'review a pull request', 'deploy to staging', 'clean up CSV')."
		},
		"top_k": {
			"type": "integer",
			"minimum": 1,
			"maximum": 10,
			"description": "How many candidate skills to return. Default 3; max 10."
		}
	}
}`)

// SearchSkills implements the search_skills system tool.
//
// SearchSkills 是 search_skills 系统工具的实现。
type SearchSkills struct {
	svc *skillapp.Service
}

// Identity --------------------------------------------------------------------

func (t *SearchSkills) Name() string                { return "search_skills" }
func (t *SearchSkills) Description() string         { return searchSkillsDescription }
func (t *SearchSkills) Parameters() json.RawMessage { return searchSkillsSchema }

// Static metadata -------------------------------------------------------------

func (t *SearchSkills) IsReadOnly() bool        { return true }
func (t *SearchSkills) NeedsReadFirst() bool    { return false }
func (t *SearchSkills) RequiresWorkspace() bool { return false }


func (t *SearchSkills) ValidateInput(args json.RawMessage) error {
	var a struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("search_skills.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return ErrEmptyQuery
	}
	return nil
}

func (t *SearchSkills) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}


// searchResult is the per-skill row in the JSON response; body is not included (L2 progressive disclosure).
//
// searchResult 是响应里每个 skill 的一行；body 不含（L2 progressive disclosure）。
type searchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsFork      bool   `json:"isFork"`
	Arguments   []string `json:"arguments,omitempty"`
}

// Execute calls Service.Search and returns the result; failures map to LLM-friendly strings.
//
// Execute 调 Service.Search 返结果；失败映射为 LLM 友好字符串。
func (t *SearchSkills) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_skills.Execute: parse args: %w", err)
	}
	topK := args.TopK
	if topK <= 0 {
		topK = defaultTopK
	}

	skills, err := t.svc.Search(ctx, args.Query, topK)
	if err != nil {
		// LLM-resolution failure is the typical case (no chat model
		// configured). err.Error() is sanitized at the framework
		// boundary; pass through verbatim.
		// LLM 解析失败是典型场景（未配 chat model）。framework boundary
		// 已清洗 err.Error()；原样透传。
		return fmt.Sprintf("Search failed: %s.", err.Error()), nil
	}

	if len(skills) == 0 {
		return "No skills installed.", nil
	}

	out := make([]searchResult, 0, len(skills))
	for _, sk := range skills {
		out = append(out, searchResult{
			Name:        sk.Name,
			Description: sk.Description,
			IsFork:      sk.Frontmatter.Context == "fork",
			Arguments:   sk.Frontmatter.Arguments,
		})
	}
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("search_skills.Execute: marshal result: %w", err)
	}
	return string(body), nil
}


var _ toolapp.Tool = (*SearchSkills)(nil)
