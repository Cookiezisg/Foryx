/* Foryx demo — 组件 floating（浮层定位原语；收掉 documents place()/caretRect() + 各 sort-menu 自摆位 + 各处 Escape 监听副本）。
   契约：组件 = 工厂函数 → 句柄；自载同名 .css；只读令牌；fg- 前缀；不碰任何 feature/别组件内部。
   API：
     Floating.open(anchorRect, contentEl, {below, layer}) → {el, close()}
        anchorRect = 选区/光标/按钮的 getBoundingClientRect()；contentEl = 已建好的弹层内容节点。
        贴 anchor 摆位 + 钳进视口（8px 安全边）+ spring 弹入 + 注册 z 层 + 接管单一 Escape/点外即收。
     Floating.onEscape(fn) → off   订阅栈顶 Escape（最上层浮层先吃；返回取消函数）。
   为何做原语：documents 的斜杠窗/选中条/块菜单 + 三个侧栏 sort-menu 各自手摆 getBoundingClientRect()、各自抄 spring/Escape——
     摆位钳制、层叠基线、Escape 协调本是纯机制，抽成一个前门避免漂移（同 status-dot 收态机制的心智）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  // —— z 层注册表：栈式发号，越后开越高（关一层归还基线）；起点高于岛/抽屉，低于窗口拖拽 ——
  const Z_BASE = 1000;
  const stack = [];                                 // [{el, onEsc, onClose}] 栈顶 = 最上层

  // —— 单一 Escape 订阅栈：栈顶浮层先吃 Escape；onEscape 的纯订阅者排其后 ——
  const escSubs = [];                               // 无浮层时的全局 Escape 订阅者（如关侧栏抽屉）

  function topZ() { return Z_BASE + stack.length * 10; }

  // 贴 anchor 摆位并钳进视口：below=true 落在下方(+6)、否则上方(-8)，与 design-lab place() 偏移一致；
  // 溢出则翻面、左右钳到 8px 安全边内（fixed 定位 → 用视口坐标，无需减容器原点）。
  function place(el, rect, below) {
    const vw = window.innerWidth, vh = window.innerHeight, M = 8, GAP_B = 6, GAP_A = 8;
    const w = el.offsetWidth, h = el.offsetHeight;

    // 纵向：先按意图放，溢出则翻到另一面
    let top = below ? rect.bottom + GAP_B : rect.top - h - GAP_A;
    if (below && top + h > vh - M) { const up = rect.top - h - GAP_A; if (up >= M) top = up; }
    else if (!below && top < M) { const dn = rect.bottom + GAP_B; if (dn + h <= vh - M) top = dn; }
    top = Math.min(Math.max(M, top), vh - h - M);

    // 横向：左缘贴 anchor 左缘，钳进 [M, vw-w-M]
    let left = Math.min(Math.max(M, rect.left), vw - w - M);

    el.style.left = left + 'px';
    el.style.top = top + 'px';
  }

  function open(anchorRect, contentEl, opts = {}) {
    const below = opts.below !== false;             // 默认落下方（斜杠窗/AI 询问），传 false 走上方（选中工具条）
    const layer = window.tag('.fg-float', '');
    layer.style.zIndex = topZ();
    layer.appendChild(contentEl);
    document.body.appendChild(layer);

    // 量得真实尺寸后摆位 + spring 弹入（弹入由 .css 的 fgFloatIn 接管，走 --ease-spring）
    place(layer, anchorRect, below);

    const handle = {
      el: layer,
      reposition(nextRect) { place(layer, nextRect || anchorRect, below); },
      close() {
        const i = stack.indexOf(rec);
        if (i < 0) return;                          // 已关，幂等
        stack.splice(i, 1);
        layer.remove();
        opts.onClose && opts.onClose();
      },
    };
    const rec = { el: layer, handle };
    stack.push(rec);

    // 点浮层外即收（捕获阶段，下一帧再挂避免吃掉触发本次 open 的同一次点击）
    setTimeout(() => {
      rec.onDoc = e => { if (!layer.contains(e.target)) handle.close(); };
      document.addEventListener('mousedown', rec.onDoc, true);
    }, 0);
    const origClose = handle.close;
    handle.close = () => { if (rec.onDoc) document.removeEventListener('mousedown', rec.onDoc, true); origClose(); };

    return handle;
  }

  // 单一 Escape 管理：栈顶浮层先关；无浮层则派给 onEscape 订阅者（后订阅先吃 = 栈式）
  function onEscape(fn) {
    escSubs.push(fn);
    return () => { const i = escSubs.indexOf(fn); if (i >= 0) escSubs.splice(i, 1); };
  }
  if (!window.__fgFloatEsc) {
    window.__fgFloatEsc = true;
    document.addEventListener('keydown', e => {
      if (e.key !== 'Escape') return;
      if (stack.length) { e.preventDefault(); stack[stack.length - 1].handle.close(); return; }
      if (escSubs.length) { e.preventDefault(); escSubs[escSubs.length - 1](e); }
    });
  }

  window.Floating = { open, onEscape };
})();
