/* Forgify design-lab — Chat 海洋编排（单独，一人负责整个 oceans/chat/ 文件夹）。
   注册进外壳：Shell.registerOcean('chat', { build(sea) })，渲染对话流 + composer 到 #sea，
   右岛(实体卡)交给同目录 entity-card.js。信号交互 = 本海洋的灵魂。
   依赖：shared/icons.js · shared/shell.js · ./entity-card.js（ChatEntityCard）。 */
(function () {
  const $ = (s, r = document) => r.querySelector(s);
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  let runId = 0;
  const alive = id => id === runId;

  Shell.registerOcean('chat', {
    crumb: '前端设计 (fork)',
    build(sea) {
      sea.innerHTML = `
        <div class="conv" id="conv"><div class="col" id="col"></div></div>
        <div class="composer">
          <div class="cwrap">
            <div class="ctx">
              <span class="repo"><span data-i="repo"></span> Forgify</span>
              <span class="branch"><span data-i="branch"></span> main</span>
              <span class="diff"><span class="add">+172</span> <span class="del">−1</span></span>
              <button class="pr">Create PR <span data-i="chevd"></span></button>
            </div>
            <div class="box">
              <div class="field"><input id="ta" placeholder="Type / for commands"><span class="enter" data-i="enter"></span></div>
              <div class="bar">
                <button class="cbtn" id="b_auto">Auto <span data-i="chevd"></span></button>
                <button class="cbtn ic" data-i="plus"></button>
                <button class="cbtn ic" data-i="mic"></button>
                <span class="right"><span class="lbl">Opus 4.8</span><span class="lbl">Ultracode</span><span class="spin" data-i="spin"></span></span>
              </div>
            </div>
          </div>
        </div>`;
      const sz = { repo: 14, branch: 13, chevd: 13, enter: 16, plus: 17, mic: 16, spin: 14 };
      sea.querySelectorAll('[data-i]').forEach(el => { const k = el.dataset.i; el.innerHTML = icon(k, sz[k] || 16); });

      // 主区头：本海洋的按钮（重播信号交互 + 右岛面板切换）
      Shell.headExtra(`<button class="ibtn" id="i_replay" title="重播信号交互">${icon('play', 16)}</button><button class="ibtn" id="i_panel">${icon('panel')}</button>`);
      $('#i_replay').onclick = run;
      $('#i_panel').onclick = () => ChatEntityCard.toggle();
      $('#ta').closest('.box').querySelector('.enter').onclick = run;

      run();
    },
  });

  // —— 对话块：打字机 / 用户气泡 / AI turn / 工具调用组 ——
  const conv = () => $('#conv'), col = () => $('#col');
  const toBottom = () => { conv().scrollTop = conv().scrollHeight; };

  function typeInto(node, text, cps = 58) {
    const id = runId;
    return new Promise(res => {
      const caret = document.createElement('span'); caret.className = 'caret'; node.appendChild(caret);
      let i = 0;
      (function step() {
        if (!alive(id)) { caret.remove(); return res(); }
        caret.insertAdjacentText('beforebegin', text[i++] ?? '');
        toBottom();
        if (i > text.length) { caret.remove(); return res(); }
        setTimeout(step, 1000 / cps + Math.random() * 16);
      })();
    });
  }
  function userMsg(html) { const m = document.createElement('div'); m.className = 'umsg'; m.innerHTML = `<div class="b">${html}</div>`; col().appendChild(m); toBottom(); }
  function aiTurn() { const t = document.createElement('div'); t.className = 'turn';
    t.innerHTML = `<div class="spark">${icon('spark', 16, 1.6)}</div><div class="amsg"></div>`; col().appendChild(t); toBottom(); return $('.amsg', t); }
  function para(body) { const p = document.createElement('p'); body.appendChild(p); return p; }

  // 工具调用组（复刻 Claude Code）：运行态文字流光 → 收敛「Used N tools ›」+ 外框工具列表（各自展开详情）
  function toolAct(body) {
    const w = document.createElement('div'); w.className = 'tg';
    w.innerHTML = `<div class="tg-sum run"><span class="tk"></span><span class="chev" style="display:none">${icon('chevr', 14)}</span></div>
      <div class="tg-list"><div class="w"><div class="tg-box"></div></div></div>`;
    body.appendChild(w); toBottom();
    const sum = $('.tg-sum', w), tk = $('.tk', w), chev = $('.chev', w), box = $('.tg-box', w);
    sum.onclick = () => { if (box.innerHTML) w.classList.toggle('open'); };
    return {
      status(t) { tk.textContent = t; toBottom(); },
      settle(summary, itemsHTML) {
        sum.classList.remove('run'); tk.textContent = summary; box.innerHTML = itemsHTML; chev.style.display = ''; w.classList.add('open');
        box.querySelectorAll('.ti-sum').forEach(s => s.onclick = () => s.closest('.ti').classList.toggle('open'));
        toBottom();
      },
    };
  }

  // —— 信号交互编排 ——
  async function run() {
    const id = ++runId;
    col().innerHTML = ''; ChatEntityCard.hide(); if (ChatEntityCard.el) ChatEntityCard.el.innerHTML = '';

    userMsg('帮我把仓库这周的 PR 理一理。');
    const h = aiTurn();
    h.innerHTML = `<p>这周有 9 个 PR，其中 3 个还在等审。要我让 <span class="ref-pill" data-open>${icon('agent', 12)}Researcher</span> 出一份带链接的周报吗？</p>`;
    if (!alive(id)) return; await sleep(700);

    userMsg('把 Researcher 升级到 Opus，并要求它所有结论都带引用来源。');
    await sleep(450); if (!alive(id)) return;
    const body = aiTurn();
    await typeInto(para(body), '好的，我来更新 Researcher：切换模型到 claude-opus-4-8，并在系统提示里加上引用要求。', 66);
    if (!alive(id)) return;

    await sleep(250);
    const ta = toolAct(body); ta.status('Reading Researcher…');
    await sleep(800); if (!alive(id)) return;
    ta.status('Updating Researcher…');             // ← 这一刻唤出右岛
    await sleep(350); if (!alive(id)) return;
    ChatEntityCard.render({ name: 'Researcher', version: 4, model: 'claude-sonnet-4-6',
      desc: '深度调研助手：检索、交叉验证，产出带引用的结构化综述。',
      system: '你是严谨的调研助手。检索、交叉验证、给出结构化综述。',
      tools: ['web_fetch', 'read_file', 'python', 'summarize'], live: true });
    const aside = ChatEntityCard.el;
    await sleep(650); if (!alive(id)) return;

    const mv = $('[data-f="model"] .val', aside); mv.classList.add('flash'); mv.textContent = 'claude-opus-4-8';
    await sleep(900); if (!alive(id)) return;
    const sv = $('[data-f="system"] .val', aside); sv.classList.add('flash');
    await typeInto(sv, '\n\n务必为每条结论附引用来源（行内链接）。', 42);
    if (!alive(id)) return; await sleep(500);
    const tl = $('[data-f="tools"] .taglist', aside);
    const nt = document.createElement('span'); nt.className = 'tag new'; nt.textContent = 'web_search'; tl.appendChild(nt);
    $('[data-tc]', aside).textContent = tl.children.length;
    await sleep(700); if (!alive(id)) return;

    $('[data-ver]', aside).textContent = 'v5'; ChatEntityCard.setLive(false);
    ta.settle('Used 2 tools', `
      <div class="ti">
        <div class="ti-sum"><span class="v">Used</span> <span class="nm">read_agent(Researcher)</span> <span class="chev">${icon('chevr', 14)}</span></div>
        <div class="ti-det"><div class="w"><div class="tbox"><div class="out">model: claude-sonnet-4-6
tools: web_fetch, read_file, python, summarize</div></div></div></div>
      </div>
      <div class="ti open">
        <div class="ti-sum"><span class="v">Used</span> <span class="nm">update_agent(Researcher)</span> <span class="chev">${icon('chevr', 14)}</span></div>
        <div class="ti-det"><div class="w"><div class="tbox"><div class="diff">
          <div class="dline del"><span class="s">−</span><span class="c">model: claude-sonnet-4-6</span></div>
          <div class="dline add"><span class="s">+</span><span class="c">model: claude-opus-4-8</span></div>
          <div class="dline add"><span class="s">+</span><span class="c">system += 务必为每条结论附引用来源（行内链接）</span></div>
          <div class="dline add"><span class="s">+</span><span class="c">tools += web_search</span></div>
        </div></div></div></div>
      </div>`);
    await sleep(350); if (!alive(id)) return;
    await typeInto(para(body), '完成 ✅ Researcher 现在跑在 Opus 上，每条结论都会附引用。', 66);

    col().querySelectorAll('[data-open]').forEach(p => p.onclick = () => ChatEntityCard.show());
  }
})();
