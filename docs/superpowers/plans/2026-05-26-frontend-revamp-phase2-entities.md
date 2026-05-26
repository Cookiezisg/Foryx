# 前端 Revamp 阶段 2:entities 层 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: 用 superpowers:subagent-driven-development 逐 task 执行。步骤用 `- [ ]` 勾选。

**Goal:** 把 9 个 `src/api/*.js` 业务文件 + `src/store/chat.js` 按 FSD 拆成 **12 个 entity slice**,每个 slice 定 `model/types.ts`(实体形状)+ `api/*.ts`(定型 hooks)+ `index.ts`(public API barrel),旧路径转 re-export shim。**行为/UI 零改动**(vitest 711 全程绿即证)。

**Architecture:** entities 是 FSD 第 2 层(对位后端 `domain`),**只能 import `shared`**(+ 同层 `@x` cross-import)。每 slice 结构 `api/ + model/ + index.ts`(阶段 2 不建 `ui/`,展示卡留阶段 3/4 组件归位时)。`conversation` 额外含 `model/chatStore.ts`(原 SSE 树 store,算法原样保留)。混装文件(`forge.js`/`config.js`/`library.js`/`relations.js`)拆分后退化成 re-export shim。

**Tech Stack:** TypeScript(`strict:false` 渐进、`allowJs`)、TanStack Query、zustand、react-i18next、vitest、steiger、eslint-plugin-boundaries。

---

## 通用纪律(每个 task 都遵守)

- **独占 main 工作树**,直接在 `main` 开发,**严禁开新分支**(多 session 共享工作树)。
- **精确 git add**:只 add 本 task 产物(列出的文件)。**严禁 `git add -A`/`git add .`**——工作树长期有其它在途文件 + `frontend/tests/manual/probe-*.mjs` 探针 + `backend/lintprompts`、`backend/server`(audit 残留),**绝不能带上**。commit 前 `git status` 核对暂存区。
- **绝不碰** `backend/` 任何文件。
- commit message **中文**、**无任何 AI attribution**(无 `Co-Authored-By`/`Generated with`)。
- commit 后 **`git push`**。撞 `index.lock`:`ps aux | grep "[g]it"` + 看 `.git/index.lock` 时间戳,确认无活动 git 进程的孤儿锁才 `rm -f`,**绝不盲删**。
- 所有命令在 `frontend/` 内跑(除非注明仓库根)。

## 通用搬迁 SOP(每个 entity 通用,样板见 Task 2.1)

> 每个 entity task 都按这 6 步;只有 Task 2.1 写出完整代码作模板,其余 task 引用本 SOP + 给「source 映射 / types 来源 / 特殊点」。

1. **定 `entities/<x>/model/types.ts`**:对照**后端 domain 设计文档** `documents/version-1.2/service-design-documents/<domain>.md` + **契约** `service-contract-documents/api-design.md` + 运行时响应 shape,手写实体 interface(对齐后端字段名 camelCase,见 §N3)。`strict:false` 下可适度宽松,但**不要裸 `any`**;不确定的字段用精确 union 或 `?:` optional,别用 `any` 兜底。导出所有相关类型(实体 + 子结构 + 请求 body 类型)。
2. **搬 api hooks**:原 `src/api/<file>.js` 里属于本 entity 的 hooks → `entities/<x>/api/<x>.ts`,**逻辑逐字搬**(包括 `enabled` gate、queryKey `qk.*`、`invalidateQueries` 调用——这些**本阶段一律不改**,留阶段 4)。给每个 hook 标注 req/resp 类型(用 step 1 的 types + `shared/api` 的 `Envelope<T>`/`ListResult<T>`)。`apiFetch`/`qk`/`pickList` 从 `@shared/api` import。
3. **建 `entities/<x>/index.ts`** public API barrel:re-export 该 entity 对外用到的 hooks + 类型。**外部只许 import 这个 barrel**。
4. **旧 `src/api/<file>.js` 转 re-export shim**:`export * from "@entities/<x>";`(或具名 re-export 保形),保证所有现有调用点(组件/pane/hook)**零修改**。grep 调用点确认不破。混装文件(config/forge/library/relations)在多个 task 间逐步掏空,最终整体变 shim 聚合。
5. **边界债当场豁免**:entities boundaries 规则是 **`error`**(样板 Task 2.1 已设,与阶段 1 shared 同款)。若本 entity 的 api hook 为 `enabled` gate 读了 `src/store/settings`(`activeUserId`/`lang`)——这是**阶段 4 身份层**才解的债,**本阶段不解耦**——**当场**在该 import 上方加 inline `// eslint-disable-next-line boundaries/dependencies` + 紧挨 `// TODO(阶段4): identity store 接管 activeUserId 后移除`(与阶段 1 shared 的 3 处豁免同模式)。不要拖到收口。报告里列出本 task 加了哪些豁免。
6. **验证门**(全过):`npx tsc --noEmit`(exit 0)/ `npx vitest run`(**711 passed**,不减)/ `npm run build`(成功)/ `npx eslint src/entities/<x> src/api/<file>.js`(无 boundaries error)/ `npm run fsd`(steiger,本 entity slice 无 FSD 内部违规)。然后精确 commit + push。

