---
id: WRK-031
type: working
status: active
owner: @weilin
created: 2026-06-20
reviewed: 2026-06-20
review-due: 2026-09-18
audience: [human, ai]
landed-into:
---

# 完整支持每一个 MCP（含 OAuth）—— 后端要做什么（需求清单，待核）

> **本文件是给「另一个人/agent 去核实」的需求清单**，不是已定方案。目标：让 MCP 市场里**每一个**服务器都能真正连上用——包括当前装上去就坏的 OAuth-only 远程服务器。
> 现状与结论的代码/规范依据见文末「核实清单 + 来源」，核实者请逐条对着仓库与 MCP 规范打勾。

---

## 状态（0620 更新）

**调研已完成 + 判据已精化 + 档 1 已落地。** 一次系统调研（每 server 一个 agent + 2 轮对抗 cross-check，297 个 agent）按**精化后的判据**分类全部 99 个 server。判据从「是否需要 auth」**改为「纯代码 vs 厂商业务步骤」**：写完整 OAuth 2.1 + PKCE + DCR 客户端算纯代码（做）；需 vendor 去厂商注册 app / 过审 / 进 allowlist / 架 proxy 算业务步骤（不做）；remote OAuth 的分界 = 是否支持 DCR。

结果：works-now 59 + static-token 25 + oauth-dcr 10 = **94 可用**；oauth-app-registration **5 永不做**。决策记入 [ADR 0006](../../decisions/0006-mcp-curated-whitelist.md)。

- **档 1（works-now + static-token，84 个）已实现**：`infra/mcp` 的 `CuratedCatalog` 白名单 + `catalog.json` auth 覆盖，结构性根治本文 §0/§3 的「静默坏连接」+「空 Authorization」两 bug。
- **档 2（oauth-dcr）= 本文 §1–§5 的 OAuth 客户端，已实现并纳入全部 10 个**：`infra/mcp/oauth`（纯协议：发现 RFC 9728/8414 → DCR RFC 7591 → PKCE RFC 7636 → 授权码 + 资源指示符 RFC 8707 → token 交换/刷新）+ `app/mcp/oauth_flow.go`（探测 + loopback 回调 RFC 8252 + 系统浏览器拉起 + token 加密存储/刷新/重存）。已纳入 atlassian/webflow/miro/amplitude/stackoverflow/wix/intercom/getguru/oakallow + **Glean**（每租户模板 URL 经安装时用户填 `GLEAN_MCP_URL` 解析——`Remote.URLEnv` 机制）。下面 §1–§5 即已落地的 OAuth 客户端规格（保留作参考）；§4 的 Google 等仍是「永不做」。**至此 94/99 可用，5 个永不做。**

---

## 0. 为什么要做（现状的洞）

当前后端 MCP 认证**只能表达两种静态字符串**：stdio 包的 `env` 变量 + remote 端点的固定 HTTP `header`（如 `Authorization: Bearer {TOKEN}`）。**全仓 MCP 三层 + transport 层 grep 不到任何 OAuth / authorize / refresh_token / redirect / callback / PKCE 处理**——「贴静态 token」是唯一认证手段。

后果（实测内嵌 `registry_snapshot.json` 99 个服务器）：

| 形态 | 数量 | 现状 |
|---|---|---|
| stdio-only（env 塞 token/key/连接串） | ~52 | ✅ 已支持 |
| both（stdio + remote） | ~9 | ✅ 走 stdio 那支 |
| **remote-only 且零静态 header** | **35** | ❌ **OAuth-only，且当前会「静默装一个零认证坏连接」——不报错、UI 不提示，真调用才 401** |

MCP 规范走向（2025-03 引入 → 2025-06 把 server 定为 OAuth Resource Server → **2025-11 收紧：任何可经互联网访问的 remote MCP server「必须」OAuth 2.1 + PKCE(S256)，无例外**）。同时规范明确 **stdio server 应从 env 取静态凭据、不走 OAuth**——所以我们对 stdio 的现有模型是合规的，洞只在 remote。

**「完整支持每一个 MCP」= 给后端补一套 MCP 客户端 OAuth 2.1 授权码流 + 修当前静默坏连接 + 处理 Google 这类不支持 DCR 的特例。**

---

## 1. 核心：实现一个 MCP 客户端 OAuth 2.1 + PKCE 流

按 MCP Authorization 规范，remote server 是 OAuth **Resource Server**（只验 token、不发 token），客户端要自己走完整发现 + 授权。需要补的组件（按调用顺序）：

