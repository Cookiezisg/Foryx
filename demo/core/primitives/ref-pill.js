/* Anselm 原语 A3 — <an-ref-pill kind id label>。实体提及药丸 = 类型图标 + 文案，可点。
   why：收掉 chat 海洋散落的 refPill 模板 + data-ent→openEntity 各处副本，归一成一个前门——
   点击对外派 composed 'an-ref'{kind,id}，由装配层转 Intent.select（原语不直接碰任何 openEntity）。
   图标优先取 ENTITY_KINDS[kind].icon（9 类实体）；未登记则把 kind 当 icons.js key 兜底（doc/search 等纯提及）。
   id 非空才算可点坐标——空 id = 纯标注、不显手型、不派发。 */
(function () {
  class AnRefPill extends window.AnElement {
    static tag = "an-ref-pill";
    static observed = ["kind", "id", "label"];
    static css = `
      :host { display: inline-flex; vertical-align: var(--hairline); }
      /* 与 <an-tags> 的 chip 同款：小（t-meta）· 白底（island）· 海岸线描边——不喧宾夺主 */
      .pill {
        display: inline-flex; align-items: center; gap: var(--grid);
        padding: calc(var(--grid) / 2) var(--gap-tight);
        border-radius: var(--r-pill); border: var(--hairline) solid var(--line);
        background: var(--island); color: var(--ink-2);
        font-size: var(--t-meta); font-weight: 500; white-space: nowrap;
        transition: background var(--d-fast), color var(--d-fast);
      }
      :host([id]) .pill { cursor: pointer; }
      :host([id]) .pill:hover { background: var(--island-3); color: var(--ink); }
      .ico { display: grid; place-items: center; color: var(--ink-3); }
      .ico svg { width: var(--icon-sm); height: var(--icon-sm); }
    `;
    render() {
      const e = window.anEsc;
      const kind = this.attr("kind", "");
      const K = window.ENTITY_KINDS || {};
      const ico = (K[kind] && K[kind].icon) || kind;
      return `<span class="pill"><span class="ico">${window.icon(ico, 12)}</span>${e(this.attr("label", ""))}</span>`;
    }
    hydrate() {
      // 仅有 id（提及坐标）才挂点击 → 一个前门派 'an-ref'{kind,id}；装配层转 Intent.select
      const id = this.attr("id");
      if (id == null || id === "") return;
      this.$(".pill").addEventListener("click", () => this.emit("an-ref", { kind: this.attr("kind"), id }));
    }
  }
  window.AnElement.define(AnRefPill);
})();
