/* Forgify demo — 功能注册表（唯一可被功能作者编辑的共享文件，且只 append 一行）。
   app.html / index.html / 四导航 / 画廊全部由它生成——加/改一个海洋只动这里一行 + 自己的文件夹，永不碰别人。
   字段：id · label · icon(icons.js key) · nav(1=进四导航) · default(首屏) · axis(avatar|bell 非导航轴) ·
        owns[](Intent 选中 kind 归属) · sea/rail/css(模块入口，相对 demo/ 根) · standalone(独立整页) · gallery/desc(画廊卡)。
   规则：append-only；不存在的入口文件 = 该海洋懒加载时占位，不报错。 */
window.MANIFEST = [
  { id: 'chat',          label: 'Chat',       icon: 'chat',      nav: 1, default: 1, owns: ['conversation'],
    sea: 'features/chat/sea.js',       rail: 'features/chat/rail.js',       gallery: 1, desc: '主对话 + 信号交互：AI 锻造实体时右岛实时编辑。' },
  { id: 'entities',      label: 'Entities',   icon: 'entities',  nav: 1,             owns: ['entity'],
    sea: 'features/entities/sea.js',   rail: 'features/entities/rail.js',   gallery: 1, desc: '四项全能实体的家：完整展示 + 调试 + 修改。' },
  { id: 'scheduler',     label: 'Scheduler',  icon: 'scheduler', nav: 1,             owns: ['workflow', 'run', 'node'],
    sea: 'features/scheduler/sea.js',  rail: 'features/scheduler/rail.js',  gallery: 1, desc: '运维驾驶舱：Conducted Keynote 活运行图 + 历史 + 审批。' },
  { id: 'documents',     label: 'Documents',  icon: 'doc',       nav: 1,             owns: ['document'],
    sea: 'features/documents/sea.js',  rail: 'features/documents/rail.js',  gallery: 1, desc: '零-markdown 心智的 WYSIWYG 文档库。' },
  { id: 'settings',      label: 'Settings',   icon: 'gear',      nav: 0, axis: 'avatar',
    sea: 'features/settings/sea.js',                                        gallery: 1, desc: '工作区 / 模型密钥 / 连接器 / 运行时配置。' },
  { id: 'notifications', label: '通知',        icon: 'bell',      nav: 0, axis: 'bell', owns: ['notification'],
    rail: 'features/notifications/rail.js' },
  { id: 'onboarding',    label: 'Onboarding', icon: 'spark',     standalone: 'features/onboarding/onboarding.html', gallery: 1, desc: '首启配置向导：外观 / 语言 → API Key。' },
];
