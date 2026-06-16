/* Foryx 原语 — InfoCard。无边信息单元:靠标题、留白、层级组织内容,不靠横线切割。
   API: FyInfoCard.html({ title?, icon?, meta?, body?, actions? })。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function html(o) {
    o = o || {};
    var ic = o.icon ? '<span class="fy-info-ico">' + window.icon(o.icon) + '</span>' : '';
    var meta = o.meta ? '<span class="fy-info-meta">' + o.meta + '</span>' : '';
    var head = o.title || o.icon || o.meta
      ? '<div class="fy-info-head">' + ic + '<span class="fy-info-title">' + window.esc(o.title || '') + '</span>' + meta + '</div>'
      : '';
    var actions = o.actions ? '<div class="fy-info-actions">' + o.actions + '</div>' : '';
    return '<section class="fy-info-card">' + head + '<div class="fy-info-body">' + (o.body || '') + '</div>' + actions + '</section>';
  }

  function mount(host, o) {
    var e = window.el(html(o));
    if (host) host.appendChild(e);
    return { el: e, body: e.querySelector('.fy-info-body') };
  }

  window.FyInfoCard = { html: html, mount: mount };
})();
