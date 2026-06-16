/* Foryx 原语 — Section(段)。默认小节标题(大写灰)+ 内容岛;plain 变体给文档型海洋直接铺在白海面上。
   opts:{ label?, variant?:'plain', body }(body = html 串或元素)。后续补可折叠 fold。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function html(o) {
    o = o || {};
    var cls = 'fy-sec' + (o.variant === 'plain' ? ' fy-sec-plain' : '');
    var lbl = o.label ? '<div class="fy-sec-label">' + window.esc(o.label) + '</div>' : '';
    var body = (typeof o.body === 'string') ? o.body : '';
    return '<section class="' + cls + '">' + lbl + '<div class="fy-sec-body">' + body + '</div></section>';
  }

  function mount(host, o) {
    var el = window.el(html(o));
    if (o && o.body instanceof Node) el.querySelector('.fy-sec-body').appendChild(o.body);
    if (host) host.appendChild(el);
    return { el: el, body: el.querySelector('.fy-sec-body') };
  }

  window.FySection = { html: html, mount: mount };
})();
