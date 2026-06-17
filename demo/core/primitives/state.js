/* Anselm 原语 F5 — <an-state variant icon title hint>。空/加载/错误占位：居中 icon + 标题 + 说明 + 动作槽。
   variant ∈ empty(默认中性) | loading(icon 配 shimmer 微动) | error(--danger 色)。
   解剖（垂直居中栈）：icon 落入一个柔和「井」（island 圆底，与 16 图标同心放大）→ 标题 → 说明 → 动作。
   icon 缺省按 variant 兜底（empty→inbox / loading→spin / error→triangle-alert）；动作走具名 slot[name=action]（放 <an-button>）。 */
(function () {
  // variant → 兜底图标语义 key（无 icon 属性时落此）。
  var FALLBACK_ICON = { empty: "inbox", loading: "spin", error: "triangle-alert" };

  class AnState extends window.AnElement {
    static tag = "an-state";
    static observed = ["variant", "icon", "title", "hint"];
    static css = `
      :host { display: block; }
      .state {
        display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center;
        gap: var(--sp-3); padding: var(--sp-12) var(--sp-6); min-height: var(--w-block);
      }
      /* icon「井」：圆形柔底，放大图标但保持光学同心；尺寸走 --island-head（44）= 头部级留白单元 */
      .well {
        display: grid; place-items: center; flex: none;
        width: var(--island-head); height: var(--island-head); border-radius: var(--r-pill);
        background: var(--island-3); color: var(--ink-3);
      }
      .well svg { width: var(--t-h3); height: var(--t-h3); }   /* 20 · 中号语义图标 */
      .copy { display: flex; flex-direction: column; align-items: center; gap: var(--sp-1); max-width: var(--w-block); }
      .title { color: var(--ink); font-size: var(--t-body); font-weight: 600; line-height: var(--lh-ui); }
      .hint  { color: var(--ink-3); font-size: var(--t-meta); line-height: var(--lh-prose); }
      .action { margin-top: var(--sp-2); display: flex; align-items: center; justify-content: center; gap: var(--sp-2); }
      .action:not(:has(*)) { display: none; }   /* 无动作即塌掉，免留白 */

      /* loading — icon 旋转 + 井 shimmer 呼吸（无 accent，克制） */
      :host([variant="loading"]) .well { color: var(--ink-2); }
      :host([variant="loading"]) .well svg { animation: an-state-spin calc(var(--d-slow) * 3) linear infinite; }
      :host([variant="loading"]) .well {
        background: linear-gradient(100deg, var(--island-3) 30%, var(--island-4) 50%, var(--island-3) 70%);
        background-size: 200% 100%;
        animation: an-state-shimmer calc(var(--d-slow) * 4) var(--ease-out) infinite;
      }
      /* error — danger 调性 */
      :host([variant="error"]) .well { background: var(--danger-soft); color: var(--danger); }
      :host([variant="error"]) .title { color: var(--ink); }

      @keyframes an-state-spin { to { transform: rotate(360deg); } }
      @keyframes an-state-shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
    `;
    render() {
      const e = window.anEsc;
      const variant = this.attr("variant", "empty");
      const ic = this.attr("icon") || FALLBACK_ICON[variant] || FALLBACK_ICON.empty;
      const title = this.attr("title");
      const hint = this.attr("hint");
      const titleEl = (title != null && title !== "") ? `<div class="title">${e(title)}</div>` : "";
      const hintEl = (hint != null && hint !== "") ? `<div class="hint">${e(hint)}</div>` : "";
      const copy = (titleEl || hintEl) ? `<div class="copy">${titleEl}${hintEl}</div>` : "";
      return `<div class="state">`
        + `<span class="well">${window.icon(ic, 20)}</span>`
        + copy
        + `<div class="action"><slot name="action"></slot></div>`
        + `</div>`;
    }
  }
  window.AnElement.define(AnState);
})();
