# User — 本地多 Profile 切换

> V1.2 §20 / 2026-05-17。**单机多账号**（个人 / 工作 / 副业），不是真 auth。Slack workspace 切换风格——单机谁拿 laptop 谁有控制权。

---

## 1. 为什么要这个 domain

V1.2 之前 backend 用硬编码 `DefaultLocalUserID = "local-user"` 处理所有请求。所有 entity 的 `user_id` 列都填这个值。**用户实际场景**：

- 同一台 laptop 想分开"个人项目"与"工作项目"——不同 API key 池、不同 conv 历史、不同 forge 库
- 团队共享 dev 机（罕见但有），临时切到同事 profile dogfood
- 用户给不同领域起不同身份（"daily-research" vs "side-game-dev"）

**不是真 auth**——单机本地，密码 / OS 钥匙串都是反生产力。**身份 = 数据隔离器**而已。

---

## 2. 核心决策

- **DB 多用户已就绪**：14 entity 全带 `user_id` 列 + repo 自动 scope。User domain 只补"identity"那一层
- **Session = X-Forgify-User-ID header**（SSE EventSource API 限制 → `?userID=` query 兜底）
- **无密码 / 无锁**：V1.2 minimal scope；V1.5 真撞需求再加
- **后台 polling / 系统服务**仍跑默认 user 上下文（catalog / skill / mcp）——切换是 active user 的 HTTP / SSE 视野，不重建 service tree（留 V1.5）
- **Trigger fire 用 workflow 所有者 user_id**：哪怕 active 是别人，A 的 cron workflow 仍 fire 进 A 的 flowrun
- **No 数据物理删除**：删 profile 走 GORM soft-delete，DB 行保留可恢复

---

## 3. 端到端推演

### 3.1 启动选 profile

```
[GET /api/v1/users]
  → app/user.Service.List → infra/store/user.Store.List → DB
  → []User
[browser localStorage forgify:active-user]
  ↓ if 0 users → bootstrap.EnsureDefault("local-user","default")
  ↓ if 1 user  → auto-pick
  ↓ if 2+      → UserPicker.vue 弹出
  → POST /api/v1/users/{id}:activate
  → 写 X-Forgify-User-ID header 到每次请求
  → SSE 重连带 ?userID=
```

### 3.2 切换 profile

```
UserSwitcher dropdown → switchTo(uid)
  → POST /users/{uid}:activate (touch last_used_at)
  → localStorage.setItem("forgify:active-user", uid)
  → window.location.reload()
  → 全前端 state 重建,新请求带新 user header
```

---

## 4. 领域模型

### User struct（`internal/domain/user/user.go`）

```go
type User struct {
    ID          string         `gorm:"primaryKey;type:text" json:"id"`
    Username    string         `gorm:"not null;uniqueIndex" json:"username"`
    DisplayName string         `gorm:"type:text;default:''" json:"displayName"`
    AvatarColor string         `gorm:"type:text;default:''" json:"avatarColor,omitempty"`
    Language    string         `gorm:"type:text;default:'zh-CN';check:language IN ('zh-CN','en')" json:"language"`
    LastUsedAt  *time.Time     `json:"lastUsedAt,omitempty"`
    CreatedAt   time.Time      `json:"createdAt"`
    UpdatedAt   time.Time      `json:"updatedAt"`
    DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
```

| 字段 | 说明 |
|---|---|
| `ID` | 默认 user 固定 `"local-user"` 匹配 reqctxpkg.DefaultLocalUserID；新用户 `u_<16hex>` |
| `Username` | 1-32 [a-z0-9_-]；UNIQUE；登录态键 |
| `DisplayName` | 展示用昵称（缺省 = username）|
| `AvatarColor` | hex 色 `#4f46e5`；UI tile + dropdown 用 |
| `Language` | `zh-CN` 默认 / `en`；CHECK 约束；§21 i18n |
| `LastUsedAt` | activate 时刷；picker 高亮最近用 |

### 错误 sentinel

```go
var (
    ErrNotFound          = errors.New("user: not found")
    ErrUsernameRequired  = errors.New("user: username required")
    ErrUsernameConflict  = errors.New("user: username already exists")
    ErrUsernameInvalid   = errors.New("user: username must be 1-32 chars, [a-z0-9_-]")
    ErrCannotDeleteLast  = errors.New("user: cannot delete the last user")
    ErrLanguageInvalid   = errors.New("user: language must be one of zh-CN, en")
)
```

errmap 见 [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md)。

---

## 5. Repository 接口

```go
type Repository interface {
    Save(ctx, *User) error
    Get(ctx, id) (*User, error)
    GetByUsername(ctx, username) (*User, error)
    List(ctx) ([]*User, error)
    Delete(ctx, id) error
    Count(ctx) (int, error)
    TouchLastUsed(ctx, id) error
}
```

**注**：唯一不按 ctx user_id scope 的 repo——user 自己就是身份。

---

## 6. Service 层

### Create 流程

1. lowercase username + regex 校验 `^[a-z0-9_-]{1,32}$`
2. 默认 Language = `zh-CN`；显式传入校验白名单
3. DisplayName 缺省 = username
4. ID = `u_<16hex>`
5. UNIQUE conflict → `ErrUsernameConflict`

### EnsureDefault 启动迁移

