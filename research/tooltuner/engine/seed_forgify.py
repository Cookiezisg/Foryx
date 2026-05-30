"""One-time bootstrap: materialize the Forgify target from the existing research catalog.

Dumps surfaces/{tools,grouping}.json from spec_catalog (llm-experiments, to be retired) and
writes the empty three-tier memory (STATE/CONCLUSIONS/ROUNDS/scores). Prose surfaces
(system_prompt/teaching/examples) + curated backlog are hand-written separately.

Run once: `python3 engine/seed_forgify.py`. After llm-experiments retires the target is already
materialized, so this needn't run again (re-seed would come from Forgify's real tool code instead).
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

# spec_catalog still lives in the soon-retired llm-experiments dir; import for the one-time dump.
ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT.parent / "llm-experiments"))
import spec_catalog as sc  # noqa: E402

TARGET = ROOT / "target"
SURF = TARGET / "surfaces"

# the 6 lazy domain groups (validated domain-6 > 11-split); rest is resident.
LAZY_FAMILIES = ["function", "handler", "workflow", "mcp", "document", "skill"]


def _unwrap(t: dict) -> dict:
    f = t.get("function") or t
    return {"name": f["name"], "description": f["description"], "parameters": f["parameters"]}


def main() -> None:
    SURF.mkdir(parents=True, exist_ok=True)
    (TARGET / "rounds").mkdir(exist_ok=True)

    tools, grouping_lazy, resident = [], {}, []
    for fam, fam_tools in sc.FAMILIES.items():
        names = []
        for t in fam_tools:
            u = _unwrap(t)
            tools.append(u)
            names.append(u["name"])
        if fam in LAZY_FAMILIES:
            grouping_lazy[fam] = names
        else:
            resident += names

    (SURF / "tools.json").write_text(json.dumps(tools, ensure_ascii=False, indent=2))
    (SURF / "grouping.json").write_text(json.dumps(
        {"resident": resident, "lazy": grouping_lazy,
         "_note": "domain-6 lazy groups (validated > 11-split); a tunable surface."},
        ensure_ascii=False, indent=2))

    # axes: the two seed dimensions (growable — the AI adds more).
    (TARGET / "axes.json").write_text(json.dumps([
        {"key": "selection", "name": "选对工具", "how_judged": "judge: did the model call the expected tool?",
         "added_round": 0, "why": "seed axis (议题1:描述清不清楚)"},
        {"key": "usage", "name": "用对/结果正确", "how_judged": "judge: given it called, are args/artifact correct per rubric? (code: real subprocess run)",
         "added_round": 0, "why": "seed axis (议题2:指令/schema 约束到对没)"},
    ], ensure_ascii=False, indent=2))

    (TARGET / "config.json").write_text(json.dumps({
        "model_under_test": "deepseek-v4-flash",
        "backend": "single_turn",          # agent may switch to multi-turn per-experiment
        "domain_hint": "电商/客服/运维/金融/内容/IoT/HR/物流/医疗/教育/SaaS/数据/社交/游戏",
        "judges_n": 3,
    }, ensure_ascii=False, indent=2))

    # empty NOW + data files (created with headers so a fresh target reads cleanly).
    (TARGET / "scores.jsonl").write_text("")
    (TARGET / "changelog.md").write_text("# Changelog — 被采纳的 surface 改动(交付物 provenance)\n\n_空。每条:surface · tool · before→after · ab lift · why_\n")
    (TARGET / "recommendations.md").write_text("# Recommendations — 设计级建议(给人,不自动改)\n\n_空。每条:date · 面/工具 · 建议 · 证据 · 状态_\n")
    (TARGET / "CONCLUSIONS.md").write_text("# CONCLUSIONS — durable 结论\n\n## 已证真理(G 式)\n_从 round 提升而来,带证据轮次,去重。_\n\n## known-good(查实没问题,别再碰)\n_空_\n\n## 已采纳设计变更\n_空_\n")
    (TARGET / "ROUNDS.md").write_text("# ROUNDS — 轮次索引(一轮一行)\n\n| NNNN | 日期 | 目标 | 头条结果 | 花费 |\n|---|---|---|---|---|\n")
    (TARGET / "STATE.md").write_text("# STATE — 当前真相(每轮末重生成,勿手改)\n\n_尚未跑过任何轮。运行第一轮后由 memory.regen_state 生成。_\n")

    print(f"seeded {TARGET}")
    print(f"  surfaces/tools.json: {len(tools)} tools")
    print(f"  grouping: resident={len(resident)} lazy={ {k: len(v) for k, v in grouping_lazy.items()} }")


if __name__ == "__main__":
    main()