1. **401 + `WWW-Authenticate` 发现**：调 remote server 收 `401` 时，解析响应头 `WWW-Authenticate` 拿到 protected-resource-metadata 地址。
   *（当前 `headerRoundTripper` 只无脑塞固定 header、不读 401 头——这是起点。）*
2. **Protected Resource Metadata（RFC 9728）**：`GET {server}/.well-known/oauth-protected-resource` → 拿到该资源信任的 **authorization server** 列表。
3. **Authorization Server Metadata（RFC 8414）**：`GET {as}/.well-known/oauth-authorization-server` → 拿到 `authorization_endpoint` / `token_endpoint` / `registration_endpoint` / 支持的 PKCE 方法。
4. **动态客户端注册 DCR（RFC 7591）**：`POST {registration_endpoint}` 自动注册一个 client → 拿 `client_id`（可能含 `client_secret`）。免去用户手填 client_id。
   *（不支持 DCR 的 AS——尤其 Google——走特例，见 §4。）*
5. **授权码 + PKCE（OAuth 2.1，S256）**：生成 `code_verifier`/`code_challenge`，构造 authorize URL（带 `redirect_uri`=本地回调、`resource`=目标 server、`scope`），**用系统浏览器打开**让用户同意。
6. **本地回调 server（桌面 loopback）**：临时起一个 `http://127.0.0.1:<随机端口>/callback` 接 redirect、取 `code`（带 `state` 校验防 CSRF）。
7. **Token 交换**：`POST {token_endpoint}`（`code` + `code_verifier` + `redirect_uri`）→ `access_token` + `refresh_token` + `expires_in`。
8. **Token 加密落盘**：per-server 存 `access_token`/`refresh_token`/`expiry`/`token_endpoint`/`scopes`/`client_id`(+`secret`)。复用现有 `config_enc`（AES-GCM + 机器指纹派生密钥）那套加密通道。
9. **Token 自动刷新**：`access_token` 临过期用 `refresh_token` 静默换新；刷新失败（refresh 过期/被撤）→ 标记需重新授权、提示用户再走一次 §5。
10. **Resource Indicators（RFC 8707）**：token 请求带 `resource` 参数，绑死 token 只能用于该 MCP server（防 token 被挪用到别的资源）。

> 这 10 条是一个**完整的 OAuth 2.1 客户端**，不是小补丁。

---

## 2. 数据模型改动

- **`mcp_servers` / `config_enc`**：现在只存 `{env, headers}` 两种静态串。要扩成能存 **OAuth 凭据束**：`client_id` / `client_secret?` / `access_token` / `refresh_token` / `token_expiry` / `token_endpoint` / `authorization_server` / `scopes`。建议仍走 `config_enc` 单列加密（别新开明文列）。
- **认证类型标记**：server 行需要一个 `auth_kind`（`none` | `static`(env/header) | `oauth`）让 install/运行时分流，并供 UI 显示「此服务器走 OAuth」。

---

## 3. registry 解析 + install 流程改动

- **registry 解析（`parseGitHub`）**：当前只映射 `name/description/packages/remotes` 4 个顶层字段，把 remote 的认证元数据全丢了。要么读上游 server.json 里的认证提示，要么接受「registry 不内联 OAuth 凭据（这是规范设计）→ 一律运行时 401 发现」。
- **install 形态分流**：一个 entry 可能同时有 `packages`（local stdio）和 `remotes`（remote URL）。
  - **优先选 `packages`（stdio）那支**——env/header 占位符就在这里、token-only 天然适配，能装的尽量走 stdio。
  - 只有 `remotes`、且 401 发现需要 OAuth → 走 §1 的 OAuth 装机流。
- **修当前 bug（即使不做完整 OAuth 也该先修）**：`Plan` 对「remote 且没有可填 header」的 entry 现在会**静默建一个空认证连接**。至少要改成：识别到「需要认证但收集不到静态凭据」时，**显式拒绝并提示「此服务器需 OAuth，当前不支持」**，别让用户以为装好了。
- **另一个小 bug**：声明了 header 但 `Value` 无 `{占位符}` 的（apify / monday），现在会产出 `Authorization: ""`（空 token）——收集逻辑要把这类也纳入「必填凭据」。

---

## 4. 特例与边界

