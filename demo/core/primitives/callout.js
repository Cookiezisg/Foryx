/* Anselm 原语 F7 — <an-callout tone icon>（整行宽警示条；移植 design Attention）。
   解剖：左图标格（顶对齐、tone 上色）+ 富文本体（默认 slot，可含 <b> 强调）。
   tone ∈ warn(默认) | danger | info | ok：warn/danger/ok 走语义软底 + 描边；info 走透明底 + 细描边（纯 doc-callout，无填充）。
   描边 = 语义色与 --line 的 color-mix（软描边，不喧宾夺主）；图标默认随 tone（icon 属性可覆盖）。
   属性：tone | icon（覆盖默认图标）。body 走默认 slot。 */
(function () {
  // tone → 默认图标语义 key（调用方可用 icon 属性覆盖）
  const TONE_ICON = { warn: "flag", danger: "close", info: "shield", ok: "check" };
  const TONES = { warn: 1, danger: 1, info: 1, ok: 1 };

  class AnCallout extends window.AnElement {
    static tag = "an-callout";
    static observed = ["tone", "icon"];
    static css = `
      :host { display: block; }
      .callout {
        display: flex; align-items: flex-start; gap: var(--sp-2);
        padding: var(--sp-3) var(--btn-pad-x);
        border: var(--hairline) solid var(--line); border-radius: var(--r-chip);
        font-size: var(--t-body); line-height: var(--lh-prose); color: var(--ink-2);
        transition: border-color var(--d-fast) var(--ease-spring), background var(--d-fast) var(--ease-spring);
      }
      :host([tone="warn"]) .callout,
      :host(:not([tone])) .callout { border-color: color-mix(in srgb, var(--warn) 38%, var(--line)); background: var(--warn-soft); }
      :host([tone="danger"]) .callout { border-color: color-mix(in srgb, var(--danger) 36%, var(--line)); background: var(--danger-soft); }
      :host([tone="ok"])     .callout { border-color: color-mix(in srgb, var(--ok) 36%, var(--line)); background: var(--ok-soft); }
      :host([tone="info"])   .callout { border-color: var(--line); background: transparent; }

      .ico { flex: none; margin-top: var(--hairline); display: grid; place-items: center; color: var(--warn); }
      .ico svg { display: block; width: var(--icon); height: var(--icon); }
      :host([tone="danger"]) .ico { color: var(--danger); }
      :host([tone="ok"])     .ico { color: var(--ok); }
      :host([tone="info"])   .ico { color: var(--accent); }

      .body { min-width: 0; }
      ::slotted(b), ::slotted(strong) { color: var(--ink); font-weight: 600; }
    `;

    render() {
      const tone = TONES[this.attr("tone")] ? this.attr("tone") : "warn";
      const key = this.attr("icon") || TONE_ICON[tone];
      return `<div class="callout">`
        + `<span class="ico">${window.icon(key)}</span>`
        + `<span class="body"><slot></slot></span>`
        + `</div>`;
    }
  }
  window.AnElement.define(AnCallout);
})();
