// Package envfix runs the trinity LLM tools' internal env-fix retry loop after a sandbox install fails.
//
// Package envfix 跑 trinity LLM 工具的 env-fix 重试循环（沙箱装失败后用主 LLM 改 deps 重试）。
package envfix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
)

// DefaultMaxAttempts caps env install attempts (initial + LLM-suggested retries).
//
// DefaultMaxAttempts 装环境尝试次数上限（初次 + LLM 修建议）。
const DefaultMaxAttempts = 3

// Attempt records the outcome of one install attempt (EnvStatus = "ready" | "failed").
//
// Attempt 记一次装环境的结果（EnvStatus = "ready" | "failed"）。
type Attempt struct {
	Number    int      `json:"attempt"`
	Deps      []string `json:"deps"`
	EnvStatus string   `json:"envStatus"`
	EnvError  string   `json:"envError,omitempty"`
}

// LoopHooks are tool-supplied UI side-effect callbacks; nil hooks are no-ops.
//
// LoopHooks 工具侧 UI 副作用回调，nil 视为 no-op。
type LoopHooks struct {
	OnAttemptResult func(ctx context.Context, a Attempt)
	OnFixing        func(ctx context.Context, attemptNum int)
}

// Options configures one RunLoop call; Bundle and ApplyDeps are required.
//
// Options 配置一次 RunLoop；Bundle 与 ApplyDeps 必填。
type Options struct {
	Bundle         *llmclientpkg.Bundle
	InitialAttempt Attempt
	MaxAttempts    int
	ApplyDeps      func(ctx context.Context, deps []string) (envStatus, envError string, err error)
	Hooks          LoopHooks
}

// Result is the terminal state after RunLoop; FatalErr non-nil iff ApplyDeps fataled.
//
// Result RunLoop 终态；FatalErr 非 nil 表示 ApplyDeps 致命错。
type Result struct {
	FinalEnvStatus string    `json:"envStatus"`
	FinalEnvError  string    `json:"envError,omitempty"`
	AttemptsUsed   int       `json:"attemptsUsed"`
	History        []Attempt `json:"attemptHistory"`
	FatalErr       error     `json:"-"`
}

var ErrNoBundle = errors.New("envfix: nil LLM bundle")

// RunLoop drives the env-fix retry loop after the caller's initial install.
//
// RunLoop 在调用方初次装环境之后驱动 env-fix 重试循环。
func RunLoop(ctx context.Context, opts Options) Result {
	max := opts.MaxAttempts
	if max <= 0 {
		max = DefaultMaxAttempts
	}
	history := []Attempt{opts.InitialAttempt}

	if opts.Hooks.OnAttemptResult != nil {
		opts.Hooks.OnAttemptResult(ctx, opts.InitialAttempt)
	}

	if opts.InitialAttempt.EnvStatus == "ready" {
		return Result{
			FinalEnvStatus: "ready",
			AttemptsUsed:   1,
			History:        history,
		}
	}

	if opts.Bundle == nil {
		return Result{
			FinalEnvStatus: "failed",
			FinalEnvError:  ErrNoBundle.Error(),
			AttemptsUsed:   1,
			History:        history,
			FatalErr:       ErrNoBundle,
		}
	}

	currentDeps := append([]string(nil), opts.InitialAttempt.Deps...)
	currentErr := opts.InitialAttempt.EnvError

	for attempt := 2; attempt <= max; attempt++ {
		if opts.Hooks.OnFixing != nil {
			opts.Hooks.OnFixing(ctx, attempt)
		}

		newDeps, llmErr := suggestDeps(ctx, opts.Bundle, currentDeps, currentErr, history)
		if llmErr != nil {
			return Result{
				FinalEnvStatus: "failed",
				FinalEnvError:  fmt.Sprintf("env-fix LLM call failed: %v", llmErr),
				AttemptsUsed:   attempt - 1,
				History:        history,
			}
		}

		status, errMsg, applyErr := opts.ApplyDeps(ctx, newDeps)
		if applyErr != nil {
			return Result{
				FinalEnvStatus: "failed",
				FinalEnvError:  applyErr.Error(),
				AttemptsUsed:   attempt,
				History:        history,
				FatalErr:       applyErr,
			}
		}

		a := Attempt{
			Number:    attempt,
			Deps:      newDeps,
			EnvStatus: status,
			EnvError:  errMsg,
		}
		history = append(history, a)
		if opts.Hooks.OnAttemptResult != nil {
			opts.Hooks.OnAttemptResult(ctx, a)
		}

		if status == "ready" {
			return Result{
				FinalEnvStatus: "ready",
				AttemptsUsed:   attempt,
				History:        history,
			}
		}
		currentDeps = newDeps
		currentErr = errMsg
	}

	return Result{
		FinalEnvStatus: "failed",
		FinalEnvError:  currentErr,
		AttemptsUsed:   max,
		History:        history,
	}
}

func suggestDeps(
	ctx context.Context,
	bundle *llmclientpkg.Bundle,
	currentDeps []string,
	lastErr string,
	history []Attempt,
) ([]string, error) {
	prompt := buildPrompt(currentDeps, lastErr, history)

	resp, err := llminfra.Generate(ctx, bundle.Client, llminfra.Request{
		ModelID:  bundle.ModelID,
		Key:      bundle.Key,
		BaseURL:  bundle.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: prompt}},
	})
	if err != nil {
		return nil, fmt.Errorf("envfix: llm generate: %w", err)
	}

	var out struct {
		Deps []string `json:"deps"`
	}

	// Fast path: prompt asks for "JSON only"; direct unmarshal first.
	// 快路径：prompt 要求 "JSON only"，先直接 unmarshal。
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp)), &out); err == nil {
		return out.Deps, nil
	}

	jsonStr, ok := llmparsepkg.ExtractJSON(resp)
	if !ok {
		return nil, fmt.Errorf("envfix: no JSON in LLM response: %q", resp)
	}
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		return nil, fmt.Errorf("envfix: parse deps JSON: %w", err)
	}
	return out.Deps, nil
}

func buildPrompt(currentDeps []string, lastErr string, history []Attempt) string {
	var sb strings.Builder
	sb.WriteString("You are fixing a Python venv install that failed. Suggest a revised dependency list.\n\n")
	sb.WriteString("Current dependencies (PEP 508 specifiers):\n")
	if len(currentDeps) == 0 {
		sb.WriteString("  (empty)\n")
	} else {
		for _, d := range currentDeps {
			fmt.Fprintf(&sb, "  - %s\n", d)
		}
	}
	sb.WriteString("\nLast install error (uv/pip stderr):\n")
	if lastErr == "" {
		sb.WriteString("  (no stderr captured)\n")
	} else {
		fmt.Fprintf(&sb, "%s\n", strings.TrimSpace(lastErr))
	}

	if len(history) > 1 {
		sb.WriteString("\nPrior attempts:\n")
		for _, a := range history {
			fmt.Fprintf(&sb, "  attempt %d: deps=%v status=%s err=%q\n",
				a.Number, a.Deps, a.EnvStatus, truncate(a.EnvError, 200))
		}
	}

	sb.WriteString(`
Rules:
- Only fix the dependency list (typos, version conflicts, missing constraints).
- Do NOT add new packages unrelated to the current list.
- Do NOT modify any Python code (code is not your concern here).
- Keep the same packages where possible; just adjust versions or fix names.
- If you cannot determine a fix from the info above, return the deps unchanged.

Return JSON only, no commentary:
{"deps": ["pandas>=2.0", "numpy"]}
`)
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
