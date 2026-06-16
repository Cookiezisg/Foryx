/* Foryx 原语 — Field(键值行)。左 label(+hint),右 控件/值。设置、实体 meta、信息块通用。
   模版:两列网格 [1fr auto],跨行 label 列 / 控件列对齐(不手量)。control = html 串(Button/Input/Badge/纯值)。
   KV.html([[k,v,opt]], opts) 渲染紧凑定义列表;KV.defs(host, [[k,v,opt]]) 渲染 Field 行。opts:{ label, hint?, control }。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function html(o) {
    o = o || {};
    var hint = o.hint ? '<div class="fy-field-hint">' + window.esc(o.hint) + '</div>' : '';
    return '<div class="fy-field">'
      + '<div class="fy-field-l"><div class="fy-field-k">' + window.esc(o.label || '') + '</div>' + hint + '</div>'
      + '<div class="fy-field-c">' + (o.control || '') + '</div></div>';
  }
  function mount(host, o) { var e = window.el(html(o)); if (host) host.appendChild(e); return { el: e }; }

  // 紧凑定义列表:rows = [[k, v, {mono?, html?}], …](v 默认按纯文本 escape)
  function kvHtml(rows) {
    return '<div class="fy-kv-list">' + (rows || []).map(function (r) {
      var opt = r[2] || {};
      var v = opt.html ? (r[1] || '') : window.esc(r[1] == null ? '—' : r[1]);
      return '<div class="fy-kv-row"><span class="fy-kv-k">' + window.esc(r[0]) + '</span>'
        + '<span class="fy-kv-v' + (opt.mono ? ' fy-mono' : '') + '">' + v + '</span></div>';
    }).join('') + '</div>';
  }

  // 定义列表:rows = [[k, v, {mono?}], …](v 为纯文本/值)
  function defs(host, rows) {
    (rows || []).forEach(function (r) {
      var v = '<span class="fy-field-v' + (r[2] && r[2].mono ? ' fy-mono' : '') + '">' + window.esc(r[1] == null ? '—' : r[1]) + '</span>';
      mount(host, { label: r[0], control: v });
    });
    return host;
  }

  window.FyField = { html: html, mount: mount };
  window.KV = { html: kvHtml, defs: defs };
})();
