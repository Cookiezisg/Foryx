/* Anselm 原语 D6 — <an-toolbar title meta compact>。三段工具条：left | main | right。
   局部工具面与页面动作条共用——无边、无卡，只提供动作与标题的对齐骨架。
   why：工具条不是卡（不造边界），只是“左附件 / 主体 / 右动作”三格对齐网格，去除页面各处手摆 flex。
   解剖：grid [auto | 1fr | auto]，左右两段 flex 收附件、main 居中收标题或自定义内容。
   槽：slot[name=left] 放前缀附件；默认 slot = main（title/meta 缺省时承载自定义 body）；slot[name=right] 放 <an-action-group>。
   title/meta 属性给定时渲染标准“标题 + 次级 meta”，否则 main 走默认 slot。 */
(function () {
  class AnToolbar extends window.AnElement {
    static tag = "an-toolbar";
    static observed = ["title", "meta", "compact", "bordered"];
    static css = `
      :host { display: block; }
      /* [bordered]：作顶部工具栏——底描边 + 内边距 + island 底（页/编辑器顶栏 compose 本件，不再手摆 flex+border） */
      :host([bordered]) { border-bottom: var(--hairline) solid var(--line); background: var(--island); }
      :host([bordered]) .toolbar { padding: var(--sp-2) var(--sp-4); }
      .toolbar {
        display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: var(--sp-2);
        min-height: var(--row); min-width: 0; color: var(--ink-2); font-size: var(--t-body);
      }
      :host([compact]) .toolbar { min-height: var(--ctl); }
      .left, .right {
        display: flex; align-items: center; gap: var(--sp-2); min-width: 0; flex-wrap: wrap;
      }
      .right { justify-content: flex-end; }
      .main {
        min-width: 0; display: inline-flex; align-items: center; gap: var(--sp-2); overflow: hidden;
      }
      .title {
        min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        color: var(--ink); font-weight: 600; line-height: var(--lh-ui);
      }
      .meta {
        flex: none; color: var(--ink-3); font-size: var(--t-meta); line-height: var(--lh-ui);
      }
    `;
    render() {
      const e = window.anEsc;
      let main;
      if (this.attr("title") || this.attr("meta")) {
        const title = this.attr("title") ? `<span class="title">${e(this.attr("title"))}</span>` : "";
        const meta = this.attr("meta") ? `<span class="meta">${e(this.attr("meta"))}</span>` : "";
        main = title + meta;
      } else {
        main = `<slot></slot>`;
      }
      return `<div class="toolbar">`
        + `<div class="left"><slot name="left"></slot></div>`
        + `<div class="main">${main}</div>`
        + `<div class="right"><slot name="right"></slot></div>`
        + `</div>`;
    }
  }
  window.AnElement.define(AnToolbar);
})();