## 样板已确立的复用模式(Task 2.1 已产出 + 验证,后续 task 直接遵循,勿重复配置)

- **从 `@shared/api` barrel import**(`apiFetch`/`pickList`/`qk`),**不要**深引 `@shared/api/httpClient`(steiger `no-public-api-sidestep` 会报)。
- `@entities/*` alias 已在 `tsconfig.json` 配好,vite/vitest 经 `vite-tsconfig-paths` 自动继承——**无需每 task 改 alias**。
- `steiger.config.js` 已对 `src/entities/**` 关 `fsd/insignificant-slice`(迁移期 shared-tmp 跨层引用 steiger 追不到)——**无需每 task 改 steiger**。
- `eslint.config.js` 的 `entities` element(`{type:"entities", pattern:"src/entities/*", capture:["slice"]}`)+ 规则(entities 只许 import `shared`,禁上层/同层)已注册为 **`error`**——**无需每 task 改 boundaries 规则**,只需按 SOP step 5 当场 inline disable 自己的 entity→store 越界。
- hook 形态:`useQuery<T>({queryKey: qk.x(), queryFn: () => apiFetch(path), select: pickList<T>})`(列表用 `select: pickList`,**严格对照原 api 文件实际写法**——是 `select` 还是 `.then`、有无 `staleTime`,逐字保留);mutation 标 `useMutation<Resp, Error, Vars>`。
- 实体类型从后端 `service-design-documents/<domain>.md` 的 Go struct json tag 取字段(camelCase);`json:"-"` 字段不出现在响应、省略。

## entity 清单(12 个)+ source 映射

| # | entity | 后端 domain doc | source(`src/api/`/`store/`) | hooks(逐字搬) | 备注 |
|---|---|---|---|---|---|
| 2.1 | **apikey** | `apikey.md` | `config.js`(部分) | useApiKeys / useCreateApiKey / useUpdateApiKey / useDeleteApiKey / useTestApiKey | **样板**,首次注册 entities element |
| 2.2 | **model-config** | `model-config` / `apikey.md` | `config.js`(剩余) | useModelConfigs / useUpsertModelConfig / useProviders / useScenarios | providers/scenarios 是只读白名单,归此;config.js 掏空后转 shim |
| 2.3 | **user** | `user.md` | `users.js` | useUsers / useCreateUser / useUpdateUser / useDeleteUser | activeUserId 切换逻辑(settings.set + invalidate)**原样留**,阶段4 改 |
| 2.4 | **conversation**(api) | `conversation.md` | `conversations.js` | useConversations / useConversation / useConversationMessages / useCreateConversation / useUpdateConversation / useDeleteConversation / useSendMessage / useCancelStream | message hooks 并入 conversation(不单建 message slice) |
| 2.5 | **conversation**(model/chatStore) | `chat.md` / `eventlog.go` | `store/chat.js` | — | **复杂**:SSE 树 store,rAF 合并 + 树重建算法**原样保留**;定 Message/Block 类型 |
| 2.6 | **function** | `function.md` | `forge.js`(部分) | useFunctions / useFunction / useFunctionVersions / useAcceptFunction / useRejectFunction / useRevertFunction / useRunFunction / useDeleteFunction | forge.js 拆分起点 |
| 2.7 | **handler** | `handler.md` | `forge.js`(部分) | useHandlers / useHandler / useHandlerVersions / useHandlerConfig / useAcceptHandler / useRejectHandler / useCallHandler / useDeleteHandler | |
| 2.8 | **workflow** | `workflow.md` | `forge.js`(剩余) | useWorkflows / useWorkflow / useWorkflowVersions / useAcceptWorkflow / useRejectWorkflow / useDeleteWorkflow / useUpdateWorkflow / useRunWorkflow / useEditWorkflow / useCapabilityCheck | `useIterateForge`(跨 kind)**留 forge.js 残壳** + `// TODO(阶段3): features/forge-iterate` |
| 2.9 | **flowrun** | `flowrun.md` | `flowruns.js` | useFlowRuns / useFlowRun / useFlowRunNodes / useCancelFlowRun / useApproveNode / useRejectNode / useTriageFlowRun | |
| 2.10 | **document** + **skill** | `document.md` / `skill.md` | `library.js`(部分) | document: useDocumentTree/useDocuments/useDocument/useCreateDocument/useUpdateDocument/useDeleteDocument/useMoveDocument;skill: useSkills/useSkill | 一个 task 两 entity(skill 仅只读、轻) |
| 2.11 | **mcp** + **memory** | `mcp.md` / `memory.md` | `library.js`(部分) | mcp: useMcpServers/useReconnectMcp/useRemoveMcp;memory: useMemories/useMemory/useCreateMemory/useUpdateMemory/useDeleteMemory/usePinMemory | |
| 2.12 | **relation** | `relation.md` | `relations.js` + `library.js`(useRelations) | useAllRelations / useRelationFilter / useNeighborhood / useRelations | library.js 掏空后转 shim |

