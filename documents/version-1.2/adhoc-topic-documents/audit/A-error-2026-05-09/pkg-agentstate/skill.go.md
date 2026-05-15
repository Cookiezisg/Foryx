# File audit: backend/internal/pkg/agentstate/skill.go

LOC: 211. ActiveSkill 旁路 + atomic.Pointer + skill-allowed-tools 模式匹配（matchAllowedTool / wildcardMatch）。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | skill.go:43-45 | `func (s *AgentState) SetActiveSkill(skill *skilldomain.Skill) {`<br>`	s.activeSkill.Store(skill)`<br>`}` | A.1 | OK | atomic.Pointer.Store——无错误，并发安全。godoc 行 35-42 显式说明 last-write-wins 设计（§9.5 race 良性）。 | — | — | — | — |
| 2 | skill.go:53-55 | `func (s *AgentState) ActiveSkill() *skilldomain.Skill {`<br>`	return s.activeSkill.Load()`<br>`}` | A.1 | OK | atomic.Pointer.Load——无错误，并发安全。godoc 行 47-52 警告 caller "treat returned pointer as read-only"。 | — | — | — | — |
| 3 | skill.go:63-68 | `func (s *AgentState) ClearActiveSkillIfMatches(name string) {`<br>`	cur := s.activeSkill.Load()`<br>`	if cur != nil && cur.Name == name {`<br>`		s.activeSkill.CompareAndSwap(cur, nil)`<br>`	}`<br>`}` | A.1 | OK | CAS pattern——若另一个 goroutine 在 Load 和 CAS 之间替换 activeSkill，CAS 失败但 ClearActiveSkillIfMatches 行为正确（"不被 stomped"）。godoc 行 57-62 显式说明并发交错场景。无错误返回。 | — | — | — | — |
| 4 | skill.go:78-89 | `func (s *AgentState) IsToolPreApprovedBySkill(toolName string, argsJSON []byte) bool {`<br>`	skill := s.activeSkill.Load()`<br>`	if skill == nil {`<br>`		return false`<br>`	}`<br>`	for _, pattern := range skill.Frontmatter.AllowedTools {`<br>`		if matchAllowedTool(pattern, toolName, argsJSON) {`<br>`			return true`<br>`		}`<br>`	}`<br>`	return false`<br>`}` | A.1 | OK | nil skill / 空 allowed-tools 都返 false——godoc 显式说明。无错误概念，纯 predicate。 | — | — | — | — |
| 5 | skill.go:101-132 | `func matchAllowedTool(pattern, toolName string, argsJSON []byte) bool {`<br>`	open := strings.IndexByte(pattern, '(')`<br>`	if open < 0 { return pattern == toolName }`<br>`	close := strings.LastIndexByte(pattern, ')')`<br>`	if close <= open {`<br>`		// Malformed pattern (open paren without matching close); treat as`<br>`		// non-match rather than panic — author bug should not collapse`<br>`		// permission gating.`<br>`		return false`<br>`	}`<br>`	...` | A.1 | OK | 畸形 pattern 处理——godoc 行 109-113 显式说明"author bug should not collapse permission gating"——返 false（不授权）是 fail-safe 设计：宁可拒授权也不 panic。**注意**：这看起来像 §S3 "把错误转成成功"反例，但**返 false = 拒授权 = 安全侧**——是 fail-safe，不是 fail-silent。授权 gating 失败的合理 fallback 是"拒"而非"准"，这与吞 user-visible 错误不同（如 ctx-cancel 后吞 DB write）。 | — | — | — | — |
| 6 | skill.go:140-152 | `func extractPrimaryArg(toolName string, argsJSON []byte) string {`<br>`	switch toolName {`<br>`	case "Bash":`<br>`		var args struct {`<br>`			Command string `+"`json:\"command\"`"+`<br>`		}`<br>`		if err := json.Unmarshal(argsJSON, &args); err != nil {`<br>`			return ""`<br>`		}`<br>`		return args.Command`<br>`	}`<br>`	return ""`<br>`}` | A.1 | EDGE | **JSON unmarshal 错误返 ""**——后续 matchAllowedTool 行 128 "if primary == "" { return false }" → 拒授权。同 site#5 是 fail-safe 设计（畸形 args → 拒）。**但**：本函数把 unmarshal err 完全吞掉、不 log——若 LLM 突然产生畸形 Bash args 会持续命中此分支，没有任何 telemetry 提示。**严格按 §S3**："严禁用静默跳过掩盖失败"——unmarshal err 这种"非应该"的 case，应至少 log。**对照 site#5 的差异**：site#5 是**已知 author bug 模式**（畸形 pattern 写在 skill 文件里），fail-safe 是设计意图；site#6 是 **runtime LLM 输入畸形**——更 unusual，应至少留 telemetry 痕迹。归 LOW EDGE 而非 violation 因为：(a) 调用方 `IsToolPreApprovedBySkill` 是 predicate（无 error 返回值的位置可挂 telemetry），(b) ToolDispatch 后续会用畸形 args 调真实 tool，那时 tool 自己的 ValidateInput 会报错，不致命；但这个静默吞仍是 §S3 灰区。 | LOW | 畸形 LLM Bash args 触发的"无授权"路径无 telemetry，调试时难发现 | extractPrimaryArg 第二个返回值返 error 或在 unmarshal 失败时调 logger.Debug — 但本函数没注入 logger，需要把 logger 加入 AgentState（侵入性较大）。退而求其次：内联注释明确"LLM 畸形 args 是 unusual case，期望发生频率为零；落 ValidateInput 路径报错"——把这层未表达的设计意图显式化 | FOUND |
| 7 | skill.go:169-199 | `func wildcardMatch(pattern, subject string) bool {`<br>`	parts := strings.Split(pattern, "*")`<br>`	...` | A.1 | OK | 纯字符串处理 predicate；无错误路径。godoc 例子覆盖边界。 | — | — | — | — |
| 8 | skill.go:210 | `type activeSkillSlot = atomic.Pointer[skilldomain.Skill]` | A.1 | OK | 类型别名——godoc 行 201-209 解释为何用 atomic.Pointer 而非 mutex。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: 1 LOW (site#6 — extractPrimaryArg 静默吞 json.Unmarshal err，无 telemetry)
  - notes:
    - site#5 / site#6 都是 fail-safe (返 false → 拒授权)，不会让用户看到错误状态——属设计意图
    - site#5 OK 因为是已知 author bug 模式（pattern 写错），符合"author bug should not collapse permission gating"显式注释
    - site#6 LOW 因为是 runtime LLM 输入，无 telemetry 时 LLM 模式漂移触发的拒授权会被误判为"skill 配置错"

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 无（atomic.Pointer 是纯内存原语，无 DB / 网络）
  - 违 violations: N/A: package doesn't do terminal writes (in-memory atomic state only)

A.3 §S15 ID 生成:
  - ID generation calls: 无
  - violations: N/A: package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (no error returns from any function in this file)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: 无
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels
