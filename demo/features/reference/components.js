/* Anselm feature — reference 画廊的两件本地组件 + el 助手。
   why 本地组件而非裸 DOM：画廊本身也要标准化——每个原语的展位（标题+态栅格）与每个状态的态格（标签+活体实例）各成一件可复用组件，
   gallery.js 只做 catalog → 组件 的映射，不手搓任何视觉/布局。
   摆放约定：态格 = 白底圆角 tile（标签在上、实例在下），栅格 align-items:start（按内容贴合、不被同行拉高留空）；宽件整行铺满。 */
(function () {
  const e = window.anEsc;

  // 极简 el 助手：el(tag, attrs?, ...children)；children 可为 string/Node/数组。供 catalog/gallery 声明活体实例时复用。
  function el(tag, attrs, ...kids) {
    const n = document.createElement(tag);
    if (attrs) for (const k in attrs) {
      const v = attrs[k];
      if (v === false || v == null) continue;
      if (k === "_") { kids.unshift(v); continue; }                 // _ = 文本快捷
      if (k.startsWith("on") && typeof v === "function") n.addEventListener(k.slice(2).toLowerCase(), v);
      else if (k === "html") n.innerHTML = v;
      else if (k === "prop") Object.assign(n, v);                   // prop:{...} 直接挂 JS 属性（如 .rows/.data/.graph）
      else n.setAttribute(k, v === true ? "" : v);
    }
    kids.flat().forEach((c) => { if (c == null || c === false) return; n.append(c.nodeType ? c : document.createTextNode(String(c))); });
    return n;
  }

  // <an-specimen label span> — 一个状态 tile：mono 标签（上）+ 活体实例（下，默认 slot），白底圆角 + 细描边环。
  class AnSpecimen extends window.AnElement {
    static tag = "an-specimen";
    static observed = ["label", "span"];
    static css = `
      :host { display: block; min-width: 0; }
      :host([span]) { grid-column: 1 / -1; }
      .cell {
        display: flex; flex-direction: column; gap: var(--sp-2); min-width: 0;
        padding: var(--sp-3) var(--sp-4); background: var(--island); border-radius: var(--r-card);
        box-shadow: inset 0 0 0 var(--hairline) var(--line);
      }
      .lbl { flex: none; font-family: var(--mono); font-size: var(--t-meta); color: var(--ink-3); }
      /* 实例区：块流——块件（row/card/code/graph）占满宽，行内件（badge/button/dot/pill）自然宽贴左 */
      .stage { min-width: 0; }
      ::slotted(*) { max-width: 100%; }
    `;
    render() {
      const lbl = this.attr("label") ? `<div class="lbl">${e(this.attr("label"))}</div>` : "";
      return `<div class="cell">${lbl}<div class="stage"><slot></slot></div></div>`;
    }
  }

  // <an-spec name tag blurb> — 一个原语的展位：标题行（名 + tag chip + 一句话）+ 自适应态栅格（align-items:start 贴合内容）。
  class AnSpec extends window.AnElement {
    static tag = "an-spec";
    static observed = ["name", "tag", "blurb"];
    static css = `
      :host { display: block; scroll-margin-top: var(--sp-6); }
      .head { margin-bottom: var(--sp-3); }
      .title { display: flex; align-items: baseline; gap: var(--gap); flex-wrap: wrap; }
      .name { font-size: var(--t-strong); font-weight: 600; color: var(--ink); }
      .tag { font-family: var(--mono); font-size: var(--t-meta); color: var(--accent); }
      .blurb { margin-top: var(--grid); font-size: var(--t-meta); color: var(--ink-3); line-height: var(--lh-prose); }
      .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(var(--spec-col), 1fr)); gap: var(--sp-3); align-items: start; }
    `;
    render() {
      const tag = this.attr("tag") ? `<span class="tag">&lt;${e(this.attr("tag"))}&gt;</span>` : "";
      const blurb = this.attr("blurb") ? `<div class="blurb">${e(this.attr("blurb"))}</div>` : "";
      return `<div class="head"><div class="title"><span class="name">${e(this.attr("name") || "")}</span>${tag}</div>${blurb}</div>`
        + `<div class="grid"><slot></slot></div>`;
    }
  }

  window.AnElement.define(AnSpecimen);
  window.AnElement.define(AnSpec);
  window.AnRef = Object.assign(window.AnRef || {}, { el });
})();
