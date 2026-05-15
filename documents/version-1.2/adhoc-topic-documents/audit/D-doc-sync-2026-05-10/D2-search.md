# D2 — `service-design-documents/search.md` ↔ `internal/app/tool/search/` Sync Audit

**Doc**: `documents/version-1.2/service-design-documents/search.md` (257 lines)
**Code**: `backend/internal/app/tool/search/` (5 files: `search.go`, `grep.go`, `grep_rg.go`, `grep_stdlib.go`, `glob.go`)
**Spec authorities**: CLAUDE.md §S14 (doc-sync) + §S18 (Tool interface)

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `SearchTools` factory takes a `log *zap.Logger` second parameter — used internally by `Grep` for warn-on-rg-fallback log | `search.go:43` | MED |
| `Grep` struct has `log *zap.Logger` field (used in `Execute` line 286 to log rg-backend failure → stdlib fallback transition) | `grep.go:189` | MED |
| `Grep.Execute` warn-log on rg failure (`"Grep: rg backend failed; falling through to stdlib (results may differ for PCRE-only patterns)"`) — defect-class lesson "B2 bash auto-route silent fallback" reference; not mentioned in doc §5.1 | `grep.go:285-289` | MED |

---

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| §4.3 factory signature shows `func SearchTools(pathGuard pathguardpkg.PathGuard) []toolapp.Tool` — actual signature has additional `log *zap.Logger` param | search.md:157 vs `search.go:43` | MED |
| §5.1 dual-backend pseudocode shows `rg fallback execStdlib`comment "失败（rg exit ≠ 0,1）→ fallback execStdlib（防止 rg 偶发故障让搜索全跪）" — code does this BUT doc omits the audit-trail warn log requirement (defect class lesson encoded in code but missing from design narrative) | search.md:177 vs `grep.go:285-289` | LOW |

---

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| `Glob.Execute` reads `os.DirFS(root)` rooted FS — doc §5.4 says this. But §4.2 returns "matches" with absolute paths (after `filepath.Join(root, rel)`) — works fine and doc §4.2 example payload shows absolute paths, so OK; just noting glob input rel + output absolute mapping is implicit. | `glob.go:239` | LOW (no real mismatch, just implicit) |
| `Grep` ValidateInput rejects `pattern == "" || strings-only-whitespace` (line 217 uses `strings.TrimSpace`) but doc §4.1 ValidateInput list says "pattern 缺 / 空 / 仅空白" — aligned. Same for Glob.ValidateInput line 172 — but Glob does NOT trim whitespace; it rejects only literal `""`. Doc §4.2 ValidateInput list says only "限于 `ErrEmptyPattern`"; doesn't claim whitespace check. Sub-spec drift: Grep + Glob have inconsistent emptiness rule (Grep trims, Glob doesn't). | `glob.go:172` vs `grep.go:217` | LOW (consistency hygiene) |

---

## Sub-check

- **Tool list aligned**: yes — doc §4 lists Grep / Glob; code factory `SearchTools` returns `newGrep(pathGuard, log)` + `newGlob(pathGuard)` in that order.
- **9-method interface aligned**: yes — Each tool implements all 9 methods. `var _ toolapp.Tool = (*Grep)(nil)` at `grep.go:297` and `var _ toolapp.Tool = (*Glob)(nil)` at `glob.go:303`.
- **Static metadata (IsReadOnly / NeedsReadFirst / RequiresWorkspace) aligned**: yes — Both tools match §S18 §8 table:
  - Grep: `(true, false, true)` ✓ — `grep.go:200-202`
  - Glob: `(true, false, true)` ✓ — `glob.go:154-156`
- **Parameters schema aligned**: yes — Doc §4.1/§4.2 Args tables match `grepSchema` / `globSchema` field names + types + required arrays. Confirmed: pattern, path, glob, type, output_mode (enum 3 values), -A/-B/-C, -n, -i, multiline, head_limit (Grep); pattern, path, limit (Glob).
- **Emit pattern (eventlog Emitter)**: N/A — Search tools return final string; no streaming events.
- **Sentinel/errmap**: Sentinels (`ErrEmptyPattern`, `ErrInvalidOutputMode`) are tool-internal validation errors; per §S17 don't need errmap. Doc §8 says "errmap 无登记" — aligned.

---

## Summary

**0 HIGH / 2 MED / 3 LOW** — search.md drifted on **factory signature** (missing log param) and the **rg → stdlib fallback warn log** behavioural change. The latter encodes a Forgify-specific defect-class lesson ("don't silently swap backends") that the design doc should document, since the warn log is part of the tool's safety story (operator must see when stdlib becomes the de-facto backend).

Tool surface (names, schemas, metadata) is all aligned with code + §S18 §8.
