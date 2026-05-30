"""Programmatic workflow STRUCTURAL verifier — the cheap selector for best-of-N + the metric for
reflexion/self-consistency. Parses a create_workflow's ops and checks the validated structural
rubric (when-guards / no-dangling / default / terminal / case-no-add-edge / retry-emit). Returns
(passed:bool, score:0..1, fails:[...]). NOT a semantic judge — just structure (correlates well).
"""
from __future__ import annotations
import json
import re

_REF_RE = re.compile(r"\b(?:fn|hd|ag|wf)_[0-9a-zA-Z]{4,}\b")


def verify_workflow(args):
    """args = the create_workflow tool args ({name, ops:[...]})."""
    fails = []
    if not isinstance(args, dict):
        return False, 0.0, ["unparseable args"]
    ops = args.get("ops", [])
    if not isinstance(ops, list) or not ops:
        return False, 0.0, ["no ops"]

    nodes = {}        # id -> node dict
    case_nodes = []   # (id, node)
    add_edges = []    # (from, to)
    for o in ops:
        if not isinstance(o, dict):
            continue
        op = o.get("op")
        if op == "add_node":
            n = o.get("node", {})
            nid = n.get("id")
            if nid:
                nodes[nid] = n
                if n.get("type") == "case":
                    case_nodes.append((nid, n))
        elif op in ("add_edge", "connect"):
            add_edges.append((o.get("from"), o.get("to")))

    node_ids = set(nodes)
    checks = {}

    # 1. has a trigger
    checks["has_trigger"] = any(n.get("type") == "trigger" for n in nodes.values())

    # 2/3/4. case nodes: per-branch when + to; final default (when:"true" or _default); not key-match.
    case_ok = True
    nodangle_ok = True
    default_ok = True
    case_no_addedge = True
    for cid, n in case_nodes:
        cfg = n.get("config", {}) if isinstance(n.get("config"), dict) else {}
        branches = cfg.get("branches")
        if not isinstance(branches, dict) or not branches:
            # key-match style (expression + no per-branch when) → the LLM-hostile design
            if cfg.get("expression") is not None:
                case_ok = False
            else:
                case_ok = False
            continue
        bvals = list(branches.values())
        # each branch needs a `when` guard; `to` is OPTIONAL (a terminal default branch
        # like {"when":"true"} legitimately omits `to` — the flow ends there, per the design).
        if not all(isinstance(b, dict) and ("when" in b) for b in bvals):
            case_ok = False
        # final default: some branch when == "true" (string) OR a branch named _default
        has_default = any((isinstance(b, dict) and str(b.get("when")).strip() == "true") for b in bvals) \
            or "_default" in branches or "default" in branches
        if not has_default:
            default_ok = False
        # branch targets must point to existing nodes
        for b in bvals:
            if isinstance(b, dict) and b.get("to") and b["to"] not in node_ids:
                nodangle_ok = False
        # case shouldn't ALSO route via add_edge
        if any(frm == cid for frm, _ in add_edges):
            case_no_addedge = False
    checks["case_when_guards"] = case_ok if case_nodes else True
    checks["case_has_default"] = default_ok if case_nodes else True
    checks["case_no_dangling"] = nodangle_ok if case_nodes else True
    checks["case_no_addedge"] = case_no_addedge if case_nodes else True

    # 5. add_edge targets exist
    edge_ok = all((t in node_ids) for _, t in add_edges if t) and all((f in node_ids) for f, _ in add_edges if f)
    checks["edges_point_to_nodes"] = edge_ok

    # 6. no terminal node has to:null literal (dangling) — scan ops blob
    blob = json.dumps(ops, ensure_ascii=False)
    checks["no_to_null"] = '"to": null' not in blob and '"to":null' not in blob

    # 7. if a back-edge exists (case 'to' points to a node that appears earlier / forms a loop),
    #    a counter should be emitted somewhere (heuristic: 'attempt' or '+ 1' in an emit).
    has_emit_counter = ("emit" in blob and ("+ 1" in blob or "+1" in blob or "attempt" in blob))
    # only require if there's plausibly a loop (a case branch 'to' equals an already-defined earlier node)
    loop_suspected = False
    seen = []
    for o in ops:
        if isinstance(o, dict) and o.get("op") == "add_node":
            seen.append(o.get("node", {}).get("id"))
        if isinstance(o, dict) and o.get("op") == "add_node" and o.get("node", {}).get("type") == "case":
            cfg = o["node"].get("config", {})
            for b in (cfg.get("branches", {}) or {}).values():
                if isinstance(b, dict) and b.get("to") in seen:
                    loop_suspected = True
    checks["retry_emits_counter"] = has_emit_counter if loop_suspected else True

    fails = [k for k, v in checks.items() if not v]
    score = sum(1 for v in checks.values() if v) / len(checks)
    passed = len(fails) == 0
    return passed, round(score, 3), fails


if __name__ == "__main__":
    import sys
    d = json.load(open(sys.argv[1]))
    for r in (d if isinstance(d, list) else d.get("reps", [])):
        tcs = r.get("tool_calls", [])
        a = tcs[0].get("args", {}) if tcs else {}
        print(r.get("id"), verify_workflow(a))
