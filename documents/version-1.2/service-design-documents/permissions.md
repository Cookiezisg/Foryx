# Permissions + Hooks — V1.2 §3 final-sweep

**Phase**：V1.2 §3 final-sweep（接 §1 compaction / §2 memory 之后的 ship gate 第三块）
**状态**：📐 设计期（**等用户过审 → OK 后实现**）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../final-sweep.md`](../final-sweep.md) §3 —— 原始 9 子项需求
- [`./chat.md`](./chat.md) §S18 —— tool 接口现状（9 方法）
- [`./filesystem.md`](./filesystem.md) §6 —— 现有 PathGuard
- [`./shell.md`](./shell.md) §6 D5 —— Bash 故意不走 PathGuard 的决策（本设计要重审）

---

## 1. 一句话

**抄 Claude Code 的成熟权限模型**：tool 危险等级框架硬编码（不进 9 方法接口）+ `~/.forgify/settings.json` deny/ask/allow 第一匹配 + Pre/Post/Stop 3 个 hook 时机 + 默认 protected paths 写黑名单。**Forgify 单用户本地** 简化：跳过 managed-tier 层级，单一 settings 文件即可；hot reload settings 文件改即生效（Claude Code 不做但我们应做，单用户本地 UX 更好）。

---

## 2. 端到端推演（§5）

```
LLM 发起 tool_call ("Bash", {command: "rm -rf node_modules"})
  ↓
chat/tools.go::runTools 派发到 Bash tool
  ↓
─── 新增 PermissionGate（权限闸） ───
  ↓
1. 框架按 tool name 算 危险等级 → "danger" (Bash 硬编码)
  ↓
2. 加载 settings.json permissions 规则
  ↓
3. PreToolUse hooks 全跑（按 matcher 过滤）
  ↓
4. 评估顺序: deny → ask → allow → defaultMode
   • deny 第一匹配命中 → 422 BLOCKED_BY_RULE，emit tool_result error
   • ask 命中 → ask user via AskUserQuestion；用户拒 → BLOCKED；允 → 缓存到 session 跳到下一步
   • allow 命中 → 透过
   • 都未匹配 → defaultMode 决定（auto/ask/dontAsk）
  ↓
─── PathGuard（filesystem tools 专用，已有，不变）───
  ↓
─── Tool.Execute 真跑 ───
  ↓
─── 新增 PostToolUse hooks ───
  ↓
hooks 收到 {tool_name, tool_input, result}; 可在下轮 LLM context 注入 feedback
  ↓
tool_result 落 message_blocks 给 LLM 看
```

**端到端跨 domain 依赖**：
- `app/tool/permissionsgate`：新包，3 个子组件（rules / hooks / hardcoded levels）
- `chat/tools.go::runTools`：每个 tool 派发前调 gate
- `app/ask/Service`：复用现有的 AskUserQuestion 通道（hook ask 决策走 ask）
- `infra/settings`：新 infra 包，watch + parse `~/.forgify/settings.json`

---

## 3. 设计原则

| 原则 | 落地 |
|---|---|
| **抄成熟设计** | Claude Code 已是事实标准；语义照抄；只为 Forgify 单用户本地优化 |
| **Tool 接口不动** | 9 方法 + 3 标准注入字段（summary/destructive/execution_group）已稳；危险等级在框架查表 |
| **deny first** | deny 永不能被 allow 推翻；ask 即用户决策；都不匹配走 defaultMode |
| **第一匹配赢，不累加** | glob 命中即停（O(规则数) per dispatch，可接受）|
| **Hot reload** | 改 settings.json 立刻生效（fsnotify watch 1s 兜底 poll）—— **Claude Code 不做，单用户本地我们做** |
| **Hook = shell** | 第一形态只支持 shell stdin/stdout JSON 协议；HTTP / MCP 等 v2 再加 |
| **失败软退**| hook timeout / exit≠0/2 → log + continue（非 blocking）；只有 exit=2 才 blocking |
| **没有 managed-tier** | Claude Code 三层（managed / project / user），我们单层 `~/.forgify/settings.json` |
| **destructive=true 自动 ask** | LLM 自报 destructive 的 tool_call **强制走 ask**（即使 settings 没配规则，destructive 也提示）|

---

## 4. Tool 危险等级（框架硬编码）

**不改 9 方法接口**。新 `app/tool/permissionsgate/levels.go` 定一张静态表：

```go
type DangerLevel string

