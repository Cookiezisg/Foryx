/* Anselm 原语 E2 — <an-json-tree>。唯一的 JSON/结构化展示原语：JSON 必须解析成结构化树、不裸露原始串。
   解剖：object/array = 可折叠 summary 行（chevron + key + {n}/[n]，<details> 承载折叠态）；
     leaf = key/value 行（值按类型上色）。缩进走 --indent、行高 --row、密度继承 Row。
   属性：json（JSON 串）| label（根名，默认 root）| open-depth（默认：有根=根+第一层 / 无根=第一层）|
     root（root="false" 隐藏同名根行，让外层 Section/InfoCard 标题承担语义）。
   数据：优先读 .data 属性（对象，经 JS 设入）；否则解析 json 串属性。 */
(function () {
  function kind(v) {
    if (v === null) return "null";
    if (Array.isArray(v)) return "array";
    return typeof v === "object" ? "object" : typeof v;
  }

  function meta(v) {
    if (Array.isArray(v)) return "[" + v.length + "]";
    if (v && typeof v === "object") return "{" + Object.keys(v).length + "}";
    return kind(v);
  }

  function valueText(v) {
    if (v === null) return "null";
    if (typeof v === "string") return v === "" ? '""' : v;
    if (typeof v === "number" || typeof v === "boolean") return String(v);
    return meta(v);
  }

  // 缩进 = depth × --indent（depth 为纯整数倍数）；depth 0 flush 到容器左缘（不再加 pad-row 基），与 KV 同起点
  function pad(depth) {
    return ` style="padding-left: calc(${depth} * var(--indent))"`;
  }

  function children(v, depth, openDepth) {
    if (Array.isArray(v)) {
      return v.map(function (item, i) { return node(String(i), item, depth, openDepth); }).join("");
    }
    return Object.keys(v || {}).map(function (key) { return node(key, v[key], depth, openDepth); }).join("");
  }

  function leaf(label, v, depth) {
    var e = window.anEsc, k = kind(v);
    return '<div class="row leaf"' + pad(depth) + ">"
      + '<span class="key">' + e(label) + "</span>"
      + '<span class="value ' + k + '">' + e(valueText(v)) + "</span>"
      + "</div>";
  }

  function node(label, v, depth, openDepth) {
    var e = window.anEsc, k = kind(v);
    if (k !== "object" && k !== "array") return leaf(label, v, depth);
    var open = depth < openDepth ? " open" : "";
    return '<details class="node ' + k + '"' + open + ">"
      + '<summary class="row branch"' + pad(depth) + ">"
      + '<span class="lead">' + window.icon("chevr") + "</span>"
      + '<span class="key">' + e(label) + "</span>"
      + '<span class="meta">' + e(meta(v)) + "</span>"
      + "</summary>"
      + '<div class="children">' + children(v, depth + 1, openDepth) + "</div>"
      + "</details>";
  }

  class AnJsonTree extends window.AnElement {
    static tag = "an-json-tree";
    static observed = ["json", "label", "open-depth", "root"];
    static css = `
      :host { display: block; }
      .tree { color: var(--ink-2); font-size: var(--t-body); }
      .node { margin: 0; }
      .node > summary { list-style: none; }
      .node > summary::-webkit-details-marker { display: none; }

      /* branch = [chevron 槽 | key | meta]；leaf = [key | value]（无 lead 槽 → 扁平输出与 KV 同起点，不缩进成格子） */
      .row {
        display: grid; grid-template-columns: var(--lead) auto minmax(0, 1fr);
        align-items: center; column-gap: var(--gap); min-height: var(--row);
        padding: 0 var(--pad-row); border-radius: var(--r-btn); color: var(--ink-2);
      }
      .leaf { grid-template-columns: auto minmax(0, 1fr); }
      .branch { cursor: pointer; }
      .branch:hover { background: var(--island-3); color: var(--ink); }
      .lead { width: var(--lead); height: var(--lead); display: grid; place-items: center; color: var(--ink-3); }
      .lead svg { display: block; width: var(--icon); height: var(--icon); transition: transform var(--d-mid) var(--ease-spring); }
      .node[open] > .branch .lead svg { transform: rotate(90deg); }

      .key { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: inherit; }
      .meta,
      .value {
        min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        justify-self: start; font-family: var(--mono); font-size: var(--t-meta); color: var(--ink-3);
      }
      .string { color: var(--cd-str); }
      .number { color: var(--cd-num); }
      .boolean { color: var(--cd-kw); }
      .null { color: var(--cd-com); font-style: italic; }
      .error .key,
      .error .value { color: var(--danger); }
    `;

    // .data 走 JS 属性（对象不经线缆字符串）；设入即重渲染
    get data() { return this._data; }
    set data(v) { this._data = v; if (this.isConnected) this._render(); }

    _resolve() {
      // 优先 .data 对象；否则解析 json 串属性
      if (this._data !== undefined) return { ok: true, value: this._data };
      var raw = this.attr("json");
      if (raw != null) {
        try { return { ok: true, value: JSON.parse(raw) }; }
        catch (err) { return { ok: false, message: err.message }; }
      }
      return { ok: true, value: null };
    }

    render() {
      var e = window.anEsc;
      var parsed = this._resolve();
      if (!parsed.ok) {
        return '<div class="tree"><div class="row error"'
          + ' style="padding-left: var(--pad-row)">'
          + '<span class="lead"></span><span class="key">invalid JSON</span>'
          + '<span class="value">' + e(parsed.message) + "</span></div></div>";
      }
      var label = this.attr("label", "root");
      var root = this.attr("root") !== "false";
      var od = this.attr("open-depth");
      var openDepth = od == null || od === "" ? (root ? 2 : 1) : this.num("open-depth", root ? 2 : 1);
      var k = kind(parsed.value);
      if (!root && (k === "object" || k === "array")) {
        return '<div class="tree">' + children(parsed.value, 0, openDepth) + "</div>";
      }
      return '<div class="tree">' + node(label, parsed.value, 0, openDepth) + "</div>";
    }
  }
  window.AnElement.define(AnJsonTree);
})();
