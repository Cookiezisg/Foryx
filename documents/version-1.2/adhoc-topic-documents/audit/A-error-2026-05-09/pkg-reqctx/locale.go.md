# File audit: backend/internal/pkg/reqctx/locale.go

LOC: 44. Locale ctx ferry + 类型 + IsSupported predicate。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | locale.go:11-17 | `type Locale string`<br>`const (`<br>`	LocaleZhCN    Locale = "zh-CN"`<br>`	LocaleEn      Locale = "en"`<br>`	DefaultLocale        = LocaleZhCN`<br>`)` | A.3 | OK | 常量定义，无 ID 生成。 | — | — | — | — |
| 2 | locale.go:22-24 | `func (l Locale) IsSupported() bool {`<br>`	return l == LocaleZhCN || l == LocaleEn`<br>`}` | A.1 | OK | 纯 predicate，无错误路径。 | — | — | — | — |
| 3 | locale.go:31-33 | `func SetLocale(ctx context.Context, l Locale) context.Context {`<br>`	return context.WithValue(ctx, localeKey{}, l)`<br>`}` | A.2 | OK | ctx setter，无错误路径。 | — | — | — | — |
| 4 | locale.go:39-44 | `func GetLocale(ctx context.Context) Locale {`<br>`	if l, ok := ctx.Value(localeKey{}).(Locale); ok && l.IsSupported() {`<br>`		return l`<br>`	}`<br>`	return DefaultLocale`<br>`}` | A.1 | OK | 总返可用值（preference，非 identity）——godoc 行 35-37 显式说明这不是 identity 类 require pattern，缺失返默认值是合法 branch。**注意**：unsupported locale (如 "fr") 也走 default fallback——这是设计意图（行 22 IsSupported 显式仅认 zhCN/en）。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - notes: site#4 GetLocale 走 fallback to DefaultLocale 是 "preference" 设计语义（godoc 行 35-37），非吞错误（无错误概念）

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 无
  - violations: N/A: package doesn't do terminal writes (pure ctx ferry)

A.3 §S15 ID 生成:
  - ID generation calls: 无
  - violations: N/A: package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (no error returns)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: 无
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels
