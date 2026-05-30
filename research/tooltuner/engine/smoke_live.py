"""Minimal LIVE smoke: ONE real DeepSeek call through run_model on one create_function scenario.

Proves the live path end-to-end (model_client → DeepSeek → run_model → real code execution). Costs
~¥0.001. Writes to a throwaway /tmp round (does NOT pollute the real target's round count).
Run: `python3 engine/smoke_live.py`.
"""
from __future__ import annotations

import json
import tempfile
from pathlib import Path

import memory as mem
import model_client as mc
import run_model

td = mem.target_dir()
rd = Path(tempfile.mkdtemp()) / "0000"
(rd / "traces").mkdir(parents=True)

scen = [{
    "id": "create_function_live", "expected_tool": "create_function",
    "user": "写一个函数，输入一个整数列表，返回其中所有偶数的和。",
    "intent": "create_function summing even numbers in a list",
    "rubric": ["kind normal", "sums even numbers", "runs"],
    "code_test": {"harness": "assert f([1,2,3,4])==6\nassert f([1,3,5])==0\nprint('OK')"},
}]
# NOTE the harness calls f(...) — the model's function may be named differently; bind it.
scen[0]["code_test"]["harness"] = (
    "import builtins\n_g={k:v for k,v in list(globals().items())}\n"
    "f=next(v for k,v in _g.items() if callable(v) and k not in ('builtins',))\n"
    "assert f([1,2,3,4])==6\nassert f([1,3,5])==0\nprint('OK')")

print(f"budget before: ¥{mc.cumulative_cost_rmb():.4f}")
traces = run_model.run(td, scen, rd, tool_names=["create_function"], config=mem.load_json(td / "config.json"))
t = traces[0]
print(f"called: {t['called']}")
print(f"args.code present: {bool(next((c['args'].get('code') for c in t['tool_calls'] if c['args'].get('code')), None))}")
print(f"exec_result: {t.get('exec_result')}")
print(f"cost this scenario: ¥{t['cost_rmb']:.5f} · cumulative ¥{mc.cumulative_cost_rmb():.4f}")
print("LIVE SMOKE OK" if "create_function" in t["called"] else "LIVE SMOKE: model did not call create_function (read trace)")
