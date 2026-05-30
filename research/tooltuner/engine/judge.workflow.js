// tooltuner judge — adversarial multi-judge, scores every scenario per the target's CURRENT axes.
// Invoke via Workflow tool with args: {targetDir, roundDir, axes:[{key,how_judged}], judgesN?, decorrelate?}.
// Reads <roundDir>/traces/*.json, writes <roundDir>/verdicts.json = [{id, <axisKey>:bool, why}].
export const meta = {
  name: 'tooltuner-judge',
  description: 'Adversarial N-judge scoring of a round\'s traces against the target\'s current axes; majority.',
  phases: [{ title: 'Judge', detail: 'N judges over the round, per-axis booleans, majority' }],
}
let a = (typeof args !== 'undefined' && args) || {}
if (typeof a === 'string') { try { a = JSON.parse(a) } catch (e) { /* leave as {} below */ } }
const TARGET = a.targetDir
const ROUND = a.roundDir
const AXES = a.axes || [{ key: 'selection' }, { key: 'usage' }]
const JN = a.judgesN || 3
if (!TARGET || !ROUND) throw new Error('args need {targetDir, roundDir}')

const axisKeys = AXES.map((x) => x.key)
const PER = {
  type: 'object', required: ['id', ...axisKeys], additionalProperties: true,
  properties: Object.assign({ id: { type: 'string' }, why: { type: 'string' } },
    ...axisKeys.map((k) => ({ [k]: { type: 'boolean' } }))),
}
const SCHEMA = { type: 'object', required: ['per'], additionalProperties: false,
  properties: { per: { type: 'array', items: PER } } }

const axisDoc = AXES.map((x) => `- ${x.key}: ${x.how_judged || (x.key === 'selection' ? 'did the model call the expected tool? (correct search-first then it = true)' : 'given it called, are args/artifact correct per the scenario rubric? default skeptical')}`).join('\n')

const judgePrompt = (n) => `Adversarial judge #${n} for a tooltuner round. Read EVERY trace in ${ROUND}/traces/*.json — each is {id, user, intent, rubric, expected_tool, called:[names], tool_calls:[{name,args}], content, reasoning, exec_result?}.
For EACH scenario, score these axes as booleans:
${axisDoc}
Rules: be DEFAULT-SKEPTICAL on quality axes; a correct search-first then the expected tool still counts as selection=true; if exec_result is present treat it as ground truth for usage. Judge by the rubric + intent, not by surface plausibility. Cover EVERY scenario.
Return per schema: per[]{id, ${axisKeys.join(', ')}, why}.`

phase('Judge')
// JN independent judges over the whole round (decorrelate = vary the prompt framing per judge).
const judges = await parallel(Array.from({ length: JN }, (_v, i) => () =>
  agent(judgePrompt(i + 1) + (a.decorrelate ? `\n(Judge ${i + 1}: weight ${i % 2 ? 'the rubric letter' : 'the user\'s real intent'} more.)` : ''),
    { label: `judge:${i + 1}`, phase: 'Judge', schema: SCHEMA }).then((j) => j?.per || []).catch(() => [])))

// majority per (id, axis)
const ids = [...new Set(judges.flat().map((p) => p.id))]
const verdicts = ids.map((id) => {
  const ps = judges.map((jp) => jp.find((x) => x.id === id)).filter(Boolean)
  const v = { id }
  for (const k of axisKeys) {
    const yes = ps.filter((p) => p[k] === true).length
    v[k] = yes >= Math.ceil(ps.length / 2)
  }
  return v
})

// persist via an agent (script body has no fs).
await agent(`Use the Write tool to write this EXACT content (valid JSON, verbatim, nothing else) to ${ROUND}/verdicts.json:\n${JSON.stringify(verdicts, null, 2)}`,
  { label: 'persist', phase: 'Judge' })
log(`Judged ${verdicts.length} scenarios × ${axisKeys.length} axes, ${JN} judges → ${ROUND}/verdicts.json`)
return { n: verdicts.length, axes: axisKeys, verdicts }
