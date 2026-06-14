/* Forgify design-lab — 外壳框架（内核，只读消费；勿在海洋/侧栏里改本文件）。
   只做三件事：搭圆角浮窗、开三个槽位、提供海洋挂载 API。
     槽位： #left（侧栏模块填）· #sea（海洋中区填）· .body（海洋把自己的右岛 append 进来）
     主区头： 面包屑 + #head-extra（海洋自己加按钮）+ 主题切换
   海洋只需：Shell.registerOcean(id, { crumb, build(sea) }) 然后 Shell.mount(id)。
   ⚠ 右岛是「海洋的」：由海洋自己渲染进 Shell.body 并自管显隐，外壳不掺和。 */
(function () {
  const $ = (s, r = document) => r.querySelector(s);

  document.body.innerHTML = `
    <div class="win">
      <div class="body" id="body">
        <aside class="side" id="left"></aside>            <!-- 侧栏模块填 -->
        <main class="main">
          <div class="main-head">
            <span id="head-lead" style="display:flex;align-items:center;gap:4px"></span>  <!-- 中性领位槽：侧栏放再展开按钮 -->
            <div class="crumb"><span class="ico" id="i_repo"></span> Forgify <span class="sep">/</span>
              <span class="muted" id="crumb-ocean">前端设计 (fork)</span> <span class="chev" id="i_cd"></span></div>
            <span class="grow"></span>
            <span id="head-extra" style="display:flex;gap:4px"></span>   <!-- 海洋加按钮 -->
            <button class="ibtn" id="i_theme"></button>
          </div>
          <div class="sea" id="sea"></div>                  <!-- 海洋中区填 -->
        </main>
        <!-- 海洋的右岛由海洋自己 append 到 #body -->
      </div>
    </div>`;

  $('#i_repo').innerHTML = icon('repo', 15);
  $('#i_cd').innerHTML = icon('chevd', 14);
  $('#i_theme').innerHTML = icon('moon');
  $('#i_theme').onclick = () => {
    const d = document.documentElement.dataset.theme === 'dark';
    document.documentElement.dataset.theme = d ? 'light' : 'dark';
    $('#i_theme').innerHTML = icon(d ? 'moon' : 'sun');
  };

  window.Shell = {
    $,
    oceans: {},
    get left() { return $('#left'); },         // 侧栏槽
    get sea() { return $('#sea'); },            // 海洋中区槽
    get body() { return $('#body'); },          // 海洋把右岛 append 到这
    get headLead() { return $('#head-lead'); }, // 主区头最左中性布局槽（侧栏放再展开按钮；海洋勿碰）
    get sideWidth() { return parseFloat(getComputedStyle($('#left')).width) || 0; }, // 只读·optional·海洋勿用
    headExtra(html) { const s = $('#head-extra'); s.innerHTML = html; return s; },
    crumb(text) { $('#crumb-ocean').textContent = text; },
    registerOcean(id, def) { this.oceans[id] = def; },
    mount(id) {
      const o = this.oceans[id];
      if (!o) return console.warn('[Shell] ocean not registered:', id);
      this.sea.innerHTML = '';
      $('#head-extra').innerHTML = '';
      this.body.querySelectorAll('[data-ocean-right]').forEach(el => el.remove());   // 清掉上个海洋的右岛
      if (o.crumb) this.crumb(o.crumb);
      o.build(this.sea);
    },
  };
})();
