# 08 — Claude Code 权限与安全系统

## 信息来源与局限

主要参考：
- https://code.claude.com/docs/en/permissions (官方完整文档)
- https://code.claude.com/docs/en/sandboxing (官方)
- https://www.petefreitag.com/blog/claude-code-permissions/
- https://ona.com/stories/how-claude-code-escapes-its-own-denylist-and-sandbox
- https://kotrotsos.medium.com/claude-code-internals-part-8-the-permission-system-624bd7bb66b7
- https://dev.to/klement_gunndu/lock-down-claude-code-with-5-permission-patterns-4gcn

---

## 1. 5 层权限 Cascade

### 1.1 整体优先级（自上而下，先到的赢）

✅ 综合多个来源 + 官方 docs：

```
1. Managed deny rules            ← 组织管理员 (最高，谁都不能 override)
2. PreToolUse hook (deny)         ← 任何 hook 返 deny → 阻断
3. Settings deny rules            ← project/user 的 deny 规则
4. PreToolUse hook (allow)        ← hook 返 allow（仅用于跳过 ask）
5. Settings ask rules             ← 弹对话框
6. Settings allow rules           ← 通过
7. Permission mode 默认行为       ← default 弹框 / acceptEdits 自动接受 / plan 只读 / ...
8. Tool 自身 isReadOnly default   ← 没规则时按 tool 元字段判
```

**关键点**：
- Deny 永远 beats allow
- Hook **能 tighten 不能 loosen**：hook deny 能 override settings allow；hook allow 不能绕 settings deny
- Auto mode 在 default 之上加一层"YOLO classifier"做风险评估

### 1.2 Tool 自身 checkPermissions

✅ 每个 tool 实现可定义 `checkPermissions(input, mode)` 钩子：
- Read：默认任何 mode 都允许
- Bash：拦危险命令（详见 §1.3）
- Edit/Write：plan mode 拒；default mode 弹框；acceptEdits mode 通过

### 1.3 Bash 危险命令黑名单

✅ Bash tool 内部还有**硬编码的危险命令检测**（不可配置）：
- `rm -rf /` / `rm -rf ~` / `rm -rf $HOME` 等绝对路径删根
- 写到 `.git/`, `.claude/`, `.vscode/`, `.idea/`, `.husky/` 这些保护目录（即使在 bypassPermissions 模式下也仍弹框）
- `.claude/commands/`, `.claude/agents/`, `.claude/skills/` 是例外（Claude 经常写这些）

✅ 复合命令拆解：Claude Code 识别 `&&`, `||`, `;`, `|`, `|&`, `&`, 换行作为命令分隔符。`Bash(safe-cmd *)` 规则**不会**自动允许 `safe-cmd && rm -rf /`。每个 subcommand 必须独立匹配。

✅ Process wrapper 剥离（不可配置白名单）：`timeout, time, nice, nohup, stdbuf` 在匹配前自动剥离，所以 `Bash(npm test *)` 也匹配 `timeout 30 npm test`。**不剥离** `direnv exec`/`devbox run`/`docker exec`/`xargs -n1`/`watch`/`flock`/`find -exec` 这些可注入命令——避免被绕过。

### 1.4 Read-only 命令白名单

✅ 这些 Bash 命令**任何模式都不弹框**：`ls, cat, head, tail, grep, find, wc, diff, stat, du, cd`，加 git 的只读命令（log/status/show/diff）。集合不可配置。

### 1.5 路径规则语法（gitignore 风格）

✅ Read/Edit 规则的 4 种 path 模式：

| 模式 | 含义 | 例 |
|---|---|---|
| `//path` | 文件系统绝对路径 | `Read(//Users/a/secrets/**)` |
| `~/path` | home 目录 | `Read(~/.zshrc)` |
| `/path` | **项目 root 相对**（注意：不是绝对路径！） | `Edit(/src/**/*.ts)` |
| `path` / `./path` | 当前目录相对 | `Read(*.env)` |

⚠️ **大坑**：`/Users/alice/file` **不是**绝对路径，是项目 root 相对——必须 `//Users/alice/file` 才是绝对。

✅ Glob：`*` 匹配同目录文件名；`**` 跨目录递归。

### 1.6 Symlink 处理

✅ 有 symlink 时检查**两条路径**（symlink 自身 + 解析目标）：
- Allow rule：**两条都通过**才允许
- Deny rule：**任一匹配**就拒

例：`Read(./project/**)` allow + `Read(~/.ssh/**)` deny。`./project/key -> ~/.ssh/id_rsa` symlink → 拒（target 命中 deny）。

---

## 2. Permission Modes

### 2.1 6 种模式

