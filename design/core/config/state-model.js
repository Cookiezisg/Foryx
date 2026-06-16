/* Foryx demo — 状态模型（单一事实源，收掉 en-st/eo-st/wf-st/cv-st/rp + ENV/CFG/CONN 各处副本）。
   DOT = 通用 5 态点；scheduler 的 flowrun 词汇经 ALIAS 折回；ENV/CFG/CONN = 类型化徽文案。
   字段：color(语义色名) · label · pulse(脉冲) · hollow(空心环)。component status-dot 据此渲染。 */
window.STATE_MODEL = {
  // 通用实体/会话 5 态（idle/run/wait/err/done）+ 监听
  DOT: {
    idle:      { color: 'ink-3',  label: '闲置',   hollow: true },
    run:       { color: 'accent', label: '运行中', pulse: true },
    wait:      { color: 'warn',   label: '需处理', pulse: true },
    err:       { color: 'danger', label: '失败' },
    done:      { color: 'ok',     label: '就绪' },
    listening: { color: 'accent', label: '监听中', pulse: true },
  },
  // flowrun 词汇 → DOT（run/node 状态枚举对齐后端）
  ALIAS: { running: 'run', completed: 'done', failed: 'err', parked: 'wait', cancelled: 'idle', future: 'idle', waiting: 'wait', ok: 'done' },
  // 类型化徽（env 物化 / handler config / mcp 连接）
  ENV:  { pending: ['idle', '排队'], syncing: ['run', '物化中…'], ready: ['done', '就绪'], failed: ['err', '失败'] },
  CFG:  { unconfigured: ['idle', '未配置'], partially_configured: ['run', '部分配置'], ready: ['done', '就绪'] },
  CONN: { ready: ['done', 'connected'], failed: ['err', 'auth required'], pending: ['idle', '未连接'] },
};
// 解析任意状态字符串 → 规范 DOT key
window.stState = s => window.STATE_MODEL.DOT[s] ? s : (window.STATE_MODEL.ALIAS[s] || 'idle');
