/* Anselm 原语 D2 — <an-info-card title icon meta>。无边信息单元：靠标题/留白/层级组织内容，不靠横线切割。
   head（icon + title + meta）仅在有 title/icon/meta 时出现；body 走默认 slot；动作走具名 slot[name=actions]。
   <an-info-card title="Schedule" icon="clock" meta="UTC"><an-row …></an-row><an-button slot="actions">Edit</an-button></an-info-card>。 */
(function () {
  class AnInfoCard extends window.AnElement {
    static tag = "an-info-card";
    static observed = ["title", "icon", "meta"];
    static css = `
      :host { display: block; }
      .card { padding: var(--sp-1) var(--sp-2); border-radius: var(--r-btn); }
      .head { display: flex; align-items: center; gap: var(--gap); min-height: var(--ctl); margin-bottom: var(--sp-1); }
      .ico { display: grid; place-items: center; flex: none; color: var(--ink-3); }
      .ico svg { width: var(--icon-sm); height: var(--icon-sm); }
      .title { flex: 1; min-width: 0; color: var(--ink-3); font-size: var(--t-meta); font-weight: 600; line-height: var(--lh-ui); }
      /* flex:0 1 auto + min-width:0 + 省略：常态贴内容、超长可缩并截断，绝不把 head 撑破 */
      .meta { flex: 0 1 auto; min-width: 0; display: inline-flex; align-items: center; gap: var(--gap-tight); color: var(--ink-3); font-size: var(--t-meta); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
      /* 间距归容器：卡内块间节奏（比段内更紧） */
      .body { min-width: 0; display: flex; flex-direction: column; gap: var(--sp-2); }
      .actions { display: flex; align-items: center; gap: var(--sp-2); margin-top: var(--sp-3); }
      /* 无动作时塌掉 actions 行的上边距占用 */
      .actions:not(:has(*)) { display: none; }
    `;
    render() {
      const e = window.anEsc;
      const hasIcon = !!this.attr("icon");
      const hasTitle = this.attr("title") != null && this.attr("title") !== "";
      const hasMeta = this.attr("meta") != null && this.attr("meta") !== "";
      const ic = hasIcon ? `<span class="ico">${window.icon(this.attr("icon"), 12)}</span>` : "";
      const title = hasTitle ? `<span class="title">${e(this.attr("title"))}</span>` : "";
      const meta = hasMeta ? `<span class="meta">${e(this.attr("meta"))}</span>` : "";
      const head = (hasIcon || hasTitle || hasMeta) ? `<div class="head">${ic}${title}${meta}</div>` : "";
      return `<section class="card">${head}<div class="body"><slot></slot></div><div class="actions"><slot name="actions"></slot></div></section>`;
    }
  }
  window.AnElement.define(AnInfoCard);
})();
