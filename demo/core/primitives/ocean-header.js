/* Anselm 原语 D5 — <an-ocean-header crumb title editable>。海洋页头：面包屑 + 大标题 + 可选 meta + 右侧动作。
   坐于海面（无卡），与正文段（白岛）分层；页头 = 你在哪（面包屑）/ 这是什么（标题）/ 附注（meta）/ 能做什么（动作）四件事，各有锚位与 token 间距。
   crumb：'|' 分隔层级串，自动插 / 分隔符。槽：slot[name=actions] 顶行右动作；slot[name=meta] meta 行。
   editable：标题【内生】就地改名——hover 现铅笔 → 点进编辑态：<h1> 本体原地变 contenteditable（保持 h2 字号/盒/位置，绝不缩成 input、零页面偏移），
     铅笔位换 ✓/✕；Enter/✓/失焦提交、Esc/✕/空值还原。派 composed 'an-title-change'{value,prev}（消费方落库）。done 一次性守卫。 */
(function () {
  class AnOceanHeader extends window.AnElement {
    static tag = "an-ocean-header";
    static observed = ["crumb", "title", "editable"];
    static css = `
      :host { display: block; }
      .oh { padding-bottom: var(--sp-6); }
      .top { display: flex; align-items: center; gap: var(--sp-3); min-height: var(--ctl); }
      .crumb {
        flex: 1; min-width: 0; display: flex; align-items: center; gap: var(--gap-tight);
        font-size: var(--t-meta); color: var(--ink-3); overflow: hidden; white-space: nowrap;
      }
      .sep { color: var(--line-strong); }
      .actions { flex: none; display: flex; align-items: center; gap: var(--sp-2); }
      .meta {
        display: flex; align-items: center; gap: var(--sp-4);
        font-size: var(--t-meta); color: var(--ink-3); flex-wrap: wrap;
      }
      ::slotted(an-badge) { height: var(--trail); }

      /* 标题行：h1（吃富余、可换行）+ 编辑动作槽（固定）。h1 字号/盒在显示与编辑态【完全一致】 */
      .title-row { display: flex; align-items: baseline; gap: var(--sp-2); margin: var(--sp-2) 0; min-width: 0; }
      .title {
        min-width: 0; flex: 1 1 auto; overflow-wrap: anywhere;
        font-size: var(--t-h2); font-weight: 600; line-height: var(--lh-tight); letter-spacing: 0; margin: 0;
      }
      .title.editing { outline: none; box-shadow: 0 0 0 var(--line-2) var(--accent-line); border-radius: var(--r-tag);
        background: var(--accent-soft); cursor: text; }
      .t-acts { flex: none; display: inline-flex; align-items: center; gap: var(--gap-tight); align-self: center; }
      .t-btn { display: none; place-items: center; flex: none; width: var(--ctl-sm); height: var(--ctl-sm); border-radius: var(--r-tag);
        color: var(--ink-3); transition: background var(--d-fast), color var(--d-fast); }
      .t-btn svg { width: var(--icon-sm); height: var(--icon-sm); }
      .title-row:hover .t-edit, .title-row:focus-within .t-edit, .t-acts.editing .t-save, .t-acts.editing .t-cancel { display: grid; }
      .t-acts.editing .t-edit { display: none; }
      .t-edit:hover, .t-cancel:hover { background: var(--island-3); color: var(--ink); }
      .t-save { color: var(--accent); }
      .t-save:hover { background: var(--accent-soft); color: var(--accent); }
    `;
    render() {
      const e = window.anEsc;
      const parts = (this.attr("crumb") || "").split("|").map((s) => s.trim()).filter(Boolean);
      const crumb = parts.length ? parts.map((c, i) => (i ? `<span class="sep">/</span>` : "") + `<span>${e(c)}</span>`).join("") : "";
      const title = this.attr("title");
      let titleEl = "";
      if (title != null) {
        const h1 = `<h1 class="title">${e(title)}</h1>`;
        titleEl = this.has("editable")
          ? `<div class="title-row"><h1 class="title">${e(title)}</h1>`
            + `<span class="t-acts">`
            + `<button type="button" class="t-btn t-edit" data-edit aria-label="编辑标题">${window.icon("edit")}</button>`
            + `<button type="button" class="t-btn t-save" data-save aria-label="保存">${window.icon("check")}</button>`
            + `<button type="button" class="t-btn t-cancel" data-cancel aria-label="取消">${window.icon("close")}</button>`
            + `</span></div>`
          : h1;
      }
      return `<header class="oh">`
        + `<div class="top"><div class="crumb">${crumb}</div><div class="actions"><slot name="actions"></slot></div></div>`
        + titleEl
        + `<div class="meta"><slot name="meta"></slot></div>`
        + `</header>`;
    }

    hydrate() {
      const edit = this.$(".t-edit"); if (edit) edit.addEventListener("click", () => this._beginTitleEdit());
    }

    // 内生就地编辑：<h1> 本体原地 contenteditable（同字号同盒），done 一次性守卫（✓/✕ mousedown 与 blur 双触只生效一次）。
    _beginTitleEdit() {
      const h1 = this.$(".title"), acts = this.$(".t-acts"); if (!h1 || !acts) return;
      const orig = this.attr("title", "");
      let done = false;
      h1.setAttribute("contenteditable", "plaintext-only");
      h1.classList.add("editing"); acts.classList.add("editing");
      const sel = window.getSelection();
      if (sel) { const r = document.createRange(); r.selectNodeContents(h1); sel.removeAllRanges(); sel.addRange(r); }
      h1.focus();
      const finish = (ok) => {
        if (done) return; done = true;
        h1.removeEventListener("keydown", onKey); h1.removeEventListener("blur", onBlur);
        const v = (h1.textContent || "").trim();
        if (ok && v && v !== orig) { this.setAttribute("title", v); this.emit("an-title-change", { value: v, prev: orig }); }
        else this._render();   // 取消 / 空标题（必填，唯一物理校验）/ 未改 → 回显
      };
      const onKey = (ev) => {
        if (ev.key === "Escape") { ev.preventDefault(); finish(false); }
        else if (ev.key === "Enter") { ev.preventDefault(); finish(true); }
      };
      const onBlur = () => finish(true);
      h1.addEventListener("keydown", onKey);
      h1.addEventListener("blur", onBlur);
      // ✓/✕：mousedown 抢在 h1 blur 之前定调（取消优先回滚），done 守卫兜双触
      this.$(".t-save").addEventListener("mousedown", (ev) => { ev.preventDefault(); finish(true); });
      this.$(".t-cancel").addEventListener("mousedown", (ev) => { ev.preventDefault(); finish(false); });
    }
  }
  window.AnElement.define(AnOceanHeader);
})();
