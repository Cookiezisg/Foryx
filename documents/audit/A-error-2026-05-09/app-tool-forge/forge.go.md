# audit: backend/internal/app/tool/forge/forge.go

LOC: 197
Read: full file (lines 1-197)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | forge.go:81-84 | `att, err := repo.GetAttachment(ctx, s); if err != nil { return nil, fmt.Errorf("resolveAttachments: %w", err) }` | A.4 | EDGE | §S16: prefix is `resolveAttachments:` (helper-style, no pkg qualifier). Has `%w` ✓; sentinel chain preserved through bare function name. Same style as forge.go:#streamCode helpers and many other packages — internally consistent but not §S16 spec literal `<pkg>.<Method>:` form. | LOW | identical UX (errors.Is reaches sentinel either way); harder to grep call-site | tighten to `forgetool.resolveAttachments: %w` for consistency with §S16 spec literal | FOUND |
| 2 | forge.go:113-116 | `bc, err := llmclientpkg.Resolve(ctx, picker, keys, factory); if err != nil { return "", fmt.Errorf("streamCode: %w", err) }` | A.4 | EDGE | §S16: prefix is `streamCode:` (helper-style); same pattern as #1. Sentinel chain preserved (`llmclientpkg.ErrPickModel` / `ErrResolveCreds` from llmclientpkg.Resolve will unwrap correctly). | LOW | identical UX | tighten prefix to `forgetool.streamCode:` | FOUND |
| 3 | forge.go:126-135 | `for event := range bc.Client.Stream(ctx, req) { switch event.Type { case llminfra.EventText: buf.WriteString(event.Delta); ...; case llminfra.EventError: return "", fmt.Errorf("streamCode: %w", event.Err) }}` | A.4 | EDGE | §S16: same `streamCode:` prefix issue. POSITIVE: post-commit 363b084 introduces llm.ErrAuthFailed/ErrRateLimited/etc. sentinels; `event.Err` is a wrapped llminfra error, so chain is now discriminative. errors.Is path enables apikey.MarkInvalid (which is invoked from search.go in app/tool/web — not from here directly). | LOW | identical UX; harder grep | same as #2 | FOUND |
| 4 | forge.go:137-139 | `if err := ctx.Err(); err != nil { return "", fmt.Errorf("streamCode: %w", err) }` | A.4 | EDGE | §S16: same `streamCode:` prefix. ctx.Err() is stdlib context.Canceled / DeadlineExceeded (registered errmap.go:181-182 in commits e36f890 / earlier). | LOW | identical UX | same as #2 | FOUND |
| 5 | forge.go:184-196 | `func extractCode(raw string) string { ... }` (no error returns) | N/A | OK | pure string utility, no error paths — N/A | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (all errors propagate through %w wrap; resolveAttachments returns err untouched, streamCode bubbles all events with %w)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is pure helper layer (resolveAttachments / streamCode / extractCode / buildXxxPrompt). Terminal writes happen in app/forge service (DEFERRED pending forge rewrite); tool layer just delegates.

A.3 §S15 ID 生成:
  - ID generation calls: none in this file (NewForgeID / NewVersionID called from create.go + edit.go — see those traces)
  - violations: N/A — file generates no business IDs

A.4 §S16 错误 wrap 格式:
  - violations: sites #1 (resolveAttachments prefix), #2/#3/#4 (streamCode prefix) — all helper-style instead of canonical `forgetool.<Method>:` form. Functional UX identical (sentinel chain preserved via %w).

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (only helpers)
