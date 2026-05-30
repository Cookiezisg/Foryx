"""Three-tier target memory (SPEC §2). The legibility guarantee lives here.

Source of truth (append / machine):  scores.jsonl · rounds.jsonl · backlog.json · surfaces/ ·
                                       axes.json · changelog.md · recommendations.md · CONCLUSIONS.md
Rendered views (regenerated, never hand-edited):  STATE.md · ROUNDS.md

So no matter how many rounds, "current truth" (STATE/CONCLUSIONS) stays small + correct, each round is
an immutable capsule under rounds/NNNN/, and ROUNDS.md is a one-line-per-round index. Raw traces GC.
"""
from __future__ import annotations

import datetime as _dt
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def target_dir() -> Path:
    return ROOT / "target"


def _now() -> str:
    return _dt.datetime.now().strftime("%Y-%m-%d %H:%M")


def _today() -> str:
    return _dt.date.today().isoformat()


def _load_jsonl(p: Path) -> list[dict]:
    if not p.exists():
        return []
    return [json.loads(ln) for ln in p.read_text().splitlines() if ln.strip()]


def _append_jsonl(p: Path, rows: list[dict]) -> None:
    with p.open("a") as f:
        for r in rows:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")


def load_json(p: Path, default=None):
    return json.loads(p.read_text()) if p.exists() else default


# ── surfaces ────────────────────────────────────────────────────────────
def load_surfaces(td: Path) -> dict:
    """All LLM-facing surfaces as {name: content}. tools/grouping are parsed json; rest is text."""
    s = td / "surfaces"
    out: dict = {}
    for p in sorted(s.glob("*")):
        out[p.stem] = json.loads(p.read_text()) if p.suffix == ".json" else p.read_text()
    return out


def surfaces_hash(td: Path) -> str:
    h = hashlib.sha256()
    for p in sorted((td / "surfaces").glob("*")):
        h.update(p.read_bytes())
    return h.hexdigest()[:12]


def assemble_system_prompt(td: Path) -> str:
    """Final system prompt fed to the model-under-test: identity → examples → teaching (殿后)."""
    s = td / "surfaces"
    parts = []
    for name in ("system_prompt.md", "examples.md", "teaching.md"):
        p = s / name
        if p.exists():
            parts.append(p.read_text())
    return "\n\n".join(parts)


# ── rounds (immutable capsules) ─────────────────────────────────────────
def next_round(td: Path) -> int:
    rows = _load_jsonl(td / "rounds.jsonl")
    return (max((r["round"] for r in rows), default=0)) + 1


def start_round(td: Path) -> tuple[int, Path]:
    n = next_round(td)
    d = td / "rounds" / f"{n:04d}"
    (d / "traces").mkdir(parents=True, exist_ok=True)
    return n, d


def record_scores(td: Path, round_id: int, rows: list[dict]) -> None:
    """rows: [{tool, axis, pct, n, ci}]. Stamps round + ts + surfaces_hash."""
    sh = surfaces_hash(td)
    _append_jsonl(td / "scores.jsonl", [
        {"round": round_id, "ts": _now(), "surfaces_hash": sh, **r} for r in rows])


def write_capsule(td: Path, round_id: int, md: str) -> None:
    (td / "rounds" / f"{round_id:04d}" / "round.md").write_text(md)


def finalize_round(td: Path, round_id: int, *, type: str, goal: str, headline: str, cost: float) -> None:
    """Close a round cleanly (SPEC §2.4): index it + regenerate the human views."""
    _append_jsonl(td / "rounds.jsonl", [{
        "round": round_id, "date": _today(), "type": type,
        "goal": goal, "headline": headline, "cost": round(cost, 4)}])
    regen_views(td)


# ── curated appends ─────────────────────────────────────────────────────
def append_changelog(td: Path, line: str) -> None:
    with (td / "changelog.md").open("a") as f:
        f.write(f"\n- [{_today()}] {line}")


