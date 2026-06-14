/* Forgify design-lab — Chat 海洋 · 右岛块（实体卡）。
   右岛是「本海洋的」：自己 append 到 Shell.body（作第三个 flex 子），自管显隐与流式编辑。
   依赖：shared/icons.js（icon）+ shared/shell.js（Shell.body）。样式在同目录 chat.css。 */
window.ChatEntityCard = (function () {
  const $ = (s, r) => r.querySelector(s);
  let el = null;

  function ensure() {
    if (el && document.body.contains(el)) return el;
    el = document.createElement('aside');
    el.className = 'aside';
    el.setAttribute('data-ocean-right', 'chat');   // 外壳 mount 时据此清理上个海洋的右岛
    Shell.body.appendChild(el);
    return el;
  }

  function render(a) {
    ensure();
    el.classList.add('show');
    el.innerHTML = `
      <div class="aside-head">
        <span class="etype">${icon('agent', 17)}</span>
        <span class="ht"><b>${a.name}</b><span class="sub">Agent · <span class="ver" data-ver>v${a.version}</span></span></span>
        <span data-live>${a.live ? '<span class="live"><span class="pulse"></span>编辑中</span>' : ''}</span>
        <button class="ibtn" data-close>${icon('close', 16)}</button>
      </div>
      <div class="aside-tabs"><button class="on">Overview</button><button>Versions</button><button>Runs</button><button>Iterate</button></div>
      <div class="aside-body">
        ${a.desc ? `<div class="ec-desc">${a.desc}</div>` : ''}
        <div class="field" data-f="model"><label>Model</label><div class="val mono">${a.model}</div></div>
        <div class="field" data-f="system"><label>System prompt</label><div class="val">${a.system}</div></div>
        <div class="field" data-f="tools"><label>Tools <span class="ct" data-tc>${a.tools.length}</span></label><div class="val" style="padding:8px"><div class="taglist">${a.tools.map(t => `<span class="tag">${t}</span>`).join('')}</div></div></div>
      </div>
      <div class="ec-foot"><span class="mono">ag_7f3c2a91</span> · 12 runs<span class="gap"></span><span class="act">历史版本</span></div>`;
    $('[data-close]', el).onclick = hide;
    el.querySelectorAll('.aside-tabs button').forEach(b => b.onclick = () => {
      el.querySelectorAll('.aside-tabs button').forEach(x => x.classList.remove('on')); b.classList.add('on');
    });
    return el;
  }

  function setLive(on) {
    const s = $('[data-live]', el); if (!s) return;
    s.innerHTML = on ? '<span class="live"><span class="pulse"></span>编辑中</span>'
                     : '<span class="live" style="opacity:.65">已保存</span>';
  }

  const show = () => { ensure().classList.add('show'); };
  const hide = () => { if (el) el.classList.remove('show'); };
  const toggle = () => { ensure().classList.toggle('show'); };

  return { ensure, render, setLive, show, hide, toggle, get el() { return el; }, $ };
})();
