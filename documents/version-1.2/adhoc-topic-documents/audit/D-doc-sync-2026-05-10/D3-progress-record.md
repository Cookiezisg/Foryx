# D3 — progress-record.md vs git log gap audit

**Date**: 2026-05-10
**Scope**: 最近 100 commits (从 `99b274c` 2026-05-07 00:22 到 `5186a95` 2026-05-10 22:42) 与 progress-record.md 末尾 dev log 对账。
**判断标准**: §S19 dev log 节制（1-2 句、~30-100 字、含 [tag]/模块/关键数字）；commit-message-only 决策细节不必双登 dev log；但**架构变化 / 新 sentinel / 新功能模块 / 大重构 / 真 bug 修复 / 滚动 audit 进度**应记。

---

## 概况

progress-record.md 末尾 dev log 最近的合法日期段是 **2026-05-09** (5 entries) + **2026-05-08** (12 entries) + **2026-05-07** (3 entries)。**2026-05-10 整天 0 entries**——但 2026-05-10 当天 git log 录了 **31 commits**，含 D contract-doc audit + 滚动 A 阶段 audit + forge_redesign 设计 + B1-B4 handler audit。

更早的 2026-05-08 / 09 也有零散 commit 没记 dev log（testend M-series / 沙箱 conv env owner.ID 修 / Marketplace V3 准备 / 等）。

---

## Recent commits not in progress-record (gaps)

### 2026-05-10（**31 commits 全部漏记**——HIGH 严重）

