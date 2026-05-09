# audit: backend/internal/app/forge/sandbox_types.go

LOC: 131
Read: full file (lines 1-131)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | sandbox_types.go:68-74 | `type SyncError struct { Cause error; Stderr string }; func (e *SyncError) Error() string { return e.Stderr }; func (e *SyncError) Unwrap() error { return e.Cause }` | A.4 | OK | error wrapper type with proper Unwrap() — preserves sentinel chain through errors.Is. Error() returns Stderr (the user-actionable info). §S16 compliant via Unwrap. | N-A | — | — | — |
| 2 | sandbox_types.go:96-107 | `func ComputeEnvID(deps []string, pythonVersion string) string { ...; h := sha256.Sum256([]byte(payload)); return "env_" + hex.EncodeToString(h[:6]) }` | A.3 | EDGE | §S15: this generates an `env_` prefix ID, but it's NOT a business-domain ID per §S15 spec list (which specifies `cv_/msg_/aki_/...` etc). It's a content-derived hash for sandbox env identity (deduplication key); deterministic across processes intentionally — `crypto/rand` would defeat the dedup purpose. Not a §S15 violation: §S15 governs randomly-minted business IDs, not content-derived hashes. **However**: the `env_` prefix isn't in the §S15 spec list nor documented as exception → §S14 doc-sync concern. | LOW | none — function is correct for its purpose; just unlisted in spec | document `env_` prefix in CLAUDE.md §S15 OR sandbox.md as content-derived exception, OR rename to non-`X_` form to avoid mimicking the §S15 ID convention | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (file is purely value types + pure functions; no error handling sites)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is value types + ComputeEnvID hash function; no DB / writes

A.3 §S15 ID 生成:
  - ID generation calls: ComputeEnvID at #2 (content-derived hash, NOT crypto/rand-based business ID)
  - violations: not strictly — §S15 governs randomly-minted business IDs not content-derived hashes; the `env_` prefix collision with §S15 prefix conventions is §S14 concern (out of Phase A scope)

A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls; only the SyncError type which uses proper Unwrap()/Error() pattern)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: SyncError type (struct, not sentinel; consumed via errors.As at higher layer)
  - 已登记 errmap: N/A — SyncError is a wrapper type not a sentinel; errors.As caller extracts Stderr field
  - missing: N/A
