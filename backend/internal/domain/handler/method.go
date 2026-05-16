package handler

// MethodSpec is one Python method's full description (schema + body).
//
// MethodSpec 是一个 Python method 的完整描述（schema + body）。
type MethodSpec struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Args         []ArgSpec      `json:"args"`
	ReturnSchema map[string]any `json:"returnSchema,omitempty"`

	// Body is the Python method body without the def header.
	// Body 是 method body 字符串，不含 def 头。
	Body string `json:"body"`

	// Streaming=true means body uses yield; driver translates each yield into a progress delta.
	// Streaming=true 表 body 用 yield；driver 把每次 yield 翻成 progress delta。
	Streaming bool `json:"streaming"`

	// Timeout in ms for this method call (0 = driver default 30s); ctx cancel still wins.
	// 单 method timeout（ms，0=driver 默认 30s）；ctx cancel 优先。
	Timeout int `json:"timeout,omitempty"`
}

// ArgSpec describes one method argument's JSON-schema shape.
//
// ArgSpec 是一个 method 参数的 JSON-schema 形状。
type ArgSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// InitArgSpec describes one __init__ one-time parameter; Sensitive=true means encrypted at rest.
//
// InitArgSpec 是 __init__ 一次性参数的 schema；Sensitive=true 表加密存、GET 返掩码。
type InitArgSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"`
	Default     any    `json:"default,omitempty"`
}
