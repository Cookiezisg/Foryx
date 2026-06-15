/* Forgify demo — 外壳框架（核心，只读消费；勿在海洋里改本文件）。
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
            <div class="crumb"><span class="ico" id="i_repo"></span> Forgify <span class="sep">/</span> <span class="muted" id="crumb-ocean"></span></div>
            <span class="grow"></span>
            <span id="head-extra"></span>
            <button class="ibtn" id="i_theme" title="明暗主题"></button>
          </div>
          <div class="sea" id="sea"></div>
        </main>
      </div>
    </div>`;

  $('#i_repo').innerHTML = icon('forge', 15);
  $('#i_theme').innerHTML = icon('moon');
  $('#i_theme').onclick = () => {
    const d = document.documentElement.dataset.theme === 'dark';
    document.documentElement.dataset.theme = d ? 'light' : 'dark';
    $('#i_theme').innerHTML = icon(d ? 'moon' : 'sun');
  };

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
    crumb(text) { $('#crumb-ocean').textContent = text || ''; },
    registerOcean(id, def) { this.oceans[id] = def; },
    mount(id) {
      const o = this.oceans[id];
      if (!o) return console.warn('[Shell] ocean not registered:', id);
      this.sea.innerHTML = '';
      $('#head-extra').innerHTML = '';
      this.body.querySelectorAll('[data-ocean-right]').forEach(el => el.remove());
      if (o.crumb) this.crumb(o.crumb);
      o.build(this.sea);
    },
    returnTo(id) { if (this.toOcean) this.toOcean(id); },
  };
})();
