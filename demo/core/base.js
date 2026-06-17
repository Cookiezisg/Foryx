/* Anselm demo — AnElement：所有原语的 Web Component 基类（结构约束的地基）。
   why：原语 = 原生 custom element + Shadow DOM。feature 只能写 <an-*> 标签来拼，够不到原语内部、改不了样式 —— “各自造轮子” 在结构上不可能。
   token 是 :root 上的自定义属性，会【穿透 shadow 边界】继续生效，所以组件内 var(--…) 照常工作。
   契约：子类声明 static tag / static css / static observed[] / render()（可选 hydrate()）。
   一个文件 = 一个原语 = 一个主人（CSS 内联在 static css，shadow 内类名无需前缀——已被封装隔离）。 */
(function () {
  const BASE_CSS = `
    :host { display: block; }
    :host([hidden]) { display: none; }
    *, *::before, *::after { box-sizing: border-box; }
    button { font: inherit; color: inherit; background: none; border: 0; padding: 0; cursor: pointer; }
    svg { display: block; }
    :focus-visible { outline: var(--line-2) solid var(--accent-line); outline-offset: var(--hairline); }
    :focus:not(:focus-visible) { outline: none; }
  `;

  let baseSheet;
  function sheet(css) { const s = new CSSStyleSheet(); s.replaceSync(css); return s; }

  class AnElement extends HTMLElement {
    constructor() { super(); this.attachShadow({ mode: "open", delegatesFocus: !!this.constructor.delegatesFocus }); }
    connectedCallback() { this._render(); }
    attributeChangedCallback() { if (this.isConnected) this._render(); }

    _render() {
      if (!baseSheet) baseSheet = sheet(BASE_CSS);
      const C = this.constructor;
      if (!C._sheet) C._sheet = sheet(C.css || "");
      this.shadowRoot.adoptedStyleSheets = [baseSheet, C._sheet];
      this.shadowRoot.innerHTML = this.render ? this.render() : "";
      if (this.hydrate) this.hydrate();
    }

    attr(n, d) { const v = this.getAttribute(n); return v == null ? d : v; }
    has(n) { return this.hasAttribute(n); }
    num(n, d) { const v = parseInt(this.getAttribute(n) || "", 10); return Number.isFinite(v) ? v : (d || 0); }
    $(s) { return this.shadowRoot.querySelector(s); }
    $$(s) { return [].slice.call(this.shadowRoot.querySelectorAll(s)); }
    emit(name, detail) { this.dispatchEvent(new CustomEvent(name, { detail: detail || {}, bubbles: true, composed: true })); }
  }

  AnElement.define = function (C) {
    if (C.observed) Object.defineProperty(C, "observedAttributes", { value: C.observed });
    if (!customElements.get(C.tag)) customElements.define(C.tag, C);
  };

  // 单一 HTML 转义（原语 render() 共用）
  function anEsc(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }
  // 名称人性化：run_function → run function（标识符去下划线/连字符，供工具名等展示，全 demo 统一）
  function anLabel(s) { return String(s == null ? "" : s).replace(/[_-]+/g, " ").trim(); }

  window.AnElement = AnElement;
  window.anEsc = anEsc;
  window.anLabel = anLabel;
})();
