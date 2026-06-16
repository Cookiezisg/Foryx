/* Foryx demo — 外壳框架（核心，只读消费；勿在海洋里改本文件）。
   搭圆角浮窗、开三槽(#left 侧栏 / #sea 海面 / #body 右岛宿主)、提供海洋挂载 API + 主题 + 滚动自隐。
   海洋只需：Shell.registerOcean(id,{crumb,build(sea)})；选中走 Intent、不在 Shell 上挂 openX。
   ⚠ 右岛由海洋经组件 right-island 自渲染进 Shell.body，外壳 mount 时据 data-ocean-right 清理。 */
(function () {
  const $ = (s, r = document) => r.querySelector(s);
  document.body.innerHTML = `
    <div class="win">
      <div class="body" id="body">
        <aside class="side" id="left"></aside>
        <main class="main">
          <div class="main-head">
            <span id="head-lead"></span>
            <span class="grow"></span>
            <span id="head-extra"></span>
          </div>
          <div class="sea" id="sea"></div>
        </main>
      </div>
    </div>`;

  // 外壳不再显示面包屑/品牌盾牌/主题钮——海洋自管头部（head-lead/head-extra）；主题归设置海洋。

  // 滚动自隐：捕获阶段抓全文档滚动 → 置位 html[data-scrolling]，停 700ms 清
  let st;
  document.addEventListener('scroll', () => {
    document.documentElement.dataset.scrolling = '';
    clearTimeout(st); st = setTimeout(() => delete document.documentElement.dataset.scrolling, 700);
  }, true);

  window.Shell = {
    $, oceans: {}, _back: null,
    get left() { return $('#left'); },
    get sea() { return $('#sea'); },
    get body() { return $('#body'); },
    get headLead() { return $('#head-lead'); },
    get sideWidth() { return parseFloat(getComputedStyle($('#left')).width) || 0; },
    headExtra(html) { const s = $('#head-extra'); s.innerHTML = html; return s; },
    crumb() { /* 外壳不再显示面包屑；海洋自管头部。保留空方法供海洋调用不报错。 */ },
    registerOcean(id, def) { this.oceans[id] = def; },
    mount(id) {
      const o = this.oceans[id];
      if (!o) return console.warn('[Shell] ocean not registered:', id);
      this._ocean = id;   // 当前内容海洋（Intent.select 据此免在同海洋内重挂、保侧栏高亮）
      this.sea.innerHTML = '';
      $('#head-extra').innerHTML = '';
      this.body.querySelectorAll('[data-ocean-right]').forEach(el => el.remove());
      if (o.crumb) this.crumb(o.crumb);
      o.build(this.sea);
    },
    returnTo(id) { if (this.toOcean) this.toOcean(id); },
  };
})();
