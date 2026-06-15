/* Forgify demo — 组件 version-diff（实体版本 diff，单一事实源；收掉 entities 海洋 versionDiff/lineDiff + chat 的 .dline diff 副本）。
   契约：组件 = 工厂函数 → {el, select}；自载同名 .css；只读令牌 + CodeEditor 公开高亮工厂；fg- 前缀；不碰别的海洋。
   API：VersionDiff.mount(host,{versions,field,caption}) → {el, select(n)}
        versions = [{v, ...fields}]（按新→旧排序，v 为版本号、active 标当前、t 副标、reason 说明）
        field    = 取要 diff 的字段（字符串名 或 v=>string 取值器）；caption = diff 区说明文案
        VersionDiff.lineDiff(a,b) → ops（LCS 行级 diff，[op,text][] · op∈ctx/add/del）——静态、纯算法、可独立调。
   形态铁律：左版本列 196px airy（mono v 号 + active 角标 + on/cur 态）+ 右白底细描边 diff 区；
            选某版即与其下一版（更旧）逐行 LCS 对比，add 染 --ok、del 染 --danger，行入场 eoDlIn 错峰。
            行内语法着色【只】走 CodeEditor.highlight（唯一 tokenizer），不在此另起 hl。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  // 行内着色单一事实源：优先公开 CodeEditor.highlight；缺失则裸转义兜底（不另起第二套 tokenizer）
  const hl = code => (window.CodeEditor && window.CodeEditor.highlight) ? window.CodeEditor.highlight(code) : esc(code);

  // 行级 diff（LCS）——逆向 DP 求最长公共子序列，回溯出 ctx/add/del 操作序列。纯算法、无副作用、对外静态可调。
  function lineDiff(a, b) {
    const A = String(a == null ? '' : a).split('\n'), B = String(b == null ? '' : b).split('\n'), m = A.length, n = B.length;
    const dp = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
    for (let i = m - 1; i >= 0; i--) for (let j = n - 1; j >= 0; j--) dp[i][j] = A[i] === B[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    const o = []; let i = 0, j = 0;
    while (i < m && j < n) { if (A[i] === B[j]) { o.push(['ctx', A[i]]); i++; j++; } else if (dp[i + 1][j] >= dp[i][j + 1]) o.push(['del', A[i++]]); else o.push(['add', B[j++]]); }
    while (i < m) o.push(['del', A[i++]]); while (j < n) o.push(['add', B[j++]]); return o;
  }

  function mount(host, { versions = [], field, caption = '' } = {}) {
    // field 既收字段名串、也收取值器函数——归一成 v=>string
    const pick = typeof field === 'function' ? field : (v => v && v[field]);
    const w = window.tag('div.fg-vd');
    w.innerHTML = `<div class="fg-vd-list"></div>`
      + `<div class="fg-vd-diff"><div class="fg-vd-cap"></div><div class="fg-vd-body"></div></div>`;
    const list = w.querySelector('.fg-vd-list'), capE = w.querySelector('.fg-vd-cap'), body = w.querySelector('.fg-vd-body');
    let sel = 0;

    function paint() {
      // 版本列：mono v 号 + 副标 + 说明；active=当前(角标) · on=选中
      list.innerHTML = versions.map((v, i) =>
        `<div class="fg-vd-row${v.active ? ' cur' : ''}${i === sel ? ' on' : ''}" data-i="${i}">`
        + `<span class="fg-vd-vn">v${esc(v.v)}</span>`
        + `<span class="fg-vd-vt">${esc(v.t || '')}</span>`
        + `<span class="fg-vd-vd">${esc(v.reason || '')}</span></div>`).join('');
      list.querySelectorAll('.fg-vd-row').forEach(r => r.onclick = () => select(+r.dataset.i));

      const nv = versions[sel], ov = versions[sel + 1];
      if (!nv) { capE.innerHTML = ''; body.innerHTML = ''; return; }

      // 最早版本：无更旧可比——整段以 ctx 渲染（着色但不染增删）
      if (!ov) {
        capE.innerHTML = `<span class="fg-vd-mono">v${esc(nv.v)}</span><span>· 最早版本</span>`;
        const src = String(pick(nv) == null ? '' : pick(nv));
        body.innerHTML = src.split('\n').map((l, i) =>
          `<div class="fg-vd-dl"><span class="fg-vd-ln">${i + 1}</span><span class="fg-vd-sg"></span><span class="fg-vd-ct">${hl(l)}</span></div>`).join('');
        return;
      }

      // 与下一版（更旧）逐行 LCS；add/del 计数右对齐、del 行不占行号
      const d = lineDiff(pick(ov), pick(nv)); let ln = 0, ad = 0, de = 0;
      body.innerHTML = d.map(([op, t]) => {
        const sg = op === 'add' ? '+' : op === 'del' ? '−' : ' ';
        if (op === 'add') ad++; if (op === 'del') de++;
        return `<div class="fg-vd-dl ${op}"><span class="fg-vd-ln">${op === 'del' ? '' : ++ln}</span>`
          + `<span class="fg-vd-sg">${sg}</span><span class="fg-vd-ct">${hl(t)}</span></div>`;
      }).join('');
      capE.innerHTML = `<span class="fg-vd-mono">v${esc(ov.v)} → v${esc(nv.v)}</span>`
        + `<span>· ${esc(caption)}</span>`
        + `<span class="fg-vd-pm"><span class="fg-vd-a">+${ad}</span> <span class="fg-vd-d">−${de}</span></span>`;
    }

    function select(n) {
      if (n < 0 || n >= versions.length || n === sel) { if (!list.children.length) paint(); return; }
      sel = n; paint();
    }

    paint();
    if (host) host.appendChild(w);
    return { el: w, select };
  }

  window.VersionDiff = { mount, lineDiff };
})();
