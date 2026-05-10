# Package audit summary: transport/httpapi/handlers (B3 — 3 medium-large files)

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: B3 3 文件中 dev_mock_llm.go / sandbox.go **0 violation**。skills.go **1 MED** (site 11 multipart read loop 静默 break non-EOF io err，silent fallback 把截断 buf 当合法 SKILL.md 入 service)。其他全 EDGE-LOW（dev-only graceful / silent fallback by design）。
- **§S9 detached ctx 终态写**: B3 3 文件 **0 violation at handler layer; 11 EDGE-FLAGS 转 sandbox-app + skillapp audit batches**——所有终态写候选 (sandbox: envAction/runtimeAction/gc/installRuntime/convEnvKindAction/convEnvsAction; skill: Create/Replace/Delete/Import/Activate) handler 层全部 r.Context() 透传 service。**这是 §S9 在 transport 层的标准姿态**——detached 责任在 app 层 (与 chat SendMessage 同模式)。**任务描述担忧"sandbox install 是 long-running，可能起 fire-and-forget goroutine——需 detached ctx" → handler 层不预设 detached 是 OK 的，转 sandbox-app audit batch**。
- **§S15 ID 生成**: B3 3 文件 **0 violation**——dev_mock_llm 不生成 ID；sandbox handler 透传 r.PathValue 用 service 返回值；skill 用人类可读 name 作主键（§S15 例外，skill.md 设计选择）。
- **§S16 错误 wrap 格式**: B3 **8 EDGE-LOW**——dev_mock_llm site 2/3（无 prefix + inline JSON decode）；sandbox 0；skills site 5/11/11c/13/15（**全部源自 decodeJSONLimit 单一 helper**）。**HIGH-ROI cross-cutting fix**：重构 skills.go::decodeJSONLimit 复用 apikey.go::decodeJSON pattern (§S16 `handlers.decodeJSONLimit: %w` + joinInvalidRequest)，5 处 inline INVALID_REQUEST 自动消除。
- **§S17 errmap 单一事实源**: B3 3 文件 **0 violation**——dev_mock_llm 0 sentinel 消费；sandbox 10/12 sandboxdomain sentinel 全登记 (剩余 2 是 ErrDockerNotInstalled/ErrDockerDaemonDown，0 消费者，Phase 5 docker 预留——**pre-registration 候选 LOW**)；skill 5/5 skilldomain sentinel 全登记。

## Files audited

| File | LOC | Sites | OK | EDGE | VIOLATION |
|---|---|---|---|---|---|
| dev_mock_llm.go | 250 | 14 | 12 | 2 | 0 |
| sandbox.go | 364 | 17 | 9 | 8 | 0 |
| skills.go | 384 | 16 | 6 | 9 | 1 (MED) |
| **TOTAL** | **998** | **47** | **27** | **19** | **1** |

> EDGE-classified rows surfaced as LOW severity; 1 MED on skills.go site 11 (multipart silent break).

## Severity breakdown

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 1 | skills.go site 11 (multipart read loop 静默 break non-EOF io err) |
| LOW | 18 | dev_mock_llm site 2/3; sandbox site 2/7/8/10/11/13/13b/15; skills site 5/7/8/9/11b/11c/11e/13/14/15 |

**Net: 0 HIGH / 1 MED / 18 LOW (mostly EDGE on §S6 wire-code ad-hoc + §S9 transport-layer pass-through)**

## Status (post-fix)

| site | severity | status | commit |
|---|---|---|---|
| skills.go site 11 (multipart silent break) | MED | FIXED | this batch — io.ReadAll 替代手卷 loop |
| skills.go site 5 / 11c / 13 (decodeJSONLimit handler 调用) | LOW | FIXED | this batch — 4 处改 FromDomainError |
| skills.go site 15 (decodeJSONLimit helper) | LOW | FIXED | this batch — `handlers.decodeJSONLimit:` 前缀 + joinInvalidRequest |
| skills.go site 11 (open part err 缺 prefix) | LOW | FIXED | this batch — 加 `handlers.Import:` 前缀 |
| skills.go site 11b (inline f.Close() vs defer) | LOW | WAIVED | 改用 io.ReadAll 后 Close 紧跟，path linear；GC finalize 兜底 |
| skills.go site 7/8/9/11e/14 (终态写 §S9 EDGE) | LOW | EDGE-FLAG | 转 skillapp audit batch（handler 层 r.Context() 透传是 §S9 标准姿态） |
| sandbox.go ErrDockerNotInstalled/DaemonDown | LOW | FIXED | this batch — pre-register errmap 行（Phase 5 docker 接入零摩擦） |
| sandbox.go site 2/7/8/10/11/13/13b/15 (主要 EDGE-FLAG) | LOW | EDGE-FLAG | 同 §S9 — 转 sandbox-app audit batch |
| dev_mock_llm.go site 2 (fmt.Errorf 缺 prefix) | LOW | FIXED | this batch — 加 `handlers.toStreamEvent:` 前缀 |
| dev_mock_llm.go site 3 (inline JSON decode 未复用 decodeJSON) | LOW | FIXED | this batch — 改用 decodeJSON helper |

