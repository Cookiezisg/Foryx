/* Anselm 原语 B5 — <an-tabs>。文字下划线切换器：灰默认 / 选中黑 + 一条弹簧滑块（下划线 transform+width 跟随选中项）。
   懒建 + 隐藏不销毁：每个 pane 首次选中时才 render(paneEl) 建一次，之后切走只隐藏、不销毁——
     可编辑 tab 的内容/滚动/输入状态跨切换保留（修旧"切走即丢"的退化）。
   items 走 JS property（承载 render 回调 + 可选 count，无法塞进 attribute）：
     el.items = [{ key, label, count?, render(paneEl) }]；el.value = key（缺省取首项）。
   切换派发 composed 'an-pick'（detail.key）；下划线滑块是唯一动效（d-mid + ease-spring）。 */
(function () {
  class AnTabs extends window.AnElement {
    static tag = "an-tabs";
    static css = `
      :host { display: block; }
      :host([disabled]) { pointer-events: none; opacity: .4; }   /* 与 button/input 同语汇的禁用态 */
      .strip { position: relative; display: flex; gap: var(--sp-4); }
      /* 弹簧滑块：绝对定位贴底，transform+width 双轴跟随选中 tab（唯一动效） */
      .slider {
        position: absolute; left: 0; bottom: 0; width: 0; height: var(--line-2);
        background: var(--ink); border-radius: var(--r-pill);
        transition: transform var(--d-mid) var(--ease-spring), width var(--d-mid) var(--ease-spring);
      }
      .tab {
        position: relative; display: inline-flex; align-items: center; gap: var(--gap-tight);
        height: var(--tab-h); padding: 0 calc(var(--grid) / 2);
        color: var(--ink-3); font-size: var(--t-body); font-weight: 500; white-space: nowrap;
        transition: color var(--d-fast);
      }
      .tab:hover { color: var(--ink-2); }
      .tab.on { color: var(--ink); }
      /* 计数：选中项常驻的次级数字，tabular-nums 防跳动 */
      .count { font-size: var(--t-meta); color: var(--ink-3); font-variant-numeric: tabular-nums; }
      .tab.on .count { color: var(--ink-2); }
      .panes { padding-top: var(--sp-4); }
      .pane[hidden] { display: none; }
    `;
    render() {
      const e = window.anEsc;
      const items = this._items || [];
      const tabs = items.map((it) => {
        const cnt = (it.count != null && it.count !== "")
          ? `<span class="count">${e(it.count)}</span>` : "";
        return `<button class="tab" type="button" data-key="${e(it.key)}">${e(it.label)}${cnt}</button>`;
      }).join("");
      return `<div class="strip"><span class="slider"></span>${tabs}</div><div class="panes"></div>`;
    }
    hydrate() {
      const items = this._items || [];
      const strip = this.$(".strip");
      if (!strip) return;
      this.$$(".tab").forEach((b) => {
        b.addEventListener("click", () => this.select(b.dataset.key, true));
      });
      // 初选：当前 value 或首项（不派发——构建态非用户行为）
      const initial = (this._value != null && items.some((x) => x.key === this._value))
        ? this._value : (items[0] && items[0].key);
      if (initial != null) this.select(initial, false);
      // host 可能尚未布局完成（display:none / 异步插入）→ 下一帧补量滑块
      requestAnimationFrame(() => this._slide(false));
    }

    // ── items / value 走 property：承载 render 回调，attribute 装不下 ──
    set items(v) { this._items = Array.isArray(v) ? v : []; if (this.isConnected) this._render(); }
    get items() { return this._items || []; }
    set value(v) { this._value = v; if (this.isConnected) this.select(v, false); }
    get value() { return this._value; }

    select(key, fire) {
      const items = this._items || [];
      if (!items.some((x) => x.key === key)) return;
      this._value = key;
      this.$$(".tab").forEach((b) => b.classList.toggle("on", b.dataset.key === key));
      this._slide(true);
      // 懒建 + 隐藏不销毁：pane 首次选中才 render，之后只切显隐（状态保留）
      const panes = this.$(".panes");
      if (panes) {
        let pane = null;
        panes.querySelectorAll(".pane").forEach((p) => { if (p.dataset.key === key) pane = p; });
        if (!pane) {
          pane = document.createElement("div");
          pane.className = "pane";
          pane.dataset.key = key;
          const it = items.find((x) => x.key === key);
          if (it && typeof it.render === "function") it.render(pane);
          panes.appendChild(pane);
        }
        panes.querySelectorAll(".pane").forEach((p) => { p.hidden = (p !== pane); });
      }
      if (fire) this.emit("an-pick", { key });
    }

    // 下划线滑块落位：宽度/水平偏移取自选中 tab；首帧无动画（防加载抖）
    _slide(animate) {
      const slider = this.$(".slider");
      const on = this.$(".tab.on");
      if (!slider || !on) return;
      if (!animate) slider.style.transition = "none";
      slider.style.width = on.offsetWidth + "px";
      slider.style.transform = `translateX(${on.offsetLeft}px)`;
      if (!animate) { void slider.offsetWidth; slider.style.transition = ""; }
    }
  }
  window.AnElement.define(AnTabs);
})();
