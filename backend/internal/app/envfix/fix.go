package envfix

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	modelclientapp "github.com/sunweilin/anselm/backend/internal/app/modelclient"
	modeldomain "github.com/sunweilin/anselm/backend/internal/domain/model"
	llminfra "github.com/sunweilin/anselm/backend/internal/infra/llm"
	jsonrepairpkg "github.com/sunweilin/anselm/backend/internal/pkg/jsonrepair"
)

// suggestDeps asks the utility model for a revised dependency list given the failing
// deps + captured stderr. Resolution goes through modelclient — the one shared chain,
// so the wire model id is never confused with the base URL.
//
// suggestDeps 给定失败的 deps + 捕获的 stderr，让 utility 模型给修正依赖列表。解析走
// modelclient——唯一共享解析链，故 wire model id 不会被误设成 base URL。
func (p *Provisioner) suggestDeps(ctx context.Context, currentDeps []string, lastErr string, history []Attempt) ([]string, error) {
	client, req, _, err := modelclientapp.Resolve(ctx, modeldomain.ScenarioUtility, nil, p.picker, p.keys, p.factory)
	if err != nil {
		return nil, fmt.Errorf("envfix.suggestDeps: resolve utility model: %w", err)
	}
	req.Messages = []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: buildFixPrompt(currentDeps, lastErr, history)}}
	out, err := llminfra.Generate(ctx, client, req)
	if err != nil {
		return nil, fmt.Errorf("envfix.suggestDeps: llm generate: %w", err)
	}
	return parseDeps(out)
}

// parseDeps extracts {"deps":[...]} from the model reply, repairing malformed JSON
// (fences / trailing commas) via jsonrepair before unmarshal.
//
// parseDeps 从模型回复抽 {"deps":[...]}，unmarshal 前用 jsonrepair 修畸形 JSON
// （围栏 / 尾逗号）。
func parseDeps(resp string) ([]string, error) {
	var out struct {
		Deps []string `json:"deps"`
	}
	repaired := jsonrepairpkg.Repair(strings.TrimSpace(resp))
	if err := json.Unmarshal([]byte(repaired), &out); err != nil {
		return nil, fmt.Errorf("envfix.parseDeps: no parseable deps JSON in reply: %w", err)
	}
	return out.Deps, nil
}

// buildFixPrompt constrains the model to ONLY adjust dependencies (versions / names /
// constraints) — never code — and to return JSON only.
//
// buildFixPrompt 把模型约束为只调依赖（版本 / 名字 / 约束）、绝不碰代码、只返 JSON。
func buildFixPrompt(currentDeps []string, lastErr string, history []Attempt) string {
	var sb strings.Builder
	sb.WriteString("A Python/Node package install failed. Suggest a revised dependency list.\n\n")

	sb.WriteString("Current dependencies:\n")
	if len(currentDeps) == 0 {
		sb.WriteString("  (empty)\n")
	} else {
		for _, d := range currentDeps {
			fmt.Fprintf(&sb, "  - %s\n", d)
		}
	}

	sb.WriteString("\nInstall error (package-manager stderr):\n")
	if strings.TrimSpace(lastErr) == "" {
		sb.WriteString("  (no stderr captured)\n")
	} else {
		fmt.Fprintf(&sb, "%s\n", strings.TrimSpace(lastErr))
	}

	if len(history) > 1 {
		sb.WriteString("\nPrior attempts:\n")
		for _, a := range history {
			fmt.Fprintf(&sb, "  attempt %d: deps=%v ok=%v err=%q\n",
				a.Number, a.Deps, a.OK, truncate(a.Error, 200))
		}
	}

	sb.WriteString(`
Rules:
- Only fix the dependency list (typos, version conflicts, missing/over-tight constraints).
- Do NOT add packages unrelated to the current list.
- Do NOT modify any code — code is not your concern here.
- Keep the same packages where possible; adjust versions or fix names.
- NEVER drop/remove a declared package just to make the error disappear: renaming a typo (e.g. "beautifulsoup" -> "beautifulsoup4") or loosening a version is a fix; removing a required package is NOT — the code still imports it, so the env would build but fail at runtime. Return one package per originally-declared package.
- If you cannot determine a fix, return the deps unchanged.

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
