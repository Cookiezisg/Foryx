/* Forgify design-lab — 【Entities 海洋】的左侧栏内容（独立文件，一人负责；与外壳/别的海洋解耦）。
   外壳 sidebar.js 据四导航懒加载本文件；自注入 entities.css，经 SideBar.register('entities', render) 挂载。
   形态：New + 搜索 → 类型按钮条(All + 9 类,带计数) → 点类型展开该类型(All=按组总览)。
   行复用 Chat 状态点 .en-st(idle/run/wait/err/done),按实体重映射;skill 无状态点(文件式),只标 allowed-tools 数。
   侧栏=实体列表导航;点行→右岛开实体卡(此处示意选中高亮)。执行/运行记录归 Scheduler、memory 归 Settings。
   类名全 en- 前缀,勿与 chat(cv-/cm-)及海洋海面 CSS 撞名。 */
(function () {
  const dir = new URL('.', document.currentScript.src).href;
  if (!document.querySelector('link[data-sb="entities"]')) {
    const l = document.createElement('link');
    l.rel = 'stylesheet'; l.href = dir + 'entities.css'; l.dataset.sb = 'entities';
    document.head.appendChild(l);
  }

  // 九类型 + All（icon/label/分组）。approval 借 shield。
  const TYPES = [
    { id: 'all', label: 'All', icon: 'entities' },
    { id: 'fn', label: 'Functions', icon: 'function', g: 'Quadrinity' },
    { id: 'hd', label: 'Handlers', icon: 'handler', g: 'Quadrinity' },
    { id: 'ag', label: 'Agents', icon: 'agent', g: 'Quadrinity' },
    { id: 'wf', label: 'Workflows', icon: 'workflow', g: 'Quadrinity' },
    { id: 'trg', label: 'Triggers', icon: 'trigger', g: 'Graph parts' },
    { id: 'ctl', label: 'Controls', icon: 'control', g: 'Graph parts' },
    { id: 'apf', label: 'Approvals', icon: 'shield', g: 'Graph parts' },
    { id: 'mcp', label: 'MCP', icon: 'mcp', g: 'Connections' },
    { id: 'skill', label: 'Skills', icon: 'skill', g: 'Skills' },
  ];
  const GROUPS = [['Quadrinity', ['fn', 'hd', 'ag', 'wf']], ['Graph parts', ['trg', 'ctl', 'apf']], ['Connections', ['mcp']], ['Skills', ['skill']]];

  // 示意数据。接后端：各实体 GET list（分页）;状态点冷启动取 REST 初值、entities/notifications 流跳变。
  // st 映射:function env / handler 进程·配置 / workflow lifecycle / trigger listening / mcp 连接态 → 五态(done绿/run蓝脉冲/wait橙脉冲/err红/idle空心)
  const ENTS = [
    { ty: 'fn', name: 'process_invoice', ver: 5, st: 'done', on: true }, // env ready
    { ty: 'fn', name: 'fetch_news', ver: 2, st: 'run' },                 // env syncing
    { ty: 'fn', name: 'parse_pdf', ver: 1, st: 'err' },                  // env failed
    { ty: 'hd', name: 'slack_handler', ver: 3, st: 'done' },             // running
    { ty: 'hd', name: 'db_pool', ver: 2, st: 'wait' },                   // 缺配置(该动手)
    { ty: 'ag', name: 'research_agent', ver: 2, st: 'idle' },            // agent 无运行态
    { ty: 'ag', name: 'summarizer', ver: 4, st: 'idle' },
    { ty: 'wf', name: 'nightly_report', ver: 8, st: 'run' },             // active(监听中)
    { ty: 'wf', name: 'invoice_flow', ver: 3, st: 'wait' },              // needsAttention
    { ty: 'wf', name: 'archive_cleanup', ver: 1, st: 'idle' },           // inactive
    { ty: 'trg', name: 'cron_2am', st: 'run' },                          // listening
    { ty: 'trg', name: 'webhook_pr', st: 'idle' },                       // 无 active wf 用
    { ty: 'ctl', name: 'route_by_amount', ver: 2, st: 'idle' },
    { ty: 'apf', name: 'manager_approval', ver: 4, st: 'idle' },
    { ty: 'mcp', name: 'github_mcp', st: 'done' },                       // ready
    { ty: 'mcp', name: 'linear_mcp', st: 'wait' },                       // degraded
    { ty: 'skill', name: 'deep_research', tools: 3 },                    // 文件式,无状态
    { ty: 'skill', name: 'pdf_extract', tools: 1 },
  ];
  const TY_LABEL = Object.fromEntries(TYPES.map(t => [t.id, t.id]));

  let active = 'all';

  const row = e => {
    const dot = e.ty === 'skill' ? `<span class="en-st none"></span>` : `<span class="en-st ${e.st || 'idle'}"></span>`;
    const meta = e.ty === 'skill' ? `<span class="en-tools">⚷ ${e.tools}</span>` : (e.ver ? `<span class="en-ver">v${e.ver}</span>` : '');
    return `<div class="en${e.on ? ' on' : ''}">${dot}<span class="en-t">${e.name}</span><span class="en-ty">${TY_LABEL[e.ty]}</span>${meta}<span class="en-more">${icon('more', 16)}</span></div>`;
  };
  const count = id => id === 'all' ? ENTS.length : ENTS.filter(e => e.ty === id).length;
  const chip = t => `<button class="en-chip${active === t.id ? ' on' : ''}" data-ty="${t.id}" title="${t.label}">${icon(t.icon, 15)}<span class="en-chip-l">${t.label}</span><span class="en-chip-n">${count(t.id)}</span></button>`;

  function listHTML() {
    if (active !== 'all') return `<div class="en-flat">${ENTS.filter(e => e.ty === active).map(row).join('')}</div>`;
    return GROUPS.map(([g, tys]) => {
      const items = ENTS.filter(e => tys.includes(e.ty));
      return `<div class="ensec collapsible open">
        <button class="ensec-h tog"><span class="ensec-t">${g}</span><span class="cnt">${items.length}</span><span class="chev">${icon('chevr', 14)}</span></button>
        <div class="ensec-body">${items.map(row).join('')}</div></div>`;
    }).join('');
  }

  function wireList(host) {
    host.querySelectorAll('.tog').forEach(h => h.onclick = () => h.closest('.collapsible').classList.toggle('open'));
    host.querySelectorAll('.en').forEach(it => it.onclick = e => {
      if (e.target.closest('.en-more')) return;
      host.querySelectorAll('.en').forEach(x => x.classList.remove('on')); it.classList.add('on');
      // 接后端：点行 → 右岛开统一实体卡（详情/编辑/版本）。此处仅高亮示意。
    });
  }

  function render(host) {
    host.innerHTML = `
      <button class="en-new">${icon('plus', 18)}<span>New entity</span></button>
      <div class="en-filter">${icon('search', 16)}<input type="text" placeholder="Search entities…"></div>
      <div class="en-chips">${TYPES.map(chip).join('')}</div>
      <div class="en-body">${listHTML()}</div>`;
    // 类型按钮 → 切换展开的类型
    host.querySelectorAll('.en-chip').forEach(c => c.onclick = () => {
      active = c.dataset.ty;
      host.querySelectorAll('.en-chip').forEach(x => x.classList.toggle('on', x === c));
      host.querySelector('.en-body').innerHTML = listHTML();
      wireList(host);
    });
    wireList(host);
    // 标题快滤（前端名字过滤）
    const fin = host.querySelector('.en-filter input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      host.querySelectorAll('.en-body .en').forEach(en => { en.style.display = en.querySelector('.en-t').textContent.toLowerCase().includes(q) ? '' : 'none'; });
    };
  }

  SideBar.register('entities', render);
})();
