# 前端架构 Revamp 实现计划 — 阶段 0-1(TS 地基 + shared 层)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development(推荐)或 superpowers:executing-plans 逐 task 执行。步骤用 `- [ ]` 跟踪。

**Goal:** 给前端立起 TypeScript + FSD 边界护栏(阶段 0),并把零业务的 `shared` 层迁好、定型(阶段 1)—— 这是后续 entities/features/widgets/pages/app 全部重构的地基。

**Architecture:** 增量、非破坏。`allowJs` 让 `.jsx`/`.ts(x)` 共存;旧 import 路径用兼容 re-export shim 过渡,不一次性改全;`eslint-plugin-boundaries` 先把现有目录注册成临时 element 标 warning(量化违规),建好 `shared/` 后再对它强制。每 task 后 vitest 必须全绿(行为不变)。

**Tech Stack:** TypeScript 5、Vite 6、vitest 4、eslint 9(flat config)+ eslint-plugin-boundaries、steiger(@feature-sliced)。React 18 + zustand + TanStack Query 不变。

> 关联 spec:`docs/superpowers/specs/2026-05-26-frontend-architecture-revamp-design.md`(读它了解 6 阶段全貌 / FSD 6 层 / 身份层)。本 plan 只覆盖**阶段 0-1**;阶段 2-5 在本 plan 落地后各自 writing-plans(task 形态依赖阶段 1 的实际产出)。

---

## 通用纪律(每个 task 都遵守)

- **行为不变**:这是纯地基/搬迁,不改任何业务逻辑、UI、后端。判据 = 现有 vitest 断言不动且全绿。
- **精确 git add**:多 session 共享 main 工作树(有并行 audit session 改后端)。只 `git add` 本 task 改的前端文件,**严禁 `git add -A`/`.`/`frontend`**。commit 中文 msg、**无 AI attribution**,`git push origin main`;撞 `index.lock` 等锁释放、撞 non-ff 则 `git -c rebase.autoStash=true pull --rebase origin main` 再 push;**绝不开分支**。
- **验证三件套**:每 task 末 `cd frontend && npx vitest run`(全绿)+ 涉及配置的加 `npx tsc --noEmit`。阶段末加 `make dev` 冒烟(窗口起 + 连后端)。
- 命令在 `frontend/` 跑,git 在仓库根跑。

---

## 阶段 0:护栏 + TS 地基(零搬家)

目标:装好 TS / eslint-boundaries / steiger,`.jsx` 原样能跑、tsc 能编、lint 把现有违规以 warning 亮出来。**不动任何源文件内容。**

### Task 0.1:装依赖

**Files:** `frontend/package.json`(devDependencies)

- [x] **Step 1: 装 TS + 类型**
```bash
cd frontend && npm i -D typescript @types/react @types/react-dom @types/node
```
- [x] **Step 2: 装 eslint + boundaries + steiger**
```bash
cd frontend && npm i -D eslint@^9 @eslint/js globals typescript-eslint eslint-plugin-react-hooks eslint-plugin-boundaries vite-tsconfig-paths
npm i -D steiger @feature-sliced/steiger-plugin
```
- [x] **Step 3: 验证装上**
Run: `cd frontend && npx tsc --version && npx eslint --version && npx steiger --version`
Expected: 各输出版本号,无报错。

### Task 0.2:`tsconfig.json`(渐进 strict + allowJs)

**Files:** Create `frontend/tsconfig.json`、`frontend/tsconfig.node.json`