**不建 slice 的**(本阶段不动):`notifications.js`(useNotificationsSnapshot)→ 阶段4 归 `app/sse`/`widgets/notifications-drawer`;`attachment` 随 message 携带;`todo`/`sandbox_env` 前端不消费。

## 不变量(行为不变 —— 阶段 2 绝不碰的东西)

- **`enabled` gate 逐字保留**:盘点显示 gate 7/14 不一致(useApiKeys/useModelConfigs/useSkills/useMemories 等无 uid gate),**本阶段一字不改**,统一留阶段 4 身份层。
- **`invalidateQueries` 逻辑不动**:含 user.js 切换用户的全量 invalidate、各 mutation 的失效集——原样搬,失效收口留阶段 4。
- **queryKey(`qk.*`)不变**:继续用 `@shared/api/queryKeys` 的现有 key。
- **UI/组件零改动**:本阶段只动 `api/`+`store/chat.js`,不碰任何 `.jsx` 组件(除非组件直接深引被搬文件的内部符号——正常都走 shim,不应发生)。
- **SSE hooks 不动**:`useEventLog`/`useForge`/`useNotifications`/`SSEProvider` 本阶段不迁(留阶段 4 `app/sse`);它们调用的 chatStore selectors/actions 在 2.5 搬家后**保持同名同签名**,靠 shim 或更新 import 维持。

---

## Task 2.1:entities/apikey(样板 + 注册 entities 边界)

**Files:**
- Create: `frontend/src/entities/apikey/model/types.ts`
- Create: `frontend/src/entities/apikey/api/apikey.ts`
- Create: `frontend/src/entities/apikey/index.ts`
- Modify: `frontend/src/api/config.js`(apikey 部分转 re-export)
- Modify: `frontend/eslint.config.js`(注册 `entities` element + 规则)

- [x] **Step 1: 读 source + 后端 doc**

读 `frontend/src/api/config.js`(看 apikey hooks 的实现:`useApiKeys` 等的 queryKey、apiFetch path、body 形状)、`documents/version-1.2/service-design-documents/apikey.md`(实体字段)、`frontend/src/shared/api/httpClient.ts`(`apiFetch`/`Envelope`/`ListResult`/`pickList` 签名)、`frontend/src/shared/api/queryKeys.ts`(`qk` 有哪些 key)。

- [x] **Step 2: 写 `model/types.ts`**

对齐后端 `apikey.md` 写实体类型。结构示范(**字段以后端 apikey.md 为准**,下面是形态模板):

```ts
// API key 实体(对齐后端 apikey domain)。
export interface ApiKey {
  id: string;
  provider: string;
  label: string;
  keyMasked: string;        // 后端只回脱敏串
  createdAt: string;
  updatedAt: string;
}

export interface CreateApiKeyBody {
  provider: string;
  label: string;
  key: string;
}

export type UpdateApiKeyPatch = Partial<Pick<ApiKey, "label">>;

export interface TestApiKeyResult {
  ok: boolean;
  detail?: string;
}
```

- [x] **Step 3: 写 `api/apikey.ts`**

从 `config.js` 把 apikey 的 5 个 hook 逐字搬来 + 标类型。模板(**保持原 hook 的 queryKey/path/invalidate 逻辑不变**):

