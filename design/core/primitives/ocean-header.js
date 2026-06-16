/* Foryx 原语 — OceanHeader(海洋页头)。面包屑 + 标题 + meta 行 + 右侧动作。杀掉裸 headExtra 共享槽(SPEC §4.2)。
   opts:{ crumb:[…], title, meta?(html 串), actions?(html 串) }。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function html(o) {
    o = o || {};
    var crumb = (o.crumb && o.crumb.length)
      ? '<div class="fy-oh-crumb">' + o.crumb.map(function (c, i) {
          return (i ? '<span class="fy-oh-sep">/</span>' : '') + '<span>' + window.esc(c) + '</span>';
        }).join('') + '</div>'
      : '<div class="fy-oh-crumb"></div>';
    var actions = o.actions ? '<div class="fy-oh-actions">' + o.actions + '</div>' : '';
    var meta = o.meta ? '<div class="fy-oh-meta">' + o.meta + '</div>' : '';
    return '<header class="fy-oh"><div class="fy-oh-top">' + crumb + actions + '</div>'
      + '<h1 class="fy-oh-title">' + window.esc(o.title || '') + '</h1>' + meta + '</header>';
  }
  function mount(host, o) { var e = window.el(html(o)); if (host) host.appendChild(e); return { el: e }; }

  window.FyOceanHeader = { html: html, mount: mount };
})();
