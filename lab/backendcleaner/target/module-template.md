# Canonical 模块骨架（clean arch，按需取层）

> 后端架构必须一致：同类东西同样组织、分层职责与命名统一。
> 但**一致 ≠ 强行套层**——没状态的能力不塞 store，没 HTTP 的不塞 handler。
> 模板规定的是"如果有这一层，它叫什么、职责到哪"，不是"必须有这一层"。

---

## 分层职责（依赖方向：transport → app → domain ∪ infra/store；infra/store → domain；domain 不依赖任何人）

| 层 | 路径 | 职责 | 禁止 |
|---|---|---|---|
| **domain** | `internal/domain/<m>/` | 实体（带 GORM tag，一份到底）+ Repository 接口 + 领域错误 sentinel + 纯领域逻辑/校验 | **禁止 import `app/` 或 `infra/`**（go list 会查） |
| **store** | `internal/infra/store/<m>/` | 实现 `domain.<M>.Repository`，GORM CRUD + 游标分页 | 不做业务编排 |
| **app** | `internal/app/<m>/` | Service：协调本域 domain + 别的 service（**接口注入，不 import 兄弟 service 的具体类型**）。业务编排在此 | 不碰 HTTP/SQL 细节 |
| **tool 适配** | `internal/app/tool/<m>/` | 把能力暴露为 Tool（S18 九方法接口）。import `app/<m>` + `app/tool` 基础 | — |
| **transport** | `internal/transport/httpapi/handlers/<m>.go` | HTTP handler：解析 → 调 app → envelope。一个文件对应一个 API 资源域（S5） | 不写业务逻辑 |

## 按需取层

- 无持久化 → 无 `store/<m>`（例：纯计算/编排域）。
- 不暴露 HTTP → 无 `handlers/<m>.go`（例：`loop`、`contextmgr` 内部引擎）。
- 不作为 LLM 工具 → 无 `tool/<m>`。
- 反例（违背"不为架构化拆文件"）：给一个只有 3 个字段的配置域硬造 domain+store+app+handler 四层。

## 命名 / 规范（继承 CLAUDE.md，强制）

- **S5** 物理文件对齐：handler 文件名 = API 资源域；domain 文件名 = Repository 接口域。
- **S13** import 别名：`internal/` 包导入带 `<name><role>` 别名（`apikeydomain`、`chatapp`、`functionstore`）。
- **S11** 双语注释：`// English` 换行 `// 中文`；只写 Why 不写 What。
- **S15** ID 宪法：`<prefix>_<16hex>`，前缀登记进 `database.md`。
- **N1** envelope：成功 `{"data":...}` / 失败 `{"error":{code,message,details}}`。
- **N3** 线缆 camelCase / 物理列 snake_case。**N4** List 必须 `?cursor=&limit=`。**N5** 非 CRUD 用 `:action`。
- **D1** 业务表软删 `deleted_at`；Journal/Log 禁删。**D2** 除全局配置外都带 `user_id`。

## "干净"的可执行定义（每轮自证，详见 PLAYBOOK §干净）

- 读实现直接看出业务意图；函数短、一事一函数。
- 分支少，每个分支有当前产品理由；错误路径明确，不靠 silent fallback / best-effort / 兼容 alias 掩盖。
- 命名表达领域含义，少用 Manager/Helper/Util/Adapter。
- 抽象只在"减少真实重复 / 切明确边界 / 稳定外部契约"时用；不把旧复杂度搬进新 helper。
- 行数变少但理解成本变高 ≠ 干净；interface 变多但职责没更清楚 ≠ 干净。

## 一个全层模块的标准文件清单（参考）

```
internal/domain/<m>/<m>.go              实体 + Repository 接口 + 错误
internal/infra/store/<m>/<m>.go         Repository 实现
internal/app/<m>/<m>.go                 Service
internal/app/tool/<m>/<m>.go            Tool 适配（若是工具）
internal/transport/httpapi/handlers/<m>.go   HTTP handler（若有 API）
+ 各层 <m>_test.go（新标准，见 PLAYBOOK §测试）
```