```go
n, _ := repo.Count(ctx)
if n > 0 { return nil, nil }  // noop
u := &User{
    ID: "local-user",        // 匹配 reqctxpkg.DefaultLocalUserID
    Username: "default",
    DisplayName: "Default",
    AvatarColor: "#4f46e5",
    Language: "zh-CN",
}
```

老的所有 row 的 `user_id="local-user"` → 在 UI 里自然 surface 为"default" profile，用户可改名。

### Delete 守卫

```go
if Count == 1 → ErrCannotDeleteLast
```

防止"删光最后一个 user 进入无 user 死状态"。

---

## 7. HTTP API（5 端点）

详细 wire 形见 [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md)。

| Method | Path | 用途 |
|---|---|---|
| `GET /api/v1/users` | 列表（无分页，量小）|
| `POST /api/v1/users` | 创建 |
| `GET /api/v1/users/{id}` | 单查 |
| `PATCH /api/v1/users/{id}` | partial update（displayName / avatarColor / language）|
| `DELETE /api/v1/users/{id}` | 软删（拒最后一个）|
| `POST /api/v1/users/{id}:activate` | touch last-used + 返当前 User |

---

## 8. 数据库表

```sql
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL DEFAULT '',
    avatar_color  TEXT NOT NULL DEFAULT '',
    language      TEXT NOT NULL DEFAULT 'zh-CN' CHECK(language IN ('zh-CN','en')),
    last_used_at  DATETIME,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    DATETIME
);

CREATE UNIQUE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);
```

---

## 9. Session 机制（与 middleware 协议）

`InjectUserIDWith(resolver)` 中间件 ctx 注入路径：

```
1. Read header X-Forgify-User-ID
   OR fallback to query ?userID=  (SSE EventSource 不能自定义 header)
2. If non-empty:
   resolver.Get(ctx, uid) → 验证存在
   → 命中：用此 uid
3. Else / unknown:
   resolver.List(ctx) → 取首个 user
   → 命中：用首个 user.ID
4. Else (空 DB):
   → DefaultLocalUserID 兜底
```

前端 `client.ts` 自动注入；SSE `sse.ts` URL append `?userID=`。

---

## 10. 文件系统 per-user

`pkg/userpath.UserHome(homeRoot, uid)` 返 `homeRoot/users/<uid>/`。

| 子路径 | 用途 |
|---|---|
| `mcp.json` | MCP 服务器注册（per user）|
| `skills/` | Skill 目录（per user）|
| `.catalog.json` | Catalog 缓存（per user）|
| `settings.json` | Permissions / hooks 设置（per user）|

**`pkg/userpath.MigrateLegacy(homeRoot, uid, names...)`**：把单用户期 `homeRoot/<name>` 平迁到 `homeRoot/users/<uid>/<name>`，target 已存在则 skip。启动期对 default user 做一次。

**共享**：`homeRoot/sandbox/`（mise runtime + per-conv env）+ `homeRoot/forgify.db`（SQLite 内 user_id 列 scope）。

---

## 11. V1.5 已 defer 项

- **后台 polling per-user**：catalog / skill / mcp 现仍按默认 user 跑；切 active user 时不重建 service tree
- **Trigger 自动注册**：workflow create 时 auto-register trigger 路径未实装（今天只有手动 `:trigger`）；当真接入时 `Spec.UserID` 字段已就位
- **密码 / 锁**：单机本地不需要，真要做用 Argon2id + DB 加密敏感字段
- **macOS Keychain 集成**
- **远程同步 profile**

---

## 12. 与其他 domain 协作

```
Browser localStorage / cookie
  ↓ X-Forgify-User-ID
HTTP Middleware (InjectUserIDWith)
  ↓ reqctxpkg.SetUserID(ctx, uid)
  ↓
[14 个 user-scoped domain]
  ├── apikey / model / conversation / chat
  ├── function / handler / workflow / flowrun
  ├── mcp / skill / document / memory
  └── trigger (启动注册按 user iterate)
```

SSE Bridge（eventlog / notifications / forge）按 ctx user_id 路由 → User A 看不见 User B 的事件。

---

## 13. 实现清单（✅ V1.2 完成）

### domain 层 ✅
- `User` struct + 6 sentinel + `Repository` interface + `IsValidLanguage`

### infra 层 ✅
- `infra/store/user/user.go` GORM 实现 + UNIQUE conflict 翻 sentinel

### app 层 ✅
- `app/user/user.go` Service：`Create` / `Get` / `GetByUsername` / `List` / `Update` / `Delete` / `EnsureDefault` / `TouchLastUsed`

### transport 层 ✅
- `handlers/users.go` 6 endpoints + errmap 6 行

### 配套 ✅
- `middleware/auth.go` 重写为 `InjectUserIDWith(resolver)` + legacy `InjectUserID` 兜底
- `pkg/userpath` `UserHome` + `MigrateLegacy`
- main.go bootstrap：EnsureDefault + 4 个文件系统 root 切到 `defaultUserHome` + Rehydrate 遍历 user
- `triggerdomain.Spec.UserID` + onFire 用 spec.UserID

### 测试 ✅
- 8 user CRUD 单测 + 3 middleware header 路径单测 + 4 userpath 迁移单测 = **15 新单测**

### 前端 ✅
- `api/users.ts` + `stores/users.ts`
- `UserPicker.vue` 启动选择屏
- `UserSwitcher.vue` TopBar avatar dropdown
- `/config/profile` 管理页（含 language select）
