/* Foryx — DOM 公共件(全局,原语共用)。tag / qs / qsa / esc。
   esc 单一实现(`demo` 里抄了 10 遍 → 收归一份,见 SPEC §1/§7)。 */
(function () {
  // tag('div.a.b', attrs?, html?) → element
  function tag(spec, attrs, html) {
    var parts = String(spec).split('.');
    var el = document.createElement(parts[0] || 'div');
    for (var i = 1; i < parts.length; i++) if (parts[i]) el.classList.add(parts[i]);
    if (attrs != null && typeof attrs === 'object' && !(attrs instanceof Node)) {
      for (var k in attrs) if (attrs[k] != null) el.setAttribute(k, attrs[k]);
    } else if (attrs != null) { html = attrs; }
    if (html != null) el.innerHTML = html;
    return el;
  }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  // 由 html(opts) 字符串建元素并返回首子(原语 mount 共用)
  function el(html) { var d = document.createElement('div'); d.innerHTML = html; return d.firstElementChild; }

  window.tag = tag;
  window.esc = esc;
  window.el = el;
  window.qs = function (s, r) { return (r || document).querySelector(s); };
  window.qsa = function (s, r) { return [].slice.call((r || document).querySelectorAll(s)); };
})();
