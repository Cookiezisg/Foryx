/* Anselm 原语 — <an-group-label>。极薄分组/段小标题（uppercase + meta 字号 + 600 + ink-3）。
   why：这套小标题排版原本散在 buildRail 分组标签、图编辑器检查器段标题等多处内联 cssText 重抄——收进本件做单源，改样式只动一处。
   文本走默认 slot；纵向外距走皮肤（邻近原则：上分隔、下贴附本组）。 */
(function () {
  class AnGroupLabel extends window.AnElement {
    static tag = "an-group-label";
    static css = `
      :host { display: block; font-size: var(--t-meta); font-weight: 600; text-transform: uppercase;
        letter-spacing: 0; color: var(--ink-3); padding: var(--sp-2) var(--grid) var(--sp-1); }
    `;
    render() { return `<slot></slot>`; }
  }
  window.AnElement.define(AnGroupLabel);
})();
