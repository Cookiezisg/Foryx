package llm

import (
	"encoding/json"
	"strings"
)

// modelSpec is one provider's static knowledge of a model family: capability numbers plus the
// native configurable knobs, keyed by modelID prefix (list order decides precedence — list the
// most specific prefix first). Providers whose /models endpoint returns only ids depend on this;
// rich-endpoint providers use it as fallback when the payload omits numbers.
//
// modelSpec 是某家对一个模型族的静态知识：能力数字 + 原生可调旋钮，按 modelID 前缀匹配（列表
// 顺序定优先——最具体前缀列在前）。贫 /models 家依赖它；富 /models 家在载荷缺数字时拿它兜底。
type modelSpec struct {
	prefix     string
	ctx        int
	out        int
	knobs      []Knob
	vision     bool // model accepts image input natively (via the OpenAI-compat image_url path)
	nativeDocs bool // model accepts an inline document (PDF) natively
}

// matchSpec returns the first spec whose prefix matches modelID (case-insensitive).
//
// matchSpec 返回首个 prefix 命中 modelID 的 spec（大小写不敏感）。
func matchSpec(specs []modelSpec, modelID string) (modelSpec, bool) {
	id := strings.ToLower(strings.TrimSpace(modelID))
	for _, s := range specs {
		if strings.HasPrefix(id, s.prefix) {
			return s, true
		}
	}
	return modelSpec{}, false
}

// enumKnob builds an enum-type Knob descriptor with native key/values/default kept verbatim.
//
// enumKnob 构造 enum 型 Knob 描述符，key/取值/默认全原生原样。
func enumKnob(key, label string, values []string, def string) Knob {
	return Knob{Key: key, Label: label, Type: "enum", Values: values, Default: def}
}

// boolKnob builds a boolean Knob (frontend renders a toggle); def is "true"/"false".
//
// boolKnob 构造布尔 Knob（前端渲染开关）；def 为 "true"/"false"。
func boolKnob(key, label, def string) Knob {
	return Knob{Key: key, Label: label, Type: "bool", Default: def}
}

// intKnob builds an integer Knob (frontend renders a number input); def is the default as a
// string ("" = provider/model default).
//
// intKnob 构造整数 Knob（前端渲染数字输入）；def 为默认值字符串（"" = provider/模型默认）。
func intKnob(key, label, def string) Knob {
	return Knob{Key: key, Label: label, Type: "int", Default: def}
}

// decodeOpenAICompatModelIDs parses an OpenAI-style GET /models body ({"data":[{"id":...}]}) into
// its id list. Shared by every OpenAI-compat provider — this is list plumbing, not wire dialect
// (each provider still owns its own BuildRequest).
//
// decodeOpenAICompatModelIDs 解析 OpenAI 式 GET /models 返回（{"data":[{"id":...}]}）的 id 列表。
// 所有 OpenAI-compat 家共享——这是列表管道、非 wire 方言（各家仍自持 BuildRequest）。
func decodeOpenAICompatModelIDs(raw string) []string {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(raw), &resp) != nil {
		return nil
	}
	out := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out
}

// describeFromSpecs assembles ModelInfo for each id in an OpenAI-compat /models body by looking it
// up in specs; ids with no matching spec are skipped (unknown to this build's static catalog — the
// user may still target such a model id directly, it just carries no knobs/specs here).
//
// describeFromSpecs 对 OpenAI-compat /models 返回里每个 id 查 specs 装配 ModelInfo；无匹配 spec 的
// id 跳过（本版静态目录未知——用户仍可直接用该 id，只是这里没有旋钮/规格）。
func describeFromSpecs(specs []modelSpec, raw string) []ModelInfo {
	ids := decodeOpenAICompatModelIDs(raw)
	out := make([]ModelInfo, 0, len(ids))
	for _, id := range ids {
		s, ok := matchSpec(specs, id)
		if !ok {
			continue
		}
		out = append(out, ModelInfo{
			ID:            id,
			DisplayName:   id,
			ContextWindow: s.ctx,
			MaxOutput:     s.out,
			Knobs:         s.knobs,
			Vision:        s.vision,
			NativeDocs:    s.nativeDocs,
		})
	}
	return out
}
