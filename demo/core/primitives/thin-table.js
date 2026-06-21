/* Anselm 原语 C4 — <an-thin-table>。结构化多列展示——借鉴 Field·KV 的洁净键值语汇，做成对齐多列、非表格。
   解剖：单层 CSS 网格（列轨由 columns 定义），每行用 subgrid 继承父列轨 → 跨行严格对齐、不靠手量。
     无表格 chrome（无粗头线、无行分隔线、无斑马）；表头 = 灰 meta 列名，数据行 --row 行高、留白成行（同 KV 靠字色/留白分层）。
   密度复用 Row：列间 --sp-4、行高 --row。selectable → 行 hover 提墨 + 选中底。
   数据走 .columns / .rows 属性（结构化不经线缆）：columns=[{key,label,align?}]（align∈left|right|center）· rows=[{key:value,…}]。
   动作：selectable 时点行派发 composed 'an-row-click'{row}（row = 原始行对象）。 */
(function () {
  class AnThinTable extends window.AnElement {
    static tag = "an-thin-table";
    static observed = ["selectable"];
    static css = `
      :host { display: block; }
      /* 单网格：列轨在 .grid 上一次定义；每行 subgrid 继承之 → 列跨行对齐（列间距 / 行内边距由父继承） */
      .grid { display: grid; row-gap: var(--zero); column-gap: var(--sp-4); }
      .tr {
        display: grid; grid-column: 1 / -1; grid-template-columns: subgrid;
        align-items: center; min-height: var(--row); padding: 0 var(--pad-row); border-radius: var(--r-btn);
      }

      /* 表头：灰 meta 列名、无线（借鉴 KV——靠字色与留白分层，不靠分隔线） */
      .tr.head { min-height: var(--ctl-sm); align-items: end; }
      .th {
        min-width: 0; color: var(--ink-3); font-size: var(--t-meta); font-weight: 600;
        line-height: var(--lh-ui); white-space: nowrap; padding-bottom: var(--grid);
      }

      /* 数据格：首列主值 ink、其余次级 ink-2 等宽数；顶对齐留白成行 */
      .td {
        min-width: 0; color: var(--ink); font-size: var(--t-body); line-height: var(--lh-ui);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
      }
      .td:not(:first-child) { color: var(--ink-2); font-variant-numeric: tabular-nums; }
      .right  { justify-self: end; text-align: right; }
      .center { justify-self: center; text-align: center; }

      /* 可点行：hover 提墨 + 选中底（subgrid 行即一个可命中单元，整条高亮） */
      :host([selectable]) .tr.row { cursor: pointer; transition: background var(--d-fast); }
      :host([selectable]) .tr.row:hover { background: var(--island-3); }
      :host([selectable]) .tr.row:hover .td { color: var(--ink); }
      :host([selectable]) .tr.row.on { background: var(--island-4); }
      :host([selectable]) .tr.row.on .td { color: var(--ink); }
    `;

    // 列定义 / 行数据走 JS 属性（结构化数据不经线缆字符串）；设入即重渲染
    get columns() { return this._columns || []; }
    set columns(v) { this._columns = Array.isArray(v) ? v : []; if (this.isConnected) this._render(); }
    get rows() { return this._rows || []; }
    set rows(v) { this._rows = Array.isArray(v) ? v : []; if (this.isConnected) this._render(); }

    _alignCls(a) { return a === "right" ? " right" : a === "center" ? " center" : ""; }

    render() {
      const e = window.anEsc;
      const cols = this._columns || [];
      const rows = this._rows || [];
      // 列轨：首列吃富余 minmax(0,1fr)，其余 minmax(0,auto) 贴内容但【可缩】——裸 auto 无下限会被超长值撑破整表、ellipsis 失效；minmax(0,auto) 让非首列也能压缩截断
      const tracks = cols.map((c, i) => (i === 0 ? "minmax(0, 1fr)" : "minmax(0, auto)")).join(" ");

      const head = cols
        .map((c) => `<span class="th${this._alignCls(c.align)}">${e(c.label != null ? c.label : c.key)}</span>`)
        .join("");

      const body = rows
        .map((r) => {
          const cells = cols
            .map((c) => {
              const val = r == null ? "" : r[c.key];
              return `<span class="td${this._alignCls(c.align)}">${e(val == null ? "" : val)}</span>`;
            })
            .join("");
          return `<div class="tr row">${cells}</div>`;
        })
        .join("");

      return `<div class="grid" style="grid-template-columns: ${tracks}"><div class="tr head">${head}</div>${body}</div>`;
    }

    hydrate() {
      // 可点行：行 → 选中单选 + 对外 composed an-row-click{row}（携原始行对象）
      if (!this.has("selectable")) return;
      const trs = this.$$(".tr.row");
      trs.forEach((tr, i) => {
        tr.addEventListener("click", () => {
          trs.forEach((x) => x.classList.remove("on"));
          tr.classList.add("on");
          this.emit("an-row-click", { row: (this._rows || [])[i] });
        });
      });
    }
  }
  window.AnElement.define(AnThinTable);
})();
