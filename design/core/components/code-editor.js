/* Foryx demo — 组件 code-editor（单一事实源；收掉 entities codeEditor()/hl()/TOK/KW + documents highlight() 各处副本）。
   契约：工厂挂载 → handle；自载同名 .css；只读令牌（Atom One --cd-*）；fg- 前缀；不碰别的海洋。
   形态：无边编辑板 = 透明 textarea 叠在高亮 pre 上 + 右上角语言标签；可改（活体重绘 + onDirty 回执）或只读。
   这里是【唯一】语法高亮事实源：TOK 正则 + KW 集 + hl()——entities 超集（含 $n/${}/CEL arg），documents 子集靠它达到 parity。
   API：CodeEditor.mount(host,{code,lang,corner,readOnly,onDirty}) → {el, value(), setValue(s), focus()}
        · CodeEditor.highlight(code, lang) → html（非可编辑渲染用，如 version-diff / documents 的 pre 着色）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));

  // ===== 语法高亮（Atom One；唯一 tokenizer）=====
  // 关键字全集（JS / Python / 通用），命中染 kw 色。
  const KW = new Set('const let var function def class return if elif else for while do in of import from export default new await async try except catch finally raise throw with as lambda yield and or not is None True False true false null undefined self this match case pass break continue'.split(' '));
  // 五组捕获（顺序即优先级）：① 注释 ② 字符串/模板 ③ 插值参数 $n/${}/{{}} ④ 数字 ⑤ 标识符（→ kw / fn / 裸字）。
  const TOK = /(#[^\n]*|\/\/[^\n]*|\/\*[\s\S]*?\*\/)|(`(?:\\.|[^`\\])*`|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|(\$\{[^}]*\}|\$\d+|\{\{[^}]*\}\})|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g;

  // lang 形参为前向兼容（统一 tokenizer 覆盖 py/js/markdown/cel，暂不按语言分流）。
  function highlight(code, lang) {
    let out = '', last = 0, m;
    TOK.lastIndex = 0;
    while ((m = TOK.exec(code))) {
      out += esc(code.slice(last, m.index));
      if (m[1]) out += `<span class="fg-ce-com">${esc(m[1])}</span>`;
      else if (m[2]) out += `<span class="fg-ce-str">${esc(m[2])}</span>`;
      else if (m[3]) out += `<span class="fg-ce-arg">${esc(m[3])}</span>`;
      else if (m[4]) out += `<span class="fg-ce-num">${esc(m[4])}</span>`;
      else {
        const w = m[5];
        // 裸标识符后紧跟「(」→ 当函数名（fn 色），否则保持普通墨色。
        out += KW.has(w) ? `<span class="fg-ce-kw">${w}</span>`
          : (/^\s*\(/.test(code.slice(m.index + w.length)) ? `<span class="fg-ce-fn">${esc(w)}</span>` : esc(w));
      }
      last = m.index + m[0].length;
    }
    return out + esc(code.slice(last));
  }

  function cornerText(value) {
    const s = String(value || '').trim();
    if (!s) return '';
    if (/^python\b/i.test(s)) return s.replace(/^python\b/i, 'Python');
    if (/^javascript\b/i.test(s)) return s.replace(/^javascript\b/i, 'JavaScript');
    if (/^typescript\b/i.test(s)) return s.replace(/^typescript\b/i, 'TypeScript');
    if (/^json\b/i.test(s)) return s.replace(/^json\b/i, 'JSON');
    return s;
  }

  function mount(host, opts = {}) {
    const { code = '', lang, corner = '', readOnly = false, onDirty } = opts;
    const cornerLabel = cornerText(corner || lang || '');
    const w = tag('div.fg-ce');
    w.innerHTML = `<div class="fg-ce-area"><pre><code></code></pre><textarea spellcheck="false" ${readOnly ? 'readonly' : ''}></textarea></div>${cornerLabel ? `<span class="fg-ce-lang">${esc(cornerLabel)}</span>` : ''}`;

    const ta = w.querySelector('textarea'), codeEl = w.querySelector('code'), pre = w.querySelector('pre');

    // 重绘：高亮叠层由 textarea 驱动，结构不再生成行号槽。
    const paint = () => {
      codeEl.innerHTML = highlight(ta.value, lang);
    };
    ta.value = code;
    paint();

    ta.addEventListener('input', () => { paint(); onDirty && onDirty(); });
    // 高亮 pre 是「下层投影」，透明 textarea 滚动时同步其滚动位（横竖）。
    ta.addEventListener('scroll', () => { pre.scrollTop = ta.scrollTop; pre.scrollLeft = ta.scrollLeft; });
    // Tab 插四空格（代码块语义），而非把焦点移出。
    ta.addEventListener('keydown', e => {
      if (e.key === 'Tab') {
        e.preventDefault();
        const s = ta.selectionStart, en = ta.selectionEnd;
        ta.value = ta.value.slice(0, s) + '    ' + ta.value.slice(en);
        ta.selectionStart = ta.selectionEnd = s + 4;
        paint();
        onDirty && onDirty();
      }
    });

    if (host) host.appendChild(w);

    return {
      el: w,
      value: () => ta.value,
      setValue: s => { ta.value = s == null ? '' : String(s); paint(); },
      focus: () => ta.focus(),
    };
  }

  window.CodeEditor = { mount, highlight };
})();
