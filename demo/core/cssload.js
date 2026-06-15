/* Forgify demo — CSS 自载（幂等，按 href 去重）。
   契约：每个组件/海洋调 cssload(import.meta-less → 传自身 .css 的 URL) 自注入样式；
   于是没有中央 <head> 样式清单可争（防撞铁律之一），任意海洋在任意入口都能带齐皮肤。 */
(function () {
  const seen = new Set();
  window.cssload = function (href) {
    if (seen.has(href)) return;
    seen.add(href);
    const l = document.createElement('link');
    l.rel = 'stylesheet'; l.href = href; l.dataset.fg = '1';
    document.head.appendChild(l);
  };
  // 便捷：给定 currentScript.src 求同名 .css（组件用 cssNextTo(s) 一行自载）
  window.cssNextTo = function (scriptEl) {
    if (!scriptEl || !scriptEl.src) return;
    window.cssload(scriptEl.src.replace(/\.js(\?.*)?$/, '.css'));
  };
})();
