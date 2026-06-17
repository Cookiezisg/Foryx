/* Anselm 原语 B2 — <an-input multiline mono full value placeholder>。
   值叶子：单行 input；multiline → textarea（高 auto，可竖向 resize）；mono → 等宽紧凑；full → 占满宽。
   统一焦点环（accent-line + accent-soft 外环）。输入对外派发 composed 'an-input'（detail.value）。 */
(function () {
  class AnInput extends window.AnElement {
    static tag = "an-input";
    static observed = ["multiline", "mono", "full", "value", "placeholder", "disabled", "readonly"];
    static css = `
      :host { display: inline-block; }
      :host([full]) { display: block; }
      .input {
        height: var(--ctl); min-width: var(--input-min); padding: 0 var(--sp-3);
        border: var(--hairline) solid var(--line); border-radius: var(--r-btn); background: var(--island);
        font: inherit; font-size: var(--t-body); color: var(--ink);
        transition: border-color var(--d-fast), box-shadow var(--d-fast);
      }
      .input::placeholder { color: var(--ink-3); }
      .input:focus { outline: none; border-color: var(--accent-line); box-shadow: 0 0 0 var(--focus-ring) var(--accent-soft); }
      :host([full]) .input { width: 100%; min-width: 0; }
      .input.area {
        height: auto; min-height: calc(var(--ctl) * 2); padding: var(--sp-2) var(--sp-3);
        resize: vertical; line-height: var(--lh-ui);
      }
      :host([mono]) .input { font-family: var(--mono); font-size: var(--t-meta); }
      /* disabled / readonly：与 button/dropdown 同语汇——禁用半透 + 不可点；只读静音字 + 默认指针 */
      :host([disabled]) { pointer-events: none; }
      :host([disabled]) .input { opacity: .4; }
      :host([readonly]) .input { color: var(--ink-3); cursor: default; }
    `;
    render() {
      const e = window.anEsc;
      const ph = e(this.attr("placeholder", ""));
      const val = this.attr("value", "");
      const flags = (this.has("disabled") ? " disabled" : "") + (this.has("readonly") ? " readonly" : "");
      if (this.has("multiline")) {
        return `<textarea class="input area" placeholder="${ph}"${flags}>${e(val)}</textarea>`;
      }
      return `<input class="input" placeholder="${ph}" value="${e(val)}"${flags}>`;
    }
    hydrate() {
      // 受控叶子：把原生 input/change 收敛成对外 composed 'an-input'（feature 只听一个动词）
      const field = this.$(".input");
      if (!field) return;
      field.addEventListener("input", () => this.emit("an-input", { value: field.value }));
    }
    // 复用入口：宿主（如可编辑 KV）进入编辑态时把焦点落进原生输入框
    focus() { const f = this.$(".input"); if (f) f.focus(); }
    get value() { const f = this.$(".input"); return f ? f.value : this.attr("value", ""); }
  }
  window.AnElement.define(AnInput);
})();