| Mode | 行为 |
|---|---|
| `default` | 首次每个 tool 弹框 |
| `acceptEdits` | 自动接受 file edits + 常见 fs 命令（mkdir/touch/mv/cp）；其他仍弹 |
| `plan` | 只读模式；不能 edit 不能 exec |
| `auto` | research preview；后台 ML classifier 评估，自动批 / 弹 / 拒 |
| `dontAsk` | 自动拒所有非预批准的 |
| `bypassPermissions` | **危险**：跳过所有弹框（除保护目录外） |

✅ 切换：`Shift+Tab` 终端循环；CLI flag `--permission-mode <name>`；settings.json `defaultMode`。

✅ `bypassPermissions` 仍受 hard guard：
- 写 `.git/`, `.claude/`(除 commands/agents/skills), `.vscode/`, `.idea/`, `.husky/` 仍弹
- 删 `/`, home, 关键系统路径仍弹
- 可用 managed `disableBypassPermissionsMode: "disable"` 完全禁用

### 2.2 Auto mode YOLO classifier

⚠️ Auto mode 后台 ML classifier 把每次 tool call 分 HIGH / MEDIUM / LOW 风险：
- LOW（如 ls / grep / cat 已有内容）：自动批
- MEDIUM（如 Edit 现有文件 / npm install）：根据 settings 配置批或问
- HIGH（如 Bash rm / 写 sensitive file / 网络外联）：弹框

❌ classifier 的具体特征工程 / 模型架构 **未在公开分析中找到**。社区猜测是一个小型 fine-tuned model 跑在本地或 Anthropic 后端。

✅ Configure auto mode：用 `/configure-auto-mode` 告诉 classifier 哪些基础设施可信（"我们用 GitHub Actions + AWS"）。

---

## 3. Protected Files

### 3.1 默认保护清单

✅ 写入这些目录（不论模式）会触发额外保护：
- `.git/`（除非用户 explicitly 用 `git` 命令）
- `.claude/` 但 `.claude/commands/`、`.claude/agents/`、`.claude/skills/` 例外（Claude 应该能在这写自己的工具）
- `.vscode/`, `.idea/`, `.husky/`
- 用户 home 的 `.bashrc`, `.zshrc`, `.profile`, `.gitconfig` 等
- `~/.aws/credentials`, `~/.ssh/`, `~/.docker/config.json`, `~/.npmrc` 等密钥/token 文件

⚠️ 这套清单**不全部公开**。社区反编译笔记提到核心是路径正则 + 文件名 base 检查；具体 regex 未公开。

### 3.2 检查点

✅ 检查发生在 tool 自身的 `checkPermissions` 里（**不是**在通用权限层）。所以 Edit/Write 各自实现各自的保护逻辑。

---

## 4. Permission Explainer

✅ HIGH 风险 + auto mode 时，**单独调一次 LLM**（小模型 Haiku）生成"为什么风险"的人话解释，展示在权限对话框上方。

```
⚠️ 风险评估：HIGH
  Claude 想运行：sudo rm -rf /var/log/old-*
  原因：删除系统日志可能影响审计；通配符可能匹配 active 日志
  建议：拒绝；改用 truncate 或归档
[ 批准 ] [ 拒绝 ] [ 批准并永久允许此模式 ]
```

❌ explainer prompt 完整文本未在公开分析中找到。

---

## 5. 路径安全攻防

✅ Claude Code 在 path 处理路径上做的硬化：

### 5.1 URL 编码检测
- `Read(./project/%2E%2E/secrets/k.txt)` → 先 URL decode 再匹配 → `../secrets/k.txt` → 命中 deny

### 5.2 Unicode 归一化
- NFD/NFC 转换：`./café` 与 `./cafe\u0301` 都视为同一路径
- 防 homograph attack：拉丁 `o` vs 西里尔 `о` 字符——内部表查替换