const (
    LevelReadOnly         DangerLevel = "read_only"          // 不提示
    LevelWorkspaceWrite   DangerLevel = "workspace_write"    // 首次 ask，session 内记
    LevelDangerFullAccess DangerLevel = "danger_full_access" // 每次 ask（除非 settings.allow）
)

// 静态查表，新加 tool 必须在此登记（compile-time 防遗漏可加单测）
var toolLevels = map[string]DangerLevel{
    // ── ReadOnly ──
    "Read": LevelReadOnly,
    "Glob": LevelReadOnly,
    "Grep": LevelReadOnly,
    "WebFetch": LevelReadOnly,
    "WebSearch": LevelReadOnly,
    "TaskList": LevelReadOnly,
    "TaskGet": LevelReadOnly,
    "AskUserQuestion": LevelReadOnly,
    "BashOutput": LevelReadOnly,
    "search_forges": LevelReadOnly,
    "get_forge": LevelReadOnly,
    "read_memory": LevelReadOnly,
    "search_function_executions": LevelReadOnly,
    "get_function_execution": LevelReadOnly,
    // ... 全部 readOnly tools

    // ── WorkspaceWrite ──
    "Edit": LevelWorkspaceWrite,
    "Write": LevelWorkspaceWrite,
    "TaskCreate": LevelWorkspaceWrite,
    "TaskUpdate": LevelWorkspaceWrite,
    "write_memory": LevelWorkspaceWrite,
    "forget_memory": LevelWorkspaceWrite,
    "create_forge": LevelWorkspaceWrite,
    "edit_forge": LevelWorkspaceWrite,
    // ...

    // ── DangerFullAccess ──
    "Bash":        LevelDangerFullAccess,
    "KillShell":   LevelDangerFullAccess,
    "run_function": LevelDangerFullAccess,
    "call_handler": LevelDangerFullAccess,
    "trigger_workflow": LevelDangerFullAccess,
    // sandbox-bypassing tools + remote-effecting tools
}

func LookupLevel(toolName string) DangerLevel {
    if l, ok := toolLevels[toolName]; ok {
        return l
    }
    // 未知 tool（如 MCP 动态注入）默认 WorkspaceWrite（保守）
    return LevelWorkspaceWrite
}
```

**与现有 `Tool.IsReadOnly()` 的关系**：`IsReadOnly()` 当前只是文档（不驱动并发，并发由 execution_group 字段决定）。可以让 `LookupLevel` 在未登记时 fall back 到 `IsReadOnly()` —— `true` → ReadOnly，`false` → WorkspaceWrite。MCP 动态 tool 走此路径。

---

## 5. Settings 文件 schema

`~/.forgify/settings.json`：

```json
{
  "permissions": {
    "defaultMode": "ask",
    "deny": [
      "Bash(rm -rf *)",
      "Bash(curl http*)",
      "Read(.env)",
      "Edit(.env)",
      "Edit(.git/**)",
      "Write(.git/**)"
    ],
    "ask": [
      "Bash(git push *)",
      "Bash(npm publish)",
      "Write(~/**)"
    ],
    "allow": [
      "Bash(npm:*)",
      "Bash(yarn:*)",
      "Bash(git status)",
      "Bash(git log)",
      "Bash(git diff)",
      "Read(./**)",
      "Edit(./src/**)",
      "Glob(./**)",
      "Grep(./**)"
    ]
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "command": "~/.forgify/hooks/bash-guard.sh",
        "timeout": 10
      },
      {
        "matcher": "Edit|Write",
        "command": "~/.forgify/hooks/file-guard.py",
        "timeout": 10
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "command": "~/.forgify/hooks/maybe-run-tests.sh",
        "timeout": 30
      }
    ],
    "Stop": [
      {
        "command": "~/.forgify/hooks/check-todos-empty.sh",
        "timeout": 5
      }
    ]
  },
  "protectedPaths": {
    "denyWrite": [
      ".git/**", ".env", ".envrc", "node_modules/**", "~/.ssh/**"
    ]
  }
}
```

### 5.1 Glob 语法

抄 Claude Code:
- `*` —— 任意字符序列（含空格）。`Bash(ls *)` 匹配 `ls -la`、`ls -R`（强制 word boundary，不匹配 `lsof`）
- `**` —— 多级目录递归
- `~` —— home dir 展开
- `./...` —— 相对 cwd
- `/...` —— 相对项目根
- `//...` —— 绝对路径
- `Bash(npm:*|yarn:*)` —— 多规则或
- `Bash` 裸名 —— 匹配所有 args
- `WebFetch(domain:github.com)` —— 网络 tool 用 `domain:` 前缀

