# D2 — sandbox.md ↔ code gap report

Audited `documents/version-1.2/service-design-documents/sandbox.md` against `internal/{domain,app,infra/sandbox,infra/store/sandbox}/sandbox/` + `transport/httpapi/handlers/sandbox.go` + `cmd/server/main.go::registerSandboxStack`.

D1 already covered the missing 12 sandbox HTTP routes in api-design.md and the schema/index gaps in database-design.md; this report focuses on the design-doc-vs-code drift specific to `sandbox.md`.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `sandboxdomain.ErrInvalidOwnerID` sentinel — owner.ID PATH-meta guard | `internal/domain/sandbox/sandbox.go:191` | MED |
| `sandboxdomain.ErrCmdRequired` sentinel — Spawn empty-Cmd path | `internal/domain/sandbox/sandbox.go:199` | MED |
| `Env.RunningPID` + `Env.RunningStartedAt` columns (Layer B leak prevention) | `internal/domain/sandbox/sandbox.go:104-105` | HIGH |
| Repository methods `SetEnvRunningPID` / `ClearEnvRunningPID` / `ListEnvsWithRunningPID` (Layer B) | `internal/domain/sandbox/sandbox.go:245-247` + `infra/store/sandbox/sandbox.go:317-363` | HIGH |
| Repository method `UpdateRuntime` (added) | `internal/domain/sandbox/sandbox.go:228` | LOW |
| Service method `RestoreOrCleanupOnBoot` (Layer B boot scan) | `internal/app/sandbox/restore.go:51` | HIGH |
| Service method `Shutdown` (Layer A graceful exit handle kill) | `internal/app/sandbox/spawn.go:142` | HIGH |
| Service method `GetEnv(ctx, id)` (used by handler) | `internal/app/sandbox/sandbox.go:309` | LOW |
| Service method `DeleteRuntime(ctx, id)` (used by handler) | `internal/app/sandbox/sandbox.go:321` | LOW |
| Service method `SandboxRoot()` / `MiseBin()` accessors used by main | `internal/app/sandbox/sandbox.go:163,169` | LOW |
| Layer A activeHandles tracking (`trackedHandle`, `nextHandleID`) | `internal/app/sandbox/spawn.go:296-337` + `sandbox.go:128-129` | HIGH |
| Service field `notif notificationspkg.Publisher` + `publishEnv` / `publishEnvDeleted` notifications on every env state transition (status=installing/ready/failed/destroyed) | `internal/app/sandbox/sandbox.go:84,539,574,610,639,657,678` | HIGH |
| Codesign helper for darwin embed extraction | `internal/infra/sandbox/codesign.go` | LOW |
| `RunWithStderrCapture` shared helper | `internal/infra/sandbox/exec_helper.go` | LOW |
| Per-platform `proc_*.go` for cross-platform process kill semantics | `internal/infra/sandbox/proc_{darwin,linux,windows}.go` | LOW |
| `MiseGlobalConfigPath` + `writeMiseConfig` (disables every mise attestation backend) | `internal/infra/sandbox/mise.go:159,181` | MED |
| Service.EnsureEnv uses `_` separator for owner.ID instead of doc's `:` | `internal/app/sandbox/sandbox.go:479` (rejects `:`) + handler `convID + "_" + kind` | HIGH |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| §4 EnvManager matrix declares "原 11 EnvManager 矩阵已删除（2026-05-08）" but §5b (Docker) + §7 v1 ship list (Playwright/Dotnet/Static/Generic + 6 mise installers) + §15 main.go example all still describe these as live | sandbox.md:132-143 vs §5b:286-347 / §7:448-540 / §15:1066-1078 | HIGH |
| §5 Sentinel error block lists `ErrDockerNotInstalled` + `ErrDockerDaemonDown` — code keeps them but registers no Docker installer/EnvManager and removed all Docker handling | sandbox.md:265-281 vs `infra/sandbox/` (no docker*.go) | HIGH |
| §5b "Docker runtime" entire section (DockerInstaller / DockerEnvManager / BuildDockerRunArgs helper / V2 marketplace integration) — no implementation exists | sandbox.md:286-347 | HIGH |
| §7 Installer table claims Playwright / Dotnet / Static-binary installers and 11 EnvManagers | sandbox.md:455-477 | HIGH |
| §7 D2-3 implementation order claims "D2-3a (已完成)" through D2-3f for all 11 EnvManagers | sandbox.md:478-489 | HIGH |
| §7 main.go装配示例 registers 13 installers + 11 env managers | sandbox.md:493-540 | HIGH |
| §8 Service signatures: doc shows `func (s *Service) GCEnvs(...)`, code is `GC(...)` | sandbox.md:590 vs `app/sandbox/sandbox.go:350` | MED |
| §8 Service signatures: doc shows `func (s *Service) Spawn(...)` not declaring it returns `LongLivedHandle` separately; code splits as `Spawn` + `SpawnLongLived` (matches eventually but doc table doesn't pair the SpawnOpts.LongLived → use SpawnLongLived requirement) | sandbox.md:580-581 | LOW |
| §9.2 Per-language isolation table includes Rust / Java / Go / Ruby / PHP / Dotnet rows | sandbox.md:728-737 | MED |
| §9.5 conversation owner.ID convention written as `"<conv_id>:<runtime_kind>"` (with `:` separator). Code rejects `:` via ErrInvalidOwnerID and substitutes `_` separator | sandbox.md:218-219, 783-792, 807 vs `app/sandbox/sandbox.go:479` + `transport/httpapi/handlers/sandbox.go:167,317` | HIGH |
| §9.5 Bash auto-route example pseudocode references `b.sandbox.SpawnShell(ctx, owner, cmd)` and `EnvBinDirs` method on EnvManager | sandbox.md:817-887 vs no `SpawnShell` / `EnvBinDirs` in code (Bash uses `b.sandbox.Spawn` with pre-resolved env+PATH); EnvManager interface has `EnvBin` not `EnvBinDirs` | MED |
| §9.5 detectRuntime examples include `dotnet`, `rust`, `go`, `ruby`, `php`, `java` regex rows — doc claims only python+node sandbox stack | sandbox.md:830-840 (matches actual `bash_route.go:60-67` but is inconsistent with §4 V3 stack which only registers python+node) | MED |
| §10 SSE 事件 — 不新增: claims sandbox does not emit independent SSE events; code emits `sandbox_env` notifications on every env state transition (installing/ready/failed/destroyed) | sandbox.md:945-961 vs `app/sandbox/sandbox.go:539,574,610,639` | HIGH |
| §11 HTTP API table missing `GET /api/v1/sandbox/envs/{id}` (single env detail) — present in code as h.GetEnv | sandbox.md:973 (listed in 11.2 but the route table at 967-998 includes it; double-checking — yes it's listed once) | — |
| §13 Test coverage table lists `internal/infra/sandbox/installer/mise/mise_test.go` (subdirectory) — actually flat at `infra/sandbox/mise_test.go` per §S12 平铺 (not subdir) | sandbox.md:1023 | LOW |
| §13 Test files for playwright/envmanager 各语言 — none of these exist | sandbox.md:1024-1026 | MED |
| §14 与其他 domain 的关系 row "events" mentions install progress goes through chat.message tool_call | sandbox.md:1044 — actually now goes via `installprogresspkg.Run` which writes to ctx eventlog Emitter (correct end behaviour) but described differently in doc | LOW |
| §15.1 BootstrapStatus signature `BootstrapStatus() (ok bool, err string)` — code uses separate `IsReady() bool` + `BootstrapError() error` | sandbox.md:1127 vs `app/sandbox/sandbox.go:176,184` | MED |
| §17 Cross-platform claim "Java MCP server: ... mvn/gradle wrapper 路径可能有坑" implies Java runtime support — but Marketplace V3 dropped this | sandbox.md:1216 | LOW |

## Mismatched

| Item | Code | Doc | Severity |
|---|---|---|---|
| Number of installers registered in main | 3 (python, node, uv) | 13+ (python/node/rust/java/go/ruby/php/uv/pnpm/maven/bundler/composer/playwright/dotnet/static-binary) | HIGH |
| Number of EnvManagers registered in main | 2 (Python, Node) | 11 (Python/Node/Rust/Go/Java/Ruby/PHP/Dotnet/Static/Playwright/Generic) | HIGH |
| Bootstrap step description: doc §1 + §2 mention `~/.forgify/sandbox/bootstrap/mise` etc. | Code extracts to `<dataDir>/sandbox/bin/mise` | sandbox.md:16,29 vs `infra/sandbox/mise.go:70` | LOW |
| Sentinel count | 11 (incl. ErrInvalidOwnerID + ErrCmdRequired + ErrDockerNotInstalled + ErrDockerDaemonDown) | 8 listed in §5 (with ErrDocker* added but ErrInvalidOwnerID + ErrCmdRequired absent) + 8 in §12 errmap table | MED |
| §12 error code table lists 8 Sentinel rows (no Docker, no InvalidOwnerID, no CmdRequired) | errmap has 11 sandbox sentinels (incl. SANDBOX_INVALID_OWNER_ID + SANDBOX_CMD_REQUIRED) | sandbox.md:1004-1014 vs `errmap.go:103-113` | MED |
| RuntimeInstaller signature: doc shows `Install(ctx, version, sandboxRoot, stream) (relPath, err)` | Matches | sandbox.md:398 vs `installer.go:29` | — |
| EnvManager method `EnvBin(envPath, binName) string` | Matches | sandbox.md:441 vs `installer.go:85` | — |
| §4 default versions: Python 3.12.x / Node 22.x | main: `python: "3.12"` / `node: "22"` (resolves to current patch via mise) | matches conceptually | — |
| §4 uv pin claim "0.11.4" (doc footnote) | Matches `main.go:524` (with rationale comment) | matches | — |
| §15 装配 sequence numbered 1-8: "5. NEW: subagentapp / mcpapp / skillapp / catalogapp" | actual main.go ordering similar but registerSandboxStack runs after Bootstrap (matches §15.1 degraded mode design) | matches conceptually | LOW |
| §10 ProgressFunc signature `ProgressFunc func(stage, message string, percent int)` | Matches | sandbox.md:170 vs `domain/sandbox/sandbox.go:171` | — |
| §6 Repository.DeleteRuntime — code adds via Service.DeleteRuntime which checks ErrEnvInUse first | Matches | sandbox.md:361 vs `app/sandbox/sandbox.go:321` | — |

## Sub-check
- Entities aligned: **partial** — Runtime/Env structs match field names + GORM tags + indexes; but design omits running_pid + running_started_at columns and doesn't reflect status CHECK constraint (`check:status IN (...)` is in code but not doc §5)
- Service methods aligned: **no** — ~5 methods in code missing from doc (Shutdown / RestoreOrCleanupOnBoot / GetEnv / DeleteRuntime / SandboxRoot / MiseBin / EnsureTool); doc names `GCEnvs` ≠ code `GC`
- Endpoints aligned: yes (all 12 routes in handler are mentioned in §11; D1 covered the api-design.md gap, not relevant here)
- Sentinels aligned: **no** — 3 sentinels missing from doc §5/§12 (ErrInvalidOwnerID, ErrCmdRequired); errmap has 11 sandbox entries vs doc's 8
- Cross-domain deps aligned: yes — forge/mcp via PluginSandbox interface; Bash auto-route; conversation env on hard-delete (skill not yet wired but doc §14 says "v1 not needed" which matches)
- 端到端推演 valid: **no** — §2 "运行期 - 用户装 Playwright MCP" still describes Playwright + Chromium browsers extras flow; §2 "Forge 跑代码" + "MCP server 长生命周期 spawn" are conceptually correct but mention `envs/forge/<envid_A>/.venv` etc. which match
- Phase 5 / V3 / Layer A+B 大变更已反映: **no** — Layer A/B leak prevention work (Shutdown / boot scan / running_pid manifest columns) entirely absent from doc; sandbox_env notification publishing absent; V3 npm+pypi-only collapse left §4 statement-only with §5b/§7/§15 still describing pre-V3 state

---

## Summary

- HIGH: 9 (V3 collapse not propagated past §4 / Docker section / 11-installer wiring / 11-EnvManager wiring / fork stale; Layer A+B columns + methods stale; sandbox_env notification unmentioned; conversation owner.ID separator mismatch `_` vs `:`; SSE-not-emitted claim wrong)
- MED: 7 (ErrInvalidOwnerID/ErrCmdRequired sentinels missing from §5/§12; mise config write helper undocumented; GC method name; BootstrapStatus signature; per-language isolation table stale; SpawnShell/EnvBinDirs reference to nonexistent code; doc §11 OK)
- LOW: 8 (test layout subdir vs flat; misc helpers undocumented; method-pair description gaps; bootstrap path string drift; etc.)
