/* Foryx 原语 — Section(段)。小节标题(大写灰)+ 内容岛。杀掉 entities/documents/scheduler 各自手搓的 sec/foldSec。
   opts:{ label?, body }(body = html 串或元素)。后续补可折叠 fold。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function html(o) {
    o = o || {};
    var lbl = o.label ? '<div class="fy-sec-label">' + window.esc(o.label) + '</div>' : '';
    var body = (typeof o.body === 'string') ? o.body : '';
    return '<section class="fy-sec">' + lbl + '<div class="fy-sec-body">' + body + '</div></section>';
  }

  function mount(host, o) {
    var el = window.el(html(o));
    if (o && o.body instanceof Node) el.querySelector('.fy-sec-body').appendChild(o.body);
    if (host) host.appendChild(el);
    return { el: el, body: el.querySelector('.fy-sec-body') };
  }

  window.FySection = { html: html, mount: mount };
})();
