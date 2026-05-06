// toast.js — global Alpine store for transient notifications. Replaces the
// `alert(...)` modal dialogs scattered across send / save / delete paths
// (which interrupt focus + obscure the underlying state). Toasts slide in
// at the top-right corner, auto-dismiss after 4s, stack vertically.
//
// Usage from anywhere:
//   toast.success('Saved')
//   toast.error('Save failed: ' + e.message)
//   toast.info('Merging branches…', { sticky: true })   // no auto-dismiss
//
// toast.js — 瞬时通知的 Alpine 全局 store。替换散落在 send/save/delete 路径
// 里的 alert()（打断焦点 + 遮挡背景状态）。toast 从右上角滑入，4s 自动消失，
// 多条垂直堆叠。

document.addEventListener('alpine:init', () => {
  Alpine.store('toasts', {
    list: [],
    _idCounter: 0,

    push(type, text, opts) {
      opts = opts || {};
      const id = ++this._idCounter;
      this.list.push({ id, type, text });
      if (!opts.sticky) {
        setTimeout(() => this.dismiss(id), opts.ms || 4000);
      }
      return id;
    },

    dismiss(id) {
      this.list = this.list.filter(t => t.id !== id);
    },

    clear() {
      this.list = [];
    },
  });
});

// Window-level shorthand. Usable from anywhere — including outside Alpine
// component contexts (callbacks, fetch handlers, etc.). All four call into
// the same store; if Alpine isn't ready yet (very early page load), no-op
// silently rather than crashing.
//
// 全局简短调用。在 Alpine 组件外（callback / fetch handler）也能用。Alpine
// 还没好时静默 no-op，不崩。
window.toast = {
  success(text, opts) { _push('success', text, opts); },
  error(text, opts)   { _push('error', text, opts); },
  info(text, opts)    { _push('info', text, opts); },
  warn(text, opts)    { _push('warn', text, opts); },
};

function _push(type, text, opts) {
  if (window.Alpine && window.Alpine.store && window.Alpine.store('toasts')) {
    window.Alpine.store('toasts').push(type, text, opts);
  } else {
    // Alpine not ready — fall back to console so dev still sees the message.
    // Alpine 还没初始化——回退 console 让 dev 仍能看到消息。
    const c = type === 'error' ? 'error' : type === 'warn' ? 'warn' : 'log';
    // eslint-disable-next-line no-console
    console[c]('[toast]', text);
  }
}
