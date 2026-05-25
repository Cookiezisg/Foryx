# 首次启动 / 身份引导重做 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把"首次启动 / 身份"重做成一个干净模块 —— ① 显式的会话就绪状态机根治"带未校验身份开跑"的 401 洪水；② toB 级 6 步引导（含选模型 / 可选搜索 / 语言·明暗自动识别 / 主题色实时）。

**Architecture:** Part A 抽一个纯函数 `computeBootState()`（booting/onboarding/ready）+ App.jsx 一个 latch，保证 AppShell 只在 `activeUserId ∈ users` 时挂载；user-scoped 列表查询补 `enabled` gate 做纵深防御。Part B 重写 `Onboarding.jsx` 为 split「舞台 + 旅程进度 + 内容」6 步向导，文案抽到双语 strings 模块；workspace 步即建 user（后续步才能写 user-scoped 的 key/model），靠 latch 防止建 user 后向导被卸载。后端无改动。

**Tech Stack:** React 18 + Vite + Zustand（settings persist）+ TanStack Query + framer-motion + vitest。后端 Go（仅读，不改 endpoint）。

**Spec:** `docs/superpowers/specs/2026-05-25-onboarding-identity-bootstrap-design.md`
**Mockup（已批准，gitignored，仅本机）:** `.superpowers/brainstorm/83659-1779706284/content/onboarding-full.html`

**分支:** 在 feature 分支 `onboarding-identity-bootstrap` 上做，完工走 finishing-a-development-branch 合回 main。

---

## File Structure

| 文件 | 动作 | 职责 |
|---|---|---|
| `frontend/src/store/boot.js` | Create | `computeBootState()` 纯状态机 + `detectLang()` 设备语言探测 |
| `frontend/src/store/boot.test.js` | Create | 状态机 + 探测的单测 |
| `frontend/src/store/settings.js` | Modify | `lang` 默认值改成 `detectLang()`（首次无持久化时生效）|
| `frontend/src/App.jsx` | Rewrite | latch + `computeBootState` 驱动 onboarding/booting/ready |
| `frontend/src/api/conversations.js` | Modify | `useConversations` 加 `enabled` gate |
| `frontend/src/api/forge.js` | Modify | `useFunctions/useHandlers/useWorkflows` 加 gate |
| `frontend/src/api/flowruns.js` | Modify | `useFlowRuns` 加 gate |
| `frontend/src/api/notifications.js` | Modify | `useNotificationsSnapshot` 加 gate |
| `frontend/src/api/library.js` | Modify | `useDocuments` 加 gate |
| `frontend/src/components/overlays/onboarding-strings.js` | Create | 双语文案 + ACCENTS / LLM_HINTS / SEARCH_HINTS / 默认模型 |
| `frontend/src/components/overlays/Onboarding.jsx` | Rewrite | 6 步向导：shell + 状态 + verify + finish |
| `frontend/src/styles/components.css` | Modify | 替换 `.onb-*` 块为 split 设计（主题 token 化）|
| `backend/internal/app/tool/web/*.go` | Maybe | WebSearch 无 key 时的可操作提示（先查现状）|
| `documents/version-1.2/frontend-prd.md` 等 | Modify | 文档同步（§S14 / F1）|

---

## Task 1: Boot-state 纯模块 + 测试

**Files:**
- Create: `frontend/src/store/boot.js`
- Test: `frontend/src/store/boot.test.js`

- [ ] **Step 1: 写失败测试**

```js
// frontend/src/store/boot.test.js
import { describe, it, expect, vi, afterEach } from "vitest";
import { computeBootState, detectLang } from "./boot.js";

describe("computeBootState", () => {
  const base = { onboardingActive: false, usersLoading: false, usersError: false, users: [], activeUserId: null };

  it("latched onboarding wins over everything", () => {
    expect(computeBootState({ ...base, onboardingActive: true, users: [{ id: "u_1" }], activeUserId: "u_1" })).toBe("onboarding");
  });
  it("users still loading -> booting", () => {
    expect(computeBootState({ ...base, usersLoading: true })).toBe("booting");
  });
  it("fresh install (zero users) -> onboarding", () => {
    expect(computeBootState({ ...base, users: [] })).toBe("onboarding");
  });
  it("users exist but activeUserId null -> booting (waiting on self-heal)", () => {
    expect(computeBootState({ ...base, users: [{ id: "u_1" }], activeUserId: null })).toBe("booting");
  });
  it("STALE activeUserId not in users -> booting, never ready", () => {
    expect(computeBootState({ ...base, users: [{ id: "u_1" }], activeUserId: "u_dead" })).toBe("booting");
  });
  it("valid activeUserId in users -> ready", () => {
    expect(computeBootState({ ...base, users: [{ id: "u_1" }], activeUserId: "u_1" })).toBe("ready");
  });
  it("users error with no users -> booting (do not flash onboarding on fetch error)", () => {
    expect(computeBootState({ ...base, usersError: true, users: [] })).toBe("booting");
  });
});

describe("detectLang", () => {
  afterEach(() => vi.unstubAllGlobals());
  it("zh-* -> zh", () => {
    vi.stubGlobal("navigator", { language: "zh-CN" });
    expect(detectLang()).toBe("zh");
  });
  it("en-US -> en", () => {
    vi.stubGlobal("navigator", { language: "en-US" });
    expect(detectLang()).toBe("en");
  });
  it("other locale -> en", () => {
    vi.stubGlobal("navigator", { language: "fr-FR" });
    expect(detectLang()).toBe("en");
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd frontend && npx vitest run src/store/boot.test.js`
Expected: FAIL — `boot.js` 不存在 / 导出未定义。

- [ ] **Step 3: 实现 boot.js**

```js
// frontend/src/store/boot.js
// Boot-state machine + device-language detection. Pure, no heavy imports,
// so the readiness logic is unit-testable without rendering the app.
//
// 启动状态机 + 设备语言探测；纯函数,可独立单测,不渲染整个 app。

// computeBootState — single source of truth for "what do we render at the
// root". `ready` REQUIRES activeUserId to actually exist in `users`; a stale
// (non-null but unknown) id is NOT ready — that was the 401-flood root cause.
//
// 根渲染状态唯一裁决处。ready 必须 activeUserId 确在 users 里;脏 id(非空但
// 不在列表)不算 ready —— 那正是 401 洪水的根因。
export function computeBootState({ onboardingActive, usersLoading, usersError, users, activeUserId }) {
  if (onboardingActive) return "onboarding";
  if (usersLoading) return "booting";
  if (!usersError && users.length === 0) return "onboarding";
  const valid = !!activeUserId && users.some((u) => u.id === activeUserId);
  return valid ? "ready" : "booting";
}

// detectLang — first-run language from the device. zh* → zh, else en.
// Only consulted when settings has no persisted lang (see settings DEFAULTS).
export function detectLang() {
  if (typeof navigator === "undefined") return "zh";
  const l = (navigator.language || navigator.userLanguage || "").toLowerCase();
  return l.startsWith("zh") ? "zh" : "en";
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd frontend && npx vitest run src/store/boot.test.js`
Expected: PASS（10 个用例全绿）。

- [ ] **Step 5: 提交**

```bash
git add frontend/src/store/boot.js frontend/src/store/boot.test.js
git commit -m "feat(frontend): boot-state machine + device-lang detection (pure, tested)"
```

---

## Task 2: settings 语言默认 + App.jsx 状态机

**Files:**
- Modify: `frontend/src/store/settings.js:11-20`
- Rewrite: `frontend/src/App.jsx`

- [ ] **Step 1: settings.js 用 detectLang 作 lang 默认**

`settings.js` 顶部 import 加一行，DEFAULTS 的 `lang` 改成函数调用：

```js
import { create } from "zustand";
import { persist } from "zustand/middleware";
import { detectLang } from "./boot.js";

const DEFAULTS = {
  theme: "system",       // "system" | "light" | "dark"
  accent: "claude",      // "claude" | "blue" | "ink" | "green" | "purple"
  density: "cozy",       // "compact" | "cozy" | "comfortable"
  lang: detectLang(),    // first-run: device language (zh*/en); persisted after
  reasoningDefault: "collapsed",
  leftPct: 50,
  activeUserId: null,
  onboarded: false,
};
```

其余（`useSettings` / `resolveTheme` / `applyTheme`）不动。persist 会用 localStorage 覆盖默认值，所以 `detectLang()` 只在首次（无持久化）生效。

