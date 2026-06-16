/* Foryx 原语 — RightIsland(右岛抽屉)。跟海洋走的右侧白岛:头(图标+标题+关)+ 可滚正文。
   宽走 token(--island-w),不每海洋手编;幂等键 = feature id(SPEC §4.13)。opts:{ title, icon?, body }。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function html(o) {
    o = o || {};
    var ic = o.icon ? '<span class="fy-island-ico">' + window.icon(o.icon, 16) + '</span>' : '';
    return '<aside class="fy-island">'
      + '<div class="fy-island-head">' + ic + '<span class="fy-island-title">' + window.esc(o.title || '') + '</span>'
      + '<button type="button" class="fy-island-x" title="关闭">' + window.icon('close', 16) + '</button></div>'
      + '<div class="fy-island-body">' + (o.body || '') + '</div></aside>';
  }
  function mount(host, o) {
    var e = window.el(html(o));
    if (host) host.appendChild(e);
    return { el: e, body: e.querySelector('.fy-island-body') };
  }

  window.FyRightIsland = { html: html, mount: mount };
})();
