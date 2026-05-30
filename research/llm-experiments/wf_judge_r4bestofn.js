export const meta = {
  name: 'judge-r4-bestofn',
  description: 'R4 best-of-N + self-consistency: judge n1 (baseline) vs bestN (verifier-pick) vs selfcon (modal-signature) on the SAME hard create_workflow scenarios. Paired semantic comparison.',
  phases: [{ title: 'Judge', detail: '3 judges per variant' }],
}
const VARIANTS = ['n1', 'bestN', 'selfcon']
const SCHEMA = { type: 'object', required: ['per'], additionalProperties: false, properties: { per: { type: 'array', items: {
  type: 'object', required: ['id', 'correct'], additionalProperties: false,
  properties: { id: { type: 'string' }, correct: { type: 'boolean' }, failed: { type: 'array', items: { type: 'string' } } } } } } }

const judgePrompt = (v) => `Adversarial semantic judge — HARD create_workflow scenarios. Read /tmp/r4bon/${v}.json: array of {id, user, intent, rubric, tool_calls[0].args (the workflow ops)}.
For EACH item correct=true ONLY if the workflow genuinely satisfies EVERY rubric check: right node types & order, case routing via per-branch when-guards (no key-match, final when:"true"/_default), retry bounded+emit if present, no dangling refs, terminal omits to, first node after cron fetches data, data flows. Default skeptical. No-tool-call → failed:["no-call"].
Return per schema: per[]{id, correct, failed[]}. Cover EVERY item.`

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
log('R4 best-of-N + self-consistency (HARD create_workflow, 语义判官):')
for (const x of a) log(`  ${x.v}: ${x.correct}/${x.n} = ${(x.rate * 100).toFixed(0)}% ±${Math.round(x.ci95 * 100)}`)
const n1 = a.find((x) => x.v === 'n1'), bn = a.find((x) => x.v === 'bestN')
if (n1 && bn) log(`\n→ best-of-${5} vs N=1: ${(n1.rate * 100).toFixed(0)}% → ${(bn.rate * 100).toFixed(0)}% (lift ${Math.round((bn.rate - n1.rate) * 100)}pt)`)
return { variants: a }
