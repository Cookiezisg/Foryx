"""No-token self-test of the Python plumbing: run_model → score → memory, with a MOCKED model.

Proves the pipeline wires up (incl. real code execution + the exec-overrides-judge rule + round
capsule + STATE regen) without spending any DeepSeek budget. Run: `python3 engine/smoke_mock.py`.
"""
from __future__ import annotations

import json
import tempfile
from pathlib import Path

import memory as mem
import model_client as mc
import run_model
import score as sc


def _fake_result(code: str):
    args = json.dumps({"name": "f", "kind": "normal", "code": code, "summary": "mock"})
    r = mc.Result(content="", reasoning="", finish_reason="tool_calls", cost_rmb=0.0, leaked=False,
                  tool_calls=[{"id": "c1", "function": {"name": "create_function", "arguments": args}}])
    r._leak = []
    return r


GOOD = "def calc(x):\n    return x * 2"
BAD = "def calc(x):\n    return x * 3"  # harness asserts ==6 → fails


def main() -> None:
    td = Path(tempfile.mkdtemp()) / "forgify"
    # minimal target (copy just what run_model needs from the real seed)
    real = mem.target_dir()
    (td / "surfaces").mkdir(parents=True)
    for f in (real / "surfaces").glob("*"):
        (td / "surfaces" / f.name).write_text(f.read_text())
    (td / "config.json").write_text((real / "config.json").read_text())
    (td / "axes.json").write_text((real / "axes.json").read_text())
    (td / "backlog.json").write_text((real / "backlog.json").read_text())
    for nm in ("CONCLUSIONS.md", "ROUNDS.md", "STATE.md", "changelog.md", "recommendations.md"):
        (td / nm).write_text((real / nm).read_text())
    (td / "scores.jsonl").write_text("")

    scenarios = [
        {"id": "create_function_1", "user": "写个翻倍函数", "intent": "calc doubles", "expected_tool": "create_function",
         "rubric": ["doubles"], "code_test": {"harness": "assert calc(3)==6\nprint('OK')"}},
        {"id": "create_function_2", "user": "写个翻倍函数", "intent": "calc doubles", "expected_tool": "create_function",
         "rubric": ["doubles"], "code_test": {"harness": "assert calc(3)==6\nprint('OK')"}},
    ]
    # mock the model: scenario 1 → good code (exec clean), scenario 2 → bad code (exec error)
    seq = iter([_fake_result(GOOD), _fake_result(BAD)])
    mc.chat = lambda *a, **k: next(seq)  # type: ignore

    n, rd = mem.start_round(td)
    traces = run_model.run(td, scenarios, rd, tool_names=["create_function"], config=mem.load_json(td / "config.json"))
    assert traces[0]["called"] == ["create_function"], traces[0]
    assert traces[0]["exec_result"]["exec"] == "clean", traces[0]["exec_result"]
    assert traces[1]["exec_result"]["exec"] == "error", traces[1]["exec_result"]

    axes = [a["key"] for a in mem.load_json(td / "axes.json", [])]
    # judge says BOTH usage=true, but exec must OVERRIDE scenario 2 → usage=false
    verdicts = [{"id": "create_function_1", "selection": True, "usage": True},
                {"id": "create_function_2", "selection": True, "usage": True}]
    scored = sc.score(traces, verdicts, axes)
    usage = next(r for r in scored["rows"] if r["axis"] == "usage")
    assert usage["pct"] == 50, f"exec override failed: {usage}"  # 1 clean / 1 error = 50%

    mem.record_scores(td, n, scored["rows"])
    mem.write_capsule(td, n, f"# Round {n:04d} — smoke\n类型: meta\n结果: selection 100%, usage 50% (exec override worked)\n")
    mem.finalize_round(td, n, type="meta", goal="smoke plumbing", headline="usage 50% (exec>judge ✓)", cost=0.0)

    state = (td / "STATE.md").read_text()
    assert "create_function" in state and "已转 **1** 轮" in state, state
    print("SMOKE OK — run_model + real-exec + exec-overrides-judge + score + round capsule + STATE regen all wired.")
    print(f"  selection {next(r for r in scored['rows'] if r['axis']=='selection')['pct']}% · usage {usage['pct']}% (exec override)")
    print(f"  STATE/ROUNDS regenerated under {td}")


if __name__ == "__main__":
    main()