| Commit | Date | Subject | Worth recording? | Severity |
|---|---|---|---|---|
| `5186a95` | 2026-05-10 22:42 | fix(doc-sync): close 7 HIGH + 11 MED from D contract-doc audit | ✅ 必记（用户特别点名 "D 阶段 5 commit 应有 dev log"；D contract audit 18 fix 是文档同步纪律 #7 的执行） | **HIGH** |
| `872a265` | 2026-05-10 18:38 | docs(cmd): close 2 LOW from cmd audit (server + resources) | ⚠️ 滚动 audit batch 之一 — 与其他 audit 一起合并记一行 OK | LOW |
| `3f89c03` | 2026-05-10 18:29 | fix(handlers-B4): close 3 MED + 7 LOW from handlers-B4 audit (mcp+dev) | ✅ 必记（B 系列 audit 4 batch 有完整滚动） | **MED** |
| `a183b16` | 2026-05-10 18:13 | fix(handlers-B3): close 1 MED + 7 LOW from handlers-B3 audit (3 files) | ✅ 必记 | **MED** |
| `8d7f797` | 2026-05-10 17:55 | fix(handlers-B2): close 8 LOW from handlers-B2 audit (5 medium files) | ✅ 必记 | **MED** |
| `905d141` | 2026-05-10 17:35 | docs(handlers-B1): close 2 LOW from handlers-B1 audit (10 small files) | ✅ 必记（B 系列 audit 完整链） | **MED** |
| `87b9fe7` | 2026-05-10 17:22 | fix(eventlog): close 1 MED + 3 LOW from pkg-eventlog audit | ✅ 滚动 audit 一项 | LOW |
| `9cb09b2` | 2026-05-10 17:08 | fix(installprogress): close 1 LOW StopBlock ctx-asymmetry — use Background | ⚠️ 单 LOW fix，可合并 | LOW |
| `a13c21d` | 2026-05-10 16:57 | fix(pkg/llmclient): close 4 LOW %w:%v + drop pkg-{notifications,pathguard} traces | ⚠️ 滚动 audit | LOW |
| `2a0a1a0` | 2026-05-10 16:49 | docs(forge_redesign): plans 02-06 — full implementation roadmap | ✅ **必记**（5 份新设计文档 plans 02-06 落档，§S14 文档同步纪律要求） | **HIGH** |
| `5362abb` | 2026-05-10 16:47 | docs(audit): pkg/{idgen,llmparse,pagination} traces — 0 HIGH/MED, 4 LOW | ⚠️ 滚动 audit | LOW |
| `4bc237f` | 2026-05-10 16:42 | docs(audit): transport/{middleware,response,router} traces — 0 HIGH/MED, 4 LOW | ⚠️ 滚动 audit | LOW |
| `41d9212` | 2026-05-10 16:31 | docs(forge_redesign): plan 01 — Function domain implementation | ✅ **必记**（forge_redesign Function domain plan 落档） | **HIGH** |
| `d7360e3` | 2026-05-10 16:29 | docs(audit): app-model + app-conversation traces — both clean (0 violations) | ⚠️ 滚动 audit | LOW |
| `406dfb4` | 2026-05-10 16:23 | docs(audit): app-todo traces — textbook clean (0 violations) | ⚠️ 滚动 audit | LOW |
| `f98c152` | 2026-05-10 16:20 | docs(forge_redesign): trinity architecture spec — Function/Handler/Workflow | ✅ **必记**（trinity architecture spec 是大方向决策，Phase 4/5 重定向） | **HIGH** |
| `a70d73a` | 2026-05-10 16:18 | fix(subagent): close 2 MED §S9 emit-side detach + mapEventLogStatus drift | ✅ 必记（§S9 detached ctx 真 bug + mapEventLogStatus drift） | **MED** |
| `2d47cb0` | 2026-05-10 16:02 | fix(catalog): close MED — ErrAllSourcesFailed sentinel + errmap | ✅ 必记（新 sentinel + errmap 行——§S14 三处联动） | **MED** |
| `75b1b6e` | 2026-05-10 15:49 | docs(audit): app-tool-skill traces — clean (0 HIGH/MED, 4 LOW WAIVE) | ⚠️ 滚动 audit | LOW |
| `c4f714d` | 2026-05-10 15:43 | docs(audit): app-tool-todo traces — clean (0 HIGH/MED, 4 LOW WAIVE) | ⚠️ 滚动 audit | LOW |
| `54ab931` | 2026-05-10 15:36 | docs(errmap): fix stale subagent comment naming nonexistent sentinels | ⚠️ 单 doc-fix，可合并 | LOW |
| `120e108` | 2026-05-10 15:35 | docs(audit): app-tool-subagent traces — textbook clean (0 violations) | ⚠️ 滚动 audit | LOW |
| `e423101` | 2026-05-10 14:53 | docs(audit): app-tool-ask traces — clean (0 HIGH/MED, 1 LOW) | ⚠️ 滚动 audit | LOW |
| `fdf3d7b` | 2026-05-10 14:33 | docs(audit): drop app-tool-mcp traces — §S18 textbook-clean | ⚠️ 滚动 audit | LOW |
| `7f3ef2c` | 2026-05-10 14:27 | fix(skill): close 2 MED %w:%v sentinel-chain truncation in scan.go | ✅ 必记（真 §S16 sentinel-chain 修复） | **MED** |
| `64d9535` | 2026-05-10 14:13 | fix(tool/forge): close 2 §S3/§S17 LOW from app-tool-forge audit | ⚠️ 滚动 audit | LOW |
| `57b916e` | 2026-05-10 13:58 | docs(audit): drop app-tool-filesystem traces — architecturally clean | ⚠️ 滚动 audit | LOW |
| `7dba737` | 2026-05-10 13:36 | fix(tool/web): close 2 MED — sentinel-based MarkInvalid + non-silent BYOK fail | ✅ 必记（新 sentinel + §S3 修复） | **MED** |
| `f9d0265` | 2026-05-10 13:16 | fix(tool/search+shell): close 1 HIGH + 8 LOW from app-tool audits | ✅ **必记**（HIGH 修复——audit 找到的真 bug） | **HIGH** |
| `d2b8af8` | 2026-05-10 12:55 | fix(sandbox): close infra-sandbox EDGE — §S16 prefixes + scanner.Err | ⚠️ 滚动 audit | LOW |
| `363b084` | 2026-05-10 12:34 | fix(llm): close 17 LOW from infra-llm audit — sentinel + prefix sweep | ✅ 必记（17 LOW 大批 sentinel 整理） | **MED** |

### 2026-05-10 早间（02:00-02:18，**5 commits 漏记**）

| Commit | Date | Subject | Worth recording? | Severity |
|---|---|---|---|---|
| `d6b626f` | 2026-05-10 02:18 | fix(sandbox): close 4 MED %w:%v sentinel-chain truncation in install paths | ✅ 必记（§S16 sentinel-chain 真 bug 修复 4 处） | **MED** |
| `da425c9` | 2026-05-10 02:00 | docs(audit): drop app-forge traces — CRITICAL B1 fixed, others FOUND | ⚠️ 滚动 audit drop | LOW |
| `ff8fd77` | 2026-05-10 01:56 | fix(forge): owner.ID separator `:` → `_` to match B1 + ErrInvalidOwnerID | ✅ 必记（CRITICAL bug——sandbox owner.ID 解析冲突；新 sentinel） | **HIGH** |
| `94ab56a` | 2026-05-10 01:48 | fix(llm+errmap): introduce HTTP-status sentinels for provider errors | ✅ 必记（新 HTTP-status sentinel 家族——§S17 errmap 单一事实源） | **HIGH** |
| `0d4a48e` | 2026-05-10 01:31 | fix(sandbox): close 6 §S16 wrap-format LOW + adjust empty-Cmd to sentinel | ⚠️ 滚动 audit | LOW |

