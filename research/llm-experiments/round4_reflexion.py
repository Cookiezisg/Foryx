"""R4 ③ Reflexion / self-critique: after deepseek forges a HARD workflow, inject a structured
self-review turn ("check your output against the request + these criteria; resubmit if wrong").
Records original (turn-1) + revised (turn-2) → judged → does self-critique lift quality? API-only.
Output /tmp/r4refl/{orig,revised}.json
"""
from __future__ import annotations
import json, os, sys
from pathlib import Path
import catalog_v2 as cat
import deepseek_client as ds
from wave1_gen import SYSTEM, parse_args

OUT = Path("/tmp/r4refl"); OUT.mkdir(exist_ok=True)
TOOLS = cat.workflow_tools("V3-full-teaching")

CRITIQUE = ("对照用户原始需求,逐项审查你刚提交的 workflow:\n"
            "① 实体/节点类型对吗?判断/分类/抽取该用 agent 节点,确定性逻辑该用 tool(fn/hd);\n"
            "② case 路由是不是每分支一个 when 布尔守卫、且有最后一条 when:\"true\" 兜底?有没有误用 add_edge 连 case?\n"
            "③ 有没有悬空的 to(指向不存在的节点)?终止节点是否省略了 to?\n"
            "④ cron/manual 触发后,第一个节点是不是先 fetch 数据(不能让后续节点收空 payload)?\n"
            "⑤ 若有重试回边,是否 emit 了自增计数且有界?\n"
            "若发现任何问题,重新提交一个修正后的完整 create_workflow;若确认全部正确,只回复\"确认无误\"不要再调工具。")


def _reasoning(res):
    try:
        return (res.raw_response.get("choices") or [{}])[0].get("message", {}).get("reasoning_content")
    except Exception:
        return None


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
            msgs = [{"role": "system", "content": SYSTEM}, {"role": "user", "content": s["user"]}]
            r1 = ds.chat_complete(messages=msgs, tools=TOOLS, scenario="r4refl", variant="orig",
                                  temperature=None, max_tokens=16000, disable_thinking=False)
            t1 = r1.effective_tool_calls
            orig = parse_args(t1[0]) if t1 else {}
            # self-critique turn
            asst = {"role": "assistant", "content": r1.content or "", "tool_calls": t1}
            rc = _reasoning(r1)
            if rc:
                asst["reasoning_content"] = rc
            msgs2 = msgs + [asst,
                            {"role": "tool", "tool_call_id": (t1[0].get("id") if t1 else "c") or "c", "content": json.dumps({"data": {"id": "wf_pending", "status": "pending"}})},
                            {"role": "user", "content": CRITIQUE}]
            r2 = ds.chat_complete(messages=msgs2, tools=TOOLS, scenario="r4refl_fix", variant="revised",
                                  temperature=None, max_tokens=16000, disable_thinking=False)
            t2 = r2.effective_tool_calls
            revised = parse_args(t2[0]) if t2 else orig  # if it said "确认无误"(no call), keep orig
            base = {"id": s["id"], "user": s["user"], "intent": s.get("intent", ""), "rubric": s.get("rubric", [])}
            mk = lambda a: {**base, "called": ["create_workflow"], "tool_calls": [{"name": "create_workflow", "args": a}], "code": ""}
            return {"orig": mk(orig), "revised": mk(revised), "changed": bool(t2)}
        except ds.BudgetExhausted:
            budget["v"] = True
            return None
        except Exception:
            return None

    res = {"orig": [], "revised": []}; changed = 0
    with cf.ThreadPoolExecutor(max_workers=workers) as ex:
        for fut in cf.as_completed([ex.submit(one, s) for s in scens]):
            r = fut.result()
            if r:
                res["orig"].append(r["orig"]); res["revised"].append(r["revised"]); changed += r["changed"]
    for k in res:
        res[k].sort(key=lambda x: x.get("id", ""))
        (OUT / f"{k}.json").write_text(json.dumps(res[k], ensure_ascii=False, indent=2))
    print(f"n={len(res['orig'])} | 自审后改了的: {changed} | ¥{ds.cumulative_cost_rmb():.2f}")
    print("(语义判官对比 orig vs revised 见 wf_judge_r4refl.js)")


if __name__ == "__main__":
    run()
