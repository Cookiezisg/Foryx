# apikey 契约（M1.2 设计蓝图）

> 经多轮讨论敲定（2026-06-04）。这是重写前的设计定稿，重写照此。
> 核心决策：apikey 大幅收窄为「加密保险箱 + 哑探针 + 按 id 发钥匙」，
> 不沾任何 provider 语义、不掺选择决策。

---

## 定位

apikey 只管**钥匙本身**的生命周期：存它、加密它、测它活没活、按 id 发它。
「该用哪把钥匙」「这把钥匙背后有哪些模型 / 能力」全部**不在** apikey。

---

## 负责（4 件事）

| 能力 | 说明 |
|---|---|
| **CRUD + 加密** | Create（明文加密入库 + 脱敏 KeyMasked）/ Update（含 key 旋转：重新加密+重置 test 状态）/ Delete（带引用检查 → ErrInUse）/ Get / List（分页）|
| **哑探测** | 按家发探测请求（端点+认证各家不同）→ 判 HTTP 200 → 存「状态 + **原始返回** + 延迟 + 时间」。**零解析** |
| **按 id 发钥匙** | `ResolveCredentialsByID(id)` —— 唯一精确出口，解密递出明文凭证包 |
| **按 id 标失效** | `MarkInvalidByID(id)` —— 调用方 401/403 时按 id 精确标记 |

## 不负责（全部下放）

- **选哪把 key**：LLM → **model 模块**（per-scenario api_key_id + 对话/节点 override）；搜索 → **未来搜索配置模块**（显式配 api_key_id，不启发式，防乱烧钱）
- **模型理解**（有哪些模型 / 上下文窗口 / effort / 1M / option）→ **model 模块**（吃 apikey 存的 `test_response` 原始返回 + 静态目录兜底）
- **搜索返回理解** → 搜索模块

---

## 测试哲学：哑探针

- **「怎么探」留 apikey**：各家端点 + 认证方式（openai 打 `/models` Bearer、anthropic 打 `/v1/messages` ping、google key 在 query、ollama `/api/tags`、brave `X-Subscription-Token`、tavily key 在 body…）。发请求是测试动作本身。
- **「怎么读返回」全下放**：apikey 看完 HTTP 状态码就完事，body **原样存进 `test_response`**，绝不拆。
- 探测方式（保留，各家自包含）：`get_models / anthropic_ping / google_list_models / ollama_tags / custom / search_ping / always_ok`。
- 后果：anthropic ping 拿不到模型列表 → `test_response` 里没模型 → model 模块解析时发现空 → 用**静态目录兜底**（缺口在 model 轮修，不在 apikey）。

---

## 实体字段（去 GORM，db tag）

```
APIKey{
  ID            ws_<没有，是 aki_<16hex>>          db:"id,pk"
  WorkspaceID   workspace 隔离                      db:"workspace_id,ws"  json:"-"
  Provider                                          json:"provider"
  DisplayName   per-workspace 唯一                  json:"displayName"
  KeyEncrypted  密文                                json:"-"
  KeyMasked     脱敏展示                            json:"keyMasked"
  BaseURL / APIFormat                               连接所需
  TestStatus    ok/error/pending                    能不能用
  TestError     失败原因
  TestResponse  原始返回 body（成功存上游 JSON）     ← 替代旧 ModelsFound
  LastTestedAt
  CreatedAt / UpdatedAt / DeletedAt(软删)
}
```

**删**：`ModelsFound`（解析后列表 → 改 `TestResponse` 原始）、`IsDefault`（选择策略 → 搜索配置）。
**唯一性**：`(workspace_id, display_name)` partial unique index（active）。

---

## KeyProvider 端口（28 模块依赖，收窄）

```
旧：ResolveCredentials(provider) / ResolveCredentialsByID(id) / MarkInvalid(provider) / DefaultSearchProvider()
新：ResolveCredentialsByID(id) / MarkInvalidByID(id)     ← 全按 id，精确
```
LLM 28 消费者本就按 id 拿（model 配置/override），不受影响；唯一用 provider 版的 WebSearch 在波次 2，随搜索配置重建。

---

## 复用盘点（设计原则 #8）—— apikey 是 orm 自动隔离首个真正受益者

| 旧手写样板 | backend-new 复用 |
|---|---|
| 满屏 `RequireUserID + Where("user_id=?")` | **orm 自动 workspace 隔离**（全删）|
| 手搓 `isDisplayNameConflict` 字符串 | **orm `ErrConflict`**（M1.1 已补）→ `ErrDisplayNameConflict` |
| 手写 tuple cursor 分页 | **orm `Page()`** |
| gorm 时间戳/软删 | orm 自动 |
| 加密 | `domain/crypto.Encryptor`（M0.3）|
| id | `idgen.New("aki")` |

**正确的不复用**：tester 不走 infra/llm（探连通 ≠ 调 generate；search 不在 llm）；provider 元数据暂不与 llm registry 合并（关注点不同，观察点）。

---

## RefScanner（删除引用检查，DIP 端口）

Delete 前查 model_config / conv override / node override 是否引用此 key → ErrInUse。
**保留 DIP 端口**，具体 scanner 实现注入留 model/conversation/workflow 各轮 + M7 wiring（同 workspace 的 WorkspaceResolver 套路）。
