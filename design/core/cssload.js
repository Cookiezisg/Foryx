/* Foryx — cssNextTo:原语/feature 自载同名 .css(无中央 <head> 样式清单可争)。
   用法:文件首行 if (window.cssNextTo) cssNextTo(document.currentScript); */
(function () {
  window.cssNextTo = function (script) {
    if (!script || !script.src) return;
    var href = script.src.replace(/\.js(\?.*)?$/, '.css');
    if (document.querySelector('link[data-css="' + href + '"]')) return;
    var l = document.createElement('link');
    l.rel = 'stylesheet'; l.href = href; l.setAttribute('data-css', href);
    document.head.appendChild(l);
  };
})();
