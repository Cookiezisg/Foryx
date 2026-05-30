"""R4 ① best-of-N + verify  AND  ⑤ self-consistency (share the same N samples).
For each HARD create_workflow scenario, sample N candidates (temp=default → diverse). Then pick 3 ways:
  - n1        : first candidate (= the N=1 baseline)
  - bestN     : the candidate the structural verifier (wf_verify) scores highest (best-of-N)
  - selfcon   : the candidate matching the MODAL structural signature (self-consistency vote)
Writes 3 result sets (same shape as r3cxres) → judged by wf_judge_r4bestofn.js → paired comparison.
API-only. temp=default. Output /tmp/r4bon/{n1,bestN,selfcon}.json
"""
from __future__ import annotations
import json, os, sys
from pathlib import Path
from collections import Counter
import catalog_v2 as cat
import deepseek_client as ds
from wave1_gen import SYSTEM, parse_args
from wf_verify import verify_workflow

OUT = Path("/tmp/r4bon"); OUT.mkdir(exist_ok=True)
TOOLS = cat.workflow_tools("V3-full-teaching")
N = 5


def sig(args):
    """Coarse structural signature for self-consistency voting."""
    ops = args.get("ops", []) if isinstance(args, dict) else []
    nts = tuple(sorted(o.get("node", {}).get("type", "") for o in ops if isinstance(o, dict) and o.get("op") == "add_node"))
    ncase = sum(1 for o in ops if isinstance(o, dict) and o.get("node", {}).get("type") == "case")
    return (len(ops), nts, ncase)


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
        cands = []
        for _ in range(N):
            try:
                res = ds.chat_complete(messages=[{"role": "system", "content": SYSTEM}, {"role": "user", "content": s["user"]}],
                                       tools=TOOLS, scenario="r4bon", variant="bestofn",
                                       temperature=None, max_tokens=16000, disable_thinking=False)
                tcs = res.effective_tool_calls
                a = parse_args(tcs[0]) if tcs else {}
                p, score, fails = verify_workflow(a)
                cands.append({"args": a, "score": score, "passed": p})
            except ds.BudgetExhausted:
                budget["v"] = True
                break
            except Exception:
                cands.append({"args": {}, "score": 0.0, "passed": False})
        if not cands:
            return None
        n1 = cands[0]
        bestN = max(cands, key=lambda c: c["score"])
        sigs = Counter(str(sig(c["args"])) for c in cands)
        modal = sigs.most_common(1)[0][0]
        selfcon = next((c for c in cands if str(sig(c["args"])) == modal), cands[0])
        base = {"id": s["id"], "user": s["user"], "intent": s.get("intent", ""), "rubric": s.get("rubric", []), "expected_tool": "create_workflow"}
        mk = lambda c: {**base, "called": ["create_workflow"], "tool_calls": [{"name": "create_workflow", "args": c["args"]}], "code": ""}
        return {"n1": mk(n1), "bestN": mk(bestN), "selfcon": mk(selfcon),
                "score_n1": n1["score"], "score_bestN": bestN["score"]}

    results = {"n1": [], "bestN": [], "selfcon": []}
    with cf.ThreadPoolExecutor(max_workers=workers) as ex:
        for fut in cf.as_completed([ex.submit(one, s) for s in scens]):
            r = fut.result()
            if r:
                for k in ("n1", "bestN", "selfcon"):
                    results[k].append(r[k])
    for k in ("n1", "bestN", "selfcon"):
        results[k].sort(key=lambda x: x.get("id", ""))
        (OUT / f"{k}.json").write_text(json.dumps(results[k], ensure_ascii=False, indent=2))
    n = len(results["n1"])
    s1 = sum(1 for r in results["n1"] if verify_workflow(r["tool_calls"][0]["args"])[0])
    sb = sum(1 for r in results["bestN"] if verify_workflow(r["tool_calls"][0]["args"])[0])
    print(f"n={n} | 结构验证 pass: n1={s1}/{n}={100*s1/n:.0f}% → bestN={sb}/{n}={100*sb/n:.0f}% | ¥{ds.cumulative_cost_rmb():.2f}")
    print("(语义判官对比见 wf_judge_r4bestofn.js)")


if __name__ == "__main__":
    run()
