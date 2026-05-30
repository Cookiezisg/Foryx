// tooltuner gen — author N distinct scenarios per tool for a target.
// Invoke via Workflow tool with args: {targetDir, roundDir, tools:[name...], n?, domainHint?}.
// Each agent writes <roundDir>/scenarios/<tool>.json (a JSON array). The operator merges them.
export const meta = {
  name: 'tooltuner-gen',
  description: 'Author ≥N DISTINCT scenarios per tool (maximize diversity) for a tooltuner target round.',
  phases: [{ title: 'Generate', detail: 'one agent per tool, ≥N distinct scenarios each' }],
}
let a = (typeof args !== 'undefined' && args) || {}
if (typeof a === 'string') { try { a = JSON.parse(a) } catch (e) { /* leave as {} below */ } }
const TARGET = a.targetDir
const ROUND = a.roundDir
const TOOLS = a.tools || []
const N = a.n || 50
const HINT = a.domainHint || '电商/客服/运维/金融/内容/IoT/HR/物流/医疗/教育/SaaS/数据/社交/游戏'
if (!TARGET || !ROUND || !TOOLS.length) throw new Error('args need {targetDir, roundDir, tools[]}')

const RESULT = { type: 'object', required: ['tool', 'count'], additionalProperties: false,
  properties: { tool: { type: 'string' }, count: { type: 'integer' }, note: { type: 'string' } } }

const prompt = (tool) => `Author a COVERAGE test-set for the LLM-facing tool \`${tool}\`.
1. Read its exact schema: the \`${tool}\` entry in ${TARGET}/surfaces/tools.json (name/description/parameters). Skim ${TARGET}/surfaces/teaching.md for rules that apply.
2. Author **≥${N} DISTINCT scenarios** — each a realistic user request that SHOULD cause \`${tool}\` to be the correct call. MAXIMIZE diversity: vary domain (${HINT}), entities, phrasing, complexity, surrounding context. NO near-duplicates.
3. Each scenario = {"id":"${tool}_<n>", "user":"<one-paragraph Chinese request>", "intent":"<English: the exactly-correct ${tool} call + args>", "rubric":["3-6 concrete checks: calls ${tool}; correct non-hallucinated args/artifact; right behavior"], "expected_tool":"${tool}"}. For code tools (create_function/create_handler) ALSO add "code_test":{"harness":"<python that imports nothing external, calls the produced function/class with test inputs, asserts, prints OK>"} so the code can be really executed.
4. Write the JSON array to ${ROUND}/scenarios/${tool}.json (run \`mkdir -p ${ROUND}/scenarios\` first) via the Write tool. Verify it parses with python3.
Return {tool, count (≥${N})}. Quality + diversity matter — real coverage, do NOT pad with near-dups.`

phase('Generate')
const results = await parallel(TOOLS.map((tool) => () =>
  agent(prompt(tool), { label: `gen:${tool}`, phase: 'Generate', schema: RESULT, agentType: 'general-purpose' })
    .then((r) => r || { tool, count: 0 }).catch((e) => ({ tool, count: 0, note: String(e).slice(0, 80) }))))

const ok = results.filter(Boolean)
const total = ok.reduce((s, r) => s + (r.count || 0), 0)
const under = ok.filter((r) => (r.count || 0) < N).map((r) => `${r.tool}:${r.count}`)
log(`Generated ${total} scenarios across ${ok.length} tools → ${ROUND}/scenarios/. Under-${N}: ${under.join(', ') || 'none ✓'}`)
return { total, tools: ok.length, under, perTool: ok.map((r) => [r.tool, r.count]) }