### 5.2 评估顺序（**严格 deny → ask → allow → default**）

```go
func Evaluate(toolName string, args json.RawMessage, mode string, rules *Rules) Decision {
    formatted := formatForMatch(toolName, args) // e.g. "Bash(rm -rf node_modules)"

    for _, pattern := range rules.Deny {
        if glob.Match(pattern, formatted) {
            return Decision{Action: ActionDeny, Reason: pattern}
        }
    }
    for _, pattern := range rules.Ask {
        if glob.Match(pattern, formatted) {
            return Decision{Action: ActionAsk, Reason: pattern}
        }
    }
    for _, pattern := range rules.Allow {
        if glob.Match(pattern, formatted) {
            return Decision{Action: ActionAllow, Reason: pattern}
        }
    }
    return Decision{Action: ActionFromMode(rules.DefaultMode)}
}
```

**defaultMode 4 值**：
- `auto` —— 都通过，不提示（最宽松，给完全自动场景）
- `ask` —— 提示用户（默认）
- `dontAsk` —— 不提示直接通过（≈ auto，但 destructive=true 仍 ask）
- `bypassPermissions` —— 完全跳 permissions 系统（仅 protectedPaths 仍生效）

### 5.3 Hot reload

`infra/settings/watcher.go`：
- fsnotify watch `~/.forgify/settings.json`
- 改动事件 debounce 100ms 后 reparse
- parse 失败 → log + 保留旧规则（不让坏 JSON 卡死）
- 同时 1s poll 兜底（macOS fsnotify 有时不发事件）

**Service.GetRules() 返当前快照**（atomic.Value 持），评估全程用同一快照避免中途换规则。

---

## 6. Hook 协议

**第一形态：shell stdin/stdout JSON**。HTTP / MCP server 形态留 v2。

### 6.1 Hook 配置块

```json
{
  "matcher": "Bash|Edit",       // 简单字面 regex；匹配 tool_name
  "command": "/path/to/script", // exec 形式（无 shell）；或 "sh:" 前缀走 shell
  "args": [],                   // 可选 exec args
  "timeout": 10,                // 秒，默认 30
  "if": "Bash(rm *)"            // 可选 glob filter，仅匹配的 args 才跑
}
```

### 6.2 Hook stdin（JSON）

```json
{
  "session_id": "cv_xxx",
  "conversation_id": "cv_xxx",
  "cwd": "/Users/sp14921/dev",
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": {"command": "rm -rf node_modules"},
  "tool_use_id": "tc_xxx",
  "danger_level": "danger_full_access"
}
```

`PostToolUse` 增加：
```json
{
  ...,
  "tool_output": "removed 'node_modules/...'",
  "tool_error": "",
  "tool_status": "completed",
  "elapsed_ms": 234
}
```

`Stop` 仅有 `session_id` + `conversation_id` + `cwd`。

### 6.3 Hook stdout（JSON）

```json
{
  "decision": "allow",            // allow | deny | ask | defer (跳过决策)
  "reason": "matches safe pattern", // 用户可见
  "injectIntoNextTurn": "测试没过别停。", // PostToolUse 专用，注入到下轮 LLM context
  "systemMessage": "...",         // 注入 user-visible message
  "suppressOutput": false
}
```

