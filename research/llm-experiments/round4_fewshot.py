"""R4 ④ few-shot gold example: prepend ONE correct complex-workflow example (showing when-guards /
retry-emit / cron→fetch / no-dangling / terminal) to the system prompt, re-run the HARD create_workflow
scenarios → does "看着学" lift the 52%? API-only. Output /tmp/r4fs/fewshot.json
"""
from __future__ import annotations
import json, os, sys
from pathlib import Path
import catalog_v2 as cat
import deepseek_client as ds
from wave1_gen import SYSTEM, parse_args

OUT = Path("/tmp/r4fs"); OUT.mkdir(exist_ok=True)
TOOLS = cat.workflow_tools("V3-full-teaching")

# ONE gold example demonstrating every validated pattern.
GOLD = """

参考范例(一个正确的复杂 workflow,注意每个模式)——
用户:"每小时拉取新订单,金额≥5000的要人工审批,审批通过或小额直接发货,发货失败重试最多3次再转人工。"
正确的 create_workflow.ops:
[
 {"op":"add_node","node":{"id":"t","type":"trigger","config":{"kind":"cron","cron":"0 * * * *"}}},
 {"op":"add_node","node":{"id":"fetch","type":"tool","config":{"ref":"fn_fetch_orders","args":{}}}},     // cron 后第一步先 fetch
 {"op":"add_node","node":{"id":"route","type":"case","config":{"branches":{
     "big":{"when":"payload.amount >= 5000","to":"approve"},
     "small":{"when":"true","to":"ship"}}}}},                                                            // 每分支 when 守卫 + 最后 when:true 兜底
 {"op":"add_node","node":{"id":"approve","type":"approval","config":{"prompt":"订单{{payload.id}}金额{{payload.amount}}需审批","branches":{"approved":{"to":"ship"},"rejected":{"to":"notify_reject"}}}}},
 {"op":"add_node","node":{"id":"ship","type":"tool","config":{"ref":"fn_ship","args":{"orderId":"{{payload.id}}"}}}},
 {"op":"add_node","node":{"id":"shipcheck","type":"case","config":{"branches":{
     "retry":{"when":"!payload.ok && (has(payload.attempt) ? payload.attempt : 0) < 3","to":"ship","emit":{"attempt":"(has(payload.attempt) ? payload.attempt : 0) + 1"}},  // 重试回边:有界 + emit 自增计数
     "done":{"when":"payload.ok","to":"notify_done"},
     "giveup":{"when":"true","to":"escalate_human"}}}}},
 {"op":"add_node","node":{"id":"notify_reject","type":"tool","config":{"ref":"fn_notify","args":{}}}},   // 终止节点省略 to
 {"op":"add_node","node":{"id":"notify_done","type":"tool","config":{"ref":"fn_notify","args":{}}}},
 {"op":"add_node","node":{"id":"escalate_human","type":"tool","config":{"ref":"fn_assign_human","args":{}}}},
 {"op":"add_edge","from":"t","to":"fetch"},{"op":"add_edge","from":"fetch","to":"route"},
 {"op":"add_edge","from":"ship","to":"shipcheck"}                                                        // case 出口走 branches 不走 add_edge
]
照这个模式建,但贴合用户的具体需求。
"""
SYSP = SYSTEM + GOLD


def run(workers=16):
    import concurrent.futures as cf
    if not os.environ.get("DEEPSEEK_API_KEY"):
        kf = Path("/tmp/.ds_key")
        if kf.exists():
            os.environ["DEEPSEEK_API_KEY"] = kf.read_text().strip()
    scens = json.loads(Path("/tmp/r3complex/create_workflow.json").read_text())
    budget = {"v": False}

    def one(s):
        if budget["v"]:
            return None
        try:
            res = ds.chat_complete(messages=[{"role": "system", "content": SYSP}, {"role": "user", "content": s["user"]}],
                                   tools=TOOLS, scenario="r4fs", variant="fewshot",
                                   temperature=None, max_tokens=16000, disable_thinking=False)
            tcs = res.effective_tool_calls
            a = parse_args(tcs[0]) if tcs else {}
            return {"id": s["id"], "user": s["user"], "intent": s.get("intent", ""), "rubric": s.get("rubric", []),
                    "called": ["create_workflow"] if tcs else [], "tool_calls": [{"name": "create_workflow", "args": a}], "code": ""}
        except ds.BudgetExhausted:
            budget["v"] = True
            return None
        except Exception:
            return {"id": s.get("id"), "tool_calls": [{"name": "create_workflow", "args": {}}]}

    recs = []
    with cf.ThreadPoolExecutor(max_workers=workers) as ex:
        for fut in cf.as_completed([ex.submit(one, s) for s in scens]):
            r = fut.result()
            if r:
                recs.append(r)
    recs.sort(key=lambda x: x.get("id", ""))
    (OUT / "fewshot.json").write_text(json.dumps(recs, ensure_ascii=False, indent=2))
    print(f"n={len(recs)} | ¥{ds.cumulative_cost_rmb():.2f}(语义判官见 wf_judge_r4fs.js,对比 baseline 52%)")


if __name__ == "__main__":
    run()