```ts
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch, pickList } from "@shared/api/httpClient";
import { qk } from "@shared/api/queryKeys";
import type { ApiKey, CreateApiKeyBody, UpdateApiKeyPatch, TestApiKeyResult } from "../model/types";

export function useApiKeys() {
  return useQuery<ApiKey[]>({
    queryKey: qk.apiKeys(),
    queryFn: () => apiFetch("/api-keys?limit=100").then(pickList<ApiKey>),
  });
}

export function useCreateApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateApiKeyBody) =>
      apiFetch("/api-keys", { method: "POST", body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.apiKeys() }),
  });
}
// ... useUpdateApiKey / useDeleteApiKey / useTestApiKey 同理逐字搬 + 标类型
```

> 注意:严格对照 `config.js` 现有实现搬——参数签名(是 `(id, patch)` 还是 `({id, patch})`)、invalidate 集合、path 一律不变。上面是形态示范,**以 config.js 实际为准**。

- [x] **Step 4: 写 `index.ts` barrel**

```ts
export {
  useApiKeys, useCreateApiKey, useUpdateApiKey, useDeleteApiKey, useTestApiKey,
} from "./api/apikey";
export type {
  ApiKey, CreateApiKeyBody, UpdateApiKeyPatch, TestApiKeyResult,
} from "./model/types";
```

- [x] **Step 5: `config.js` apikey 部分转 re-export**

在 `config.js` 顶部把 apikey 的 5 个 hook 改成 `export { useApiKeys, useCreateApiKey, useUpdateApiKey, useDeleteApiKey, useTestApiKey } from "@entities/apikey";`,**删掉它们的原实现**(model-config/providers/scenarios 的实现暂时原地保留,等 Task 2.2)。grep `from .*api/config` 确认调用点(如 `ApiKeysSection.jsx`)零改动。

- [x] **Step 6: 注册 `entities` boundaries element**

读 `frontend/eslint.config.js`,在 `boundaries/elements` 加 `{ type: "entities", pattern: "src/entities/*", capture: ["slice"] }`(注意:`src/entities/<slice>/**`,capture slice 名以便 @x 规则)。加规则:`entities` 允许 import `shared`;**禁止** import `app`/`pages`/`widgets`/`features`(及 `shared-tmp`/`feature-tmp`/`app-tmp`);同层 `entities` 默认禁(cross-import 后续用 `@x` 再放行)。本阶段 entities 规则设 **`warn`**(因为多个 entity 会临时 import `store/settings` 做 gate,且 store 尚未分层;收口 Task 2.13 再升 error + 豁免)。同时在 `frontend/tsconfig.json` 确认/补 path alias `@entities/*` → `./src/entities/*`(若阶段 0 没配,补上;vite/vitest 用 `vite-tsconfig-paths` 自动继承)。

- [x] **Step 7: 验证 + commit**

跑 SOP step 6 的 5 项验证门。精确 add:`git add frontend/src/entities/apikey frontend/src/api/config.js frontend/eslint.config.js frontend/tsconfig.json`。commit:`feat(frontend): entities/apikey 定型 + 注册 entities 边界(阶段2样板)`。push。

---

## Task 2.2:entities/model-config(+ providers/scenarios)

**Files:** Create `entities/model-config/{model/types.ts, api/model-config.ts, index.ts}`;Modify `src/api/config.js`(掏空 → 纯 shim)。

按 **SOP**。要点:
- types 对齐后端 model-config + provider/scenario 白名单(`apikey.md` 或相关 doc)。`ModelConfig`(scenario→provider/model 映射)、`Provider`、`Scenario`。
- 搬 `useModelConfigs`/`useUpsertModelConfig`/`useProviders`/`useScenarios`(providers/scenarios 是只读白名单,归本 slice 的 api)。
- `config.js` 此时 apikey 已 re-export(2.1)、model-config 也搬走 → **整个 config.js 变 re-export shim**:`export * from "@entities/apikey"; export * from "@entities/model-config";`(具名保形)。grep `ProviderGrid.jsx`/`SearchSection.jsx`/`ModelSelect.jsx`/`ApiKeysSection.jsx` 等调用点确认零改。
- 验证门 + commit `feat(frontend): entities/model-config 定型,config.js 转 shim(阶段2)` + push。

---

## Task 2.3:entities/user

**Files:** Create `entities/user/{model/types.ts, api/user.ts, index.ts}`;Modify `src/api/users.js`(转 shim)。

