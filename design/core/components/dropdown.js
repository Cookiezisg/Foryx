/* Foryx demo — 组件 dropdown（单一事实源；收掉 onboarding buildDD + settings .st-dd 各处自定义下拉副本）。
   契约：组件 = 工厂函数 → handle；自载同名 .css；只读令牌/图标；fg- 前缀；不碰别的海洋。
   API：Dropdown.mount(host,{options,value,onChange}) → {el,value()}。
   option = {value,label,meta?}；按钮显当前 label(+meta)，弹层为富行(label/meta/勾)，spring 弹入、点外即收。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  // 全局：点空白处收掉所有展开的下拉（仅注册一次）
  if (!window.__fgDDClose) {
    window.__fgDDClose = true;
    document.addEventListener('click', () => closeAll());
  }
  function closeAll(except) {
    qsa('.fg-dd.open').forEach(d => { if (d !== except) d.classList.remove('open'); });
  }

  function mount(host, cfg = {}) {
    const opts = cfg.options || [];
    let cur = cfg.value != null ? cfg.value : (opts.length ? opts[0].value : null);

    const rows = opts.map(o =>
      `<div class="fg-dd-opt" data-v="${o.value}">` +
        `<span class="fg-dd-lab">${o.label}</span>` +
        `<span class="fg-dd-meta">${o.meta || ''}</span>` +
        `<span class="fg-dd-ck">${window.icon('check', 14, 2.2)}</span>` +
      `</div>`).join('');

    const el = tag('.fg-dd',
      `<button class="fg-dd-btn" type="button">` +
        `<span class="fg-dd-lab"></span>` +
        `<span class="fg-dd-meta"></span>` +
        `<span class="fg-dd-caret">${window.icon('chevd', 15)}</span>` +
      `</button>` +
      `<div class="fg-dd-pop">${rows}</div>`);

    const btn = qs('.fg-dd-btn', el);
    const paint = () => {
      const o = opts.find(x => x.value === cur) || opts[0] || {};
      qs('.fg-dd-lab', btn).textContent = o.label || '';
      qs('.fg-dd-meta', btn).textContent = o.meta || '';
      qsa('.fg-dd-opt', el).forEach(d => d.classList.toggle('sel', d.dataset.v === cur));
    };

    // 按钮：先关同级再切自身（stopPropagation 避免被文档级收掉）
    btn.onclick = e => {
      e.stopPropagation();
      const open = el.classList.contains('open');
      closeAll(el);
      el.classList.toggle('open', !open);
    };
    qsa('.fg-dd-opt', el).forEach(d => d.onclick = e => {
      e.stopPropagation();
      cur = d.dataset.v;
      el.classList.remove('open');
      paint();
      cfg.onChange && cfg.onChange(cur);
    });

    paint();
    if (host) host.appendChild(el);
    return { el, value: () => cur };
  }

  window.Dropdown = { mount };
})();
