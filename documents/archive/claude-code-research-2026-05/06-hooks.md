# 06 — Claude Code Hooks 系统

## 信息来源与局限

主要参考：
- https://code.claude.com/docs/en/hooks-guide (官方完整文档，含全部 27+ 事件)
- https://code.claude.com/docs/en/hooks (reference)
- https://claudefa.st/blog/tools/hooks/hooks-guide
- https://claude.com/blog/how-to-configure-hooks
- https://github.com/anthropics/claude-code/issues/10412

---

## 1. Hook 类型完整清单（27 个事件）

✅ 直接抄自官方 docs（v2.1.x），按生命周期分类：

### 1.1 Session 生命周期

| 事件 | 何时触发 | matcher 维度 | 可被 exit 2 阻断？ |
|---|---|---|---|
| `SessionStart` | session 开始或 resume | `startup`, `resume`, `clear`, `compact` | 否（仅 stderr 显示） |
| `Setup` | `--init-only` / 一次性准备 | `init`, `maintenance` | 否 |
| `SessionEnd` | session 终止 | `clear`, `resume`, `logout`, `prompt_input_exit`, `bypass_permissions_disabled`, `other` | 否 |

### 1.2 用户交互

| 事件 | 何时 | matcher | exit 2？ |
|---|---|---|---|
| `UserPromptSubmit` | 用户提交 prompt 后、Claude 处理前 | 无 | 是（exit 2 阻断这次提交） |
| `UserPromptExpansion` | slash command 展开为 prompt 时 | 命令名 | 是 |
| `Notification` | Claude 发通知（permission_prompt 等） | `permission_prompt`, `idle_prompt`, `auth_success`, ... | 否 |

### 1.3 Tool 生命周期 ⭐ 最常用

| 事件 | 何时 | matcher | exit 2？ |
|---|---|---|---|
| `PreToolUse` | tool call 即将执行前（在权限检查之前） | tool 名（`Bash`, `Edit\|Write`, `mcp__github__*`） | **是** —— 阻断 tool call |
| `PermissionRequest` | 权限对话框出现时（仅 interactive） | tool 名 | 是 |
| `PermissionDenied` | tool call 被 auto mode classifier 拒绝 | tool 名 | — 可返回 `{retry: true}` |
| `PostToolUse` | tool 成功后 | tool 名 | 是（feedback 给 LLM）|
| `PostToolUseFailure` | tool 失败后 | tool 名 | 同 PostToolUse |
| `PostToolBatch` | 整个并行 batch 完成、下次 LLM call 前 | 无 | 否 |

### 1.4 Subagent / Task

| 事件 | 何时 | matcher |
|---|---|---|
| `SubagentStart` | spawn subagent | agent type (`Explore`, `Plan`, ...) |
| `SubagentStop` | subagent 终止 | agent type |
| `TaskCreated` | TaskCreate 调用 | 无 |
| `TaskCompleted` | task 完成 | 无 |
| `TeammateIdle` | teammate 即将 idle | 无 |

### 1.5 终止 / 控制

| 事件 | 何时 | exit 2？ |
|---|---|---|
| `Stop` | Claude 完成本次回应 | **是** —— 阻止停止，让 Claude 继续工作 |
| `StopFailure` | turn 因 API error 结束（输出/exit code 被忽略）| — |

### 1.6 Compaction

| 事件 | 何时 |
|---|---|
| `PreCompact` | 压缩前 |
| `PostCompact` | 压缩后 |

### 1.7 配置 / 文件 / 目录

| 事件 | 何时 | matcher |
|---|---|---|
| `InstructionsLoaded` | CLAUDE.md / rules 被加载到 context | `session_start`, `nested_traversal`, `path_glob_match`, `include`, `compact` |
| `ConfigChange` | settings/skills 文件变化 | `user_settings`, `project_settings`, `local_settings`, `policy_settings`, `skills` |
| `CwdChanged` | 工作目录变化（如 cd） | 无 |
| `FileChanged` | 监视的文件变化 | filename glob (`.envrc\|.env`) |

### 1.8 Worktree / MCP

| 事件 | 何时 |
|---|---|
| `WorktreeCreate` | 创建 worktree（替换默认 git 行为） |
| `WorktreeRemove` | 移除 worktree |
| `Elicitation` | MCP server 请求用户输入 |
| `ElicitationResult` | 用户回应 elicitation 后 |

---

## 2. Hook 实现方式（5 种 type）

### 2.1 type: "command" ⭐ 最常用

