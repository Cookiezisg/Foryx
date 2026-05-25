# 设置重做 — 设计

> Date: 2026-05-25 · Status: 待用户审 · Scope: 前端设置 UI 重做(去 tab、key 为中心、引导页式新增、统一为一个居中 modal)。后端基本无改动(仅可能一个小的"搜索默认"补充,见 §11)。

---

## 1. 背景与问题

当前设置散在两处且有真问题:

- **快捷 popover**(侧栏齿轮锚定,左下角):账号切换 + 主题/色调/密度/语言 + "完整设置"入口。
- **ConfigPane 完整设置**(主区,5 个 tab):API Keys / Model / Sandbox / 外观 / 数据。

问题:

1. **过度 tab 化** —— `Sandbox` 和 `数据` 两个 tab 各自只有一张只读信息卡(无任何可调项),却各占一个 tab。
2. **重复** —— popover 的 主题/色调/密度/语言 与 ConfigPane「外观」tab 是同一组设置的两份 UI,两处维护。
3. **加 Key 的 UI 太"表单"** —— `<select>` 下拉 + 文本框的轻量抽屉,和项目其它地方(引导页色块网格)的质感脱节。

参考标杆:**侧边栏**(目前唯一迭代成功的组件)的视觉语言 + **引导页**(刚落地)的 provider 网格 / 验证流。

## 2. 目标

把设置收敛成**一个居中 modal**,内部:**API Key 为中心**的凭证管理 + 引导页式的新增体验 + 自动适配的外观,全程对齐侧边栏 + 引导页的质感。删掉过度 tab 与重复 UI。

---

## 3. 总体决策

- **摆放:居中 modal(P2)**。侧栏齿轮触发,暗背景(`--bg-overlay`),`Esc` / 点遮罩关闭。理由:Keys 表 / provider 网格需要横向空间,居中天然平衡(不像角落 popover 那样"悬空大块")。
- **统一为一个设置面** —— **删除 `SettingsPopover`**(其账号切换 + 主题/色调/密度/语言 全部并入本 modal)+ **删除 `ConfigPane` 的 tab 结构**。齿轮直接开这个 modal。一处事实源,消灭重复。
- 新组件:`SettingsModal.jsx`(壳 + 手风琴);各 section 拆成独立组件(见 §5-§8)。

## 4. 结构 + 手风琴

modal 自上而下:

| 区 | 类型 | 说明 |
|---|---|---|
| **账号** | 常驻(不折叠)| 头像 + 名 + "切换/新建"(吸收原 popover 账号区)|
| **API Keys** | 折叠 section | key 为中心(§5)|
| **网络搜索** | 折叠 section(标「可选」)| 同 key 模式,无模型(§6)|
| **外观** | 折叠 section | 主题/色调/密度/语言/推理(§7)|
| **系统** | 折叠 section | 只读:数据目录 + 沙箱 + 版本(Sandbox+数据 折进此,§8)|

**手风琴互斥(铁律)**:同一时刻**最多展开一个 section**(点开一个 → 其它收起)。section 内的 **key 行也互斥**(一次只展开一个 key 的详情)。

## 5. API Keys section(key 为中心)

**这是凭证管理,不是模型管理** —— 一个 key 列表,模型/用途挂在每个 key 上。

- **key 行(折叠态)**:厂商色块 + 厂商名 + 掩码 key + 已选模型(mono 小字)+ `对话默认` 徽章(当前在用的那把)+ 验证状态徽章。在用的 key 有 accent 边框。
- **key 行(展开)**:就地下拉出详情区:
  - **模型** —— 该 key 用哪个 model(下拉,值来自该 key 的 `modelsFound`)。
  - **用途** —— 分段 `对话默认` / `仅备用`。设某 key 为「对话默认」= **跨 key 单选**(其它自动变「仅备用」,徽章随之移动)。这就是"哪把 key 是当前对话在用的"。
  - **重新验证** · **删除**。
- **新增**:列表底部一个 **「+ API Key」加号**(平时只是这个加号)。点开 → **就地撑高,展开一个引导页式的双列 provider 网格**(色块卡片,已存 key 的右上角打 ✓)+ 选中厂商后出现 **Key 输入 + "验证并获取模型" + 模型下拉** → 取消/保存。**不用 `<select>` 选厂商**(用网格)。

「对话默认」落到后端 = `PUT /model-configs/chat { provider, modelId }`(已支持)。

## 6. 网络搜索 section(可选)

同 §5 的 key 模式,差别:

- key 上挂的「用途」是 `搜索默认` / `仅备用`(无模型选择)。
- 新增 → 引导页式双列**搜索 provider 网格**(博查 / Brave / Serper / Tavily)+ Key + 验证。
- 标「可选」;不配也能用(agent 用搜索时才需要)。

## 7. 外观 section

行式:label(左)+ 控件(右):

- **主题** —— 分段 `浅色 / 深色 / 跟随系统`
- **主题色** —— 5 色块(claude 橙默认),选中橙圈,实时变色(沿用引导页机制)
- **密度** —— 分段 `紧凑 / 适中 / 舒展`
- **语言** —— 分段 `中文 / English`
- **推理过程** —— 分段 `默认折叠 / 默认展开`

