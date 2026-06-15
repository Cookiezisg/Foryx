/* Forgify demo — 启动编排（核心）。读 manifest 选默认海洋(或 location.hash 深链 app.html#scheduler)，挂载。
   外壳/侧栏在脚本加载时已自初始化；boot 只决定「首屏挂哪个」+ 响应 hash 切换。~20 行。 */
(function () {
  const M = window.MANIFEST || [];
  function startId() {
    const hash = (location.hash || '').replace('#', '').trim();
    if (M.find(f => f.id === hash && (f.nav || f.axis))) return hash;
    return (M.find(f => f.default) || M.find(f => f.nav) || {}).id;
  }
  function go() { const id = startId(); if (window.Shell && Shell.toOcean && id) Shell.toOcean(id); }
  go();
  window.addEventListener('hashchange', go);   // 深链/画廊跳转即时切
})();
