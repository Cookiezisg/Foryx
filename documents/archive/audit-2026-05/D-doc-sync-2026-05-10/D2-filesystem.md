# D2 — `service-design-documents/filesystem.md` ↔ `internal/app/tool/filesystem/` Sync Audit

**Doc**: `documents/version-1.2/service-design-documents/filesystem.md` (297 lines)
**Code**: `backend/internal/app/tool/filesystem/` (3 tool files: `read.go`, `write.go`, `edit.go` + `filesystem.go` factory)
**Spec authorities**: CLAUDE.md §S14 (doc-sync) + §S18 (Tool interface + §8 metadata table)

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `Read` description text contains "Some sensitive paths… are blocked for safety" line — doc §4.1 special-case list omits this normal-path guidance | `read.go:91` | LOW |
| `Write` description includes "Do NOT create documentation files (*.md, README) or files outside the user's working scope unless explicitly requested" — Forgify-specific safety note absent from doc | `write.go:67` | LOW |
| `Read` description hints "system reminder warning" verbatim text — doc §4.1 special list shows this but description text wording not quoted | `read.go:90` | LOW (description drift) |
| `Edit` description's "preserve exact indentation as it appears AFTER the line number prefix" instruction (LLM-facing carve-out for Read output) | `edit.go:76` | LOW |

---

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| Doc §4.2 says `Schema 用 *string 检测 content 字段缺失` — actual `Write.ValidateInput` does use `*string` BUT JSON schema (`writeSchema`) declares `content` as plain `"type": "string"` with `"required": ["file_path", "content"]`. Doc claim is half-true: ValidateInput uses `*string`, schema uses required-array. Reader is left assuming JSON Schema declares `nullable string`, which it doesn't. | filesystem.md:132 vs `write.go:74-87` | LOW (description drift) |

---

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| `Edit` ValidateInput sentinel `ErrEmptyOldString` rejects `nil` OR empty pointer; doc §4.3 ValidateInput list says "old_string 空 / 缺" mapping to `ErrEmptyOldString`. But code line 153 also explicitly rejects `nil` pointer (key missing) with the same sentinel — implementation rolls "missing key" + "empty value" into one sentinel, doc separates them ambiguously. | `edit.go:153` | LOW |
| `Edit` ValidateInput rejects missing `new_string` field with **anonymous** `errors.New("new_string field is required (use empty string to delete the matched text)")` — doc §4.3 lists this as "缺 `new_string` key → `errors.New(...)`". Aligns, but anonymous error → no sentinel for any future external consumer. | `edit.go:157` | LOW |

---

## Sub-check

- **Tool list aligned**: yes — doc §4 lists Read / Write / Edit; code factory `FilesystemTools` (filesystem.go:45) returns those 3 in order.
- **9-method interface aligned**: yes — Each tool implements all 9 methods (Identity 3, static metadata 3, args-dependent 2, Execute 1). Confirmed by `var _ toolapp.Tool = ...` compile-time checks at `read.go:333`, `write.go:281`, `edit.go:338`.
- **Static metadata (IsReadOnly / NeedsReadFirst / RequiresWorkspace) aligned**: yes — All three tools match §S18 §8 table:
  - Read: `(true, false, true)` ✓ — `read.go:135-137`
  - Write: `(false, true, true)` ✓ — `write.go:107-109`
  - Edit: `(false, true, true)` ✓ — `edit.go:127-129`
- **Parameters schema aligned**: yes — Doc §4.1/§4.2/§4.3 Args tables match `readSchema` / `writeSchema` / `editSchema` field names + types + required arrays exactly. `replace_all` boolean default `false` documented and present in `editSchema`.
- **Emit pattern (eventlog Emitter)**: N/A — Filesystem tools don't push streaming events. They return content as the tool_result string; framework handles the surrounding tool_call lifecycle. §S18 §3 only requires Emitter for tools that emit progress / streaming output mid-execute.
- **Sentinel/errmap**: All filesystem sentinels (`ErrEmptyFilePath`, `ErrPathNotAbsolute`, `ErrNegativeOffset`, `ErrNegativeLimit`, `ErrEmptyOldString`, `ErrEditNoOp`) are tool-internal — return as `error` from `ValidateInput`, framework converts to LLM-facing tool_result string. Per §S17, these don't need errmap registration since they never reach `responsehttpapi.FromDomainError`. Doc §8 explicitly states "errmap 无登记" — aligned.

---

## Summary

**0 HIGH / 0 MED / 5 LOW** — filesystem.md is **closely aligned** with code. The 5 LOW items are all description-drift / minor wording carve-outs, not structural mismatches. All 3 tools have correct metadata vs §S18 §8 table, factory signature matches, schemas match, sentinel surface matches.

This domain is a model citizen for doc-sync discipline.
