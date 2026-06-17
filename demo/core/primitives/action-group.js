/* Anselm 原语 D7 — <an-action-group compact end block stack>。动作组：统一按钮间距 / 右对齐 / 事件委托，
   杜绝页面手摆按钮。子 <an-button>（或任意带 data-action 的元素）走默认 slot。
   why：对齐与间距由结构焊死，页面拼装时只决定“放哪些动作、向哪头靠”，量不到、也摆不歪。
   end=右对齐（默认左）；compact=密集间距；block=占满宽；stack=纵向铺开。
   交互：点击带 data-action 的子项 → 派发 composed 'an-action'（detail.action 携动作名）。 */
(function () {
  class AnActionGroup extends window.AnElement {
    static tag = "an-action-group";
    static observed = ["compact", "end", "block", "stack", "label"];
    static css = `
      :host { display: inline-flex; }
      :host([block]) { display: flex; width: 100%; }
      .group {
        display: inline-flex; align-items: center; justify-content: flex-start; gap: var(--sp-2);
        min-width: 0;
      }
      :host([compact]) .group { gap: var(--sp-1); }
      :host([end]) .group { justify-content: flex-end; }
      :host([block]) .group { display: flex; width: 100%; }
      :host([stack]) .group { flex-direction: column; align-items: stretch; }
      :host([stack]) ::slotted(*) { width: 100%; }
    `;
    render() {
      const aria = this.attr("label") ? ` aria-label="${window.anEsc(this.attr("label"))}"` : "";
      return `<div class="group" role="group"${aria}><slot></slot></div>`;
    }
    hydrate() {
      // 监听挂在持久 host 上（非每次重渲新建的 shadow 子节点）→ 只绑一次，杜绝属性重渲堆叠监听、an-action 多发。
      if (this._wired) return; this._wired = true;
      // 事件委托：动作名挂在子项 data-action 上，组负责统一上抛 → 页面只听一个 an-action。
      this.addEventListener("click", (ev) => {
        const node = ev.target.closest("[data-action]");
        if (!node || !this.contains(node)) return;
        this.emit("an-action", { action: node.getAttribute("data-action") });
      });
    }
  }
  window.AnElement.define(AnActionGroup);
})();