## Cross-cutting

### 1. decodeJSONLimit refactor — HIGH-ROI cross-cutting fix

skills.go::decodeJSONLimit (site 15) 与 apikey.go::decodeJSON (B2 已修) 是**两个独立 helper 走两条不同错误路径**：

- `decodeJSON` (apikey.go:217): `return fmt.Errorf("handlers.decodeJSON: %w", joinInvalidRequest(err))` — §S16 合规，handler 用 FromDomainError 透传
- `decodeJSONLimit` (skills.go:373): 裸 errors.New ("request body exceeds size limit") + `return err` — §S16 prefix 缺，handler 必须 inline `INVALID_REQUEST` 字面量

**1-helper 修复消除 5+ handler 路径的 §S16 不一致**：

```go
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, maxBytes int64, out any) error {
    body := http.MaxBytesReader(w, r.Body, maxBytes)
    dec := json.NewDecoder(body)
    if err := dec.Decode(out); err != nil {
        var maxBytesError *http.MaxBytesError
        if errors.As(err, &maxBytesError) {
            return fmt.Errorf("handlers.decodeJSONLimit: request body exceeds %d bytes: %w", maxBytes, joinInvalidRequest(err))
        }
        return fmt.Errorf("handlers.decodeJSONLimit: %w", joinInvalidRequest(err))
    }
    return nil
}
```

handler 5 处调用 `responsehttpapi.Error(w, ..., "INVALID_REQUEST", err.Error(), nil)` 改用 `responsehttpapi.FromDomainError(w, h.log, err)`——5 处 inline 字面量 wire code 全部消除。**强烈建议合入下批 fix**——B2 _summary 已识别 decodeJSON 是 cross-cutting，本文件是其延伸。

### 2. Multipart silent break — MED skills.go site 11

唯一非 EDGE 的 §S3 violation 候选。skills.go:280-288 multipart 读取 loop：

```go
for {
    n, rerr := f.Read(tmp)
    if n > 0 { buf = append(buf, tmp[:n]...) }
    if rerr != nil { break }   // ❌ io.EOF 与真错不区分
}
```

io.EOF 是合法终止（必 break），但 disk fail / connection drop 等非 EOF io err 也被静默 break——把截断 buf 当合法 SKILL.md 入 service。service 层 frontmatter parse 兜底报错（不会落库错的内容），但**调试时丢线索**——errmap 报"INVALID_FRONTMATTER" 误导。

**1 行修复**：

```go
if errors.Is(rerr, io.EOF) { break }
if rerr != nil {
    responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
        "read part "+fh.Filename+": "+rerr.Error(), nil)
    return
}
```

或更简洁地直接用 `io.ReadAll(f)` 替代手卷 loop（一行 + 错误 explicit）。

### 3. Sandbox 终态写 EDGE-FLAG — sandbox-app audit batch 跟踪

sandbox.go 6 处终态写 (envAction/runtimeAction/gc/installRuntime/convEnvKindAction/convEnvsAction) 全部 r.Context() 透传——detached ctx 责任在 sandbox-app service。**任务描述明示** "sandbox install 是 long-running, 可能起 fire-and-forget goroutine——需 detached ctx"——**handler 层 OK**，转 sandbox-app audit batch 检查：

- `sandboxapp.Service.EnsureRuntime` 是否在 detached ctx 跑 mise install + manifest 终态写双保？
- `sandboxapp.Service.Destroy` 是否分离 manifest 标记 vs disk rm 的 commit 时序？
- `sandboxapp.Service.GC` 是否每 env 独立 tx + 中途 cancel 不留半完成？

handler 层 r.Context() 透传是合理的中间层选择（用户主动 cancel 透传给 service），detached 责任不应预设在 transport——这是 §S9 标准姿态。

### 4. Skill 终态写同 sandbox 模式

skills.go 5 处终态写 (Create/Replace/Delete/Import/Activate) 全部 r.Context() 透传——同 sandbox.go pattern。skill 文件系统 IO 比 sandbox install 短得多（几 KB 写），cancel 风险量级显著较小，但严格 §S9 跟 sandbox 同 EDGE-FLAG 转 skillapp audit batch。

### 5. Wire code ad-hoc 词汇 (跨文件)

sandbox.go 自创 `OWNER_KIND_REQUIRED` (site 2) / `KIND_REQUIRED` (site 13b) / `UNKNOWN_ACTION` (site 7b/9)；dev_mock_llm.go `TRACER_DISABLED` (site 11)。**判定**：合规 §S6（handler 业务校验自创 wire code 是允许的），但项目层面应有约定——是统一收编到 errmap 的 sentinel？还是允许 handler 字面量？**当前两种风格混用**（B1/B2/B3 都观察到），建议在 `service-contract-documents/error-codes.md` 加节说明哪些 wire code 是 sentinel-driven、哪些是 handler-local。**LOW 全文件**——非 §S17 严格违规。

