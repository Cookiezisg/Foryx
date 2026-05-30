export const meta = {
  name: 'judge-r4-gepa',
  description: 'R4 GEPA round-1: judge base teaching vs GEPA-V1 (semantic architecture rules) on the HELD-OUT hard create_workflow set. Does evolving the teaching toward architecture decisions lift quality?',
  phases: [{ title: 'Judge', detail: '3 judges per variant' }],
}
const VARIANTS = [['heldout_base', 'base 教学'], ['heldout_gepa_v1', 'GEPA-V1 架构守则']]
const SCHEMA = { type: 'object', required: ['per'], additionalProperties: false, properties: { per: { type: 'array', items: {
  type: 'object', required: ['id', 'correct'], additionalProperties: false,
  properties: { id: { type: 'string' }, correct: { type: 'boolean' }, failed: { type: 'array', items: { type: 'string' } } } } } } }
const judgePrompt = (f) => `Adversarial semantic judge — HELD-OUT hard create_workflow. Read /tmp/r4gepa/${f}.json: array of {id, user, intent, rubric, tool_calls[0].args}.
correct=true ONLY if the workflow satisfies EVERY rubric check — right node types (agent for judgement/classify, tool for deterministic), correct case-when routing + default, polling-vs-cron correct, no dangling, terminal omits to, retry bounded+emit, data flows. Default skeptical.
Return per schema per[]{id, correct, failed[]}. Cover every item.`
phase('Judge')
const results = await parallel(VARIANTS.map(([f, label]) => () =>
  parallel([0, 1, 2].map((j) => () => agent(judgePrompt(f), { label: `j${j}:${label}`, phase: 'Judge', schema: SCHEMA }).catch(() => null)))
    .then((js) => ({ f, label, judges: js.filter(Boolean) }))))
function ci(p, n) { return n ? +(1.96 * Math.sqrt(p * (1 - p) / n)).toFixed(3) : 0 }
function agg(r) {
  const idx = new Set(); for (const j of r.judges) for (const p of (j.per || [])) idx.add(p.id)
  const ids = [...idx]; let correct = 0, n = 0
  for (const id of ids) { let yes = 0, tot = 0; for (const j of r.judges) { const p = (j.per || []).find((x) => x.id === id); if (!p) continue; tot++; if (p.correct) yes++ } if (!tot) continue; n++; if (yes >= Math.ceil(tot / 2)) correct++ }
  return { label: r.label, n, correct, rate: n ? +(correct / n).toFixed(2) : 0, ci95: ci(n ? correct / n : 0, n) }
}
const a = results.filter(Boolean).map(agg)
log('R4 GEPA round-1 (held-out, base vs GEPA-V1 架构守则):')
for (const x of a) log(`  ${x.label}: ${x.correct}/${x.n} = ${Math.round(x.rate * 100)}% ±${Math.round(x.ci95 * 100)}`)
const b = a.find((x) => x.label.startsWith('base')), v = a.find((x) => x.label.startsWith('GEPA'))
if (b && v) log(`\n→ GEPA-V1 paired lift: ${Math.round(b.rate * 100)}% → ${Math.round(v.rate * 100)}% (${Math.round((v.rate - b.rate) * 100)}pt)`)
return { variants: a }
