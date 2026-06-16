/* Foryx demo — 组件 approval-gate（单一事实源；收掉 chat .approval-card 与 scheduler/.sa-inbox 两处人在环决策面）。
   契约：组件 = 工厂 → handle；自载同名 .css；只读令牌 + icons；fg- 前缀；不碰任何 feature/别组件内部。
   两味同骨：chat = 内存危险闸（批准/始终批准/拒绝 + danger 自报，无倒计时）；durable = flowrun :decide（通过/驳回 + 倒计时 deadline + 渲染 prompt + 可选 reason）。
   为何同一组件两 flavor 而非两组件：~90% 皮肤共享（盾牌头 + warn 描边 + settled 收口），差异只在头部右侧（badge vs 倒计时）+ 动作动词 + 主体（args/answer vs rendered/reason）。
   API：ApprovalGate.mount(host, {flavor,title,danger,summary,prompt,ddl,allowReason,placeholder}) → {el, settle(text), wait(autoAct, ms) → Promise<{act, reason?}>}
     flavor 'chat'    → approve / approve_always / deny；danger 三级（safe/cautious/dangerous）自报徽；顶角 pulse 点；无倒计时。
     flavor 'durable' → yes(通过) / no(驳回)；warn 倒计时 ddl；渲染 prompt；allowReason → reason textarea；first-wins 脚注。
   决策属人在环动作（非实体导航），故 wait 只 resolve 决议、不触 Intent。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const ICO = window.icon || (() => '');

  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  // 倒计时无共享 clock 图标（icons.js append-only、未登记 clock）→ 内联描边时钟，warn 色随父
  const CLOCK = '<svg class="fg-apg-clock" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 7.5V12l3 1.8"/></svg>';

  // chat 味：危险闸（批准/始终批准/拒绝）。danger 三级自报徽 + 顶角 pulse 点 + args 框 + 预授权说明。
  function chatBody(o) {
    const danger = o.danger || 'dangerous';
    return `
      <span class="fg-apg-pulse"></span>
      <div class="fg-apg-head">
        <span class="fg-apg-shield">${ICO('shield', 16)}</span>
        <span class="fg-apg-tt"><b>${esc(o.title || '需要你批准')}</b><span class="fg-apg-tool">${esc(o.tool || '')}</span></span>
        <span class="fg-apg-badge ${esc(danger)}">${esc(danger)}</span>
      </div>
      <div class="fg-apg-body">
        <div class="fg-apg-sum">${esc(o.summary || '')}</div>
        ${o.args ? `<div class="fg-apg-args">${esc(o.args)}</div>` : ''}
        <div class="fg-apg-actions">
          <button class="fg-apg-btn primary" data-act="approve">${ICO('check', 15)} 批准</button>
          <button class="fg-apg-btn ghost" data-act="approve_always">始终批准</button>
          <span class="fg-apg-note">本会话内预授权</span>
          <button class="fg-apg-btn deny" data-act="deny">拒绝</button>
        </div>
      </div>`;
  }

  // durable 味：flowrun :decide（通过/驳回）。warn 倒计时 + 渲染 prompt + 可选 reason + first-wins 脚注。
  function durableBody(o) {
    return `
      <div class="fg-apg-head">
        <span class="fg-apg-shield">${ICO('shield', 16)}</span>
        <span class="fg-apg-tt"><b>${esc(o.title || '审批收件箱')}</b><span class="fg-apg-sub">flowrun parked · 等人工决策</span></span>
        ${o.ddl ? `<span class="fg-apg-countdown">${CLOCK} ${esc(o.ddl)}</span>` : ''}
      </div>
      <div class="fg-apg-body">
        <div class="fg-apg-rendered">${esc(o.prompt || '')}</div>
        ${o.allowReason ? `<textarea class="fg-apg-reason" rows="2" placeholder="${esc(o.placeholder || '理由（可选）…')}"></textarea>` : ''}
        <div class="fg-apg-actions">
          <button class="fg-apg-btn primary" data-act="yes">${ICO('check', 15)} 通过</button>
          <button class="fg-apg-btn deny" data-act="no">驳回</button>
        </div>
        <div class="fg-apg-foot">first-wins：人工决策与超时同源，谁先到算谁（输家 422）。</div>
      </div>`;
  }

  function mount(host, opts = {}) {
    const h = typeof host === 'string' ? window.qs(host) : host;
    const durable = opts.flavor === 'durable';
    const el = window.tag(`div.fg-apg.${durable ? 'durable' : 'chat'}`);
    el.innerHTML = (durable ? durableBody(opts) : chatBody(opts))
      + `<div class="fg-apg-settled"><span class="fg-apg-ico">${ICO('check', 15)}</span><span data-settled></span></div>`;

    const reasonEl = () => window.qs('.fg-apg-reason', el);
    if (h) h.appendChild(el);

    return {
      el,
      // 收口：替换为「已决」面（隐头/体，亮绿勾 + 文案），并松开 warn 描边
      settle(text) {
        const t = window.qs('[data-settled]', el);
        if (t) t.textContent = text == null ? '' : text;
        el.classList.add('settled');
      },
      // 等用户决议；autoAct/ms 用于自动播放（模拟点选）。durable 带可选 reason。
      wait(autoAct, ms) {
        return new Promise(res => {
          let done = false;
          const fin = act => {
            if (done) return; done = true;
            const out = { act };
            if (durable) { const r = reasonEl(); out.reason = r ? r.value : ''; }
            res(out);
          };
          window.qsa('[data-act]', el).forEach(b => { b.onclick = () => fin(b.dataset.act); });
          if (autoAct) setTimeout(() => fin(autoAct), ms || 1800);
        });
      },
    };
  }

  window.ApprovalGate = { mount };
})();
