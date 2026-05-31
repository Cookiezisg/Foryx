# Audit: backend/internal/transport/httpapi/middleware/locale.go

**LOC**: 35 (production); function `InjectLocale` + helper `parseAcceptLanguage`.

## Purpose

Parse `Accept-Language` header → stamp ctx with a supported `reqctxpkg.Locale`. Unsupported/missing → `DefaultLocale` (zh-CN per `parseAcceptLanguage` fallback).

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | locale.go:15-21 | `func InjectLocale(next http.Handler) http.Handler { return http.HandlerFunc(func(w, r) { loc := parseAcceptLanguage(r.Header.Get("Accept-Language")); ctx := reqctxpkg.SetLocale(r.Context(), loc); next.ServeHTTP(w, r.WithContext(ctx)) }) }` | A.1 | OK | §S3 — fallback to default locale on missing header is **explicit design** (godoc line 11: "Unsupported / missing → reqctxpkg.DefaultLocale"). Not a silent fallback masking failure: i18n preference is best-effort, no error semantics — every request must have a locale, and zh-CN default is the project's primary user language. | — | — | — | — |
| 2 | locale.go:29-35 | `func parseAcceptLanguage(header string) reqctxpkg.Locale { header = strings.ToLower(strings.TrimSpace(header)); if strings.HasPrefix(header, "en") { return reqctxpkg.LocaleEn }; return reqctxpkg.LocaleZhCN }` | A.1 | OK | §S3 — simplified BCP47 prefix match. The "everything else → zh-CN" fallback is documented (godoc line 23-25 / 27); no error path. Future improvement (x/text/language) noted but not required for current scope. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A: middleware doesn't perform terminal writes (purely reads/decorates ctx)
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no error returns)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: none
  - missing: N/A: file defines no sentinels
```

## Findings

**Clean** — no §S3/S9/S15/S16/S17 issues. Default-locale fallback is documented intent, not silent error swallowing.
