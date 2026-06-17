/* Anselm 原语 E1 — <an-code-editor lang editable compact wrap inline>。唯一代码块/轻编辑原语。
   默认 = 编辑器块：单框（圆角 + 边，框内无横线）+ 紧凑顶栏（左 复制/换行 icon · 右 语言标签，规范大小写）+ 行号 + 高亮。
   inline → 退化为正文里的无框高亮板（无顶栏/无框/无行号），用于紧凑内联场景（如 version-diff 行）。
   可读（默认）/ 可编辑（[editable]，派发 composed 'an-input'{value}）。
   高亮：正则 → --cd-* span（py/js/markdown/CEL 及 $n/${}/{{}}）。静态 AnCodeEditor.highlight(code,lang) 供 version-diff 复用。
   code 经 textContent 入：<an-code-editor lang="py">def f(): pass</an-code-editor>。 */
(function () {
  var KW = new Set('const let var function def class return if elif else for while do in of import from export default new await async try except catch finally raise throw with as lambda yield and or not is None True False true false null undefined self this match case pass break continue'.split(' '));
  var TOK = /(#[^\n]*|\/\/[^\n]*|\/\*[\s\S]*?\*\/)|(`(?:\\.|[^`\\])*`|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|(\$\{[^}]*\}|\$\d+|\{\{[^}]*\}\})|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g;
  var LANG = { py: "Python", python: "Python", js: "JavaScript", javascript: "JavaScript", ts: "TypeScript", typescript: "TypeScript",
    json: "JSON", md: "Markdown", markdown: "Markdown", cel: "CEL", sh: "Shell", bash: "Shell", go: "Go", sql: "SQL",
    html: "HTML", css: "CSS", yaml: "YAML", yml: "YAML", toml: "TOML", rs: "Rust", rust: "Rust" };

  function langLabel(l) { if (!l) return ""; var k = String(l).toLowerCase(); return LANG[k] || (l.charAt(0).toUpperCase() + l.slice(1)); }

  function highlight(code, lang) {
    var esc = window.anEsc, out = '', last = 0, m;
    code = String(code == null ? '' : code);
    TOK.lastIndex = 0;
    while ((m = TOK.exec(code))) {
      out += esc(code.slice(last, m.index));
      if (m[1]) out += '<span class="cd-com">' + esc(m[1]) + '</span>';
      else if (m[2]) out += '<span class="cd-str">' + esc(m[2]) + '</span>';
      else if (m[3]) out += '<span class="cd-arg">' + esc(m[3]) + '</span>';
      else if (m[4]) out += '<span class="cd-num">' + esc(m[4]) + '</span>';
      else {
        var w = m[5];
        if (KW.has(w)) out += '<span class="cd-kw">' + esc(w) + '</span>';
        else if (/^\s*\(/.test(code.slice(m.index + w.length))) out += '<span class="cd-fn">' + esc(w) + '</span>';
        else out += esc(w);
      }
      last = m.index + m[0].length;
    }
    return out + esc(code.slice(last));
  }
  function lineNos(code) {
    var n = String(code == null ? '' : code).split('\n').length, s = '';
    for (var i = 1; i <= n; i++) s += '<i>' + i + '</i>';
    return s;
  }

  class AnCodeEditor extends window.AnElement {
    static tag = "an-code-editor";
    static observed = ["lang", "editable", "compact", "wrap", "inline"];
    static css = `
      :host { display: block; }
      /* 描边走 inset box-shadow 而非 border——圆角 + 半透明 border 在四角 1px 自叠加变深成"灰尖"（Retina 更明显），inset 环均匀无叠加；focus 时叠加聚焦光环。 */
      .code { display: flex; flex-direction: column; min-width: 0;
        box-shadow: inset 0 0 0 var(--hairline) var(--line); border-radius: var(--r-card); background: var(--island); overflow: hidden; }
      :host([inline]) .code { box-shadow: none; border-radius: 0; background: transparent; }
      :host([editable]:not([inline])) .code:focus-within { box-shadow: inset 0 0 0 var(--hairline) var(--accent-line), 0 0 0 var(--focus-ring) var(--accent-soft); }
      /* inline 可编辑（如 run-terminal 的 args）无框，聚焦只给外侧柔环——否则编辑时零反馈 */
      :host([inline][editable]) .code:focus-within { box-shadow: 0 0 0 var(--focus-ring) var(--accent-soft); border-radius: var(--r-tag); }

      /* 顶栏：紧凑单框无横线——左 icon 钮、右 语言；底部不留白，正文气口归 .pre */
      .bar { flex: none; display: flex; align-items: center; gap: var(--grid); padding: var(--sp-2) var(--sp-3) 0; }
      :host([inline]) .bar { display: none; }
      .act { width: var(--ctl-sm); height: var(--ctl-sm); display: grid; place-items: center; border-radius: var(--r-tag);
        color: var(--ink-3); transition: background var(--d-fast), color var(--d-fast); }
      .act:hover { background: var(--island-3); color: var(--ink); }
      .act.on { color: var(--accent); }
      .act svg { width: var(--icon-sm); height: var(--icon-sm); }
      /* 编辑态文本钮：取消（中性）/ 保存（accent） */
      .btn { height: var(--ctl-sm); padding: 0 var(--btn-pad-x-sm); border-radius: var(--r-tag);
        color: var(--ink-2); font-size: var(--t-meta); transition: background var(--d-fast), color var(--d-fast); }
      .btn:hover { background: var(--island-3); color: var(--ink); }
      .btn.save { color: var(--accent); font-weight: 600; }
      .btn.save:hover { background: var(--accent-soft); }
      .grow { flex: 1; }
      .lang { color: var(--ink-3); font-size: var(--t-meta); line-height: var(--lh-ui); }

      .main { display: flex; min-width: 0; }
      .gut { flex: none; padding: var(--sp-2) 0 var(--sp-3); color: var(--ink-3); opacity: .5;
        font-family: var(--mono); font-size: var(--t-meta); line-height: var(--lh-prose); text-align: right; user-select: none; }
      :host([inline]) .gut { display: none; }
      .gut i { display: block; padding: 0 var(--sp-2) 0 var(--sp-3); font-style: normal; }

      .area { position: relative; flex: 1; min-width: 0; }
      .pre, .input { margin: 0; padding: var(--sp-2) var(--sp-4) var(--sp-3) var(--sp-2); border: 0;
        font-family: var(--mono); font-size: var(--t-meta); line-height: var(--lh-prose); white-space: pre; tab-size: 4; }
      :host([inline]) .pre, :host([inline]) .input { padding: 0; }
      :host([wrap]) .pre, :host([wrap]) .input { white-space: pre-wrap; word-break: break-word; }
      :host([compact]) .gut, :host([compact]) .pre, :host([compact]) .input { padding-top: var(--sp-1); padding-bottom: var(--sp-1); }
      .pre { min-width: 100%; box-sizing: border-box; overflow: auto; color: var(--ink-2); }
      .pre code { font: inherit; }
      :host(:not([editable])) .area { overflow-x: auto; }
      .input { position: absolute; inset: 0; width: 100%; height: 100%; resize: none; outline: none;
        background: transparent; color: transparent; caret-color: var(--accent); overflow: auto; }
      .input::selection { background: var(--accent-soft); }

      .cd-com { color: var(--cd-com); font-style: italic; }
      .cd-kw  { color: var(--cd-kw); }
      .cd-str { color: var(--cd-str); }
      .cd-num { color: var(--cd-num); }
      .cd-fn  { color: var(--cd-fn); }
      .cd-arg { color: var(--accent); font-weight: 600; }
    `;
    render() {
      // 种子优先取活值 _value（已编辑 / .value 设入）；否则取 light-DOM textContent —— 否则任一 observed 属性变更（如切换 wrap）重渲会把已输入内容抹回原始种子
      var code = String(this._value != null ? this._value : (this.textContent == null ? '' : this.textContent));
      var inline = this.has("inline");
      var lang = langLabel(this.attr("lang"));
      // inline editable = 常驻编辑（run-terminal args）；非 inline editable = 默认只读、点编辑进编辑态
      var editing = inline ? this.has("editable") : !!this._editing;
      var left = this._editing
        ? '<button type="button" class="btn" data-act="cancel">取消</button><button type="button" class="btn save" data-act="save">保存</button>'
        : ('<button type="button" class="act" data-act="copy" title="复制">' + window.icon("copy") + '</button>'
          + '<button type="button" class="act" data-act="wrap" title="自动换行">' + window.icon("wrap") + '</button>'
          + (this.has("editable") ? '<button type="button" class="act" data-act="edit" title="编辑">' + window.icon("edit") + '</button>' : ''));
      var bar = inline ? "" :
        '<div class="bar">' + left +
        '<span class="grow"></span>' + (lang ? '<span class="lang">' + window.anEsc(lang) + '</span>' : '') + '</div>';
      var gut = inline ? "" : '<div class="gut">' + lineNos(code) + '</div>';
      var input = editing ? '<textarea class="input" spellcheck="false">' + window.anEsc(code) + '</textarea>' : '';
      return '<div class="code">' + bar +
        '<div class="main">' + gut +
        '<div class="area"><pre class="pre"><code>' + highlight(code, this.attr("lang")) + '</code></pre>' + input + '</div>' +
        '</div></div>';
    }
    hydrate() {
      var self = this;
      if (this._value == null) this._value = this.textContent || "";   // 仅首次从种子初始化（重渲不覆盖已编辑活值）
      var copyBtn = this.$('[data-act="copy"]');
      if (copyBtn) copyBtn.addEventListener("click", function () {
        // 复制成功才亮"已复制"——async 写入失败（无权限/非安全上下文）不谎报成功，且不抛未捕获 rejection
        Promise.resolve(navigator.clipboard ? navigator.clipboard.writeText(self.value) : Promise.reject()).then(function () {
          copyBtn.classList.add("on"); copyBtn.innerHTML = window.icon("check");
          setTimeout(function () { copyBtn.classList.remove("on"); copyBtn.innerHTML = window.icon("copy"); }, 1200);
        }, function () {});
      });
      var wrapBtn = this.$('[data-act="wrap"]');
      if (wrapBtn) wrapBtn.addEventListener("click", function () { wrapBtn.classList.toggle("on", self.toggleAttribute("wrap")); });

      // 编辑态切换（非 inline）：编辑 → 进编辑态（快照当前值供取消）；保存 → 提交 + 派 an-change；取消 → 还原
      var editBtn = this.$('[data-act="edit"]');
      if (editBtn) editBtn.addEventListener("click", function () { self._snapshot = self._value; self._editing = true; self._render(); requestAnimationFrame(function () { self.focus(); }); });
      var saveBtn = this.$('[data-act="save"]');
      if (saveBtn) saveBtn.addEventListener("click", function () { self._editing = false; self._render(); self.emit("an-change", { value: self._value }); });
      var cancelBtn = this.$('[data-act="cancel"]');
      if (cancelBtn) cancelBtn.addEventListener("click", function () { self._value = self._snapshot != null ? self._snapshot : self._value; self._editing = false; self._render(); });

      var input = this.$(".input");
      if (!input) return;
      function repaint() {
        self._value = input.value;
        var codeEl = self.$(".pre code"); if (codeEl) codeEl.innerHTML = highlight(input.value, self.attr("lang"));
        if (!self.has("inline")) { var g = self.$(".gut"); if (g) g.innerHTML = lineNos(input.value); }
      }
      input.addEventListener("input", function () { repaint(); self.emit("an-input", { value: input.value }); });
      input.addEventListener("scroll", function () { var pre = self.$(".pre"); if (pre) { pre.scrollTop = input.scrollTop; pre.scrollLeft = input.scrollLeft; } });
      input.addEventListener("keydown", function (e) {
        if (e.key !== "Tab") return;
        e.preventDefault();
        var s = input.selectionStart, en = input.selectionEnd;
        input.value = input.value.slice(0, s) + "    " + input.value.slice(en);
        input.selectionStart = input.selectionEnd = s + 4;
        input.dispatchEvent(new Event("input"));
      });
    }
    get value() { var i = this.$(".input"); return i ? i.value : (this._value || ""); }
    set value(s) {
      var v = String(s == null ? "" : s); this._value = v;
      var input = this.$(".input"); if (input) input.value = v;
      var codeEl = this.$(".pre code"); if (codeEl) codeEl.innerHTML = highlight(v, this.attr("lang"));
      if (!this.has("inline")) { var g = this.$(".gut"); if (g) g.innerHTML = lineNos(v); }
    }
    focus() { var i = this.$(".input"); if (i) i.focus(); }
  }
  AnCodeEditor.highlight = highlight;
  window.AnElement.define(AnCodeEditor);
  window.AnCodeEditor = AnCodeEditor;
})();
