# Anselm `demo/` — 设计系统 + 完整能力 demo（Flutter 1:1 事实源）

> 这是 Anselm 桌面端的**严格约束设计系统**与**完整产品能力 demo**：纯静态、不连后端、不进后端门禁。
> 目标 = 一套丑布局根本表达不出来的规范 + 一个展示产品所有能力的可跑 demo，供后续 Flutter 桌面端**逐组件 1:1 参照**。
> `design/` 是上一版（迁移负债：双组件系统/带 bug/覆盖错位），**只作参照、不再开发**；本目录是干净重建。

---

## 一句话心法

**一致性 = 约束的副产品。** 规范的力量在“拒绝什么”，不在“能表达什么”。
照规范做（拼 `<an-*>` 原语）比不照做（手写 CSS）更省事 —— 这才是真强制。

---

## 三层强制（约束怎么真正咬合）

| 层 | 机制 | 治什么 | 落点 |
|---|---|---|---|
| **L1 机械门禁** | `node tools/lint.mjs` 扫描：禁裸 `hex/px/ms`、禁在 tokens 外定义 `--变量`、强制 `an-` 标签前缀 | 裸值、乱定义、命名漂 | [`tools/lint.mjs`](tools/lint.mjs) |
| **L2 结构封装** | 原语 = 原生 **Web Component + Shadow DOM**。feature 只能写 `<an-*>` 标签，**够不到内部、改不了样式** | “各自造轮子” 在结构上不可能 | [`core/base.js`](core/base.js) + `core/primitives/` |
| **L3 声明式 + 覆盖闸** | 页面由 schema 声明驱动（手搓布局表达不出来）+ [`CAPABILITY.md`](CAPABILITY.md) 把“覆盖所有能力”变成可勾选清单 | 实现不一致、覆盖错位/不贴后端 | `core/schema/{kind-schema,render}.js` + `core/config/*` + `CAPABILITY.md` |

> token 是 `:root` 上的自定义属性，**穿透 shadow 边界**继续生效 —— 所以组件内 `var(--…)` 照常工作、换肤零成本。

---

## 目录 = 归属（一文件一主人）

```
demo/
├── README.md            # 本文（宪法）
├── CAPABILITY.md        # 能力清单 = 后端契约的投影（覆盖闸：做哪些面）
├── PATTERNS.md          # 原语/Pattern 覆盖登记（杜绝造轮子：每个 UI 范式的归宿）
├── reference.html       # 能力参考画廊（独立页·单独管理）：声明式 catalog 驱动，每原语一展位、各态一格；与 app 同源加载全部原语，便于拓展/debug
├── app.html             # 完整产品 app 宿主（三岛骨架可跑；海洋内容 Phase 3 铺）
├── index.html           # 入口画廊（规划）
├── core/
│   ├── tokens.css       # 🔒 唯一值源（密度 2 幂 · 布局 2:3:6 · 字阶模数）
│   ├── reset.css        # 🔒 light-dom 基座（与 tokens 是门禁唯二豁免层）
│   ├── base.js          # AnElement 基类（Shadow DOM + token + 生命周期）
│   ├── icons.js         # 图标语法：领域 key → Lucide 名（单一映射）
│   ├── vendor/          # 🔒 第三方 vendored（lucide.js 1715 图标 + LICENSE，门禁豁免）
│   ├── primitives/      # 强制层原语（47 个 <an-*> custom element，CSS 内联在 static css；floating/menu/model-picker/mention/toast/dialog 6 个命令式浮层/交互模块）
│   ├── config/          # 单源枚举：entity-kinds（9 kind）· state-model（状态翻译）
│   ├── schema/          # 声明式：kind-schema（KIND_SCHEMA）· render（renderEntity 渲染器，L3 闭环）
│   ├── patterns/        # 复合件（EntityCard/RunGraph 等，只由 primitives 拼，Phase 3）
│   ├── manifest.js      # 海洋注册表（append-only）
│   ├── intent.js · live.js   # 选中通道 / 三流实时（mock→真 SseGateway 同契约）
│   ├── shell.js         # <an-shell> 三岛外壳（布局/收起/拖拽/滚动）
│   ├── sidebar.js       # <an-sidebar> 左岛（导航/轴/peek）
│   └── app.js           # 装配根/控制器（manifest→nav，切海洋，注入 Intent）
├── features/<ocean>/    # 海洋（一文件夹一主人，只装配、零 bespoke CSS，Phase 3）
└── tools/lint.mjs · serve.mjs   # L1 门禁脚本 · no-cache 预览服务
```

---

## 原语契约（写一个 `<an-*>` 怎么写）

```js
class AnXxx extends window.AnElement {
  static tag = "an-xxx";              // 必须 an- 前缀
  static observed = ["a", "b"];       // 反射的属性（变更即重渲）
  static css = `...`;                 // 内联 CSS（shadow 内类名无需前缀——已隔离）
  render() { return `...`; }           // 返回 shadow innerHTML
  hydrate() { /* 可选：绑事件，emit('an-xxx-event') */ }
}
window.AnElement.define(AnXxx);
```

- 取值：`this.attr(n,d)` / `this.has(n)` / `this.num(n,d)`；查 shadow：`this.$(s)` / `this.$$(s)`。
- 对外事件：`this.emit('an-select', {...})`（`composed:true`，feature `addEventListener` 接）。
- 转义：`window.anEsc(s)`（单一实现）；图标：`window.icon(name, size?, stroke?)`。
- **跨海洋通信只走 Intent / Live**（Phase 1b），原语之间不互相 import。

---

## 数系（值的数学出处，详见 `core/tokens.css` 注释）

- **密度 = 纯 2 的幂**：grid 4 · gap 8 · icon/lead 16 · row 32（2²·2³·2⁴·2⁵）。
- **布局 = 谐波 2:3:6**（u=120）：侧栏 240 · 右岛 360 · 内容 720；1440 窗 = 12u 平铺。
- **字阶 = 模数**：display 16/20/24/32 落 4 网格；body 13 是锚（不入 2 幂）；meta 12 是 UI 下限。
- 层次靠**字色**（列表项默认 `--ink-2` 灰、hover/选中才 `--ink` 黑；meta `--ink-3`），不靠字号；**accent 极度克制**（仅主 CTA / 实时 / 选中点）。
- hover 显隐 / 同槽互换 = **0ms 即时**（不入 transition：更脆 + 规避无头渲染器“未完成过渡冻初值”坑）。

---

## 运行 / 门禁

```bash
node demo/tools/lint.mjs                 # L1 门禁（CI/pre-commit，非零退出=失败）
# 预览：preview_start name=demo → /app.html（完整三岛骨架）· /reference.html（原语规格台）
```
**无头渲染器两坑**：① 默认态须正确渲染的属性禁进 transition；② 量布局前先 `preview_resize 1440 900`（视口偶尔塌 1px）。

---

## 工程纪律（本工作区约定）

- 中文回复；代码/路径/英文 commit 半句保持原样。commit **不加** `Co-Authored-By`。
- 每次 commit 后 **push origin/main**；**只在 main、不开分支**；精确 `git add demo/`（别 `git add -A`）。
- `design/` 是上一版参照、不动；`demo/` 是本工作区。
- 改 token 一处、全系统跟着变；改原语 = 改 `core/primitives/<x>.js`（CSS 内联其中）。
