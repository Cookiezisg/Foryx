/* Anselm 原语 — <an-version-diff lang range note bare>。版本 diff（单框 unified diff，非左右双栏）：
   旧→新逐行 LCS，增行 --ok 绿、删行 --danger 红，同一代码框内上下堆叠（对齐 GitHub unified）。
   行内语法着色【只】走 AnCodeEditor.highlight（唯一 tokenizer），缺失裸转义兜底——不另起第二套高亮。
   API：prop before/after（旧/新文本，设入即重渲算 LCS + 计 +N/−N）；attr lang（透传 highlight）/ range（顶栏 "v3 → v4"）/ note（变更说明）/ bare（隐顶栏，内联场景）。
        最早版本 before 留空 → 整段以 ctx 渲染（着色不染增删）。静态 AnVersionDiff.lineDiff(a,b)→[op,text][] 纯算法可独立调。 */
(function () {
  // 行级 diff（LCS）——逆向 DP 求最长公共子序列、回溯出 ctx/add/del 序列。纯算法、无副作用、对外静态可调。
  function lineDiff(a, b) {
    const A = String(a == null ? "" : a).split("\n"), B = String(b == null ? "" : b).split("\n"), m = A.length, n = B.length;
    const dp = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0));
    for (let i = m - 1; i >= 0; i--) for (let j = n - 1; j >= 0; j--) dp[i][j] = A[i] === B[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
    const o = []; let i = 0, j = 0;
    while (i < m && j < n) { if (A[i] === B[j]) { o.push(["ctx", A[i]]); i++; j++; } else if (dp[i + 1][j] >= dp[i][j + 1]) o.push(["del", A[i++]]); else o.push(["add", B[j++]]); }
    while (i < m) o.push(["del", A[i++]]); while (j < n) o.push(["add", B[j++]]); return o;
  }
  // 行内着色单源：优先 AnCodeEditor.highlight(code, lang)（签名带 lang，务必透传）；缺失裸转义兜底。
  const hl = (code, lang) => (window.AnCodeEditor && window.AnCodeEditor.highlight) ? window.AnCodeEditor.highlight(code, lang) : window.anEsc(code);

  class AnVersionDiff extends window.AnElement {
    static tag = "an-version-diff";
    static observed = ["lang", "range", "note", "bare"];
    static css = `
      :host { display: block; }
      /* 单框：对齐 code-editor .code 语汇（描边 + 圆角 + island 底，框内无横线）。描边走 inset box-shadow 而非 border——半透明 border 圆角四角 1px 自叠加成"灰尖"，inset 环均匀无叠加。 */
      .vd { display: flex; flex-direction: column; min-width: 0;
        box-shadow: inset 0 0 0 var(--hairline) var(--line); border-radius: var(--r-card); background: var(--island); overflow: hidden; }
      :host([bare]) .vd { box-shadow: none; border-radius: 0; background: transparent; }
      /* 顶栏：版本范围（mono）+ 说明 + 增删计数 */
      .cap { flex: none; display: flex; align-items: center; gap: var(--sp-2); padding: var(--sp-2) var(--sp-3) 0;
        font-size: var(--t-meta); color: var(--ink-3); }
      :host([bare]) .cap { display: none; }
      .range { font-family: var(--mono); font-variant-numeric: tabular-nums; color: var(--ink-2); }
      .note { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .grow { flex: 1; }
      .pm { flex: none; font-family: var(--mono); font-variant-numeric: tabular-nums; }
      .pm .add { color: var(--ok); } .pm .del { color: var(--danger); }
      /* diff 主体：横向可滚，等宽，逐行三列网格 [行号 符号 代码] */
      .body { overflow-x: auto; font-family: var(--mono); font-size: var(--t-meta); line-height: var(--lh-prose);
        padding: var(--sp-2) 0 var(--sp-3); }
      .dl { display: grid; grid-template-columns: var(--trail) var(--trail) minmax(0, 1fr); align-items: baseline;
        min-width: 100%; padding: 0 var(--sp-3); }
      .dl.add { background: var(--ok-soft); } .dl.del { background: var(--danger-soft); }
      .ln { text-align: right; padding-right: var(--sp-2); color: var(--ink-3); opacity: .5; user-select: none; }
      .sg { text-align: center; user-select: none; }
      .dl.add .sg, .dl.add .ct { color: var(--ok); } .dl.del .sg, .dl.del .ct { color: var(--danger); }
      .dl.ctx .ct { color: var(--ink-2); }
      .ct { white-space: pre; }
      .cd-com { color: var(--cd-com); font-style: italic; } .cd-kw { color: var(--cd-kw); }
      .cd-str { color: var(--cd-str); } .cd-num { color: var(--cd-num); } .cd-fn { color: var(--cd-fn); }
      .cd-arg { color: var(--accent); font-weight: 600; }
    `;

    set before(v) { this._before = v; if (this.isConnected) this._render(); }
    get before() { return this._before; }
    set after(v) { this._after = v; if (this.isConnected) this._render(); }
    get after() { return this._after; }

    render() {
      const e = window.anEsc, lang = this.attr("lang");
      const before = this._before, after = this._after == null ? "" : String(this._after);
      let rows, ad = 0, de = 0, ln = 0;
      if (before == null || before === "") {
        // 最早版本：无更旧可比——整段 ctx（着色不染增删）
        rows = after.split("\n").map((l) => `<div class="dl ctx"><span class="ln">${++ln}</span><span class="sg"></span><span class="ct">${hl(l, lang)}</span></div>`).join("");
      } else {
        rows = lineDiff(before, after).map(([op, t]) => {
          const sg = op === "add" ? "+" : op === "del" ? "−" : " ";
          if (op === "add") ad++; else if (op === "del") de++;
          return `<div class="dl ${op}"><span class="ln">${op === "del" ? "" : ++ln}</span><span class="sg">${sg}</span><span class="ct">${hl(t, lang)}</span></div>`;
        }).join("");
      }
      const range = this.attr("range") ? `<span class="range">${e(this.attr("range"))}</span>` : "";
      const note = this.attr("note") ? `<span class="note">${e(this.attr("note"))}</span>` : "";
      const pm = (ad || de) ? `<span class="pm"><span class="add">+${ad}</span> <span class="del">−${de}</span></span>` : "";
      return `<div class="vd"><div class="cap">${range}${note}<span class="grow"></span>${pm}</div><div class="body">${rows}</div></div>`;
    }
  }
  AnVersionDiff.lineDiff = lineDiff;
  window.AnElement.define(AnVersionDiff);
  window.AnVersionDiff = AnVersionDiff;
})();
