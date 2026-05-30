"""R4 ⑦ :iterate loop — the core product flow. Turn-1: model builds the workflow from `build`.
Turn-2: inject the user `correction`; model applies the fix (edit_workflow or rebuild). Capture both
→ judge: was the correction applied correctly AND the rest kept intact (no clobber)? API-only.
Output /tmp/r4iter/results.json
"""
from __future__ import annotations
import json, os, sys
from pathlib import Path
import catalog_v2 as cat
import deepseek_client as ds
from wave1_gen import SYSTEM, parse_args

OUT = Path("/tmp/r4iter"); OUT.mkdir(exist_ok=True)
TOOLS = cat.workflow_tools("V3-full-teaching")  # create_workflow + edit_workflow


def _reasoning(res):
    try:
        return (res.raw_response.get("choices") or [{}])[0].get("message", {}).get("reasoning_content")
    except Exception:
        return None


def run(workers=12):
    import concurrent.futures as cf
    if not os.environ.get("DEEPSEEK_API_KEY"):
        kf = Path("/tmp/.ds_key")
        if kf.exists():
            os.environ["DEEPSEEK_API_KEY"] = kf.read_text().strip()
    scens = json.loads((OUT / "scenarios.json").read_text())
    budget = {"v": False}

    def one(s):
        if budget["v"]:
            return None
        try:
            msgs = [{"role": "system", "content": SYSTEM}, {"role": "user", "content": s["build"]}]
            r1 = ds.chat_complete(messages=msgs, tools=TOOLS, scenario="r4iter_build", variant="build",
                                  temperature=None, max_tokens=16000, disable_thinking=False)
            t1 = r1.effective_tool_calls
            build_call = [{"name": (t.get("function") or t).get("name"), "args": parse_args(t)} for t in t1]
            wf_id = "wf_a1b2c3d4e5f60718"
            asst = {"role": "assistant", "content": r1.content or "", "tool_calls": t1}
            rc = _reasoning(r1)
            if rc:
                asst["reasoning_content"] = rc
            msgs2 = msgs + [asst]
            for t in t1:
                msgs2.append({"role": "tool", "tool_call_id": t.get("id") or "c", "content": json.dumps({"data": {"id": wf_id, "status": "pending"}})})
            msgs2.append({"role": "user", "content": s["correction"]})
            r2 = ds.chat_complete(messages=msgs2, tools=TOOLS, scenario="r4iter_fix", variant="fix",
                                  temperature=None, max_tokens=16000, disable_thinking=False)
            t2 = r2.effective_tool_calls
            fix_call = [{"name": (t.get("function") or t).get("name"), "args": parse_args(t)} for t in t2]
            return {"id": s["id"], "build": s["build"], "correction": s["correction"], "intent": s.get("intent", ""),
                    "rubric": s.get("rubric", []), "build_call": build_call, "fix_call": fix_call,
                    "fix_tool": (fix_call[0]["name"] if fix_call else "NONE")}
        except ds.BudgetExhausted:
            budget["v"] = True
            return None
        except Exception as e:
            return {"id": s.get("id"), "error": f"{type(e).__name__}: {e}"}

    recs = []
    with cf.ThreadPoolExecutor(max_workers=workers) as ex:
        for fut in cf.as_completed([ex.submit(one, s) for s in scens]):
            r = fut.result()
            if r:
                recs.append(r)
    recs.sort(key=lambda x: x.get("id", ""))
    (OUT / "results.json").write_text(json.dumps(recs, ensure_ascii=False, indent=2))
    from collections import Counter
    print(f"n={len(recs)} | 修正用的工具: {dict(Counter(r.get('fix_tool') for r in recs))} | ¥{ds.cumulative_cost_rmb():.2f}")
    print("(语义判官见 wf_judge_r4iter.js)")


if __name__ == "__main__":
    run()