按 **SOP**。要点:
- types 对齐 `user.md`:`User`(id/username/displayName/avatarColor/language/createdAt/updatedAt)、`CreateUserBody`、`UpdateUserPatch`。
- 搬 `useUsers`/`useCreateUser`/`useUpdateUser`/`useDeleteUser`。**`useDeleteUser` 里切换/invalidate 逻辑原样**;`useUpdateUser` 的 `({id, patch})` 签名保持。
- 切换 activeUserId 的 `settings.set({activeUserId}) + queryClient.invalidateQueries()` 若在某 hook 内,**原样搬**(阶段4 才接 identity)。若读了 `store/settings` → SOP step 5 豁免 + TODO(阶段4)。
- `users.js` 转 shim `export * from "@entities/user";`。grep `Onboarding.jsx`/`SettingsModal.jsx` 调用点。
- commit `feat(frontend): entities/user 定型(阶段2)` + push。

---

## Task 2.4:entities/conversation(api 部分)

**Files:** Create `entities/conversation/{model/types.ts, api/conversation.ts, index.ts}`;Modify `src/api/conversations.js`(转 shim)。

按 **SOP**。要点:
- types 对齐 `conversation.md` + `chat.md`:`Conversation`(id/title/status/createdAt/...)、`Message`(role/status/blocks/stopReason/errorCode/tokens/...)、`Block`(7 个 BlockType 联合 `"text"|"reasoning"|"tool_call"|"tool_result"|"progress"|"message"|"compaction"` + status 4 态联合,对齐后端 `eventlog.go`)。**Message/Block 类型在本 task 定**(供 2.5 chatStore 复用),放 `model/types.ts`。
- 搬 conversation + message 的 8 个 hook(含 `useSendMessage` 的「no invalidate, SSE drives」注释保留、`useCancelStream`)。
- `index.ts` 同时导出 hooks + Conversation/Message/Block 类型(2.5 的 chatStore 会从同 slice import 类型)。
- `conversations.js` 转 shim。grep `ChatPane.jsx`/`Sidebar.jsx`/`Composer.jsx`/`useContextStrip.js`/`useEntityName.js` 调用点。
- commit `feat(frontend): entities/conversation api 定型(阶段2)` + push。

---

## Task 2.5:entities/conversation(model/chatStore —— SSE 树,复杂)

**Files:** Create `entities/conversation/model/chatStore.ts`;Modify `src/store/chat.js`(转 shim);Modify `entities/conversation/index.ts`(补导出 chatStore + selectors)。

> 这是阶段 2 最复杂的搬迁。`store/chat.js` 是 SSE 驱动的消息/块树 store,含 rAF delta 合并 + parentBlockId 树重建算法 + 稳定引用 selectors。**算法一行都不改,只换家 + 加类型。**

- [x] **Step 1: 读 `src/store/chat.js` 全文**,搞清 state shape(`convs[convId]={messages,blocks,topMsgIds,lastSeq}`、`hydratedConvs`)、actions(`ensureConv`/`resetConv`/`resetAll`/`hydrateConv`/`onMessageStart`/`onMessageStop`/`onBlockStart`/`onBlockDelta`/`onBlockStop`)、selectors(`selectTopMessageIds`/`selectBlock`/`selectChildIds`)、rAF 缓冲机制。

- [x] **Step 2: 搬到 `entities/conversation/model/chatStore.ts`**,逐字保留逻辑。用 2.4 定义的 `Message`/`Block` 类型标注 state。zustand store 定义加 `interface ChatState { ... }`(state 字段 + action 签名)。rAF 缓冲、tree 重建、selector 的稳定引用语义**完全不变**。

- [x] **Step 3: `src/store/chat.js` 转 shim**:`export * from "@entities/conversation/model/chatStore";`(`useChatStore` default/named + 3 个 selector 函数都要 re-export 保形)。

- [x] **Step 4: 更新消费方 import**:grep `from .*store/chat`(`BlockRenderer.jsx`、`useEventLog.js`、`ChatPane`、各渲染组件)。它们走 shim 不用改;但若有深引内部的,确认 shim 覆盖。**`sse/useEventLog.js` 调用的 `onMessageStart` 等 action 名/签名必须不变**(本阶段 SSE 不迁,靠 shim 衔接)。

- [x] **Step 5: 验证 + commit**。特别确认 vitest 里 chat store 相关测试(树重建、rAF、selector)全绿——这是行为不变的关键证据。commit `refactor(frontend): chatStore 迁入 entities/conversation/model(阶段2)` + push。

---

## Task 2.6:entities/function(forge.js 拆分起点)

**Files:** Create `entities/function/{model/types.ts, api/function.ts, index.ts}`;Modify `src/api/forge.js`(function 部分转 re-export)。

