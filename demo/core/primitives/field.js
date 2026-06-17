/* Anselm 原语 C2 · C3 — 键值叶子两式（同域一文件，共用块模型 + 内生编辑）：
   · <an-field label hint value editable editor>  键值大行：label + 值；值在受约束 1fr 列内换行/截断，绝不溢出/重叠/挤扁 label。自适应高度（贴内容）。
   · <an-kv rows mono wrap>                        紧凑定义列表：值右贴边；【过长自动换行】（per-row 自检），不溢出不重叠。
   块模型：列轨 = [key 内容宽 minmax(0,auto) | value 受约束 minmax(0,1fr) | 编辑槽 auto]，每槽 min-width:0 → 长值在自己列消化。
   编辑是【内生】能力且【与标题一致】：值【原地】变 contenteditable（同字号/同盒/同位置，零偏移、不改高），尾槽铅笔→✓/✕；
     Enter/✓/失焦提交、Esc/✕/空值还原。枚举值仍用 <an-dropdown>（离散选择）。Field 派 'an-field-change'；KV 派 'an-kv-change'。done 一次性守卫。 */
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

  // 内生就地编辑（自由文本）：值槽原地 contenteditable，同盒同字号。返回 finish(commit) 供尾槽 ✓/✕ 调用。done 一次性。
  // realVal=真值（显示为 — 占位时编辑前清空）；commit(text) 写回+重渲+派事件；取消/未改→host._render()。onState(editing) 切尾槽铅笔↔✓✕。
  function editText(host, valueEl, realVal, commit, onState) {
    const orig = realVal == null ? "" : String(realVal);
    valueEl.textContent = orig;
    let done = false;
    valueEl.setAttribute("contenteditable", "plaintext-only");
    valueEl.classList.add("editing");
    if (onState) onState(true);
    const sel = window.getSelection();
    if (sel) { const r = document.createRange(); r.selectNodeContents(valueEl); sel.removeAllRanges(); sel.addRange(r); }
    valueEl.focus();
    const finish = (ok) => {
      if (done) return; done = true;
      valueEl.removeEventListener("keydown", onKey); valueEl.removeEventListener("blur", onBlur);
      valueEl.removeAttribute("contenteditable"); valueEl.classList.remove("editing");
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

  // 编辑尾槽片段（与 ocean-header 标题编辑【同款】：铅笔 hover 现，编辑态换 ✓/✕；同尺寸不引发偏移）。
  function actsHtml() {
    return `<span class="acts">`
      + `<button type="button" class="a-btn a-edit" data-edit aria-label="编辑">${window.icon("edit")}</button>`
      + `<button type="button" class="a-btn a-save" data-save aria-label="保存">${window.icon("check")}</button>`
      + `<button type="button" class="a-btn a-cancel" data-cancel aria-label="取消">${window.icon("close")}</button>`
      + `</span>`;
  }
  /* 编辑尾槽【绝对浮层】（不占网格列、不偷值宽——值始终满列，长内容才换行不被挤）；hover/编辑态显，背景接 island-3 与行 hover 底融为一体。 */
  const ACTS_CSS = `
    .acts { position: absolute; top: 50%; right: var(--pad-row); transform: translateY(-50%); display: inline-flex; align-items: center; gap: var(--gap-tight);
      background: var(--island-3); border-radius: var(--r-tag); padding-left: var(--grid); z-index: 1; }
    .a-btn { display: none; place-items: center; flex: none; width: var(--ctl-sm); height: var(--ctl-sm); border-radius: var(--r-tag);
      color: var(--ink-3); transition: background var(--d-fast), color var(--d-fast); }
    .a-btn svg { width: var(--icon-sm); height: var(--icon-sm); }
    .a-edit:hover, .a-cancel:hover { background: var(--island-4); color: var(--ink); }
    .a-save { color: var(--accent); } .a-save:hover { background: var(--accent-soft); color: var(--accent); }
    .v.editing { outline: none; box-shadow: inset 0 0 0 var(--line-2) var(--accent-line); border-radius: var(--r-tag); background: var(--accent-soft); cursor: text; }
  `;

  class AnField extends window.AnElement {
    static tag = "an-field";
    static observed = ["label", "hint", "value", "editable", "editor", "wrap"];
    static css = `
      :host { display: block; }
      /* 块模型 + 自适应高度（贴内容、不留空白）：[label 内容宽 | value 满 1fr 列]，编辑尾槽绝对浮层不偷值宽（短值单行、长值才换行） */
      .field {
        position: relative; display: grid; grid-template-columns: minmax(0, auto) minmax(0, 1fr); align-items: baseline; column-gap: var(--sp-4);
        padding: var(--sp-2) var(--pad-row); border-radius: var(--r-btn); transition: background var(--d-fast);
      }
      :host(:hover) .field, :host([editable]:focus-within) .field { background: var(--island-3); }
      .l { min-width: 0; align-self: baseline; }
      .k { min-width: 0; font-size: var(--t-body); color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
      .hint { min-width: 0; font-size: var(--t-meta); color: var(--ink-3); line-height: var(--lh-ui); margin-top: calc(var(--grid) / 2); overflow-wrap: anywhere; }
      .c { min-width: 0; display: block; }
      .v { min-width: 0; display: block; font-size: var(--t-body); color: var(--ink-2); white-space: normal; overflow-wrap: anywhere; }
      :host([editable]:hover) .a-edit, :host([editable]:focus-within) .a-edit { display: grid; }
      .acts.editing .a-edit { display: none; }
      .acts.editing .a-save, .acts.editing .a-cancel { display: grid; }
      ${ACTS_CSS}
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
      const acts = editableText ? actsHtml() : "";
      return `<div class="field"><div class="l"><div class="k">${label}</div>${hint}</div><div class="c">${control}</div>${acts}</div>`;
    }

    hydrate() {
      this._editing = false;  // 每次重渲解锁（编辑收尾必经重渲）——配合 _startEdit 守卫挡快速双击
      const edit = this.$(".a-edit"); if (!edit) return;
      edit.addEventListener("click", () => this._startEdit());
    }
    _startEdit() {
      if (this._editing) return;  // 守卫：编辑中再点铅笔不重入（否则两套 editText 监听器抢同一 .v、收尾互踩）
      const vEl = this.$(".v"), acts = this.$(".acts"); if (!vEl) return;
      this._editing = true;
      const commit = (value) => { this.setAttribute("value", value); this.emit("an-field-change", { label: this.attr("label"), value }); };
      if ((this.attr("editor") || "input") === "select") { editSelect(this, vEl, { value: this.attr("value"), options: this._options || [] }, commit); return; }
      const finish = editText(this, vEl, this.attr("value"), commit, (on) => acts && acts.classList.toggle("editing", on));
      this.$(".a-save").addEventListener("mousedown", (ev) => { ev.preventDefault(); finish(true); });
      this.$(".a-cancel").addEventListener("mousedown", (ev) => { ev.preventDefault(); finish(false); });
    }
  }
  window.AnElement.define(AnField);

  class AnKv extends window.AnElement {
    static tag = "an-kv";
    static observed = ["rows", "mono", "wrap"];
    static css = `
      :host { display: block; }
      .list { display: flex; flex-direction: column; }
      /* 块模型：[key 内容宽 | value 满 1fr 列]，编辑尾槽绝对浮层不偷值宽；value 右贴边单行，【溢出自检后逐行转多行左对齐】（.row.w 类） */
      .row {
        position: relative; display: grid; grid-template-columns: minmax(0, auto) minmax(0, 1fr); align-items: baseline;
        column-gap: var(--sp-3); min-height: var(--row); padding: var(--sp-1) var(--pad-row);
        border-radius: var(--r-btn); transition: background var(--d-fast);
      }
      .row:hover, .row.editable:focus-within { background: var(--island-3); }
      .k {
        min-width: 0; display: inline-flex; align-items: baseline; gap: var(--gap-tight);
        color: var(--ink-2); font-size: var(--t-body); line-height: var(--lh-ui); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
      }
      .v {
        min-width: 0; justify-self: stretch; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        color: var(--ink-3); font-size: var(--t-meta); line-height: var(--lh-ui);
        font-variant-numeric: tabular-nums; text-align: right;
      }
      :host([mono]) .v { font-family: var(--mono); }
      /* 过长值：转多行左对齐换行（自检命中 .row.w 或全件 [wrap]）——在 1fr 列内消化，绝不溢出/重叠 key */
      :host([wrap]) .row, .row.w { align-items: start; }
      :host([wrap]) .v, .row.w .v { white-space: normal; overflow: visible; text-overflow: clip; overflow-wrap: anywhere; text-align: left; }
      /* kv 行编辑槽走 ACTS 同款；编辑触发与显示对齐 Field/标题（hover 现铅笔） */
      .row.editable:hover .a-edit, .row.editable:focus-within .a-edit { display: grid; }
      ${ACTS_CSS}
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
        const key = `<span class="k"><span class="kt">${e(r.key)}</span></span>`;
        const acts = r.editable ? actsHtml() : "";
        return `<div class="row${r.editable ? " editable" : ""}" data-i="${i}">${key}<span class="v">${e(v)}</span>${acts}</div>`;
      }).join("");
      return `<div class="list">${body}</div>`;
    }

    hydrate() {
      this._editing = false;  // 每次重渲解锁（编辑收尾必经重渲）——配合 start 守卫挡快速双击
      // 过长值自检 → 逐行转多行（自适应换行）：rAF 等布局后量 scrollWidth，超列宽即给该行加 .w
      this._autowrap();
      this.$$('.row.editable').forEach((row) => {
        const i = Number(row.dataset.i);
        const start = () => {
          if (this._editing) return;  // 守卫：编辑中再点铅笔不重入（含同行双击）
          const vEl = row.querySelector(".v"), acts = row.querySelector(".acts"), rec = this._rows[i];
          const commit = (value) => this._commit(i, value);
          if (rec.editor === "select") { editSelect(this, vEl, rec, commit); return; }
          this._editing = true;
          const finish = editText(this, vEl, rec.value, commit, (on) => acts && acts.classList.toggle("editing", on));
          row.querySelector(".a-save").addEventListener("mousedown", (ev) => { ev.preventDefault(); finish(true); });
          row.querySelector(".a-cancel").addEventListener("mousedown", (ev) => { ev.preventDefault(); finish(false); });
        };
        const eb = row.querySelector(".a-edit"); if (eb) eb.addEventListener("click", start);
      });
    }
    // 过长值自检 → 给该行加 .w 转多行（value scrollWidth 超列宽即溢出）。idempotent（add 不重复）；
    // 多档 setTimeout 兜底惰性 tab/段布局时机不定，确保拿到真实宽度后落地。
    _autowrap() {
      if (this.has("wrap")) return;
      const apply = () => {
        if (!this.isConnected) return false;
        if (this.getBoundingClientRect().width < 40) return false;
        this.$$(".row").forEach((row) => { const v = row.querySelector(".v"); if (v && v.scrollWidth > v.clientWidth + 1) row.classList.add("w"); });
        return true;
      };
      if (!apply()) requestAnimationFrame(apply);
      [80, 250, 600].forEach((ms) => setTimeout(apply, ms));
    }
    _commit(i, value) {
      this._rows[i] = Object.assign({}, this._rows[i], { value });
      this._render();
      this.emit("an-kv-change", { key: this._rows[i].key, value, index: i });
    }
  }
  window.AnElement.define(AnKv);
})();
