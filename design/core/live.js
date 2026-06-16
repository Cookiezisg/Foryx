/* Foryx demo — 实时通道（核心，SSE 形）。E1 铁律：全系统仅 messages/entities/notifications 三条流，workspace 级、bus 做 demux。
   契约：Live.on(scope, fn) → {close()}；ephemeral 帧 seq=0(flowrun tick / AI 编辑 delta) 只改瞬时视图态；durable seq>0 才进耐久缓存。
   现为纯前端 mock 发射器（Live.emit 由海洋示意自驱）；接后端时整体换成真 SseGateway，海洋订阅契约不变。 */
(function () {
  const subs = { messages: [], entities: [], notifications: [] };
  window.Live = {
    on(scope, fn) { (subs[scope] = subs[scope] || []).push(fn); return { close() { subs[scope] = (subs[scope] || []).filter(x => x !== fn); } }; },
    emit(scope, frame) { (subs[scope] || []).forEach(fn => { try { fn(frame); } catch (e) { console.warn('[Live]', e); } }); },
  };
})();
