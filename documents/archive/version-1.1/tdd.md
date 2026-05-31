# Forgify V1.1 — 技术设计文档

**版本**：v1.1  
**日期**：2026-04-20  
**配套 PRD**：PRD_1.1.md  
**基于**：TDD_1.0.md (v0.4) + Tier 1-4 实际代码

---

## 0. 变更范围

V1.1 是**纯前端架构重构 + 少量后端增强**。所有现有后端 API 不变。

| 层 | 变更量 | 说明 |
|---|---|---|
| 前端 | **大** | Tab 系统、Layout 组件、状态管理重构 |
| 后端 | **小** | 新增 5 个 API（元数据编辑、标签、版本、测试用例） |
| 数据库 | **小** | 1 个迁移（3 张新表） |

---

## 1. 前端架构：Tab 系统

### 1.1 核心数据结构

```typescript
// context/TabContext.tsx

type LayoutType = 'chat' | 'tool' | 'workflow' | 'chat-tool' | 'chat-workflow'

interface TabItem {
  id: string                    // 唯一标识（UUID）
  layout: LayoutType            // 布局类型
  label: string                 // Tab 标签文字
  icon?: string                 // Tab 图标
  pinned?: boolean              // 固定 Tab（Home/Inbox/Settings）不可关闭
  // 各布局类型的参数
  conversationId?: string       // chat / chat-tool / chat-workflow
  toolId?: string               // tool / chat-tool
  workflowId?: string           // workflow / chat-workflow
}

interface TabContextValue {
  tabs: TabItem[]
  activeTabId: string | null
  openTab: (tab: Omit<TabItem, 'id'>) => string   // 返回新 Tab ID
  closeTab: (id: string) => void
  setActiveTab: (id: string) => void
  updateTab: (id: string, patch: Partial<TabItem>) => void  // 用于布局升级
}
```

### 1.2 Tab 状态持久化

```typescript
// Tab 列表持久化到 localStorage
const STORAGE_KEY = 'forgify.tabs'

// 保存：tabs 数组 + activeTabId
// 恢复：App 启动时从 localStorage 读取，若为空则打开默认 Home Tab
```

### 1.3 Tab Bar 组件

```
文件：components/TabBar.tsx
```

```
┌─[🏠 Home]─[💬 报价讨论]─[📦 send_email]─[💬 新对话]─[+]─────────┐
└──────────────────────────────────────────────────────────────────┘
```

- 每个 Tab：图标 + label + 关闭按钮（pinned 的显示 Pin 图标代替 ×）
- 活跃 Tab 有底部高亮线
- `[+]` 按钮：打开新的空 Chat Tab
- Tab 过多时水平滚动（`overflow-x: auto, scrollbarWidth: 'none'`）
- Tab 宽度：最大 180px，最小 60px，文字超出截断
- **拖拽排序**：HTML5 原生 drag-and-drop，拖拽时 `opacity: 0.4`，目标位置蓝色 `boxShadow` 指示线
- **右键上下文菜单**：固定/取消固定、关闭、关闭其他、关闭右侧、关闭全部
- **窗口拖拽**：TabBar 外层 `WebkitAppRegion: 'drag'`，Tab 按钮和 + 按钮 `no-drag`

### 1.4 Layout 组件

```
文件：components/layouts/
├── ChatLayout.tsx          # 全宽对话
├── ToolLayout.tsx          # 全宽工具视图
├── WorkflowLayout.tsx      # 全宽工作流画布（Tier 5 实现，当前占位）
├── ChatToolLayout.tsx      # 左对话 右工具（可拖拽分屏）
├── ChatWorkflowLayout.tsx  # 左对话 右画布（Tier 5 实现，当前占位）
└── SplitContainer.tsx      # 可拖拽分屏 + 可收起的通用容器
```

**每个 Layout 接收的 Props：**

```typescript
// ChatLayout
{ conversationId: string }

// ToolLayout
{ toolId: string }

// WorkflowLayout
{ workflowId: string }

// ChatToolLayout
{ conversationId: string; toolId: string }

// ChatWorkflowLayout
{ conversationId: string; workflowId: string }
```

**Layout 内部完全复用现有 Page 组件：**

```tsx
// ChatLayout.tsx — 就是现有的 ChatContent
export function ChatLayout({ conversationId }: { conversationId: string }) {
  return <ChatContent activeConversationId={conversationId} />
}

// ToolLayout.tsx — 就是现有的 ToolMainView
export function ToolLayout({ toolId }: { toolId: string }) {
  return <ToolMainView toolId={toolId} onDeleted={() => {}} />
}

// ChatToolLayout.tsx — 分屏容器包裹两者
export function ChatToolLayout({ conversationId, toolId }: Props) {
  return (
    <SplitContainer
      left={<ChatContent activeConversationId={conversationId} />}
      right={<ToolMainView toolId={toolId} onDeleted={() => {}} />}
      rightLabel={toolName}
      storageKey={`forgify.split.${conversationId}`}
    />
  )
}
```

