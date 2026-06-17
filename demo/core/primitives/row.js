/* Anselm 原语 C1 — <an-row>（核心）。唯一的“一行”：承载会话/实体/workflow/文档树/通知/设置类目。
   解剖（模版铁律）：三列网格 [行首槽 --lead | 标签 1fr | 尾槽 --trail 锚位]，对齐由结构保证、绝不手量。
     · 行首槽：dot / icon↔chevron 叠放居中 → 7px 点与 16px 图标同心。
     · 尾槽：meta 文本与 action 按钮叠放同一隐形锚位、右边缘锚定 → hover 互换不重排、右端成线。
   属性：icon | dot | label | hint | meta | selected | collapsible | open | passive | depth。
   动作：具名 slot[name=actions] 放 <an-button variant="icon">；行选中派发 CustomEvent 'an-select'。
   两个同槽互换 + 字色铁律全焊进皮肤；opacity 互换即时（不入过渡）。 */
(function () {
  class AnRow extends window.AnElement {
    static tag = "an-row";
    static observed = ["icon", "dot", "label", "hint", "meta", "selected", "collapsible", "open", "passive", "depth"];
    static css = `
      :host { display: block; }
      .row {
        position: relative;
        display: grid; grid-template-columns: var(--lead) 1fr auto; align-items: center; column-gap: var(--gap);
        height: var(--row); padding: 0 var(--pad-row); border-radius: var(--r-btn);
        color: var(--ink-2); font-size: var(--t-body); cursor: pointer;
        transition: background var(--d-fast);
      }
      :host([hint]) .row { height: auto; min-height: var(--field-row); padding-top: var(--sp-1); padding-bottom: var(--sp-1); }
      :host(:hover) .row { background: var(--island-3); color: var(--ink); }
      :host([selected]) .row { background: var(--island-4); color: var(--ink); }
      /* emphatic：选中态走 accent 强调（软底 + 左 inset accent 条）——列表型选中（如 run 看板）复用，不再各处另起炉灶 */
      :host([emphatic][selected]) .row { background: var(--accent-soft); color: var(--ink); box-shadow: inset var(--line-2) 0 0 var(--accent); }
      /* mono：标签等宽（run id / hash / 引用名等 tabular 文本） */
      :host([mono]) .label { font-family: var(--mono); }
      :host([passive]) .row { cursor: default; }
      :host([passive]:hover) .row { background: transparent; color: var(--ink-2); }

      .lead { width: var(--lead); height: var(--lead); display: grid; }
      .lead > * { grid-area: 1 / 1; place-self: center; color: var(--ink-3); }
      .ico, .chev { display: grid; place-items: center; }
      .ico svg, .chev svg { width: var(--icon); height: var(--icon); }
      :host([collapsible]) .chev { opacity: 0; transition: transform var(--d-mid) var(--ease-spring); }
      :host([collapsible]:hover) .ico  { opacity: 0; }
      :host([collapsible]:hover) .chev { opacity: 1; }
      :host([collapsible][open]) .chev { transform: rotate(90deg); }

      .main { min-width: 0; display: grid; gap: calc(var(--grid) / 2); }
      .label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      :host([hint]) .label { color: var(--ink); }
      /* hint 多行自适应：label 恒单行（导航/列表契约），hint 作"说明值"可换行（承载长机制/描述，不再截断挤压） */
      .hint { min-width: 0; overflow-wrap: anywhere;
        color: var(--ink-3); font-size: var(--t-meta); line-height: var(--lh-ui); }
      /* 多行 hint 行：整行顶对齐——lead 图标/点与 label 首行对齐，不随多行体垂直居中漂移 */
      :host([hint]) .row { align-items: start; }
      :host([hint]) .trail { align-items: start; }

      .trail { display: grid; justify-items: end; align-items: center; }
      .trail > * { grid-area: 1 / 1; }
      .meta {
        display: inline-flex; align-items: center; justify-content: flex-end;
        min-width: var(--trail); height: var(--trail);
        padding: 0 var(--trail-inset) 0 var(--sp-2);
        font-size: var(--t-meta); color: var(--ink-3); white-space: nowrap; font-variant-numeric: tabular-nums;
      }
      :host([has-acts]:hover) .meta { opacity: 0; }
      .acts { display: flex; align-items: center; justify-content: flex-end; gap: 0; opacity: 0; }
      :host(:hover) .acts { opacity: 1; }
    `;
    render() {
      const e = window.anEsc;
      let lead;
      if (this.attr("dot")) {
        lead = `<an-status-dot state="${e(this.attr("dot"))}"></an-status-dot>`;
      } else {
        const ic = this.attr("icon") ? `<span class="ico">${window.icon(this.attr("icon"))}</span>` : "";
        const chev = this.has("collapsible") ? `<span class="chev">${window.icon("chevr")}</span>` : "";
        lead = ic + chev;
      }
      const main = this.attr("hint")
        ? `<span class="main"><span class="label">${e(this.attr("label"))}</span><span class="hint">${e(this.attr("hint"))}</span></span>`
        : `<span class="label">${e(this.attr("label"))}</span>`;
      const meta = (this.attr("meta") != null && this.attr("meta") !== "") ? `<span class="meta">${e(this.attr("meta"))}</span>` : "";
      const trail = `<span class="trail">${meta}<span class="acts"><slot name="actions"></slot></span></span>`;
      const depth = this.num("depth", 0);
      const pad = depth ? ` style="padding-left: calc(var(--pad-row) + ${depth} * var(--indent))"` : "";
      return `<div class="row"${pad}><span class="lead">${lead}</span>${main}${trail}</div>`;
    }
    hydrate() {
      this.toggleAttribute("has-acts", !!this.querySelector('[slot="actions"]'));
      const select = () => { if (!this.has("passive")) this.emit("an-select"); };
      const lead = this.$(".lead"), main = this.$(".main") || this.$(".label");
      // 树节点（collapsible 且非 passive）：点行首槽 chevron → an-toggle（仅折叠）、点标题 → an-select（打开）——分流"折叠 vs 选中"。
      // 非 collapsible / passive 行为不变（行首槽与标题同 an-select；passive 不派）。
      const onLead = () => { if (this.has("collapsible") && !this.has("passive")) this.emit("an-toggle"); else select(); };
      if (lead) lead.addEventListener("click", onLead);
      if (main) main.addEventListener("click", select);
    }
  }
  window.AnElement.define(AnRow);
})();