### 2026-05-10 凌晨（00:00-00:41，**6 commits 漏记**）

| Commit | Date | Subject | Worth recording? | Severity |
|---|---|---|---|---|
| `e36f890` | 2026-05-10 00:41 | fix(sandbox): close §S9 ready/failed + §S17 sentinel gaps from audit | ✅ 必记（§S9 detached ctx + §S17 sentinel） | **MED** |
| `e5b65fa` | 2026-05-10 00:37 | fix(chat) + docs(audit): close chat host.go #8 + finalize batches 1-2 statuses | ⚠️ 滚动 audit closeout | LOW |
| `505d6e3` | 2026-05-10 00:31 | fix(mcp+loop): close §S16 wrap-format + Marshal-discard polish from audit | ⚠️ 滚动 audit polish | LOW |
| `4f147b9` | 2026-05-10 00:17 | docs(audit): drop app-mcp + app-loop traces — batch 1 partial closeout | ⚠️ 滚动 audit drop | LOW |
| `26f9c55` | 2026-05-10 00:13 | fix(mcp+loop): close §S3 + §S16 findings from audit batch 1 | ✅ 必记（§S3 错误吞 + §S16 wrap 修复） | **MED** |
| `054e242` | 2026-05-10 00:04 | docs(audit): close app-chat audit — 13 FIXED, 2 LOW pending review | ⚠️ 滚动 audit | LOW |
| `f272503` | 2026-05-10 00:00 | fix(chat): close §S9 + §S3 + §S16 findings from app-chat audit | ✅ 必记（§S9 detached ctx + §S3 真 bug 修复——chat 是 hot path） | **HIGH** |

### 2026-05-09 晚间（**漏记 2 commits**）

| Commit | Date | Subject | Worth recording? | Severity |
|---|---|---|---|---|
| `409f8c7` | 2026-05-09 23:31 | docs(audit): close out apikey calibration — 5 FIXED + 2 WAIVED | ⚠️ apikey audit closeout——line 402 的 calibration 入口已记，closeout 可省 | LOW |
| `1b96a5e` | 2026-05-09 23:29 | fix(apikey): close 5 LOW §S16 wrap-format inconsistencies from audit | ⚠️ 5 LOW 风格修复——line 402 dev log 提到 "8 LOW 待用户拍"，结论可补一行 | LOW |

### 2026-05-09 中午（**漏记 3 commits**）

| Commit | Date | Subject | Worth recording? | Severity |
|---|---|---|---|---|
| `888739c` | 2026-05-09 17:21 | fix(sandbox+bash): 进度共享 helper + 错误 surface + sandbox_env notif | ✅ 必记（3 个修复 + 新 notification type sandbox_env） | **MED** |
| `3cdf18a` | 2026-05-09 16:47 | fix(sandbox): conv env owner.ID 用 _ 替 : 解 PATH 冲突 | ✅ 必记（PATH 冲突真 bug；与 ff8fd77 owner.ID 修复同源） | **MED** |
| `9789b19` | 2026-05-09 01:11 | fix(sandbox): reset-all conv envs 路由注册漏 :reset-all 后缀 | ⚠️ 单端点漏注册 fix——可合并 | LOW |

### 2026-05-09 凌晨 testend M-series（**12 commits 全部漏记**）