### 6.4 Exit code 语义

- `0` —— 解析 stdout JSON 应用决策
- `2` —— **blocking 错**：stderr 喂给 LLM 当 tool_result error，**中断本 tool 执行**（PreToolUse）；PostToolUse 时不中断但 stderr 注入下轮 context
- 其他 —— 非 blocking：log + 继续；不解析 stdout

### 6.5 Hook 触发时机表

| Event | 触发点 | 输入 | 可否阻断 |
|---|---|---|---|
| `PreToolUse` | Permission gate 后，Tool.Execute 前 | tool_name + tool_input | ✅（exit=2 / decision=deny）|
| `PostToolUse` | Tool.Execute 后 | tool_input + tool_output + status | ❌ 仅注入 feedback |
| `Stop` | Agent 发出 finalPersist 前（无 tool_call、对话即将终态） | session_id | ✅（决议 continue → LLM 再跑一轮）|

**故意只 3 个时机**。Claude Code 有 12+，绝大部分（SubagentStart / FileChanged / CwdChanged / PreCompact 等）V1.2 用不上；v2 按需加。

### 6.6 三种 hook 形式对比

| 形式 | 协议 | 何时用 | V1.2 支持？ |
|---|---|---|---|
| **shell command** | stdin/stdout JSON | 默认，开发者自由度高 | ✅ |
| **HTTP** | POST JSON 到 URL | 远程服务做权限策略 | ❌ v2 |
| **MCP tool** | 调已连 MCP server 的 tool | MCP 生态成熟后 | ❌ v2 |

---

## 7. Protected paths（write 黑名单）

**默认硬编码 + 用户 settings 追加**。仅守 write tools（Edit / Write）；Read 已有 PathGuard。

### 7.1 默认黑名单（硬编码）

```go
var defaultProtectedWrite = []string{
    ".git/**", ".env", ".env.*", ".envrc",
    "node_modules/**", ".venv/**", "venv/**",
    "~/.ssh/**", "~/.aws/**", "~/.gnupg/**",
    "**/__pycache__/**", "**/*.pyc",
}
```

### 7.2 用户 settings 追加

`settings.json` 的 `protectedPaths.denyWrite` 数组追加自定义（不能去掉默认）。

### 7.3 实现

Edit / Write tool 的 Execute 前查 `pathguard.AllowWrite(path)`（新方法，复用现有 PathGuard 包结构）。命中默认或用户 deny → 返友好错误 `path %q is in protected-write list; aborting`。

### 7.4 Bash 不查 protectedPaths

**沿用现有 §S18 决策 D5**：Bash 是用户日常命令代理，自动黑名单意义不大。`Bash(rm -rf .git)` 等用 permissions.deny 配置拦（而非 protectedPaths）。

---

## 8. 数据模型变更

**无 DB schema 改动**。settings.json 是磁盘 JSON 文件；hook 执行无持久化；session-cache（"本 session 已 allow 过 Bash(npm test)"）是内存 map（agent 退出即清）。

---

## 9. 包结构（按 §S12）

```
internal/
├── domain/permissions/
│   └── permissions.go          # DangerLevel / Decision / Action 常量 + Rules struct
├── app/tool/permissionsgate/
│   ├── permissionsgate.go      # Gate (Service) + Evaluate + 集成 PreToolUse hook
│   ├── levels.go               # toolLevels map + LookupLevel
│   ├── glob.go                 # Bash / 路径 glob 匹配（参考 doublestar 库）
│   └── session_cache.go        # per-session ask-once 缓存
├── app/hooks/
│   ├── hooks.go                # HookRunner Service + PreToolUse/PostToolUse/Stop 入口
│   ├── shell.go                # shell exec form 实现
│   └── parse.go                # stdin/stdout JSON 协议
├── infra/settings/
│   ├── settings.go             # Service：load + watch + atomic.Value 快照
│   └── parse.go                # JSON schema 解析 + 校验
└── pkg/pathguard/             # 已有，加 AllowWrite() 方法
```

依赖方向：
```
chat (runTools)
   ↓
app/tool/permissionsgate (Gate.Evaluate)
   ↓                         ↓
app/hooks (HookRunner)   infra/settings (Service.GetRules)
   ↓
glob 匹配
```