✅ shell 命令，stdin/stdout/stderr/exit code 协议：

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Edit|Write",
      "hooks": [{
        "type": "command",
        "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/protect-files.sh",
        "timeout": 30
      }]
    }]
  }
}
```

✅ Stdin（JSON）：
```json
{
  "session_id": "abc123",
  "cwd": "/Users/sarah/myproject",
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": { "command": "npm test" }
}
```

✅ Stdout / Exit code：
- **exit 0**：通过。stdout 在 `UserPromptSubmit` / `SessionStart` / `SessionStart:compact` 等事件下会被**作为 system reminder 注入 Claude context**
- **exit 2**：阻断。stderr 内容作为 feedback 给 LLM
- **其他 exit**：通过；transcript 显示 hook 错误

✅ JSON 结构化输出（更精细控制）：

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "deny",
    "permissionDecisionReason": "Use rg instead of grep"
  }
}
```

`permissionDecision`：
- `"allow"`：跳过权限对话框（**仍受 deny 规则约束**）
- `"deny"`：阻断
- `"ask"`：强制弹对话框
- `"defer"`（仅 `-p` 模式）：保留 tool call 让 SDK wrapper 接管

### 2.2 type: "http"

```json
{
  "type": "http",
  "url": "http://localhost:8080/hooks/tool-use",
  "headers": { "Authorization": "Bearer $MY_TOKEN" },
  "allowedEnvVars": ["MY_TOKEN"]
}
```

✅ POST event JSON，响应 body 用同 JSON 协议。HTTP 状态码**单独**不能阻断（必须 2xx + 正确 body）。

### 2.3 type: "prompt"（LLM 评估）

```json
{
  "type": "prompt",
  "prompt": "Check if all tasks are complete. If not, respond {\"ok\": false, \"reason\": \"<what remains>\"}.",
  "model": "haiku"
}
```

✅ 模型默认 Haiku；返回 JSON `{ok: bool, reason: string}`。**单回合 LLM call，不调 tool**。

### 2.4 type: "agent"（Subagent 验证）

```json
{
  "type": "agent",
  "prompt": "Verify all unit tests pass. Run the test suite. $ARGUMENTS",
  "timeout": 120
}
```

✅ 启 subagent 跑（最多 50 turns + 60s 默认 timeout，可调）；和 prompt hook 一样返 `{ok, reason}`。**实验性**。

### 2.5 type: "mcp_tool"

✅ 调用已连接 MCP server 的 tool 作为 hook。schema 略复杂，少见。

---

## 3. settings.json 完整配置 schema

### 3.1 基础结构

```json
{
  "hooks": {
    "<EventName>": [
      {
        "matcher": "<pattern>",          // 可选，per-event 不同维度
        "hooks": [                        // 此 matcher 下的 hook 列表
          {
            "type": "command",            // command/http/prompt/agent/mcp_tool
            "command": "<shell>",         // command type
            "timeout": 30,                // 秒，默认 600
            "if": "Bash(git *)"           // ★ v2.1.85+，permission rule 语法
          }
        ]
      }
    ]
  },
  "disableAllHooks": false                // 紧急关闭所有 hook
}
```

### 3.2 matcher 语法

✅ **PreToolUse / PostToolUse 类**：tool 名，支持正则
- `"Bash"` 精确
- `"Edit|Write"` 多选
- `"mcp__github__.*"` 正则
- `"mcp__.*__write.*"` 跨 server

✅ **`if` 字段（v2.1.85+）**：用 permission rule 语法在 hook 启动**前**做参数级筛选——避免每条 Bash 命令都启动 hook 进程

```json
{
  "matcher": "Bash",
  "hooks": [{
    "type": "command",
    "if": "Bash(git *)",        // 只在 git 子命令时启动 hook
    "command": "..."
  }]
}
```

### 3.3 多 hook 执行

✅ 同事件多个匹配 hook **并行**跑；相同命令自动去重。多个 hook 返回 decision 时，**取最严**：
- 任一返 `deny` → 拒
- 任一返 `ask` → 弹框（即使其他返 `allow`）
- additionalContext 取**所有**hook 的并集喂给 Claude

### 3.4 配置位置（scope）

| 位置 | scope | 是否分享 |
|---|---|---|
| `~/.claude/settings.json` | 用户全局 | 否 |
| `<proj>/.claude/settings.json` | 项目 | 是（commit） |
| `<proj>/.claude/settings.local.json` | 项目本地 | 否（gitignore） |
| Managed policy | 组织 | 是（管理员） |
| Plugin `hooks/hooks.json` | plugin 启用时 | 是 |
| Skill/agent frontmatter | skill/agent 激活时 | 是 |

---

## 4. MCP Tool Hook 匹配

✅ MCP tool 命名：`mcp__<server>__<tool>`，例如 `mcp__github__create_pull_request`。

✅ 匹配方式：
- `mcp__github` 匹配 `github` server 的所有 tool（前缀）
- `mcp__github__*` 也行（regex 通配）
- `mcp__github__create_pr.*` 子集

✅ MCP tool 的 PreToolUse 和普通 tool 完全一样的协议；hook input 里 `tool_name` 是完整带前缀名。

