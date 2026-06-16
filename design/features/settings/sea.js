/* Foryx demo — 设置海洋海面：薄组合（样板）。仅渲染「当前类目详情」白海——类目导航在侧栏接管（features/settings/rail.js，镜像 notifications）。
   类目选择来自侧栏 rail → Intent.select({kind:'settingsCat'}) → 本海面 Intent.on('settingsCat') 切详情（settingsCat 无 owner，Intent 直接广播）。
   几乎不画控件像素：Segmented(主题/语言/开关) + Dropdown(模型选择) + KV(工作区/密钥元信息) + StatusDot(密钥/连接器/运行时徽) + ThinTable(工作区/运行时列)。
   概览的「最常用实体」行可点 → Intent.select({kind,id}) 跳实体海洋（一个前门，零跨海洋 import）。
   依赖 mock/models.js。注册 Shell.registerOcean('settings')。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const M = () => window.MOCK_MODELS || {};

  // ——— 薄布局脚手架（set- 专属；控件槽由组件填）———
  // 配置分区：uppercase 小标题 + 白岛；岛内每行 = 左 label/hint + 右控件槽
  const sec = (label) => {
    const s = tag('div.set-sec');
    if (label) s.appendChild(tag('div.set-seclab', label));
    const isl = tag('div.set-island'); s.appendChild(isl);
    return { el: s, island: isl };
  };
  // 行：左 k(+hint) 右控件容器（返回 .c 供 mount 组件）
  function row(island, k, hint) {
    const r = tag('div.set-row');
    r.appendChild(tag('div.set-rl', `<div class="set-rk">${k || ''}</div>${hint ? `<div class="set-rh">${hint}</div>` : ''}`));
    const c = tag('div.set-rc'); r.appendChild(c);
    island.appendChild(r);
    return c;
  }
  const htitle = t => tag('div.set-htitle', t);
  // 朴素小按钮（settings 局部动作钮，非组件——纯展示）
  const btn = (label, cls) => tag('button.set-btn' + (cls ? '.' + cls : ''), { type: 'button' }, label);
  const icbtn = title => tag('button.set-icbtn', { type: 'button', title }, icon('trash', 15));

  // ——— 各类目渲染（imperative：组件 mount 进控件槽）———
  const RENDER = {
    overview(d) {
      const m = M(), ws = m.workspace || {};
      // 身份 hero
      const hero = tag('div.set-hero',
        `<div class="set-av">${ws.initial || 'P'}</div>` +
        `<div class="set-nm">${ws.name || ''}</div>` +
        `<div class="set-sub">本地工作区 · 自 ${ws.createdAt || ''} · 活跃 ${ws.activeDays || 0} 天</div>`);
      d.appendChild(hero);

      // 统计岛
      const stats = tag('div.set-stats');
      (m.stats || []).forEach(s => stats.appendChild(tag('div.set-stat', `<div class="num">${s.num}</div><div class="lab">${s.label}</div>`)));
      d.appendChild(stats);

      // 活动热力图
      const actv = tag('div.set-actv',
        `<div class="set-actvhd"><span class="t">活动</span></div>` +
        `<div class="set-grid"></div><div class="set-months"></div>`);
      d.appendChild(actv);
      Segmented.mount(actv.querySelector('.set-actvhd'), ['每日', '每周', '累计'], { value: '每日' });
      heatmap(actv.querySelector('.set-grid'), actv.querySelector('.set-months'));

      // 双栏：活动洞察(KV) + 最常用实体(RefPill 可点)
      const cols = tag('div.set-cols');
      const left = tag('div'); left.appendChild(tag('h3', '活动洞察'));
      KV.defs(left, (m.insights || []).map(([k, v]) => [k, v]));
      const right = tag('div'); right.appendChild(tag('h3', '最常用实体'));
      const ents = tag('div.set-ents'); right.appendChild(ents);
      (m.topEntities || []).forEach(e => {
        const r = tag('div.set-erow',
          `<span class="set-ei">${icon(((window.ENTITY_KINDS || {})[e.kind] || {}).icon || e.kind, 16)}</span>` +
          `<span class="nm">${e.name}<em>${e.kind}</em></span><span class="ct">${e.count} 次</span>`);
        r.onclick = () => Intent.select({ kind: 'entity', id: e.ref, source: 'settings' });
        ents.appendChild(r);
      });
      cols.appendChild(left); cols.appendChild(right);
      d.appendChild(cols);
    },

    general(d) {
      d.appendChild(htitle('通用'));
      const a = sec('外观与语言'); d.appendChild(a.el);
      Segmented.mount(row(a.island, '主题'), ['亮', '暗', '跟随系统'], { value: '亮', onPick: applyTheme });
      Segmented.mount(row(a.island, '界面与回复语言'), ['中文', 'English'], { value: '中文' });
      const w = sec('网页抓取'); d.appendChild(w.el);
      Segmented.mount(row(w.island, 'WebFetch 抓取方式', 'Jina 模式会把 URL 发往第三方公共 reader（提取更好但出本机）'), ['本地直取', 'Jina Reader'], { value: '本地直取' });
    },

    models(d) {
      const m = M();
      d.appendChild(htitle('模型与密钥'));
      const dm = sec('默认模型'); d.appendChild(dm.el);
      const dim = m.defaultModels || {}, dp = m.defaultPick || {};
      Dropdown.mount(row(dm.island, '对话'), { options: dim.chat || [], value: dp.chat });
      Dropdown.mount(row(dm.island, '工具 / Utility'), { options: dim.utility || [], value: dp.utility });
      Dropdown.mount(row(dm.island, 'Agent'), { options: dim.agent || [], value: dp.agent });

      const ks = sec('API 密钥（本地保险箱 · AES-GCM）'); d.appendChild(ks.el);
      (m.apiKeys || []).forEach(k => {
        const c = row(ks.island, k.provider);
        c.insertAdjacentHTML('beforeend', StatusDot.badge('CONN', k.status));
        if (k.status !== 'ready') c.appendChild(btn('测试'));
        c.appendChild(icbtn('删除'));
      });
      row(ks.island, '').appendChild(addBtn('添加密钥'));
    },

    search(d) {
      const m = M(), e = m.embedding || {};
      d.appendChild(htitle('搜索与嵌入'));
      const eng = sec('嵌入引擎'); d.appendChild(eng.el);
      Segmented.mount(row(eng.island, '引擎'), ['内置', 'Ollama', '关闭'], { value: ['内置', 'Ollama', '关闭'][e.engine || 0] });
      const stc = row(eng.island, '引擎状态');
      stc.insertAdjacentHTML('beforeend', StatusDot.badge('ENV', e.status || 'ready'));
      stc.insertAdjacentHTML('beforeend', `<span class="set-meta">${e.model || ''}</span>`);

      const ol = sec('Ollama（选 Ollama 时）'); d.appendChild(ol.el);
      row(ol.island, '地址').appendChild(input(e.ollamaAddr || '', true));
      row(ol.island, '模型').appendChild(input(e.ollamaModel || '', true));

      const ix = sec('索引'); d.appendChild(ix.el);
      row(ix.island, '重建全部索引', '将重建全部文档/实体索引，期间检索短暂不可用').appendChild(btn('重建'));
    },

    mcp(d) {
      const m = M();
      d.appendChild(htitle('连接器 · MCP'));
      const s = sec('MCP Servers'); d.appendChild(s.el);
      (m.connectors || []).forEach(c => {
        const slot = row(s.island, c.name);
        slot.insertAdjacentHTML('beforeend', StatusDot.badge('CONN', c.status));
        slot.appendChild(btn('重连', 'ghost'));
        slot.appendChild(icbtn('删除'));
      });
      row(s.island, '').appendChild(addBtn('添加 Server'));
    },

    runtimes(d) {
      const m = M();
      d.appendChild(htitle('运行时与磁盘'));
      const rt = sec('沙箱运行时（按需下载 · 钉死版本）'); d.appendChild(rt.el);
      (m.runtimes || []).forEach(r => {
        const slot = row(rt.island, `<span class="set-chip">${r.chip}</span>${r.name}`);
        slot.insertAdjacentHTML('beforeend', StatusDot.badge('ENV', r.status));
        slot.appendChild(r.status === 'ready' ? btn('删除', 'ghost') : btn('安装'));
      });
      const dk = sec('磁盘'); d.appendChild(dk.el);
      const slot = row(dk.island, '沙箱占用');
      slot.insertAdjacentHTML('beforeend', `<span class="set-meta">${m.diskUsage || ''}</span>`);
      slot.appendChild(btn('清理'));
    },

    workspace(d) {
      const m = M(), ws = m.workspace || {};
      d.appendChild(htitle('工作区'));
      const cur = sec('当前工作区'); d.appendChild(cur.el);
      row(cur.island, '名称').appendChild(input(ws.name || ''));
      row(cur.island, '数据目录').insertAdjacentHTML('beforeend', `<span class="set-meta mono">${ws.dataDir || ''}</span>`);

      const all = sec('全部工作区'); d.appendChild(all.el);
      const tbl = tag('div.set-tblslot'); all.island.appendChild(tbl);
      ThinTable.table(tbl, ['工作区', '状态'], (m.workspaces || []).map(w => [
        w.name,
        w.current ? `<span class="set-cur">${StatusDot.dot('done')}当前</span>` : `<button class="set-btn ghost set-tswitch">切换</button>`,
      ]));
      row(all.island, '').appendChild(addBtn('新建工作区'));

      // 危险区
      const dz = tag('div.set-danger',
        `<div class="dl">删除工作区</div>` +
        `<div class="dd">将级联永久删除该工作区的全部对话、实体、调度与本地文件，无法恢复。请输入工作区名以确认。</div>` +
        `<div class="set-dangerrow"></div>`);
      d.appendChild(dz);
      const dr = dz.querySelector('.set-dangerrow');
      const confirm = input(`输入 ${ws.name || ''} 确认`, false, true);
      dr.appendChild(confirm);
      const del = btn('删除', 'danger'); del.disabled = true; dr.appendChild(del);
      confirm.addEventListener('input', () => { del.disabled = confirm.value.trim() !== (ws.name || ''); });
    },

    notif(d) {
      const m = M(), n = m.notif || {};
      d.appendChild(htitle('通知'));
      const cc = sec('并发'); d.appendChild(cc.el);
      row(cc.island, '活动运行上限', '同时进行的工作流运行数上限').appendChild(input(String(n.concurrency || 4), false, false, 'set-num'));
      const nt = sec('通知'); d.appendChild(nt.el);
      const onoff = ['开', '关'];
      Segmented.mount(row(nt.island, '运行完成'), onoff, { value: onoff[n.runComplete || 0] });
      Segmented.mount(row(nt.island, '待审批'), onoff, { value: onoff[n.needsApproval || 0] });
      Segmented.mount(row(nt.island, '实体变更'), onoff, { value: onoff[n.entityChange || 0] });
    },

    about(d) {
      const m = M();
      d.appendChild(htitle('关于'));
      const s = sec(''); d.appendChild(s.el);
      row(s.island, '版本').insertAdjacentHTML('beforeend', `<span class="set-meta">${m.version || ''}</span>`);
      row(s.island, '数据目录').appendChild(btn('打开目录', 'ghost'));
      row(s.island, '隐私').insertAdjacentHTML('beforeend', `<span class="set-meta">只存本地 SQLite · 绝不外传</span>`);
    },
  };

  // ——— 小工厂（settings 局部展示控件，非组件库）———
  function addBtn(label) {
    const b = tag('button.set-btn.ghost', { type: 'button' }, `${icon('plus', 14)} ${label}`);
    return b;
  }
  function input(value, mono, full, extra) {
    const i = tag('input.set-in' + (mono ? '.mono' : '') + (full ? '.full' : '') + (extra ? '.' + extra : ''));
    i.value = value; return i;
  }
  function applyTheme(v) {
    if (v === '亮') document.documentElement.dataset.theme = 'light';
    else if (v === '暗') document.documentElement.dataset.theme = 'dark';
    else document.documentElement.removeAttribute('data-theme');
  }

  // 概览热力图：40 列 × 7 行；accent 浓度 5 档（token color-mix，零裸 hex），空格走 island-3
  function heatmap(grid, months) {
    const lvl = n => n === 0 ? 'var(--island-3)' : `color-mix(in srgb, var(--accent) ${[0, 18, 34, 54, 76][n]}%, transparent)`;
    for (let w = 0; w < 40; w++) {
      const col = tag('div.set-hcol');
      for (let dd = 0; dd < 7; dd++) {
        const c = tag('div.set-hc');
        const r = Math.random(), wknd = (dd === 0 || dd === 6);
        let l = r > (wknd ? 0.62 : 0.34) ? 1 + Math.floor(Math.random() * 4) : 0;
        if (w < 4 && Math.random() > 0.45) l = 0;
        c.style.background = lvl(l); col.appendChild(c);
      }
      grid.appendChild(col);
    }
    ['7月', '8月', '9月', '10月', '11月', '12月', '1月', '2月', '3月', '4月', '5月', '6月']
      .forEach(mo => months.appendChild(tag('span', mo)));
  }

  // ——— 当前类目详情渲染（detail = 唯一白海；类目导航在侧栏 rail）———
  let detail;
  const show = id => {
    if (!detail) return;   // 海面未挂时（rail 先于海面 build 触发）忽略，build 会补渲染默认类目
    detail.classList.toggle('center', id === 'overview');
    detail.innerHTML = '';
    (RENDER[id] || RENDER.overview)(detail);
    detail.scrollTop = 0;
  };

  // ——— 注册海洋（仅详情；类目列在侧栏接管，见 rail.js）———
  Shell.registerOcean('settings', {
    crumb: '设置',
    build(sea) {
      const root = tag('div.set-root');
      detail = tag('section.set-detail');
      root.appendChild(detail);
      sea.appendChild(root);
      show('overview');   // 默认概览（侧栏 rail 默认也高亮概览，二者同步）
    },
  });

  // 类目选择来自侧栏 rail（settingsCat 无 owner → Intent 直接广播）→ 切详情
  Intent.on('settingsCat', sel => show(sel.id));
})();