- [ ] **Step 2: 重写 App.jsx**

完整替换 `frontend/src/App.jsx`：

```jsx
// Root component — boot-state machine (onboarding/booting/ready), theme
// propagation, SSE bootstrap, AppShell.
//
// 根组件 —— 启动状态机；theme dataset 同步;挂 SSE;渲染 AppShell。

import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AppShell } from "./components/layout/AppShell.jsx";
import { Onboarding } from "./components/overlays/Onboarding.jsx";
import { SSEProvider } from "./sse/SSEProvider.jsx";
import { useSettings, applyTheme } from "./store/settings.js";
import { computeBootState } from "./store/boot.js";
import { useChatStore } from "./store/chat.js";
import { useUIStore } from "./store/ui.js";
import { apiFetch, qk, pickList } from "./api/client.js";

// Honor `?onboarding=1` for tests / manual reruns. Production never sets it.
function urlForceOnboarding() {
  if (typeof window === "undefined") return false;
  try { return new URLSearchParams(window.location.search).get("onboarding") === "1"; }
  catch { return false; }
}

export default function App() {
  const settings = useSettings();
  const qc = useQueryClient();
  const prevUid = useRef(settings.activeUserId);
  const [forceOnboarding, setForceOnboarding] = useState(urlForceOnboarding);
  const [onboardingActive, setOnboardingActive] = useState(false);

  useEffect(() => {
    applyTheme(settings);
  }, [settings.theme, settings.accent, settings.density, settings.lang]);

  // Account switch / first-account-set: drop old user's chat tree, invalidate
  // every REST cache, clear cross-user pane state (stale activeConv would 404
  // on send). Fires when activeUserId changes (incl. set during onboarding).
  //
  // 切账号:清 chat store + 失效所有 query + 清 cross-user 残留 pane 状态。
  useEffect(() => {
    if (prevUid.current === settings.activeUserId) return;
    prevUid.current = settings.activeUserId;
    useChatStore.getState().resetAll();
    const ui = useUIStore.getState();
    ui.setActiveConv?.(null);
    if (ui.setActiveFlowRun) ui.setActiveFlowRun(null);
    if (ui.setActiveDocument) ui.setActiveDocument(null);
    qc.invalidateQueries();
  }, [settings.activeUserId, qc]);

  useEffect(() => {
    if (settings.theme !== "system") return;
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const fn = () => applyTheme(settings);
    mql.addEventListener?.("change", fn);
    return () => mql.removeEventListener?.("change", fn);
  }, [settings.theme]);

  // /users drives fresh-install detection AND activeUserId self-heal.
  const usersQ = useQuery({
    queryKey: qk.users(),
    queryFn: () => apiFetch("/users"),
    select: pickList,
  });
  const users = usersQ.data || [];

  // Self-heal: stale activeUserId (points at a deleted user) → clear; no
  // active id but users exist → select the first. Runs every render until it
  // converges; the boot state holds AppShell back until it does.
  //
  // 自愈:脏 id 清掉;无 id 且有 user 选第一个。收敛前 boot state 不放行 AppShell。
  useEffect(() => {
    if (usersQ.isLoading || usersQ.isError) return;
    const activeId = settings.activeUserId;
    if (activeId && !users.find((u) => u.id === activeId)) {
      settings.set({ activeUserId: null });
      return;
    }
    if (!activeId && users.length >= 1) {
      settings.set({ activeUserId: users[0].id });
    }
  }, [usersQ.isLoading, usersQ.isError, users, settings.activeUserId]);

  // Latch: once there's a reason to onboard (fresh install or ?onboarding=1),
  // stay in onboarding until the wizard calls onFinish — even though creating
  // the workspace mid-wizard makes users.length>0 (which would otherwise flip
  // us out and unmount the half-finished wizard).
  //
  // latch:一旦该引导(fresh install 或 ?onboarding=1)就锁住,直到 onFinish。
  // 否则向导中途建了 user → users>0 → 被卸载。
  const wantOnboarding =
    forceOnboarding || (!usersQ.isLoading && !usersQ.isError && users.length === 0);
  useEffect(() => {
    if (!onboardingActive && wantOnboarding) setOnboardingActive(true);
  }, [onboardingActive, wantOnboarding]);

  const boot = computeBootState({
    onboardingActive,
    usersLoading: usersQ.isLoading,
    usersError: usersQ.isError,
    users,
    activeUserId: settings.activeUserId,
  });

  const finishOnboarding = () => {
    setForceOnboarding(false);
    setOnboardingActive(false);
  };

  if (boot === "onboarding") {
    return (
      <SSEProvider>
        <Onboarding onFinish={finishOnboarding} />
      </SSEProvider>
    );
  }
  if (boot === "booting") {
    return <SSEProvider><div className="app-booting" /></SSEProvider>;
  }
  return (
    <SSEProvider>
      <AppShell />
    </SSEProvider>
  );
}
```

- [ ] **Step 3: 构建验证**

Run: `cd frontend && npx vitest run src/store/boot.test.js && npm run build`
Expected: 测试绿；build 成功（无未定义 import）。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/store/settings.js frontend/src/App.jsx
git commit -m "feat(frontend): identity readiness state machine — hold AppShell until activeUserId ∈ users; first-run lang from device"
```

---

## Task 3: user-scoped 列表查询补 enabled gate（纵深防御）

**Files:**
- Modify: `frontend/src/api/conversations.js:8-15`
- Modify: `frontend/src/api/forge.js`（useFunctions / useHandlers / useWorkflows）
- Modify: `frontend/src/api/flowruns.js:9-16`
- Modify: `frontend/src/api/notifications.js:9-15`
- Modify: `frontend/src/api/library.js:108`

模式：每个列表 hook 读 `activeUserId`，加 `enabled: !!uid`。AppShell 只在 ready 挂载，这是纵深防御 —— 即便某 hook 早挂，无身份也不发请求。

- [ ] **Step 1: conversations.js**

文件顶部 import 加 `import { useSettings } from "../store/settings.js";`，改 `useConversations`：

```js
export function useConversations() {
  const uid = useSettings((s) => s.activeUserId);
  return useQuery({
    queryKey: qk.conversations(),
    queryFn: () => apiFetch("/conversations?limit=100"),
    select: pickList,
    enabled: !!uid,
  });
}
```

- [ ] **Step 2: forge.js（三个列表 hook）**

顶部加 `import { useSettings } from "../store/settings.js";`。三处套同样的 gate：

```js
export function useFunctions() {
  const uid = useSettings((s) => s.activeUserId);
  return useQuery({ queryKey: qk.functions(), queryFn: () => apiFetch("/functions?limit=200"), select: pickList, enabled: !!uid });
}
export function useHandlers() {
  const uid = useSettings((s) => s.activeUserId);
  return useQuery({ queryKey: qk.handlers(), queryFn: () => apiFetch("/handlers?limit=200"), select: pickList, enabled: !!uid });
}
export function useWorkflows() {
  const uid = useSettings((s) => s.activeUserId);
  return useQuery({ queryKey: qk.workflows(), queryFn: () => apiFetch("/workflows?limit=200"), select: pickList, enabled: !!uid });
}
```

- [ ] **Step 3: flowruns.js**

顶部加 import；改 `useFlowRuns`：

```js
export function useFlowRuns(params = {}) {
  const uid = useSettings((s) => s.activeUserId);
  const qs = new URLSearchParams({ limit: "100", ...params }).toString();
  return useQuery({
    queryKey: [...qk.flowruns(), params],
    queryFn: () => apiFetch(`/flowruns?${qs}`),
    select: pickList,
    enabled: !!uid,
  });
}
```

- [ ] **Step 4: notifications.js**

顶部加 import；改 `useNotificationsSnapshot`：

```js
export function useNotificationsSnapshot(limit = 50) {
  const uid = useSettings((s) => s.activeUserId);
  return useQuery({
    queryKey: qk.notificationsSnap(),
    queryFn: () => apiFetch(`/notifications?limit=${limit}`, { headers: { Accept: "application/json" } }),
    select: pickList,
    enabled: !!uid,
  });
}
```

- [ ] **Step 5: library.js（useDocuments）**

读 `useDocuments`（line 108）当前实现，顶部加 import（若未引入），加 `const uid = useSettings((s) => s.activeUserId);` + `enabled: !!uid`（若该 hook 已有 `enabled`，与之 `&&` 合并：`enabled: !!uid && <原条件>`）。

- [ ] **Step 6: 构建 + 全量单测**

Run: `cd frontend && npm run build && npx vitest run`
Expected: build 成功；vitest 全绿（无 hook-rules 报错；`useSettings` 在 hook 顶层调用合法）。

- [ ] **Step 7: 提交**

```bash
git add frontend/src/api/conversations.js frontend/src/api/forge.js frontend/src/api/flowruns.js frontend/src/api/notifications.js frontend/src/api/library.js
git commit -m "feat(frontend): gate user-scoped list queries on activeUserId — defense-in-depth vs 401 flood"
```

---

## Task 4: 引导双语文案 + 常量模块

**Files:**
- Create: `frontend/src/components/overlays/onboarding-strings.js`

- [ ] **Step 1: 写 onboarding-strings.js**

```js
// Onboarding copy (zh / en) + UI constants. Scoped i18n: full-app i18n is a
// future module. Picked by settings.lang (device-detected on first run).
//
// 引导文案(中/英)+ 常量。scoped i18n;全 app i18n 留以后。按 settings.lang 取。

