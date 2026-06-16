/* Foryx demo — 组件 tags（单一事实源；收掉 entities 海洋的 .eo-tags/.eo-tag/add/none 各处副本）。
   契约：工厂挂载 → handle；自载同名 .css；只读令牌/图标；fg- 前缀；不碰别的海洋。
   形态：药丸行 = 可选图标 + 标签 + 内联 × 删；末尾虚线 add 药丸；mode single|multi（single 加入即替换）。
   item 可为字符串或 {label,health}（health=ok 绿点 / bad 红点）；增删走 onChange 回执（替原海洋 markDirty）。
   API：Tags.mount(host,{items,icon,mode,addLabel,onChange}) → {el, items()}。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));

  function mount(host, opts = {}) {
    const { items = [], icon: ic, mode = 'multi', addLabel = 'add', onChange } = opts;
    const box = tag('div.fg-tags');
    const list = [];
    const emit = () => onChange && onChange(list.map(r => r.health ? { label: r.label, health: r.health } : r.label));

    // 单个药丸：图标 + health 点 + 标签 + × 删；× 移除并回执
    function mk(label, health) {
      const rec = { label, health: health || null };
      const ico = ic ? `<span class="fg-tags-ico">${window.icon(ic, 12)}</span>` : '';
      const mh = health ? `<span class="fg-tags-mh ${health === 'bad' ? 'bad' : 'ok'}"></span>` : '';
      const c = tag('span.fg-tags-tag', `${ico}${mh}<span>${esc(label)}</span><span class="fg-tags-x">${window.icon('close', 11)}</span>`);
      c.querySelector('.fg-tags-x').onclick = () => { c.remove(); const i = list.indexOf(rec); if (i > -1) list.splice(i, 1); emit(); };
      rec.el = c;
      return rec;
    }

    function append(label, health) {
      const empty = box.querySelector('.fg-tags-none');
      if (empty) empty.remove();
      const rec = mk(label, health);
      list.push(rec);
      box.insertBefore(rec.el, add);
    }

    if (!items.length) box.appendChild(Object.assign(tag('span.fg-tags-none'), { textContent: '— 无 —' }));
    items.forEach(it => { const rec = typeof it === 'string' ? mk(it) : mk(it.label, it.health); list.push(rec); box.appendChild(rec.el); });

    const add = tag('span.fg-tags-tag.fg-tags-add', `${window.icon('plus', 11)}<span>${esc(addLabel)}</span>`);
    add.onclick = () => {
      const n = (window.prompt && prompt(addLabel)) || '';
      if (!n) return;
      // single：加入即清空既有（同步内存与 DOM）
      if (mode === 'single') { list.splice(0).forEach(r => r.el.remove()); }
      append(n);
      emit();
    };
    box.appendChild(add);
    host.appendChild(box);

    return { el: box, items: () => list.map(r => r.health ? { label: r.label, health: r.health } : r.label) };
  }

  window.Tags = { mount };
})();
