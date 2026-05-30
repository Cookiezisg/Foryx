export const meta = {
  name: 'judge-r4-reflexion',
  description: 'R4 reflexion: judge orig (turn-1 forge) vs revised (after self-critique) on the SAME hard create_workflow scenarios. Does self-critique lift quality?',
  phases: [{ title: 'Judge', detail: '3 judges per variant' }],
}
const VARIANTS = ['orig', 'revised']
const SCHEMA = { type: 'object', required: ['per'], additionalProperties: false, properties: { per: { type: 'array', items: {
  type: 'object', required: ['id', 'correct'], additionalProperties: false,
  properties: { id: { type: 'string' }, correct: { type: 'boolean' }, failed: { type: 'array', items: { type: 'string' } } } } } } }
const judgePrompt = (v) => `Adversarial semantic judge — HARD create_workflow. Read /tmp/r4refl/${v}.json: array of {id, user, intent, rubric, tool_calls[0].args}.
correct=true ONLY if the workflow satisfies EVERY rubric check (right node types & order, case via per-branch when-guards + final default, no dangling, terminal omits to, retry bounded+emit, cron→fetch first, data flows). Default skeptical.
Return per schema per[]{id, correct, failed[]}. Cover every item.`
phase('Judge')
const results = await parallel(VARIANTS.map((v) => () =>
  parallel([0, 1, 2].map((j) => () => agent(judgePrompt(v), { label: `j${j}:${v}`, phase: 'Judge', schema: SCHEMA }).catch(() => null)))
    .then((js) => ({ v, judges: js.filter(Boolean) }))))
function ci(p, n) { return n ? +(1.96 * Math.sqrt(p * (1 - p) / n)).toFixed(3) : 0 }
function agg(r) {
  const idx = new Set(); for (const j of r.judges) for (const p of (j.per || [])) idx.add(p.id)
  const ids = [...idx]; let correct = 0, n = 0
  for (const id of ids) { let yes = 0, tot = 0; for (const j of r.judges) { const p = (j.per || []).find((x) => x.id === id); if (!p) continue; tot++; if (p.correct) yes++ } if (!tot) continue; n++; if (yes >= Math.ceil(tot / 2)) correct++ }
  return { v: r.v, n, correct, rate: n ? +(correct / n).toFixed(2) : 0, ci95: ci(n ? correct / n : 0, n) }
}
const a = results.filter(Boolean).map(agg)
log('R4 reflexion (HARD create_workflow, orig vs revised):')
for (const x of a) log(`  ${x.v}: ${x.correct}/${x.n} = ${(x.rate * 100).toFixed(0)}% ±${Math.round(x.ci95 * 100)}`)
const o = a.find((x) => x.v === 'orig'), rv = a.find((x) => x.v === 'revised')
if (o && rv) log(`\n→ 自审 lift: ${(o.rate * 100).toFixed(0)}% → ${(rv.rate * 100).toFixed(0)}% (${Math.round((rv.rate - o.rate) * 100)}pt)`)
return { variants: a }