export const ACCENTS = [
  ["claude", "#d97757"],
  ["blue", "#2383e2"],
  ["ink", "#37352f"],
  ["green", "#0f7b6c"],
  ["purple", "#6940a5"],
];

// LLM provider chips (abbr + brand color). Keyed by backend provider `name`.
export const LLM_HINTS = {
  deepseek: { abbr: "DS", color: "#4D6BFE" },
  openai: { abbr: "OA", color: "#10A37F" },
  anthropic: { abbr: "AN", color: "#D97757" },
  google: { abbr: "GO", color: "#4285F4" },
  qwen: { abbr: "QW", color: "#615CED" },
  zhipu: { abbr: "ZP", color: "#3870E0" },
  moonshot: { abbr: "MS", color: "#37352F" },
  ollama: { abbr: "OL", color: "#6b6459" },
};

export const SEARCH_HINTS = {
  bocha: { abbr: "BC", color: "#1f9d55" },
  brave: { abbr: "BR", color: "#fb542b" },
  serper: { abbr: "SE", color: "#5436da" },
  tavily: { abbr: "TV", color: "#0f7b6c" },
};

// Fallback model id used ONLY when :test returns no modelsFound (e.g.
// Anthropic ping). Must be a real, runnable id.
export const PROVIDER_DEFAULT_MODEL = {
  anthropic: "claude-sonnet-4-6",
};

export const STRINGS = {
  zh: {
    brandSub: "本地 AI 工作台 · v1.2",
    footer1: "数据本地存储于",
    footer2: "不上传 · 无需登录",
    back: "上一步", next: "继续", start: "开始", skip: "跳过", enter: "进入 Forgify",
    auto: "已根据系统",
    journey: {
      welcome: ["欢迎", "了解 Forgify"],
      workspace: ["工作空间", "命名"],
      appearance: ["外观", "主题与语言"],
      model: ["模型", "API Key 与模型"],
      search: ["搜索", "可选"],
      done: ["完成", "开始使用"],
    },
    welcome: {
      kicker: "第 1 步 · 欢迎",
      title: "欢迎使用 Forgify",
      sub: "本地优先的 AI agent 工作台。用自然语言驱动它完成任务,并将过程沉淀为可复用的工具。",
      features: [
        ["对话驱动", "描述目标,agent 自主选择工具、编写代码、运行工作流。"],
        ["能力沉淀", "协助构建 Function、Handler、Workflow,内置版本管理与回滚。"],
        ["本地运行", "数据存储于本地,不上传云端,无需登录。"],
      ],
    },
    workspace: {
      kicker: "第 2 步 · 工作空间",
      title: "创建工作空间",
      sub: "为工作空间命名。后续可在设置中新增或切换,各空间的数据相互隔离。",
      label: "工作空间名称",
      placeholder: "例如 个人 / 工作 / 写作",
      hint: "显示在侧边栏底部。切换工作空间时仅切换该空间的数据。",
    },
    appearance: {
      kicker: "第 3 步 · 外观",
      title: "外观与语言",
      sub: "语言与主题已根据系统设置自动选择,可随时调整。以下均可在「设置」中修改。",
      accent: "主题色", language: "语言", theme: "主题",
      themeOpts: { light: "浅色", dark: "深色", system: "跟随系统" },
    },
    model: {
      kicker: "第 4 步 · 模型",
      title: "配置模型",
      sub: "选择厂商、填入 API Key,验证后选择要使用的模型。也可稍后在「设置」中配置。",
      providerLabel: "模型服务商",
      scrollNote: "可滚动查看全部厂商",
      keyLabel: (p) => `${p} API Key`,
      keyPlaceholder: "sk-…",
      verify: "验证并获取模型", verifying: "验证中…", verified: "已验证",
      modelLabel: "模型", pickModel: "选择模型",
      ollamaHint: "Ollama 为本地推理,无需 API Key。请确保 ollama serve 已启动。",
      availHint: (list) => `可用模型:${list.join(" · ")}(下拉切换)`,
      skip: "稍后配置 →",
    },
    search: {
      kicker: "第 5 步 · 联网搜索",
      optional: "· 可选",
      title: "联网搜索",
      sub: "配置一个搜索服务,agent 即可联网检索资料。不配也能正常使用 —— 需要时在「设置」里再加。",
      providerLabel: "搜索服务商",
      keyLabel: (p) => `${p} API Key`,
      keyPlaceholder: "填入 key,或跳过此步",
    },
    done: {
      title: "设置完成",
      sub: "一切就绪。开始你的第一个对话,或让 agent 为你构建第一个工具。",
      recap: { workspace: "工作空间", accent: "主题色", model: "模型", search: "搜索" },
      none: "稍后",
    },
    toast: {
      userFail: "创建工作空间失败",
      keyVerified: "API Key 已验证",
      keyFail: "Key 已保存,但验证未通过",
      opFail: "操作失败",
      welcome: "欢迎使用 Forgify",
    },
  },
  en: {
    brandSub: "Local AI workspace · v1.2",
    footer1: "Data stored locally at",
    footer2: "No upload · No login",
    back: "Back", next: "Continue", start: "Start", skip: "Skip", enter: "Enter Forgify",
    auto: "From system",
    journey: {
      welcome: ["Welcome", "About Forgify"],
      workspace: ["Workspace", "Name it"],
      appearance: ["Appearance", "Theme & language"],
      model: ["Model", "API key & model"],
      search: ["Search", "Optional"],
      done: ["Done", "Get started"],
    },
    welcome: {
      kicker: "Step 1 · Welcome",
      title: "Welcome to Forgify",
      sub: "A local-first AI agent workspace. Drive it with natural language, and distill the work into reusable tools.",
      features: [
        ["Conversation-driven", "Describe a goal; the agent picks tools, writes code, runs workflows."],
        ["Distilled capability", "Build Functions, Handlers, Workflows — with built-in versioning and rollback."],
        ["Runs locally", "Data lives on your machine. No cloud upload, no login."],
      ],
    },
    workspace: {
      kicker: "Step 2 · Workspace",
      title: "Create a workspace",
      sub: "Name your workspace. Add or switch more later in Settings; each one's data is isolated.",
      label: "Workspace name",
      placeholder: "e.g. Personal / Work / Writing",
      hint: "Shown at the bottom of the sidebar. Switching swaps only that workspace's data.",
    },
    appearance: {
      kicker: "Step 3 · Appearance",
      title: "Appearance & language",
      sub: "Language and theme follow your system by default. Adjust anytime — all of this lives in Settings.",
      accent: "Accent", language: "Language", theme: "Theme",
      themeOpts: { light: "Light", dark: "Dark", system: "System" },
    },
    model: {
      kicker: "Step 4 · Model",
      title: "Configure a model",
      sub: "Pick a provider, enter an API key, verify, then choose a model. You can also do this later in Settings.",
      providerLabel: "Model provider",
      scrollNote: "Scroll for all providers",
      keyLabel: (p) => `${p} API key`,
      keyPlaceholder: "sk-…",
      verify: "Verify & list models", verifying: "Verifying…", verified: "Verified",
      modelLabel: "Model", pickModel: "Select a model",
      ollamaHint: "Ollama runs locally — no API key needed. Make sure `ollama serve` is running.",
      availHint: (list) => `Available: ${list.join(" · ")} (switch in dropdown)`,
      skip: "Set up later →",
    },
    search: {
      kicker: "Step 5 · Web search",
      optional: "· Optional",
      title: "Web search",
      sub: "Configure a search provider and the agent can browse the web. Optional — add it later in Settings.",
      providerLabel: "Search provider",
      keyLabel: (p) => `${p} API key`,
      keyPlaceholder: "Enter a key, or skip this step",
    },
    done: {
      title: "All set",
      sub: "Everything's ready. Start your first conversation, or have the agent build your first tool.",
      recap: { workspace: "Workspace", accent: "Accent", model: "Model", search: "Search" },
      none: "Later",
    },
    toast: {
      userFail: "Failed to create workspace",
      keyVerified: "API key verified",
      keyFail: "Key saved, but verification failed",
      opFail: "Operation failed",
      welcome: "Welcome to Forgify",
    },
  },
};
```

- [ ] **Step 2: 提交**

```bash
git add frontend/src/components/overlays/onboarding-strings.js
git commit -m "feat(frontend): bilingual onboarding strings + provider/accent constants"
```

---

## Task 5: 引导 CSS（split 设计，主题 token 化）

**Files:**
- Modify: `frontend/src/styles/components.css`

把现有 `.onboarding*` / `.onb-*` 全部规则替换为下面的 split 设计块（颜色用主题 token，暗色自动生效）。先 `grep -n "\.onb\|\.onboarding\|app-booting" src/styles/components.css` 找到全部范围，删干净再贴新块。

- [ ] **Step 1: 替换 onboarding CSS 块**

```css
/* ── Onboarding — split stage + journey + content ──────────────── */
.app-booting { position: fixed; inset: 0; background: var(--bg-window); }

