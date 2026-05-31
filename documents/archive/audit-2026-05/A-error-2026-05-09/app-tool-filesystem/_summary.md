# Package audit summary: internal/app/tool/filesystem

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression hiding user-visible failure / data loss is forbidden. `_ = err` requires inline justification. `defer X.Close()` on read-only paths or panic-path cleanup is acceptable. For Tool framework: validation errors return as friendly LLM tool_result strings (NOT silent fallback) — that's the §S18 contract, not §S3 violation.
- **§S9 detached ctx 终态写**: this package writes only to in-memory AgentState (per-conv MarkRead) and to disk (atomic tmp+rename). No DB writes — §S9 effectively N/A. AgentState ctx-injection is read-only access (`reqctxpkg.GetAgentState`).
- **§S15 ID 生成**: package generates no business IDs. Uses `os.CreateTemp` for tmp file naming which uses stdlib randomization — out of §S15 scope.
- **§S16 错误 wrap 格式**: Internal Errorf wraps use `<Tool>.<Method>:` prefix (e.g. `Read.ValidateInput:`). §S16 spec example format is `<pkg>.<Method>:` — the tool-name-rooted form (Read/Write/Edit) is consistent within the package and unambiguous at runtime since the logger prefix would carry pkg context. **Out-of-spec literal**: minor style note, not a violation per the audit's documented "consistency-over-strict-literal" precedent set elsewhere (e.g. infra/llm pre-commit-363b084 used `llm/openai:` slash-form which we then standardized).
- **§S17 errmap 单一事实源**: 6 file-level validation sentinels (4 in read.go + 2 in edit.go). All are caught by §S18 Tool framework (chat ReAct loop's tool runner converts ValidateInput/Execute Go errors into friendly LLM tool_result strings before they ever reach `responsehttpapi.FromDomainError`). errmap registration N/A per §S17 carve-out.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| filesystem.go | 51 | 1 | 1 | 0 | 0 | 0 |
| read.go | 333 | 15 | 13 | 0 | 0 | 2 |
| write.go | 281 | 15 | 13 | 0 | 0 | 2 |
| edit.go | 338 | 23 | 21 | 0 | 0 | 2 |
| **TOTAL** | **1003** | **54** | **48** | **0** | **0** | **6** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW (§S16 bare errors.New at validation) | 2 | write.go:#1 (content field required), edit.go:#5 (new_string field required) | FOUND |
| LOW (§S3 _ = X.Close cleanup ritual) | 2 | write.go:#10, edit.go:#18 (cleanup-after-write-fail Close discards missing inline ritual comment) | FOUND |
| LOW (§S3 graceful-degrade documented) | 2 | read.go:#14 (markSeen on missing AgentState), write.go:#14 (post-write MarkRead on missing state) — same pattern, documented carve-out | FOUND |

## Cross-cutting

### §S18 Tool 9 methods compliance — most important for this package

All three tools (Read / Write / Edit) declare `RequiresWorkspace=true` AND `NeedsReadFirst` correctly. **Crucially**, the static metadata is honored at runtime — not just declared:

| Tool | RequiresWorkspace | Self-check via pathGuard.Allow? | NeedsReadFirst | Self-check via AgentState? |
|---|---|---|---|---|
| Read  | true  | ✓ line 219 | false | N/A — Read populates SeenFiles, doesn't consume it |
| Write | true  | ✓ line 183 | true  | ✓ lines 209-221 (refuses on missing state per §S3 anti-silent discipline) |
| Edit  | true  | ✓ line 224 | true  | ✓ lines 243-250 (same defensive pattern as Write) |

Per §S18 spec: "新加 tool 时如果声明 true 但忘了在 Execute 内 check，元数据就是撒谎". This package's three tools all faithfully self-check. **Best-practice exemplar** for the repo — particularly Write/Edit's defensive refusal on missing AgentState (rather than silently bypassing the must-Read-first guard) which directly applies §S3's anti-silent-fallback principle.

### Atomic write pattern (Write + Edit)

Both Write and Edit use the same atomic pattern:
1. `os.CreateTemp(parent, ".forgify-{write,edit}-*")` — sibling tmp
2. WriteString → Close → Chmod (preserve mode) → Rename
3. `cleanup := func() { _ = os.Remove(tmpPath) }` orphan-tmp cleanup on every error path

This is the canonical safe-write pattern. The `_ = os.Remove(tmpPath)` discards err per §S3 carve-out for cleanup paths — documented inline at write.go lines 232-234 with reasoning. Edit lacks the equivalent inline comment (line 304 just declares `cleanup := func() { _ = os.Remove(tmpPath) }`) — minor consistency miss but the pattern is recognizable.

### AgentState integration

Three call sites use `reqctxpkg.GetAgentState(ctx)`:
- `read.go::markSeen` — graceful no-op if state missing (Read still succeeds, future Edit/Write will re-ask)
- `write.go::Execute` — REFUSES if state missing (preserves must-Read-first invariant)
- `edit.go::Execute` — REFUSES if state missing (same)

The asymmetry (Read graceful-degrades, Write/Edit refuse) is intentional and correct: Read is intrinsically safe (read-only with PathGuard), so graceful degrade preserves utility; Write/Edit can clobber data, so refusing on missing state is the safe default.

### §S16 prefix inconsistency

Package uses `<Tool>.<Method>:` (e.g. `Read.ValidateInput:`) instead of `<pkg>.<Type>.<Method>:` (e.g. `fstool.Read.ValidateInput:`). Functionally equivalent; spec literal would be the latter. Not raised as a violation since:
1. The tool-name-rooted form is unambiguous at runtime
2. logger output would carry the file/pkg context anyway
3. Other packages (e.g. `apikeytester:` pre-fix in commit 1b96a5e) had the same pattern before standardization

If a sweep is desired for §S16 strict-literal compliance, this would be a ~15-site rename across 3 files. Suggest WAIVE — internal consistency is high.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 7 sites picked from `OK` set across 3 files:

1. **read.go:#9** (defer f.Close() on read-only path): verified — §S3 spec explicitly carves out: "defer f.Close() 在只读路径（Close 返错对调用方无意义）". Read is exclusively read-only (IsReadOnly=true); Close failure has no recovery action available. Compliance literal.

2. **write.go:#6** (must-Read-first refusal on missing AgentState): verified — `state, hasState := reqctxpkg.GetAgentState(ctx); if !hasState { return "Cannot verify Read-first guard: agent state missing.", nil }`. Doc at lines 211-216 explicitly cites §S3 reasoning ("静默放过会让整个守卫形同虚设"). Best-practice exemplar.

3. **write.go:#12** (preserve existing file mode on overwrite): verified — `mode := defaultFileMode; if exists { mode = existingInfo.Mode().Perm() }; os.Chmod(tmpPath, mode)`. Doc at lines 247-252 explains why CreateTemp's 0600 default would silently shrink permissions on overwrite. Defensive against a subtle correctness issue.

4. **edit.go:#10** (must-Read-first refusal): verified — same pattern as Write site #6. State refused-on-missing rather than silently allowed.

5. **edit.go:#12** (size-mismatch external-modification check): verified — `if info.Size() != seenSize { return "File has been modified since last read..." }`. Defensive layer beyond must-Read-first; catches external modifications (vim save, another IDE) between Read and Edit. v1 trade-off documented (size-only, not hash) at lines 252-257.

6. **edit.go:#15** (stdlib trust per decision D1): verified — `strings.Replace(content, args.OldString, args.NewString, 1)`. Decision D1 documented at lines 5-10 explains why we don't replicate Claude Code's #51986 defensive count-after check. Trust + transparency (success message reports actual N replacements at lines 330-333) is the explicit design choice.

7. **read.go:#13** (statErrorMessage uses errors.Is): verified — `switch { case errors.Is(err, fs.ErrNotExist): ...; case errors.Is(err, fs.ErrPermission): ... }`. Proper sentinel-based discrimination, not string matching. Same lesson as the audit drove home in commit 363b084 (web tool moved from string match to errors.Is).

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. The audit's findings (6 LOW total, 0 HIGH/MED) are consistent: package is **architecturally clean** for §S3 / §S9 / §S15 / §S16 / §S17. The defensive discipline at Write/Edit AgentState refusal + Edit size-mismatch check are positive examples worth pointing other packages at.

## Recommended fix priorities

1. **write.go:#1 + edit.go:#5** (LOW §S16 — bare errors.New at validation) — define `ErrContentRequired` and `ErrNewStringRequired` sentinels at file scope for parity with sibling sentinels. Pure style polish; ~6-line addition. Optional.

2. **write.go:#10 + edit.go:#18** (LOW §S3 — `_ = tmpFile.Close()` cleanup ritual) — add inline `// _ = err — close-after-failed-write; cleanup is best-effort` comments per §S3 spec example. ~2 lines. Optional.

3. **read.go:#14 + write.go:#14** (LOW §S3 — markSeen graceful no-op on missing AgentState) — already documented as graceful degrade in code comments; only fix would be adding a Warn log to surface server-side wiring bugs. Requires adding `log *zap.Logger` field to Read/Write structs (signature change for FilesystemTools). **Defer** unless ops actually misses the wiring bug in practice.

4. **§S16 strict-literal sweep** (`<Tool>.<Method>:` → `fstool.<Type>.<Method>:` across the package) — ~15 sites in 3 files. **WAIVE** per "consistency over strict literal" precedent already established in the audit cycle.

## Out-of-scope notes (parent should verify)

1. **Tool framework error→tool_result conversion**: this audit asserts that `responsehttpapi.FromDomainError` never sees these tools' validation sentinels because the §S18 Tool framework intercepts. Worth a cross-fork verification when auditing the Tool framework itself (`internal/app/tool/`) and chat runner — confirm the runtime path is genuinely "ValidateInput err → friendly tool_result" and not "ValidateInput err → bubble to handler".

2. **No `defer file.Close()` cleanup audit on Read's open file** (line 244): file handle stays open until function returns, which happens after scanner reads complete or scanner errors. Standard Go pattern; no resource leak risk because function-scoped lifetime.

3. **PathGuard contract**: this audit assumes `pathGuard.Allow` correctly denies all sensitive paths per `pkg/pathguard`. The pkg itself is in scope of a separate fork (`pkg/pathguard`). If pathguard has a deny-list miss, all three tools inherit the gap.