按 **SOP**。要点:
- types 对齐 `function.md`:`ForgeFunction`(或 `FunctionEntity`,避免和 JS `Function` 撞名——**用 `FunctionEntity`**)+ `FunctionVersion` + pending/run 相关。
- 搬 function 的 8 个 hook。注意 forge.js 是 256 行混装,**只搬 function 部分**,handler/workflow/iterate 原地留待 2.7/2.8。
- `forge.js` 的 function hooks 改 `export { ... } from "@entities/function";` 删原实现。grep `ForgeList.jsx`/`FunctionDetail.jsx`/`ForgePane.jsx`/`useEntityName.js`。
- **注意 `sse/useForge.js`** 里 forge_completed 后按 kind invalidate `qk.functions()/qk.function(id)/qk.functionVersions(id)`——本阶段**不动 useForge.js**(留阶段4),那些 qk 调用继续有效。
- commit `feat(frontend): entities/function 定型(forge.js 拆分起点,阶段2)` + push。

---

## Task 2.7:entities/handler

**Files:** Create `entities/handler/{model/types.ts, api/handler.ts, index.ts}`;Modify `src/api/forge.js`(handler 部分转 re-export)。

按 **SOP**。要点:types 对齐 `handler.md`(`Handler` + `HandlerVersion` + `HandlerConfig` + `HandlerInstance`)。搬 handler 的 8 个 hook(含 `useHandlerConfig`/`useCallHandler`)。`forge.js` handler 部分转 re-export。commit `feat(frontend): entities/handler 定型(阶段2)` + push。

---

## Task 2.8:entities/workflow(+ forge.js 残壳处理)

**Files:** Create `entities/workflow/{model/types.ts, api/workflow.ts, index.ts}`;Modify `src/api/forge.js`(workflow 转 re-export;iterate 残壳)。

按 **SOP**。要点:
- types 对齐 `workflow.md`(`Workflow` + `WorkflowVersion`,workflow 编辑 ops / capability-check 结果类型)。
- 搬 workflow 的 10 个 hook(含 `useEditWorkflow`/`useCapabilityCheck`/`useRunWorkflow`)。
- **`useIterateForge`(跨 function/handler/workflow,POST `/{kind}s/{id}:iterate`)是用例级编排,不属任何单 entity** → 阶段 3 归 `features/forge-iterate`。本阶段**留在 `forge.js` 里**(不搬),`forge.js` 此时 = `export {function/handler/workflow hooks} from "@entities/*"` + **原地保留 `useIterateForge` 实现** + 顶部加注释 `// TODO(阶段3): useIterateForge → features/forge-iterate/model`。
- grep `WorkflowDetail.jsx`/`WorkflowEditor`/iterate 调用点(AskAiTrigger 等)确认不破。
- commit `feat(frontend): entities/workflow 定型,forge.js 仅余 iterate 残壳(阶段2)` + push。

---

## Task 2.9:entities/flowrun

**Files:** Create `entities/flowrun/{model/types.ts, api/flowrun.ts, index.ts}`;Modify `src/api/flowruns.js`(转 shim)。

按 **SOP**。要点:types 对齐 `flowrun.md`(`FlowRun` + `FlowRunNode` + 审批 decision 类型)。搬 7 个 hook(`useFlowRuns` 的 params queryKey、`useApproveNode`/`useRejectNode`/`useTriageFlowRun`)。`flowruns.js` 转 shim。grep `ExecuteOverview.jsx`/`FlowRunDetail.jsx`/`ApprovalBanner.jsx`/`useContextStrip.js`。commit `feat(frontend): entities/flowrun 定型(阶段2)` + push。

---

## Task 2.10:entities/document + entities/skill(library.js 拆分起点)

**Files:** Create `entities/document/{model/types.ts, api/document.ts, index.ts}` + `entities/skill/{model/types.ts, api/skill.ts, index.ts}`;Modify `src/api/library.js`(document/skill 部分转 re-export)。

按 **SOP**(两个 entity,各走一遍)。要点:
- document:types 对齐 `document.md`(`Document` + tree node 结构 `DocTreeNode`)。搬 7 个 hook(`useDocumentTree`/`useMoveDocument` 等)。注意 `useDocumentTree` 的特殊 queryKey `["documents","tree"]`。
- skill:types 对齐 `skill.md`(`Skill`,只读)。搬 `useSkills`/`useSkill`。
- `library.js` 是混装(document/skill/mcp/memory/relations),**只搬 document/skill**,mcp/memory/relations 原地留待 2.11/2.12。document/skill hooks 改 re-export。grep `DocumentsPane.jsx`/`SkillsPane.jsx`/`useEntityName.js`。
- commit `feat(frontend): entities/document + skill 定型(library.js 拆分起点,阶段2)` + push。