.onb-overlay {
  position: fixed; inset: 0; z-index: 60;
  display: flex; align-items: center; justify-content: center;
  background: var(--bg-overlay); padding: 24px;
}
.onb-card {
  display: flex; width: min(920px, 94vw); height: min(560px, 92vh);
  background: var(--bg-paper); border-radius: var(--radius-xl); overflow: hidden;
  box-shadow: var(--shadow-lg); border: 1px solid var(--border-soft);
}

/* stage */
.onb-stage {
  position: relative; width: 36%; padding: 28px 24px;
  display: flex; flex-direction: column;
  background: linear-gradient(168deg, var(--accent-soft), var(--bg-sidebar));
  border-right: 1px solid var(--border); overflow: hidden;
}
.onb-stage::after {
  content: "F"; position: absolute; right: -28px; bottom: -44px;
  font-size: 240px; font-weight: 800; color: var(--accent);
  opacity: 0.06; line-height: 1; pointer-events: none;
}
.onb-brand { display: flex; align-items: center; gap: 10px; position: relative; z-index: 1; }
.onb-mark {
  width: 34px; height: 34px; border-radius: var(--radius-md); background: var(--accent);
  display: flex; align-items: center; justify-content: center;
  box-shadow: 0 4px 12px -2px oklch(from var(--accent) l c h / 0.5);
}
.onb-mark svg { width: 20px; height: 20px; stroke: var(--accent-fg); fill: none; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.onb-brand-name { font-size: var(--fs-15); font-weight: 650; color: var(--fg-strong); }
.onb-brand-sub { font-size: var(--fs-11); color: var(--fg-faint); margin-top: 1px; }

.onb-journey { margin-top: 26px; display: flex; flex-direction: column; position: relative; z-index: 1; }
.onb-jstep { display: flex; gap: 11px; padding: 7px 9px; border-radius: var(--radius-md); position: relative; }
.onb-jstep.is-active { background: var(--bg-paper); box-shadow: var(--shadow-sm); }
.onb-jdot {
  flex: none; width: 22px; height: 22px; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: var(--fs-11); font-weight: 600; background: var(--bg-paper);
  border: 1.5px solid var(--border-strong); color: var(--fg-faint); z-index: 1;
}
.onb-jstep.is-done .onb-jdot { background: var(--accent); border-color: var(--accent); color: var(--accent-fg); }
.onb-jstep.is-active .onb-jdot { border-color: var(--accent); color: var(--accent); box-shadow: 0 0 0 4px var(--accent-soft); }
.onb-jdot svg { width: 11px; height: 11px; stroke: var(--accent-fg); fill: none; stroke-width: 2.5; stroke-linecap: round; stroke-linejoin: round; }
.onb-jline { position: absolute; left: 19.5px; top: 29px; width: 1.5px; height: 14px; background: var(--border-strong); }
.onb-jstep.is-done .onb-jline { background: var(--accent); }
.onb-jtext { padding-top: 2px; min-width: 0; }
.onb-jtitle { font-size: var(--fs-13); font-weight: 550; color: var(--fg-muted); }
.onb-jstep.is-active .onb-jtitle { color: var(--fg-strong); font-weight: 600; }
.onb-jdesc { font-size: var(--fs-11); color: var(--fg-faint); margin-top: 1px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.onb-foot { margin-top: auto; font-size: var(--fs-11); color: var(--fg-faint); display: flex; align-items: center; gap: 6px; position: relative; z-index: 1; line-height: 1.5; }
.onb-foot svg { width: 12px; height: 12px; stroke: var(--fg-faint); fill: none; stroke-width: 1.6; flex: none; }
.onb-foot code { font-family: var(--font-mono); font-size: var(--fs-11); color: var(--fg-muted); background: var(--bg-paper); padding: 1px 5px; border-radius: var(--radius-sm); }

/* pane */
.onb-pane { flex: 1; display: flex; flex-direction: column; padding: 36px 40px 22px; min-width: 0; }
.onb-pane.is-centered { justify-content: center; text-align: center; align-items: center; }
.onb-kicker { font-size: var(--fs-11); font-weight: 600; letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent); margin-bottom: 10px; }
.onb-title { font-size: var(--fs-28); font-weight: 640; letter-spacing: -0.02em; line-height: 1.18; color: var(--fg-strong); }
.onb-sub { font-size: var(--fs-13); color: var(--fg-muted); margin-top: 8px; line-height: 1.55; max-width: 460px; }

.onb-features { margin-top: 24px; display: flex; flex-direction: column; gap: 15px; }
.onb-feature { display: flex; gap: 13px; }
.onb-fic { flex: none; width: 36px; height: 36px; border-radius: var(--radius-md); background: var(--accent-soft); display: flex; align-items: center; justify-content: center; }
.onb-fic svg { width: 18px; height: 18px; stroke: var(--accent); fill: none; stroke-width: 1.7; }
.onb-ftitle { font-size: var(--fs-14); font-weight: 600; color: var(--fg-strong); }
.onb-fdesc { font-size: var(--fs-13); color: var(--fg-muted); margin-top: 2px; line-height: 1.5; }

/* workspace */
.onb-ws { margin-top: 28px; display: flex; gap: 16px; }
.onb-avatar { flex: none; width: 54px; height: 54px; border-radius: var(--radius-lg); background: var(--accent); color: var(--accent-fg); display: flex; align-items: center; justify-content: center; font-size: var(--fs-22); font-weight: 600; }
.onb-field { flex: 1; }
.onb-label { font-size: var(--fs-13); font-weight: 550; color: var(--fg-muted); margin-bottom: 7px; display: flex; align-items: center; gap: 8px; }
.onb-input { width: 100%; border: 1px solid var(--accent); border-radius: var(--radius-md); padding: 0 14px; height: 46px; font-size: var(--fs-16); font-family: var(--font-sans); color: var(--fg-strong); outline: none; background: var(--bg-input); box-shadow: 0 0 0 3px var(--accent-soft); }
.onb-hint { font-size: var(--fs-12); color: var(--fg-faint); margin-top: 8px; line-height: 1.5; }

/* appearance */
.onb-fg { margin-top: 20px; }
.onb-auto { font-size: var(--fs-11); font-weight: 500; color: var(--status-success); background: oklch(from var(--status-success) l c h / 0.12); border-radius: var(--radius-pill); padding: 2px 8px; }
.onb-swatches { display: flex; gap: 11px; }
.onb-swatch { width: 28px; height: 28px; border-radius: 50%; cursor: pointer; border: none; position: relative; transition: transform var(--t-fast); }
.onb-swatch:hover { transform: scale(1.08); }
.onb-swatch.is-active { box-shadow: 0 0 0 2px var(--bg-paper), 0 0 0 4px var(--accent); }
.onb-seg { display: inline-flex; background: var(--bg-elev); border-radius: var(--radius-md); padding: 3px; gap: 2px; }
.onb-seg-opt { font-size: var(--fs-13); font-weight: 500; color: var(--fg-muted); padding: 6px 16px; border-radius: var(--radius-sm); cursor: pointer; border: none; background: none; font-family: var(--font-sans); }
.onb-seg-opt.is-active { background: var(--bg-paper); color: var(--fg-strong); font-weight: 550; box-shadow: var(--shadow-sm); }

/* provider grid (model + search) */
.onb-gridwrap { margin-top: 18px; position: relative; }
.onb-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; max-height: 150px; overflow-y: auto; padding-right: 8px; }
.onb-grid.is-tall { max-height: none; }
.onb-grid::-webkit-scrollbar { width: 8px; }
.onb-grid::-webkit-scrollbar-thumb { background: var(--border-strong); border-radius: 4px; border: 2px solid var(--bg-paper); }
.onb-grid-fade { position: absolute; left: 0; right: 8px; bottom: 0; height: 20px; background: linear-gradient(transparent, var(--bg-paper)); pointer-events: none; }
.onb-prov { display: flex; align-items: center; gap: 10px; padding: 9px 11px; border: 1px solid var(--border); border-radius: var(--radius-md); cursor: pointer; background: var(--bg-paper); text-align: left; }
.onb-prov:hover { border-color: var(--border-strong); }
.onb-prov.is-active { border-color: var(--accent); background: var(--accent-soft); box-shadow: 0 0 0 1px var(--accent) inset; }
.onb-pchip { flex: none; width: 27px; height: 27px; border-radius: var(--radius-sm); display: flex; align-items: center; justify-content: center; color: #fff; font-size: var(--fs-11); font-weight: 700; }
.onb-pname { font-size: var(--fs-13); font-weight: 550; color: var(--fg-strong); }
.onb-pdesc { font-size: var(--fs-11); color: var(--fg-faint); margin-top: 1px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 110px; }
.onb-scrollnote { font-size: var(--fs-11); color: var(--fg-faint); margin-top: 7px; }

/* key + model select */
.onb-twofield { margin-top: 14px; display: flex; gap: 12px; align-items: flex-end; }
.onb-keyfield { margin-top: 14px; }
.onb-twofield .onb-keyfield { margin-top: 0; }
.onb-klabel { font-size: var(--fs-12); font-weight: 550; color: var(--fg-muted); margin-bottom: 6px; }
.onb-kinput { display: flex; align-items: center; gap: 8px; border: 1px solid var(--accent); border-radius: var(--radius-md); padding: 0 12px; height: 40px; box-shadow: 0 0 0 3px var(--accent-soft); background: var(--bg-input); }
.onb-kinput.is-plain { border-color: var(--border-strong); box-shadow: none; }
.onb-kinput svg { width: 14px; height: 14px; stroke: var(--fg-faint); fill: none; stroke-width: 1.7; flex: none; }
.onb-kinput input { flex: 1; border: none; outline: none; background: transparent; font-size: var(--fs-13); font-family: var(--font-sans); color: var(--fg-strong); min-width: 0; }
.onb-verify-btn { background: none; border: none; color: var(--accent); font-family: var(--font-sans); font-size: var(--fs-12); font-weight: 600; cursor: pointer; white-space: nowrap; padding: 0; }
.onb-verify-btn:disabled { color: var(--fg-faint); cursor: default; }
.onb-verified { font-size: var(--fs-11); font-weight: 600; color: var(--status-success); white-space: nowrap; display: flex; align-items: center; gap: 3px; }
.onb-verified svg { width: 12px; height: 12px; stroke: var(--status-success); fill: none; stroke-width: 2.5; }
.onb-mselect { width: 100%; height: 40px; border: 1px solid var(--border-strong); border-radius: var(--radius-md); padding: 0 12px; background: var(--bg-input); color: var(--fg-strong); font-size: var(--fs-13); font-family: var(--font-mono); cursor: pointer; outline: none; }
.onb-khint { font-size: var(--fs-11); color: var(--fg-faint); margin-top: 7px; }
.onb-skip { background: none; border: none; font-family: var(--font-sans); font-size: var(--fs-12); color: var(--fg-muted); cursor: pointer; margin-top: 12px; }
.onb-skip:hover { color: var(--accent); }
.onb-banner { margin-top: 16px; background: var(--bg-elev); border: 1px solid var(--border); border-radius: var(--radius-md); padding: 11px 13px; font-size: var(--fs-12); color: var(--fg-muted); line-height: 1.5; display: flex; gap: 9px; }
.onb-banner svg { width: 16px; height: 16px; stroke: var(--accent); fill: none; stroke-width: 1.7; flex: none; margin-top: 1px; }
.onb-banner code { font-family: var(--font-mono); }

/* done */
.onb-donemark { width: 54px; height: 54px; border-radius: 50%; background: var(--accent-soft); display: flex; align-items: center; justify-content: center; margin-bottom: 8px; }
.onb-donemark svg { width: 26px; height: 26px; stroke: var(--accent); fill: none; stroke-width: 2.4; stroke-linecap: round; stroke-linejoin: round; }
.onb-recap { margin-top: 24px; display: grid; grid-template-columns: 1fr 1fr; gap: 10px; width: 100%; max-width: 380px; }
.onb-recap-card { border: 1px solid var(--border); border-radius: var(--radius-md); padding: 13px; background: var(--bg-elev); display: flex; flex-direction: column; align-items: center; gap: 7px; }
.onb-recap-label { font-size: var(--fs-11); color: var(--fg-faint); font-weight: 550; }
.onb-recap-value { font-size: var(--fs-13); font-weight: 600; color: var(--fg-strong); display: flex; align-items: center; justify-content: center; min-height: 20px; }
.onb-recap-value.is-muted { color: var(--fg-faint); font-weight: 500; }
.onb-recap-dot { width: 18px; height: 18px; border-radius: 50%; box-shadow: 0 0 0 3px var(--bg-paper), 0 0 0 4px var(--border-strong); }

/* actions */
.onb-actions { margin-top: auto; padding-top: 18px; display: flex; align-items: center; gap: 13px; }
.onb-progress { flex: 1; display: flex; flex-direction: column; gap: 6px; }
.onb-progress-label { font-size: var(--fs-11); color: var(--fg-faint); }
.onb-progress-track { height: 3px; border-radius: 3px; background: var(--border); overflow: hidden; }
.onb-progress-fill { height: 100%; background: var(--accent); border-radius: 3px; transition: width var(--t-med); }
```

- [ ] **Step 2: 提交**

```bash
git add frontend/src/styles/components.css
git commit -m "style(frontend): onboarding split-stage CSS (theme-tokenized, dark-mode safe)"
```

---

## Task 6: Onboarding.jsx 重写（6 步 + verify + finish）

**Files:**
- Rewrite: `frontend/src/components/overlays/Onboarding.jsx`

依赖：Task 4 strings、Task 5 CSS、`useDeleteApiKey`（已存在，config.js:56）。`Icon` 成员可用：Check / ChevronDown / ArrowRight / MessageSquare / Wrench / Server / Globe / KeyRound / Search / Sparkles。

- [ ] **Step 1: 完整替换 Onboarding.jsx**

```jsx
// Onboarding — toB 6-step first-run wizard (split stage + journey).
//   1 Welcome    — what Forgify is.
//   2 Workspace  — create the first local user (POST /users), sets activeUserId.
//   3 Appearance — accent (live) + language + theme; all write straight to settings.
//   4 Model      — provider + key → verify (:test) → pick model → POST /model-configs.
//   5 Search     — optional search provider + key (POST /api-keys, category=search).
//   6 Done       — recap + enter.
//
// Workspace creates the user EARLY so steps 4/5 can write user-scoped keys;
// App's onboarding latch keeps this mounted afterwards. finish() flips
// settings.onboarded.
//
// 6 步 toB 首启向导。workspace 步即建 user(后续步才能写 user 作用域的 key);
// App 的 latch 保证建 user 后不被卸载。finish 置 onboarded。

import { useEffect, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { useQueryClient } from "@tanstack/react-query";
import { Icon } from "../primitives/Icon.jsx";
import { Button } from "../primitives/Button.jsx";
import { useSettings } from "../../store/settings.js";
import { useUIStore } from "../../store/ui.js";
import { useCreateUser } from "../../api/users.js";
import { useProviders, useCreateApiKey, useTestApiKey, useUpsertModelConfig, useDeleteApiKey } from "../../api/config.js";
import { STRINGS, ACCENTS, LLM_HINTS, SEARCH_HINTS, PROVIDER_DEFAULT_MODEL } from "./onboarding-strings.js";

const STEP_KEYS = ["welcome", "workspace", "appearance", "model", "search", "done"];
const ANVIL = (
  <svg viewBox="0 0 24 24"><path d="M12 2v3" /><path d="M5 5l2 2" /><path d="M19 5l-2 2" /><path d="M4 12h4l2-3l4 6l2-3h4" /><path d="M5 17h14" /><path d="M7 21l1-4" /><path d="M17 21l-1-4" /></svg>
);

export function Onboarding({ onFinish }) {
  const settings = useSettings();
  const qc = useQueryClient();
  const pushToast = useUIStore((s) => s.pushToast);
  const t = STRINGS[settings.lang] || STRINGS.zh;

  const createUser = useCreateUser();
  const { data: providers = [] } = useProviders();
  const createKey = useCreateApiKey();
  const testKey = useTestApiKey();
  const deleteKey = useDeleteApiKey();
  const upsertModel = useUpsertModelConfig();

  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);

  // workspace
  const [name, setName] = useState("");
  const [createdUserId, setCreatedUserId] = useState(null);
  // model
  const [provider, setProvider] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [createdKeyId, setCreatedKeyId] = useState(null);
  const [createdKeyText, setCreatedKeyText] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [verified, setVerified] = useState(false);
  const [models, setModels] = useState([]);
  const [modelId, setModelId] = useState("");
  // search
  const [searchProvider, setSearchProvider] = useState("");
  const [searchKey, setSearchKey] = useState("");

  const llm = providers.filter((p) => p.category === "llm" && p.name !== "mock" && p.name !== "custom");
  const search = providers.filter((p) => p.category === "search");
  const stepKey = STEP_KEYS[step];

  const run = async (fn) => {
    setBusy(true);
    try { await fn(); }
    catch (err) { pushToast({ kind: "error", title: t.toast.opFail, desc: err.message }); }
    finally { setBusy(false); }
  };

  const ensureUser = async () => {
    if (createdUserId) return;
    const user = await createUser.mutateAsync({
      username: name.trim().toLowerCase().replace(/\s+/g, "-"),
      displayName: name.trim(),
      avatarColor: ACCENTS.find(([k]) => k === settings.accent)?.[1] || "#d97757",
    });
    setCreatedUserId(user.id);
    settings.set({ activeUserId: user.id });
  };

  // Reset key/model state when switching provider; best-effort delete the
  // orphaned key created for the previous provider.
  const pickProvider = (name) => {
    if (createdKeyId) deleteKey.mutate(createdKeyId);
    setProvider(name);
    setApiKey(""); setCreatedKeyId(null); setCreatedKeyText("");
    setVerified(false); setModels([]); setModelId("");
  };

  const onKeyChange = (v) => {
    setApiKey(v);
    if (verified) { setVerified(false); setModels([]); setModelId(""); }
  };

  const verify = () => run(async () => {
    setVerifying(true);
    try {
      let keyId = createdKeyId;
      if (keyId && createdKeyText !== apiKey) {
        deleteKey.mutate(keyId);
        keyId = null; setCreatedKeyId(null);
      }
      if (!keyId) {
        const k = await createKey.mutateAsync({
          provider, key: apiKey || "ollama-no-key", displayName: `${provider} (onboarding)`,
        });
        keyId = k.id; setCreatedKeyId(k.id); setCreatedKeyText(apiKey);
      }
      const res = await testKey.mutateAsync(keyId);
      const found = res?.modelsFound || [];
      const opts = found.length ? found : (PROVIDER_DEFAULT_MODEL[provider] ? [PROVIDER_DEFAULT_MODEL[provider]] : []);
      setModels(opts);
      setModelId(opts[0] || "");
      setVerified(true);
      pushToast({ kind: "success", title: t.toast.keyVerified });
    } catch (err) {
      setVerified(false);
      pushToast({ kind: "warn", title: t.toast.keyFail, desc: err.message });
    } finally {
      setVerifying(false);
    }
  });

  const finish = () => {
    settings.set({ onboarded: true });
    qc.invalidateQueries();
    pushToast({ kind: "success", title: t.toast.welcome, desc: name.trim() });
    onFinish?.();
  };

  const advance = () => setStep((s) => Math.min(STEP_KEYS.length - 1, s + 1));
  const back = () => setStep((s) => Math.max(0, s - 1));

  const handleNext = () => {
    switch (stepKey) {
      case "workspace": return run(async () => { await ensureUser(); advance(); });
      case "model": return run(async () => {
        if (verified && modelId) await upsertModel.mutateAsync({ scenario: "chat", provider, modelId });
        advance();
      });
      case "search": return run(async () => {
        if (searchProvider && searchKey.trim()) {
          await createKey.mutateAsync({ provider: searchProvider, key: searchKey.trim(), displayName: `${searchProvider} (onboarding)` });
        }
        advance();
      });
      case "done": return finish();
      default: return advance();
    }
  };

  const canNext = () => {
    if (stepKey === "workspace") return name.trim().length > 0;
    return true;
  };

  // journey desc shows captured values once a step is behind us.
  const jdesc = (key, fallback) => {
    if (key === "workspace" && createdUserId) return name.trim();
    if (key === "appearance" && step > 2) return `${accentLabel(settings.accent)} · ${settings.lang === "zh" ? "中文" : "EN"}`;
    if (key === "model" && step > 3) return verified ? providerDisplay(provider) : t.done.none;
    if (key === "search" && step > 4) return searchProvider ? providerDisplay(searchProvider) : t.done.none;
    return fallback;
  };
  const providerDisplay = (n) => providers.find((p) => p.name === n)?.displayName || n;
  const accentLabel = (a) => a;

  return (
    <AnimatePresence>
      <motion.div className="onb-overlay" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
        <motion.div
          className="onb-card"
          initial={{ opacity: 0, scale: 0.97, y: 8 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          transition={{ duration: 0.32, ease: [0.2, 0.8, 0.2, 1] }}
        >
          <aside className="onb-stage">
            <div className="onb-brand">
              <div className="onb-mark">{ANVIL}</div>
              <div>
                <div className="onb-brand-name">Forgify</div>
                <div className="onb-brand-sub">{t.brandSub}</div>
              </div>
            </div>
            <div className="onb-journey">
              {STEP_KEYS.map((key, i) => {
                const [jt, jd] = t.journey[key];
                const cls = "onb-jstep" + (i === step ? " is-active" : "") + (i < step ? " is-done" : "");
                return (
                  <div key={key} className={cls}>
                    {i < STEP_KEYS.length - 1 && <div className="onb-jline" />}
                    <div className="onb-jdot">{i < step ? <Icon.Check /> : i + 1}</div>
                    <div className="onb-jtext">
                      <div className="onb-jtitle">{jt}</div>
                      <div className="onb-jdesc">{jdesc(key, jd)}</div>
                    </div>
                  </div>
                );
              })}
            </div>
            <div className="onb-foot">
              <Icon.KeyRound />
              <span>{t.footer1} <code>~/.forgify/</code><br />{t.footer2}</span>
            </div>
          </aside>

          <section className={"onb-pane" + (stepKey === "done" ? " is-centered" : "")}>
            {stepKey === "welcome" && <Welcome t={t} />}
            {stepKey === "workspace" && <Workspace t={t} name={name} setName={setName} accent={settings.accent} />}
            {stepKey === "appearance" && <Appearance t={t} settings={settings} />}
            {stepKey === "model" && (
              <Model
                t={t} providers={llm} provider={provider} pickProvider={pickProvider}
                apiKey={apiKey} onKeyChange={onKeyChange} verify={verify}
                verifying={verifying} verified={verified} models={models}
                modelId={modelId} setModelId={setModelId}
              />
            )}
            {stepKey === "search" && (
              <Search
                t={t} providers={search} provider={searchProvider} setProvider={setSearchProvider}
                apiKey={searchKey} setApiKey={setSearchKey}
              />
            )}
            {stepKey === "done" && (
              <Done t={t} name={name} accent={settings.accent} provider={verified ? providerDisplay(provider) : null} search={searchProvider ? providerDisplay(searchProvider) : null} />
            )}

            <div className="onb-actions">
              {stepKey !== "done" && (
                <div className="onb-progress">
                  <div className="onb-progress-label">{settings.lang === "zh" ? "步骤" : "Step"} {step + 1} / {STEP_KEYS.length}</div>
                  <div className="onb-progress-track">
                    <div className="onb-progress-fill" style={{ width: `${((step + 1) / STEP_KEYS.length) * 100}%` }} />
                  </div>
                </div>
              )}
              {stepKey === "search" ? (
                <Button variant="ghost" size="sm" onClick={advance} disabled={busy}>{t.skip}</Button>
              ) : step > 0 && stepKey !== "done" ? (
                <Button variant="ghost" size="sm" onClick={back} disabled={busy}>← {t.back}</Button>
              ) : null}
              <Button variant="accent" size="sm" onClick={handleNext} disabled={!canNext() || busy} loading={busy}>
                {stepKey === "done" ? t.enter : stepKey === "welcome" ? t.start : t.next}
                <Icon.ArrowRight />
              </Button>
            </div>
          </section>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
}

function Welcome({ t }) {
  const icons = [Icon.MessageSquare, Icon.Wrench, Icon.Server];
  return (
    <>
      <div className="onb-kicker">{t.welcome.kicker}</div>
      <div className="onb-title">{t.welcome.title}</div>
      <div className="onb-sub">{t.welcome.sub}</div>
      <div className="onb-features">
        {t.welcome.features.map(([title, desc], i) => {
          const I = icons[i];
          return (
            <div className="onb-feature" key={i}>
              <div className="onb-fic"><I /></div>
              <div><div className="onb-ftitle">{title}</div><div className="onb-fdesc">{desc}</div></div>
            </div>
          );
        })}
      </div>
    </>
  );
}

function Workspace({ t, name, setName, accent }) {
  const color = ACCENTS.find(([k]) => k === accent)?.[1] || "#d97757";
  return (
    <>
      <div className="onb-kicker">{t.workspace.kicker}</div>
      <div className="onb-title">{t.workspace.title}</div>
      <div className="onb-sub">{t.workspace.sub}</div>
      <div className="onb-ws">
        <div className="onb-avatar" style={{ background: color }}>{name.trim().slice(0, 1).toUpperCase() || "W"}</div>
        <div className="onb-field">
          <div className="onb-label">{t.workspace.label}</div>
          <input className="onb-input" placeholder={t.workspace.placeholder} value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          <div className="onb-hint">{t.workspace.hint}</div>
        </div>
      </div>
    </>
  );
}

function Appearance({ t, settings }) {
  const Seg = ({ value, opts }) => (
    <div className="onb-seg">
      {opts.map(([k, label]) => (
        <button key={k} className={"onb-seg-opt" + (value === k ? " is-active" : "")} onClick={() => settings.set(k === value ? {} : segPatch(k, opts))}>{label}</button>
      ))}
    </div>
  );
  // segPatch maps option key to the settings field — derived from which group.
  return (
    <>
      <div className="onb-kicker">{t.appearance.kicker}</div>
      <div className="onb-title">{t.appearance.title}</div>
      <div className="onb-sub">{t.appearance.sub}</div>

      <div className="onb-fg">
        <div className="onb-label">{t.appearance.accent}</div>
        <div className="onb-swatches">
          {ACCENTS.map(([k, c]) => (
            <button key={k} className={"onb-swatch" + (settings.accent === k ? " is-active" : "")} style={{ background: c }} onClick={() => settings.set({ accent: k })} />
          ))}
        </div>
      </div>

      <div className="onb-fg">
        <div className="onb-label">{t.appearance.language} <span className="onb-auto">{t.auto}</span></div>
        <div className="onb-seg">
          {[["zh", "中文"], ["en", "English"]].map(([k, label]) => (
            <button key={k} className={"onb-seg-opt" + (settings.lang === k ? " is-active" : "")} onClick={() => settings.set({ lang: k })}>{label}</button>
          ))}
        </div>
      </div>

      <div className="onb-fg">
        <div className="onb-label">{t.appearance.theme} <span className="onb-auto">{t.auto}</span></div>
        <div className="onb-seg">
          {[["light", t.appearance.themeOpts.light], ["dark", t.appearance.themeOpts.dark], ["system", t.appearance.themeOpts.system]].map(([k, label]) => (
            <button key={k} className={"onb-seg-opt" + (settings.theme === k ? " is-active" : "")} onClick={() => settings.set({ theme: k })}>{label}</button>
          ))}
        </div>
      </div>
    </>
  );
}

function ProviderGrid({ providers, hints, selected, onPick, tall }) {
  return (
    <div className="onb-gridwrap">
      <div className={"onb-grid" + (tall ? " is-tall" : "")}>
        {providers.map((p) => {
          const h = hints[p.name] || { abbr: p.name.slice(0, 2).toUpperCase(), color: "#6b6459" };
          return (
            <button key={p.name} className={"onb-prov" + (selected === p.name ? " is-active" : "")} onClick={() => onPick(p.name)}>
              <span className="onb-pchip" style={{ background: h.color }}>{h.abbr}</span>
              <span style={{ minWidth: 0 }}>
                <span className="onb-pname">{p.displayName || p.name}</span>
                <span className="onb-pdesc" style={{ display: "block" }}>{p.defaultBaseUrl?.replace(/^https?:\/\//, "") || ""}</span>
              </span>
            </button>
          );
        })}
      </div>
      {!tall && <div className="onb-grid-fade" />}
    </div>
  );
}

function Model({ t, providers, provider, pickProvider, apiKey, onKeyChange, verify, verifying, verified, models, modelId, setModelId }) {
  const isOllama = provider === "ollama";
  const display = providers.find((p) => p.name === provider)?.displayName || provider;
  return (
    <>
      <div className="onb-kicker">{t.model.kicker}</div>
      <div className="onb-title">{t.model.title}</div>
      <div className="onb-sub">{t.model.sub}</div>
      <ProviderGrid providers={providers} hints={LLM_HINTS} selected={provider} onPick={pickProvider} />
      <div className="onb-scrollnote">{t.model.scrollNote}</div>

      {provider && (
        <>
          <div className="onb-twofield">
            <div className="onb-keyfield" style={{ flex: 1.3 }}>
              <div className="onb-klabel">{isOllama ? display : t.model.keyLabel(display)}</div>
              <div className="onb-kinput">
                <Icon.KeyRound />
                {!isOllama && (
                  <input type="password" placeholder={t.model.keyPlaceholder} value={apiKey} onChange={(e) => onKeyChange(e.target.value)} autoFocus />
                )}
                {isOllama && <input value="localhost:11434" readOnly style={{ color: "var(--fg-faint)" }} />}
                {verified ? (
                  <span className="onb-verified"><Icon.Check /> {t.model.verified}</span>
                ) : (
                  <button className="onb-verify-btn" onClick={verify} disabled={verifying || (!isOllama && !apiKey.trim())}>
                    {verifying ? t.model.verifying : t.model.verify}
                  </button>
                )}
              </div>
            </div>
            {verified && models.length > 0 && (
              <div className="onb-keyfield" style={{ flex: 1 }}>
                <div className="onb-klabel">{t.model.modelLabel}</div>
                <select className="onb-mselect" value={modelId} onChange={(e) => setModelId(e.target.value)}>
                  {models.map((m) => <option key={m} value={m}>{m}</option>)}
                </select>
              </div>
            )}
          </div>
          {isOllama && <div className="onb-banner"><Icon.Server /><span>{t.model.ollamaHint}</span></div>}
          {verified && models.length > 0 && <div className="onb-khint">{t.model.availHint(models)}</div>}
        </>
      )}
    </>
  );
}

function Search({ t, providers, provider, setProvider, apiKey, setApiKey }) {
  const display = providers.find((p) => p.name === provider)?.displayName || provider;
  return (
    <>
      <div className="onb-kicker">{t.search.kicker} <span style={{ color: "var(--fg-faint)", fontWeight: 500, textTransform: "none", letterSpacing: 0 }}>{t.search.optional}</span></div>
      <div className="onb-title">{t.search.title}</div>
      <div className="onb-sub">{t.search.sub}</div>
      <div className="onb-fg">
        <div className="onb-label">{t.search.providerLabel}</div>
        <ProviderGrid providers={providers} hints={SEARCH_HINTS} selected={provider} onPick={setProvider} tall />
      </div>
      {provider && (
        <div className="onb-keyfield">
          <div className="onb-klabel">{t.search.keyLabel(display)}</div>
          <div className="onb-kinput is-plain">
            <Icon.KeyRound />
            <input type="password" placeholder={t.search.keyPlaceholder} value={apiKey} onChange={(e) => setApiKey(e.target.value)} autoFocus />
          </div>
        </div>
      )}
    </>
  );
}

function Done({ t, name, accent, provider, search }) {
  const color = ACCENTS.find(([k]) => k === accent)?.[1] || "#d97757";
  return (
    <>
      <div className="onb-donemark"><Icon.Check /></div>
      <div className="onb-title">{t.done.title}</div>
      <div className="onb-sub">{t.done.sub}</div>
      <div className="onb-recap">
        <Recap label={t.done.recap.workspace} value={name} />
        <Recap label={t.done.recap.accent}><span className="onb-recap-dot" style={{ background: color }} /></Recap>
        <Recap label={t.done.recap.model} value={provider} muted={!provider} fallback={t.done.none} />
        <Recap label={t.done.recap.search} value={search} muted={!search} fallback={t.done.none} />
      </div>
    </>
  );
}

function Recap({ label, value, children, muted, fallback }) {
  return (
    <div className="onb-recap-card">
      <div className="onb-recap-label">{label}</div>
      <div className={"onb-recap-value" + (muted ? " is-muted" : "")}>{children || value || fallback}</div>
    </div>
  );
}
```

> **实现注记（Appearance 的 Seg 简化）:** 上面 Appearance 内联了三组 seg（accent/language/theme），把草拟的 `Seg`/`segPatch` 抽象删掉 —— 三组各自直连 `settings.set({...})` 更直白（YAGNI）。实现时直接用内联三组，不要保留 `Seg`/`segPatch`。

- [ ] **Step 2: build + lint**

Run: `cd frontend && npm run build`
Expected: 成功，无未使用 import（删掉 `Seg`/`segPatch` 草稿后）。

- [ ] **Step 3: 浏览器验证（golden path + 边界）**

Run: `cd backend && make test-console`（起 dev server）；另开 `cd frontend && npm run dev`，访问 `http://localhost:5173/?onboarding=1`。
逐项确认：
- 6 步 split 布局、左轨 done=✓/active=光圈、暖色舞台 + 淡 F、底部「数据本地」。
- 语言：改 `navigator.language` / 直接点 step3 语言切换，整个向导文案实时切。
- 主题色点击实时变（含舞台 gradient / 进度条 / 按钮）。暗色：系统切暗色 → 向导跟随。
- 模型步：选 provider → 填 key → 点「验证并获取模型」→ ✓ 已验证 + 模型下拉出现 + 可切换；ollama 无 key 也能验证 + banner。
- 搜索步：选博查/Brave/...，「跳过」直接到完成；填 key + 继续也到完成。
- 完成步居中、recap 4 卡、主题色卡是色块（无文字）。
- 「进入 Forgify」→ 落到 AppShell，**控制台无 401 刷屏**，侧边栏底部显示工作空间名。

- [ ] **Step 4: 提交**

```bash
git add frontend/src/components/overlays/Onboarding.jsx
git commit -m "feat(frontend): rewrite onboarding — toB 6-step split wizard (model select + optional search + live theme/lang)"
```

---

## Task 7: WebSearch 无 key 承接提示（先查现状再定）

**Files:**
- Investigate: `backend/internal/app/tool/web/`
- Maybe modify: WebSearch tool 的无 key 错误路径

- [ ] **Step 1: 查现状**

Run:
```bash
cd backend && grep -rn "search\|Search" internal/app/tool/web/*.go | grep -i "key\|no key\|missing\|UNCONFIGURED\|not configured" | head
```
读 WebSearch tool 在"用户没配 search key"时返回的 tool_result 文案。

- [ ] **Step 2: 判定**

- 若已是可操作提示（含"配置搜索 / 设置"指引）→ **不改**，本任务跳到 Step 4 记 doc。
- 若是裸 error（如 "no search api key"）→ 改成可操作中文提示：`未配置搜索服务。在「设置 → API Keys」添加搜索服务商(博查 / Brave / Serper / Tavily)的 key 即可联网检索。`（保持 tool_result 错误语义，仅改 message 文案）。

- [ ] **Step 3:（若改了）build + 单测**

Run: `cd backend && go build ./... && make test-unit && staticcheck ./...`
Expected: 全绿。

- [ ] **Step 4: 提交（若有改动）**

```bash
git add backend/internal/app/tool/web/
git commit -m "feat(backend): actionable WebSearch hint when no search key configured"
```

---

## Task 8: 文档同步（§S14 / F1）

**Files:**
- Modify: `documents/version-1.2/frontend-prd.md`
- Modify: `documents/version-1.2/DESIGN.md`（如 split 成为新约定）
- Modify: `documents/version-1.2/progress-record.md`

- [ ] **Step 1: frontend-prd.md**

- onboarding 章节重写：6 步（welcome/workspace/appearance/model/search/done）+ 就绪状态机（booting/onboarding/ready，readiness=activeUserId∈users）+ 语言/明暗自动识别 + 模型选择（修 modelsFound[0] 瞎猜）+ 可选搜索 + 承接。
- §17 endpoint 表：确认 onboarding 用到的 `/users` `/providers` `/api-keys(+:test)` `/model-configs` 与实际一致；不一致就修。
- §16 bug 记录：补「401 洪水根因 = 脏 activeUserId 越过 resolvingUser 闸门 → 状态机修复」一行。
- §15 Phase：onboarding 重做项打勾。

- [ ] **Step 2: DESIGN.md**

- 若 split「舞台 + 旅程进度」要成为 overlay 类约定 → §11/§12 补一条；问候/文案 toB 化原则若需 → 对应章补。

- [ ] **Step 3: progress-record.md**

加 dev log（1-2 句，§S19）：401 根因 + 就绪状态机 + 引导 6 步重做 + 语言/明暗检测 + 测试数（boot.test.js N 例）。

- [ ] **Step 4: 提交**

```bash
git add documents/
git commit -m "docs: onboarding redesign + readiness state machine (PRD/DESIGN/progress sync)"
```

---

## Self-Review

**Spec coverage（逐节核对 spec）:**
- §3 Part A 状态机 → Task 1（computeBootState）+ Task 2（App latch/三态）+ Task 3（query gate）。✅
- §3.2 脏 id → 不挂 AppShell：computeBootState 的 `valid = activeUserId ∈ users` + booting 兜底。✅
- §4 6 步 UI → Task 6（6 步）+ Task 5（CSS）。✅
- §5 语言/明暗自动 + 主题色实时 → Task 1 detectLang + Task 2 settings 默认 + Appearance 直写 settings（applyTheme 经 App effect 实时）。✅
- §6 双语引导 → Task 4 STRINGS zh/en。✅
- §7 模型选择（修 [0] 瞎猜）→ Task 6 verify + 下拉 + `upsertModel({modelId})`。✅
- §8 搜索可选 + 承接 → Task 6 Search 步（跳过）+ Task 7 WebSearch 提示。✅
- §9 后端不改 → 仅 Task 7 可能改 tool 文案（非 endpoint）。✅
- §10 错误处理 → `run()` toast 不前进；verify 失败保留 key + 提示；搜索可跳过；booting 占位。✅
- §11 测试 → boot.test.js（状态机 + 探测）；向导走浏览器验证（项目惯例 UI 在浏览器验，非重组件测试）。✅
- §13 文档 → Task 8。✅

**Placeholder scan:** Task 7 是"先查再定"的调查门控（给了 grep + 两种确定走向），非模糊占位。其余均含完整代码。Appearance 的 `Seg`/`segPatch` 草稿已用注记明确删除并给出内联实现。

**Type consistency:** `computeBootState` 入参在 Task 1 定义、Task 2 调用一致；strings 字段（`t.toast.*` / `t.journey[key]` / `t.model.availHint`）在 Task 4 定义、Task 6 使用一致；`models`/`modelId` 命名（避开 `useUpsertModelConfig` → `upsertModel`）一致；`enabled: !!uid` 七处同构。
