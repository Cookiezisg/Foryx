/* Anselm 原语 G5 — <an-kind-legend>。图节点 5 类的只读颜色图例（色点 + 类名），自 window.AnGraph.{KIND_ORDER,KIND} 取数、零属性。
   why 内化：原图编辑器 rail 手搓 flex-wrap + 每点内联 cssText + 拼接颜色——图例是可复用只读 viz（rail / reference 画廊同用），收进皮肤。
   每点颜色是 per-kind 数据（KIND[k].c，单源自图配置），随数据内联到 background；结构/间距/字色全在皮肤。 */
(function () {
  class AnKindLegend extends window.AnElement {
    static tag = "an-kind-legend";
    static css = `
      :host { display: flex; flex-wrap: wrap; gap: var(--sp-2) var(--sp-3); font-size: var(--t-meta); color: var(--ink-3); }
      .item { display: inline-flex; align-items: center; gap: var(--gap-tight); }
      .dot { flex: none; width: var(--dot); height: var(--dot); border-radius: var(--r-pill); }
    `;
    render() {
      const G = window.AnGraph || {}, KIND = G.KIND || {}, ORDER = G.KIND_ORDER || [];
      return ORDER.map((k) => {
        const c = (KIND[k] || {}).c || "var(--ink-3)";   // per-kind 颜色：单源自图配置 KIND[k].c
        const label = window.anEsc((KIND[k] || {}).label || k);
        return `<span class="item"><span class="dot" style="background:${c}"></span>${label}</span>`;
      }).join("");
    }
  }
  window.AnElement.define(AnKindLegend);
})();
