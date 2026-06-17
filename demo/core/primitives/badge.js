/* Anselm 原语 A2 — <an-badge tone dot>。状态/标签药丸：语义柔底 + 语义字色。
   tone ∈ neutral(默认) | ok | warn | danger | accent。
   dot（可选）= 状态点态：idle/run/wait/err/done，复用 <an-status-dot>（状态归一单一路径）。
   label 走默认 slot：<an-badge tone="ok" dot="done">passed</an-badge>。 */
(function () {
  class AnBadge extends window.AnElement {
    static tag = "an-badge";
    static observed = ["tone", "dot"];
    static css = `
      :host { display: inline-flex; }
      .badge {
        display: inline-flex; align-items: center; gap: var(--gap-tight);
        height: var(--badge-h); padding: 0 var(--badge-pad-x); border-radius: var(--r-pill);
        font-size: var(--t-meta); font-weight: 500; white-space: nowrap;
        background: var(--island-3); color: var(--ink-2);
      }
      :host([tone="ok"])     .badge { background: var(--ok-soft);     color: var(--ok); }
      :host([tone="warn"])   .badge { background: var(--warn-soft);   color: var(--warn); }
      :host([tone="danger"]) .badge { background: var(--danger-soft); color: var(--danger); }
      :host([tone="accent"]) .badge { background: var(--accent-soft); color: var(--accent); }
    `;
    render() {
      const dot = this.attr("dot")
        ? `<an-status-dot state="${window.anEsc(this.attr("dot"))}"></an-status-dot>`
        : "";
      return `<span class="badge">${dot}<span><slot></slot></span></span>`;
    }
  }
  window.AnElement.define(AnBadge);
})();
