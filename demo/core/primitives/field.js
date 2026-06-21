/* Anselm 原语 C2 · C3 — 键值叶子两式（同域一文件，共用就地编辑机制）：
   · <an-field label hint value editable editor>  键值大行：label 左 + 值右对齐；可编辑值走统一就地编辑机制。无 value → 默认 slot（放下拉/按钮等控件，仍右对齐）。
   · <an-kv rows mono wrap>                        紧凑定义列表：key 左 · value 右；可编辑行走同一机制。
   【统一就地编辑机制】（所有 kv 可编辑值 + 标题共用同一交互）：
     平时——只显示 key 左 + value 右，无 affordance。
     hover——在 key 右边空两格冒出【铅笔钮】（触发点贴锚点文字）。
     点铅笔——右边的 value 原地变成【白底+光标编辑框】（不全选、只给光标），框右侧出【✓/✕】确认（编辑发生处）。
       Enter/✓/失焦提交、Esc/✕/空值还原。✓/✕ 走 mousedown+preventDefault 抢在 blur 前定调（取消优先回滚）。
     即：触发铅笔在锚点（key）右、编辑框+✓✕ 在值处——铅笔与确认分两处（与「一体三连钮」不同，故不用 an-edit-affordance）。
   枚举值仍用 <an-dropdown>（离散选择）。Field 派 'an-field-change'；KV 派 'an-kv-change'。done 一次性守卫。 */
