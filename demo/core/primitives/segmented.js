/* Anselm 原语 B4 — <an-segmented>。灰药丸分段切换器：白轨道 + 海岸线，一颗灰胶囊在选项间弹簧滑动（唯一动效）。
   段宽由文案自适应，pill 用 transform+width 跟随选中项；选中文案从 ink-2 提到 ink。
   items 走 JS property（统一与 an-tabs 一致）：el.items = ['A','B'] 或 [{ value, label }]；el.value = 初选 value（缺省首项）。
   键盘 roving：←/→ 在选项间循环移焦并选中（tablist 自动激活语义）。切换派发 composed 'an-pick'（detail.value/index）。 */
(function () {
  class AnSegmented extends window.AnElement {
    static tag = "an-segmented";
    static css = `
      :host { display: inline-flex; }
      :host([disabled]) { pointer-events: none; opacity: .4; }   /* 与 button/input 同语汇的禁用态 */
      .seg {
        position: relative; display: inline-flex; gap: var(--gap-hair); padding: var(--focus-ring);
        background: var(--island-3); border: var(--hairline) solid var(--line); border-radius: var(--r-pill);
      }
      /* 滑动胶囊：绝对定位、压在按钮层下，transform+width 双轴弹簧过渡（唯一动效） */
      .pill {
        position: absolute; left: 0; top: 0; width: 0; height: 0; z-index: 0; border-radius: var(--r-pill);
        background: var(--island-4);
        transition: transform var(--d-mid) var(--ease-spring), width var(--d-mid) var(--ease-spring);
      }
      .btn {
        position: relative; z-index: 1; display: inline-flex; align-items: center; justify-content: center;
        gap: var(--gap-tight); height: var(--row); padding: 0 var(--btn-pad-x); border-radius: var(--r-pill);
        font-size: var(--t-body); font-weight: 600; color: var(--ink-2); white-space: nowrap;
        transition: color var(--d-mid) var(--ease-spring);
      }
      .btn.on { color: var(--ink); }
    `;
    render() {
      const e = window.anEsc;
      const items = this._items || [];
      const btns = items.map((it, i) =>
        `<button class="btn" type="button" role="tab" data-i="${i}" data-v="${e(it.value)}"
           tabindex="-1" aria-selected="false"><span>${e(it.label)}</span></button>`
      ).join("");
      return `<div class="seg" role="tablist"><span class="pill"></span>${btns}</div>`;
    }
    hydrate() {
      const seg = this.$(".seg");
      if (!seg) return;
      this.$$(".btn").forEach((b) => {
        b.addEventListener("click", () => this.select(parseInt(b.dataset.i, 10), false, true));
      });
      // roving：←/→ 循环移焦并选中（段控选中即焦点）
      seg.addEventListener("keydown", (ev) => {
        if (ev.key !== "ArrowLeft" && ev.key !== "ArrowRight") return;
        ev.preventDefault();
        const n = (this._items || []).length;
        if (!n) return;
        const next = ev.key === "ArrowRight" ? (this._idx + 1) % n : (this._idx - 1 + n) % n;
        this.select(next, true, true);
      });
      // 初选：当前 value 或首项（不派发——构建态）
      const items = this._items || [];
      let idx = items.findIndex((x) => x.value === this._value);
      if (idx < 0) idx = 0;
      this.select(idx, false, false);
      // host 可能尚未布局完成 → 下一帧补量落位
      requestAnimationFrame(() => this._place(false));
    }

    // ── items / value 走 property（与 an-tabs 一致）──
    set items(v) {
      this._items = (Array.isArray(v) ? v : []).map((o) =>
        typeof o === "string" ? { value: o, label: o } : { value: o.value, label: o.label != null ? o.label : o.value }
      );
      if (this.isConnected) this._render();
    }
    get items() { return this._items || []; }
    set value(v) {
      this._value = v;
      if (this.isConnected) {
        const idx = (this._items || []).findIndex((x) => x.value === v);
        if (idx >= 0) this.select(idx, false, false);
      }
    }
    get value() { return this._value; }

    select(i, focus, fire) {
      const items = this._items || [];
      if (i < 0 || i >= items.length) return;
      this._idx = i;
      this._value = items[i].value;
      this.$$(".btn").forEach((b, j) => {
        const on = j === i;
        b.classList.toggle("on", on);
        b.setAttribute("aria-selected", on ? "true" : "false");
        b.tabIndex = on ? 0 : -1;
      });
      this._place(true);
      if (focus) { const b = this.$$(".btn")[i]; if (b) b.focus(); }
      if (fire) this.emit("an-pick", { value: items[i].value, index: i });
    }

    // pill 落位：宽高取自选中按钮、transform 跟随其 offset；首帧无动画（防加载抖）
    _place(animate) {
      const pill = this.$(".pill");
      const b = this.$$(".btn")[this._idx];
      if (!pill || !b) return;
      if (!animate) pill.style.transition = "none";
      pill.style.width = b.offsetWidth + "px";
      pill.style.height = b.offsetHeight + "px";
      pill.style.transform = `translate(${b.offsetLeft}px, ${b.offsetTop}px)`;
      if (!animate) { void pill.offsetWidth; pill.style.transition = ""; }
    }
  }
  window.AnElement.define(AnSegmented);
})();