---

## 5. Hook 与权限系统的关系

✅ 优先级（来自 §06 + §08 综合）：

```
1. Managed deny rules        ← 最高，无人能 override
2. PreToolUse hook (deny)    ← 任何 deny 即阻断
3. Permission rules (deny)   ← 比 hook allow 优先
4. PreToolUse hook (allow)   ← 但仍受 deny rules 拦
5. Permission rules (ask)    ← 即使 hook allow 也弹框
6. Permission rules (allow)
7. Permission mode (default fallback)
```

✅ 关键："hook 能 tighten 但不能 loosen"——hook 返 `allow` 不能绕过 settings deny；但 hook 返 `deny` **能** override allow 规则、能 override `bypassPermissions` 模式。这就是为啥企业 IT 喜欢 hook 做 hard policy。

---

## 6. Stop Hook 死循环防御

✅ 经典坑：

```
LLM 完 → Stop hook → block → LLM 继续 → 完 → Stop hook → block → 无限...
```

✅ 解：hook input 带 `stop_hook_active: boolean`。Hook 必须自查：

```bash
INPUT=$(cat)
if [ "$(echo "$INPUT" | jq -r '.stop_hook_active')" = "true" ]; then
  exit 0   # 已经强制过一次了，放过
fi
# ... 业务判断 ...
```

✅ Claude Code 也保护：上一条 message 是 API error 时，Stop hook 自动 skip——避免 hook 把 error 当 reason 又触发新 turn。

---

## 7. 对 Forgify 的改进建议

> 现状：`.claude/settings.local.json`（如果有）可能存在一个 PostToolUse hook（按用户描述，但本仓库 clone 中**未找到 `.claude` 目录**——可能是 fork 没带过来）；无正式 hook 接口；`chat/tools.go executeTool` 是没有 hook 插槽的纯函数。

| # | 改进 | 优先级 | Go 实施要点 |
|---|---|---|---|
| 1 | **正式 Hook 接口** | P0 | 新文件 `chat/hooks.go`：```go<br>type HookEvent string<br>const (<br>  EvPreToolUse  HookEvent = "PreToolUse"<br>  EvPostToolUse           = "PostToolUse"<br>  EvStop                  = "Stop"<br>  EvSessionStart          = "SessionStart"<br>  EvUserPromptSubmit      = "UserPromptSubmit"<br>)<br><br>type HookInput struct {<br>  Event     HookEvent<br>  SessionID string<br>  CWD       string<br>  ToolName  string                 // PreToolUse/PostToolUse only<br>  ToolInput json.RawMessage<br>  ToolOutput string               // PostToolUse only<br>  StopHookActive bool             // Stop only<br>}<br><br>type HookOutput struct {<br>  Allow            bool<br>  Reason           string         // exit 2 等价：阻断 + 给 LLM 反馈<br>  AdditionalContext string        // 注入下一轮 system reminder<br>}<br><br>type Hook interface {<br>  Run(ctx context.Context, in HookInput) (HookOutput, error)<br>}<br>``` |
| 2 | **PreToolUse hook 插入点** | P0 | `chat/tools.go executeTool`（行 113）开头：`if out := s.hooks.RunPreToolUse(ctx, name, argsJSON); !out.Allow { return out.Reason, false }` |
| 3 | **PostToolUse hook** | P0 | executeTool 末尾、return 之前：`s.hooks.RunPostToolUse(ctx, name, argsJSON, output, ok)`。返回的 `AdditionalContext` 加到下条 user message 作为 system reminder |
| 4 | **Stop hook** | P1 | 见报告 01 改进 #2 |
| 5 | **配置文件解析** | P0 | `internal/pkg/hooksconfig`：解析 `.forgify/hooks.json`（Forgify 风格 JSON 即可，schema 抄 Claude Code）。支持 command/HTTP 两种 type 起步 |
| 6 | **执行：command 类型** | P0 | `os/exec` 跑 shell；stdin 写 JSON；30s 默认超时；exit 2 = block；解析 stdout JSON |
| 7 | **执行：HTTP 类型** | P1 | `net/http` POST JSON；2xx 必须；解析 body 同 JSON 协议 |
| 8 | **matcher pattern** | P1 | 用 Go regexp 库；按 event 类型选不同字段（tool_name / source / matcher 维度） |
| 9 | **disableAllHooks 全局开关** | P3 | 配置一项 |
| 10 | **stop_hook_active 死循环保护** | P1 | 在 chat/runner.go 加一个 attempt counter，第二次 stop 已被 block 后强制让 LLM 停（log warning） |

最先做：**#1 + #2 + #3 + #5 + #6**——形成完整的 PreToolUse / PostToolUse 命令钩子链路，约 2 天工作量。这就够覆盖"编辑 backend/ 时注入文档同步提醒"这类用户已有需求。


