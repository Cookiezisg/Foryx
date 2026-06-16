/* Foryx 原语 — Toolbar。三段工具条:left / main / right,用于局部工具面和页面动作条。
   API: FyToolbar.html({ left?, title?, meta?, body?, right?, actions?, compact? })。
   right/actions 可传 ActionGroup items 或 html 串。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function slotHtml(v) {
    if (!v) return '';
    if (Array.isArray(v)) return window.FyActionGroup.html({ items: v, align: 'end' });
    return v;
  }

  function html(o) {
    o = o || {};
    var main = o.body || '';
    if (!main && (o.title || o.meta)) {
      main = '<span class="fy-toolbar-title">' + window.esc(o.title || '') + '</span>'
        + (o.meta ? '<span class="fy-toolbar-meta">' + o.meta + '</span>' : '');
    }
    var right = slotHtml(o.right || o.actions);
    var cls = 'fy-toolbar' + (o.compact ? ' fy-toolbar-compact' : '');
    return '<div class="' + cls + '">'
      + '<div class="fy-toolbar-left">' + slotHtml(o.left) + '</div>'
      + '<div class="fy-toolbar-main">' + main + '</div>'
      + '<div class="fy-toolbar-right">' + right + '</div>'
      + '</div>';
  }

  function mount(host, o) {
    var e = window.el(html(o));
    if (host) host.appendChild(e);
    return { el: e };
  }

  window.FyToolbar = { html: html, mount: mount };
})();
