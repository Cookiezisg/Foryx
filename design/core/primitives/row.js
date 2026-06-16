/* Foryx 原语 — Row(核心)。唯一的"一行":承载会话/实体/workflow/文档树/通知/设置类目。
   解剖(SPEC §4.5):[行首槽 dot|icon↔chevron] gap [标签 flex:1] [尾槽 meta常驻↔动作hover]。
   两个同槽互换(行首 icon↔chevron / 行尾 meta↔action)+ 字色铁律 全焊在皮肤里。
   opts:{ leading:{dot|icon}, label, meta, actions:[{icon,act,title}], collapsible, open, selected, depth, id }。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function leadHtml(o) {
    if (o.leading && o.leading.dot) return '<span class="fy-row-lead">' + window.FyDot.dot(o.leading.dot) + '</span>';
    var ic = (o.leading && o.leading.icon) ? '<span class="fy-row-ico">' + window.icon(o.leading.icon, 16) + '</span>' : '';
    var chev = o.collapsible ? '<span class="fy-row-chev">' + window.icon('chevr', 16) + '</span>' : '';
    return '<span class="fy-row-lead">' + ic + chev + '</span>';
  }

  function html(o) {
    o = o || {};
    var hasActs = !!(o.actions && o.actions.length);
    var meta = (o.meta != null && o.meta !== '') ? '<span class="fy-row-meta">' + window.esc(o.meta) + '</span>' : '';
    var acts = hasActs
      ? '<span class="fy-row-acts">' + o.actions.map(function (a) {
          return '<button type="button" class="fy-row-act" data-act="' + window.esc(a.act || '') + '" title="' + window.esc(a.title || '') + '">' + window.icon(a.icon, 16) + '</button>';
        }).join('') + '</span>'
      : '';
    // 尾槽:meta 与动作【叠放同一格、共用中心】→ swap 绕光学中心切换、不平移
    var trail = (meta || acts) ? '<span class="fy-row-trail">' + meta + acts + '</span>' : '';
    var cls = 'fy-row' + (o.selected ? ' on' : '') + (o.collapsible ? ' fy-collapsible' : '') + (o.open ? ' open' : '') + (hasActs ? ' fy-has-acts' : '');
    var style = o.depth ? ' style="padding-left:calc(var(--pad-row) + ' + (o.depth | 0) + ' * var(--indent))"' : '';
    var data = (o.id != null) ? ' data-id="' + window.esc(o.id) + '"' : '';
    return '<div class="' + cls + '"' + data + style + '>'
      + leadHtml(o)
      + '<span class="fy-row-label">' + window.esc(o.label) + '</span>'
      + trail + '</div>';
  }

  function mount(host, o) {
    var el = window.el(html(o));
    if (o && o.onClick) el.addEventListener('click', function (e) {
      if (e.target.closest('.fy-row-act')) return;        // 动作钮不触发行选中
      o.onClick(o, e);
    });
    if (host) host.appendChild(el);
    return { el: el };
  }

  window.FyRow = { html: html, mount: mount };
})();