---

## Task 2.11:entities/mcp + entities/memory

**Files:** Create `entities/mcp/{...}` + `entities/memory/{...}`;Modify `src/api/library.js`(mcp/memory 转 re-export)。

按 **SOP**。要点:
- mcp:types 对齐 `mcp.md`(`McpServer` + health 状态)。搬 `useMcpServers`/`useReconnectMcp`/`useRemoveMcp`。
- memory:types 对齐 `memory.md`(`Memory`,name 作主键、pinned 字段)。搬 6 个 hook(注意 `encodeURIComponent(name)` 的 path、`usePinMemory`、`useMemories(type?)` 的 `pickList` select)。
- `library.js` mcp/memory 转 re-export(此时仅余 relations 待 2.12)。grep `McpPane.jsx`/`MemoryPane.jsx`。
- commit `feat(frontend): entities/mcp + memory 定型(阶段2)` + push。

---

## Task 2.12:entities/relation(relations.js + library.js 收尾)

**Files:** Create `entities/relation/{model/types.ts, api/relation.ts, index.ts}`;Modify `src/api/relations.js`(转 shim) + `src/api/library.js`(转纯 shim)。

按 **SOP**。要点:
- types 对齐 `relation.md`(`Relation`{fromKind,fromId,toKind,toId,kind} + neighborhood 结果)。
- 搬 `relations.js` 的 `useAllRelations`/`useRelationFilter`/`useNeighborhood` + `library.js` 残留的 `useRelations(entityId)`(都归 relation slice)。注意各自特殊 queryKey(`["relations","all"]`/`["relations","filter",filter]`)。
- `relations.js` 转 shim;`library.js` 此时全部掏空 → **转纯 re-export shim**:`export * from "@entities/document"; ...skill/mcp/memory/relation`(具名保形,保所有旧 `from .*api/library` 调用点)。
- grep `RelGraph.jsx`/Dashboard/`EntityRelMeta` 调用点。
- commit `feat(frontend): entities/relation 定型,library.js 转 shim(阶段2)` + push。

---

## Task 2.13:阶段 2 收口(entities 边界升 error + steiger + 验证 + 文档)

**Files:** Modify `frontend/eslint.config.js`(entities 规则 warn→error + 豁免)、`frontend/steiger.config.js`(移除已迁 api 文件的 ignore)、`docs/superpowers/plans/2026-05-26-frontend-revamp-phase2-entities.md`(勾选)。

- [x] **Step 1: 验证 entities 边界无遗漏越界**

entities 规则在样板 Task 2.1 已设 **`error`**,各 entity task 已**当场** inline disable 自己的 entity→store 越界。本步清点:`grep -rn "eslint-disable-next-line boundaries" src/entities` 列出所有豁免,确认**每一处都有 `// TODO(阶段4)` 标记**且确属 activeUserId/lang gate(非滥用掩盖可避免的越界)。跑 `npx eslint src/entities` 确认除这些已知豁免外 **0 error**。逐条列出豁免(哪个 entity 哪个 hook)。

- [x] **Step 2: steiger ignore 收缩**

`steiger.config.js` 的 ignore 列表移除已迁完的 `src/api/*`(config/conversations/forge/flowruns/library/relations/users —— 它们现在是 shim,但仍是旧扁平结构;判断:若它们纯 shim 且 steiger 不报错可移出 ignore,否则保留到阶段 5 全清时)。`src/store/chat.js`(已 shim)同理。`src/entities` **不加 ignore**(要让 steiger 检查 entities FSD 结构)。跑 `npm run fsd`,确认 `src/entities` 下所有 slice 的 public API/层级/cross-import 干净(`✔ No problems found` 对 entities 范围)。

- [x] **Step 3: 全量验证**

`npx tsc --noEmit`(0)+ `npx vitest run`(**711**)+ `npm run build` + `make lint-frontend`(仓库根;typecheck+lint+fsd 三段全过,entities error 规则下无非豁免违规)+ **`make dev` 冒烟**(仓库根,起壳确认 12 entity 搬迁后 app 正常加载、数据正常拉取、SSE 正常——这是 entities 层全部搬完的端到端确认)。

- [x] **Step 4: 文档勾选 + commit**

