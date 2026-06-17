/* Anselm 原语 D4 — <an-right-island title icon>。右岛内容壳：head（icon + title）+ InfoCard body 堆叠。
   why：宽度由外层 <an-shell> 的 right slot 控制——本组件只画岛皮肤 + 头 + 正文，不自管开合、不每海洋手编宽。
   body 走默认 slot（堆叠若干 <an-info-card>）；正文区可滚但不显滚轮。 */
(function () {
  class AnRightIsland extends window.AnElement {
    static tag = "an-right-island";
    static observed = ["title", "icon"];
    static css = `
      :host { display: block; height: 100%; }
      /* 岛皮肤与左岛（<an-sidebar>）同源：仅宽度不同，圆角/描边/投影一律复用，杜绝两岛观感漂移 */
      .island {
        position: relative; display: flex; flex-direction: column; height: 100%;
        background: var(--island); border: var(--hairline) solid var(--line);
        border-radius: var(--r-chip); box-shadow: var(--shadow-float); overflow: hidden;
      }
      .head { flex: none; display: flex; align-items: center; gap: var(--gap); height: var(--island-head); padding: var(--sp-2) var(--sp-4) 0; }
      .ico { display: grid; place-items: center; flex: none; color: var(--ink-3); }
      .ico svg { width: var(--icon); height: var(--icon); }
      .title { flex: 1; min-width: 0; font-size: var(--t-body); font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      /* 正文【块流】滚动区（非 column flex）——块流子项恒为自然高度、绝不被压扁；内容超高 .body 自滚。卡间距走 margin（替代 flex gap）。
         （column flex 在嵌套 shadow 里会把运行终端等子项的高度解析短一截、末行被 overflow:hidden 裁掉，故改块流。） */
      .body {
        flex: 1; min-height: 0; overflow-x: hidden; overflow-y: auto; padding: var(--sp-2) var(--sp-4);
        scrollbar-width: none; -ms-overflow-style: none;
      }
      .body::-webkit-scrollbar { width: 0; height: 0; }
      ::slotted(* + *) { margin-top: var(--sp-3); }
    `;
    render() {
      const e = window.anEsc;
      const ic = this.attr("icon") ? `<span class="ico">${window.icon(this.attr("icon"))}</span>` : "";
      const title = `<span class="title">${e(this.attr("title") || "")}</span>`;
      return `<aside class="island"><div class="head">${ic}${title}</div><div class="body"><slot></slot></div></aside>`;
    }
  }
  window.AnElement.define(AnRightIsland);
})();