| Commit | Date | Subject | Worth recording? | Severity |
|---|---|---|---|---|
| `8f9162c` | 2026-05-09 01:37 | refactor(testend): M14 — wire tab 事件级 err 跳空对象 | ⚠️ M-series 应汇总记一行（"testend M1-M14 polish 14 件"） | LOW |
| `accb944` | 2026-05-09 01:31 | fix(testend): M13 — store 加 activeRightTab 修跨 4 tab polling | ⚠️ M-series | LOW |
| `c4ac075` | 2026-05-09 01:28 | fix(testend): M12 — 测试集合 2 处过时 /api/v1/events 引用 | ⚠️ M-series | LOW |
| `133843c` | 2026-05-09 01:25 | fix(testend): M11 — sql tab schema 漂移修对 | ⚠️ M-series | LOW |
| `c144d59` | 2026-05-09 01:20 | refactor(testend): M10 — logs tab 防御性 parse + 连接状态指示 | ⚠️ M-series | LOW |
| `759157b` | 2026-05-09 01:16 | refactor(testend): M9 — catalog tab 3 处文案过时清理 | ⚠️ M-series | LOW |
| `8e8e051` | 2026-05-09 01:14 | feat(testend): M8 — skill tab 补 6 个未渲染的 frontmatter 字段 | ⚠️ M-series | LOW |
| `96a312e` | 2026-05-09 00:54 | refactor(testend): M6 — mcp tab 去除 5000+ 残留 + reconnect/delete 错误反馈 | ⚠️ M-series | LOW |
| `a6f1252` | 2026-05-09 00:49 | refactor(testend): M5 — config tab 加错误反馈 | ⚠️ M-series | LOW |
| `6676211` | 2026-05-09 00:45 | feat(testend): M4 — notifs tab 重做为统一 SSE + toast 语义 feed | ⚠️ M-series | LOW |
| `3bcdff4` | 2026-05-09 00:39 | feat(testend): M3.5 — chat 渲染 subagent 嵌套 + progress 独立 block | ✅ **必记**（chat.js 大改——subagent 嵌套渲染 + progress 独立 block） | **MED** |
| `c7a846d` | 2026-05-09 00:33 | refactor(testend): M3 — chat.js 6 件润色 | ⚠️ M-series | LOW |
| `52ca54a` | 2026-05-09 00:26 | feat(testend): M2 — SSE tab 重写为 raw 两通道 viewer | ⚠️ M-series | LOW |
| `b1f050e` | 2026-05-09 00:20 | refactor(testend): M1 — 删 subagent 死 tab | ⚠️ M-series | LOW |

**M-series 整体合并** ：testend 14 件 polish 应一条 dev log 总结（"M1-M14 testend 全 tab 适配事件日志协议 + 错误反馈 + 死代码清"），LOW 单条不必逐个记，但 M3.5 (chat subagent 嵌套渲染) 是较大改动应在汇总里点一句。

### 2026-05-08（**漏记 6 commits**）

| Commit | Date | Subject | Worth recording? | Severity |
|---|---|---|---|---|
| `49792c5` | 2026-05-08 23:51 | feat(mcp): gmail entry → google-workspace（taylorwilsdon, 全套 + 真维护）| ⚠️ marketplace registry entry 替换——Tier 2 OAuth；可合并 V3 | LOW |
| `8a9b853` | 2026-05-08 23:27 | test(mcp): AllSmoke 真验装机路径——stub 凭证 + 严守测试作者 bug | ✅ 必记（pipeline test AllSmoke + 严守测试作者 bug 是 §T6 实践） | **MED** |
| `a75dde5` | 2026-05-08 23:10 | test(mcp): curated marketplace pipeline — 21 smoke + 5 T0 live tool calls | ✅ 必记（21 + 5 T0 pipeline test 是大覆盖批次） | **MED** |
| `b22417d` | 2026-05-08 16:47 | feat(ask): testend AskUserQuestion 交互 UI + 描述强调用户自由输入 | ✅ 必记（AskUserQuestion UI——§4 反前端规则的临时例外但有功能性） | **MED** |
| `3c50b8c` | 2026-05-08 16:37 | refactor(mcp): marketplace 改 search-only + 修 v0.1 schema 真实形状 | ✅ 必记（marketplace V3 雏形——search-only + v0.1 schema 修） | **MED** |
| `fa9b8c4` | 2026-05-08 16:13 | fix(chat+notifications): user-message render race + emit/notification 路径接通 + pipeline 修复 | ✅ 必记（user-message render race 真 bug + pipeline 修复——chat hot path） | **HIGH** |

---

## Stale entries (claimed work but code shows otherwise)

| progress-record entry | Status | Issue |
|---|---|---|
| line 408 `2026-05-08 [refactor] Marketplace V3 — curated 21` 提到"删 OfficialRegistrySource + FakeRegistrySource" | ⚠️ 时序倒置 | line 408 在文件顺序上排在 line 391 (`Marketplace V2`) 之**后**，但内容上是 V2 → V3 的演化。dev log 是按时间顺序写，但 V2 (commit 53b805e 2026-05-07 20:18) 与 V3 第一阶段 (3c50b8c 2026-05-08 16:37) + V3 curated 21 (862f960 2026-05-08 21:38) + V3 search→list (ede777d 2026-05-09 18:07) 跨多个 commit。当前 dev log 把 V3 的 3 个 commit 混合到 1 条 line 408，**漏记中间** 3c50b8c (search-only + v0.1 schema 修) 这一笔；建议按 commit 拆 |
| line 391 `2026-05-08 Marketplace V2` 提到 "MCPTools 工厂从 2 工具变 5 工具" + "search_mcp_marketplace" + "install_mcp_server" 5 工具新增 | ⚠️ 已被 V3 撤销 | V3 (line 408) 把 search_mcp_marketplace → list_mcp_marketplace、删 alias、改 InstallFromRegistry 签名——V2 dev log 还说 5 工具新增、双 namespace+alias，但 V3 后这部分已被推翻；不算 stale (历史叙事保留 OK)，但可在 line 408 末尾点一句 "撤回 V2 的 alias 双命名" |
| line 402 `[audit + fix] Phase A1.1 calibration shot` 写 "8 LOW (style 一致性，待用户拍)" | ⚠️ 状态过时 | 这 8 LOW 在 1b96a5e (5 LOW §S16 wrap-format) 已修了 5 + 409f8c7 closeout (2 WAIVED)；line 402 应有 follow-up 一行 "5 FIXED + 2 WAIVED + 1 ?" 或直接合并到原条目末尾 |
| line 404 `[fix] testend 进入后所有按钮静默卡死` 提到 "tab-sse.js 删" | ✅ OK | 与 commit 0b86083 一致，无 drift |

