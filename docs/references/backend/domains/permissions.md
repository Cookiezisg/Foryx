---
id: DOC-116
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Permissions & Hooks — 安全治理与拦截原理

> **核心职责**：Permissions 是 Forgify 的 **“治安官”**。它通过 `settings.json` 定义的一组声明式规则，管控 LLM 工具调用的权限，并支持在调用前后插入物理钩子（Hooks）进行审计或干预。

---

## 1. 物理模型 (Data Anatomy)

Permissions 数据完全持久化在物理文件 `~/.forgify/users/<uid>/settings.json` 中，不入数据库。

### 1.1 `Settings` 结构
```typescript
interface Settings {
    permissions: {
        // 工具黑白名单
        allow: string[]; // 支持 glob, 如 "fs_*"
        deny: string[];
        ask: string[];  // 命中则阻塞询问用户
    };
    hooks: {
        preToolUse: HookSpec[];  // 调用前执行
        postToolUse: HookSpec[]; // 调用后执行
        onStop: HookSpec[];      // 对话中止时执行
    };
    protectedPaths: string[]; // 物理路径保护，禁止 fs 工具访问
}
```

### 1.2 `HookSpec` (钩子规格)
```typescript
interface HookSpec {
    matcher: string; // 匹配工具名
    command: string; // 执行的物理命令
    args: string[];  
    if?: string;    // CEL 表达式条件
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Danger-Level Classification (危险等级)
Forgify 将工具分为三个危险等级：
- **Safe** (默认)：只读、无副作用的操作。
- **Warning**：修改本地文件或发送常规请求。
- **Critical**：删除资源、修改系统设置、执行 Shell 命令。

### 2.2 Triple-Gate Intervention (三重门拦截)
工具在物理执行前必须依次通过：
1. **Rule Gate**：检查 `deny/allow` 清单。
2. **Pre-Tool Hook**：拉起外部子进程进行审计（如：用第三方扫描器扫描 LLM 生成的代码）。
3. **User Confirmation**：若命中 `ask` 规则或工具标为 `destructive`，则挂起等待用户 UI 确认。

### 2.3 CEL-Based Hook Condition (动态钩子)
钩子支持 `if` 表达式：
- **原理**：利用 CEL 语言。
- **变量**：支持访问 `args` (调用参数) 和 `tool` (工具名)。
- **效果**：实现“只有当 `fs_write` 的路径包含 `/etc/` 时才拦截”这种精细化管控。

---

## 3. 生命周期 (Lifecycle)

1. **配置 (Seeding)**：系统启动加载 `settings.json`。
2. **侦听 (Intercepting)**：`chat.Service` 在创建 `chatHost` 时注入 `permissionsgate`。
3. **验证 (Evaluating)**：LLM 发起 tool_call。
4. **决策 (Deciding)**：
   - `allow` -> 继续执行。
   - `deny` -> 返回 `BLOCKED_BY_RULE` 错误给 LLM。
   - `ask` -> 挂起，触发前端弹出授权对话框。
5. **审计 (Post-Action)**：调用完成后，执行 `postToolUse` 钩子记录结果。

---

## 4. 跨域集成 (Interactions)

- **Chat**：作为 Interceptor 嵌入 ReAct 循环。
- **Workflow**：在 Workflow 执行节点中同样应用此权限模型。
- **User**：每个用户的 `settings.json` 物理物理隔离。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrBlockedByRule` | 422 | `PERM_DENIED` | 被 `deny` 规则强制拦截。 |
| `ErrHookFailed` | 502 | `HOOK_EXEC_ERROR` | 钩子命令运行报错（如 command 不存在）。 |
| `ErrInvalidSettings` | 400 | `INVALID_SETTINGS` | JSON 格式错误或 CEL 语法错。 |
| `ErrUserRejected` | 403 | `USER_REJECTED` | 用户在 `ask` 弹窗中点击了“拒绝”。 |
