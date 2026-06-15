/* Foryx 原语 — SidebarList。整条侧栏 = 一个模版:New / 过滤 / 分组标签 / 类型头 / 实体行 全部走同一行网格(Row),
   行首列结构共享 → New 的 + 、Search 的 🔍、类型图标、实体点 永远对齐(不靠手量,SPEC §4.6)。
   杀掉 demo 五份手搓 rail。opts:{ newLabel, filterPlaceholder, groups:[{label, types:[{icon,label,count,open,rows:[rowOpts…]}]}] , onNew, onSelect }。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function newRow(label) {
    return '<button type="button" class="fy-row fy-sl-action">'
      + '<span class="fy-row-lead"><span class="fy-row-ico">' + window.icon('plus', 16) + '</span></span>'
      + '<span class="fy-row-label">' + window.esc(label) + '</span><span></span></button>';
  }
  function filterRow(ph) {
    return '<div class="fy-row fy-sl-filter">'
      + '<span class="fy-row-lead"><span class="fy-row-ico">' + window.icon('search', 16) + '</span></span>'
      + '<span class="fy-row-label"><input class="fy-sl-input" placeholder="' + window.esc(ph || '') + '"></span>'
      + '<button type="button" class="fy-row-act fy-sl-sliders" title="显示选项">' + window.icon('sliders', 16) + '</button></div>';
  }
  function typeBlock(t) {
    var header = window.FyRow.html({ leading: { icon: t.icon }, label: t.label, collapsible: true, open: !!t.open, meta: (t.count != null ? String(t.count) : '') });
    var rows = (t.rows || []).map(function (r) {
      return window.FyRow.html(Object.assign({ depth: 1 }, r));   // 实体行缩进一级(模版内,leading 列仍对齐)
    }).join('');
    return '<div class="fy-sl-type' + (t.open ? ' open' : '') + '">' + header + '<div class="fy-sl-rows">' + rows + '</div></div>';
  }
  function groupBlock(g) {
    var lbl = g.label ? '<div class="fy-sl-grp">' + window.esc(g.label) + '</div>' : '';
    return lbl + (g.types || []).map(typeBlock).join('');
  }

  function html(o) {
    o = o || {};
    return '<div class="fy-sl">'
      + newRow(o.newLabel || 'New')
      + filterRow(o.filterPlaceholder)
      + '<div class="fy-sl-tree">' + (o.groups || []).map(groupBlock).join('') + '</div>'
      + '</div>';
  }

  function mount(host, o) {
    o = o || {};
    var el = window.el(html(o));
    // 类型头折叠
    window.qsa('.fy-sl-type > .fy-collapsible', el).forEach(function (h) {
      h.addEventListener('click', function () { h.parentNode.classList.toggle('open'); h.classList.toggle('open'); });
    });
    // 实体行选中
    window.qsa('.fy-sl-rows .fy-row', el).forEach(function (r) {
      r.addEventListener('click', function (e) {
        if (e.target.closest('.fy-row-act')) return;
        window.qsa('.fy-row.on', el).forEach(function (x) { x.classList.remove('on'); });
        r.classList.add('on');
        if (o.onSelect) o.onSelect(r.dataset.id, r);
      });
    });
    var nw = el.querySelector('.fy-sl-action');
    if (nw && o.onNew) nw.onclick = o.onNew;
    if (host) host.appendChild(el);
    return { el: el };
  }

  window.FySidebarList = { html: html, mount: mount };
})();
