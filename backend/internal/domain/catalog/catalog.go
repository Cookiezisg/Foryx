// Package catalog is the domain layer for the Capability Catalog injected into chat system prompts.
//
// Package catalog 是注入 chat system prompt 的能力清单的 domain 层。
package catalog

import (
	"errors"
	"time"
)

// Catalog is the derived view that gets injected into chat system prompts.
//
// Catalog 是注入 chat system prompt 的派生视图，由 app/catalog.Service 构建。
type Catalog struct {
	Summary     string               `json:"summary"`
	Coverage    map[string][]string  `json:"coverage"`
	Fingerprint string               `json:"fingerprint"`
	GeneratedAt time.Time            `json:"generatedAt"`
	Version     int                  `json:"version"`
	SourcesAt   map[string]time.Time `json:"sourcesAt"`
	GeneratedBy string               `json:"generatedBy"`
}

var (
	// ErrCoverageIncomplete signals the LLM output dropped source items; caller switches to mechanical fallback.
	//
	// ErrCoverageIncomplete：LLM 输出漏 item；调用方切 mechanicalFallback。
	ErrCoverageIncomplete = errors.New("catalog: generator output missing items")

	// ErrGenerationFailed wraps LLM transport / decode failures; same fallback contract.
	//
	// ErrGenerationFailed 包底层 LLM 传输 / 解码失败；fallback 契约同上。
	ErrGenerationFailed = errors.New("catalog: LLM generation failed")

	// ErrAllSourcesFailed is returned when every source errored; mapped to 503 in errmap.
	//
	// ErrAllSourcesFailed 所有 source 报错时返回；errmap 映射 503。
	ErrAllSourcesFailed = errors.New("catalog: all sources failed; previous cache retained")
)

// SystemPromptProvider is the narrow interface chat.runner consumes to fetch the catalog text.
//
// SystemPromptProvider 是 chat.runner 取 catalog 文本的窄接口。
type SystemPromptProvider interface {
	GetForSystemPrompt() string
}
