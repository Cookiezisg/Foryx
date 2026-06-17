/* Anselm demo — <an-shell>：三岛外壳（布局脚手架，≈ Flutter Scaffold）。
   职责：圆角浮窗 + 三岛布局（左岛/海洋/右岛）+ 左岛收起/拖拽调宽/持久化 + 滚动自隐 + 海洋头部浮层。
   不碰内容：左岛内容 = slot[left]（<an-sidebar>）；海洋 = slot[sea]；右岛 = slot[right]；头部 = slot[head-lead/head-extra]。
   开关铁律（SPEC §3.1）：左岛展开时 panel-left 在左岛内（sidebar 自带）；收起时迁到海洋左上（本组件 reopen）；panel-right 永在海洋右上。
   API：setSea(node) / setRight(node|null) / setHeadExtra(node|null) / toggleLeft() / toggleRight()。 */
(function () {
  const KEY_W = "fy.side.w", KEY_OFF = "fy.side.collapsed";

  class AnShell extends window.AnElement {
    static tag = "an-shell";
    static observed = ["side", "right"];
    static css = `
      :host { display: block; height: 100%; }
      .win { height: 100%; display: flex; flex-direction: column;
        background: var(--island); border-radius: var(--r-island); box-shadow: var(--shadow-win); overflow: hidden; }
      .body { flex: 1; min-height: 0; display: flex; gap: var(--sp-2); padding: var(--sp-2); }

      /* 左岛容器：只管宽度回流（岛皮肤在 <an-sidebar> 自身）。--side-w 行内由本组件持有/持久化。 */
      .leftwrap { position: relative; flex: none; width: var(--side-w);
        transition: width var(--d-slow) var(--ease-spring), opacity var(--d-mid), margin-right var(--d-slow) var(--ease-spring); }
      :host([side="off"]) .leftwrap { width: var(--zero); opacity: 0; margin-right: calc(-1 * var(--sp-2)); overflow: hidden; pointer-events: none; }
      :host([dragging]) .leftwrap { transition: none; }
      .grip { position: absolute; top: var(--zero); right: var(--zero); width: var(--sp-2); height: 100%; cursor: col-resize; z-index: 3; }
      .grip::after { content: ""; position: absolute; inset: 0 var(--hairline) 0 auto; width: var(--line-2); border-radius: var(--line-2);
        background: transparent; transition: background var(--d-fast); }
      .grip:hover::after { background: var(--line-strong); }
      :host([side="off"]) .grip { display: none; }

      .main { position: relative; flex: 1; min-width: 0; overflow: hidden; display: flex; flex-direction: column; }
      /* 海洋头 = 不占流的顶部浮层：正文滚到顶不被横带切。透明 + pointer-events:none，仅角落控件可点。 */
      .head { position: absolute; top: var(--zero); left: var(--zero); right: var(--zero); z-index: 5;
        display: flex; align-items: center; gap: var(--gap); height: var(--island-head); padding: 0 var(--sp-2) 0 var(--sp-3); pointer-events: none; }
      .lead, .extra { pointer-events: auto; display: flex; align-items: center; gap: var(--grid); }
      .grow { flex: 1; }
      .reopen { width: var(--zero); opacity: 0; overflow: hidden; pointer-events: none;
        transition: opacity var(--d-mid), width var(--d-slow) var(--ease-spring); }
      :host([side="off"]) .reopen { width: var(--ctl); opacity: 1; pointer-events: auto; }
      .pright { display: none; }
      :host([has-right]) .pright { display: inline-flex; }

      .sea { flex: 1; min-height: 0; display: flex; flex-direction: column; }

      /* 右岛容器：宽度回流；岛皮肤在 <an-right-island>。岛内容 width:100% 随 wrap 宽，故 open 时 overflow:visible 不会溢出、
         只让右岛阴影显出来（收起态保持 hidden，宽度归 0 不漏内容）——根治"右岛无阴影"（原 overflow:hidden 把阴影裁了）。 */
      .rightwrap { flex: none; width: var(--zero); overflow: hidden; opacity: 0;
        transition: width var(--d-slow) var(--ease-spring), opacity var(--d-mid); }
      :host([right="open"]) .rightwrap { width: var(--island-w); opacity: 1; overflow: visible; }
    `;

    render() {
      return `
        <div class="win"><div class="body">
          <div class="leftwrap"><slot name="left"></slot><div class="grip"></div></div>
          <div class="main">
            <div class="head">
              <span class="lead">
                <an-button class="reopen" variant="icon" icon="panel-left"></an-button>
                <slot name="head-lead"></slot>
              </span>
              <span class="grow"></span>
              <span class="extra">
                <slot name="head-extra"></slot>
                <an-button class="pright" variant="icon" icon="panel-right"></an-button>
              </span>
            </div>
            <div class="sea"><slot name="sea"></slot></div>
          </div>
          <div class="rightwrap"><slot name="right"></slot></div>
        </div></div>`;
    }

    hydrate() {
      // 初始态：从 localStorage 恢复宽度/收起（首帧前置，避免闪）。
      const w = parseInt(localStorage.getItem(KEY_W) || "", 10);
      this.style.setProperty("--side-w", (w >= 240 && w <= 420 ? w : 240) + "px");
      if (!this.hasAttribute("side")) this.setAttribute("side", localStorage.getItem(KEY_OFF) === "1" ? "off" : "on");

      this.$(".reopen").addEventListener("click", () => this.toggleLeft());
      this.$(".pright").addEventListener("click", () => this.toggleRight());
      this._wireGrip();
      this._wireScroll();
    }

    _wireGrip() {
      const grip = this.$(".grip");
      let sx = 0, sw = 0, dragging = false;
      grip.addEventListener("pointerdown", (e) => {
        if (this.getAttribute("side") === "off") return;
        dragging = true; sx = e.clientX;
        sw = parseFloat(getComputedStyle(this).getPropertyValue("--side-w")) || 240;
        this.setAttribute("dragging", ""); document.body.style.userSelect = "none"; document.body.style.cursor = "col-resize"; e.preventDefault();
      });
      window.addEventListener("pointermove", (e) => {
        if (!dragging) return;
        const w = Math.max(240, Math.min(420, sw + (e.clientX - sx)));
        this.style.setProperty("--side-w", w + "px");
      });
      window.addEventListener("pointerup", () => {
        if (!dragging) return; dragging = false; this.removeAttribute("dragging");
        document.body.style.userSelect = ""; document.body.style.cursor = "";
        localStorage.setItem(KEY_W, String(Math.round(parseFloat(getComputedStyle(this).getPropertyValue("--side-w")) || 240)));
      });
    }

    _wireScroll() {
      // 滚动自隐：捕获全文档滚动 → html[data-scrolling]，停 700ms 清（reset.css 据此显隐 webkit thumb）。
      let st;
      this._onScroll = () => {
        document.documentElement.dataset.scrolling = "";
        clearTimeout(st); st = setTimeout(() => delete document.documentElement.dataset.scrolling, 700);
      };
      document.addEventListener("scroll", this._onScroll, true);
    }
    disconnectedCallback() { if (this._onScroll) document.removeEventListener("scroll", this._onScroll, true); }

    // ── 内容注入 ──
    _slot(name, node) {
      [].slice.call(this.querySelectorAll('[slot="' + name + '"]')).forEach((n) => n.remove());
      if (node) { node.setAttribute("slot", name); this.appendChild(node); }
    }
    setSea(node) { this._slot("sea", node); }
    setHeadExtra(node) { this._slot("head-extra", node); }
    // 注入即开、清空即合（feature 给了右岛内容就展开，给 null 就收起）
    setRight(node) { this._slot("right", node); this.toggleAttribute("has-right", !!node); this.setAttribute("right", node ? "open" : "closed"); }

    toggleLeft() {
      const off = this.getAttribute("side") !== "off";
      this.setAttribute("side", off ? "off" : "on");
      localStorage.setItem(KEY_OFF, off ? "1" : "0");
    }
    toggleRight() { this.setAttribute("right", this.getAttribute("right") === "open" ? "closed" : "open"); }
  }
  window.AnElement.define(AnShell);
})();