- [x] **Step 1: 写 `frontend/tsconfig.json`**
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "allowJs": true,
    "checkJs": false,
    "strict": false,
    "noUnusedLocals": false,
    "noEmit": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"],
      "@shared/*": ["src/shared/*"],
      "@entities/*": ["src/entities/*"],
      "@features/*": ["src/features/*"],
      "@widgets/*": ["src/widgets/*"],
      "@pages/*": ["src/pages/*"],
      "@app/*": ["src/app/*"]
    }
  },
  "include": ["src"],
  "exclude": ["node_modules", "dist", "coverage"]
}
```
> `strict:false` + `allowJs:true` 是迁移期设定:`.jsx` 不报错、TS 文件渐进定型。阶段 5 收尾再开满 strict。`paths` 对齐 FSD 层(目录现在还没建,先声明)。前端无 `wailsjs/`,无需 exclude。

- [x] **Step 2: 写 `frontend/tsconfig.node.json`(给 vite/eslint config 自身)**
```json
{
  "compilerOptions": { "module": "ESNext", "moduleResolution": "bundler", "allowSyntheticDefaultImports": true },
  "include": ["vite.config.*", "vitest.config.*", "eslint.config.*"]
}
```
- [x] **Step 3: 验证 tsc 能编现有 .jsx**
Run: `cd frontend && npx tsc --noEmit`
Expected: PASS(allowJs + checkJs:false 下 .jsx 不被类型检查,应无错;若有 import 路径错按提示修)。

### Task 0.3:Vite + Vitest path alias

**Files:** Modify `frontend/vite.config.js`、`frontend/vitest.config.js`

- [x] **Step 1: `vite.config.js` 加 tsconfigPaths 插件**
在 plugins 数组加 `tsconfigPaths()`(import 自 `vite-tsconfig-paths`)。这样 `@shared/*` 等 alias 在 build/dev 生效,且与 tsconfig 单一真相。
- [x] **Step 2: `vitest.config.js` 同步 alias**
vitest 复用 vite 的 resolve,加同一个 `tsconfigPaths()` 插件(或 `test.alias` 指向 tsconfig paths)。
- [x] **Step 3: 验证**
Run: `cd frontend && npx vitest run && npm run build`
Expected: 全绿 + build 成功(alias 还没被用,只是就位,不破坏现状)。

### Task 0.4:ESLint flat config + boundaries(现有目录注册成临时 element,先 warn)

**Files:** Create `frontend/eslint.config.js`

- [x] **Step 1: 写 `frontend/eslint.config.js`**
```js
import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import boundaries from "eslint-plugin-boundaries";

export default tseslint.config(
  { ignores: ["dist", "coverage", "node_modules", "**/*.test.{js,jsx,ts,tsx}"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["src/**/*.{js,jsx,ts,tsx}"],
    languageOptions: { globals: { ...globals.browser } },
    plugins: { "react-hooks": reactHooks, boundaries },
    settings: {
      // 迁移期:把现有顶层目录注册成临时 element,等 FSD 目录建好再换成 6 层
      "boundaries/elements": [
        { type: "shared-tmp", pattern: "src/{bridge,api,sse,store,hooks,motion,i18n,components/primitives}/**" },
        { type: "app-tmp",    pattern: "src/{App.jsx,main.jsx}" },
        { type: "feature-tmp", pattern: "src/{panes,components/{overlays,config,shared,layout}}/**" }
      ]
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // 阶段 0 先全 warn,量化违规,不阻断
      "boundaries/element-types": ["warn", {
        default: "allow",
        rules: [{ from: "shared-tmp", disallow: ["feature-tmp", "app-tmp"], message: "shared 不能依赖上层" }]
      }],
      "no-unused-vars": "off",
      "@typescript-eslint/no-unused-vars": "off",
      "@typescript-eslint/no-explicit-any": "off"
    }
  }
);
```
> 阶段 0 只把"shared 倒灌上层"标 warning(量化现有违规,如 `sse/shared.js` 是否引上层)。完整 6 层 disallow 规则在阶段 1+ 逐步收紧。recommended 的其他规则先放松(避免一次性几百红)。

- [x] **Step 2: 跑 lint 看违规数(不阻断)**
Run: `cd frontend && npx eslint src 2>&1 | tail -30`
Expected: 输出一批 warning(记录数量作为基线;典型会看到 AppShell 从 panes 倒灌、RelGraph 引 api 等)。**这一步的产出是"违规清单",不是绿。**

### Task 0.5:package.json scripts + Make 门禁

**Files:** Modify `frontend/package.json`、仓库根 `Makefile`

- [x] **Step 1: package.json 加 scripts**
```json
"typecheck": "tsc --noEmit",
"lint": "eslint src",
"fsd": "steiger src"
```
- [x] **Step 2: Makefile 加 `lint-frontend` 目标**
```makefile
lint-frontend:
	cd frontend && npm run typecheck && npm run lint
