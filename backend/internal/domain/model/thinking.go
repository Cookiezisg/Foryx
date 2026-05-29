package model

// ThinkingSpec is the provider-neutral reasoning intent carried with a model
// selection. nil = auto (send no thinking param, = current default behavior).
// Each provider adapter translates it to its wire shape (effort enum / budget
// tokens / enabled-disabled) in BuildRequest.
//
// ThinkingSpec 是随模型选择携带的、provider 中立的推理意图。nil = auto(不发
// thinking 参数,= 现状)。各 provider adapter 在 BuildRequest 里翻译成自己的
// 线上形状(effort 枚举 / budget token / 开关)。
type ThinkingSpec struct {
	Mode   string `json:"mode"`             // "auto" | "off" | "on"
	Effort string `json:"effort,omitempty"` // "minimal|low|medium|high|xhigh|max" — effort-shape providers
	Budget int    `json:"budget,omitempty"` // reasoning token budget — budget-shape providers
}
