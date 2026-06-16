/* Foryx demo — 入口画廊（核心，index.html 用）。海洋/形态卡 + 读我，全由 manifest 生成。
   加海洋 = manifest append 一行(带 gallery:1)；这里零改。卡跳 standalone || app.html#<id>。 */
(function () {
  const M = window.MANIFEST || [];
  const card = (href, ic, name, desc) => `<a class="g-card" href="${href}">
    <span class="g-row"><span class="g-ic">${icon(ic, 17)}</span><b>${name}</b></span>
    <span class="g-desc">${desc || ''}</span></a>`;

  const demos = M.filter(f => f.gallery).map(f => card(f.standalone || ('app.html#' + f.id), f.icon, f.label, f.desc));
  // 整体 app 入口置顶
  demos.unshift(card('app.html', 'forge', 'Foryx · 完整 app', '完整产品形态：四 tab 切海洋 + 头像进设置 + 铃铛通知。'));

  const READS = [
    ['SPEC.md', 'shield', 'SPEC.md', '布局语法、token、三岛、按钮、滚动和无边界设计规范。'],
    ['reference.html', 'entities', 'reference.html', 'Foryx 原语活体规格台。'],
  ];

  const d = document.getElementById('demos'); if (d) d.innerHTML = demos.join('');
  const r = document.getElementById('reads'); if (r) r.innerHTML = READS.map(x => card(...x)).join('');
})();
