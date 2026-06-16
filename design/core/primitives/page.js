/* Foryx 原语 — Page(记录页骨架)。居中 --w-content 列 + 唯一滚动区。杀掉各海洋手搓的 .doc-root/.sch-col(SPEC §4.1)。
   header/tabs/sections 全填进 .col。mount(host, {body?}) → {el, col}。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function html(o) {
    o = o || {};
    return '<div class="fy-page"><div class="fy-page-col">' + (o.body || '') + '</div></div>';
  }
  function mount(host, o) {
    var e = window.el(html(o));
    if (host) host.appendChild(e);
    return { el: e, col: e.querySelector('.fy-page-col') };
  }

  window.FyPage = { html: html, mount: mount };
})();
