# STATE — 当前真相(第 0 轮后重生成,勿手改)

- 已转 **0** 轮 · 累计 **¥0** · surfaces `a92a6bf12062`
- 维度集: selection, usage

## 每工具每轴现分(latest)
_尚无分数。跑第一轮后填充。_

## 待办 top(完整见 backlog.json)
- `weak` **create_workflow** — doc14 复杂大图 usage ~52%。先读 raw 验是真弱还是判官过严,再动。 ⚠️verify-first
- `weak` **create_agent** — doc14 复杂 agent 73%,疑被 G1 畸形 JSON 拖累 → 多半是后端 JSON-repair 的事,非描述。验。 ⚠️verify-first
- `reprobe` **call_mcp_tool / skill / memory** — doc14 selection 67-74 偏低;疑 search-first 假阴性(测量假象高发区)。读 raw 复核。 ⚠️verify-first
- `redesign` **tools.json:create_agent.set_output_schema** — G10:value 裸 object → pin {kind,schema}。schema 是 surface,可直接改 tools.json 试 paired-lift。
- `redesign` **tools.json:trigger/edit ops** — G10:cron 字段 / update_code op 形状 pin 死。
- `lever` **examples.md** — few-shot gold 示例已放;测它对复杂建的 paired-lift(预期 ~+11)。
- `new_axis` **efficiency** — 提案新轴:轮数/token 成本。模型有没有绕远(5 轮干 2 轮的活)。先定 how_judged 再立。
- `new_axis` **calibration** — 提案新轴:该不该用工具/何时反问(Subagent/AskUserQuestion 欠选)。
