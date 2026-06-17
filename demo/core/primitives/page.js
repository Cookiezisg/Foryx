/* Anselm 原语 D3 — <an-page>。记录页骨架：居中 --w-content 列 + 唯一滚动区 + 浮动 overlay 滚轮。
   why：滚轮走 overlay thumb（rAF 节流 + 空闲隐藏），不占 native gutter——内容列宽不随滚动条出现而抖。
   内容走默认 slot（header/tabs/sections 全塞进来）；滚轮 thumb 由 hydrate() 命令式驱动。 */
(function () {
  class AnPage extends window.AnElement {
    static tag = "an-page";
    static observed = [];
    static css = `
      :host { display: block; }
      .page { position: relative; flex: 1; min-height: 0; overflow: hidden; height: 100%; }
      .scroll {
        height: 100%; min-height: 0; overflow-y: auto;
        scrollbar-width: none; -ms-overflow-style: none;
      }
      .scroll::-webkit-scrollbar { width: 0; height: 0; }
      .col { max-width: var(--w-content); margin: 0 auto; padding: var(--sp-6) var(--sp-6) var(--sp-12); }
      .scrollbar {
        position: absolute; top: var(--sp-2); right: calc(var(--grid) / 2); bottom: var(--sp-2);
        display: block; width: var(--grid);
        opacity: 0; pointer-events: none; transition: opacity var(--d-fast);
      }
      .page.has-scroll:hover .scrollbar,
      .page.has-scroll.is-scrolling .scrollbar { opacity: 1; }
      .scrollbar i {
        position: absolute; top: 0; right: 0; width: var(--grid); min-height: var(--ctl-sm);
        border-radius: var(--r-pill); background: var(--line-strong);
      }
    `;
    render() {
      return `<div class="page"><div class="scroll"><div class="col"><slot></slot></div></div><span class="scrollbar" aria-hidden="true"><i></i></span></div>`;
    }
    hydrate() {
      const root = this.$(".page");
      const scroller = this.$(".scroll");
      const rail = this.$(".scrollbar");
      const thumb = rail && rail.querySelector("i");
      if (!root || !scroller || !rail || !thumb) return;
      if (this._wired) return; this._wired = true;   // 监听挂在持久 host/scroller 上 → 只绑一次（守住"observed 为空"不变量，未来加属性也不堆叠）
      let hideTimer = 0;
      let ticking = false;

      const update = (show) => {
        const maxScroll = scroller.scrollHeight - scroller.clientHeight;
        if (maxScroll <= 0) {
          root.classList.remove("has-scroll");
          root.classList.remove("is-scrolling");
          return;
        }
        root.classList.add("has-scroll");
        const railH = rail.clientHeight;
        const cs = getComputedStyle(this);
        const minH = parseFloat(cs.getPropertyValue("--ctl-sm")) || parseFloat(cs.getPropertyValue("--ctl"));
        const thumbH = Math.max(minH, (railH * scroller.clientHeight) / scroller.scrollHeight);
        const top = ((railH - thumbH) * scroller.scrollTop) / maxScroll;
        thumb.style.height = thumbH + "px";
        thumb.style.transform = "translateY(" + top + "px)";
        if (show) {
          root.classList.add("is-scrolling");
          clearTimeout(hideTimer);
          // 空闲约 0.7s 后淡出 thumb（idle-hide）；为运行时数值、非源 token 范畴。
          hideTimer = setTimeout(() => root.classList.remove("is-scrolling"), 700);
        }
      };

      const requestUpdate = (show) => {
        if (ticking) return;
        ticking = true;
        requestAnimationFrame(() => { ticking = false; update(show); });
      };

      scroller.addEventListener("scroll", () => requestUpdate(true), { passive: true });
      this.addEventListener("mouseenter", () => requestUpdate(false));
      requestUpdate(false);
      if (window.ResizeObserver) {
        const ro = new ResizeObserver(() => requestUpdate(false));
        ro.observe(scroller);
        ro.observe(this.$(".col"));
      }
    }
  }
  window.AnElement.define(AnPage);
})();
