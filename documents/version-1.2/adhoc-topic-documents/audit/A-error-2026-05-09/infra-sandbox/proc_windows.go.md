# audit: backend/internal/infra/sandbox/proc_windows.go

LOC: 150
Read: full file (lines 1-150)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | proc_windows.go:79-82 | `func EnsureMasterJob() error { masterJobOnce.Do(initMasterJob); return masterJobErr }` | A.4 | OK | sync.Once + cached err pattern; idempotent | N-A | — | — | — |
| 2 | proc_windows.go:84-89 | `h, err := windows.CreateJobObject(nil, nil); if err != nil { masterJobErr = fmt.Errorf("CreateJobObject: %w", err); return }` | A.4 | EDGE | §S16: has `%w` ✓; prefix is `CreateJobObject:` (Windows API name) not canonical `<pkg>.<Method>:`. Caller (Spawn paths) treats this as best-effort soft-fail per line 121-130 comment. | LOW | error message reads "CreateJobObject: <syscall err>"; caller logs context. errors.Is preserves syscall chain. | tighten: `fmt.Errorf("sandbox.initMasterJob: CreateJobObject: %w", err)` for grep traceability | FOUND |
| 3 | proc_windows.go:95-105 | `_, err = windows.SetInformationJobObject(...); if err != nil { _ = windows.CloseHandle(h); masterJobErr = fmt.Errorf("SetInformationJobObject: %w", err); return }` | A.1/A.4 | EDGE | **§S3**: `_ = windows.CloseHandle(h)` discards Close error in cleanup path (already-failed Job init). Per §S3 panic-path-cleanup carve-out OK; missing inline comment. **§S16**: prefix `SetInformationJobObject:` same family as #2. | LOW | leaked Job handle if CloseHandle fails (rare; OS reaps on process exit) | add inline comment: `_ = windows.CloseHandle(h) // best-effort cleanup; SetInformationJobObject failure is the actionable error`; tighten prefix to `sandbox.initMasterJob:` | FOUND |
| 4 | proc_windows.go:106-110 | `if err := windows.AssignProcessToJobObject(...); err != nil { _ = windows.CloseHandle(h); masterJobErr = fmt.Errorf("AssignProcessToJobObject(self): %w", err); return }` | A.1/A.4 | EDGE | same pattern as #3 (CloseHandle discard + non-canonical prefix) | LOW | same as #3 | same as #3 | FOUND |
| 5 | proc_windows.go:120-130 | `func setupProcessGroup(cmd *exec.Cmd) { _ = EnsureMasterJob() }` | A.1 | OK | **§S3 documented soft-fail** — comment lines 121-130 explicitly explain why error is discarded ("Best-effort. If the job init failed... we still proceed — the spawn just won't get the catastrophic-cleanup safety net. Service.Shutdown() (Layer A) and boot-time PID scan (Layer B) still work."). Inline-justified per §S3 spec. Acceptable. | N-A | — | — | — |
| 6 | proc_windows.go:140-149 | `func killProcessGroup(cmd *exec.Cmd) error { ...; out, err := exec.Command("taskkill", ...).CombinedOutput(); if err != nil { return fmt.Errorf("taskkill: %w (output: %s)", err, out) } }` | A.4 | EDGE | §S16: has `%w` ✓; prefix `taskkill:` (command name) not canonical pkg.Method. Cross-platform shape: proc_darwin/linux use bare syscall passthrough, proc_windows wraps because exec.Command err needs context (taskkill output captures useful failure detail). | LOW | inconsistency vs proc_darwin/linux which bare-return syscall err — harder to grep call sites consistently | tighten: `fmt.Errorf("sandbox.killProcessGroup: taskkill: %w (output: %s)", err, out)` | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: sites #3, #4 (LOW EDGE — `_ = CloseHandle(h)` cleanup-after-Job-init-failure without inline comment, panic-path carve-out applies but spec says comment required)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is platform-specific Job Object setup; no DB writes; masterJob handle is process-lifetime cached state, not terminal write

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — no business ID generation

A.4 §S16 错误 wrap 格式:
  - violations: sites #2, #3, #4, #6 (LOW — descriptive Windows-API-name prefixes vs canonical `sandbox.<funcName>:`); sentinel chain preserved via %w throughout

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (uses stdlib + Windows-API errors)
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels

## Cross-platform consistency note

proc_darwin (LOC 54), proc_linux (LOC 56), proc_windows (LOC 150) follow consistent strategy at conceptual level (process-tree management) but diverge in error-wrapping conventions:

- proc_darwin / proc_linux: bare passthrough of syscall.Kill err at the lowest layer (conventional)
- proc_windows: wraps taskkill exec err with descriptive prefix (because taskkill captures stderr that adds value)

The per-platform divergence is **functionally appropriate** — Windows's exec-based taskkill genuinely needs the output captured, while unix's `syscall.Kill` is a self-contained errno. The wrapping inconsistency is noise (descriptive prefix without pkg.Method qualifier) but not a defect.
