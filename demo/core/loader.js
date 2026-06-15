/* Forgify demo — 懒加载器（核心）。按 manifest 把海洋的 sea/rail 模块按需拉起，幂等。
   配合 boot/sidebar：点任意导航 → loadFeature(id) 载入其 sea.js(+rail.js) → 模块顶层自注册(Shell.registerOcean / SideBar.register)。
   于是任意入口(app/单页预览/reference)都能挂任意海洋——根治「海面待接入」(只有文件真 404 才占位)。 */
(function () {
  const jsCache = {};
  function loadJs(src) {
    if (jsCache[src]) return jsCache[src];
    jsCache[src] = new Promise((res, rej) => {
      const s = document.createElement('script');
      s.src = src; s.onload = () => res(src);
      s.onerror = () => { delete jsCache[src]; rej(new Error('load fail ' + src)); };
      document.head.appendChild(s);
    });
    return jsCache[src];
  }
  function loadFeature(id) {
    const f = (window.MANIFEST || []).find(x => x.id === id);
    if (!f) return Promise.resolve(false);
    const ps = [];
    if (f.sea) ps.push(loadJs(f.sea).catch(() => {}));
    if (f.rail) ps.push(loadJs(f.rail).catch(() => {}));
    return Promise.all(ps).then(() => true);
  }
  window.loader = { loadJs, loadFeature };
})();