### 6. Sandbox docker sentinel pre-registration 候选

`ErrDockerNotInstalled` (sandbox.go:206) / `ErrDockerDaemonDown` (sandbox.go:213) 已声明但 0 消费者——Phase 5 docker 接入预留。**§S17 严格说**：未到达 handler 路径的 sentinel 不必登记。**判定 OK**——但**Phase 5 风险**：未来 docker runtime kind 接入时若忘加 errmap 行，会触发"unmapped domain error" 警告。

**建议同步登记**（成本极低 2 行 errmap.go，收益 Phase 5 接入零摩擦）：

```go
sandboxdomain.ErrDockerNotInstalled: {http.StatusUnprocessableEntity, "SANDBOX_DOCKER_NOT_INSTALLED"},
sandboxdomain.ErrDockerDaemonDown:   {http.StatusServiceUnavailable, "SANDBOX_DOCKER_DAEMON_DOWN"},
```

非强制（当前 0 路径触达，严格 §S17 不要求），但**HIGH-ROI suggestion**。

### 7. Skill activate "SubagentService is nil" unmapped path (LOW)

skill.go site 14 标识：activate.go:104 `"fork requested but SubagentService is nil"` 是无 sentinel 内部错——会触发 errmap 的"unmapped domain error" 日志告警。**严格 §S17 不算违规**（不是 sentinel），但产线触达污染日志。建议 activate.go 内部用 errorsdomain.ErrInternal wrap 或加新 `skillapp.ErrSubagentUnavailable` sentinel + errmap 行。**LOW**——非本批 fix 范围（属 skillapp audit batch）。

### 8. dev handler skill — 与 B2 dev_info.go 风格统一

dev_mock_llm.go 与 B2 dev_info.go 同属 `--dev` gated，但**最干净的 dev handler 之一**——10 endpoints 全 thin (~10-30 LOC)，0 §S3 EDGE 集中。task spec 担忧的"§N7 SSE wire format"不适用——本 handler 0 SSE 端点。

### 9. handler 层 §S6 总体一致性

B3 3 文件中 sandbox.go 14 endpoint + skills.go 9 endpoint + dev_mock_llm 5 endpoint = **28 endpoints 全部薄 handler**（解 JSON → 调 service → 写 envelope）。唯一例外：skills.go::Import (~70 LOC) — multipart 处理 + JSON 双路径，但 multipart 文件解析是天然 transport 层职责（§S6 例外明示，同 B2 dev_info.go walkHomeTree 推理），合规。

## Recommended fix priorities

按 §S20 + §S14 优先级 — 本批 **0 HIGH / 1 MED / 18 LOW**：

1. **skills.go site 11 (multipart silent break) — MED**：1 行修 io.EOF 区分 / 或换 io.ReadAll。**§S20 不留下次**——可触发 (disk fail / network drop) 即应修。
2. **skills.go site 15 (decodeJSONLimit refactor) — HIGH-ROI cross-cutting**：1-helper 修复消除 5+ handler §S16 不一致 + 5 处 inline INVALID_REQUEST 字面量。**强建议**合入下批 fix（B2 已修 decodeJSON 的延伸）。
3. **sandbox.go ErrDockerNotInstalled/DaemonDown pre-registration — LOW HIGH-ROI**：2 行 errmap 行——Phase 5 docker 接入零摩擦。**建议但非必需**。
4. **dev_mock_llm.go site 2/3 — LOW**：dev-only 风格收齐（site 3 改用 decodeJSON helper），与 site 15 同批顺手修。
5. **sandbox.go site 10 (gc Atoi silent fallback) — LOW**：1 行 inline 注释钉住"silent fallback by design"避免后续审查误判。
6. **partial-success UX (sandbox.go site 15)**：errors.Join 多 error 返 wire body——产品层面待 PM 决定，非 §S20 强制。

## Out-of-scope notes

1. `_test.go` 文件按 fork 约束未读。
2. `sandboxapp.Service.EnsureRuntime/Destroy/DeleteRuntime/GC/RetryBootstrap/IsReady` 内部 detached ctx + mise process 管理 + manifest 落库 — 属 sandbox-app audit batch。
3. `skillapp.Service.Create/Replace/Delete/Import/Activate/Scan/Body` 内部 detached ctx + 文件系统 IO + Watcher rescan + subagent fork — 属 skillapp audit batch。
4. `skillapp.Scan` 内部 per-file parse 错处理 (`scan.go:153-162`) — handler 透传 batch-level err 合规，但 service 层 silent skip 单文件错的设计 (Watcher 模式) 待 skillapp audit 验证。
5. `cmd/server` DI / `main.go` log nil-defense 一致性 (skills.go:60 vs 其他 handler) — 属 cmd-server audit batch。
6. `internal/llminfra` MockClient + Tracer 内部并发安全 + buffer 管理 — 属 infra-llm audit batch。
