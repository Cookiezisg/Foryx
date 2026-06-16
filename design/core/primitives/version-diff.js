/* Foryx 原语 — VersionDiff。版本不是说明列表,而是可点击的差异工作面。
   API: FyVersionDiff.mount(host,{ versions, field?, caption? }) → {el,select,lineDiff}。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function text(v) { return String(v == null ? '' : v); }
  function pickVersion(v) { return v.version || v.v || ''; }
  function pickLabel(v) { return v.label || v.title || (v.active ? '当前版本' : '历史版本'); }
  function pickReason(v) { return v.reason || v.hint || ''; }
  function sourceOf(v, field) {
    if (typeof field === 'function') return text(field(v));
    if (field) return text(v[field]);
    return text(v.source || v.code || v.prompt || v.body || '');
  }

  function lineDiff(oldText, newText) {
    var A = text(oldText).split('\n');
    var B = text(newText).split('\n');
    var m = A.length;
    var n = B.length;
    var dp = Array.from({ length: m + 1 }, function () { return new Array(n + 1).fill(0); });
    var i;
    var j;
    for (i = m - 1; i >= 0; i--) {
      for (j = n - 1; j >= 0; j--) {
        dp[i][j] = A[i] === B[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
      }
    }
    var out = [];
    i = 0; j = 0;
    while (i < m && j < n) {
      if (A[i] === B[j]) { out.push(['ctx', A[i]]); i++; j++; }
      else if (dp[i + 1][j] >= dp[i][j + 1]) out.push(['del', A[i++]]);
      else out.push(['add', B[j++]]);
    }
    while (i < m) out.push(['del', A[i++]]);
    while (j < n) out.push(['add', B[j++]]);
    return out;
  }

  function highlight(line) {
    return window.FyCodeEditor && window.FyCodeEditor.highlight
      ? window.FyCodeEditor.highlight(line)
      : window.esc(line);
  }

  function mount(host, o) {
    o = o || {};
    var versions = o.versions || [];
    var selected = Math.max(0, Math.min(o.value || 0, versions.length - 1));
    var el = window.tag('div.fy-version-diff');
    el.innerHTML = '<div class="fy-vdiff-list"></div><div class="fy-vdiff-main"><div class="fy-vdiff-cap"></div><div class="fy-vdiff-body"></div></div>';
    var list = el.querySelector('.fy-vdiff-list');
    var cap = el.querySelector('.fy-vdiff-cap');
    var body = el.querySelector('.fy-vdiff-body');

    function renderList() {
      list.innerHTML = versions.map(function (v, i) {
        return '<button type="button" class="fy-vdiff-row' + (i === selected ? ' on' : '') + (v.active ? ' cur' : '') + '" data-index="' + i + '">'
          + '<span class="fy-vdiff-v">v' + window.esc(pickVersion(v)) + '</span>'
          + '<span class="fy-vdiff-text"><span class="fy-vdiff-label">' + window.esc(pickLabel(v)) + '</span>'
          + '<span class="fy-vdiff-reason">' + window.esc(pickReason(v)) + '</span></span>'
          + '</button>';
      }).join('');
      window.qsa('.fy-vdiff-row', list).forEach(function (row) {
        row.addEventListener('click', function () { select(Number(row.dataset.index)); });
      });
    }

    function renderBody() {
      var cur = versions[selected];
      var prev = versions[selected + 1];
      if (!cur) {
        cap.innerHTML = '';
        body.innerHTML = '';
        return;
      }
      var ops;
      var line = 0;
      var add = 0;
      var del = 0;
      if (prev) {
        ops = lineDiff(sourceOf(prev, o.field), sourceOf(cur, o.field));
      } else {
        ops = sourceOf(cur, o.field).split('\n').map(function (s) { return ['ctx', s]; });
      }
      body.innerHTML = ops.map(function (op) {
        var cls = op[0];
        if (cls === 'add') add++;
        if (cls === 'del') del++;
        if (cls !== 'del') line++;
        return '<div class="fy-vdiff-line ' + cls + '">'
          + '<span class="fy-vdiff-ln">' + (cls === 'del' ? '' : line) + '</span>'
          + '<span class="fy-vdiff-sg">' + (cls === 'add' ? '+' : cls === 'del' ? '-' : '') + '</span>'
          + '<code class="fy-vdiff-code">' + highlight(op[1]) + '</code>'
          + '</div>';
      }).join('');
      cap.innerHTML = prev
        ? '<span class="fy-vdiff-range">v' + window.esc(pickVersion(prev)) + ' -> v' + window.esc(pickVersion(cur)) + '</span>'
          + '<span class="fy-vdiff-caption">' + window.esc(o.caption || 'diff') + '</span>'
          + '<span class="fy-vdiff-count"><b class="add">+' + add + '</b><b class="del">-' + del + '</b></span>'
        : '<span class="fy-vdiff-range">v' + window.esc(pickVersion(cur)) + '</span><span class="fy-vdiff-caption">最早版本</span>';
    }

    function select(index) {
      if (index < 0 || index >= versions.length) return;
      selected = index;
      renderList();
      renderBody();
      if (o.onSelect) o.onSelect(versions[selected], selected);
    }

    renderList();
    renderBody();
    if (host) host.appendChild(el);
    return { el: el, select: select, lineDiff: lineDiff };
  }

  window.FyVersionDiff = { mount: mount, lineDiff: lineDiff };
})();
