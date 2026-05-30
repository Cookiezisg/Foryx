"""R4 ⑥ model tiering: run the HARD create_workflow scenarios with the STRONGER deepseek-reasoner
(R1) instead of flash → does escalating to a stronger model for complex builds lift the 52%? Records
cost so we can draw the quality/cost frontier. API-only. Output /tmp/r4tier/reasoner.json
"""
from __future__ import annotations
import json, os, sys
from pathlib import Path
import catalog_v2 as cat
import deepseek_client as ds
from wave1_gen import SYSTEM, parse_args

OUT = Path("/tmp/r4tier"); OUT.mkdir(exist_ok=True)
TOOLS = cat.workflow_tools("V3-full-teaching")
STRONG = "deepseek-reasoner"


def run(workers=8, limit=None):
    import concurrent.futures as cf
    if not os.environ.get("DEEPSEEK_API_KEY"):
        kf = Path("/tmp/.ds_key")
        if kf.exists():
            os.environ["DEEPSEEK_API_KEY"] = kf.read_text().strip()
    scens = json.loads(Path("/tmp/r3complex/create_workflow.json").read_text())
    if limit:
        scens = scens[:limit]
    budget = {"v": False}
    cost0 = ds.cumulative_cost_rmb()

    def one(s):
        if budget["v"]:
            return None
        try:
            # reasoner = R1 reasoning model; don't disable thinking; multi-turn would need reasoning echo.
            res = ds.chat_complete(messages=[{"role": "system", "content": SYSTEM}, {"role": "user", "content": s["user"]}],
                                   tools=TOOLS, scenario="r4tier", variant="reasoner", model=STRONG,
                                   temperature=None, max_tokens=16000, timeout=180.0)
            tcs = res.effective_tool_calls
            a = parse_args(tcs[0]) if tcs else {}
            return {"id": s["id"], "user": s["user"], "intent": s.get("intent", ""), "rubric": s.get("rubric", []),
                    "called": ["create_workflow"] if tcs else [], "tool_calls": [{"name": "create_workflow", "args": a}], "code": ""}
        except ds.BudgetExhausted:
            budget["v"] = True
            return None
        except Exception as e:
            return {"id": s.get("id"), "error": f"{type(e).__name__}: {e}", "tool_calls": [{"name": "create_workflow", "args": {}}]}

    recs = []
    with cf.ThreadPoolExecutor(max_workers=workers) as ex:
        for fut in cf.as_completed([ex.submit(one, s) for s in scens]):
            r = fut.result()
            if r:
                recs.append(r)
    recs.sort(key=lambda x: x.get("id", ""))
    (OUT / "reasoner.json").write_text(json.dumps(recs, ensure_ascii=False, indent=2))
    spent = ds.cumulative_cost_rmb() - cost0
    print(f"n={len(recs)} | reasoner 花费 ¥{spent:.2f}(对比 flash 同样 60 场景约 ¥0.5)| 累计 ¥{ds.cumulative_cost_rmb():.2f}")
    print("(语义判官见 wf_judge_r4tier.js;对比 flash baseline 52%)")


if __name__ == "__main__":
    run(limit=int(sys.argv[1]) if len(sys.argv) > 1 else None)
