"""R4 ② mini-GEPA: reflective prompt evolution. The MUTATOR is the orchestrating Claude (me) — GEPA's
design is exactly an LLM mutator reading natural-language failure traces. This script is the EVAL/metric:
  python3 round4_gepa_eval.py <variant_file> <split>     split = train|heldout|all
runs deepseek on that split of HARD create_workflow scenarios with SYSTEM + <variant_file> teaching
appended, scores with the structural verifier (cheap, fast iteration), prints pass-rate + per-FAIL traces
(for me to reflect on). Final held-out eval uses the semantic judge separately. API-only.
Writes the run to /tmp/r4gepa/<split>_<variant_tag>.json so the judge can read it.
"""
from __future__ import annotations
import json, os, sys
from pathlib import Path
import catalog_v2 as cat
import deepseek_client as ds
from wave1_gen import SYSTEM, parse_args
from wf_verify import verify_workflow

OUT = Path("/tmp/r4gepa"); OUT.mkdir(exist_ok=True)
TOOLS = cat.workflow_tools("V3-full-teaching")
ALL = json.loads(Path("/tmp/r3complex/create_workflow.json").read_text())
TRAIN = ALL[:20]
HELDOUT = ALL[40:]


def run(variant_file, split, workers=16):
    if not os.environ.get("DEEPSEEK_API_KEY"):
        kf = Path("/tmp/.ds_key")
        if kf.exists():
            os.environ["DEEPSEEK_API_KEY"] = kf.read_text().strip()
    variant = Path(variant_file).read_text() if variant_file and Path(variant_file).exists() else ""
    tag = Path(variant_file).stem if variant_file else "base"
    sysp = SYSTEM + ("\n\n" + variant if variant else "")
    scens = {"train": TRAIN, "heldout": HELDOUT, "all": ALL}[split]
    import concurrent.futures as cf
    budget = {"v": False}

    def one(s):
        if budget["v"]:
            return None
        try:
            res = ds.chat_complete(messages=[{"role": "system", "content": sysp}, {"role": "user", "content": s["user"]}],
                                   tools=TOOLS, scenario="r4gepa", variant=tag,
                                   temperature=None, max_tokens=16000, disable_thinking=False)
            tcs = res.effective_tool_calls
            a = parse_args(tcs[0]) if tcs else {}
            p, score, fails = verify_workflow(a)
            return {"id": s["id"], "user": s["user"], "intent": s.get("intent", ""), "rubric": s.get("rubric", []),
                    "called": ["create_workflow"] if tcs else [], "tool_calls": [{"name": "create_workflow", "args": a}],
                    "code": "", "_passed": p, "_fails": fails}
        except ds.BudgetExhausted:
            budget["v"] = True
            return None
        except Exception:
            return {"id": s.get("id"), "_passed": False, "_fails": ["err"], "tool_calls": [{"name": "create_workflow", "args": {}}]}

    recs = []
    with cf.ThreadPoolExecutor(max_workers=workers) as ex:
        for fut in cf.as_completed([ex.submit(one, s) for s in scens]):
            r = fut.result()
            if r:
                recs.append(r)
    recs.sort(key=lambda x: x.get("id", ""))
    (OUT / f"{split}_{tag}.json").write_text(json.dumps(recs, ensure_ascii=False, indent=2))
    n = len(recs); p = sum(1 for r in recs if r.get("_passed"))
    from collections import Counter
    fc = Counter(f for r in recs for f in r.get("_fails", []))
    print(f"[{split}/{tag}] 结构 pass {p}/{n}={100*p/n if n else 0:.0f}% | ¥{ds.cumulative_cost_rmb():.2f}")
    print("失败检查分布:", dict(fc))
    print("FAIL 轨迹(给 mutator 反思):")
    for r in recs:
        if not r.get("_passed"):
            print(f"  {r['id']}: fails={r['_fails']} | 需求={r['user'][:55]}")


if __name__ == "__main__":
    run(sys.argv[1] if len(sys.argv) > 1 else "", sys.argv[2] if len(sys.argv) > 2 else "train")
