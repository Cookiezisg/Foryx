"""Render the 91-tool catalog (spec_catalog.py) → markdown for doc 15-tool-catalog.

Single source of truth: spec_catalog.ALL_TOOLS. Re-run to keep the doc in sync with the
validated tool descriptions. Output → documents/.../15-tool-catalog.md
"""

from __future__ import annotations

import json
from pathlib import Path

from spec_catalog import FAMILIES, count

FAM_TITLE = {
    "function": "Function 锻造(11)", "handler": "Handler 锻造(12)", "agent": "Agent 锻造(11)",
    "workflow": "Workflow 编排(9)", "lifecycle": "Workflow 生命周期(3)", "runtime": "运行时观察(5)",
    "diagnosis": "错误诊断 + 修复(5)", "mcp": "资产 — MCP(5)", "skill": "资产 — Skill(3)",
    "document": "资产 — Document(7)", "memory": "资产 — Memory(3)", "base": "主对话基础(17)",
}
# validation mode per family (per §3 of CLAUDE plan)
FAM_MODE = {
    "function": "create/edit=CODE(真执行 90%);余 USAGE", "handler": "create/edit=CODE(真执行 100%);余 USAGE",
    "agent": "create/edit=ARTIFACT(90%);余 USAGE", "workflow": "create/edit=ARTIFACT(55%→check/fix);余 USAGE",
    "lifecycle": "USAGE / trigger=ARTIFACT(payload)", "runtime": "USAGE(诊断链 6/7)", "diagnosis": "USAGE(诊断链 6/7)",
    "mcp": "USAGE / call=ARTIFACT(args);lazy 激活验证", "skill": "USAGE", "document": "create/edit=CONTENT(100%);余 USAGE",
    "memory": "write=CONTENT(100%);余 USAGE", "base": "USAGE(91 全集选择 ~91% reasonable)",
}


def render() -> str:
    c = count()
    out = ["# 15 — 全 91 工具目录(每个 tool 告诉 AI 的描述,最新版)",
           "",
           "> **这是「每个 tool 的描述怎么告诉 AI」的一站式参考。** 由 `research/llm-experiments/render_spec.py` 从 `spec_catalog.py`(可执行 source of truth)渲染——改 `spec_catalog.py` 后重跑即同步。",
           "> 每个工具:最终 `Description()`(告诉 AI 的原文)+ 必填/可选参数。验证模式见各家族标题。",
           "> 这是已实测验证的基线;**动土前的改动(case→when:、ops/node pin 形状、forge 教学等 before/after)见 [13-llm-facing-implementation-guide.md](./13-llm-facing-implementation-guide.md)**;实验依据见 [14-llm-validation-research-record.md](./14-llm-validation-research-record.md)。",
           f"> 合计 **{c['TOTAL']}** 工具。被测 deepseek-v4-flash。",
           ""]
    for fam, tools in FAMILIES.items():
        out.append(f"## {FAM_TITLE[fam]}")
        out.append(f"_验证模式:{FAM_MODE[fam]}_")
        out.append("")
        for t in tools:
            f = t["function"]
            name = f["name"]
            desc = f["description"].replace("\n", " ").strip()
            req = f["parameters"].get("required", [])
            props = list(f["parameters"].get("properties", {}).keys())
            opt = [p for p in props if p not in req]
            params = ""
            if req:
                params += f"  必填:`{', '.join(req)}`"
            if opt:
                params += f"  可选:`{', '.join(opt)}`"
            out.append(f"### `{name}`")
            out.append(f"{desc}")
            if params.strip():
                out.append(f"{params.strip()}")
            out.append("")
    return "\n".join(out)


if __name__ == "__main__":
    md = render()
    dest = Path(__file__).resolve().parents[2] / "documents/version-1.2/adhoc-topic-documents/workflow-revamp/15-tool-catalog.md"
    dest.write_text(md)
    print(f"wrote {dest} ({len(md)} chars, {md.count('### ')} tools)")
