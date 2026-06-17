/* Anselm 原语 B3 — <an-dropdown>：受控单选下拉（替代原生 select）。
   解剖：当前值按钮（回显选中 label + 可选 mono meta + caret）→ 点开 window.AnMenu 富行弹层
     （label / meta / 可选 icon / 勾选当前）。弹层走命令式浮层（body 顶层逃 shadow 裁剪、复用 Row 密度）。
   为何弹层不自建：菜单原语已是“一组富行的 Floating”——勾选/meta/icon 全在 AnMenu 里，重造即违复用铁律。
   property（数组/对象只能走 property）：options=[{value,label,meta?,icon?}] · value。
   value 同步镜像为 attribute → 外部改值即重绘按钮回显。
   交互：选中派发 composed CustomEvent 'an-change'{value}。壳可选用 <an-field label> 包外层。 */
(function () {
  class AnDropdown extends window.AnElement {
    static tag = "an-dropdown";
    static observed = ["value", "disabled", "block"];
    static css = `
      :host { display: inline-block; }
      :host([block]) { display: block; }
      :host([disabled]) { pointer-events: none; }
      .dd {
        display: flex; align-items: center; gap: var(--gap);
        width: 100%; min-width: var(--input-min); height: var(--ctl);
        padding: 0 var(--btn-pad-x-sm); text-align: left;
        background: var(--island); border: var(--hairline) solid var(--line); border-radius: var(--r-btn);
        color: var(--ink); font-size: var(--t-body);
        transition: border-color var(--d-fast), background var(--d-fast);
      }
      .dd:hover, :host([open]) .dd { border-color: var(--line-strong); }
      :host([disabled]) .dd { opacity: .4; }
      /* label + meta 走基线对齐（非居中）——大小不同的字也落同一条字底线，杜绝肉眼可见的 0.x px 错位 */
      .main { flex: 1; min-width: 0; display: flex; align-items: baseline; gap: var(--gap); }
      .lab { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .lab.is-placeholder { color: var(--ink-3); }
      .meta {
        flex: none; font-size: var(--t-meta); color: var(--ink-3);
        font-family: var(--mono); font-variant-numeric: tabular-nums;
      }
      .caret {
        flex: none; display: grid; place-items: center; color: var(--ink-3);
        transition: transform var(--d-fast) var(--ease-spring);
      }
      .caret svg { width: var(--icon-sm); height: var(--icon-sm); }
      :host([open]) .caret { transform: rotate(180deg); }   /* 展开 = caret 翻转 */
    `;
    render() {
      const e = window.anEsc;
      const sel = this._selected();
      const lab = sel
        ? `<span class="lab">${e(sel.label)}</span>`
        : `<span class="lab is-placeholder">${e(this.attr("placeholder", "—"))}</span>`;
      const meta = sel && sel.meta ? `<span class="meta">${e(sel.meta)}</span>` : "";
      const caret = `<span class="caret">${window.icon("chevd")}</span>`;
      return `<button class="dd" type="button" part="button"><span class="main">${lab}${meta}</span>${caret}</button>`;
    }
    hydrate() {
      const btn = this.$(".dd");
      btn.addEventListener("click", (ev) => {
        ev.stopPropagation();
        if (this.has("disabled")) return;
        this._toggle(btn);
      });
    }

    // ── property 入口：数组/对象只能经 property（attribute 装不下）──
    set options(v) { this._options = Array.isArray(v) ? v : []; if (this.isConnected) this._render(); }
    get options() { return this._options || []; }
    // value 经 property 设亦镜像 attribute（统一走 observed → 触发重绘回显）
    set value(v) { v == null ? this.removeAttribute("value") : this.setAttribute("value", v); }
    get value() { return this.attr("value"); }

    _selected() {
      const cur = this.attr("value");
      const opts = this.options;
      return opts.find((o) => String(o.value) === String(cur)) || null;
    }

    _toggle(anchor) {
      const ns = "dropdown-" + this._ns();
      if (this.has("open")) { window.AnFloating && window.AnFloating.close(ns); return; }
      const cur = this.attr("value");
      const items = this.options.map((o) => ({
        value: o.value,
        label: o.label,
        meta: o.meta,
        icon: o.icon,                              // 富行可选 leading icon
        checked: String(o.value) === String(cur), // 勾选当前（AnMenu 无 icon 时落 check）
      }));
      this.setAttribute("open", "");
      window.AnMenu.open(anchor, {
        items,
        namespace: ns,                             // 每实例独占 ns → 多个 dropdown 并存不串台
        align: "start",
        placement: "bottom",
        onPick: (value) => { this.value = value; this.emit("an-change", { value }); },
        onClose: () => this.removeAttribute("open"),   // 浮层销毁（点外/Escape/选中）→ 复位 caret，复用地基 onClose
      });
    }

    _ns() {
      if (!this.__ns) this.__ns = Math.random().toString(36).slice(2, 8);
      return this.__ns;
    }
  }
  window.AnElement.define(AnDropdown);
})();