- **Google 系（Drive / Gmail / Calendar）= 最硬的特例**：Google **无静态 token**（OAuth 2.0 唯一入口）**且不支持 DCR**。所以 §1 第 4 步（自动注册）对 Google 失效——必须要么走 **OAuth Proxy** pattern，要么**预置一个 Anselm 自己的 Google OAuth client**（client_id/secret 内置）。后者还要过 **Google 的 OAuth app 验证**（对本地/开源 app 是真门槛：敏感 scope 要审核）。**这是独立一块工程，建议单独评估、可后置。**
- **「OAuth 在服务器内部」≠ 需要我们做 OAuth**：有些 stdio 服务器声明 `OAUTH_CLIENT_ID`/`CLIENT_SECRET` 之类 env（如 CrowdStrike/zscaler 的 M2M client-credentials）——这是**服务器进程自己**去换 token，我们只要把 client_id/secret 当**静态 env** 塞给子进程即可，**不需要实现客户端授权码流**。这类已支持，别误判成要做 OAuth。
- **remote + 静态 Bearer 旁路**：不少产品（Notion 本地版 `ntn_`、Linear/Stripe/Supabase 的 PAT、Slack `xoxp`、Figma apikey 模式）有「贴静态 token」的旁路，**优先引导用户走这条**，能少做很多 OAuth。真正无旁路的目前主要是 Google 系 + 部分一方 hosted-only 端点（Notion hosted、Sentry remote、Cloudflare official）。

---

## 5. 桌面 app 特有要点（单进程单用户）

- **本地回调 server**：loopback redirect（`http://127.0.0.1:<port>/callback`），授权完即关。注意端口随机 + `state` 防 CSRF。
- **拉起浏览器**：Go sidecar 不直接开 UI——需通过既有 sidecar↔前端通道通知 Flutter 端打开系统浏览器到 authorize URL（或后端直接 `open`/`xdg-open`，看架构选择）。
- **token 存储**：机器级加密（复用 `config_enc` 的机器指纹派生密钥）——换机/重装后 refresh_token 不可解，需重新授权（与现有 API key「换机重填」一致）。

---

## 6. 工作量 / 风险（给排期参考）

- **是一个完整 OAuth 2.1 客户端**（发现链 + DCR + PKCE 授权码 + 本地回调 + token 生命周期 + 刷新），不是小改；和现有「无授权码流」是根本架构补强。
- **MCP OAuth 规范仍在演进**（2025-11 刚收紧），实现要跟规范、且预留升级。
- **Google 那块额外要过 Google app 验证**，是流程门槛不是纯代码。
- **分期建议**（仅陈述）：① 先修 §3 的静默坏连接（小、立刻改善 UX）→ ② 做通用 remote OAuth（DCR 的那批，解锁 Notion hosted / Sentry / Cloudflare / 多数一方 hosted）→ ③ 再单独评估 Google（OAuth proxy / 内置 client + 验证）。

---

## 7. 非代码前提（注册 / 审核 / 证书 / 运维）—— 这部分不是「写代码」

> OAuth 不全是后端写代码。**取决于对端 provider 支不支持 DCR**，分两档，差别巨大。

**档 A — 支持 DCR 的 remote server（MCP 规范鼓励的路子）= 基本纯代码、零业务前提**
- **DCR（RFC 7591）让客户端运行时自注册**——不用预先去 provider 申请 client_id、不用人工建 OAuth app。
- **PKCE（公共客户端）免 client_secret**——桌面 app 不用藏密钥。
- **本地回调走 loopback http（`127.0.0.1`）**，OAuth 原生 app 规范（RFC 8252）明确允许——**不需要 TLS 证书**。
- 这一档（实现了 DCR 的合规 remote server）写完代码就能用：无注册、无审核、无证书、无运维。

**档 B — 不支持 DCR + 要审核的 provider（Google / 微软 / 大消费级）= 一堆非代码的业务事**
- **去 provider 注册 OAuth app**：在 Google Cloud Console（等）建 OAuth client → 拿 `client_id`/`client_secret`。桌面 app 的 secret 嵌进去就不 secret 了（Google 也承认 installed-app 的 secret 不算秘密），靠 PKCE 兜——但你**必须先有这个 app**。
- **过 provider 的 app 验证**（敏感/受限 scope，如 Gmail 读、Drive 全量）：
  - 验证域名 + 隐私政策 + 主页 + 品牌（logo/名字）。
  - **第三方安全评估（Google CASA）**：受限 scope 必做，**按年收费、量级数万美元、周期数周到数月**（具体价格/流程请核实者查当前 Google 政策）。
  - 不过审 = **100 测试用户上限** + 用户看到**「未验证应用」吓人警告页**。