本 plan 文件 Task 2.1–2.13 勾 `[x]`,末尾补阶段 2 完成说明(12 entity 定型 + 已知 entity→store 豁免留阶段4 + iterate 残壳留阶段3)。**不动 PRD/CLAUDE.md**(统一留阶段 5)。commit `chore(frontend): entities 边界升 error + steiger 纳入 entities(阶段2收口)` + push。

---

## Self-Review

**Spec 覆盖**(对照 spec §13 阶段 2「每个 api/*.js → entities/<x>/api/*.ts,先定 model/types.ts,再定型 api hooks,建 index.ts;store/chat.js → conversation/model/chatStore;拆 forge.js;违规 warning 逐个升 error 清零」):
- ✅ 12 entity 各有 types + api + index(SOP + Task 2.1 样板)。
- ✅ store/chat.js → conversation/model/chatStore(Task 2.5)。
- ✅ forge.js 拆 function/handler/workflow(Task 2.6–2.8),iterate 残壳留阶段3(有结构性理由:跨实体用例属 features)。
- ✅ config.js/library.js/relations.js 拆分转 shim。
- ✅ 违规升 error 清零(Task 2.13),entity→store 豁免有阶段4 归宿。

**Placeholder 扫描**:Task 2.1 给完整机制代码(types/api/barrel/shim/boundaries 模板);2.2–2.12 引用 SOP + 明确 source 映射/types 来源(后端 domain.md)/特殊点——非占位,是「机械重复样板 + 明确取数路径」(符合 spec ARGUMENTS:阶段 2 给清单+SOP+样板+验证门,不逐文件展开)。types 字段交 implementer 从后端 doc 定(避免 plan 写死 12 实体字段而过时)。

**类型一致性**:`FunctionEntity`(非 `Function`,避免撞 JS 内置)在 2.6 定义、后续引用一致;Message/Block 在 2.4 定义、2.5 chatStore 复用同名;Envelope/ListResult 来自 shared/api。

**顺序依赖**:2.4(conversation types)→ 2.5(chatStore 用 Message/Block);2.6/2.7→2.8(forge.js 逐步掏空,2.8 收 iterate 残壳);2.10/2.11→2.12(library.js 逐步掏空,2.12 转纯 shim);2.13 收口依赖前 12 全完成。subagent-driven 按编号串行执行天然满足。

**行为不变保障**:每 task 末 vitest 711;enabled/invalidate/qk 逐字保留;UI 不碰;SSE hooks 不迁(靠 chatStore 同签名衔接)。

---

## 阶段 2 完成说明(2026-05-26)

**完成状态:DONE**

**12 entity 定型清单:**
apikey / model-config / user / conversation(api + chatStore) / function / handler / workflow / flowrun / document / skill / mcp / memory / relation — 全部已建 `model/types.ts` + `api/*.ts` + `index.ts`。

**entity→store 豁免清单(留阶段4):**
以下 6 个 entity 的 api hook 读 `store/settings.activeUserId` 作 `enabled` gate,均已当场 inline disable + `// TODO(阶段4): identity store 接管 activeUserId 后移除`:
- `entities/conversation/api/conversation.ts`
- `entities/function/api/function.ts`
- `entities/handler/api/handler.ts`
- `entities/workflow/api/workflow.ts`
- `entities/flowrun/api/flowrun.ts`
- `entities/document/api/document.ts`

阶段4 身份层接管 `activeUserId` 后,这 6 处豁免统一移除,`eslint-disable-next-line` 一并删掉。

**iterate 残壳留阶段3:**
`src/api/forge.js` 仍保留 `useIterateForge` 实现(跨 function/handler/workflow 编排,属 `features/forge-iterate`);已注释 `// TODO(阶段3)`。

**api/* + store/chat.js 现为 shim(阶段5 删):**
- `src/api/config.js` — shim → `@entities/apikey` + `@entities/model-config`
- `src/api/users.js` — shim → `@entities/user`
- `src/api/conversations.js` — shim → `@entities/conversation`
- `src/api/forge.js` — shim(function/handler/workflow)+ iterate 残壳
- `src/api/flowruns.js` — shim → `@entities/flowrun`
- `src/api/library.js` — shim → `@entities/document` + `skill` + `mcp` + `memory` + `relation`
- `src/api/relations.js` — shim → `@entities/relation`
- `src/store/chat.js` — shim → `@entities/conversation/model/chatStore`

阶段5 调用点全量迁完后统一删除这些 shim。

**验证基线(2026-05-26):** tsc exit 0 / vitest 711 passed / build 成功 / make lint-frontend 0 error / steiger ✔ No problems found / make dev 冒烟通过(backend :8742 + frontend :5173 正常加载)。
