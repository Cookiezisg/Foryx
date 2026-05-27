# 前端 Revamp 阶段 5:收尾严格化 + 前端文档体系 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: 用 superpowers:subagent-driven-development 逐 task 执行。

**Goal:** revamp 收官——① **代码严格化**:全 TS strict(87 个 .jsx/.js → .tsx/.ts + `tsconfig strict:true`)、steiger 真零违规(解 4b 留的 17 处 entities→features/pages 反向)、删死代码;② **建前端规范化文档体系**(对标后端 `documents/version-1.2/`):总览 + 契约索引 + 按 slice 详设计 + CLAUDE.md 文档纪律 + PRD 同步。

**Architecture:** 不改 FSD 结构/行为(strict 只加类型;文档描述已稳定的阶段 0-4b 架构)。Part A(代码)与 Part B(文档)互不依赖,可任意序。

> **范围**:① 严格化**行为不变**(vitest 全程绿);② 文档对标后端体系;③ **进展统一进现有 `progress-record.md`,不单建 frontend-progress**;④ 规则进 CLAUDE.md(**文档最高优先级**)。

---

## 通用纪律
独占 main,**严禁开新分支**;**精确 git add**(严禁 `git add -A`——工作树有 probe 探针 + backend audit 残留);commit 前 `git status` 核对;**绝不碰 backend/**;commit 中文无 AI attribution;commit 后 `git push`;撞 `index.lock` 先 `ps aux | grep -E "[g]it (commit|add)"` + mtime 确认孤儿(Zed 短锁)才 `rm -f`。`git mv` 改扩展名保 history。**每 task 末**:tsc 0 / vitest 不减(当前 756)/ build / eslint 0 / fsd 干净。

---

# Part A:代码严格化(行为不变)

## Task 5.1:解 17 处 entities→features/pages 反向 + steiger 真零违规
**Files:** 重新归类 detail 组件;`steiger.config.js`(移 ignore);`eslint.config.js`(补规则)。
- [x] Step 1:`npm run fsd` 临时去 ignore 看 17 反向具体(预期 `entities/{function,handler,workflow}/ui/*Detail` 用 `features/forge-review`;`entities/flowrun/ui/{FlowRunDetail,RunDrawer}`+`entities/document/ui/DocEditor` 引用 pages/features)。
- [x] Step 2:**重新归类**(组合了 feature/page 的 detail 不是纯 entity 视图,FSD 上移):用 forge-review 的 3 个 Detail → `pages/forge/ui/`;FlowRunDetail/RunDrawer → `pages/execute/ui/`;DocEditor → `pages/library/ui/`。调用点 import 更新。**渲染 1:1 不变**。
- [x] Step 3:`eslint.config.js` 补:`entities` disallow features/pages/widgets/app;`features` disallow features(@x 例外)/widgets/pages/app。`steiger.config.js` 移除 17 反向 ignore(insignificant-slice/inconsistent-naming 能解则解,合理的留注释)。
- [x] Step 4:验证门 + `npm run fsd` 真 ✔ + eslint 0。commit `refactor(frontend): detail 归类 pages/ui,解 17 反向,steiger 真零违规(阶段5)` + push。✅ 完成。steiger `No problems found`。

## Task 5.2:死代码 + 非组件 .js→.ts(22 个)
- [x] `find src -name "*.js" | grep -v .test` 22 个非组件 js → `.ts` + 加类型;清死代码(leftPct 重复等,`tsc noUnusedLocals` 辅助)。验证门。commit `refactor(frontend): 非组件 .js→.ts + 清死代码(阶段5)` + push。✅ 完成。

## Task 5.3:组件 .jsx→.tsx(65 文件,按层分批)
> `strict:false` 下先迁扩展名 + 加 props 类型,strict 修在 5.4。每层一 commit:
- [x] 5.3a `shared/ui/*` / 5.3b `widgets/**` / 5.3c `pages/**`(含 5.1 移入 detail)/ 5.3d `features/**/ui` / 5.3e `entities/**/ui` / 5.3f `app/*`
每批:`git mv` .jsx→.tsx + 组件 props interface(宽松,strict 收紧在 5.4)+ import 更新 + 测试同迁;验证门;commit `refactor(frontend): <layer> 组件 .jsx→.tsx(阶段5)` + push。✅ 完成。全 .tsx。

## Task 5.4:开 tsconfig strict + 修类型(渐进)
> strict:true 暴露大量类型问题,渐进开子项,每项修到 0:
- [x] Step 1:`noImplicitAny:true` → 修隐式 any。commit。
- [x] Step 2:`strictNullChecks:true` → 修 null/undefined(guard/可选链/收窄)。**guard 是类型层,不新增运行时防御**(no validation theater;后端必给的不加 fallback)。commit。
- [x] Step 3:剩余 strict 子项 + `strict:true` + 清 `allowJs` + `noUnusedLocals`。修剩余。
- [x] Step 4:验证门(strict 全开 tsc 0 / vitest 756 / build)+ **无 any 逃逸核查**(`grep ": any\| as any" src` 评估补全,spec §16)。commit `feat(frontend): tsconfig 全 strict + 消除 any 逃逸(阶段5)` + push。✅ 完成。tsc 0 / vitest 756 / build 绿。唯一 `as any` = Tiptap `NodeViewContent as {"code" as any}`(Tiptap 上游未导出 ElementType,带注释记录)。

---

# Part B:前端文档体系(对标后端 documents/version-1.2/)
> 格式对标:总览学 `backend-design.md`;契约学 `service-contract-documents/`;详设计学 `service-design-documents/<domain>.md`(端到端推演+实现清单);进展进现有 `progress-record.md`。**读后端对应文件作样板。**

## Task 5.5:frontend-design.md(架构总览)
- [x] Create `documents/version-1.2/frontend-design.md`,对标 backend-design:① 定位(Wails 桌面前端,FSD 6 层与后端 clean arch 同构)② FSD 6 层+依赖规则 ③ 横切机制(DIP 注入 authProvider/navigation=后端 port/wire;errorMap+全局 onError;SSE 三流 app/sse+状态下沉;toast;queryKeys)④ revamp Phase 路线(0-5+bug 根治)⑤ Verification(tsc strict/vitest/steiger/boundaries/make lint-frontend/wails dev)⑥ 文档分册结构。**不重复 CLAUDE.md 规则**。✅ 完成。

## Task 5.6:frontend-contract-documents/(契约索引,3 个)
- [x] `fsd-layers.md`:6 层职责+依赖规则表+每层 slice 清单+public API(index.ts)约定+boundaries/steiger 强制点。✅
- [x] `cross-cutting.md`:横切一眼索引——DIP ports(authProvider/navigation)、errorMap(code→key 表)、SSE 三流(app/sse+下沉 forgeProgress/chatStore)、queryKeys、toastStore、i18n。✅
- [x] `entity-types.md`:12 entity TS 类型 ↔ 后端 api-design 字段映射,协议变更同步点(F1)。✅

## Task 5.7:frontend-design-documents/<slice>.md(~20+,按 slice,分批)
> 对标 service-design domain 格式(端到端推演+实现清单)。分批:
- [x] 5.7a entities(~15):conversation(含 chatStore SSE 树/rAF/树重建)/function/handler/workflow/flowrun/document/skill/mcp/memory/relation/apikey/model-config/user/session(身份 resolve+DIP)/settings。✅ 15 个 slice 文档完成。
- [x] 5.7b features(8):send-message/onboarding/forge-iterate/forge-review/workflow-edit/settings/ask-user/entity-link(用例 hook 编排+意图 API+数据流)。✅ 8 个 slice 文档完成。
- [x] 5.7c widgets/pages/app:sidebar/entity-graph/command-palette/notifications-drawer(widgets)+chat/forge/execute/library/dashboard/observe(pages,props 化)+app-shell(中枢+props 注入+SSE 挂载+boot=session.status)。✅ 15 个 slice 文档完成。共 38 个 slice 文档。

## Task 5.8:CLAUDE.md 文档纪律 + FSD 宪法 + PRD/progress 同步
- [x] **CLAUDE.md**:① 前端守则加 **FSD 6 层宪法**(层定义+依赖规则+public API+DIP 模式+横切归属表,从 spec §3/§8 提炼)② **文档同步纪律升为最高优先级 + 覆盖前端**:任何前端改动同步 design-documents/<slice>+contract-documents+progress-record;给前端版触发表(对标后端 §S14)③ Verification 命令。✅
- [x] **frontend-prd.md** 同步:§1(改 TS+FSD)、§2(FSD 6 层目录)、§5(状态分层 entities/session+app/model+shared)、§17(API→指 entity-types)。产品需求保留。✅
- [x] **progress-record.md**:加前端 revamp 完整进展(阶段 0-5+bug 根治+D6 最规范架构),与后端并列。✅
- [x] commit `docs: CLAUDE.md FSD 宪法+文档最高优先级 + PRD/progress 同步(阶段5)` + push。✅

## Task 5.9:阶段 5 收口 + revamp 总验收
- [x] spec §16 验收逐条核(全 strict 无 any / steiger+eslint 零违规 / 每 slice public API / 组件零业务 / 身份 5→1+make clean 不风暴复核 / errorMap+onError / wails dev / vitest 全绿 / 门禁 / 文档同步)。✅ 全部通过，详见 progress-record 完工日志。
- [x] 全量验证:tsc strict 0 / vitest 756 / build / make lint-frontend 三段(exit 0)。✅
- [x] plan 勾选 + progress-record revamp 完工日志。commit `chore(frontend): 阶段5 收口 — revamp 总验收(全 strict + FSD 零违规 + 文档体系)` + push。✅

---

## Self-Review
- ✅ 全 strict(5.3 tsx+5.4 strict+any 核查);steiger 真零违规(5.1);死代码(5.2)。
- ✅ 文档体系(5.5 总览+5.6 契约+5.7 slice+5.8 CLAUDE/PRD/progress)对标后端四层。
- ✅ 文档最高优先级进 CLAUDE.md;进展统一 progress-record(用户指示)。
- 风险:strict 化(5.4)渐进开子项逐个修,行为不变靠 vitest;strictNullChecks guard 是类型层不加运行时防御;17 反向归类渲染 1:1;文档量大按批。
- 依赖:Part A/B 独立可并行;5.1 在 5.3 前;5.9 最后。
