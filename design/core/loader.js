/* Foryx demo — 懒加载器（核心）。据 manifest 把海洋的 data → sea/rail 模块按需拉起，幂等。
   点任意导航 → loadFeature(id)：先加载该海洋声明的 data(mock/*)，再加载 sea.js(+rail.js) → 模块顶层自注册。
   于是任意入口都能挂任意海洋——根治「海面待接入」(只有文件真 404 才占位)。 */
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
    const data = (f.data || []).map(d => loadJs(d).catch(() => {}));   // data(mock) 先到，sea/rail 依赖它
    return Promise.all(data).then(() => {
      const ps = [];
      if (f.sea) ps.push(loadJs(f.sea).catch(() => {}));
      if (f.rail) ps.push(loadJs(f.rail).catch(() => {}));
      return Promise.all(ps);
    }).then(() => true);
  }
  window.loader = { loadJs, loadFeature };
})();
