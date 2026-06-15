/* Foryx 原语 — Button。变体 ghost(默认中性)/ primary(accent CTA)/ danger / icon(方钮)。
   高 = --ctl;统一 hover/active/focus/disabled(SPEC §6)。契约:html(opts)→串 · mount(host,opts)→{el}。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function html(o) {
    o = o || {};
    var v = o.variant || 'ghost';
    var cls = 'fy-btn fy-btn-' + v
      + (o.size === 'sm' ? ' fy-btn-sm' : '')
      + (o.block ? ' fy-btn-block' : '');
    var ic = o.icon ? '<span class="fy-btn-ico">' + window.icon(o.icon, 16) + '</span>' : '';
    var lbl = (v !== 'icon' && o.label) ? '<span>' + window.esc(o.label) + '</span>' : '';
    var attrs = (o.disabled ? ' disabled' : '') + (o.title ? ' title="' + window.esc(o.title) + '"' : '');
    return '<button type="button" class="' + cls + '"' + attrs + '>' + ic + lbl + '</button>';
  }

  function mount(host, o) {
    var el = window.el(html(o));
    if (o && o.onClick) el.onclick = o.onClick;
    if (host) host.appendChild(el);
    return { el: el };
  }

  window.FyButton = { html: html, mount: mount };
})();
