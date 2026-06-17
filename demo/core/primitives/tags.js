/* Anselm 原语 A4 — <an-tags>。可增删的 pill 集 + 末尾虚线 add 入口（+ 可选 health 点）。
   why：收掉 entities 海洋散落的 .eo-tags/.eo-tag/add/none 各处副本，归一成一个原语。
   形态：药丸行 = 可选 health 点 + 标签 + 内联 × 删；末尾虚线 add 药丸，点开内联输入（非 window.prompt——
     浏览器原生弹窗在无头/嵌入场景不可用，且破坏视觉，故走 shadow 内联 input 同 sidebar-list 过滤行）。
   单个 pill 外形复用 badge 语汇（药丸圆角 + 海岸线描边 + gap-tight）；health 点复用 <an-status-dot>（状态归一单一路径）。
   数据走 JS 属性（对象数组不经线缆）：items=[{label,health?}|string]；mode single|multi（single 加入即替换）。
   health ∈ ok|bad → 映射 status-dot 的 done|err（语义状态单源）。
   增删对外派 composed 'an-tags-change'{items}。 */
(function () {
  // health 词表 → status-dot 语义状态（绿/红的单一翻译，不在本原语另起色阶）
  const HEALTH_STATE = { ok: "done", bad: "err" };

  class AnTags extends window.AnElement {
    static tag = "an-tags";
    static observed = ["mode", "add-label"];
    static css = `
      :host { display: block; }
      .tags { display: flex; flex-wrap: wrap; gap: var(--gap-tight); align-items: center; }
      .tag {
        display: inline-flex; align-items: center; gap: var(--grid);
        padding: calc(var(--grid) / 2) var(--gap-tight) calc(var(--grid) / 2) var(--badge-pad-x);
        border-radius: var(--r-pill); border: var(--hairline) solid var(--line);
        background: var(--island); color: var(--ink-2); font-size: var(--t-meta);
        animation: pop var(--d-mid) var(--ease-spring);
      }
      .tag an-status-dot { line-height: 0; }
      .x {
        display: grid; place-items: center; width: var(--icon); height: var(--icon);
        border-radius: var(--r-pill); color: var(--ink-3); cursor: pointer;
        transition: background var(--d-fast), color var(--d-fast);
      }
      .x svg { width: var(--icon-sm); height: var(--icon-sm); }
      .x:hover { background: var(--danger-soft); color: var(--danger); }

      /* add 药丸：虚线描边邀约；点开内联输入（无 prompt） */
      .add {
        display: inline-flex; align-items: center; gap: var(--grid);
        padding: calc(var(--grid) / 2) var(--badge-pad-x);
        border-radius: var(--r-pill); border: var(--hairline) dashed var(--line);
        color: var(--ink-3); font-size: var(--t-meta); cursor: pointer;
        transition: color var(--d-fast), border-color var(--d-fast);
      }
      .add svg { width: var(--icon-sm); height: var(--icon-sm); }
      .add:hover { color: var(--ink); border-color: var(--line-strong); }

      /* 内联输入：药丸态文本框，提交即收为新 tag */
      .input {
        min-width: 0; width: var(--input-min); border: var(--zero); background: none;
        font: inherit; font-size: var(--t-meta); color: var(--ink);
      }
      .input::placeholder { color: var(--ink-3); }
      .input:focus { outline: none; }

      .none { font-size: var(--t-body); color: var(--ink-3); }

      @keyframes pop {
        from { transform: scale(.8); opacity: 0; }
        to   { transform: scale(1); opacity: 1; }
      }
    `;

    // items 走 JS 属性（对象数组不经线缆字符串）；设入即归一为内部记录并重渲染
    get items() { return this._items.map((r) => (r.health ? { label: r.label, health: r.health } : r.label)); }
    set items(v) { this._items = (v || []).map((it) => this._norm(it)); if (this.isConnected) this._render(); }

    // 归一：字符串 → {label}；对象取 label/health
    _norm(it) {
      if (typeof it === "string") return { label: it, health: null };
      return { label: (it && it.label) || "", health: (it && it.health) || null };
    }

    render() {
      const e = window.anEsc;
      this._items = this._items || [];
      const add = this.attr("add-label", "add");
      const pills = this._items.length
        ? this._items.map((r, i) => this._pillHtml(r, i)).join("")
        : `<span class="none">— 无 —</span>`;
      return `<div class="tags">${pills}` +
        `<span class="add" role="button">${window.icon("plus", 11)}<span>${e(add)}</span></span>` +
        `</div>`;
    }

    // 单个 pill：health 点（复用 status-dot）+ 标签 + × 删
    _pillHtml(r, i) {
      const e = window.anEsc;
      const dot = r.health
        ? `<an-status-dot state="${HEALTH_STATE[r.health] || "idle"}"></an-status-dot>`
        : "";
      return `<span class="tag" data-i="${i}">${dot}<span>${e(r.label)}</span>` +
        `<span class="x" role="button">${window.icon("close", 11)}</span></span>`;
    }

    hydrate() {
      // × 删：移除该记录 → 重渲染 → 派 change
      this.$$(".tag .x").forEach((x) => {
        x.addEventListener("click", () => {
          const i = Number(x.closest(".tag").dataset.i);
          this._items.splice(i, 1);
          this._render();
          this._change();
        });
      });
      // add：点药丸 → 原地换成内联输入；Enter 提交 / Escape·失焦取消
      const add = this.$(".add");
      if (add) add.addEventListener("click", () => this._openInput());
    }

    _openInput() {
      const add = this.$(".add");
      if (!add) return;
      const e = window.anEsc;
      add.outerHTML = `<input class="input" placeholder="${e(this.attr("add-label", "add"))}">`;
      const input = this.$(".input");
      input.focus();
      // done 守卫：_render 拆除焦点 input 会触发尾随 blur —— 防其二次 commit（多模式重复加标签 / Escape 反被提交）
      let done = false;
      const commit = () => {
        if (done) return; done = true;
        const val = input.value.trim();
        // single：加入即替换既有；multi：追加
        if (val) {
          if (this.attr("mode") === "single") this._items = [{ label: val, health: null }];
          else this._items.push({ label: val, health: null });
        }
        this._render();
        if (val) this._change();
      };
      input.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter") { ev.preventDefault(); commit(); }
        else if (ev.key === "Escape") { done = true; this._render(); }   // 取消：置位，挡掉尾随 blur 提交
      });
      input.addEventListener("blur", commit);
    }

    _change() { this.emit("an-tags-change", { items: this.items }); }
  }
  window.AnElement.define(AnTags);
})();
