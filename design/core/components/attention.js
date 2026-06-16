/* Foryx demo — 组件 attention（细描边 callout 框；收掉 entities .eo-attn + scheduler .sch-stuck 两处副本）。
   契约：组件 = 工厂函数 → html/handle；自载同名 .css；只读令牌 + icons；fg- 前缀；不碰任何 feature/别组件内部。
   API：Attention.html(iconKey, html, {tone}) → html · Attention.mount(host, {icon,html,tone}) → {el}。
   tone：warn(默认) / danger 走语义软底 + 描边；info 走白底细描边（doc-callout，无填充）。
   为何合并：两海洋各画一遍「左图标 + 提示文案」的告警条，色与留白漂移；统一为单一 tone 轴。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const ICO = window.icon || ((k, n) => '');

  // tone → 默认图标（调用方可用 iconKey 覆盖）
  const TONE_ICON = { warn: 'flag', danger: 'close', info: 'shield' };
  const norm = t => (t === 'danger' || t === 'info') ? t : 'warn';

  // 拼 callout html：左图标格 + 富文本体（body 可含 <b> 强调）
  function html(iconKey, body, opts = {}) {
    const tone = norm(opts.tone);
    const key = iconKey || TONE_ICON[tone];
    return `<div class="fg-attn ${tone}"><span class="fg-attn-ico">${ICO(key, 16)}</span><span class="fg-attn-body">${body == null ? '' : body}</span></div>`;
  }

  // 挂载进宿主，回 {el} 句柄（host 可传选择器或元素）
  function mount(host, opts = {}) {
    const h = typeof host === 'string' ? window.qs(host) : host;
    const el = window.tag('div', html(opts.icon, opts.html, opts));
    const node = el.firstElementChild;
    if (h) h.appendChild(node);
    return { el: node };
  }

  window.Attention = { html, mount };
})();
