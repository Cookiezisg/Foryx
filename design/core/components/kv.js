/* Foryx demo — 组件 kv（定义行 k/v；收掉 entities eo-dl-row 的每实体 inspector 副本）。
   契约：组件 = 工厂函数 → el；自载同名 .css；只读令牌；fg- 前缀；不碰别的海洋。
   API：KV.defs(host, rows) → el。rows=[[k, v, opt?]]，opt = { edit, mono, html, mask }。
   语义：左 label 灰、右 value 墨；mask 渲染圆点掩码、mono 走 --mono、edit 让 value contenteditable、html 注入原始片段。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  // 掩码值长度映射成圆点串（不泄露真实长度，固定 8 点，足够暗示密钥）
  const bullets = '•'.repeat(8);

  // 定义行容器：每行左 label 右 value，追加进 host 并返回容器元素
  function defs(host, rows) {
    const box = tag('div.fg-kv');
    (rows || []).forEach(([k, v, opt]) => {
      const o = opt || {};
      const vcls = `fg-kv-v${o.mono ? ' mono' : ''}${o.mask ? ' mask' : ''}`;
      const inner = o.mask ? bullets : (o.html != null ? o.html : esc(v));
      const editAttr = o.edit ? ' contenteditable="true"' : '';
      box.appendChild(tag('div.fg-kv-row',
        `<span class="fg-kv-k">${esc(k)}</span><span class="${vcls}"${editAttr}>${inner}</span>`));
    });
    if (host) host.appendChild(box);
    return box;
  }

  window.KV = { defs };
})();