### 1.5 SplitContainer 组件

```
文件：components/layouts/SplitContainer.tsx
```

通用的可拖拽分屏 + 可收起容器：

```typescript
interface Props {
  left: ReactNode
  right: ReactNode
  rightLabel: string           // 收起时竖条上显示的文字
  rightIcon?: string           // 收起时竖条上显示的图标
  storageKey: string           // localStorage key，记忆收起/展开 + 宽度
  defaultRightWidth?: number   // 默认右侧宽度（px），默认 50%
  minLeft?: number             // 左侧最小宽度，默认 300
  minRight?: number            // 右侧最小宽度，默认 320
}
```

**展开状态：** 左右两栏 + 中间拖拽条 + 双向折叠按钮  
**右侧收起状态：** 左侧全宽 + 右侧 36px 竖条（📦 + 工具名竖排）  
**左侧收起状态：** 左侧 36px 竖条（💬 + 对话标题竖排）+ 右侧全宽

**实现**：`ChatToolLayout.tsx` 直接实现分屏（未使用通用 SplitContainer），`collapsedSide: 'none' | 'left' | 'right'` 状态机确保互斥。

**左侧折叠按钮**：浮在聊天内容上方，用 `position: absolute` + 白色渐变遮罩（`linear-gradient(white 30%, transparent)`），消息自然淡出。
**右侧折叠按钮**：header bar + `borderBottom: 1px solid #f3f4f6` + ChevronRight 图标。

### 1.6 App.tsx 重构

```tsx
// 新的 App.tsx 结构
function App() {
  return (
    <ErrorBoundary>
    <LocaleProvider>
    <InboxProvider>
    <ChatProvider>
    <TabProvider>
      <div className="flex h-screen w-screen overflow-hidden bg-white">
        {/* 左侧导航 + 列表面板 */}
        <NavSidebar />
        
        {/* 右侧内容区 */}
        <div className="flex-1 flex flex-col min-w-0">
          <TabBar />
          <div className="flex-1 overflow-hidden">
            <TabContent />   {/* 根据 activeTab.layout 渲染对应 Layout */}
          </div>
        </div>
      </div>
    </TabProvider>
    </ChatProvider>
    </InboxProvider>
    </LocaleProvider>
    </ErrorBoundary>
  )
}
```

### 1.7 NavSidebar 重构

```
文件：components/NavSidebar.tsx（替代现有 SidebarNav.tsx）
```

Nav 图标点击后，侧边栏展开显示对应列表：

```
┌──────────────────────┐
│ 🏠 Home              │  ← 点击：切到 Home Tab
│                      │
│ 💬 对话    [+]       │  ← 点击展开对话列表
│   报价讨论           │     点击项目 → openTab({layout:'chat', conversationId})
│   新对话             │
│   ...                │
│                      │
│ 📦 资产    [+]       │  ← 点击展开工具/工作流列表
│  工具                │
│    send_email        │     点击 → openTab({layout:'tool', toolId})
│    parse_excel       │
│  工作流              │
│    报价处理          │     点击 → openTab({layout:'workflow', workflowId})
│                      │
│ 📥 Inbox             │  ← 点击：切到 Inbox Tab
│ ⚙️ 设置              │  ← 点击：切到 Settings Tab
└──────────────────────┘
```

**关键行为：**
- 点击已展开的分类 → 收起列表
- 列表中点击项目 → 如果已有相同内容的 Tab 则切换过去，否则打开新 Tab
- `[+]` 按钮 → 创建新项目（新对话 / 新工具锻造）

### 1.8 布局升级机制

当对话中 AI 创建了工具（`forge.code_detected` 事件触发后）：

```typescript
// 在 ChatContent 或 useChat 中监听绑定事件
onEvent(EventNames.ChatBound, ({ conversationId, assetId, assetType }) => {
  if (assetType === 'tool') {
    // 找到当前 Tab
    const tab = tabs.find(t => t.conversationId === conversationId)
    if (tab && tab.layout === 'chat') {
      // 升级布局：chat → chat-tool
      updateTab(tab.id, {
        layout: 'chat-tool',
        toolId: assetId,
      })
    }
  }
})
```

### 1.9 Tab 状态保持

**关键：切换 Tab 时不 unmount 组件。**

```tsx
// TabContent.tsx
function TabContent() {
  const { tabs, activeTabId } = useTabContext()
  
  return (
    <>
      {tabs.map(tab => (
        <div
          key={tab.id}
          style={{ display: tab.id === activeTabId ? 'flex' : 'none', height: '100%' }}
        >
          <LayoutRouter tab={tab} />
        </div>
      ))}
    </>
  )
}
```

