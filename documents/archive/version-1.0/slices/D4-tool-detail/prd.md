# D4 · 工具详情（代码视图）— 产品需求文档

**切片**：D4  
**状态**：待 Review  
**依赖**：D1、D3  
**下游**：D5（导出入口在此）

---

## 1. 这块做什么

点击工具后进入工具代码视图（ToolMainView）。在独立 Tool Tab 中全宽显示，在 Chat+Tool Tab 中作为右侧面板。这是用户查看、编辑、测试工具的主要界面。

---

## 2. 布局

> **V1.1 Tab 架构**：工具视图（ToolMainView）在两种场景中使用：
> 1. **独立 Tool Tab**：Nav 侧边栏"资产"列表点击工具 → 打开 Tool Tab，ToolMainView 占全宽
> 2. **Chat+Tool 分屏右侧**：对话绑定工具后 → Chat+Tool Tab，左对话右 ToolMainView（可双向折叠）

---

## 3. 工具代码视图（ToolMainView）

```
┌──────────────────────────────────────────────────────┐
│ 📦 [发送邮件✏️]        Built-in   [导出]  [删除]      │  ← InlineEdit
│ [通过SMTP发送邮件✏️]                                  │  ← InlineEdit
│ [邮件▼]  [v1.0↗]                                     │  ← InlineSelect + 版本badge
│ [供应商] [Q2项目] [+ 添加标签]                         │  ← TagBar
│ ──────────────────────────────────────────────────── │
│ [代码]  [参数]  [测试]                                 │
│ ──────────────────────────────────────────────────── │
│                                                      │
│  （Tab 内容区，占满剩余高度）                          │
│                                                      │
└──────────────────────────────────────────────────────┘
```

### 3.1 Header 区域

- **名称**：InlineEdit（Notion 风格点击编辑，blur/Enter 保存）
- **描述**：InlineEdit
- **分类**：InlineSelect（点击弹出下拉菜单，6 个枚举值带 i18n 标签）
- **版本 badge**：`v1.0 ↗`，点击进入版本历史模式
- **标签**：TagBar 组件（增删自由标签）
- 所有编辑通过 `PATCH /api/tools/{id}/meta` 保存，后端同步更新代码 `# @` 标注
- Built-in 工具全部只读

### 3.2 代码 Tab

- Monaco Editor，Python 语法高亮
- 默认只读；点击"编辑"按钮切换为可编辑
- 编辑后有"保存"和"取消"
- AI 代码变更（pending change）显示 diff review 模式（InlineDiff 红绿行级 diff + 接受/拒绝按钮）
- "AI 正在思考"小 banner（非阻塞）

### 3.3 参数 Tab

从代码函数签名自动解析（Python AST）的参数表格：

| 参数名 | 类型 | 必填 | 说明 |
|---|---|---|---|
| to | string | 是 | 收件人邮箱 |
| subject | string | 是 | 邮件主题 |

### 3.4 测试 Tab

- 动态表单（从参数 schema 生成输入框）
- "▶ 运行测试"按钮
- 测试结果实时显示（success/error + 输出内容 + 耗时）
- 测试历史列表（最近 20 条，按时间倒序）

### 3.5 版本历史模式

点击版本 badge `v1.0 ↗` 进入，替换 tabs 区域：
- 左侧 180px 版本列表（版本号 + 相对时间 + 变更摘要）
- 右侧 Monaco DiffEditor（side-by-side，历史代码 vs 当前代码）
- "恢复 vN" 按钮（confirm 确认后恢复）
- "← 返回编辑" 退出历史模式

---

## 4. 工具被工作流引用时的提示

若该工具已被工作流使用：保存代码时在顶部显示一条警告横幅：  
`"此工具已被 N 个工作流使用，修改后建议重新测试。"` （不阻止保存）

---

## 5. 验收标准

- [x] 点击工具 → 打开 Tool Tab（独立）或在 Chat+Tool 右侧显示
- [x] Header：名称/描述 InlineEdit + 分类 InlineSelect 下拉 + 版本 badge
- [x] 编辑元数据 → 代码 `# @` 标注同步更新
- [x] 代码 Tab：Monaco Editor 语法高亮，编辑/保存/取消
- [x] AI pending change → diff review（红绿行级 InlineDiff）+ 接受/拒绝
- [x] 参数 Tab：Python AST 准确解析参数（含泛型如 `list[int]`）
- [x] 测试 Tab：填参数 → 运行 → 结果 + 耗时 → 历史记录
- [x] 标签系统：TagBar 增删自由标签
- [x] 版本历史：版本列表 + Monaco DiffEditor + 一键恢复
- [ ] 工具被引用时保存显示警告（待 Tier 5 工作流完成）
