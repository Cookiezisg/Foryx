/* Foryx 原语 — Menu。统一菜单行:icon/check + label + meta,由 Floating 锚定。
   API: FyMenu.html({ items, compact? }) · FyMenu.open(anchor, opts)。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function itemHtml(it, idx) {
    it = it || {};
    if (it.type === 'label') {
      return '<div class="fy-menu-label">' + window.esc(it.label || '') + '</div>';
    }
    var cls = 'fy-menu-item'
      + (it.checked ? ' is-checked' : '')
      + (it.danger ? ' is-danger' : '')
      + (it.disabled ? ' is-disabled' : '');
    var lead = it.icon ? window.icon(it.icon) : (it.checked ? window.icon('check') : '');
    var meta = it.meta ? '<span class="fy-menu-meta">' + window.esc(it.meta) + '</span>' : '';
    var attrs = ' data-index="' + idx + '"'
      + (it.value != null ? ' data-value="' + window.esc(it.value) + '"' : '')
      + (it.disabled ? ' aria-disabled="true"' : '');
    return '<button type="button" class="' + cls + '"' + attrs + '>'
      + '<span class="fy-menu-lead">' + lead + '</span>'
      + '<span class="fy-menu-text">' + window.esc(it.label || '') + '</span>'
      + meta + '</button>';
  }

  function html(o) {
    o = o || {};
    var cls = 'fy-menu' + (o.compact ? ' fy-menu-compact' : '');
    return '<div class="' + cls + '" role="menu">' + (o.items || []).map(itemHtml).join('') + '</div>';
  }

  function open(anchor, o) {
    o = o || {};
    var h = window.FyFloating.open(anchor, {
      content: html(o),
      align: o.align || 'end',
      placement: o.placement || 'bottom',
      namespace: o.namespace || 'menu',
      className: 'fy-menu-pop',
    });
    h.el.querySelectorAll('.fy-menu-item').forEach(function (b) {
      b.addEventListener('click', function () {
        var it = (o.items || [])[Number(b.dataset.index)];
        if (!it || it.disabled) return;
        if (o.onPick) o.onPick(it.value, it);
        if (!it.keepOpen) window.FyFloating.close(o.namespace || 'menu');
      });
    });
    return h;
  }

  function attach(anchor, o) {
    anchor.addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      open(anchor, o);
    });
  }

  window.FyMenu = { html: html, open: open, attach: attach };
})();