```
(steiger 等阶段 1 有 FSD 结构后再并入)
- [x] **Step 3: 验证门跑通**
Run: `make lint-frontend`
Expected: typecheck 过;lint 出 warning 不报错(exit 0)。

### Task 0.6:阶段 0 冒烟 + commit

- [x] **Step 1: 全验证**
Run: `cd frontend && npx vitest run && npm run build && npx tsc --noEmit`
Expected: vitest 全绿(711)、build 成功、tsc 过。
- [x] **Step 2: `make dev` 冒烟**
Run(仓库根):`make dev` 起来后,浏览器/壳能加载(确认 TS 工具链没破坏运行)。停掉。
- [x] **Step 3: commit + push**
```bash
git add frontend/package.json frontend/package-lock.json frontend/tsconfig.json frontend/tsconfig.node.json frontend/vite.config.js frontend/vitest.config.js frontend/eslint.config.js Makefile
git commit -m "chore(frontend): TS + eslint-boundaries 地基(阶段 0,allowJs 渐进,现有违规先 warn)"
git push origin main
```

---

## 阶段 1:`shared` 层(零业务基础设施迁好 + 定型)

目标:建 `src/shared/`,把 bridge / api client / sse / queryKeys / primitives / motion / i18n 迁进去并定型;**旧路径用 re-export shim 过渡**(现有 36+ 文件的 import 不一次性改,后续阶段搬到对应层时再改),保证每 task vitest 绿。

> SOP(每个搬迁 task 通用):① 新建 `shared/<seg>/<name>.ts` 写定型后的实现;② 原文件改成**一行 re-export**(`export * from "@shared/..."`)做兼容;③ 跑相关 .test 绿;④ commit。

### Task 1.1:`shared/api/httpClient.ts`(定型 apiFetch + ApiError + Envelope)

**Files:** Create `frontend/src/shared/api/httpClient.ts`;Modify `frontend/src/api/client.js`(改 re-export)

- [x] **Step 1: 建 `shared/api/httpClient.ts`** —— 把 `api/client.js` 的 `apiFetch` / `ApiError` / `pickList` / `EMPTY_ARRAY` 搬来,加类型:
```ts
export class ApiError extends Error {
  code: string; status: number; details?: unknown;
  constructor(message: string, opts: { code?: string; status?: number; details?: unknown } = {}) {
    super(message); this.name = "ApiError";
    this.code = opts.code ?? "UNKNOWN"; this.status = opts.status ?? 0; this.details = opts.details;
  }
}
export type Envelope<T> = { data: T; nextCursor?: string | null; hasMore?: boolean };
export type ListResult<T> = { items: T[]; nextCursor?: string | null; hasMore?: boolean };
export interface ApiFetchOpts { method?: string; body?: unknown; headers?: Record<string,string>; signal?: AbortSignal; parseJSON?: boolean; }
export async function apiFetch<T = unknown>(path: string, opts?: ApiFetchOpts): Promise<T> { /* 原 client.js 逻辑,加返回类型 */ }
export const EMPTY_ARRAY: readonly never[] = Object.freeze([]);
export function pickList<T>(d: unknown): T[] { /* 原逻辑 */ }
```
> **注意**:此 task 只搬+定型,**不改 401 自愈逻辑**(那是阶段 4 身份层的事)。`activeUserHeader()` 暂保留现状(读 settings),阶段 4 改为读 identity store。
- [x] **Step 2: `api/client.js` 改 re-export shim**
```js
export { apiFetch, ApiError, pickList, EMPTY_ARRAY } from "@shared/api/httpClient";
export { qk } from "@shared/api/queryKeys";  // 见 Task 1.2
```
- [x] **Step 3: 验证**
Run: `cd frontend && npx vitest run && npx tsc --noEmit`
Expected: 全绿(所有 import `api/client.js` 的文件经 shim 透明转发)。
- [x] **Step 4: commit**(精确 add httpClient.ts + client.js)

### Task 1.2:`shared/api/queryKeys.ts`(qk 工厂 + 类型)

**Files:** Create `frontend/src/shared/api/queryKeys.ts`

- [x] Step 1: 把 `client.js` 的 `qk` 工厂搬来,加 `as const` + 返回类型(`readonly [string, ...]`)。
- [x] Step 2: client.js shim 已转发(Task 1.1 Step 2)。
- [x] Step 3: vitest + tsc 绿。Step 4: commit。

### Task 1.3:`shared/api/sse.ts`(createSSE 定型)

**Files:** Create `frontend/src/shared/api/sse.ts`;Modify `frontend/src/sse/shared.js`(re-export)

- [x] Step 1: 搬 `sse/shared.js` 的 `createSSE`,加类型(`{ path, eventHandlers, onStatus }` 接口、`SSEController`)。**不改自愈逻辑**(阶段 4 改为 signalAuthFailure)。
- [x] Step 2: `sse/shared.js` → `export * from "@shared/api/sse"`。
- [x] Step 3: vitest + tsc 绿。Step 4: commit。

### Task 1.4:`shared/api/errorMap.ts`(新建:code → 文案集中表)

**Files:** Create `frontend/src/shared/api/errorMap.ts`

- [x] Step 1: 建集中表(对位后端 `errmap.go`),把已知 error code 映射到默认用户文案 key(i18n)。先放骨架 + 已知 code(UNAUTH_NO_USER / CONVERSATION_NOT_FOUND / 通用 fallback);后续 feature 用例按需补。**此 task 只建表,不接全局 onError**(那在阶段 3/4 收口)。
- [x] Step 2: tsc 绿。Step 3: commit。

### Task 1.5:`shared/bridge/wails.ts`(定型)

**Files:** Create `frontend/src/shared/bridge/wails.ts`;Modify `frontend/src/bridge/wails.js`(re-export)

- [x] Step 1: 搬 `bridge/wails.js`(`initBaseUrl`/`apiUrl`/`GetBackendPort` 调用),加类型。Wails runtime 调用处加最小类型声明(无 wailsjs 生成目录,手写 `declare` 即可)。
- [x] Step 2: `bridge/wails.js` → re-export。
- [x] Step 3: vitest + tsc + **`make dev` 冒烟**(bridge 是 Wails 接触面,必须确认壳能拿到 backend port)。Step 4: commit。

### Task 1.6:`shared/ui/*`(primitives 搬 + 定型 props + barrel)

**Files:** Create `frontend/src/shared/ui/{Button,Badge,Icon,Kbd,Spinner,Select}.tsx` + `index.ts`;Modify `frontend/src/components/primitives/*`(re-export)

- [x] Step 1: 逐个 primitive `.jsx → shared/ui/*.tsx`,定 props 接口。`Select.tsx` 是之前做的统一 Select,props 已清晰,直接标类型。
- [x] Step 2: `shared/ui/index.ts` barrel re-export 全部。
- [x] Step 3: 旧 `components/primitives/*` 各改 re-export shim(指向 `@shared/ui/*`)。
- [x] Step 4: vitest(primitives 的 .test + 用到它们的组件测试)全绿 + tsc。Step 5: commit。

### Task 1.7:`shared/lib/{motion,i18n}`(搬)

**Files:** Create `frontend/src/shared/lib/motion.ts`;移动 `frontend/src/i18n/` → `frontend/src/shared/lib/i18n/`(或 shared/i18n);Modify 旧路径 re-export / 更新 main 的 import

- [x] Step 1: `motion/tokens.js → shared/lib/motion.ts`(定型 token 对象),旧路径 re-export。
- [x] Step 2: i18n 装配移进 `shared/lib/i18n/`(locales 保持),更新 `main.jsx` 的 `import "./i18n"` → `@shared/lib/i18n`;旧 `i18n/index.js` re-export。
- [x] Step 3: vitest(i18n key 完整性测试等)全绿 + tsc。Step 4: commit。

### Task 1.8:对 `shared/` 开启 boundaries 强制 + 引入 steiger

**Files:** Modify `frontend/eslint.config.js`、Create `frontend/steiger.config.js`

- [x] Step 1: eslint boundaries 加正式 `shared` element(`src/shared/**`),规则:`shared` 只能 import `shared`(违规升 `error`)。其余层仍 warn(还没建)。
- [x] Step 2: 建 `steiger.config.js`(@feature-sliced/steiger-plugin),先只校验已存在的 `shared` slice 的 public-api / no-orphan。
- [x] Step 3: Makefile `lint-frontend` 加 `npm run fsd`。
- [x] Step 4: `make lint-frontend` —— `shared` 层零违规(它只依赖自身);若有 shared 内文件引了上层,修。
- [x] Step 5: commit。

### Task 1.9:阶段 1 收口验证 + push

- [x] Step 1: `cd frontend && npx vitest run && npm run build && npx tsc --noEmit && npm run lint && npm run fsd`
Expected: vitest 全绿、build 成功、tsc 过、lint(shared error 级)零违规、steiger(shared)零违规。
- [x] Step 2: `make dev` 冒烟(壳 + 后端连通)。
- [x] Step 3: 确认所有旧路径 shim 透明(grep 没有断裂 import)。commit + push。

---

## 阶段 2-5 概览(各自 writing-plans,落地阶段 1 后细化)

> 不在此展开 —— 它们的 task 形态依赖阶段 1 的实际产出(shared 定型后的 import 形态、strict 实际程度、各文件读细)。每阶段落地后单独 writing-plans,产出可独立验证的增量。全貌见 spec §13。

- **阶段 2 — entities(逐实体)**:12 个 entity 各:定 `model/types.ts`(实体形状,对齐后端 contract)→ `api/*.ts`(从对应 `api/*.js` 拆+定型)→ `model/`(zustand slice;chat store 搬来)→ `ui/`(实体卡)→ `index.ts` public API。拆 `api/forge.js`(256 行)成 function/handler/workflow 三 entity。boundaries 对 entities 开 error 级。
- **阶段 3 — features(抽用例)**:把组件 onClick/effect 里的业务编排抽进 `features/*/model` 的 hook(send-message / forge-iterate / forge-review / onboarding / settings ...);组件变薄。接 TanStack 全局 onError + errorMap。
- **阶段 4 — widgets + pages + app + 身份层**:组合块归 widgets;pane → pages 薄容器;**身份层落地**(identityStore + useIdentityBootstrap,删 5 处自愈、改 httpClient/sse 为 signalAuthFailure、补 enabled gate、boot=phase)—— 把触发本次 revamp 的 401 风暴 bug 连根带走。
- **阶段 5 — 收尾严格化**:tsconfig 开满 strict、清 allowJs(全 .tsx)、删所有 re-export shim 与死代码、boundaries/steiger 全 6 层零违规、文档同步(PRD §1/§2/§5/§17 + CLAUDE.md 前端守则新增 FSD 宪法)。

---

## Self-Review

**Spec 覆盖(阶段 0-1 部分):** §2 D1(TS)→ Task 0.1-0.3 ✅;§4 TS 策略(allowJs 渐进 / paths / 定型边界)→ Task 0.2 + 1.1-1.7 ✅;§9 强制(boundaries/steiger/tsc/门禁)→ Task 0.4-0.5 + 1.8 ✅;§10 Wails(bridge 锁定 / 冒烟)→ Task 1.5 + 各阶段冒烟 ✅;§11 目录(shared 段)→ Task 1.1-1.7 ✅;§12 映射(shared 行)→ 逐 task ✅;§13 阶段 0-1 → 本 plan;阶段 2-5 → 概览 + 各自 plan(有意分批,见开头说明)。

**阶段 0-1 完成(2026-05-26):** shared 层全部定型(api/bridge/lib/ui 四段 + public API barrel);boundaries 对 shared 开启 error 级强制;steiger 配置就绪并在 src/shared 零违规;已知 i18n/sse/httpClient→store 债留阶段4(3 处 inline eslint-disable + TODO 标记)。vitest 711 passed / tsc / build 全绿。

**Placeholder 扫描:** 阶段 0-1 的配置/搬迁给了完整代码与命令;errorMap(1.4)是"建表骨架 + 已知 code",不是 TODO(后续 feature 按需补是设计,非占位);阶段 2-5 概览是有意的分批 roadmap(依赖前阶段产出),非 plan 内的 placeholder。

**类型/命名一致:** `@shared/*` alias 在 tsconfig(0.2)/vite(0.3)/eslint(0.4)/steiger(1.8)全程一致;`apiFetch`/`ApiError`/`Envelope<T>`/`qk` 在 httpClient(1.1)定义,client.js shim(1.1 S2)与 queryKeys(1.2)引用一致;re-export shim 策略全程统一(旧路径 → `@shared/...`)。