使用 `display: none` 隐藏非活跃 Tab，而不是条件渲染。这样：
- 对话的滚动位置保持
- 输入框内容保持
- Monaco Editor 状态保持
- 不需要额外的状态恢复逻辑

**注意：** Tab 数量过多时（>10），考虑用 LRU 策略 unmount 最久未使用的 Tab 以节省内存。

---

## 2. 后端增强

### 2.1 数据库迁移

```
文件：backend/internal/storage/migrations/005_tool_enhancements.sql
```

```sql
-- 工具版本历史
CREATE TABLE IF NOT EXISTS tool_versions (
    id              TEXT PRIMARY KEY,
    tool_id         TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    code            TEXT NOT NULL,
    change_summary  TEXT NOT NULL DEFAULT '',
    created_at      DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tool_versions ON tool_versions(tool_id, version DESC);

-- 工具标签
CREATE TABLE IF NOT EXISTS tool_tags (
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    tag         TEXT NOT NULL,
    PRIMARY KEY (tool_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_tool_tags ON tool_tags(tag);

-- 测试用例
CREATE TABLE IF NOT EXISTS tool_test_cases (
    id          TEXT PRIMARY KEY,
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT 'Default',
    params_json TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tool_test_cases ON tool_test_cases(tool_id);
```

### 2.2 新增 API

```go
// 元数据编辑（部分更新）
PATCH  /api/tools/{id}/meta              // body: { displayName?, description?, category? }

// 标签
GET    /api/tools/{id}/tags              // → ["供应商", "Q2"]
POST   /api/tools/{id}/tags             // body: { tag: "供应商" }
DELETE /api/tools/{id}/tags/{tag}

// 版本
GET    /api/tools/{id}/versions          // → [{version, changeSummary, createdAt}]
POST   /api/tools/{id}/versions/{v}/restore

// 测试用例
GET    /api/tools/{id}/test-cases        // → [{id, name, paramsJson}]
POST   /api/tools/{id}/test-cases       // body: { name, params }
DELETE /api/test-cases/{id}
```

### 2.3 元数据标注同步（NormalizeCodeAnnotations）

```go
// forge/parser.go — 确保代码 # @ 注释与 DB 字段一致
func NormalizeCodeAnnotations(code, displayName, description, category, version string, isBuiltin bool, requiresKey string) string
```

算法：扫描代码顶部 `# @xxx` 行块 → 替换为完整标注（固定顺序：@builtin/@custom → @version → @category → @display_name → @description → @requires_key）→ 保留非标注注释 → 确保空行分隔。

**调用位置：**
- `Save()` — INSERT 前自动归一化，覆盖所有保存路径
- `UpdateMeta()` — read-modify-write，同时更新 DB 字段 + 代码标注
- `AcceptPendingChange()` — 接受 AI 代码后从新代码解析元数据更新 DB，再归一化

### 2.4 ToolService 扩展

```go
// service/tool.go 新增方法

func (s *ToolService) UpdateMeta(id string, displayName, description, category *string) error
// 现在是 read-modify-write：读取完整 tool → 合并字段 → NormalizeCodeAnnotations → 更新 DB+code
func (s *ToolService) AddTag(id, tag string) error
func (s *ToolService) RemoveTag(id, tag string) error
func (s *ToolService) ListTags(id string) ([]string, error)
func (s *ToolService) SaveVersion(toolID, code, summary string) error  // Save() 内部调用
func (s *ToolService) ListVersions(toolID string) ([]*ToolVersion, error)
func (s *ToolService) RestoreVersion(toolID string, version int) error
func (s *ToolService) SaveTestCase(toolID, name string, params map[string]any) error
func (s *ToolService) ListTestCases(toolID string) ([]*ToolTestCase, error)
func (s *ToolService) DeleteTestCase(id string) error
```

**自动版本逻辑**：在 `Save()` 方法中，如果 code 字段变化，自动调用 `SaveVersion()` 保存旧版本。

### 2.4 AI 工具列表注入

```go
// service/chat.go — loadHistory() 中注入

func (s *ChatService) buildToolSummary() string {
    tools, _ := s.toolSvc.List("", "")
    if len(tools) == 0 { return "" }
    var sb strings.Builder
    sb.WriteString("[用户已有工具]\n")
    for _, t := range tools {
        sb.WriteString(fmt.Sprintf("- %s (%s, %s)\n", t.Name, t.Category, t.Status))
    }
    sb.WriteString("如果用户需要的功能已有工具可用，优先推荐使用。\n")
    return sb.String()
}
```

---

## 3. 目录结构变更

