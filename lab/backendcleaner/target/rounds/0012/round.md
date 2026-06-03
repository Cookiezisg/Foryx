# Round 0012 — domain/errors 结构化强化（波次 0 · M0.4）

类型 / 目标：把错误体系从「transport 集中映射表」强化为「domain 错误自带语义」。

考古发现（强化动机）：
- domain/errors 旧版仅 2 个裸 sentinel；各 domain 用 `errors.New(msg)`。
- `transport/errmap.go` 293 行 `errTable` 集中映射 ~150 sentinel → `(status, code)`，**import 27 个 domain/app/infra 包**，`O(n)` errors.Is 查表。这是 transport 反向耦合全项目的最大单点。

强化设计：
- `Error{Kind, Code, Message, Details, cause}`：结构化 domain 错误，自带语义 Kind + 稳定 wire Code（N1）+ 可选 Details（N1 error.details）+ cause。
- `Kind`：14 个语义分类（零值 = KindInternal = 500 最安全默认），注释即权威 HTTP 映射。
- `New(kind,code,msg)` 构造；`Is` **按 Code 匹配**（sentinel 与 WithCause/WithDetails 副本、fmt-wrap 在 errors.Is 下仍相等）；`WithCause`/`WithDetails` 返不可变副本。
- 跨域 sentinel：`ErrInvalidRequest` + `ErrUnauthorizedNoWorkspace`（user→workspace 正名）。

收益：transport 将**零 import 业务 domain**；错误码契约内聚 domain；`O(1)`（errors.As 直接拿 Code/Kind/Details）；N1 details 用起来。

落地（全局方针）：各 domain 轮把 `errors.New(msg)` → `errors.New(kind, code, msg)`；transport errmap M0.7 塌缩成 `statusForKind(Kind)` + `errors.As`（零 domain import）。

契约变更：`UNAUTH_NO_USER` → `UNAUTH_NO_WORKSPACE`（wire code + 联动 header `X-Forgify-User-ID`→`X-Forgify-Workspace-ID`）→ 记 contract-changes #1。

测试：6（字段/Message、WithCause+Unwrap、Is by Code（直接/clone/fmt-wrap）、不同 code 不匹配、WithDetails 不可变、sentinel）。

验证：`gofmt`/`go build ./...`/`go vet`/`go test` 绿。

是否更干净：✅ 错误语义归 domain，根除 errmap 293 行 + 27 import 巨耦合点。

覆盖状态：domain/errors cleaned（强化）。errmap 塌缩 + 各 domain error 改造入 deps-todo / 全局方针。

下一步：M0.4 续 `domain/eventlog` + `domain/notifications`。
