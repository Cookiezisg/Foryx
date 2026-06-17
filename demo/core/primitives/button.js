/* Anselm 原语 B1 — <an-button variant size icon disabled block>。
   variant ∈ ghost(默认中性) | primary(accent CTA) | danger | icon(方钮)。label 走默认 slot：<an-button variant="primary" icon="play">Run</an-button>。
   icon 变体只放图标、label 入 aria-label。统一 hover/active/focus/disabled。 */
(function () {
  class AnButton extends window.AnElement {
    static tag = "an-button";
    static observed = ["variant", "size", "icon", "disabled", "block"];
    static delegatesFocus = true;   // host.focus() 转发到内部原生 button —— 让 dialog 焦点陷阱够得着动作钮
    static css = `
      :host { display: inline-flex; }
      :host([block]) { display: flex; width: 100%; }
      :host([disabled]) { pointer-events: none; }
      button {
        display: inline-flex; align-items: center; justify-content: center; gap: var(--gap-tight);
        height: var(--ctl); padding: 0 var(--btn-pad-x); border-radius: var(--r-btn);
        font-size: var(--t-body); font-weight: 500; color: var(--ink-2); white-space: nowrap;
        transition: background var(--d-fast), color var(--d-fast);
      }
      .ico { display: grid; place-items: center; flex: none; }
      .ico svg { width: var(--icon); height: var(--icon); }
      :host([disabled]) button { opacity: .4; }
      button:hover { background: var(--island-3); color: var(--ink); }
      :host([variant="primary"]) button { background: var(--accent); color: var(--ink-on-accent); font-weight: 600; }
      :host([variant="primary"]) button:hover { background: var(--accent-hover); }
      :host([variant="danger"]) button { color: var(--danger); }
      :host([variant="danger"]) button:hover { background: var(--danger-soft); }
      :host([variant="icon"]) button { width: var(--ctl); padding: 0; color: var(--ink-3); }
      :host([variant="icon"]) button:hover { background: var(--island-3); color: var(--ink); }
      :host([size="sm"]) button { height: var(--ctl-sm); padding: 0 var(--btn-pad-x-sm); font-size: var(--t-meta); }
      :host([block]) button { width: 100%; justify-content: flex-start; }
    `;
    render() {
      const ic = this.attr("icon") ? `<span class="ico">${window.icon(this.attr("icon"))}</span>` : "";
      const lbl = this.attr("variant") === "icon" ? "" : `<slot></slot>`;
      const aria = this.attr("variant") === "icon" && this.textContent.trim()
        ? ` aria-label="${window.anEsc(this.textContent.trim())}"` : "";
      return `<button part="button"${aria}>${ic}${lbl}</button>`;
    }
  }
  window.AnElement.define(AnButton);
})();
