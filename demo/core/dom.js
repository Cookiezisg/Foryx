/* Forgify demo — DOM 微件（杀内联建节点样板）。
   tag('div.cls#id', html) → 元素；tag('button', {onclick,title}, html) → 带属性。
   故意极小：组件/海洋用它替代 document.createElement + className 的反复样板。 */
(function () {
  window.tag = function (spec, a, b) {
    const m = spec.match(/^([a-z0-9]+)?([.#][^\s]*)?$/i) || [];
    const e = document.createElement(m[1] || 'div');
    (m[2] || '').split(/(?=[.#])/).forEach(t => {
      if (t[0] === '.') e.classList.add(t.slice(1));
      else if (t[0] === '#') e.id = t.slice(1);
    });
    let attrs = null, html = null;
    if (a && typeof a === 'object') { attrs = a; html = b; } else { html = a; }
    if (attrs) for (const k in attrs) {
      if (k === 'onclick' || k.startsWith('on')) e[k] = attrs[k];
      else if (attrs[k] != null) e.setAttribute(k, attrs[k]);
    }
    if (html != null) e.innerHTML = html;
    return e;
  };
  // 便捷查询
  window.qs = (s, r) => (r || document).querySelector(s);
  window.qsa = (s, r) => [...(r || document).querySelectorAll(s)];
})();
