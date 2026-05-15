# audit: backend/internal/app/skill/mutate.go

LOC: 214
Read: full file (lines 1-214)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | mutate.go:41-52 | `Body: ...; if !ok { return nil, fmt.Errorf("skillapp.Body: %w: %q", skilldomain.ErrSkillNotFound, name) }; body, err := os.ReadFile(sk.BodyPath); if err != nil { return nil, fmt.Errorf("skillapp.Body %s: %w", name, err) }` | A.4 | OK | §S16 canonical: pkg.Method + sentinel + %w; sentinel registered errmap.go:149. | N-A | — | — | — |
| 2 | mutate.go:64-67 | `Create: if err := validateName(name); err != nil { return nil, err }` | A.4 | OK | bare-return — validateName already wraps with sentinel + pkg.Method (site #11). Sentinel chain preserved. | N-A | — | — | — |
| 3 | mutate.go:71-73, 74-76 | `if err := validateFrontmatter(fm); err != nil { return nil, err }; if err := validateBodySize(body); err != nil { return nil, err }` | A.4 | OK | bare-return — validateFrontmatter / validateBodySize wrap with sentinel + %w. Same pattern as #2. | N-A | — | — | — |
| 4 | mutate.go:78-83 | `dir := filepath.Join(s.skillsDir, name); if _, err := os.Stat(dir); err == nil { return nil, fmt.Errorf("skillapp.Create: %w: %q", skilldomain.ErrNameConflict, name) } else if !errors.Is(err, fs.ErrNotExist) { return nil, fmt.Errorf("skillapp.Create: stat: %w", err) }` | A.4 | OK | §S16 canonical for both branches. ErrNameConflict registered errmap.go:152 → 409. | N-A | — | — | — |
| 5 | mutate.go:85-87 | `if err := writeSkillDir(dir, fm, body); err != nil { return nil, fmt.Errorf("skillapp.Create %s: %w", name, err) }` | A.4 | OK | §S16 canonical wrap with name + %w. Inner writeSkillDir uses helper-style prefix (see #14). | N-A | — | — | — |
| 6 | mutate.go:88-91 | `if err := s.Scan(ctx); err != nil { return nil, fmt.Errorf("skillapp.Create %s: rescan: %w", name, err) }; return s.Get(ctx, name)` | A.2/A.4 | OK | §S16 canonical. **§S9 verdict**: Scan/Get use request ctx — but this is mid-operation (file already written; cache rebuild + return). If ctx cancels, the file persists on disk and the next polling tick (1s) will pick it up. NOT a §S9 terminal write because the on-disk file IS the source of truth — the cache is just an index. | N-A | — | — | — |
| 7 | mutate.go:100-125 | `Replace: ...` (signature mirrors Create) | A.4 | OK | §S16 canonical throughout; same pattern as Create. ErrSkillNotFound registered. | N-A | — | — | — |
| 8 | mutate.go:133-150 | `Delete: ...; if err := os.RemoveAll(dir); err != nil { return fmt.Errorf("skillapp.Delete %s: %w", name, err) }; if err := s.Scan(ctx); err != nil { return fmt.Errorf("skillapp.Delete %s: rescan: %w", name, err) }` | A.4 | OK | §S16 canonical. Scan err propagated with rescan: tag. | N-A | — | — | — |
| 9 | mutate.go:159-164 | `validateName: if !nameRegexp.MatchString(name) { return fmt.Errorf("skillapp.validateName: %w: %q (must match %s)", skilldomain.ErrInvalidName, name, nameRegexp.String()) }` | A.4 | OK | §S16 canonical: pkg.Method + sentinel + diagnostic context. Registered errmap.go:153 → 422. | N-A | — | — | — |
| 10 | mutate.go:172-181 | `validateBodySize: if len(body) > skilldomain.MaxBodyBytes { return fmt.Errorf("skillapp.validateBodySize: %w: body %d bytes (cap %d)", skilldomain.ErrBodyTooLarge, len(body), skilldomain.MaxBodyBytes) }` | A.4 | OK | §S16 canonical. | N-A | — | — | — |
| 11 | mutate.go:191-194 | `writeSkillDir: if err := os.MkdirAll(dir, 0o755); err != nil { return fmt.Errorf("mkdir %s: %w", dir, err) }` | A.4 | EDGE | §S16: prefix is `mkdir %s:` not canonical `skillapp.writeSkillDir: mkdir %s:`. Helper-style; caller wraps with their own pkg.Method (Create/Replace site #5). | LOW | identical UX (caller wraps); harder to grep `writeSkillDir` call site | wrap: `fmt.Errorf("skillapp.writeSkillDir: mkdir %s: %w", dir, err)` for grep | FOUND |
| 12 | mutate.go:195-198 | `yamlBytes, err := yaml.Marshal(&fm); if err != nil { return fmt.Errorf("marshal frontmatter: %w", err) }` | A.4 | EDGE | §S16: same helper-style prefix issue as #11. yaml.Marshal of basic-type struct is unfailable in practice but the wrap is correct. | LOW | same | wrap with `skillapp.writeSkillDir: marshal frontmatter:` | FOUND |
| 13 | mutate.go:206-208 | `if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil { return fmt.Errorf("write tmp: %w", err) }` | A.4 | EDGE | §S16: same helper-style prefix issue. | LOW | same | wrap with `skillapp.writeSkillDir: write tmp:` | FOUND |
| 14 | mutate.go:209-212 | `if err := os.Rename(tmp, target); err != nil { _ = os.Remove(tmp); return fmt.Errorf("rename: %w", err) }` | A.1/A.4 | EDGE | **dual issue**: (a) §S16 helper-style prefix (`rename:`); (b) §S3 `_ = os.Remove(tmp)` cleanup-after-rename-fail without inline justification comment. Per §S3 spec example "panic 路径里的 cleanup" / "clean-up resource" carve-out — Remove failure on .tmp leftover is unactionable (caller already has the rename error). Missing the explicit `// _ = err — <reason>` ritual. | LOW | (a) same as #11-13; (b) zero functional risk — leftover .tmp will be overwritten on next write attempt or removed by OS tmp cleanup | (a) `fmt.Errorf("skillapp.writeSkillDir: rename: %w", err)`; (b) add `// _ = err — best-effort cleanup of leftover .tmp; the rename error is the user-facing problem` | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (site #14b is documented-intent cleanup-after-fail, missing inline ritual only)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: Create/Replace/Delete each (a) write to disk, (b) rebuild cache via Scan
  - 各自 ctx 来源: request ctx for Scan call after disk write
  - violations: not present — disk file IS the source of truth; cache is index only. If ctx cancels after disk write succeeds, the next polling tick (1s) picks up the change. UX: caller may receive the cancel-error but the change is durable.
  - **Distinction from apikey.Test §S9 case**: apikey.Test's test_status is in-DB-only (no other source of truth) → must use detached. skill mutations are file-system-anchored (1s polling resyncs cache) → request ctx OK.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — skill names are user-supplied (regex-validated)

A.4 §S16 错误 wrap 格式:
  - violations: not present in canonical-spec sense
  - LOW EDGE: sites #11, #12, #13, #14a (writeSkillDir helper uses helper-style prefix; caller wraps)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none (uses skilldomain sentinels: ErrSkillNotFound / ErrNameConflict / ErrInvalidName / ErrBodyTooLarge / ErrInvalidFrontmatter)
  - 已登记 errmap: all 5 ✓ (errmap.go:149-153)
  - missing: N/A
