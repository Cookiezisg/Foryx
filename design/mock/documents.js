/* Foryx demo — 文档示意数据层（DTO 镜像后端 references/）。绝不连后端。
   每个 document 是一篇 markdown 文档；body 为渲染产物 HTML（接后端时换单块 markdown 序列化）。
   tree 为整库 metadata（GET /documents/tree），有 children 即当文件夹；backlinks = relation 入边（wikilink 反查）。
   消费者：features/documents（rail 树导航 + sea 编辑器 + 右岛元信息）。键 = document id。 */
(function () {
  // 提及用组件 RefPill 渲染（点击经 Intent.select 派发）。
  //   · document wikilink：用 'doc' 图标键取真字形；data-kind 即 'doc'，海面/编辑器据此把点击重路由成 kind:'document'（owned）。
  //   · 实体 @提及：kind 即真 ENTITY_KIND（agent/function/…），RefPill.wire 直接派 kind:entity 语义（图标也对）。
  const docLink = (ref, label) => RefPill.html('doc', label, ref);
  const entLink = (kind, ref, label) => RefPill.html(kind, label, ref);

  // —— 渲染态正文（markdown 全类型样张；接后端时换真 content） ——
  // 注：代码块用 <pre> 裸文本，海面经 CodeEditor.highlight 着色（唯一高亮事实源）。
  const DESIGN_BODY = `
    <p>这是 Foryx 文档海洋的 <b>markdown 全类型样张</b>。内容区<b>放宽</b>了外壳禁横线——可有 <a href="#">下划线链接</a>、分隔线、表格细线，大范围参考 Notion；但正文 <b>不用灰色填充块</b>、行内代码 <code>like_this</code> <b>不学</b> Notion 的红。</p>
    <p><b>不会 markdown 也没关系：</b>空行敲 <code>/</code> 唤出命令窗挑块；选中文字浮出工具条点选格式；块左侧悬停有 <code>+</code> 和拖拽手柄。</p>

    <h2>标题层级</h2>
    <p>层级靠字号阶梯，分节靠留白、不靠编号或下划线。</p>
    <h3>这是一个三级标题</h3>
    <p>三级标题下的正文。</p>

    <h2>文字样式</h2>
    <p>支持 <b>粗体</b>、<em>斜体</em>、<del>删除线</del>、<mark>高亮</mark>、<code>行内代码</code>，以及 <a href="#">带下划线的链接</a>。行内代码是白底 + 细描边的等宽字。</p>

    <h2>列表</h2>
    <p>无序、有序、任务三种：</p>
    <ul>
      <li>无序列表用小圆点</li>
      <li>支持嵌套
        <ul><li>第二层换成空心环</li><li>靠缩进，不画连接线</li></ul>
      </li>
      <li>项与项之间留白呼吸</li>
    </ul>
    <ol>
      <li>有序列表用等宽数字</li>
      <li>序号即层级线索</li>
      <li>同样的紧凑节奏</li>
    </ol>
    <ul class="doc-task">
      <li class="done"><span class="box">${icon('check', 12)}</span><span class="t">已完成（中性近黑实底 + 白勾）</span></li>
      <li class="done"><span class="box">${icon('check', 12)}</span><span class="t">不用强调蓝——完成是事实、非「正在发生」</span></li>
      <li><span class="box"></span><span class="t">未完成只是一个细描边空框</span></li>
    </ul>

    <h2>引用</h2>
    <blockquote>引用用左侧一道细竖线 + 文字降一档灰，白底无填充。学 Notion 的经典引用，但去掉了灰块。</blockquote>

    <h2>提示块 Callout</h2>
    <div class="doc-callout"><span class="ico">${icon('spark', 16, 1.6)}</span><div class="c"><b>这是一个 Callout。</b>白底 + 一圈描边 + 左图标，强调一段话而不靠底色。</div></div>

    <h2>代码</h2>
    <p>行内是 <code>const x = 1</code>；多行代码块带语法高亮、白底、一圈描边、右上角标语言：</p>
    <div class="doc-code"><span class="lang">ts</span><pre>// 文档正文 = 单块 markdown 字符串，整篇覆盖
async function render(md) {
  const html = await parse(md)   // 无版本 diff
  return wrap(html, { theme: "light", toc: true })
}</pre></div>
    <div class="doc-code"><span class="lang">py</span><pre># 抓取竞品动态并归并去重（每日 08:00 触发）
def weekly_digest(sources, since):
    items = []
    for url in sources:
        items += fetch(url, since)
    return summarize(dedupe(items))</pre></div>

    <h2>表格</h2>
    <table class="doc-table">
      <thead><tr><th>构件</th><th>处理</th><th>底色</th></tr></thead>
      <tbody>
        <tr><td>引用</td><td>左竖线 + 灰字</td><td>白底</td></tr>
        <tr><td>代码</td><td>白底 + 描边 + 等宽 + 高亮</td><td>白底</td></tr>
        <tr><td>表格</td><td>细线分隔</td><td>白底</td></tr>
      </tbody>
    </table>

    <h2>链接与提及</h2>
    <p>外部 <a href="#">下划线链接</a>；文档间 ${docLink('d4', '另一篇文档')} wikilink；提及实体 ${entLink('agent', 'ag_research', '某个 Agent')}。</p>

    <h2>分隔线</h2>
    <p>分隔线就是一条细线：</p>
    <hr>
    <p>用来分隔大段落。</p>

    <h2>图片</h2>
    <p>图片圆角裁切、可带说明：</p>
    <div class="doc-imgph">图片占位</div>
    <div class="doc-cap">图：示意配图（mockup 占位）</div>`;

  const ROADMAP_BODY = `
    <p>2026 路线图速记。本页演示同一海洋装载不同文档——切树即换正文、右岛元信息随之刷新。</p>
    <h2>Q1 · 地基</h2>
    <ul class="doc-task">
      <li class="done"><span class="box">${icon('check', 12)}</span><span class="t">durable 引擎落地（节点结果记忆化）</span></li>
      <li class="done"><span class="box">${icon('check', 12)}</span><span class="t">前端 Flutter 地基</span></li>
      <li><span class="box"></span><span class="t">四海洋 features 铺齐</span></li>
    </ul>
    <h2>Q2 · 实体海</h2>
    <p>九类全能实体卡 + 版本 diff + 就地试运行。详见 ${docLink('d3', '文档页设计')}。</p>
    <blockquote>路线图本身也是一篇文档，可被别处 wikilink 反查。</blockquote>`;

  const API_BODY = `
    <p>后端契约速查。线缆 camelCase、物理列 snake_case；统一 Envelope。</p>
    <h2>执行动词</h2>
    <p>非 CRUD 逻辑用 <code>:action</code> 后缀：</p>
    <table class="doc-table">
      <thead><tr><th>实体</th><th>动词</th></tr></thead>
      <tbody>
        <tr><td>Function</td><td><code>:run</code></td></tr>
        <tr><td>Handler</td><td><code>:call</code></td></tr>
        <tr><td>Agent</td><td><code>:invoke</code></td></tr>
        <tr><td>Workflow</td><td><code>:trigger</code></td></tr>
      </tbody>
    </table>
    <h2>分页</h2>
    <p>所有 List 接口必须支持 <code>?cursor=...&limit=...</code>。</p>`;

  window.MOCK_DOCUMENTS = {
    // 当前打开的文档 id（侧栏默认高亮 + sea 默认装载）
    cur: 'd3',

    // 整库树（metadata only）。u = 更新新近度（示意，供「Recently edited」排序）。
    tree: [
      { id: 'd1', name: 'Product', u: 4, children: [
        { id: 'd2', name: 'Frontend', u: 6, children: [
          { id: 'd3', name: '文档页设计', u: 9 },
          { id: 'd4', name: 'Roadmap 2026', u: 7 },
        ] },
        { id: 'd5', name: '竞品列表', u: 3 },
      ] },
      { id: 'd6', name: 'Engineering', u: 8, children: [
        { id: 'd7', name: 'Backend 重构记录', u: 2 },
        { id: 'd8', name: 'API 契约', u: 8 },
      ] },
      { id: 'd9', name: '随手记', u: 5 },
    ],

    recent: [{ id: 'd3', name: '文档页设计' }, { id: 'd8', name: 'API 契约' }, { id: 'd4', name: 'Roadmap 2026' }],

    // 每篇文档：path(面包屑) · title · meta(更新/字数) · tags · body(渲染 HTML) · size · backlinks(relation 入边)。
    docs: {
      d3: {
        id: 'd3', title: '文档页设计',
        path: ['产品', '前端', '文档页设计'],
        updated: '2 小时前', words: '1.2k 字', size: '3.2 KB / 1 MB',
        tags: ['design', 'markdown'],
        backlinks: [
          { id: 'd_guide', name: '上手指南', snip: '… 排版遵循 [[文档页设计]] 的海岸线一节 …' },
          { id: 'd_spec', name: '组件规格速查', snip: '… 药丸样式对齐 [[文档页设计]] …' },
          { id: 'd_onb', name: 'Onboarding 文案', snip: '… 风格沿用 [[文档页设计]] …' },
        ],
        body: DESIGN_BODY,
      },
      d4: {
        id: 'd4', title: 'Roadmap 2026',
        path: ['产品', '前端', 'Roadmap 2026'],
        updated: '昨天', words: '420 字', size: '1.1 KB / 1 MB',
        tags: ['planning'],
        backlinks: [{ id: 'd3', name: '文档页设计', snip: '… 详见 [[Roadmap 2026]] 的 Q2 实体海 …' }],
        body: ROADMAP_BODY,
      },
      d8: {
        id: 'd8', title: 'API 契约',
        path: ['工程', 'API 契约'],
        updated: '3 天前', words: '860 字', size: '2.4 KB / 1 MB',
        tags: ['backend', 'contract'],
        backlinks: [],
        body: API_BODY,
      },
    },
  };
})();
