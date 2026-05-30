export const meta = {
  name: 'judge-r4-tiering',
  description: 'R4 tiering: judge deepseek-reasoner (R1) on the HARD create_workflow scenarios, same criteria as the flash baseline (52%). Does escalating to a stronger model lift complex builds?',
  phases: [{ title: 'Judge', detail: '3 judges' }],
}
const SCHEMA = { type: 'object', required: ['per'], additionalProperties: false, properties: { per: { type: 'array', items: {
  type: 'object', required: ['id', 'correct'], additionalProperties: false,
  properties: { id: { type: 'string' }, correct: { type: 'boolean' }, failed: { type: 'array', items: { type: 'string' } } } } } } }
const judgePrompt = () => `Adversarial semantic judge — HARD create_workflow (built by deepseek-reasoner R1). Read /tmp/r4tier/reasoner.json: array of {id, user, intent, rubric, tool_calls[0].args}.
correct=true ONLY if the workflow satisfies EVERY rubric check (right node types & order, case via per-branch when-guards + final default, no dangling, terminal omits to, retry bounded+emit, cron→fetch first, data flows). Same strict bar as the flash baseline. Default skeptical. No-call → failed:["no-call"].
Return per schema per[]{id, correct, failed[]}. Cover every item.`
phase('Judge')
const js = (await parallel([0, 1, 2].map((j) => () => agent(judgePrompt(), { label: `j${j}`, phase: 'Judge', schema: SCHEMA }).catch(() => null)))).filter(Boolean)
function ci(p, n) { return n ? +(1.96 * Math.sqrt(p * (1 - p) / n)).toFixed(3) : 0 }
const idx = new Set(); for (const j of js) for (const p of (j.per || [])) idx.add(p.id)
const ids = [...idx]; let correct = 0, n = 0
for (const id of ids) { let yes = 0, tot = 0; for (const j of js) { const p = (j.per || []).find((x) => x.id === id); if (!p) continue; tot++; if (p.correct) yes++ } if (!tot) continue; n++; if (yes >= Math.ceil(tot / 2)) correct++ }
log(`R4 tiering — deepseek-reasoner(R1) on HARD create_workflow: ${correct}/${n} = ${Math.round(100 * correct / n)}% ±${Math.round(ci(correct / n, n) * 100)} (flash baseline 52%)`)
return { model: 'deepseek-reasoner', n, correct, rate: n ? +(correct / n).toFixed(2) : 0 }
