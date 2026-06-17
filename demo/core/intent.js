/* Anselm demo — 统一选中/意图通道（核心，一个前门）。
   契约：
     Intent.select({kind,id,source?})  kind∈{entity,document,workflow,run,node,conversation,notification,settingsCat}
        → 据 manifest.owns 解析归属海洋 → 切到该海洋 → 派发给该海洋的 Intent.on(kind) 处理器。
     Intent.on(kind, fn)               海洋订阅自己拥有的 kind（返回取消函数）。
     Intent.act({verb,kind,id,payload}) 非导航动作（verb 对齐 N5 :run/:call/:invoke/:trigger/:iterate/:triage/:decide）。
     Intent.push/back                  返回栈。
   ref-pill / 侧栏行 / 图节点 / 文档反链 全调 Intent.select —— 零跨海洋 import。
   解耦：海洋切换由 app 控制器经 setNavigator/setCurrent 注入，Intent 不直接依赖 shell。 */
(function () {
  const subs = {};        // kind -> [fn]
  const actSubs = [];
  const stack = [];
  let navigate = null;    // (oceanId) -> Promise|void
  let current = () => null;
  const ownerOf = (kind) => (window.MANIFEST || []).find((f) => (f.owns || []).includes(kind));

  window.Intent = {
    on(kind, fn) { (subs[kind] = subs[kind] || []).push(fn); return () => { subs[kind] = (subs[kind] || []).filter((x) => x !== fn); }; },
    select(sel) {
      const fire = () => (subs[sel.kind] || []).forEach((fn) => { try { fn(sel); } catch (e) { console.warn("[Intent]", e); } });
      const owner = ownerOf(sel.kind);
      if (owner && navigate && current() !== owner.id) {
        const p = navigate(owner.id);
        (p && p.then) ? p.then(fire) : fire();
      } else fire();
    },
    act(a) { actSubs.forEach((fn) => { try { fn(a); } catch (e) { console.warn("[Intent.act]", e); } }); },
    onAct(fn) { actSubs.push(fn); return () => { const i = actSubs.indexOf(fn); if (i >= 0) actSubs.splice(i, 1); }; },
    push(id) { stack.push(id); },
    back() { return stack.pop(); },
    setNavigator(fn) { navigate = fn; },
    setCurrent(fn) { current = fn; },
  };
})();