无循环依赖。

---

## 10. HTTP API

### 10.1 用户面

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/settings` | 拿当前 settings.json 解析结果（已脱敏，无敏感字段）|
| PUT | `/api/v1/settings` | 写整个 settings.json（atomic write tmp+rename）|
| POST | `/api/v1/settings:reload` | 强制 reload（不依赖 watcher）|
| GET | `/api/v1/permissions/tools` | 列所有 tool + danger level + 当前规则下的决策预测 |
| POST | `/api/v1/permissions/test` | 测试规则：body `{toolName, args}` → 返 decision + reason |

### 10.2 testend 内部

- `/config/permissions` 视图：tabs（rules / hooks / protected paths）+ 实时验证
- `/config/hooks` 视图：列每个 hook 时机 + 安装的 hook + run 历史 + manual fire

---

## 11. 与其他 domain 的关系

| domain | 关系 |
|---|---|
| **chat** | runTools 调 `gate.Evaluate` → 派发 PreToolUse hook → 若 Decision=Ask 走 AskUserQuestion → tool 跑 → PostToolUse hook |
| **ask** | hook decision=ask 复用现有 AskUserQuestion 通道；用户决策回灌到 gate |
| **filesystem** | Edit / Write 加 `pathguard.AllowWrite` 检查 |
| **shell** | 不变；Bash 在 gate 层走 permissions 规则 |
| **memory** | write_memory / forget_memory 标 WorkspaceWrite；hook 可看到 + 修改 |
| **subagent** | sub-agent 派 tool 同样走 gate（继承父规则）|

---

## 12. 错误码（新增 1 个 sentinel）

| Sentinel | HTTP | Wire Code |
|---|---|---|
| `permissionsgate.ErrBlockedByRule` | 422 | `BLOCKED_BY_RULE` |

仅 1 个 sentinel —— hook 失败 / settings 解析错均走 log + 内部降级（不暴露给 LLM tool_result 层）。`BLOCKED_BY_RULE` 在 tool 派发被 deny 时通过 tool_result block 返给 LLM（`status="error"`，error 字段含 reason）。

---

## 13. 测试覆盖

| 层 | 文件 | 覆盖 |
|---|---|---|
| domain | `permissions/permissions_test.go` | DangerLevel 常量 / Rules struct JSON |
| app/permissionsgate | `permissionsgate_test.go` | Evaluate 完整 deny→ask→allow→default + glob 边界 |
| app/permissionsgate | `levels_test.go` | toolLevels 全 tool 登记 + 未登记 fallback |
| app/hooks | `shell_test.go` | shell exec / timeout / exit code 语义 |
| app/hooks | `parse_test.go` | stdin JSON 完整 + stdout 解析容错 |
| infra/settings | `watcher_test.go` | fsnotify 改文件 → reload；坏 JSON 不卡死 |
| transport/handlers | `permissions_test.go` | 5 endpoints happy + error |
| pipeline | `test/permissions/permissions_test.go` | E2E: deny 规则拦 Bash / ask 走 AskUserQuestion / PostToolUse 注入 feedback 进下轮 |

---

## 14. 关键决策记录

| 决策 | 选项 | 选了 | 理由 |
|---|---|---|---|
| 危险等级位置 | Tool 接口第 10 方法 / 框架硬编码 | **框架硬编码** | 抄 Claude Code；接口稳定；新 tool 强制登记防漏 |
| Settings 多层 | managed/project/user / 单层 | **单层** | 单用户本地无需 managed；project-level 留 v2（多 workspace 后再加）|
| Hot reload | 重启生效 / fsnotify watch | **watch + poll 兜底** | 单用户本地 UX 优先；改 settings 立刻生效 |
| Hook 第一形态 | shell only / shell+HTTP / 全栈 | **shell only** | 最简；HTTP/MCP v2 按真实需求加 |
| Hook 时机数 | 12+ (claude code) / 3 (Pre/Post/Stop) | **3 个** | 80/20；其他时机 v2 按需 |
| destructive 默认行为 | 完全跟规则 / 强制 ask | **强制 ask（除非 settings.allow 显式）** | LLM 自报 destructive=true 即用户该看一眼 |
| Bash 走 protectedPaths | 走 / 不走 | **不走**（沿用 §S18 D5）| Bash 是命令代理，黑名单意义低；高危用 permissions.deny |
| Ask-once 缓存范围 | session / conversation / forever | **session** | 重启清；保守 + 简单 |
| Stop hook 行为 | 软提示 / 可强制再跑 | **可强制再跑（decision: continue）** | 配合 §3.4 "测试没过别停" 用例 |
| 评估顺序 | deny>ask>allow vs 累加 | **deny>ask>allow 第一匹配** | 抄 Claude Code；deny 永不能被推翻是安全底线 |

---

## 15. 演化方向

- **HTTP hook 形态** —— 远程权限服务（v2）
- **MCP tool hook 形态** —— 接 MCP server 的 tool 评估（v2）
- **Project-level settings** —— `.forgify/settings.json` 在 cwd 上行查找 + merge 用户级 settings（v2，多项目后）
- **Ask-once 持久化** —— 让"本对话允许的"跨 session 保留（需谨慎，安全 trade-off）
- **Permission Explainer** —— destructive 操作前单独 LLM 调用解释风险（§3.8）
- **Path traversal 强化** —— URL 解码 / Unicode normalization / backslash injection 检测（§3.9）

---

## 16. 实施次序（建议 ~2-3 天）

| Step | 工作量 | 阻塞下一步？ |
|---|---|---|
| **1. levels.go + DangerLevel + LookupLevel + 全 tool 登记** | 2h | 否 |
| **2. domain/permissions + Rules struct + JSON parser** | 2h | 否 |
| **3. infra/settings + atomic.Value + fsnotify watch** | 3h | 阻 4-7 |
| **4. app/tool/permissionsgate + Evaluate + glob 匹配** | 4h | 阻 8 |
| **5. app/hooks + shell exec + stdin/stdout 协议** | 4h | 阻 8 |
| **6. pathguard.AllowWrite + 默认 protectedPaths** | 1h | 否 |
| **7. 5 个 HTTP endpoint + handler** | 2h | 否 |
| **8. 集成 runTools + 单测 + pipeline 测试** | 4h | 否 |
| **9. testend `/config/permissions` + `/config/hooks` 视图** | 4h | 否 |
| **10. 同步 6 个文档（本文件 + S 系列 / API / DB / error / events / progress）** | 2h | 否 |

总计：~28h ≈ 3 工作日。**与 Claude Code 等价** 的最小可用版本，不含 v2 HTTP/MCP hook。

---

## 17. 已知 trade-off + 风险

- **Glob 实现** —— Go 标准库无 doublestar 支持，需引 `github.com/bmatcuk/doublestar/v4` 或自写。**建议**直接引；100 行依赖换正确性。
- **Hook 进程开销** —— 每个 tool 派发 exec 一次 shell，~10ms-50ms overhead。**可接受**；用户能在 hook 配置中按 matcher 过滤减少不必要触发。
- **Hook 超时** —— hook 卡死会拖死 tool 派发。timeout 默认 30s + 强制 kill。
- **设置文件被恶意/手贱写坏** —— `~/.forgify/settings.json` 坏 JSON → 保留旧规则 + log + testend toast。**不让坏配置卡死整个 agent**。
- **Ask-once 缓存粒度** —— 缓存 key = (toolName, normalizedArgs)。`Bash(npm test)` 和 `Bash(npm test --watch)` 分别缓存，避免 args 微调绕过。
- **Hook stdout 必须是 valid JSON** —— 否则视为非 blocking，log + 继续（不让脏输出卡住 agent）。

---

## 18. 历史

- 2026-05-16 设计完成。**等用户过审**。基于 Claude Code hooks/permissions 实测调研（2026-05 内部 spike）+ Forgify 单用户本地优化（hot reload、单层 settings、3 时机精简）。完整调研附 `adhoc-topic-documents/permissions-research-2026-05-16.md`（待写）。
