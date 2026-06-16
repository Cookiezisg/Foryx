/* Foryx 原语 — JsonTree。JSON 展示必须解析成结构化树,不裸露原始 JSON。
   API: FyJsonTree.html({ data?, json?, label?, openDepth?, root? })。
   root:false 隐藏根节点,让外层 InfoCard/Section 标题承担语义。
   openDepth 默认:有根时根 + 第一层对象/数组展开;无根时第一层对象/数组展开。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function parse(o) {
    if (o && Object.prototype.hasOwnProperty.call(o, 'data')) return { ok: true, value: o.data };
    if (o && typeof o.json === 'string') {
      try { return { ok: true, value: JSON.parse(o.json) }; }
      catch (err) { return { ok: false, message: err.message }; }
    }
    return { ok: true, value: null };
  }

  function kind(v) {
    if (v === null) return 'null';
    if (Array.isArray(v)) return 'array';
    return typeof v === 'object' ? 'object' : typeof v;
  }

  function meta(v) {
    if (Array.isArray(v)) return '[' + v.length + ']';
    if (v && typeof v === 'object') return '{' + Object.keys(v).length + '}';
    return kind(v);
  }

  function valueText(v) {
    if (v === null) return 'null';
    if (typeof v === 'string') return v === '' ? '""' : v;
    if (typeof v === 'number' || typeof v === 'boolean') return String(v);
    return meta(v);
  }

  function children(v, depth, openDepth) {
    if (Array.isArray(v)) {
      return v.map(function (item, i) { return node(String(i), item, depth, openDepth); }).join('');
    }
    return Object.keys(v || {}).map(function (key) { return node(key, v[key], depth, openDepth); }).join('');
  }

  function row(label, v, depth) {
    var k = kind(v);
    return '<div class="fy-json-row fy-json-leaf" style="--depth:' + depth + '">'
      + '<span class="fy-json-lead"></span>'
      + '<span class="fy-json-key">' + window.esc(label) + '</span>'
      + '<span class="fy-json-value fy-json-' + k + '">' + window.esc(valueText(v)) + '</span>'
      + '</div>';
  }

  function node(label, v, depth, openDepth) {
    var k = kind(v);
    if (k !== 'object' && k !== 'array') return row(label, v, depth);
    var open = depth < openDepth ? ' open' : '';
    return '<details class="fy-json-node fy-json-' + k + '"' + open + '>'
      + '<summary class="fy-json-row fy-json-branch" style="--depth:' + depth + '">'
      + '<span class="fy-json-lead">' + window.icon('chevr') + '</span>'
      + '<span class="fy-json-key">' + window.esc(label) + '</span>'
      + '<span class="fy-json-meta">' + window.esc(meta(v)) + '</span>'
      + '</summary>'
      + '<div class="fy-json-children">' + children(v, depth + 1, openDepth) + '</div>'
      + '</details>';
  }

  function html(o) {
    o = o || {};
    var parsed = parse(o);
    if (!parsed.ok) {
      return '<div class="fy-json-tree"><div class="fy-json-row fy-json-error" style="--depth:0">'
        + '<span class="fy-json-lead"></span><span class="fy-json-key">invalid JSON</span>'
        + '<span class="fy-json-value">' + window.esc(parsed.message) + '</span></div></div>';
    }
    var label = o.label || 'root';
    var root = o.root !== false;
    var openDepth = o.openDepth == null ? (root ? 2 : 1) : o.openDepth;
    if (!root && (kind(parsed.value) === 'object' || kind(parsed.value) === 'array')) {
      return '<div class="fy-json-tree">' + children(parsed.value, 0, openDepth) + '</div>';
    }
    return '<div class="fy-json-tree">' + node(label, parsed.value, 0, openDepth) + '</div>';
  }

  function mount(host, o) {
    var e = window.el(html(o));
    if (host) host.appendChild(e);
    return { el: e };
  }

  window.FyJsonTree = { html: html, mount: mount };
})();
