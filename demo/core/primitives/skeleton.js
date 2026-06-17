/* Anselm 原语 F6 — <an-skeleton variant count>。加载骨架占位：灰条 + shimmer 微动。
   variant ∈ row(默认) | card | text | lines；count = 重复条数（默认 1，lines 默认 3）。
   铁律 layout-shift 0：row 变体【复用 Row 的度量】——同 [行首槽 --lead | 1fr | 尾槽 --trail] 网格 + --row 行高，
   故骨架与实态 <an-row> 逐像素同形，加载完替换不跳动。色走 island 阶梯、shimmer 走 token 化渐变（时长 --d-slow 级）。 */
(function () {
  class AnSkeleton extends window.AnElement {
    static tag = "an-skeleton";
    static observed = ["variant", "count"];
    static css = `
      :host { display: block; }

      /* 灰条基元：island-3 底，shimmer 由 island-4 高光横扫；圆角随条粗细取 tag 级 */
      .bar {
        border-radius: var(--r-tag);
        background: linear-gradient(100deg, var(--island-3) 30%, var(--island-4) 50%, var(--island-3) 70%);
        background-size: 200% 100%;
        animation: an-skel-shimmer calc(var(--d-slow) * 4) var(--ease-out) infinite;
      }
      @keyframes an-skel-shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }

      /* ── row：镜像 <an-row> 网格（--lead | 1fr | --trail），--row 行高 → 占位/实态同形 ── */
      .skel-row {
        display: grid; grid-template-columns: var(--lead) 1fr var(--trail); align-items: center; column-gap: var(--gap);
        height: var(--row); padding: 0 var(--pad-row);
      }
      .skel-row .lead { width: var(--lead); height: var(--lead); border-radius: var(--r-pill); }   /* 行首槽：圆点/图标占位 */
      .skel-row .label { height: var(--t-body); width: 60%; }
      .skel-row .meta  { height: var(--t-meta); width: 100%; }
      .skel-row + .skel-row { margin-top: var(--grid); }

      /* ── text：单行正文条（行高 --t-body，宽随 width 属性，默认 100%） ── */
      .skel-text { height: var(--t-body); width: 100%; }

      /* ── lines：多行段落，末行收窄模拟自然断句；行距走 --gap ── */
      .skel-lines { display: flex; flex-direction: column; gap: var(--gap); }
      .skel-lines .bar { height: var(--t-body); }
      .skel-lines .bar:last-child { width: 70%; }

      /* ── card：信息卡占位（头 icon-sm 条 + 两行体），与 info-card 留白同节奏 ── */
      .skel-card { display: flex; flex-direction: column; gap: var(--sp-2); padding: var(--sp-1) var(--sp-2); }
      .skel-card .ch { height: var(--t-body); width: 40%; }
      .skel-card .cb { height: var(--t-body); width: 100%; }
      .skel-card .cb.short { width: 80%; }
    `;
    render() {
      const variant = this.attr("variant", "row");
      if (variant === "card") return this._card();
      if (variant === "text") return this._text();
      if (variant === "lines") return this._lines();
      return this._rows();
    }

    _rows() {
      const n = this.num("count", 1);
      let out = "";
      for (let i = 0; i < n; i++) {
        out += `<div class="skel-row">`
          + `<span class="bar lead"></span>`
          + `<span class="bar label"></span>`
          + `<span class="bar meta"></span>`
          + `</div>`;
      }
      return out;
    }
    _text() {
      const n = this.num("count", 1);
      let out = "";
      for (let i = 0; i < n; i++) out += `<div class="bar skel-text"></div>`;
      return out;
    }
    _lines() {
      const n = this.num("count", 3);
      let bars = "";
      for (let i = 0; i < n; i++) bars += `<span class="bar"></span>`;
      return `<div class="skel-lines">${bars}</div>`;
    }
    _card() {
      const n = this.num("count", 1);
      let out = "";
      for (let i = 0; i < n; i++) {
        out += `<div class="skel-card">`
          + `<span class="bar ch"></span>`
          + `<span class="bar cb"></span>`
          + `<span class="bar cb short"></span>`
          + `</div>`;
      }
      return out;
    }
  }
  window.AnElement.define(AnSkeleton);
})();