### 5.3 Backslash injection
- Windows backslash `\` 转 `/` 后再 glob 匹配
- 路径里的 `..\` 和 `..\..\` 一并视为 `../`

### 5.4 Case 处理
- macOS 大小写不敏感文件系统：`./Secret.env` 实际命中 `./secret.env`
- 检查时用 `realpath` 拿到 canonical 路径再匹配 deny rule

✅ 详见 ona.com 的 "How Claude Code escapes its own denylist" 文章——记录了多种历史 CVE 已修。

---

## 6. Sandboxing（OS 级）

### 6.1 macOS Seatbelt

✅ Seatbelt（macOS 自带的 sandbox profile 系统）启动 Bash 子进程时套一个 profile：
- 限制可读路径：white list = 项目目录 + cwd + additionalDirectories
- 限制可写路径：同上
- 禁止网络：所有非 loopback 流量
- 用 `sandbox-exec -f <profile.sb> <bash_command>` 启子进程

⚠️ profile 的具体内容未在公开分析中找到全文，但会包含 `(allow file-read* (subpath "..."))` 和 `(deny network-outbound)` 之类语句。

### 6.2 Linux bubblewrap

✅ 类似机制：用 `bwrap` 起隔离子进程；bind-mount 项目目录可读写；其余只读 / 屏蔽。

### 6.3 网络过滤

✅ 网络通过 **本地代理** 实施 allowlist：
- 启动一个 local HTTP/HTTPS proxy（CONNECT 拦截）
- 所有 Bash 子进程的 HTTPS_PROXY/HTTP_PROXY 环境变量指向它
- proxy 检查 settings 的 `allowedDomains` / `WebFetch(domain:*)`，命中放行，否则返 403
- **fallback**：忽略 proxy 环境变量的程序（`curl --noproxy *`）会被 Seatbelt/bubblewrap 在 socket 层 block 非 loopback

### 6.4 sandboxing 配置

```json
{
  "sandbox": {
    "enabled": true,
    "filesystem": {
      "allowRead": ["./", "./node_modules/**"],
      "denyRead": [".env", "id_rsa"]
    },
    "network": {
      "allowedDomains": ["github.com", "registry.npmjs.org"]
    },
    "autoAllowBashIfSandboxed": true   // 默认 true：sandboxed Bash 不再每条弹框
  }
}
```

---

## 7. 对 Forgify 的改进建议

> 现状：
> - 无任何权限系统
> - run_shell：30s 超时来自 `httpClient`（实际是 `exec.CommandContext` 沿用 ctx；prompt 里说的"30s"实际不准确——RunShellTool 没有显式 timeout）
> - write_file 直接覆盖任何路径，无保护
> - 单用户本地，但 Phase 4 自动 workflow 风险骤升

| # | 改进 | 优先级 | Go 实施要点 |
|---|---|---|---|
| 1 | **Tool 接口加 PermissionLevel** | P0 | tool.go 接口增加：`PermissionLevel() PermLevel`，返回 enum<br>`type PermLevel int`<br>`const (PermReadOnly = iota; PermWorkspaceWrite; PermDangerFullAccess)`<br>所有现有 tool 标注：read_file/list_dir/web_search/fetch_url/datetime/forge_get_tool/forge_search_tools = ReadOnly；write_file/forge_create_tool/forge_edit_tool = WorkspaceWrite；run_shell/run_python/forge_run_tool = DangerFullAccess |
| 2 | **Protected paths 黑名单** | P0 | `agent/permissions.go` 新文件：`func IsProtectedPath(p string) bool` 检查 `~/.ssh/**`, `~/.aws/**`, `**/.git/objects/**`, `~/.gitconfig`, `~/.zshrc`, `~/.bashrc`, `**/.env`, `**/.env.*`, `**/credentials.json`。在 write_file Execute 开头调用，命中即报错"Protected path; not writable" |
| 3 | **Bash 危险命令拦截** | P0 | run_shell 加预检：用 `regexp` 匹配几条核心规则：`rm -rf /`/`rm -rf ~`/`rm -rf $HOME`/`rm -rf .`、`mkfs.*`、`> /dev/sda*`、`dd if=.* of=/dev/`。命中拒绝并返回错误说明。这套黑名单**应该是 hard-coded 不可配置**，避免被 prompt injection 改 |
| 4 | **Settings allow / deny 最小实现** | P1 | `.forgify/permissions.json`：`{allow: ["run_shell(npm *)"], deny: ["write_file(./.env)"]}`。在 chat/tools.go executeTool 第一行做匹配。glob 用简化版语法（前缀 + `*`），不上 gitignore 全套 |
| 5 | **Permission modes** | P2 | conversation 增加 `permissionMode: "default" \| "acceptEdits" \| "plan"` 字段。default 模式在 Phase 4 workflow 自动执行时弹（前端 SSE `chat.permission_request` 事件）；acceptEdits 跳过 file write 弹；plan 拦所有非 ReadOnly tool |
| 6 | **Workflow 自动模式 = bypassPermissions + 强黑名单** | P1 | Phase 4 workflow 跑时强制：1) 所有 #2 protected paths 仍 deny；2) 所有 #3 危险命令仍 deny；3) 其他全过——这就是"安全的 bypass"|
| 7 | **不上 OS 沙箱** | — | Forgify 是单用户本地桌面 app，OS 沙箱（Seatbelt）部署成本高、收益低。**先做软件层防护就够**。除非将来跑用户上传的不可信 forge_tool 才需要考虑。 |

最先做：**#1 + #2 + #3**（半天工作量）。这三个做好已经把 90% 的事故风险（误删根、写 secret 文件、覆盖关键 dotfile）挡住了。