```
frontend/src/
├── context/
│   ├── ChatContext.tsx          # 不变
│   ├── InboxContext.tsx         # 不变
│   └── TabContext.tsx           # 【新增】Tab 状态管理
├── components/
│   ├── TabBar.tsx               # 【新增】标签栏
│   ├── NavSidebar.tsx           # 【新增】替代 SidebarNav.tsx
│   ├── ErrorBoundary.tsx        # 不变
│   ├── layouts/                 # 【新增目录】
│   │   ├── ChatLayout.tsx
│   │   ├── ToolLayout.tsx
│   │   ├── WorkflowLayout.tsx   # 占位
│   │   ├── ChatToolLayout.tsx
│   │   ├── ChatWorkflowLayout.tsx  # 占位
│   │   ├── SplitContainer.tsx
│   │   └── LayoutRouter.tsx     # 根据 TabItem.layout 分发
│   ├── chat/                    # 不变（ChatInput, MessageList, MessageItem 等）
│   ├── tools/                   # 不变（ToolMainView, ToolCard 等）
│   ├── forge/                   # 不变（ForgeCodeBlock, TestParamsModal 等）
│   ├── settings/                # 不变
│   └── common/                  # 不变
├── pages/                       # 重构
│   ├── ChatContent.tsx          # 从 ChatPage.tsx 抽出，接收 conversationId prop
│   ├── ToolContent.tsx          # 从 AssetsPage.tsx 抽出（就是 ToolMainView 包装）
│   ├── HomePage.tsx             # 不变（Tier 9 实现）
│   ├── InboxPage.tsx            # 不变（Tier 8 实现）
│   └── SettingsPage.tsx         # 不变
├── hooks/
│   ├── useChat.ts               # 不变
│   └── useTab.ts                # 【新增】Tab 操作 hook
├── lib/                         # 不变
└── App.tsx                      # 【重构】TabProvider + TabBar + TabContent
```

**删除的文件：**
- `pages/ChatPage.tsx` → 拆为 `pages/ChatContent.tsx`（只保留内容，不管左右面板）
- `pages/AssetsPage.tsx` → 拆为 Nav 中的列表 + `ToolContent.tsx`
- `components/SidebarNav.tsx` → 替换为 `NavSidebar.tsx`
- `components/SplitView.tsx` → 替换为 `layouts/SplitContainer.tsx`

---

## 4. 实现顺序

```
Phase 1: Tab 基础设施（约 6 个文件）
  ├── TabContext.tsx
  ├── TabBar.tsx
  ├── LayoutRouter.tsx
  ├── App.tsx 重构
  ├── NavSidebar.tsx
  └── 迁移现有 Page 组件为 Layout

Phase 2: 分屏布局（约 3 个文件）
  ├── SplitContainer.tsx
  ├── ChatToolLayout.tsx
  └── 布局升级逻辑（ChatBound 事件 → updateTab）

Phase 3: 后端增强（约 3 个文件）
  ├── 005_tool_enhancements.sql
  ├── service/tool.go 扩展
  └── server/routes_tools.go 新端点

Phase 4: 工具 UI 增强（已完成大部分）
  ├── ToolMainView — InlineEdit (名称/描述) + InlineSelect (分类下拉) + 版本 badge
  ├── TagBar 组件 — 标签增删
  ├── VersionHistoryView — 版本列表(180px) + Monaco DiffEditor(side-by-side) + 恢复按钮
  │   └── 通过 ToolMainView 的 historyMode 状态切换，替换 tabs 区域
  ├── NormalizeCodeAnnotations — UI 编辑 → 代码 # @ 标注同步
  └── TestCaseSelector 组件（待做）
```

---

## 5. 验证方案

```
Phase 1 验证：
  - 打开 App → 显示 Home Tab
  - 点 Nav "对话" → 列表展开 → 点击对话 → 开 Chat Tab
  - 点 Nav "资产" → 列表展开 → 点击工具 → 开 Tool Tab
  - 同时打开 3 个 Tab → Tab Bar 显示 3 个标签 → 自由切换
  - 关闭 Tab → 切到相邻 Tab
  - 刷新页面 → Tab 状态恢复

Phase 2 验证：
  - 在 Chat Tab 中让 AI 创建工具 → Tab 自动升级为 Chat+Tool
  - 拖拽分割线调节宽度
  - 收起右侧为竖条 → 点击展开
  - AI 生成新代码 → 右侧面板自动刷新

Phase 3 验证：
  - PATCH /api/tools/{id}/meta → 元数据更新
  - POST/DELETE /api/tools/{id}/tags → 标签增删
  - GET /api/tools/{id}/versions → 版本列表
  - POST /api/tools/{id}/versions/{v}/restore → 回滚成功

Phase 4 验证：
  - 点击工具名 → inline 编辑 → blur 保存
  - 添加标签 → 工具库筛选可见
  - 编辑代码保存 → 版本列表出现新版本
  - 保存测试用例 → 下次加载可用
```

---

**文档结束**
