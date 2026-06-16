/* Foryx demo — 对话海洋海面：薄组合（脚本解释器）。
   像素几乎全在组件库：BlockKit(教学脊块) + Flowrun(durable run 条) + ApprovalGate(chat 危险闸) + EntityCard(右岛流式锻造卡) + RefPill(实体提及)。
   sea = 对话流（user 气泡 / AI 全宽 markdown / 每回合 spark / composer）+ 右岛实体卡（forge beat 触发时从右滑入、逐字段流式填充）。
   选中通道：对话药丸 RefPill → Intent.select({kind}) 唤实体海洋右岛；本海洋拥有 conversation kind（Intent.on → 切会话脚本）。
   依赖 mock/conversations.js。注册 Shell.registerOcean('chat')。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const M = () => window.MOCK_CONVERSATIONS || {};
  const SCRIPT = id => (M().scripts || {})[id];
  const ENT = key => (M().entities || {})[key];
  // 工具友好名：原始 tool id → {动词, 名}（用户读人话，不见 create_function(…) 内部名）。缺则原样兜底。
  const TL = id => (M().toolNames || {})[id] || { name: id };
  const sleep = ms => new Promise(r => setTimeout(r, ms));

  // 锚点：当前播放 token（停止/切会话即 ++ → 旧异步循环自检退出）；当前脚本 + 标题
  let runId = 0, curScript = null, curTitle = '';
  let conv, col, dock, card;
  const alive = id => id === runId && (!window.Shell || Shell._ocean === 'chat');   // 切走对话海洋即令所有异步 demo 自退（右岛不漏到别的海洋）
  const toBottom = () => { if (conv) conv.scrollTop = conv.scrollHeight; };
  // 行内提及标记 {{kind:label}} → RefPill html（kind∈实体类型/doc）。label 缺省同 kind。
  const KMAP = { function: 'function', handler: 'handler', agent: 'agent', workflow: 'workflow', control: 'control', approval: 'approval', trigger: 'trigger', mcp: 'mcp', doc: 'doc', search: 'search' };
  function pills(s) {
    return String(s == null ? '' : s).replace(/\{\{(\w+):([^}]+)\}\}/g, (_, kind, label) =>
      RefPill.html(KMAP[kind] ? kind : 'doc', label, label));
  }

  // —— 对话原语（仅 user 气泡 / AI 回合 spark + 全宽块；其余皆组件）——
  function userMsg(html) { const m = tag('div.chat-umsg', `<div class="chat-ub">${pills(html)}</div>`); RefPill.wire(m); col.appendChild(m); toBottom(); }
  function aiTurn() { const t = tag('div.chat-turn', `<div class="chat-spark">${icon('spark', 16, 1.6)}</div><div class="chat-amsg"></div>`); col.appendChild(t); toBottom(); return qs('.chat-amsg', t); }
  function para(host) { const p = tag('p'); host.appendChild(p); toBottom(); return p; }

  // 打字机（caret + 逐字；停止即收 caret 退出）
  function typeInto(node, text, cps = 70) {
    const id = runId;
    return new Promise(res => {
      const caret = tag('span.chat-caret'); node.appendChild(caret);
      let i = 0;
      (function step() {
        if (!alive(id)) { caret.remove(); return res(); }
        caret.insertAdjacentText('beforebegin', text[i++] ?? '');
        toBottom();
        if (i > text.length) { caret.remove(); return res(); }
        setTimeout(step, 1000 / cps + Math.random() * 14);
      })();
    });
  }

  const setGen = on => { const c = qs('#chatComposer'); if (c) c.classList.toggle('gen', on); };
  const tdet = ti => qs('.fg-bk-ti-det > .fg-bk-w', ti);   // 取工具项详情宿主（组件结构契约）

  // —— 右岛 = 实体面板：没绑实体 → 完整列表页（点选）；绑了某实体 → EntityCard 预览 + 名字 picker（最近 + 其他级联）。
  //    浏览目录 = 全工作区实体（MOCK_ENTITIES，按类型分组，像侧边栏实体页）；forge seed 是对话锻造的实体、直接预览。
  let islandCur = null, islandH = null;   // islandCur: null(未绑→列表页) | {src:'conv'|'ws', key}
  const WS = () => window.MOCK_ENTITIES || {};                 // 全工作区实体（其他级联 + 空态列表页）
  const CONV = () => (M().entities || {});                     // 本对话的实体（对话锻造/触达，EntityCard 形状）
  // 本对话「连接的实体」= 其脚本里 forge / ents 触达的 conv key（picker 顶部快切）
  function convKeysFor(scriptId) {
    const beats = SCRIPT(scriptId) || [], conv = CONV(), set = new Set();
    const byName = nm => Object.keys(conv).find(x => conv[x].name === nm);
    beats.forEach(b => {
      if (b.type === 'ents' && b.keys) b.keys.forEach(k => { if (conv[k]) set.add(k); });
      const f = b.type === 'forge' ? b : (b.type === 'subagent' && b.forge ? b.forge : null);
      if (f && f.seed) { const k = byName(f.seed.name); if (k) set.add(k); }
    });
    return [...set];
  }
  const convKeyOf = seed => { const c = CONV(); return Object.keys(c).find(k => c[k].name === seed.name) || null; };
  // 分组（对齐实体侧边栏：Quadrinity / 图件 / 连接 / 技能 → 类型）
  const EGROUPS = [
    ['Quadrinity', [['function', 'Functions'], ['handler', 'Handlers'], ['agent', 'Agents'], ['workflow', 'Workflows']]],
    ['Graph parts', [['trigger', 'Triggers'], ['control', 'Controls'], ['approval', 'Approvals']]],
    ['Connections', [['mcp', 'MCP']]],
    ['Skills', [['skill', 'Skills']]],
  ];
  const kindIco = kind => (window.ENTITY_KINDS && ENTITY_KINDS[kind] ? ENTITY_KINDS[kind].icon : kind);
  // 工作区实体按类型分组（列表页 + 其他级联共用）
  function browseGroups() {
    const ws = WS(), byKind = {};
    Object.keys(ws).forEach(k => { (byKind[ws[k].kind] = byKind[ws[k].kind] || []).push(k); });
    return EGROUPS.map(([g, types]) => ({ group: g, types: types.map(([kind, label]) => ({ kind, label, keys: byKind[kind] || [] })).filter(t => t.keys.length) })).filter(x => x.types.length);
  }
  // MOCK_ENTITIES → EntityCard 形状适配（两套 mock 字段差异：handler/trigger/mcp/approval/workflow 重映；function/agent/control/skill 直接用）
  function wsToCard(key) {
    const e = WS()[key]; if (!e) return null;
    const a = Object.assign({ name: key, id: e.id || 'ent_' + key }, e);
    const kv = (arr, k) => { const r = (arr || []).find(x => x[0] === k); return r ? r[1] : undefined; };
    if (e.kind === 'handler') {
      a.classCode = e.code; a.runtime = e.life === 'active' ? 'ready' : 'unconfigured'; a.configState = e.cfg;
      a.methods = (e.methods || []).map(m => Array.isArray(m) ? m[0] : m);
      a.initArgs = (e.initArgs || []).map(x => Array.isArray(x) ? { name: x[0], value: x[1], sensitive: x[2] } : x);
    } else if (e.kind === 'workflow') { a.lifecycle = e.life || 'inactive'; }
    else if (e.kind === 'trigger') { a.source = kv(e.cfg, '源类型') || 'webhook'; a.config = e.cfg || []; a.listeners = (e.rel || []).flatMap(r => (r.rows || []).map(x => x.name)); }
    else if (e.kind === 'mcp') { a.transport = kv(e.cfg, '传输') || 'stdio'; a.transportCfg = e.cfg || []; a.secrets = []; }
    else if (e.kind === 'approval') { a.allowReason = kv(e.rules, 'allowReason') === 'true'; a.timeout = kv(e.rules, 'timeout') || '永不超时'; a.behavior = kv(e.rules, 'timeoutBehavior') || 'reject'; a.validity = 'ready'; }
    return a;
  }
  const islandAside = () => ((window.Shell && Shell.body) || document.body).querySelector('aside.fg-island[data-ocean-right="entity-card"]');
  const islandShown = () => { const a = islandAside(); return !!(a && a.classList.contains('show')); };

  // picker：顶部 = 本对话连接的实体（快切）；底部 = 其他（按类型浏览全工作区）
  function pickerOpts() {
    const conv = CONV();
    return {
      items: convKeysFor(curScript).map(k => ({ key: k, name: (conv[k] || {}).name || k, kind: (conv[k] || {}).kind, src: 'conv' })),
      current: islandCur && islandCur.src === 'conv' ? islandCur.key : null,
      browse: browseGroups(), onPick: (k, src) => islandTo(k, src || 'ws'),
    };
  }
  // 预览：seed 对象(forge) / conv-key(本对话实体, 直接) / ws-key(工作区, 经 wsToCard 适配)。返回 EntityCard handle。
  function islandTo(ref, src) {
    if (window.Shell && Shell._ocean !== 'chat') return;
    let seed, cur;
    if (typeof ref === 'object' && ref) { seed = ref; cur = { src: 'conv', key: convKeyOf(ref) }; }
    else if (src === 'conv') { seed = CONV()[ref]; cur = { src: 'conv', key: ref }; }
    else { seed = wsToCard(ref); cur = { src: 'ws', key: ref }; }
    if (!seed) return;
    islandCur = cur;
    card = islandH = EntityCard.mount(null, seed, { noIterate: true, picker: pickerOpts() });
    return card;
  }
  // 空态：完整列表页（分组 → 类型 → 实体行，点选 → 预览）。复用抽屉默认头。
  function islandList() {
    if (window.Shell && Shell._ocean !== 'chat') return;
    card = null; islandCur = null;
    const shell = RightIsland.create('entity-card', { title: '选择实体', icon: 'entities', width: 384 });
    if (shell.head) shell.head.style.display = '';   // EntityCard 预览时藏了默认头，列表页用回它
    shell.body.innerHTML = '';
    shell.body.appendChild(buildListPage());
    shell.show(); islandH = shell;
  }
  function buildListPage() {
    const root = tag('div.chat-elist');
    browseGroups().forEach(g => {
      root.appendChild(tag('div.chat-elist-grp', g.group));
      g.types.forEach(t => {
        root.appendChild(tag('div.chat-elist-ty', `<span class="chat-elist-tyico">${icon(kindIco(t.kind), 15)}</span><span class="chat-elist-tynm">${t.label}</span><span class="chat-elist-cnt">${t.keys.length}</span>`));
        t.keys.forEach(k => {
          const r = tag('div.chat-elist-row', { 'data-key': k }, `<span class="chat-elist-dot">${StatusDot.dot((WS()[k] || {}).status || 'idle')}</span><span class="chat-elist-nm">${k}</span>`);
          r.onclick = () => islandTo(k, 'ws');
          root.appendChild(r);
        });
      });
    });
    return root;
  }
  // 头部按钮唤起/收起：开着 → 收；有当前预览 → 重显；本对话有连接实体 → 预览第一个；都没（未绑）→ 列表页
  function toggleIsland() {
    if (window.Shell && Shell._ocean !== 'chat') return;
    if (islandShown()) { if (islandH && islandH.hide) islandH.hide(); return; }
    if (islandCur) { islandTo(islandCur.key, islandCur.src); return; }
    const ck = convKeysFor(curScript);
    if (ck.length) islandTo(ck[0], 'conv'); else islandList();
  }

  // —— 单 beat 渲染（声明式 → 组件调用）。返回 Promise（含流式/审批等待）。 ——
  async function playBeat(t, b, id) {
    if (b.type === 'compaction') { BlockKit.compaction(col, b.text); return; }

    if (b.type === 'user') { userMsg(b.html); await sleep(520); return; }

    if (b.type === 'reason') { BlockKit.reasonBlock(t, b.text); await sleep(160); return; }

    if (b.type === 'ai') { await typeInto(para(t), b.text, b.cps || 66); await sleep(150); return; }

    if (b.type === 'todo') {
      if (dock) { dock.show(); dock.set(b.rows); }
      await sleep(620); return;
    }

    if (b.type === 'ents') {
      const p = para(t);
      p.innerHTML = `${b.label}：${b.keys.map(k => { const e = ENT(k); return RefPill.html(e.kind, e.name, k); }).join(' ')} —— 点开看右岛详情。`;
      wirePills(p);
      await sleep(120); return;
    }

    if (b.type === 'turnEnd') {
      BlockKit.turnEnd(t, { code: b.code, msg: b.msg, onContinue: () => play(curScript, curTitle) });
      await sleep(120); return;
    }

    // 工具组（无流式/无审批）：摘要流光 → 工具项 → 收敛
    if (b.type === 'tool') {
      const tg = BlockKit.toolGroup(t); tg.status(b.status); await sleep(560); if (!alive(id)) return;
      (b.items || []).forEach(it => { const fl = TL(it.name); BlockKit.toolItem(tg.box, { verb: it.verb || fl.verb, name: fl.name, danger: it.danger, detailHTML: it.detail ? `<div class="fg-bk-tbox"><div class="fg-bk-out">${esc(it.detail)}</div></div>` : null }); });
      tg.settle(b.settle); await sleep(420); return;
    }

    // run：progress 块（实时 stderr）+ 独立 result 框
    if (b.type === 'run') {
      const rf = BlockKit.toolGroup(t); rf.status(b.status);
      const rfl = TL(b.toolName); const ti = BlockKit.toolItem(rf.box, { verb: rfl.verb, name: rfl.name }); rf.open();
      const det = tdet(ti); const pb = BlockKit.progressBox(det);
      for (const ln of (b.progress || [])) { await sleep(370); if (!alive(id)) return; pb.add(ln); }
      await sleep(380); if (!alive(id)) return; pb.done();
      if (b.result) BlockKit.resultBox(det, b.result);
      rf.settle(b.settle); await sleep(520); return;
    }

    // forge：锻造工具组 + 右岛 EntityCard 流式填充（THE 签名交互）
    if (b.type === 'forge') { await playForge(t, b, id); return; }

    // approval：人在环危险闸（ApprovalGate flavor:chat）→ 决议后续 progress
    if (b.type === 'approval') {
      const ch = BlockKit.toolGroup(t); ch.status(b.status); await sleep(560); if (!alive(id)) return;
      const gate = ApprovalGate.mount(t, { flavor: 'chat', title: '需要你批准', tool: b.tool, danger: b.danger, summary: b.summary, args: b.args });
      const { act } = await gate.wait(b.auto || 'approve_always', 1900); if (!alive(id)) return;
      const denied = act === 'deny';
      gate.settle(denied ? b.settleNo : b.settleOk);
      await sleep(250); if (!alive(id)) return;
      if (!denied) {
        const afl = TL(b.tool); const ti = BlockKit.toolItem(ch.box, { verb: afl.verb, name: afl.name, danger: b.danger }); ch.open();
        const pb = BlockKit.progressBox(tdet(ti));
        for (const ln of (b.progress || [])) { await sleep(350); if (!alive(id)) return; pb.add(ln); }
        pb.done(); ch.settle(b.groupSettle);
      } else { ch.settle(b.groupSettleNo); }
      await sleep(520); return;
    }

    // subagent：E3 子树（reason + 可选内嵌 forge）
    if (b.type === 'subagent') {
      const sub = BlockKit.subtree(t, b.label);
      if (b.reason) { BlockKit.reasonBlock(sub, b.reason); await sleep(420); if (!alive(id)) return; }
      if (b.forge) await playForge(sub, b.forge, id);
      await sleep(300); return;
    }

    // flowrun：durable 节点条（Flowrun.strip；逐节点 running→ok/park）
    if (b.type === 'flowrun') {
      const fr = Flowrun.strip(t, b.frid, { variant: 'flowrun' });
      let lastDecision = 'yes';
      for (const nd of b.nodes) {
        if (nd.gatedBy && lastDecision !== 'yes') continue;   // 上游 approval 驳回 → 跳过被门控的下游
        const h = fr.addNode(nd.kind, nd.id); await sleep(340); if (!alive(id)) return;
        if (nd.act === 'park') {
          lastDecision = await h.park(2200); if (!alive(id)) return;   // 内联挂 ApprovalGate(durable)
        } else {
          h.running(); await sleep(720); if (!alive(id)) return;
          if (nd.act === 'okPort') h.okPort(nd.port);
          else if (nd.act === 'fail') h.fail(nd.msg);
          else h.ok(nd.memo);
        }
      }
      fr.finish(); await sleep(500); return;
    }
  }

  // forge 子例程：锻造工具组 + EntityCard 滑入 + 流式逐字段填充（fill/reveal/setSt/setVersion）
  async function playForge(host, b, id) {
    const cf = BlockKit.toolGroup(host); cf.status(b.status); await sleep(520); if (!alive(id)) return;
    const h = islandTo(b.seed); await sleep(420); if (!alive(id)) return;   // 自动切右岛到这个实体（持久抽屉、流式填充）
    for (const s of (b.stream || [])) {
      if (!alive(id)) return;
      if (s.code != null) { await streamCode(h, s.f, s.code, id); }
      else if (s.tags) { for (const tg of s.tags) { await sleep(280); if (!alive(id)) return; appendTag(h, s.f, tg); } }
      else if (s.setSt) { h.setSt(s.setSt[0], s.setSt[1], s.setSt[2]); await sleep(300); }
      else if (s.reveal) { h.reveal(s.reveal); await sleep(280); }
      else if (s.setVersion != null) { h.setVersion(s.setVersion); await sleep(200); }
    }
    if (b.fillGraph) await streamGraph(h, b.fillGraph, id);
    await sleep(520); if (!alive(id)) return;
    h.setLive(false);   // 锻造/编辑收口 → 「已保存」
    const ffl = TL(b.toolName); BlockKit.toolItem(cf.box, { verb: ffl.verb || b.verb, name: ffl.name, detailHTML: `<div class="fg-bk-tbox"><div class="fg-bk-out">${esc(ffl.name)} · 实体版本 ops（非 git diff）</div></div>` });
    cf.settle(b.settle); await sleep(420);
  }

  // 流式代码：经公开 fill 注入一枚临时打字 <pre>，逐字落字，收口时用 CodeEditor.highlight 着色（不碰 CodeEditor 内部 textarea）
  async function streamCode(h, f, code, id) {
    h.fill(f, '<pre class="chat-code-stream"></pre>');
    const pre = qs(`[data-f="${f}"] .chat-code-stream`, h.el); if (!pre) return;
    await typeInto(pre, code, 100); if (!alive(id)) return;
    const hl = (window.CodeEditor && window.CodeEditor.highlight) ? window.CodeEditor.highlight(code, 'code') : esc(code);
    pre.innerHTML = hl;
  }
  // 往 deps/tags 字段追加一枚标签（经 fill 注入累积 html）
  function appendTag(h, f, label) {
    const fEl = qs(`[data-f="${f}"]`, h.el); if (!fEl) return;
    const slot = qs('[data-ec-slot]', fEl) || qs('.fg-ec-val', fEl) || fEl;
    const chip = tag('span.chat-tag-new', esc(label)); slot.appendChild(chip);
  }
  // workflow 图字段逐节点流入（用 mock 实体的 nodes）
  async function streamGraph(h, key, id) {
    const e = ENT(key); if (!e || !e.nodes) return;
    const fEl = qs('[data-f="graph"]', h.el); if (!fEl) return;
    const slot = qs('[data-ec-slot]', fEl) || fEl;
    slot.innerHTML = ''; const rows = tag('div.chat-ec-rows'); slot.appendChild(rows);
    for (const n of e.nodes) {
      await sleep(280); if (!alive(id)) return;
      const ic = (window.NODE_ICON || {})[n.kind] || n.kind;
      rows.appendChild(tag('div.chat-ec-row', `<span class="chat-ec-rico">${icon(ic, 15)}</span><span class="chat-ec-rid">${esc(n.id)}</span><span class="chat-ec-rref">${esc(n.ref)}</span>`));
    }
  }

  // 点药丸 → 经 Intent.select 一个前门派发到实体海洋（kind→归属海洋，id=实体 ref）；本海洋不 import 实体海洋
  function wirePills(root) { RefPill.wire(root); }

  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])); }

  // —— 播放整条脚本（teaching-spine）——
  async function play(scriptId, title) {
    const beats = SCRIPT(scriptId); if (!beats) return;
    const id = ++runId; curScript = scriptId; curTitle = title || (M().titles || {})[scriptId] || scriptId;
    setTitle(curTitle);
    col.innerHTML = ''; const _ia = islandAside(); if (_ia) _ia.remove(); card = islandH = null; islandCur = null; setGen(true); if (dock) dock.hide();   // 新会话：右岛重置为「未绑」（再唤起按连接实体决定 预览/列表）

    let turn = null;
    for (const b of beats) {
      if (!alive(id)) return;
      // user / compaction / 顶层块开新回合；AI 系块续当前回合
      if (b.type === 'user' || b.type === 'compaction') { turn = null; }
      if (['reason', 'ai', 'tool', 'run', 'forge', 'approval', 'subagent', 'flowrun', 'turnEnd', 'ents'].includes(b.type) && !turn) turn = aiTurn();
      await playBeat(turn, b, id);
    }
    if (alive(id)) setGen(false);
  }

  function setTitle(t) {
    const tt = qs('.chat-titlebar .chat-tt');
    if (tt) tt.textContent = t;
    Shell.crumb(t);
  }

  // —— 海洋注册 ——
  Shell.registerOcean('chat', {
    crumb: '对话',
    build(sea) {
      sea.innerHTML = `
        <div class="chat-conv" id="chatConv"><div class="chat-col" id="chatCol"></div></div>
        <div class="chat-dock" id="chatDock"></div>
        <div class="chat-composer" id="chatComposer">
          <div class="chat-cwrap">
            <div class="chat-mpop" id="chatMpop">
              <div class="chat-mh">提及一个实体（freeze-on-send · 发送时快照内容）</div>
              <div class="chat-mrow"><span class="chat-mico">${icon('function', 16)}</span><span class="chat-mnm">weekly_digest</span><span class="chat-mkd">Function</span></div>
              <div class="chat-mrow"><span class="chat-mico">${icon('handler', 16)}</span><span class="chat-mnm">notion_writer</span><span class="chat-mkd">Handler</span></div>
              <div class="chat-mrow"><span class="chat-mico">${icon('agent', 16)}</span><span class="chat-mnm">摘要润色</span><span class="chat-mkd">Agent</span></div>
              <div class="chat-mrow"><span class="chat-mico">${icon('workflow', 16)}</span><span class="chat-mnm">weekly_report</span><span class="chat-mkd">Workflow</span></div>
            </div>
            <div class="chat-box">
              <div class="chat-field"><input id="chatTa" placeholder="说说你想自动化什么…"><span class="chat-enter" id="chatEnter">${icon('send', 16)}</span></div>
              <div class="chat-bar">
                <button class="chat-cbtn ic" id="chatAt" title="@ 提及实体">${icon('at', 17)}</button>
                <button class="chat-cbtn ic" id="chatPlus" title="附件">${icon('plus', 17)}</button>
                <span class="chat-right">
                  <button class="chat-cbtn stop" id="chatStop">${icon('stop', 13)} 停止</button>
                  <span class="chat-spin">${icon('spin', 14)}</span>
                  <button class="chat-cbtn model"><b>claude-opus-4-8</b> ${icon('chevd', 13)}</button>
                </span>
              </div>
            </div>
          </div>
        </div>`;

      conv = qs('#chatConv', sea); col = qs('#chatCol', sea);
      dock = BlockKit.dock(qs('#chatDock', sea));

      // 主区头：实体预览右岛（随时唤起）+ 重播本回合
      Shell.headExtra(`<button class="ibtn" id="chatIsland" title="实体预览（右岛）">${icon('panel', 16)}</button><button class="ibtn" id="chatReplay" title="重播本回合">${icon('play', 16)}</button>`);
      const isl = qs('#chatIsland'); if (isl) isl.onclick = () => toggleIsland();
      const replay = qs('#chatReplay'); if (replay) replay.onclick = () => play(curScript, curTitle);

      // 对话标题（head-lead 左角；标题 + 向下箭头快捷操作）
      Shell.headLead.querySelectorAll('[data-ocean-head="chat"]').forEach(e => e.remove());
      const titlebar = tag('span.chat-titlebar', { 'data-ocean-head': 'chat' },
        `<button class="chat-title"><span class="chat-tt"></span><span class="chat-chev">${icon('chevd', 12)}</span></button>
         <div class="chat-qa"><div class="chat-qaitem" data-a="rename">重命名</div><div class="chat-qaitem" data-a="pin">置顶</div><div class="chat-qaitem" data-a="archive">归档</div><div class="chat-qaitem danger" data-a="delete">删除对话</div></div>`);
      Shell.headLead.appendChild(titlebar);
      const titleBtn = qs('.chat-title', titlebar), qa = qs('.chat-qa', titlebar);
      titleBtn.onclick = e => { e.stopPropagation(); const open = qa.classList.toggle('show'); titleBtn.classList.toggle('open', open); };
      document.addEventListener('click', () => { qa.classList.remove('show'); titleBtn.classList.remove('open'); });

      // composer 交互（演示态：@提及弹层 / 发送=重播 / 停止=cancel）
      qs('#chatAt', sea).onclick = () => qs('#chatMpop', sea).classList.toggle('show');
      sea.querySelectorAll('.chat-mrow').forEach(r => r.onclick = () => qs('#chatMpop', sea).classList.remove('show'));
      qs('#chatEnter', sea).onclick = () => play(curScript, curTitle);
      qs('#chatTa', sea).onkeydown = e => { if (e.key === 'Enter') play(curScript, curTitle); };
      qs('#chatStop', sea).onclick = () => { runId++; setGen(false); };

      play((M().default) || 'wf-weekly-report');
    },
  });

  // 本海洋拥有 conversation kind：侧栏会话行 → Intent.select({kind:'conversation'}) → 播放该脚本
  Intent.on('conversation', sel => { if (col) play(sel.id, sel.title); });
})();
