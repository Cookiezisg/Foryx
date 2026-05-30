export const meta = {
  name: 'judge-r4-iterate',
  description: 'R4 :iterate loop — did the model apply the user correction correctly AND keep the rest of the workflow intact (no clobber)? The core product-flow test.',
  phases: [{ title: 'Judge', detail: '3 judges over 24 iterate episodes' }],
}
const SCHEMA = { type: 'object', required: ['per'], additionalProperties: false, properties: { per: { type: 'array', items: {
  type: 'object', required: ['id', 'correct'], additionalProperties: false,
  properties: { id: { type: 'string' }, correct: { type: 'boolean' }, applied: { type: 'boolean' }, intact: { type: 'boolean' }, failed: { type: 'array', items: { type: 'string' } } } } } } }
const judgePrompt = () => `Adversarial judge for the :iterate product flow. Read /tmp/r4iter/results.json: array of {id, build (initial request), correction (user's edit after seeing v1), intent (correct final state), rubric, build_call (turn-1 ops), fix_call (turn-2 ops — edit_workflow or a rebuilt create_workflow), fix_tool}.
For EACH episode judge whether the model's fix_call correctly realizes the correction:
- applied = the requested change is actually made (e.g. function→agent swap done, node inserted, threshold changed, retry loop added, trigger changed).
- intact = the REST of the workflow is preserved — unrelated nodes/branches/refs NOT dropped or clobbered. (If it used edit_workflow with targeted ops, check the ops do the change without removing unrelated parts. If it REBUILT via create_workflow, check the rebuild kept all the original pieces + the change.)
- correct = applied AND intact, AND the result still valid (when-guards, no dangling, default branch).
Default skeptical. No fix tool call → applied=false. Return per schema per[]{id, correct, applied, intact, failed[]}. Cover EVERY episode.`
phase('Judge')
const judges = await parallel([0, 1, 2].map((j) => () => agent(judgePrompt(), { label: `j${j}`, phase: 'Judge', schema: SCHEMA }).catch(() => null)))
const js = judges.filter(Boolean)
function ci(p, n) { return n ? +(1.96 * Math.sqrt(p * (1 - p) / n)).toFixed(3) : 0 }
const idx = new Set(); for (const j of js) for (const p of (j.per || [])) idx.add(p.id)
const ids = [...idx]; let correct = 0, applied = 0, intact = 0, n = 0; const fails = {}
for (const id of ids) {
  let c = 0, a = 0, it = 0, tot = 0
  for (const j of js) { const p = (j.per || []).find((x) => x.id === id); if (!p) continue; tot++; if (p.correct) c++; if (p.applied) a++; if (p.intact) it++; if (!p.correct) for (const f of (p.failed || [])) fails[f] = (fails[f] || 0) + 1 }
  if (!tot) continue; n++; if (c >= Math.ceil(tot / 2)) correct++; if (a >= Math.ceil(tot / 2)) applied++; if (it >= Math.ceil(tot / 2)) intact++
}
log(`R4 :iterate 回路 (n=${n}):`)
log(`  修正正确(应用对+没破坏): ${correct}/${n} = ${Math.round(100 * correct / n)}% ±${Math.round(ci(correct / n, n) * 100)}`)
log(`  其中 应用了修改: ${applied}/${n}=${Math.round(100 * applied / n)}% | 保住其余没clobber: ${intact}/${n}=${Math.round(100 * intact / n)}%`)
log(`  top 失败: ${Object.entries(fails).sort((a, b) => b[1] - a[1]).slice(0, 5).map(([f, c]) => f.slice(0, 30) + '×' + c).join(' | ')}`)
return { n, correct, applied, intact, rate: n ? +(correct / n).toFixed(2) : 0 }
