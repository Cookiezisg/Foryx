/* Foryx 原语 — ActionGroup。统一动作组:按钮间距、右对齐、事件委托都在这里,避免页面手摆按钮。
   API: FyActionGroup.html({ items, compact?, align?, block?, stack?, label? })。
   items: Button opts 或 { html, action?, onClick? }。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function itemHtml(item, index) {
    item = item || {};
    var html = (typeof item === 'string') ? item : (item.html || window.FyButton.html(item));
    var action = item.action || item.act || '';
    var data = ' data-action-index="' + index + '"' + (action ? ' data-action="' + window.esc(action) + '"' : '');
    return '<span class="fy-action-item"' + data + '>' + html + '</span>';
  }

  function html(o) {
    o = o || {};
    var cls = 'fy-action-group'
      + (o.compact ? ' fy-action-compact' : '')
      + (o.align === 'end' ? ' fy-action-end' : '')
      + (o.block ? ' fy-action-block' : '')
      + (o.stack ? ' fy-action-stack' : '');
    var label = o.label ? ' aria-label="' + window.esc(o.label) + '"' : '';
    return '<div class="' + cls + '" role="group"' + label + '>'
      + (o.items || []).map(itemHtml).join('')
      + '</div>';
  }

  function mount(host, o) {
    var e = window.el(html(o));
    if (o) {
      e.addEventListener('click', function (ev) {
        var node = ev.target.closest('.fy-action-item');
        if (!node || !e.contains(node)) return;
        var index = Number(node.getAttribute('data-action-index'));
        var item = (o.items || [])[index];
        var action = node.getAttribute('data-action') || '';
        if (item && item.onClick) item.onClick(item, ev);
        else if (action && o.onAction) o.onAction(action, item, ev);
      });
    }
    if (host) host.appendChild(e);
    return { el: e };
  }

  window.FyActionGroup = { html: html, mount: mount };
})();