def append_recommendation(td: Path, surface_or_tool: str, rec: str, evidence: str) -> None:
    with (td / "recommendations.md").open("a") as f:
        f.write(f"\n- **[{_today()}] {surface_or_tool}** — {rec}  _(证据: {evidence}; 状态: open)_")


def promote_conclusion(td: Path, text: str) -> None:
    """Append a durable conclusion under 已证真理. The agent curates/dedups the prose."""
    p = td / "CONCLUSIONS.md"
    p.write_text(p.read_text().replace(
        "## 已证真理(G 式)\n", f"## 已证真理(G 式)\n- [{_today()}] {text}\n", 1))


# ── rendered views (regenerated) ────────────────────────────────────────
def regen_views(td: Path) -> None:
    _regen_rounds(td)
    _regen_state(td)


def _regen_rounds(td: Path) -> None:
    rows = _load_jsonl(td / "rounds.jsonl")
    out = ["# ROUNDS — 轮次索引(一轮一行,从 rounds.jsonl 重生成)", "",
           "| NNNN | 日期 | 类型 | 目标 | 头条结果 | 花费 |", "|---|---|---|---|---|---|"]
    for r in rows:
        out.append(f"| {r['round']:04d} | {r['date']} | {r.get('type','')} | {r['goal']} | {r['headline']} | ¥{r['cost']} |")
    (td / "ROUNDS.md").write_text("\n".join(out) + "\n")


def _regen_state(td: Path) -> None:
    scores = _load_jsonl(td / "scores.jsonl")
    rounds = _load_jsonl(td / "rounds.jsonl")
    axes = [a["key"] for a in load_json(td / "axes.json", [])]
    backlog = load_json(td / "backlog.json", {"open": []})

    # latest pct per (tool, axis): last score row wins (scores.jsonl is chronological).
    latest: dict = {}
    for r in scores:
        latest[(r["tool"], r["axis"])] = r
    tools = sorted({t for (t, _a) in latest})

    cost = round(sum(r.get("cost", 0) for r in rounds), 3)
    lines = [f"# STATE — 当前真相(第 {len(rounds)} 轮后重生成,勿手改)", "",
             f"- 已转 **{len(rounds)}** 轮 · 累计 **¥{cost}** · surfaces `{surfaces_hash(td)}`",
             f"- 维度集: {', '.join(axes) or '—'}", "",
             "## 每工具每轴现分(latest)"]
    if tools:
        lines.append("| tool | " + " | ".join(axes) + " | n |")
        lines.append("|---|" + "---|" * (len(axes) + 1))
        for t in tools:
            cells = []
            n = ""
            for a in axes:
                row = latest.get((t, a))
                cells.append(f"{row['pct']}%±{row.get('ci',0)}" if row else "—")
                if row:
                    n = row.get("n", "")
            lines.append(f"| {t} | " + " | ".join(cells) + f" | {n} |")
    else:
        lines.append("_尚无分数。跑第一轮后填充。_")

    lines += ["", "## 待办 top(完整见 backlog.json)"]
    for it in backlog.get("open", [])[:8]:
        vf = " ⚠️verify-first" if it.get("verify_first") else ""
        lines.append(f"- `{it['kind']}` **{it['target']}** — {it['note']}{vf}")
    (td / "STATE.md").write_text("\n".join(lines) + "\n")


# ── GC ──────────────────────────────────────────────────────────────────
def gc_traces(td: Path, keep_n: int = 10) -> int:
    """Drop raw traces of all but the last keep_n rounds (capsule round.md stays). Returns #pruned."""
    dirs = sorted((td / "rounds").glob("[0-9]" * 4), reverse=True)
    pruned = 0
    for d in dirs[keep_n:]:
        tr = d / "traces"
        if tr.exists():
            for f in tr.glob("*"):
                f.unlink()
            tr.rmdir()
            pruned += 1
    return pruned


if __name__ == "__main__":
    td = target_dir()
    regen_views(td)
    print(f"regenerated STATE.md + ROUNDS.md for {td.name}")