全部直写 `settings`(`applyTheme` 经 App effect 实时生效)。

## 8. 系统 section(只读)

- **数据目录** `~/.forgify/` + "本地 · 不上传 · 无需登录"
- **沙箱运行时** `mise` `内置` 徽章 + "python/node 按需"
- **版本** `Forgify v1.2`

(原 `Sandbox` / `数据` 两个 tab 折进这一节。)

## 9. 风格规约(对齐侧边栏 + 引导页)

- **圆角**:modal `--radius-xl`(16);section 卡 / key 卡 `--radius-lg`(14);按钮 / 徽章 `--radius-pill`(999) —— pill 跟侧边栏 nav 一致。
- **hover / 选中**:只用 `--bg-hover` / `--bg-active` 的极轻底色,**无重边框**(侧边栏同款通透感)。
- **分段控件(主题/密度/语言/用途)**:iOS 式(轨道 `--bg-elev` + 抬起选中 `--bg-paper` + `--shadow-sm`)—— 沿用引导页 `onb-seg`。
- **provider 网格 / Key 输入 / 模型下拉**:直接复用引导页样式(`onb-grid` / `onb-prov` / `onb-kinput` / `onb-mselect`)。
- **动画**:section / key 展开收起用 `--t-fast`/`--t-med` 过渡(高度 + chevron 旋转);modal 入场用现有 `scaleIn`。**禁** `<select>` 选厂商、禁裸表单。
- **accent**:claude 橙默认;主题色实时。

## 10. 组件复用(DRY)

引导页的 provider 网格 / Key 验证 / 模型下拉目前内联在 `Onboarding.jsx`。本次**抽成共享组件**(如 `components/config/ProviderGrid.jsx` / `KeyVerifyField.jsx` / `ModelSelect.jsx`),供**引导页 + 设置 modal 共用**,避免两份验证逻辑。引导页改为消费这些共享组件(行为不变)。

## 11. 后端交互

- **对话模型**:`GET/PUT /model-configs/{scenario}`(scenario=`chat`)+ `/api-keys`(create/`:test`/delete)+ `/providers`(category=llm)—— 全已存在。模型当前只做 `chat`;`web_summary` 留后续(未配时后端自动 fallback 到 chat,见 model app `PickForWebSummary`)。
- **搜索**:`/api-keys`(category=search,provider ∈ bocha/brave/serper/tavily)+ `/providers`(category=search)—— 已存在。
- **"搜索默认"选择**:当前后端 WebSearch 是"有哪个 search key 就用哪个"(无显式 active-search 选择)。所以 `搜索默认/仅备用` 若要真生效于"多个搜索 key 选一个",需要一个**小的后端补充**(active-search-provider 设置)。**本次范围**:先做搜索 key 的增删管理 + UI 上的 `搜索默认` 标记;active 选择若后端暂不支持,标记为视觉/占位,真正生效作为小跟进(writing-plans 时确认)。

## 12. 触发 + 旧代码移除

- 侧栏齿轮 `onClick` → 开 `SettingsModal`(不再开 popover)。
- **删除** `SettingsPopover.jsx`(+ 其 test)与对应的 `ui` store 状态(`settingsPopOpen` 等)。
- **删除/重构** `ConfigPane.jsx` 的 tab 结构;其各 Tab 的有用逻辑(ApiKeysTab / ModelsTab / AppearanceTab 的 hook 用法)迁入新 section 组件。`openPane("config")` 的入口若仍被引用 → 改为开 modal 或移除。

## 13. 不做 / Out of scope

- `web_summary` 模型单独配置(后续;现在 fallback 到 chat)。
- 同一 provider 多把 key(现状一 provider 一 key 足够)。
- 多搜索 key 的真·active 选择(若需后端改动则跟进,见 §11)。
- 后端模型 scenario 扩展。

## 14. 测试

- 组件单测:手风琴互斥(开 B 关 A);key 行互斥;`对话默认` 跨 key 单选;新增面板 验证成功→出模型下拉 / 验证失败→红框提示(沿用引导页测试模式)。
- 复用组件(ProviderGrid / KeyVerifyField / ModelSelect)单测;引导页回归不破。
- 浏览器走查:齿轮开 modal、四个 section 互斥、加 key 全流程、外观实时、暗色、`Esc`/遮罩关闭。
- `npm run build` + vitest 全绿。

## 15. 文档同步(F1 / §S14)

- `frontend-prd.md`:设置章节重写(modal + key 为中心 + 引导页式新增 + 手风琴);§16 记录(去 tab / 去重复 / 抽共享组件);§17 endpoint 核对。
- `DESIGN.md`:若"居中 modal + 手风琴 + pill/段控"成为新约定 → §11/§12 补。
- `progress-record.md`:dev log。

## 16. 待你拍板(spec review gate)

1. **删 `SettingsPopover`、齿轮直接开 modal**(一处事实源)—— 同意?还是想保留快捷 popover(快速切主题)?
2. **抽共享组件**(ProviderGrid / KeyVerifyField / ModelSelect)给引导页 + 设置共用 —— 同意这个 DRY 落点?
3. **搜索"默认"选择**:active-search 后端暂无,本次先做"key 增删 + 标记",真 active 选择按需小跟进 —— 可接受?
