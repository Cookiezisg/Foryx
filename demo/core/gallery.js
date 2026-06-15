/* Forgify demo — 入口画廊（核心，index.html 用）。海洋/形态卡 + 读我，全由 manifest 生成。
   加海洋 = manifest append 一行(带 gallery:1)；这里零改。卡跳 standalone || app.html#<id>。 */
(function () {
  const M = window.MANIFEST || [];
  const card = (href, ic, name, desc) => `<a class="g-card" href="${href}">
    <span class="g-row"><span class="g-ic">${icon(ic, 17)}</span><b>${name}</b></span>
    <span class="g-desc">${desc || ''}</span></a>`;

  const demos = M.filter(f => f.gallery).map(f => card(f.standalone || ('app.html#' + f.id), f.icon, f.label, f.desc));
  // 整体 app 入口置顶
  demos.unshift(card('app.html', 'forge', 'Forgify · 完整 app', '一页串起全系统：四 tab 切海洋 + 头像进设置 + 铃铛通知。从这里看整体。'));

  const READS = [
    ['core/contracts.md', 'shield', 'contracts.md', '六契约 + 文件归属：令牌 / 外壳槽 / 组件 API / Intent / Live / 数据。'],
    ['reference.html', 'entities', 'reference.html', '组件库活体规格：每个共享组件的视觉真相。'],
  ];

  const d = document.getElementById('demos'); if (d) d.innerHTML = demos.join('');
  const r = document.getElementById('reads'); if (r) r.innerHTML = READS.map(x => card(...x)).join('');
})();
