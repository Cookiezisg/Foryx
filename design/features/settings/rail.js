/* Foryx demo — 设置侧栏接管内容（头像轴；薄）。
   外壳 core/sidebar.js 头像点击 → SideBar.mount('settings') 接管 #sidebody（镜像 notifications 铃铛接管），同时 mountSea('settings') 起设置海面。
   本文件经 SideBar.register('settings', render) 挂载：← 返回 + 搜索 + 分组类目导航；点类目 → Intent.select({kind:'settingsCat'}) → 设置海面 Intent.on 切详情。
   类目结构（CATS）本文件持有（它驱动导航）；各类目详情渲染在同目录 sea.js。settingsCat 无 owner → Intent 直接广播给 sea 的 on（零跨文件 import）。
   类名 set- 专属（nav 像素在 rail.css，详情像素在 sea.css）；只读令牌。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  // 分组 → [[id, label]]（id 与 sea.js 的 RENDER 键一一对应）
  const CATS = [
    ['个人化', [['overview', '概览'], ['general', '通用']]],
    ['模型', [['models', '模型与密钥'], ['search', '搜索与嵌入']]],
    ['集成', [['mcp', '连接器'], ['runtimes', '运行时与磁盘']]],
    ['系统', [['workspace', '工作区'], ['notif', '通知'], ['about', '关于']]],
  ];

  function render(host) {
    host.innerHTML = `
      <button class="set-back" id="setBack">${icon('enter', 16)}<span>返回 Foryx</span></button>
      <div class="set-search">${icon('search', 15)}<input placeholder="搜索设置…"></div>
      <div class="set-nav" id="setNav">
        ${CATS.map(([g, items]) => `<div class="set-grp">${g}</div>` +
          items.map(([id, label]) => `<div class="set-cat" data-cat="${id}"><span class="dot"></span>${label}</div>`).join('')).join('')}
      </div>`;

    // ← 返回来源海洋（外壳头像入口已记 Shell._back）：returnTo 重挂该海洋的 rail + sea，自然退出设置接管
    host.querySelector('#setBack').onclick = () => Shell.returnTo(Shell._back || 'chat');

    // 类目点击 → 高亮 + 经 Intent 通知海面切详情（settingsCat 无 owner → 直接广播，不重挂海面）
    const cats = host.querySelectorAll('.set-cat');
    cats.forEach(c => c.onclick = () => {
      cats.forEach(x => x.classList.toggle('on', x === c));
      Intent.select({ kind: 'settingsCat', id: c.dataset.cat });
    });
    // 默认高亮概览（海面 build 默认也渲染 overview，二者同步）
    cats[0] && cats[0].classList.add('on');

    // 标题快滤：藏未命中类目
    const fin = host.querySelector('.set-search input');
    fin.oninput = () => {
      const q = fin.value.trim().toLowerCase();
      cats.forEach(c => { c.style.display = c.textContent.toLowerCase().includes(q) ? '' : 'none'; });
    };
  }

  SideBar.register('settings', render);
})();
