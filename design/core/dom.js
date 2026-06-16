/* Foryx — DOM 公共件(全局,原语共用)。tag / qs / qsa / esc。
   esc 单一实现(`demo` 里抄了 10 遍 → 收归一份,见 SPEC §1/§7)。 */
(function () {
  // tag('div.a.b#id', attrs?, html?) → element. Supports the demo product modules
  // and the design primitives with the same helper.
  function tag(spec, attrs, html) {
    var tagName = (String(spec).match(/^[A-Za-z0-9-]+/) || [])[0] || 'div';
    var el = document.createElement(tagName);
    var bits = String(spec).slice(tagName.length).match(/[.#][A-Za-z0-9_-]+/g) || [];
    bits.forEach(function (bit) {
      if (bit[0] === '.') el.classList.add(bit.slice(1));
      if (bit[0] === '#') el.id = bit.slice(1);
    });
    if (attrs != null && typeof attrs === 'object' && !(attrs instanceof Node)) {
      for (var k in attrs) {
        if (attrs[k] == null) continue;
        if (k === 'onclick' || k.indexOf('on') === 0) el[k] = attrs[k];
        else el.setAttribute(k, attrs[k]);
      }
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