(function () {
  function normRow(r) {
    if (Array.isArray(r)) return { key: r[0], value: r[1], editable: false };
    return {
      key: (r && r.key) != null ? r.key : "",
      value: r ? r.value : "",
      editable: !!(r && r.editable),
      editor: (r && r.editor) || "input",
      options: (r && r.options) || [],
    };
  }

  // 内生就地编辑（自由文本）：值槽原地 contenteditable 白底+光标。返回 finish(commit) 供 ✓/✕ 调用。done 一次性。
  // realVal=真值（显示为 — 占位时编辑前清空）；commit(text) 写回+重渲+派事件；取消/未改→host._render()。onState(editing) 切铅笔↔✓✕ 揭示。
  function editText(host, valueEl, realVal, commit, onState) {
    const orig = realVal == null ? "" : String(realVal);
    valueEl.textContent = orig;
    let done = false;
    valueEl.setAttribute("contenteditable", "plaintext-only");
    valueEl.classList.add("editing");
    if (onState) onState(true);
    const sel = window.getSelection();
    // 只给光标（落到值末尾），不全选——白底无蓝选区，把编辑权交给用户
    if (sel) { const r = document.createRange(); r.selectNodeContents(valueEl); r.collapse(false); sel.removeAllRanges(); sel.addRange(r); }
    valueEl.focus();
    const finish = (ok) => {
      if (done) return; done = true;
      valueEl.removeEventListener("keydown", onKey); valueEl.removeEventListener("blur", onBlur);
      valueEl.removeAttribute("contenteditable"); valueEl.classList.remove("editing");
      if (onState) onState(false);
      const text = (valueEl.textContent || "").trim();
      if (ok && text !== orig.trim()) commit(text);
      else host._render();
    };
    const onKey = (ev) => {
      if (ev.key === "Escape") { ev.preventDefault(); finish(false); }
      else if (ev.key === "Enter") { ev.preventDefault(); finish(true); }
    };
    const onBlur = () => finish(true);
    valueEl.addEventListener("keydown", onKey);
    valueEl.addEventListener("blur", onBlur);
    return finish;
  }

  // 枚举值就地选择：换入 <an-dropdown>。done 一次性。
  function editSelect(host, valueEl, rec, commit) {
    const wrap = document.createElement("span"); wrap.className = "edit";
    let done = false;
    const finish = (ok, value) => { if (done) return; done = true; document.removeEventListener("pointerdown", onDoc, true); if (ok) commit(value); else host._render(); };
    const dd = document.createElement("an-dropdown");
    dd.options = (rec.options || []).map((o) => (typeof o === "string" ? { value: o, label: o } : o));
    dd.value = rec.value;
    dd.addEventListener("an-change", (ev) => finish(true, ev.detail.value));
    dd.addEventListener("keydown", (ev) => { if (ev.key === "Escape") { ev.preventDefault(); finish(false); } });
    const onDoc = (ev) => {
      if (!wrap.isConnected) { finish(false); return; }
      const path = ev.composedPath ? ev.composedPath() : [ev.target];
      if (path.includes(valueEl) || path.includes(wrap) || path.some((n) => n.classList && n.classList.contains("an-float"))) return;
      finish(false);
    };
    document.addEventListener("pointerdown", onDoc, true);
    wrap.appendChild(dd); valueEl.replaceWith(wrap);
  }

  // 统一编辑机制的两块 HTML：铅笔（贴 key 右，hover 显）+ 取消/保存 确认（贴 value 右，editing 显，与 code-editor 同款文本钮：取消中性·保存 accent）。复用 an-button。
  function pencilHtml() { return `<an-button class="pencil" variant="icon" size="sm" icon="edit" aria-label="编辑"></an-button>`; }
  function confirmHtml() { return `<span class="confirm"><an-button class="cancel" size="sm">取消</an-button><an-button class="ok" size="sm">保存</an-button></span>`; }
  // 统一编辑皮肤：铅笔/取消保存 显隐 + 保存 accent 加粗 + 值编辑框白底+光标。揭示由父按 hover/editing 控（见各 css）。
  const EDIT_CSS = `
    .pencil { flex: none; }
    .confirm { flex: none; display: none; align-items: center; gap: var(--gap-tight); }
    .confirm .ok::part(button) { color: var(--accent); font-weight: 600; }
    .confirm .ok::part(button):hover { background: var(--accent-soft); color: var(--accent); }
    /* 视觉框 ≠ 逻辑框：竖向内距用负 margin 抵掉（不改行高）；横向【右侧真实占位】（margin-right:0）——框与「保存」钮靠 flex gap 隔开、不糊一起（左侧 -sp-2 抵掉，溢进 grow 不顶布局） */
    .v.editing { outline: none; box-shadow: inset 0 0 0 var(--hairline) var(--line-strong); border-radius: var(--r-tag); background: var(--island); cursor: text; min-width: var(--input-min); text-align: left; padding: var(--grid) var(--sp-2); margin: calc(var(--grid) * -1) 0 calc(var(--grid) * -1) calc(var(--sp-2) * -1); }
  `;

  class AnField extends window.AnElement {
    static tag = "an-field";
    static observed = ["label", "hint", "value", "editable", "editor", "wrap"];
    static css = `
      :host { display: block; }
      /* 一行：[label 块] [铅笔] [撑开] [value 右对齐] [✓✕]——hover 出铅笔、editing 出框+✓✕ */
      /* 逻辑框定高：行预留固定高（容得下最高的铅笔/✓✕ 24px），故 hover 出铅笔、editing 出框都在行内活动、永不撑高行——布局零抖 */
      .field { display: flex; align-items: center; gap: var(--sp-2); min-height: var(--island-head); padding: var(--grid) var(--pad-row); border-radius: var(--r-btn); transition: background var(--d-fast); }
      :host(:hover) .field, :host([editing]) .field { background: var(--island-3); }
      .l { min-width: 0; display: flex; flex-direction: column; gap: calc(var(--grid) / 2); }
      .k { min-width: 0; font-size: var(--t-body); color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
      .hint { min-width: 0; font-size: var(--t-meta); color: var(--ink-3); line-height: var(--lh-ui); overflow-wrap: anywhere; }
      .grow { flex: 1 1 auto; min-width: var(--zero); }
      .c { min-width: 0; flex: 0 1 auto; display: flex; align-items: center; justify-content: flex-end; }
      .v { min-width: 0; font-size: var(--t-body); color: var(--ink-2); text-align: right; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
      /* wrap：长 value 换行多行自适应（observed 有 wrap，此前无 CSS 是死属性）——换行即左对齐、anywhere 断无空格长串 */
      :host([wrap]) .v { white-space: normal; overflow-wrap: anywhere; text-overflow: clip; text-align: left; }
      /* 铅笔：可编辑 + hover 才显；editing 时藏（:not([editing]) 直接挡掉 hover 揭示，让位 ✓✕） */
      .pencil { display: none; }
      :host([editable]:not([editing]):hover) .pencil { display: inline-flex; }
      :host([editing]) .confirm { display: inline-flex; }
      ${EDIT_CSS}
    `;

    set options(v) { this._options = Array.isArray(v) ? v : []; }
    get options() { return this._options || []; }

    render() {
      const e = window.anEsc;
      const valAttr = this.attr("value");
      const hasValueAttr = valAttr != null;
      const editableText = this.has("editable") && hasValueAttr;
      const hint = this.attr("hint") ? `<div class="hint">${e(this.attr("hint"))}</div>` : "";
      const label = e(this.attr("label", ""));
      const control = hasValueAttr ? `<span class="v">${e(valAttr === "" ? "—" : valAttr)}</span>` : `<slot></slot>`;
      const pencil = editableText ? pencilHtml() : "";
      const confirm = editableText ? confirmHtml() : "";
      return `<div class="field"><div class="l"><div class="k">${label}</div>${hint}</div>${pencil}<span class="grow"></span><div class="c">${control}</div>${confirm}</div>`;
    }

    hydrate() {
      this._editing = false;  // 每次重渲解锁（编辑收尾必经重渲）
      const pencil = this.$(".pencil"); if (!pencil) return;
      pencil.addEventListener("click", () => this._startEdit());
      const ok = this.$(".confirm .ok"), cancel = this.$(".confirm .cancel");
      if (ok) ok.addEventListener("mousedown", (ev) => { ev.preventDefault(); this._finish && this._finish(true); });
      if (cancel) cancel.addEventListener("mousedown", (ev) => { ev.preventDefault(); this._finish && this._finish(false); });
    }
    _startEdit() {
      if (this._editing) return;  // 守卫：编辑中再触发不重入
      const vEl = this.$(".v"); if (!vEl) return;
      const commit = (value) => { this.setAttribute("value", value); this.emit("an-field-change", { label: this.attr("label"), value }); };
      if ((this.attr("editor") || "input") === "select") { editSelect(this, vEl, { value: this.attr("value"), options: this._options || [] }, commit); return; }
      this._editing = true;
      this._finish = editText(this, vEl, this.attr("value"), commit, (on) => this.toggleAttribute("editing", on));
    }
  }
  window.AnElement.define(AnField);

  class AnKv extends window.AnElement {
    static tag = "an-kv";
    static observed = ["rows", "mono", "wrap"];
    static css = `
      :host { display: block; }
      .list { display: flex; flex-direction: column; }
      /* 行：[key 左] [铅笔] [撑开] [value 右] [✓✕]，与 an-field 同款就地编辑 */
      .row { display: flex; align-items: center; gap: var(--sp-2); min-height: var(--row); padding: var(--sp-1) var(--pad-row); border-radius: var(--r-btn); transition: background var(--d-fast); }
      .row:hover, .row.editing { background: var(--island-3); }
      .k { min-width: 0; color: var(--ink-2); font-size: var(--t-body); line-height: var(--lh-ui); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
      .grow { flex: 1 1 auto; min-width: var(--zero); }
      .vwrap { min-width: 0; flex: 0 1 auto; display: flex; align-items: center; justify-content: flex-end; }
      .v { min-width: 0; color: var(--ink-3); font-size: var(--t-meta); line-height: var(--lh-ui); font-variant-numeric: tabular-nums; text-align: right; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
      :host([mono]) .v { font-family: var(--mono); }
      /* wrap：长 value 换行多行自适应（observed 有 wrap，此前无 CSS 是死属性）*/
      :host([wrap]) .v { white-space: normal; overflow-wrap: anywhere; text-overflow: clip; text-align: left; }
      /* 铅笔：可编辑行 hover 显、editing 藏（:not(.editing) 直接挡掉 hover 揭示） */
      .pencil { display: none; }
      .row.editable:not(.editing):hover .pencil { display: inline-flex; }
      .row.editing .confirm { display: inline-flex; }
      ${EDIT_CSS}
    `;

    get rows() { return this._data(); }
    set rows(v) { this._rows = (Array.isArray(v) ? v : []).map(normRow); if (this.isConnected) this._render(); }
    attributeChangedCallback(name) { if (name === "rows") this._rows = null; if (this.isConnected) this._render(); }
    _data() {
      if (!this._rows) {
        let raw = [];
        try { raw = JSON.parse(this.attr("rows", "[]")); } catch (_) { raw = []; }
        this._rows = (Array.isArray(raw) ? raw : []).map(normRow);
      }
      return this._rows;
    }

    render() {
      const e = window.anEsc;
      const body = this._data().map((r, i) => {
        const v = r.value == null || r.value === "" ? "—" : r.value;
        const pencil = r.editable ? pencilHtml() : "";
        const confirm = r.editable ? confirmHtml() : "";
        return `<div class="row${r.editable ? " editable" : ""}" data-i="${i}"><span class="k">${e(r.key)}</span>${pencil}<span class="grow"></span><span class="vwrap"><span class="v">${e(v)}</span></span>${confirm}</div>`;
      }).join("");
      return `<div class="list">${body}</div>`;
    }

    hydrate() {
      this._editing = false;
      this.$$(".row.editable").forEach((row) => {
        const i = Number(row.dataset.i);
        const pencil = row.querySelector(".pencil");
        const start = () => {
          if (this._editing) return;
          const vEl = row.querySelector(".v"), rec = this._rows[i];
          const commit = (value) => this._commit(i, value);
          if (rec.editor === "select") { editSelect(this, vEl, rec, commit); return; }
          this._editing = true;
          this._finish = editText(this, vEl, rec.value, commit, (on) => row.classList.toggle("editing", on));
        };
        if (pencil) pencil.addEventListener("click", start);
        const ok = row.querySelector(".confirm .ok"), cancel = row.querySelector(".confirm .cancel");
        if (ok) ok.addEventListener("mousedown", (ev) => { ev.preventDefault(); this._finish && this._finish(true); });
        if (cancel) cancel.addEventListener("mousedown", (ev) => { ev.preventDefault(); this._finish && this._finish(false); });
      });
    }
    _commit(i, value) {
      this._rows[i] = Object.assign({}, this._rows[i], { value });
      this._render();
      this.emit("an-kv-change", { key: this._rows[i].key, value, index: i });
    }
  }
  window.AnElement.define(AnKv);
})();
