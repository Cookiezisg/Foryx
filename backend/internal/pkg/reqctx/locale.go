package reqctx

import "context"

// Locale is the user's preferred language for AI-generated content
// (prompts, titles, summaries). NOT used for backend error messages —
// those stay English; frontend localizes by error code.
//
// Locale 是用户偏好的 AI 生成内容语言（提示 / 标题 / 摘要）。
// 不用于后端错误消息——后端保持英文，前端按 error code 本地化。
type Locale string

const (
	LocaleZhCN    Locale = "zh-CN"
	LocaleEn      Locale = "en"
	DefaultLocale        = LocaleZhCN
)

// IsSupported reports whether the locale is one this backend handles.
//
// IsSupported 报告该 locale 是否被后端支持。
func (l Locale) IsSupported() bool {
	return l == LocaleZhCN || l == LocaleEn
}

type localeKey struct{}

// SetLocale returns a copy of ctx carrying l.
//
// SetLocale 返回携带 l 的 ctx 拷贝。
func SetLocale(ctx context.Context, l Locale) context.Context {
	return context.WithValue(ctx, localeKey{}, l)
}

// GetLocale returns the locale or DefaultLocale (preference, not identity —
// always returns a usable value).
//
// GetLocale 返回 locale 或 DefaultLocale（偏好而非身份，总返回可用值）。
func GetLocale(ctx context.Context) Locale {
	if l, ok := ctx.Value(localeKey{}).(Locale); ok && l.IsSupported() {
		return l
	}
	return DefaultLocale
}
