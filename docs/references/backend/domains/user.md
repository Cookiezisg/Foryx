---
id: DOC-126
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# User Domain — 身份根节点、Profile 隔离与自愈认证全书

> **宪法级定义**：User 是 Forgify 的“物理数据隔离器”。它不提供复杂的 RBAC 权限（单机本地无此必要），而是通过物理列和中间件强制手段，确保用户在“个人”、“工作”等不同 Profile 间切换时，API Key、对话历史、工作流、沙箱环境实现绝对的物理解耦。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `User` 实体结构 (GORM 1:1)
```go
type User struct {
    // 物理 PK: u_<16hex>。符合 §S15 规范。
    ID          string         `gorm:"primaryKey;type:text" json:"id"`
    
    // 唯一标识: 1-32 位 [a-z0-9_-]。存入前强制 Lowercase。
    Username    string         `gorm:"not null;uniqueIndex:idx_users_username;type:text" json:"username"`
    
    // 装饰字段: 前端渲染 UI 标签和头像色。
    DisplayName string         `gorm:"type:text;default:''" json:"displayName"`
    AvatarColor string         `gorm:"type:text;default:''" json:"avatarColor,omitempty"` 
    
    // 国际化: 物理 CHECK 约束限制集合 {'zh-CN', 'en'}。默认 'zh-CN'。
    Language    string         `gorm:"type:text;default:'zh-CN';check:language IN ('zh-CN','en')" json:"language"`
    
    // 活跃度: 每次 :activate 动作刷墙钟，用于前端 Picker 排序。
    LastUsedAt  *time.Time     `json:"lastUsedAt,omitempty"`
    
    // 审计时间戳 (UTC)。
    CreatedAt   time.Time      `json:"createdAt"`
    UpdatedAt   time.Time      `json:"updatedAt"`
    
    // 逻辑删除: 软删后不可见。
    DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
```

### 1.2 ID 生成原理 (`pkg/idgen`)
- **熵源**：调用 `crypto/rand` 读取 8 字节随机数。
- **鲁棒性**：若 OS 级熵源损坏，`New("u")` 会触发立即 **Panic**。因为坏的随机源会导致物理碰撞，产生无法挽回的数据污染。

---

## 2. 身份识别与自愈中间件 (Auth Stack)

Forgify 采用 **“无状态声明 + 状态校验”** 模式。

### 2.1 流量入口识别 (`IdentifyUser`)
后端不持 Session，全靠 Request Header 驱动。
- **Header**: `X-Forgify-User-ID: u_<16hex>`。
- **Query Fallback**: SSE 或某些 `EventSource` 请求通过 `?userID=u_xxx` 注入。
- **深度校验原理**：中间件拿到 ID 后，**必须**调用 `resolver.Get` 去 DB 查一下。若 ID 格式对但库里没这个人（比如刚被删了），则 **Ctx 仍会被视为空**。这种“实时校验”消除了脏 ID 存在的可能性。

### 2.2 强制准入 (`RequireUser`)
在 `IdentifyUser` 执行后挂载。
- **逻辑断路**：若 Ctx 无有效 UserID -> 立即返回 `401 Unauthorized` + Code `UNAUTH_NO_USER`。
- **豁免白名单 (Exemptions)**：
  - `/api/v1/users` (POST/GET)：允许新用户注册及 Profile 列表拉取。
  - `/api/v1/health`：系统存活探针。
  - `/api/v1/providers` / `/api/v1/scenarios`：基础配置查询。

### 2.3 前端自愈环 (Self-Healing Loop)
1. 前端 `apiFetch` 收到 401。
2. 捕获 `UNAUTH_NO_USER` 错误。
3. 清除 `localStorage.activeUserId`。
4. **App.jsx Effect** 监控到 ID 为空，自动拉起 **Onboarding Overlay** 或 **User Picker**。

---

## 3. Profile 隔离与物理落地 (Isolation)

### 3.1 逻辑数据隔离
- **Repository 铁律**：除 `users` 表自身外，其余所有业务 Repository 必须在 `Find/Save` 时链式注入 `Where("user_id = ?", ctx.UserID)`。
- **副作用隔离**：`Service.Create` 时生成的任何关联资源（如默认模型配置）都会打上当前 UserID。

### 3.2 物理文件隔离 (`pkg/userpath`)
UserID 决定了磁盘上的物理位置。
- **根目录**：`~/.forgify/users/<uid>/`。
- **分流明细**：
  - `mcp.json`：该用户的 MCP 服务器配置。
  - `skills/`：该用户的技能 MD 文件。
  - `.catalog.json`：该用户的工具发现缓存。
  - `settings.json`：该用户的权限与安全规则。

---

## 4. 生命周期 (Detailed Lifecycle)

### 4.1 创建与迁徙 (`Create` & `EnsureExists`)
- **格式校验**：正则 `^[a-z0-9_-]{1,32}$`。强制 `ToLower` 以防大小写歧义产生隔离漏洞。
- **幂等性测试入口**：`EnsureExists` 仅供集成测试使用，允许指定物理 ID 以便快照对比。

### 4.2 激活与切账 (`:activate`)
- 调用该接口会触发 `TouchLastUsed`。
- 这是一个 **“仪式感端点”**：它返回最新的 User 对象，前端以此为标志刷新状态并 Reload 整个 App。

### 4.3 终极保护 (The Last Guardian)
- **拒绝死锁**：`Service.Delete` 内部执行 `Count()`。若检测到 `count <= 1`，则拒绝执行。
- **原理**：防止用户误删最后一个 Profile，导致系统失去身份根节点。若真要“重置系统”，需去控制台删除整个 `forgify.db`。

---

## 5. 跨域集成场景 (Principles in Action)

### 5.1 国际化 (i18n)
- `InjectLocale` 中间件通过 `IdentifyUser` 的结果，读取 User 实体的 `Language`。
- 将其注入 Ctx (`locale` key)，后续 LLM Prompt 模块据此动态拼装中英文 Header。

### 5.2 触发器所有权 (Trigger Owner)
- **Cron 场景**：即便当前电脑前坐的是用户 A，后台正在跑的用户 B 的 Cron 任务仍必须以用户 B 的身份执行。
- **实现**：`trigger_schedules` 表固化了 `user_id`。调度器在 Firing 时，从配置读取 owner，并将其注入 `X-Forgify-User-ID` 模拟上下文。

---

## 6. 错误字典 (Clinical Sentinels)

| Sentinel | HTTP | Wire Code | 场景及处理建议 |
|---|---|---|---|
| `ErrNotFound` | 404 | `USER_NOT_FOUND` | ID 错误。前端应立即清空 localStorage 并 Redirect。 |
| `ErrUsernameConflict` | 409 | `USERNAME_CONFLICT` | 用户名重复。Onboarding 阶段高亮输入框红色。 |
| `ErrUsernameInvalid` | 400 | `USERNAME_INVALID` | 含有空格或特殊字符。 |
| `ErrCannotDeleteLast` | 422 | `CANNOT_DELETE_LAST_USER` | 强力阻断。UI 侧应置灰最后一个用户的删除按钮。 |
| `ErrLanguageInvalid` | 400 | `LANGUAGE_INVALID` | 手敲 API 注入了非 zh-CN/en 值。 |
| - | 401 | `UNAUTH_NO_USER` | Header 缺失或 ID 在后端被物理注销。 |
