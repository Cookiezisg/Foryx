/* Forgify design-lab — 共享图标集（线性描边，1.6–1.7 stroke）。
   ⚠ APPEND-ONLY：新增图标只「加 key」；**永不改名 / 删除**已有 key（别的海洋在用，改了就打架）。
   用法：icon('chat', 16) → SVG 字符串。 */
window.ICONS = {
  side:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>',
  search:'<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>',
  repo:'<rect x="2" y="4" width="20" height="14" rx="2"/><path d="M2 18h20M8 22h8"/>',
  chevd:'<path d="m6 9 6 6 6-6"/>', chevr:'<path d="m9 6 6 6-6 6"/>',
  panel:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M15 4v16"/>',
  moon:'<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z"/>',
  sun:'<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  chat:'<path d="M21 11.5a8.4 8.4 0 0 1-8.5 8.5 9 9 0 0 1-4-1L3 20l1.5-5.5a8.5 8.5 0 1 1 16.5-3Z"/>',
  tasks:'<path d="M11 6h10M11 12h10M11 18h10"/><path d="m3 6 1.5 1.5L7 5M3 12l1.5 1.5L7 11M3 18l1.5 1.5L7 17"/>',
  code:'<path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/>',
  plus:'<path d="M12 5v14M5 12h14"/>',
  zap:'<path d="M13 2 3 14h9l-1 8 10-12h-9z"/>',
  dispatch:'<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.5 5.5 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.5A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.7 1.5Z"/>',
  sliders:'<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
  more:'<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>',
  sort:'<path d="M11 5h10M11 9h7M11 13h4M3 17l3 3 3-3M6 4v16"/>',
  branch:'<circle cx="6" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="9" r="3"/><path d="M18 12a9 9 0 0 1-9 9M6 9v6"/>',
  enter:'<path d="M9 10 4 15l5 5"/><path d="M20 4v7a4 4 0 0 1-4 4H4"/>',
  mic:'<rect x="9" y="2" width="6" height="12" rx="3"/><path d="M5 10a7 7 0 0 0 14 0M12 19v3"/>',
  spin:'<path d="M21 12a9 9 0 1 1-6.2-8.5"/>',
  spark:'<path d="M12 2v6M12 16v6M2 12h6M16 12h6M5 5l3.5 3.5M15.5 15.5 19 19M19 5l-3.5 3.5M8.5 15.5 5 19"/>',
  agent:'<rect x="5" y="8" width="14" height="11" rx="3"/><path d="M12 8V5M9 3h6"/><circle cx="9.5" cy="13" r="1.2"/><circle cx="14.5" cy="13" r="1.2"/>',
  close:'<path d="M18 6 6 18M6 6l12 12"/>',
  edit:'<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  play:'<path d="M8 5v14l11-7z"/>',
  // —— 四导航海洋图标 ——
  entities:'<rect x="3" y="3" width="8" height="8" rx="1.6"/><rect x="13" y="3" width="8" height="8" rx="1.6"/><rect x="3" y="13" width="8" height="8" rx="1.6"/><rect x="13" y="13" width="8" height="8" rx="1.6"/>',   // 2×2 格 = 四元
  scheduler:'<circle cx="12" cy="12" r="9"/><path d="M12 7.5V12l3 1.8"/>',                          // 钟（名/形待定，见 sidebar 注）
  doc:'<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/><path d="M9 13h6M9 17h4"/>',  // 折角页
  // —— Documents 海洋 ——
  folder:'<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  link:'<path d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1"/>',   // wikilink 药丸
  at:'<circle cx="12" cy="12" r="4"/><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8"/>',                            // @提及药丸
  tag:'<path d="M11 3H5a2 2 0 0 0-2 2v6l9 9a2 2 0 0 0 2.8 0l5.2-5.2a2 2 0 0 0 0-2.8L11 3Z"/><path d="M7.5 7.5h.01"/>',
  check:'<path d="m20 6-11 11-5-5"/>',                                                                                  // 任务勾选 + 已保存
  // —— 四实体 + 图节点（Chat 海洋 v1 append；at/check 已由 Documents 海洋加，复用）——
  function:'<path d="M8 3.5c-2 0-2 3-2 4.2 0 1.1-1 2.3-2 2.3 1 0 2 1.2 2 2.3 0 1.2 0 4.2 2 4.2M16 3.5c2 0 2 3 2 4.2 0 1.1 1 2.3 2 2.3-1 0-2 1.2-2 2.3 0 1.2 0 4.2-2 4.2"/>',  // {} 无状态代码块
  handler:'<rect x="4" y="4" width="16" height="7" rx="2"/><rect x="4" y="13" width="16" height="7" rx="2"/><path d="M7.5 7.5h.01M7.5 16.5h.01"/>',  // 叠层 = 常驻有状态
  workflow:'<circle cx="6" cy="6" r="2.4"/><circle cx="18" cy="6" r="2.4"/><circle cx="12" cy="18" r="2.4"/><path d="M8 7.4 10.6 15.8M16 7.4 13.4 15.8M8.4 6h7.2"/>',  // 节点+边 = 编排图
  trigger:'<circle cx="12" cy="12" r="2.3"/><path d="M6.6 6.6a8 8 0 0 0 0 10.8M17.4 6.6a8 8 0 0 1 0 10.8M3.8 3.8a12 12 0 0 0 0 16.4M20.2 3.8a12 12 0 0 1 0 16.4"/>',  // 辐射 = 信号源
  control:'<circle cx="5" cy="12" r="2"/><circle cx="19" cy="6" r="2"/><circle cx="19" cy="18" r="2"/><path d="M7 12h3.5l4.5-5M10.5 12l4.5 5"/>',  // 一进两出 = 路由分支
  action:'<rect x="3" y="3" width="18" height="18" rx="4.5"/><path d="m10 8.4 5.6 3.6L10 15.6z"/>',  // 框中 play = 执行节点
  shield:'<path d="M12 3 5 6v5.5c0 4 3 6.8 7 8 4-1.2 7-4 7-8V6z"/>',  // 盾 = 危险闸/审批
  stop:'<rect x="5" y="5" width="14" height="14" rx="3"/>',  // 方块 = 停止/取消
  flag:'<path d="M5 21V4M5 4h12l-2.5 4L17 12H5"/>',  // 旗 = 诚实终止（max_steps）
  // —— 侧栏底部:通知 / 设置 ——
  bell:'<path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.7 21a2 2 0 0 1-3.4 0"/>',  // 铃铛 = 通知(带未读角标)
  gear:'<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 0 1-4 0v-.1a1.6 1.6 0 0 0-1-1.5 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 0 1 0-4h.1a1.6 1.6 0 0 0 1.5-1 1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H9a1.6 1.6 0 0 0 1-1.5V3a2 2 0 0 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V9a1.6 1.6 0 0 0 1.5 1H21a2 2 0 0 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1Z"/>',  // 齿轮 = 设置
  // —— Documents 海洋 · 斜杠菜单块类型 ——
  text:'<path d="M4 6h16M4 12h16M4 18h11"/>',                                                  // 正文段落
  heading:'<path d="M6 5v14M18 5v14M6 12h12"/>',                                               // 标题 H
  list:'<path d="M9 6h11M9 12h11M9 18h11"/><circle cx="4.5" cy="6" r="1"/><circle cx="4.5" cy="12" r="1"/><circle cx="4.5" cy="18" r="1"/>',  // 无序
  listol:'<path d="M10 6h10M10 12h10M10 18h10"/><path d="M4 5h1v4M3.5 9h2"/>',                 // 有序
  quote:'<path d="M9 7H5v5h4l-1.5 5M19 7h-4v5h4l-1.5 5"/>',                                     // 引用
  table:'<rect x="3" y="5" width="18" height="14" rx="1.5"/><path d="M3 10.5h18M9 5v14"/>',    // 表格
  image:'<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.6"/><path d="m4.5 18 4.5-4 3.5 2.5L17 11l3 3.5"/>',  // 图片
  divider:'<path d="M3 12h18"/>',                                                              // 分隔线
  grip:'<circle cx="9" cy="6" r="1.1"/><circle cx="15" cy="6" r="1.1"/><circle cx="9" cy="12" r="1.1"/><circle cx="15" cy="12" r="1.1"/><circle cx="9" cy="18" r="1.1"/><circle cx="15" cy="18" r="1.1"/>',  // 拖拽手柄(6点)
  copy:'<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',  // 复制
  trash:'<path d="M4 7h16M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13"/>',  // 删除
  // —— Entities 海洋:连接器 / 技能 ——
  mcp:'<path d="M9 2v5M15 2v5"/><path d="M7 7h10v3a5 5 0 0 1-10 0z"/><path d="M12 15v3a3 3 0 0 0 3 3h2"/>',  // 插头 = MCP 外部连接器
  skill:'<rect x="5" y="3" width="14" height="18" rx="2"/><path d="M9 3v18"/><path d="M12.5 8h3.5M12.5 12h3.5"/>',  // 手册/playbook = 技能(文件式指令)
};
window.icon = (k, n = 16, w = 1.7) =>
  `<svg width="${n}" height="${n}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round">${ICONS[k] || ''}</svg>`;
