/* Anselm demo — 功能注册表（唯一可被功能作者编辑的共享文件，且只 append 一行）。
   app.html / 导航 / 画廊由它生成——加/改一个海洋只动这里一行 + 自己的 features/<id>/ 文件夹。
   字段：id · label · icon(语义 key) · nav(1=进侧栏导航) · default(首屏) · axis(avatar|bell 非导航轴) ·
        owns[](Intent 选中 kind 归属) · sea/rail(Phase 3 模块入口，相对 demo/ 根) · standalone(独立整页) · desc。
   规则：append-only；模块文件未就绪 = 该海洋懒加载时占位、不报错（Phase 3 逐个铺）。 */
window.MANIFEST = [
  { id: "chat", label: "Chat", icon: "chat", nav: 1, default: 1, owns: ["conversation"],
    sea: "features/chat/sea.js", rail: "features/chat/rail.js", desc: "主对话 + AI 锻造实体时右岛实时编辑。" },
  { id: "entities", label: "Entities", icon: "entities", nav: 1, owns: ["entity"],
    deps: ["features/entities/data.js", "features/entities/actions.js"], sea: "features/entities/sea.js", rail: "features/entities/rail.js", desc: "四项全能实体的家：完整展示 + 调试 + 修改。" },
  { id: "scheduler", label: "Scheduler", icon: "scheduler", nav: 1, owns: ["workflow", "run", "node"],
    sea: "features/scheduler/sea.js", rail: "features/scheduler/rail.js", desc: "运维驾驶舱：活运行图 + 历史 + 审批。" },
  { id: "documents", label: "Documents", icon: "doc", nav: 1, owns: ["document"],
    sea: "features/documents/sea.js", rail: "features/documents/rail.js", desc: "零-markdown 心智的 WYSIWYG 文档库。" },
  { id: "settings", label: "Settings", icon: "gear", nav: 0, axis: "avatar", owns: ["settingsCat"],
    sea: "features/settings/sea.js", rail: "features/settings/rail.js", desc: "工作区 / 模型密钥 / 连接器 / 运行时。" },
  { id: "notifications", label: "通知", icon: "bell", nav: 0, axis: "bell", owns: ["notification"],
    rail: "features/notifications/rail.js", desc: "需要你 + FYI 收件箱。" },
  { id: "onboarding", label: "Onboarding", icon: "sparkles", standalone: "features/onboarding/onboarding.html", desc: "首启配置向导。" },
  { id: "graph-editor", label: "图编辑器", icon: "workflow", nav: 0, owns: [],
    deps: ["features/entities/data.js"], sea: "features/graph-editor/sea.js", rail: "features/graph-editor/rail.js", desc: "workflow 编排图编辑器：拖拽增删改连线、自动规范化（纯编辑态，运行态归 scheduler）。" },
];