- **可能要自己跑一个托管 OAuth Proxy**（Google 不支持 DCR + 桌面端没法安全藏 secret，常见做法是中间放个代理）：
  - 需要**公网域名 + TLS 证书**——**你问的「搞证书」在这**：给这个 proxy 服务用的（Let's Encrypt 免费，但要有域名 + 一台长期跑的服务）。**本地回调那段不用证书。**
  - 性质等同免费档网关——一个要长期运维的服务。
- **每家 provider 各走一遍** + 持续维护（scope 变了要重审、secret 轮换、provider API/政策变更跟进）。

**另一类「证书」——桌面 app 分发的代码签名**（Apple Developer 证书 / Windows 代码签名证书）：跟 OAuth 没直接关系，是**发布 app** 的事；但沾边——签名 + 公证过的 app 更不容易触发「未知开发者/未验证」警告。属另一条线，独立预算。

**小结**：DCR 那批是纯代码（解锁不少 remote server）；**Google / 消费级这批，后端代码只是冰山一角**——大头是注册 OAuth app + 过安全审核（钱 + 数月）+ 可能托管带证书的 proxy + 长期运维，是公司/法务/预算级的事。这也正是「token-only 先跑」短期香的根本原因：它把这些全避开。

---

## 8. 核实清单 + 来源（请核实者逐条打勾）

**现状（对着仓库核）**
- [ ] MCP 三层 + transport grep `oauth|authoriz|refresh_token|redirect_uri|callback|pkce|well-known|token_endpoint|client_secret` 确无客户端 OAuth 流。
- [ ] 认证模型只有 `EnvVar`/`Header`（`domain/mcp/registry.go` 的 EnvVar/Header struct，`IsSecret` 仅驱动 UI 打码）。
- [ ] remote header 注入 = 固定 header 的 RoundTripper，每请求原样塞、永不刷新（`infra/mcp/client.go` 的 `headerRoundTripper`）。
- [ ] `Plan`（`domain/mcp/registry.go`）只把 `Value` 含 `{X}` 的 header 提为待填 EnvVar；零 header 的 remote entry 不报缺失 → 静默空认证连接。
- [ ] `resolveHeaders`（`app/mcp/install.go`）把无名 header 默认成 `Authorization`、占位符为空时产出空串。
- [ ] `parseGitHub`（`infra/mcp/registry.go`）只取 name/description/packages/remotes，丢弃上游其余认证元数据。
- [ ] 内嵌 `registry_snapshot.json` 里 remote-only 且零静态 header 的服务器数量（核实「35/99」这个量级）。

**规范（对着 MCP 官方 + RFC 核）**
- [ ] MCP Authorization 规范：remote server = OAuth Resource Server；2025-11 版「internet-accessible 必须 OAuth 2.1 + PKCE(S256)、无例外」；stdio 走 env、不做 OAuth。
- [ ] 发现链 RFC：RFC 9728（Protected Resource Metadata + `WWW-Authenticate`）、RFC 8414（AS Metadata）、RFC 7591（DCR）、RFC 8707（Resource Indicators）、PKCE。
- [ ] 哪些是 MUST vs SHOULD（决定最小可用集）。
- [ ] Google 确无静态 token、且不支持 DCR → 必须 OAuth proxy 或预置 client + app 验证。

**来源（调研已查，供核实者复核）**
- MCP Auth spec 解读：Auth0 *MCP Specs Update June 2025*；Descope *MCP Auth Spec*；Stack Overflow Blog *Auth in MCP (2026-01)*。
- registry/DCR：`modelcontextprotocol/registry`（DeepWiki）；Medium *MCP OAuth & DCR*。
- 连接器认证（token vs OAuth）：GitHub MCP docs（PAT）、makenotion/notion-mcp-server（本地 ntn_）、Linear docs（API key 旁路）、getsentry/sentry-mcp#833、Atlassian Rovo MCP auth、Stripe/Supabase MCP docs、korotovsky/slack-mcp-server。
- Google：Google Identity OAuth 2.0 docs、Google Cloud *Authenticate to Google MCP*、FastMCP Google integration（确认无静态 token + 无 DCR）。

---

## 9. 一句话给决策者

- **不做这块**：「市场 + token-only」短期可行——stdio + 静态 Bearer 旁路覆盖绝大多数热门产品，唯 Google 系彻底用不了；但要先修「静默坏连接」并对 OAuth-only 服务器明确标「需 OAuth、暂不支持」。
- **要做这块**：等于给后端补一套完整 MCP 客户端 OAuth（§1–§5），Google 再单列；解锁全部 remote 服务器，但工作量与维护成本都不小，且要跟着仍在收紧的 MCP 规范走。
