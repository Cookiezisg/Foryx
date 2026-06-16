/* Foryx 原语 — CodeEditor。唯一代码块/轻编辑原语;语法色走 --cd-* token。
   API: FyCodeEditor.html(opts)→只读串 · mount(host,opts)→{el,value,setValue,focus}。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  var KW = new Set('const let var function def class return if elif else for while do in of import from export default new await async try except catch finally raise throw with as lambda yield and or not is None True False true false null undefined self this match case pass break continue'.split(' '));
  var TOK = /(#[^\n]*|\/\/[^\n]*|\/\*[\s\S]*?\*\/)|(`(?:\\.|[^`\\])*`|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|(\$\{[^}]*\}|\$\d+|\{\{[^}]*\}\})|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g;

  function cleanClass(s) {
    return String(s || '').split(/\s+/).filter(function (x) { return /^[A-Za-z0-9_-]+$/.test(x); }).join(' ');
  }

  function highlight(code, lang) {
    code = String(code == null ? '' : code);
    var out = '';
    var last = 0;
    var m;
    TOK.lastIndex = 0;
    while ((m = TOK.exec(code))) {
      out += window.esc(code.slice(last, m.index));
      if (m[1]) out += '<span class="fy-code-com">' + window.esc(m[1]) + '</span>';
      else if (m[2]) out += '<span class="fy-code-str">' + window.esc(m[2]) + '</span>';
      else if (m[3]) out += '<span class="fy-code-arg">' + window.esc(m[3]) + '</span>';
      else if (m[4]) out += '<span class="fy-code-num">' + window.esc(m[4]) + '</span>';
      else {
        var w = m[5];
        if (KW.has(w)) out += '<span class="fy-code-kw">' + window.esc(w) + '</span>';
        else if (/^\s*\(/.test(code.slice(m.index + w.length))) out += '<span class="fy-code-fn">' + window.esc(w) + '</span>';
        else out += window.esc(w);
      }
      last = m.index + m[0].length;
    }
    return out + window.esc(code.slice(last));
  }

  function lineNumbers(code) {
    var count = String(code == null ? '' : code).split('\n').length;
    var out = '';
    for (var i = 1; i <= count; i++) out += '<i>' + i + '</i>';
    return out;
  }

  function shell(o, code, editable) {
    o = o || {};
    code = String(code == null ? '' : code);
    var corner = o.corner || o.lang || '';
    var withLines = o.numbers === true;
    var extra = cleanClass(o.className);
    var boxed = o.variant === 'boxed' || o.surface === 'boxed';
    var cls = 'fy-code'
      + (boxed ? ' fy-code-boxed' : ' fy-code-plain')
      + (editable ? ' fy-code-editable' : ' fy-code-readonly')
      + (!withLines ? ' fy-code-no-gut' : '')
      + (o.compact ? ' fy-code-compact' : '')
      + (extra ? ' ' + extra : '');
    var gut = withLines ? '<div class="fy-code-gut">' + lineNumbers(code) + '</div>' : '';
    var input = editable ? '<textarea class="fy-code-input" spellcheck="false">' + window.esc(code) + '</textarea>' : '';
    var lang = corner ? '<span class="fy-code-lang">' + window.esc(corner) + '</span>' : '';
    return '<div class="' + cls + '">' + gut +
      '<div class="fy-code-area"><pre class="fy-code-pre"><code>' + highlight(code, o.lang) + '</code></pre>' + input + '</div>' +
      lang + '</div>';
  }

  function paint(root, code, lang, withLines) {
    root.querySelector('code').innerHTML = highlight(code, lang);
    var gut = root.querySelector('.fy-code-gut');
    if (gut && withLines) gut.innerHTML = lineNumbers(code);
  }

  function html(o) {
    o = o || {};
    return shell(o, o.code, o.editable === true || o.readOnly === false);
  }

  function mount(host, o) {
    o = o || {};
    var editable = o.editable === true || o.readOnly === false;
    var withLines = o.numbers !== false;
    var el = window.el(shell(o, o.code, editable));
    var value = String(o.code == null ? '' : o.code);
    var input = el.querySelector('.fy-code-input');

    function setValue(s) {
      value = String(s == null ? '' : s);
      if (input) input.value = value;
      paint(el, value, o.lang, withLines);
    }

    if (input) {
      input.value = value;
      input.addEventListener('input', function () {
        value = input.value;
        paint(el, value, o.lang, withLines);
        if (o.onDirty) o.onDirty(value);
      });
      input.addEventListener('scroll', function () {
        var pre = el.querySelector('.fy-code-pre');
        pre.scrollTop = input.scrollTop;
        pre.scrollLeft = input.scrollLeft;
      });
      input.addEventListener('keydown', function (e) {
        if (e.key !== 'Tab') return;
        e.preventDefault();
        var s = input.selectionStart;
        var en = input.selectionEnd;
        input.value = input.value.slice(0, s) + '    ' + input.value.slice(en);
        input.selectionStart = input.selectionEnd = s + 4;
        input.dispatchEvent(new Event('input'));
      });
    }

    if (host) host.appendChild(el);
    return {
      el: el,
      value: function () { return input ? input.value : value; },
      setValue: setValue,
      focus: function () { if (input) input.focus(); },
    };
  }

  window.FyCodeEditor = { html: html, mount: mount, highlight: highlight };
})();