---

## Sub-check

- **Total commits in last 100**: 100
- **Time span covered by dev log**: 2026-05-07 ~ 2026-05-09 (3 整天 + 部分 05-08)
- **2026-05-10 dev log entries**: **0**（应有 ≥5 entry，至少滚动 audit 进度 + D contract sync + forge_redesign 3 plan + B1-B4 audit + 24:00 时段的 audit batch closeout）
- **Commits with dev log**: ~37（大部分是 2026-05-07/08 的 TE-15 ~ Marketplace V3 + 一部分 2026-05-09 的）
- **Commits worth-recording but missing**: **63**（其中按严重度统计如下）
- **Stale entries**: 3 处

### 严重度小结

| Severity | Count | 备注 |
|---|---|---|
| **HIGH** | **8** | D contract-doc audit (5186a95) / forge_redesign trinity spec (f98c152) + plans 01 (41d9212) + 02-06 (2a0a1a0) / app-tool HIGH fix (f9d0265) / chat §S9+§S3+§S16 (f272503) / forge owner.ID CRITICAL (ff8fd77) / llm HTTP-status sentinels (94ab56a) / chat user-message race (fa9b8c4) |
| **MED** | **17** | handlers-B1-B4 audit (905d141 / 8d7f797 / a183b16 / 3f89c03)、subagent §S9+drift (a70d73a)、catalog ErrAllSourcesFailed (2d47cb0)、skill scan.go §S16 (7f3ef2c)、tool/web sentinel (7dba737)、infra-llm 17 LOW (363b084)、sandbox 4 MED §S16 (d6b626f)、sandbox §S9 (e36f890)、mcp+loop §S3 (26f9c55)、sandbox+bash 进度+notif (888739c)、sandbox conv env owner.ID (3cdf18a)、testend M3.5 chat subagent 嵌套 (3bcdff4)、AllSmoke (8a9b853)、curated pipeline (a75dde5)、AskUserQuestion UI (b22417d)、marketplace search-only (3c50b8c) |
| **LOW** | **38** | 滚动 audit drop / 单 LOW fix / testend M-series 13 件（M3.5 已计 MED）/ doc-fix / 等 |

---

## 行动建议（不修文档，仅供参考）

按严重度合并写 dev log：

1. **2026-05-10 audit 滚动**：1 条总结记录 50 traces audit + 10+ FIX commit + B1-B4 handlers + cmd / pkg-{idgen,llmparse,pagination} / transport audit 等总 audit 进展（HIGH 可单写一条；MED 集中一条；LOW drop 不必逐条记）
2. **2026-05-10 forge_redesign**: 1 条 [doc] 记 trinity architecture + plan 01 + plans 02-06 落档（"完整方案见各文件"）
3. **2026-05-10 D contract-doc audit**: 1 条 [doc-fix] 记 7 HIGH + 11 MED contract-doc gap close
4. **2026-05-09 凌晨 testend M-series**: 1 条 [refactor + feat] testend M1-M14 14 件 polish（点一句 M3.5 chat 嵌套渲染 + M4 notif tab 重做）
5. **2026-05-09 17:00 sandbox 修系列**: 1 条 [fix] sandbox conv env owner.ID 解 PATH 冲突 + 进度共享 helper + 路由漏 :reset-all（3 commit 合并）
6. **2026-05-08 16:00-23:51 chat+mcp 系列**: 已记 V3 curated 大条目，需补 (a) chat user-message race fix、(b) AskUserQuestion UI、(c) marketplace v0.1 schema 修、(d) AllSmoke + curated pipeline test、(e) gmail → google-workspace
