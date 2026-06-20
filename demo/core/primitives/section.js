/* Anselm 原语 D1 — <an-section label variant grid>。段：小节标题 + 无边内容区，默认不靠横线分割。
   variant 缺省 = meta 大写灰小标签；variant="plain" = 文档型海洋的字号标题（靠留白分层）。
   grid（布尔）= body 转响应式 2 列网格（auto-fit minmax(--w-block,1fr)，窄了自动塌 1 列）——内化原 render.js 手搓的网格容器。
   内容走默认 slot；标题右侧动作走 slot="actions"（如设置段的「添加」钮）。 */
(function () {
  class AnSection extends window.AnElement {
    static tag = "an-section";
    static observed = ["label", "variant", "grid"];
    static css = `
      :host { display: block; margin-bottom: var(--sp-6); }
      :host([variant="plain"]) { margin-bottom: var(--sp-8); }
      /* 标题行：label 左 + actions 右（无 actions 时 label 独占，观感同旧） */
      .head { display: flex; align-items: center; justify-content: space-between; gap: var(--gap); margin: 0 calc(var(--grid) / 2) var(--sp-2); }
      :host([variant="plain"]) .head { margin: 0 0 var(--sp-3); }
      .label {
        font-size: var(--t-meta); font-weight: 600; text-transform: uppercase;
        color: var(--ink-3); line-height: var(--lh-ui); margin: var(--zero); min-width: var(--zero);
      }
      :host([variant="plain"]) .label {
        font-size: var(--t-strong); text-transform: none; color: var(--ink); line-height: var(--lh-tight);
      }
      /* 间距归容器：段内块间统一节奏（子件不自管外边距）。 */
      .body { display: flex; flex-direction: column; gap: var(--sp-3); }
      /* grid：响应式 2 列块网格（实体页「输入/输出」「环境」等并排卡片段） */
      :host([grid]) .body { display: grid; grid-template-columns: repeat(auto-fit, minmax(var(--w-block), 1fr)); gap: var(--sp-4); align-items: start; }
    `;
    render() {
      const head = this.attr("label")
        ? `<div class="head"><div class="label">${window.anEsc(this.attr("label"))}</div><slot name="actions"></slot></div>`
        : "";
      return `${head}<div class="body"><slot></slot></div>`;
    }
  }
  window.AnElement.define(AnSection);
})();
