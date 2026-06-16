/* Foryx demo — 组件 thin-table（细线表格；收掉 entities 海洋 .eo-tbl 的 table() 副本）。
   契约：组件 = 工厂函数 → 元素并 append 到 host；自载同名 .css；只读令牌；fg- 前缀；不碰别的海洋。
   细线、无斑马、无填充（对齐 documents 美学，零灰盒）；表头粗线、行间细线、末行无线。
   API：ThinTable.table(host, cols, rows) → el。cols=表头串数组（转义）；
   rows=[[cellHtml,...]]，cell 为已构建的 HTML（不转义，调用方负责）；
   行可带 {_cls} 加类（如 clk 可点 / muted）；cell 传 {edit:true,html} 渲染 contenteditable 可改格。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const esc = s => String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));

  // 单格 → <td>。{edit:true,html} 渲染为 contenteditable（拼写检查关）；否则 cell 即原 HTML。
  function cell(c) {
    if (c && typeof c === 'object' && c.edit) return `<td contenteditable="true" spellcheck="false">${c.html != null ? c.html : ''}</td>`;
    return `<td>${c == null ? '' : c}</td>`;
  }

  function table(host, cols, rows) {
    const t = document.createElement('table');
    t.className = 'fg-thin-table';
    t.innerHTML =
      `<thead><tr>${cols.map(c => `<th>${esc(c)}</th>`).join('')}</tr></thead>` +
      `<tbody>${rows.map(r => `<tr${r._cls ? ` class="${r._cls}"` : ''}>${r.map(cell).join('')}</tr>`).join('')}</tbody>`;
    if (host) host.appendChild(t);
    return t;
  }

  window.ThinTable = { table };
})();
