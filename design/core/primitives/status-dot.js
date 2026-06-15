/* Foryx 原语 — StatusDot。5 态:idle/run/wait/err/done(SPEC §4.12;run 呼吸)。
   dot(state)→串。状态归一只此一处(`demo` 双实现 → 收归)。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  var OK = { idle: 1, run: 1, wait: 1, err: 1, done: 1 };
  function dot(state) {
    var s = OK[state] ? state : 'idle';
    return '<span class="fy-dot fy-dot-' + s + '"></span>';
  }
  window.FyDot = { dot: dot };
})();
