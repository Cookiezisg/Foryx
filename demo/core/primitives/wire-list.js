/* Anselm 原语 — <an-wire-list>。可增删的「key → 表达式」接线组（field → CEL 映射的通用承载件）。
   每行 = [字段名 an-input] → [表达式 an-input] [删除]；底部「＋ 添加映射」增行。任一变更收集成 field→expr map，派 composed 'an-wire-change'{map}。
   why：图编辑器节点 input 接线、control 分支 when→port 等都是「可增删的 key→expr 行组」——归一到本件，杜绝各编辑器重搓行布局/回收逻辑。
   API：prop rows（{field:expr} map 或 [{field,expr}] 数组，设入即渲染）；attr keyph/exprph（占位）/ addlabel（增行钮文案，默认「添加映射」）。get rows → 当前收集的 map。 */
(function () {
  class AnWireList extends window.AnElement {
    static tag = "an-wire-list";
    static css = `
      :host { display: block; }
      .list { display: flex; flex-direction: column; gap: var(--grid); }
      .wire { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 2fr) auto; align-items: center; gap: var(--gap-tight); }
      .arr { color: var(--ink-3); }
      .wx { display: grid; place-items: center; flex: none; width: var(--ctl-sm); height: var(--ctl-sm); border-radius: var(--r-tag);
        color: var(--ink-3); transition: background var(--d-fast), color var(--d-fast); }
      .wx:hover { background: var(--island-3); color: var(--ink); }
      .wx svg { width: var(--icon-sm); height: var(--icon-sm); }
      .add { display: inline-flex; align-items: center; gap: var(--gap-tight); height: var(--ctl-sm); padding: 0 var(--btn-pad-x-sm);
        border-radius: var(--r-tag); color: var(--ink-2); font-size: var(--t-meta); transition: background var(--d-fast), color var(--d-fast); }
      .add:hover { background: var(--island-3); color: var(--ink); }
      .add svg { width: var(--icon-sm); height: var(--icon-sm); }
    `;

    set rows(v) {
      if (Array.isArray(v)) this._rows = v.map((r) => ({ field: (r && r.field) || "", expr: (r && r.expr) || "" }));
      else if (v && typeof v === "object") this._rows = Object.keys(v).map((k) => ({ field: k, expr: v[k] == null ? "" : String(v[k]) }));
      else this._rows = [];
      if (this.isConnected) this._render();
    }
    get rows() { return this._map(); }

    render() {
      const e = window.anEsc;
      const kp = this.attr("keyph", "field"), vp = this.attr("exprph", "CEL");
      const list = (this._rows || []).map((r, i) =>
        `<div class="wire" data-i="${i}">`
        + `<an-input class="wk" mono value="${e(r.field)}" placeholder="${e(kp)}"></an-input>`
        + `<span class="arr">→</span>`
        + `<an-input class="wv" mono value="${e(r.expr)}" placeholder="${e(vp)}"></an-input>`
        + `<button type="button" class="wx" data-x="${i}" title="删除映射">${window.icon("close")}</button>`
        + `</div>`).join("");
      return `<div class="list">${list}</div><button type="button" class="add" data-add>${window.icon("enter")}${e(this.attr("addlabel", "添加映射"))}</button>`;
    }

    hydrate() {
      if (!this._rows) this._rows = [];
      const add = this.$("[data-add]");
      if (add) add.addEventListener("click", () => { this._sync(); this._rows.push({ field: "", expr: "" }); this._render(); this._focusLast(); });
      this.$$(".wx").forEach((b) => b.addEventListener("click", () => { this._sync(); this._rows.splice(Number(b.dataset.x), 1); this._render(); this._commit(); }));
      // 失焦回收 + 提交（不每键提交，避免半成品 key 入 map）
      this.$$(".wk, .wv").forEach((inp) => inp.addEventListener("focusout", () => { this._sync(); this._commit(); }));
    }

    _focusLast() { const rows = this.$$(".wire"); const last = rows[rows.length - 1]; const k = last && last.querySelector(".wk"); if (k && k.focus) requestAnimationFrame(() => k.focus()); }
    _sync() {
      this._rows = this.$$(".wire").map((row) => ({
        field: (row.querySelector(".wk").value || "").trim(),
        expr: (row.querySelector(".wv").value || "").trim(),
      }));
    }
    _map() { const o = {}; (this._rows || []).forEach((r) => { if (r.field) o[r.field] = r.expr; }); return o; }
    _commit() { this.emit("an-wire-change", { map: this._map() }); }
  }
  window.AnElement.define(AnWireList);
})();
