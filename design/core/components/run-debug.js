/* Foryx demo — 组件 run-debug（运行/调试面板，单一事实源；收掉 entities 海洋 runDebug 模板）。
   契约：组件 = 工厂函数 → {el}；自载同名 .css；只读令牌 + 配置 + 图标 + StatusDot/CodeEditor 公开工厂；fg- 前缀；不碰别的海洋。
   API：RunDebug.mount(host,{argsSeed,verb,vico,gate,trace}) → {el}
        argsSeed = JSON/字段种子（默认 {}）· verb = 运行钮文案 · vico = 钮图标 · gate = 闸说明（给即返不可运行块）
        trace = { lines:[], result:{st,out,ms} } —— 纯 mock 驱动：点钮逐行吐 stdout、收尾出结果摘要（StatusDot 点 + ms）。
   形态铁律：白底细描边块；args 在上、运行条居中（输入参数 token + 右对齐运行钮）、stdout 与 result 各自一段，
            运行时 token 走 shimmer、逐行 240ms 吐字，收尾 200ms 出 StatusDot 状态 + 等宽输出 + 毫秒。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const sleep = ms => new Promise(r => setTimeout(r, ms));

  // JSON/CEL 同源语法着色（与 code-editor / version-diff 共用 --cd-* 令牌；KW 表对齐 design-lab）
  const KW = new Set('const let var function def class return if elif else for while do in of import from export default new await async try except catch finally raise throw with as lambda yield and or not is None True False true false null undefined self this match case pass break continue'.split(' '));
  const TOK = /(#[^\n]*|\/\/[^\n]*|\/\*[\s\S]*?\*\/)|(`(?:\\.|[^`\\])*`|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|(\$\{[^}]*\}|\$\d+|\{\{[^}]*\}\})|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g;
  function hl(code) {
    let out = '', last = 0, m; TOK.lastIndex = 0;
    while ((m = TOK.exec(code))) {
      out += esc(code.slice(last, m.index));
      if (m[1]) out += `<span class="hl-com">${esc(m[1])}</span>`;
      else if (m[2]) out += `<span class="hl-str">${esc(m[2])}</span>`;
      else if (m[3]) out += `<span class="hl-arg">${esc(m[3])}</span>`;
      else if (m[4]) out += `<span class="hl-num">${esc(m[4])}</span>`;
      else { const w = m[5]; out += KW.has(w) ? `<span class="hl-kw">${w}</span>` : (/^\s*\(/.test(code.slice(m.index + w.length)) ? `<span class="hl-fn">${esc(w)}</span>` : esc(w)); }
      last = m.index + m[0].length;
    }
    return out + esc(code.slice(last));
  }

  // 自含 args 编辑器：行号槽 + 透明 textarea 叠高亮 pre + 角标——优先用公开 CodeEditor，否则原位复刻 .eo-code 形态
  function argsEditor(host, code, corner) {
    if (window.CodeEditor && window.CodeEditor.mount) { return window.CodeEditor.mount(host, { code, corner }); }
    const w = window.tag('div.fg-rd-code');
    w.innerHTML = `<div class="fg-rd-gut"></div><div class="fg-rd-area"><pre><code></code></pre><textarea spellcheck="false"></textarea></div>${corner ? `<span class="fg-rd-lang">${esc(corner)}</span>` : ''}`;
    const ta = w.querySelector('textarea'), codeEl = w.querySelector('code'), gut = w.querySelector('.fg-rd-gut'), pre = w.querySelector('pre');
    const paint = () => { codeEl.innerHTML = hl(ta.value); const n = ta.value.split('\n').length; gut.innerHTML = Array.from({ length: n }, (_, i) => `<i>${i + 1}</i>`).join(''); };
    ta.value = code; paint();
    ta.addEventListener('input', paint);
    ta.addEventListener('scroll', () => { pre.scrollTop = ta.scrollTop; pre.scrollLeft = ta.scrollLeft; });
    ta.addEventListener('keydown', e => { if (e.key === 'Tab') { e.preventDefault(); const s = ta.selectionStart, en = ta.selectionEnd; ta.value = ta.value.slice(0, s) + '    ' + ta.value.slice(en); ta.selectionStart = ta.selectionEnd = s + 4; paint(); } });
    host.appendChild(w);
    return { el: w, value: () => ta.value };
  }

  function mount(host, { argsSeed = '', verb = 'Run', vico = 'play', gate = null, trace } = {}) {
    const w = window.tag('div.fg-rd');

    // 闸态：环境/配置未就绪——只渲染一行盾说明，不挂运行链路
    if (gate) {
      w.innerHTML = `<div class="fg-rd-gate"><span class="fg-rd-gateico">${window.icon('shield', 15)}</span>${esc(gate)}</div>`;
      if (host) host.appendChild(w);
      return { el: w };
    }

    w.innerHTML = `<div class="fg-rd-args"></div>`
      + `<div class="fg-rd-bar"><span class="fg-rd-tk">输入参数</span><span class="fg-rd-grow"></span>`
      + `<button class="fg-rd-run" data-go>${window.icon(vico, 13)}${esc(verb)}</button></div>`
      + `<div class="fg-rd-out"></div><div class="fg-rd-res"></div>`;

    argsEditor(w.querySelector('.fg-rd-args'), argsSeed || '{}', 'args');
    const tk = w.querySelector('.fg-rd-tk'), btn = w.querySelector('[data-go]'), out = w.querySelector('.fg-rd-out'), res = w.querySelector('.fg-rd-res');

    btn.onclick = async () => {
      btn.style.pointerEvents = 'none'; res.classList.remove('show'); out.classList.add('show'); out.textContent = '';
      tk.textContent = '运行中…'; tk.classList.add('fg-rd-shimmer');
      const lines = (trace && trace.lines) || ['→ spawn sandbox', '→ exec', 'stdout: ok'];
      for (const l of lines) { await sleep(240); out.textContent += l + '\n'; }
      await sleep(200); tk.textContent = '输入参数'; tk.classList.remove('fg-rd-shimmer'); btn.style.pointerEvents = '';
      const r = (trace && trace.result) || { st: 'ok', out: 'done', ms: 100 };
      // 结果状态点走 StatusDot（ok→done 绿 / 其余→err 红，经 stState 折正）
      res.className = 'fg-rd-res show';
      res.innerHTML = `<span class="fg-rd-st">${window.StatusDot.dot(r.st === 'ok' ? 'done' : 'err', { size: 6 })}<span class="fg-rd-stt">${esc(r.st)}</span></span>`
        + `<span class="fg-rd-ev">${esc(r.out)}</span><span class="fg-rd-ms">${esc(r.ms)}ms</span>`;
    };

    if (host) host.appendChild(w);
    return { el: w };
  }

  window.RunDebug = { mount };
})();
