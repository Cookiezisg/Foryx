/* Foryx demo — 组件 block-kit（chat teaching-spine 块工厂；单一事实源，收掉 oceans/chat 内联的 toolGroup/toolItem/progressBox/resultBox/reasonBlock/ledger/dock/subtree/turnEnd/compaction）。
   契约：组件 = 工厂函数 → {el,...} handle；自载同名 .css；只读令牌 + 图标 + Intent + StatusDot；fg- 前缀；不碰别的海洋。
   为何在地基：这些块是「对话教学脊」的渲染语汇（grid-rows 折叠 + shimmer 流光），多海洋（chat / 实体 triage / flowrun triage）共用一套——内联抄三份必漂移。
   API：
     BlockKit.toolGroup(host) → {el,box,status(t),open(),settle(s)}   工具调用组（运行流光 → 收敛 + 外框工具列表）
     BlockKit.toolItem(box,{name,verb,args,danger,detailHTML}) → el    组内一行工具（danger=cautious/dangerous 显徽；safe 不显）
     BlockKit.progressBox(host) → {el,add(line),done()}                run_function/call_handler 实时 stderr/yield（与 result 分两框）
     BlockKit.resultBox(host,json)                                     返回结果框（单一 JSON）
     BlockKit.reasonBlock(host,text) → el                             reasoning 块（默认折叠）
     BlockKit.ledger(host,head,rows) → el                            执行/调用台账（行点击展开 logs）
     BlockKit.dock(host) → {el,show,hide,set,collapse}               底部常驻 todo 进度坞（整表替换、只读、可折叠）
     BlockKit.subtree(host,label) → el                               subagent 子树（E3 ParentID 嵌套，左侧引导轨）
     BlockKit.turnEnd(host,{code,msg,onContinue}) → el               回合诚实终态（max_steps；非失败、给「继续」）
     BlockKit.compaction(host,text)                                  压缩标记（安静耳语、无框无线）
   任何 ref-pill / 实体反链由调用方注入 detailHTML；本组件不渲染药丸（药丸是 ref-pill 组件的事）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s == null ? '' : s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
  const ic = (k, n, w) => (window.icon ? window.icon(k, n, w) : '');
  // 块加入后滚到底（host 若在可滚容器内）——找最近的可滚祖先，温和置底
  function toBottom(node) {
    let p = node && node.parentElement;
    while (p) { const s = getComputedStyle(p).overflowY; if ((s === 'auto' || s === 'scroll') && p.scrollHeight > p.clientHeight) { p.scrollTop = p.scrollHeight; return; } p = p.parentElement; }
  }

  // —— 工具调用组：摘要行流光（run）→ settle 收敛为可折叠摘要 + 外框工具列表 ——
  function toolGroup(host) {
    const w = tag('div.fg-bk-tg');
    w.innerHTML = `<div class="fg-bk-tg-sum run"><span class="fg-bk-tk"></span><span class="fg-bk-chev" style="display:none">${ic('chevr', 14)}</span></div>
      <div class="fg-bk-tg-list"><div class="fg-bk-w"><div class="fg-bk-tg-box"></div></div></div>`;
    if (host) host.appendChild(w); toBottom(w);
    const sum = qs('.fg-bk-tg-sum', w), tk = qs('.fg-bk-tk', w), chev = qs('.fg-bk-chev', w), box = qs('.fg-bk-tg-box', w);
    sum.onclick = () => { if (box.children.length) w.classList.toggle('open'); };
    return {
      el: w, box,
      status(t) { tk.textContent = t; toBottom(w); },
      open() { w.classList.add('open'); toBottom(w); },
      // 默认折叠：只留摘要行、点击展开（chev 此刻才现身）
      settle(s) { sum.classList.remove('run'); tk.textContent = s; chev.style.display = ''; toBottom(w); },
    };
  }

  // —— 工具项（组内一行）：verb + mono 名 + 可选 danger 徽 + 展开详情 ——
  function toolItem(box, o = {}) {
    const ti = tag('div.fg-bk-ti' + (o.open ? ' open' : ''));
    const badge = (o.danger && o.danger !== 'safe') ? ` <span class="fg-bk-badge ${o.danger}">${esc(o.danger)}</span>` : '';
    const detail = o.detailHTML != null ? o.detailHTML
      : (o.args != null ? `<div class="fg-bk-tbox"><div class="fg-bk-out">${esc(o.args)}</div></div>` : '');
    ti.innerHTML = `<div class="fg-bk-ti-sum"><span class="fg-bk-v">${esc(o.verb || 'Used')}</span> <span class="fg-bk-nm">${esc(o.name)}</span>${badge}<span class="fg-bk-chev">${ic('chevr', 14)}</span></div>
      <div class="fg-bk-ti-det"><div class="fg-bk-w">${detail}</div></div>`;
    if (box) box.appendChild(ti);
    qs('.fg-bk-ti-sum', ti).onclick = () => ti.classList.toggle('open');
    return ti;
  }

  // —— progress 块：实时 stderr/yield（六型之一；run 流光 → done 静态）——
  function progressBox(host) {
    const pb = tag('div.fg-bk-progress.run');
    pb.innerHTML = `<div class="fg-bk-phead"><span>进度 · stderr</span><span class="fg-bk-lt"><span class="fg-bk-lt-dot"></span>实时</span></div><div class="fg-bk-plines"></div>`;
    if (host) host.appendChild(pb); toBottom(pb);
    const lines = qs('.fg-bk-plines', pb);
    return {
      el: pb,
      add(t) { lines.textContent += (lines.textContent ? '\n' : '') + t; toBottom(pb); },
      done() { pb.classList.remove('run'); pb.classList.add('done'); },
    };
  }

  // —— 返回结果框：单一 JSON（裸结果，区别于 progress 的中间过程）——
  function resultBox(host, json) {
    const cap = tag('div.fg-bk-res-cap'); cap.textContent = '返回结果（单一 JSON）';
    const b = tag('div.fg-bk-tbox', `<div class="fg-bk-out">${esc(json)}</div>`);
    if (host) { host.appendChild(cap); host.appendChild(b); }
    toBottom(b);
    return b;
  }

  // —— reasoning 块（默认折叠，复用 tg 的 grid-rows 收合机制）——
  function reasonBlock(host, text) {
    const r = tag('div.fg-bk-reason');
    r.innerHTML = `<div class="fg-bk-reason-sum"><span class="fg-bk-chev">${ic('chevr', 13)}</span>思考</div>
      <div class="fg-bk-reason-body"><div class="fg-bk-w"><div class="fg-bk-tbox"><div class="fg-bk-out">${esc(text)}</div></div></div></div>`;
    if (host) host.appendChild(r); qs('.fg-bk-reason-sum', r).onclick = () => r.classList.toggle('open'); toBottom(r);
    return r;
  }

  // —— 执行/调用台账：head={title,agg}；rows=[{st,name,meta,logs}]，行点击展开 logs ——
  function ledger(host, head = {}, rows = []) {
    const w = tag('div.fg-bk-ledger');
    w.innerHTML = `<div class="fg-bk-ledger-head"><span>${esc(head.title)}</span><span class="fg-bk-agg">${esc(head.agg || '')}</span></div>` +
      rows.map(r => `<div class="fg-bk-cl-row ${esc(r.st || '')}"><span class="fg-bk-cl-st"></span><span class="fg-bk-cl-name">${esc(r.name)}</span><span class="fg-bk-cl-meta">${esc(r.meta || '')}</span><span class="fg-bk-chev">${ic('chevr', 13)}</span></div><div class="fg-bk-cl-logs">${esc(r.logs || '')}</div>`).join('');
    if (host) host.appendChild(w); toBottom(w);
    qsa('.fg-bk-cl-row', w).forEach(row => row.onclick = () => row.classList.toggle('open'));
    return w;
  }

  // —— 底部常驻 todo 进度坞：整表替换、只读、点头折叠 ——
  function dock(host) {
    const d = tag('div.fg-bk-dock');
    d.innerHTML = `<div class="fg-bk-td-wrap"><div class="fg-bk-td-head"><span class="fg-bk-tt">Todo</span><span class="fg-bk-prog" data-prog></span><span class="fg-bk-cur" data-cur></span><span class="fg-bk-chev">${ic('chevr', 13)}</span></div>
      <div class="fg-bk-td-body"><div class="fg-bk-w"><div data-rows></div></div></div></div>`;
    if (host) host.appendChild(d);
    qs('.fg-bk-td-head', d).onclick = () => d.classList.toggle('collapsed');
    return {
      el: d,
      show() { d.classList.add('show'); },
      hide() { d.classList.remove('show', 'collapsed'); },
      collapse(on) { d.classList.toggle('collapsed', on !== false); },
      // items = [[text, state]]；state ∈ pending | in-progress | completed
      set(items) {
        const rows = qs('[data-rows]', d); if (!rows) return;
        rows.innerHTML = (items || []).map(([t, st]) => `<div class="fg-bk-todo-row ${esc(st)}"><span class="fg-bk-mk">${st === 'completed' ? ic('check', 13) : '<span class="fg-bk-circle"></span>'}</span><span class="fg-bk-t">${esc(t)}</span></div>`).join('');
        const done = (items || []).filter(i => i[1] === 'completed').length;
        qs('[data-prog]', d).textContent = `${done}/${(items || []).length}`;
        const cur = (items || []).find(i => i[1] === 'in-progress');
        qs('[data-cur]', d).textContent = cur ? '· ' + cur[0] : (items && done === items.length ? '· 全部完成' : '');
      },
    };
  }

  // —— subagent 子树（E3 ParentID 嵌套；左侧 1px 引导轨）——
  function subtree(host, label) {
    const s = tag('div.fg-bk-subtree');
    s.innerHTML = `<div class="fg-bk-sublabel"><span class="fg-bk-ico">${ic('dispatch', 13)}</span>${esc(label || 'Subagent')}</div>`;
    if (host) host.appendChild(s); toBottom(s);
    return s;
  }

  // —— 回合诚实终态（max_steps；非失败、给「继续」）——
  function turnEnd(host, o = {}) {
    const t = tag('div.fg-bk-turn-end');
    t.innerHTML = `<span class="fg-bk-te-ico">${ic('flag', 16)}</span><span class="fg-bk-te-msg">${o.msg || '回合到达步数上限，诚实终止——<b>未失败</b>。'}</span><span class="fg-bk-badge code">${esc(o.code || 'MAX_STEPS_REACHED')}</span><button class="fg-bk-cont">继续</button>`;
    if (host) host.appendChild(t);
    qs('.fg-bk-cont', t).onclick = () => o.onContinue && o.onContinue();
    toBottom(t);
    return t;
  }

  // —— compaction 标记（安静耳语、无框无线，守「禁分割线」）——
  function compaction(host, text) {
    const c = tag('div.fg-bk-compaction');
    c.textContent = text || '· 上文已压缩 · earlier context summarized ·';
    if (host) host.appendChild(c); toBottom(c);
    return c;
  }

  window.BlockKit = { toolGroup, toolItem, progressBox, resultBox, reasonBlock, ledger, dock, subtree, turnEnd, compaction };
})();
