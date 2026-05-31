# File audit: backend/internal/pkg/agentstate/agentstate.go

LOC: 131. AgentState struct + SeenFiles 跟踪 + Cwd 跟踪 + SubagentTokens 累积。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | agentstate.go:21 | `SeenFiles sync.Map // string → int64` | A.1 | OK | sync.Map 自带并发安全；godoc 行 12 显式承诺"Methods are concurrency-safe"。Store/Load 不返 error。 | — | — | — | — |
| 2 | agentstate.go:25-26 | `cwdMu sync.Mutex`<br>`cwd   string` | A.1 | OK | 私有 Mutex 保护 cwd 字段；godoc 行 23 说"empty = 'use process cwd'"——零值合法，不需 sentinel。 | — | — | — | — |
| 3 | agentstate.go:37-38 | `subTokensMu sync.Mutex`<br>`subTokens   []SubagentTokenEntry` | A.1 | OK | Mutex 保护 slice 增长；godoc 行 36 说"sibling 并发 sub-run 按 RunID 隔离；mutex 串化 slice 增长"。 | — | — | — | — |
| 4 | agentstate.go:73-79 | `func (s *AgentState) AddSubagentTokens(runID, typeName string, in, out int) {`<br>`	s.subTokensMu.Lock()`<br>`	defer s.subTokensMu.Unlock()`<br>`	s.subTokens = append(s.subTokens, SubagentTokenEntry{`<br>`		RunID: runID, TypeName: typeName, TokensIn: in, TokensOut: out,`<br>`	})`<br>`}` | A.1 / A.2 | OK | **任务 prompt 关注点**: 并发安全。Lock/Unlock 标准 pattern；defer 保证 panic 路径解锁。无错误路径——append 永不 fail（slice 增长由 runtime 处理 OOM panic）。**注意 A.2**：subagentapp.Service 调本方法时是终态写（sub-run 终结后写 token 累计），但本方法是**纯内存写**（不写 DB）——不在 §S9 "终态落库"语义内。**调用方该如何派生 ctx 不是本包责任**。 | — | — | — | — |
| 5 | agentstate.go:85-91 | `func (s *AgentState) SubagentTokenLog() []SubagentTokenEntry {`<br>`	s.subTokensMu.Lock()`<br>`	defer s.subTokensMu.Unlock()`<br>`	out := make([]SubagentTokenEntry, len(s.subTokens))`<br>`	copy(out, s.subTokens)`<br>`	return out`<br>`}` | A.1 | OK | **并发安全 read**: 持锁 + 拷贝（不 alias），调用方可 lock-free 遍历返回值。godoc 行 81-83 显式说明设计意图。 | — | — | — | — |
| 6 | agentstate.go:96-98 | `func (s *AgentState) MarkRead(path string, size int64) {`<br>`	s.SeenFiles.Store(path, size)`<br>`}` | A.1 | OK | sync.Map.Store——无错误返回，并发安全。 | — | — | — | — |
| 7 | agentstate.go:105-111 | `func (s *AgentState) WasRead(path string) (int64, bool) {`<br>`	v, ok := s.SeenFiles.Load(path)`<br>`	if !ok {`<br>`		return 0, false`<br>`	}`<br>`	return v.(int64), true`<br>`}` | A.1 | EDGE | type assertion `v.(int64)` **未检查 ok**——直接断言。若 sync.Map 里被错误存入非 int64（程序员 bug），会 panic 而非返 (0, false)。但**这是符合 §S15-style "fail-loud" 的设计**：MarkRead (site#6) 是唯一写入点，签名强制 int64，违反此契约的 Store 调用应让程序立刻崩。归 EDGE 因为单看本 site 像 §S3 风险，但全局看是 fail-loud 设计选择。 | LOW | — | — | — |
| 8 | agentstate.go:116-120 | `func (s *AgentState) Cwd() string {`<br>`	s.cwdMu.Lock()`<br>`	defer s.cwdMu.Unlock()`<br>`	return s.cwd`<br>`}` | A.1 | OK | 经典 Mutex 读 pattern——defer Unlock。 | — | — | — | — |
| 9 | agentstate.go:126-130 | `func (s *AgentState) SetCwd(path string) {`<br>`	s.cwdMu.Lock()`<br>`	defer s.cwdMu.Unlock()`<br>`	s.cwd = path`<br>`}` | A.1 | OK | 经典 Mutex 写 pattern。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - notes: site#7 的 type assertion 未带 ok-check 是 fail-loud 设计选择（程序员 bug 不应静默），非违规

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 无（本包**纯内存**——sync.Map / mutex-protected slice / mutex-protected string，不涉及 DB / 网络 / 事件）
  - 各自 ctx 来源: N/A
  - violations: N/A: package doesn't do terminal writes (in-memory state only; persistent token data is written by app/subagent.Service to DB, which is what would carry the §S9 detached-ctx burden — out of this package's scope)
  - 备注: site#4 AddSubagentTokens 看似"终态写"但只是 in-memory append——真正 §S9 终态写在调用本方法的 app/subagent.Service 中（写 DB 时），那才是审 app/subagent 时的关注点

A.3 §S15 ID 生成:
  - ID generation calls: 无
  - violations: N/A: package doesn't generate business IDs (RunID / TypeName 是参数传入，由上游 idgen.New 生成)

A.4 §S16 错误 wrap 格式:
  - violations: not present (本文件无 error 返回值)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: 无
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels
