# 设置重做 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把设置收敛成一个居中 modal —— API Key 为中心的凭证管理 + 引导页式新增 + 外观/系统 + 手风琴互斥;删掉过度 tab 与 popover↔ConfigPane 重复;并加最小后端改动让"搜索默认"真生效。

**Architecture:** 新 `SettingsModal`(齿轮触发,居中 overlay,单开手风琴)取代旧 `SettingsPopover` + `ConfigPane` 两套。引导页的 provider 网格 / Key 验证 / 模型下拉抽成 `components/config/` 共享组件,引导页 + 设置共用(DRY)。后端给 `api_keys` 加 `is_default`(per-category 单选)+ WebSearch 解析优先用默认搜索商。

**Tech Stack:** 前端 React 18 + Vite + Zustand + TanStack Query + framer-motion + vitest;后端 Go(apikey domain/store/app/handler + WebSearch tool)。

**Spec:** `docs/superpowers/specs/2026-05-25-settings-redesign-design.md`
**已批准交互原型(视觉/结构契约,已入库):** `docs/superpowers/prototypes/2026-05-25-settings.html` —— 各 section 的 HTML 结构、class、手风琴/验证/单选交互以此为准,UI 任务按它 port 成 React。
**风格基准:** 侧边栏(`.sb-item` pill 999 / `--bg-hover`/`--bg-active` / `--t-fast`)+ 引导页(`.onb-grid`/`.onb-prov`/`.onb-kinput`/`.onb-mselect`/`.onb-seg`)。

**分支:** feature 分支 `settings-redesign`,完工走 finishing-a-development-branch 合 main。

---

## File Structure

| 文件 | 动作 | 责任 |
|---|---|---|
| `backend/internal/domain/apikey/apikey.go` | Modify | `APIKey.IsDefault bool` 字段 + Repository 加 `DefaultForCategory`/`ClearDefault` 契约 |
| `backend/internal/infra/store/apikey/apikey.go` | Modify | `is_default` 列(schema_extras 幂等迁移)+ 实现新 repo 方法 |
| `backend/internal/app/apikey/apikey.go` | Modify | `UpdateInput.IsDefault`;设默认时同 category 单选;providers.go category 查询 |
| `backend/internal/app/apikey/providers.go` | Read | category 映射(已存在,供单选用)|
| `backend/internal/transport/httpapi/handlers/apikey.go` | Modify | `updateRequest.IsDefault` → UpdateInput |
| `backend/internal/app/tool/web/search.go` | Modify | 解析前先试默认搜索商 |
| `backend/internal/app/tool/web/web.go` 等(KeyProvider port)| Modify | port 加 `DefaultSearchProvider(ctx) string` |
| `frontend/src/components/config/ProviderGrid.jsx` | Create | 共享:双列 provider 色块网格(从 onboarding 抽)|
| `frontend/src/components/config/KeyVerifyField.jsx` | Create | 共享:Key 输入 + 验证态 + 错误 |
| `frontend/src/components/config/ModelSelect.jsx` | Create | 共享:模型下拉(styled select)|
| `frontend/src/components/overlays/Onboarding.jsx` | Modify | 消费上面 3 个共享组件(行为不变)|
| `frontend/src/components/overlays/SettingsModal.jsx` | Create | modal 壳 + 单开手风琴 + 账号区 |
| `frontend/src/components/config/ApiKeysSection.jsx` | Create | key 列表 + 每 key 模型/用途 + 引导页式新增 |
| `frontend/src/components/config/SearchSection.jsx` | Create | 搜索 key 列表 + 搜索默认 + 新增 |
| `frontend/src/components/config/AppearanceSection.jsx` | Create | 主题/色调/密度/语言/推理 |
| `frontend/src/components/config/SystemSection.jsx` | Create | 只读:数据目录/沙箱/版本 |
| `frontend/src/store/ui.js` | Modify | `settingsPopOpen`→`settingsOpen`;清 popover/config-pane 状态 |
| `frontend/src/components/layout/Sidebar.jsx:181` | Modify | 齿轮 → 开 `settingsOpen` |
| `frontend/src/App.jsx` 或 AppShell | Modify | 挂 `<SettingsModal/>` |
| `frontend/src/components/overlays/SettingsPopover.jsx` (+ test) | Delete | 被 modal 取代 |
| `frontend/src/panes/config/ConfigPane.jsx` | Delete | tab 结构被 modal 取代(逻辑迁入 sections)|
| `frontend/src/styles/components.css` | Modify | settings modal/section CSS;删旧 `.settings-pop*` / `.page-tab*`(若仅 config 用)|
| `frontend/src/api/config.js` | Read | 复用现有 hooks(useApiKeys/useUpdateApiKey/...)|
| `documents/version-1.2/frontend-prd.md` 等 | Modify | 文档同步 |

---

## Task 1: 后端 — api-key `IsDefault` + 搜索默认解析

**Files:**
- Modify: `backend/internal/domain/apikey/apikey.go`
- Modify: `backend/internal/infra/store/apikey/apikey.go`
- Modify: `backend/internal/app/apikey/apikey.go`
- Modify: `backend/internal/transport/httpapi/handlers/apikey.go`
- Modify: `backend/internal/app/tool/web/search.go` + KeyProvider port
- Test: `backend/internal/app/apikey/apikey_test.go`

- [ ] **Step 1: domain — 加字段 + 契约**

`apikey.go` 的 `APIKey` struct 加(放在 `ModelsFound` 后):
```go
	IsDefault    bool           `gorm:"not null;default:false" json:"isDefault"`
```
`Repository` 接口加两个方法:
```go
	// ClearDefaultForCategory unsets is_default on all of the user's keys whose
	// provider belongs to the given category (used to keep "default" single-choice).
	ClearDefaultForCategory(ctx context.Context, providers []string) error
	// DefaultProvider returns the provider name of the user's is_default key among
	// the given providers, or "" if none.
	DefaultProvider(ctx context.Context, providers []string) (string, error)
```

- [ ] **Step 2: store — 迁移 + 实现**

`infra/store/apikey/apikey.go`:AutoMigrate 已建表;`is_default` 列由 GORM tag 自动加(纯 Go SQLite + AutoMigrate 支持加列)。实现:
```go
func (s *Store) ClearDefaultForCategory(ctx context.Context, providers []string) error {
	uid := reqctxpkg.UserID(ctx)
	return s.db.WithContext(ctx).Model(&apikeydomain.APIKey{}).
		Where("user_id = ? AND provider IN ?", uid, providers).
		Update("is_default", false).Error
}
func (s *Store) DefaultProvider(ctx context.Context, providers []string) (string, error) {
	uid := reqctxpkg.UserID(ctx)
	var k apikeydomain.APIKey
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND provider IN ? AND is_default = ?", uid, providers, true).
		First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return k.Provider, err
}
```
(import `errors` + `gorm.io/gorm` 若未引入。`reqctxpkg` 取 userID 的方式照该文件现有写法。)

- [ ] **Step 3: app — UpdateInput.IsDefault + 单选**

`app/apikey/apikey.go`:`UpdateInput` 加 `IsDefault *bool`。`Update` 的 no-op 判断加 IsDefault;并在 set 为 true 时清同 category 其它默认:
```go
	if in.DisplayName == nil && in.BaseURL == nil && in.Key == nil && in.IsDefault == nil {
		return k, nil
	}
	// ... existing DisplayName/BaseURL/Key handling ...
	if in.IsDefault != nil {
		if *in.IsDefault {
			cat := providerCategory(k.Provider)            // from providers.go registry
			if err := s.repo.ClearDefaultForCategory(ctx, providersInCategory(cat)); err != nil {
				return nil, fmt.Errorf("apikey.Update: %w", err)
			}
		}
		k.IsDefault = *in.IsDefault
	}
```
`providerCategory(name)` / `providersInCategory(cat)` —— 用 `app/apikey/providers.go` 的 registry(`ProviderInfo.Category`)写两个小 helper(遍历 registry)。

- [ ] **Step 4: handler — 接 isDefault**

`handlers/apikey.go` 的 `updateRequest` 加 `IsDefault *bool \`json:"isDefault"\``;`Update` 传 `IsDefault: req.IsDefault`。

- [ ] **Step 5: WebSearch 优先默认**

KeyProvider port(WebSearch 消费的接口,`app/tool/web/web.go` 附近)加:
```go
	DefaultSearchProvider(ctx context.Context) string  // "" if none
```
apikey app 实现该 port 的适配器加一个方法,调 `repo.DefaultProvider(ctx, apikeydomain.SearchProviderPriority)`(忽略 err 返 "")。`search.go` 的 Execute,在 `for _, provider := range SearchProviderPriority` 之前先试默认:
```go
	if t.keys != nil {
		order := apikeydomain.SearchProviderPriority
		if def := t.keys.DefaultSearchProvider(ctx); def != "" {
			order = append([]string{def}, removeStr(apikeydomain.SearchProviderPriority, def)...)
		}
		for _, provider := range order {
			...
		}
	}
```
(`removeStr` 小 helper:去掉切片里的某项,避免重复试。)

- [ ] **Step 6: 测试 + 编译**

`apikey_test.go` 加:`TestUpdate_SetDefault_ClearsSiblings`(同 category 两 key,设一个默认 → 另一个 false);`TestDefaultProvider_ReturnsMarked`。
Run: `cd backend && go build ./... && make test-unit && staticcheck ./...`
Expected: 全绿(含新用例)。

- [ ] **Step 7: 提交 + 文档**

同步 `apikey.md`(IsDefault 字段 + PATCH isDefault + DefaultProvider 语义)/ `database-design.md`(is_default 列)/ `api-design.md`(PATCH 加 isDefault)/ `events-design.md` 无关。
```bash
git add backend/internal documents/version-1.2/service-design-documents/apikey.md documents/version-1.2/service-contract-documents
git commit -m "feat(backend): api-key is_default (per-category single-choice) + WebSearch prefers default search provider"
```

---

## Task 2: 前端 — 抽共享 config 组件(引导页 → 复用)

**Files:**
- Create: `frontend/src/components/config/ProviderGrid.jsx`, `KeyVerifyField.jsx`, `ModelSelect.jsx`
- Modify: `frontend/src/components/overlays/Onboarding.jsx`
- Modify: `frontend/src/components/overlays/Onboarding.test.jsx`(若选择器变)

抽取依据:`Onboarding.jsx` 现有内联的 `ProviderGrid`、Model 步的 key 输入/验证、`<select className="onb-mselect">`。三个组件接口:

- [ ] **Step 1: ProviderGrid.jsx**

```jsx
// Shared provider card grid (model + search config). Cards show a ✓ when the
// provider already has a stored key. Single-select.
import { Icon } from "../primitives/Icon.jsx";

export function ProviderGrid({ providers, hints, selected, onPick, configured = [], tall = false }) {
  return (
    <div className="onb-gridwrap">
      <div className={"onb-grid" + (tall ? " is-tall" : "")}>
        {providers.map((p) => {
          const h = hints[p.name] || { abbr: p.name.slice(0, 2).toUpperCase(), color: "#6b6459" };
          return (
            <button key={p.name} type="button"
              className={"onb-prov" + (selected === p.name ? " is-active" : "")}
              onClick={() => onPick(p.name)}>
              <span className="onb-pchip" style={{ background: h.color }}>{h.abbr}</span>
              <span style={{ minWidth: 0 }}>
                <span className="onb-pname">{p.displayName || p.name}</span>
                <span className="onb-pdesc" style={{ display: "block" }}>{p.defaultBaseUrl?.replace(/^https?:\/\//, "") || ""}</span>
              </span>
              {configured.includes(p.name) && <span className="onb-prov-ck"><Icon.Check /></span>}
            </button>
          );
        })}
      </div>
      {!tall && <div className="onb-grid-fade" />}
    </div>
  );
}
```
(CSS：复用现有 `.onb-grid/.onb-prov/...`;新增 `.onb-prov-ck`(右上角绿 ✓ 圆点)到 components.css。`LLM_HINTS`/`SEARCH_HINTS` 从 `onboarding-strings.js` 导出复用。)

- [ ] **Step 2: KeyVerifyField.jsx**

封装 key 输入 + 验证按钮/验证态/红框错误。Props:`{ label, value, onChange, onVerify, verifying, verified, error, placeholder }`。Markup 复用 `.onb-kinput`(+`is-error`)+ `.onb-verify-btn`/`.onb-verified` + `.onb-verify-err`(从 onboarding 抽)。

- [ ] **Step 3: ModelSelect.jsx**

```jsx
export function ModelSelect({ models, value, onChange, disabled }) {
  return (
    <select className="onb-mselect" value={value} onChange={(e) => onChange(e.target.value)} disabled={disabled}>
      {disabled && !models.length && <option>验证后可选</option>}
      {models.map((m) => <option key={m} value={m}>{m}</option>)}
    </select>
  );
}
```

- [ ] **Step 4: Onboarding 改为消费共享组件**

`Onboarding.jsx`:删内联 `ProviderGrid` 函数,import 共享的;Model/Search 步用 `<ProviderGrid>`/`<KeyVerifyField>`/`<ModelSelect>`。逻辑(verify/state)不变。

- [ ] **Step 5: 测试 + 构建**

Run: `cd frontend && npx vitest run src/components/overlays/Onboarding.test.jsx && npm run build`
Expected: onboarding 测试全绿(选择器若因组件化变了同步改)、build 干净。

- [ ] **Step 6: 提交**

```bash
git add frontend/src/components/config frontend/src/components/overlays/Onboarding.jsx frontend/src/components/overlays/Onboarding.test.jsx frontend/src/styles/components.css
git commit -m "refactor(frontend): extract ProviderGrid/KeyVerifyField/ModelSelect for onboarding+settings reuse"
```

---

## Task 3: 前端 — SettingsModal 壳 + 单开手风琴 + 账号区 + CSS

**Files:**
- Create: `frontend/src/components/overlays/SettingsModal.jsx`
- Modify: `frontend/src/styles/components.css`
- Test: `frontend/src/components/overlays/SettingsModal.test.jsx`

按原型 `docs/superpowers/prototypes/2026-05-25-settings.html` port:居中 overlay + `--bg-overlay` 遮罩 + `Esc`/遮罩关闭 + 入场 `scaleIn`。结构:`.set-modal`(radius 16)> head(标题"设置"+关闭)+ body(账号区 + `<ApiKeysSection/>` + `<SearchSection/>` + `<AppearanceSection/>` + `<SystemSection/>`)。

**单开手风琴**:modal 持 `openSection` state(`"keys"|"search"|"look"|"sys"|null`);每个 section 收 `open` + `onToggle`,点标题 `setOpenSection(prev => prev===k ? null : k)`。账号区常驻。

CSS:把原型的 `.set-*`/section/手风琴样式(参照 §9 风格规约,radius 14/16、pill、`--bg-hover/active`、`--t-fast` 过渡)落到 components.css,新 class 前缀统一 `set-`,选择类复用 `onb-seg`/`onb-grid` 等。

- [ ] Step 1: SettingsModal.jsx(壳 + openSection + 账号区 + Esc/遮罩关闭,gated on `useUIStore(s=>s.settingsOpen)`)。账号区从 `SettingsPopover.jsx::AccountSection` 迁(头像 + displayName + 切换/新建,用 `useUsers/useCreateUser/useDisplayName`)。
- [ ] Step 2: components.css 加 `.set-modal/.set-head/.set-body/.set-sec/.set-sec-h/...` + 手风琴过渡。
- [ ] Step 3: SettingsModal.test.jsx:`settingsOpen=true` 渲染 4 个 section 标题;点 keys 标题展开、再点 search 标题 → keys 收起(互斥)。
- [ ] Step 4: build + vitest 该文件。
- [ ] Step 5: 提交 `feat(frontend): SettingsModal shell + single-open accordion + account`。

> 注:此时 SettingsModal 尚未挂载(Task 7 才 wire),不影响现有 UI。section 组件先放占位再在 Task 4-6 填充,或 Task 4-6 先于本任务的 Step 1 引用——实现时按 Task 顺序,section 文件在 4-6 创建,Task 3 import 它们(若先做 3 则 section 先建空壳)。

---

## Task 4: 前端 — ApiKeysSection(key 为中心 + 引导页式新增)

**Files:**
- Create: `frontend/src/components/config/ApiKeysSection.jsx`
- Test: `frontend/src/components/config/ApiKeysSection.test.jsx`

数据:`useApiKeys`(列表,filter category=llm via `useProviders` 的 category)、`useProviders`、`useCreateApiKey`、`useTestApiKey`、`useDeleteApiKey`、`useUpdateApiKey`、`useModelConfigs`、`useUpsertModelConfig`。按原型结构:

- key 列表:每行 厂商色块 + 名 + 掩码 + 已选模型 + `对话默认` 徽章(= chat model-config.provider === 该 key.provider)+ 验证徽章;**行互斥单开**(section 内一次开一个,modal 传下的 openKey state 或本地)。
- key 详情(展开):`<ModelSelect models={key.modelsFound}>` + 用途分段(对话默认/仅备用)+ 重新验证/删除。
  - "对话默认" = `useUpsertModelConfig().mutate({scenario:"chat", provider:key.provider, modelId:选中})`;跨 key 单选天然成立(chat config 只有一个 provider)。
  - 模型改变 = 同上 upsert(若该 key 是对话默认)。
- "+ API Key" 加号 → 本地 `adding` state 切换出 `<ProviderGrid providers={llm} configured={已有key的provider} onPick>` + 选中后 `<KeyVerifyField>`(create→test 流,复用引导页 verify 逻辑)+ `<ModelSelect>` + 保存(写 api-key,若用户勾对话默认则 upsert model-config)。

- [ ] Step 1: 写组件(按上)。
- [ ] Step 2: ApiKeysSection.test.jsx(mock config hooks):列表渲染;"+ API Key"→ ProviderGrid 出现;选 provider→KeyVerifyField 出现;验证成功→ ModelSelect 出现。
- [ ] Step 3: build + vitest。
- [ ] Step 4: 提交 `feat(frontend): settings ApiKeysSection (key-centric + onboarding-style add)`。

---

## Task 5: 前端 — SearchSection(搜索默认 → isDefault)

**Files:**
- Create: `frontend/src/components/config/SearchSection.jsx`
- Test: `frontend/src/components/config/SearchSection.test.jsx`

同 Task 4 模式,差别:provider 用 search 类(`useProviders` filter category=search);key 详情**无模型**;用途分段 `搜索默认/仅备用` → `useUpdateApiKey(id).mutate({ isDefault: true })`(后端单选,Task 1)。新增用 `<ProviderGrid providers={search} hints={SEARCH_HINTS}>` + `<KeyVerifyField>`(无 ModelSelect)。标「可选」。

- [ ] Step 1-4: 写组件 + test(搜索默认点击发 isDefault PATCH)+ build + 提交 `feat(frontend): settings SearchSection (search key mgmt + default)`。

---

## Task 6: 前端 — AppearanceSection + SystemSection

**Files:**
- Create: `frontend/src/components/config/AppearanceSection.jsx`, `SystemSection.jsx`

- AppearanceSection:行式 主题/色调/密度/语言/推理 —— 分段(`onb-seg` 复用)+ 色块 swatch,直写 `useSettings().set(...)`(迁自 `ConfigPane::AppearanceTab` + `SettingsPopover` 的控件,合一)。
- SystemSection:只读 数据目录 `~/.forgify/` + 沙箱 `mise 内置` + 版本 `Forgify v1.2`(迁自 ConfigPane Sandbox/Data tab)。

- [ ] Step 1-3: 写两组件 + build + 提交 `feat(frontend): settings Appearance + System sections`。(纯前端无 IO,浏览器验证为主,可不加重单测。)

---

## Task 7: 前端 — 接齿轮 + 删 SettingsPopover/ConfigPane(原子切换)

**Files:**
- Modify: `frontend/src/store/ui.js`(`settingsPopOpen`→`settingsOpen` + `setSettingsOpen`;删 popover/config 残留)
- Modify: `frontend/src/components/layout/Sidebar.jsx:181`(`setSettingsPopOpen(true)`→`setSettingsOpen(true)`)
- Modify: 挂载点(`App.jsx` 或 `AppShell.jsx`):`<SettingsModal/>` 取代 `<SettingsPopover/>`
- Delete: `SettingsPopover.jsx` + `SettingsPopover.test.jsx`;`panes/config/ConfigPane.jsx`
- Modify: pane 注册表(grep `"config"` / `ConfigPane` 引用:PaneFrame/PANE_META/router 等)移除 config pane

- [ ] Step 1: ui.js 改名 + 清状态(grep `settingsPopOpen` 全删)。
- [ ] Step 2: Sidebar 齿轮 onClick 改。
- [ ] Step 3: 挂载 `<SettingsModal/>`;移除 `<SettingsPopover/>`。
- [ ] Step 4: 删两文件 + 移除 config pane 注册(`grep -rn "ConfigPane\|\"config\"" src` 清干净)。
- [ ] Step 5: build + 全量 vitest(删了 SettingsPopover.test;若有测试引用 config pane 同步删/改)。
- [ ] Step 6: 提交 `feat(frontend): gear opens SettingsModal; remove SettingsPopover + ConfigPane tabs`。

---

## Task 8: 浏览器验证(make dev + Playwright probe)

- [ ] `make dev`(repo 根);probe `frontend/tests/manual/probe-settings.mjs`(仿 probe-onboarding):开 app(已 onboarded 的 mock user)→ 点齿轮 → modal 出现 → 四 section 互斥 → 加 key 流(选 provider→key→验证)→ 外观实时 → `Esc` 关闭;截图 + 抓 console error / 401。逐项核对风格(radius/pill/过渡)对齐侧边栏。
- [ ] 修发现的视觉/交互 bug(最小干预);PRD §16 记录。

---

## Task 9: 文档同步

- [ ] `frontend-prd.md`:设置章节重写(modal + key 为中心 + 引导页式新增 + 手风琴互斥 + 搜索默认);§16 记录(去 tab/去重复/抽共享组件/删 SettingsPopover+ConfigPane);§17 endpoint(PATCH /api-keys isDefault)。
- [ ] `DESIGN.md`:居中 modal + 单开手风琴 + pill/段控 若成新约定 → §11/§12 补。
- [ ] `progress-record.md`:dev log(后端 is_default + 前端设置重做 + 共享组件抽取 + 测试数)。
- [ ] 提交 `docs: settings redesign sync (PRD/DESIGN/progress + apikey contract)`。

---

## Self-Review

**Spec coverage:** §3 居中 modal+删两套→Task 3+7;§4 结构+手风琴互斥→Task 3(+ Task 4 行互斥);§5 key 为中心+引导页式新增→Task 4;§6 搜索→Task 5(+ Task 1 后端);§7 外观→Task 6;§8 系统→Task 6;§9 风格→贯穿(原型+侧边栏基准,Task 3 CSS + Task 8 核对);§10 抽共享组件→Task 2;§11 后端 isDefault→Task 1;§12 触发+删除→Task 7;§14 测试→各 Task + Task 8;§15 文档→Task 9。全覆盖。

**Placeholder scan:** Task 3 末注"section 文件在 4-6 创建"是顺序说明非占位(给了两种落地顺序)。Task 1 backend helper(`providerCategory`/`providersInCategory`/`removeStr`)指明从 registry 写小函数,非 TBD。UI 任务以"按已入库原型 port"+ 明确组件接口/数据 hook 落地,符合本项目"原型即视觉契约"的既有做法(同 onboarding plan)。

**Type consistency:** `IsDefault`(domain)↔`UpdateInput.IsDefault *bool`(app)↔`updateRequest.IsDefault *bool`(handler)↔ `useUpdateApiKey().mutate({isDefault})`(前端)一致;`DefaultSearchProvider`/`DefaultProvider`/`ClearDefaultForCategory` 命名贯穿一致;共享组件 props(ProviderGrid `{providers,hints,selected,onPick,configured,tall}` / ModelSelect `{models,value,onChange,disabled}`)在 Task 2 定义、Task 4/5 同形使用。
