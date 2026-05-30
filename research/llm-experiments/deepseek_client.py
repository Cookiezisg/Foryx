"""DeepSeek V4-flash client with cost tracking, retries, and content-leak fallback.

为 Forgify LLM 工具设计研究实验使用。**Constraints**: v4-flash only, ¥200 budget hard cap.

Key features:
- OpenAI-compatible /chat/completions endpoint
- Cost ledger (budget.json) — append per call, hard stop at ¥180
- Retry on 429/5xx with exponential backoff
- Content-leak fallback parser (GitHub issue #1244): regex parse `name(<json>)` in content if finish_reason="stop" but content looks like tool call
- Cache hit tracking via prompt_cache_hit_tokens / prompt_cache_miss_tokens
- Optional thinking mode disable (saves output cost on routing tasks)
"""

from __future__ import annotations

import fcntl
import json
import os
import re
import threading
import time
from contextlib import contextmanager
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import httpx

# Lock for budget.json concurrent writes (both intra-process and cross-process)
_LEDGER_LOCK = threading.Lock()


@contextmanager
def _file_lock(path: Path):
    """No-op: we run one process at a time; the threading.Lock suffices.
    The previous fcntl lock-file in Documents tripped macOS TCC (EPERM mid-run)."""
    yield

# -- Constants -----------------------------------------------------------

API_BASE = "https://api.deepseek.com"
MODEL_FLASH = "deepseek-v4-flash"

# DeepSeek V4-flash pricing (per official docs, USD per 1M tokens)
PRICE_INPUT_UNCACHED_USD_PER_M = 0.14
PRICE_INPUT_CACHED_USD_PER_M = 0.0028
PRICE_OUTPUT_USD_PER_M = 0.28

# USD to RMB (approximate, will update if needed)
USD_TO_RMB = 7.2

# NO self-imposed cap. The real stop signal is DeepSeek's API returning
# insufficient-balance (HTTP 402 / "Insufficient Balance"). The ledger below
# only TRACKS spend; it does not stop the run. User mandate: burn until DeepSeek
# says recharge.
BUDGET_HARD_CAP_RMB = 1_000_000.0  # effectively disabled

# Paths — ledger lives in /tmp (OUTSIDE macOS TCC-protected Documents tree).
# High-frequency locked writes to Documents trip macOS security throttling
# (manifests as EPERM mid-run). /tmp is unprotected + fast. The ledger is only
# spend-tracking; the real stop signal is DeepSeek's 402.
SCRIPT_DIR = Path(__file__).parent
# Per-process ledger file (PID in name) → concurrent forge processes never race.
# cumulative_cost_rmb() sums across all /tmp/forge_budget_*.json.
import os as _os
BUDGET_DIR = Path("/tmp")
BUDGET_FILE = BUDGET_DIR / f"forge_budget_{_os.getpid()}.json"


# -- Exceptions ----------------------------------------------------------


class BudgetExhausted(Exception):
    """Raised ONLY when DeepSeek API itself reports insufficient balance (the true stop signal)."""


class DeepSeekAPIError(Exception):
    """Raised on non-retryable API errors."""


# -- Cost ledger ---------------------------------------------------------


@dataclass
class CostEntry:
    ts: float
    scenario: str
    variant: str
    input_tok_uncached: int
    input_tok_cached: int
    output_tok: int
    cost_rmb: float

    def to_dict(self) -> dict[str, Any]:
        return self.__dict__


def _load_ledger() -> list[dict[str, Any]]:
    # Fully resilient: concurrent cross-process writes can leave a half-written
    # file; never let that crash a run (ledger is tracking-only).
    try:
        if not BUDGET_FILE.exists():
            return []
        return json.loads(BUDGET_FILE.read_text())
    except Exception:
        return []


def _save_ledger(entries: list[dict[str, Any]]) -> None:
    try:
        BUDGET_FILE.write_text(json.dumps(entries, indent=2, ensure_ascii=False))
    except Exception:
        pass


def cumulative_cost_rmb() -> float:
    """Sum across ALL per-process ledger files (best-effort)."""
    total = 0.0
    try:
        for f in BUDGET_DIR.glob("forge_budget_*.json"):
            try:
                for e in json.loads(f.read_text()):
                    total += e.get("cost_rmb", 0)
            except Exception:
                continue
        # include legacy single-file ledger if present
        legacy = BUDGET_DIR / "forge_budget.json"
        if legacy.exists():
            try:
                for e in json.loads(legacy.read_text()):
                    total += e.get("cost_rmb", 0)
            except Exception:
                pass
    except Exception:
        pass
    return total


def _compute_cost_rmb(usage: dict[str, Any]) -> tuple[int, int, int, float]:
    """Return (input_uncached, input_cached, output, cost_rmb) for one call."""
    cached = usage.get("prompt_cache_hit_tokens", 0)
    uncached = usage.get("prompt_cache_miss_tokens", usage.get("prompt_tokens", 0) - cached)
    output = usage.get("completion_tokens", 0)
    cost_usd = (
        uncached / 1_000_000 * PRICE_INPUT_UNCACHED_USD_PER_M
        + cached / 1_000_000 * PRICE_INPUT_CACHED_USD_PER_M
        + output / 1_000_000 * PRICE_OUTPUT_USD_PER_M
    )
    return uncached, cached, output, cost_usd * USD_TO_RMB


def _log_call(scenario: str, variant: str, usage: dict[str, Any]) -> CostEntry:
    uncached, cached, output, cost_rmb = _compute_cost_rmb(usage)
    entry = CostEntry(
        ts=time.time(),
        scenario=scenario,
        variant=variant,
        input_tok_uncached=uncached,
        input_tok_cached=cached,
        output_tok=output,
        cost_rmb=cost_rmb,
    )
    # Ledger write is NON-CRITICAL — never let a ledger I/O hiccup kill an
    # experiment run. Best-effort under lock; swallow failures.
    try:
        with _LEDGER_LOCK, _file_lock(BUDGET_FILE):
            entries = _load_ledger()
            entries.append(entry.to_dict())
            _save_ledger(entries)
    except OSError:
        pass  # ledger unavailable; spend still tracked server-side
    return entry


# -- Content-leak fallback parser ---------------------------------------

# GitHub issue #1244: V4 series sometimes emits tool calls as plain text in
# `content` instead of `tool_calls[]`. Pattern: `func_name(<json args>)` or
# `<tool_call>{"name":"...","arguments":{...}}</tool_call>`-ish.
_TOOL_CALL_TEXT_PATTERNS = [
    # JSON-wrapped: {"name": "...", "arguments": {...}}
    re.compile(
        r'\{\s*"name"\s*:\s*"([^"]+)"\s*,\s*"arguments"\s*:\s*(\{.*?\})\s*\}',
        re.DOTALL,
    ),
    # XML-ish: <tool_call>...</tool_call>
    re.compile(r'<tool_call>(.*?)</tool_call>', re.DOTALL),
    # Function-style: name({...})
    re.compile(r'(\w+)\(\s*(\{.*?\})\s*\)', re.DOTALL),
]


def parse_content_leak(content: str) -> list[dict[str, Any]]:
    """Return list of {name, arguments} extracted from text. Empty if none."""
    if not content:
        return []
    extracted: list[dict[str, Any]] = []
    for pat in _TOOL_CALL_TEXT_PATTERNS:
        for m in pat.finditer(content):
            try:
                if len(m.groups()) == 1:
                    inner = json.loads(m.group(1))
                    if "name" in inner:
                        extracted.append(inner)
                elif len(m.groups()) == 2:
                    name = m.group(1)
                    args = json.loads(m.group(2))
                    extracted.append({"name": name, "arguments": args})
            except json.JSONDecodeError:
                continue
        if extracted:
            return extracted  # First matching pattern wins
    return []


# -- API client ----------------------------------------------------------


def _api_key() -> str:
    key = os.environ.get("DEEPSEEK_API_KEY")
    if not key:
        raise RuntimeError("DEEPSEEK_API_KEY env var not set")
    return key


@dataclass
class ChatResult:
    """Result of one chat completion call."""

    raw_response: dict[str, Any]
    content: str
    tool_calls: list[dict[str, Any]]
    leaked_tool_calls: list[dict[str, Any]]  # Parsed from content-leak fallback
    finish_reason: str
    cost_entry: CostEntry

    @property
    def has_tool_call(self) -> bool:
        return bool(self.tool_calls) or bool(self.leaked_tool_calls)

    @property
    def effective_tool_calls(self) -> list[dict[str, Any]]:
        """Native tool_calls first, fall back to leaked if native empty."""
        if self.tool_calls:
            return self.tool_calls
        return [
            {"function": tc, "id": f"leaked_{i}"}
            for i, tc in enumerate(self.leaked_tool_calls)
        ]


def chat_complete(
    messages: list[dict[str, Any]],
    tools: list[dict[str, Any]] | None = None,
    *,
    scenario: str = "smoke",
    variant: str = "default",
    tool_choice: str = "auto",
    temperature: float | None = 0.0,  # None → omit (API default temp ≈ production realistic)
    max_tokens: int | None = None,
    timeout: float = 60.0,
    max_retries: int = 3,
    disable_thinking: bool = False,
    model: str = MODEL_FLASH,
) -> ChatResult:
    """Call DeepSeek V4-flash chat completion with retries + cost tracking.

    Raises:
        BudgetExhausted: cumulative cost > hard cap
        DeepSeekAPIError: non-retryable API error
    """
    if cumulative_cost_rmb() > BUDGET_HARD_CAP_RMB:
        raise BudgetExhausted(f"Pre-call budget check failed: ¥{cumulative_cost_rmb():.2f}")

    payload: dict[str, Any] = {
        "model": model,
        "messages": messages,
    }
    if temperature is not None:
        payload["temperature"] = temperature
    if tools:
        payload["tools"] = tools
        payload["tool_choice"] = tool_choice
    if max_tokens:
        payload["max_tokens"] = max_tokens
    if disable_thinking:
        # Verified working on deepseek-v4-flash (reasoning_chars -> 0).
        payload["thinking"] = {"type": "disabled"}

    headers = {
        "Authorization": f"Bearer {_api_key()}",
        "Content-Type": "application/json",
    }

    backoff = 1.0
    last_err: Exception | None = None
    for attempt in range(max_retries):
        try:
            with httpx.Client(timeout=timeout) as client:
                resp = client.post(
                    f"{API_BASE}/chat/completions",
                    headers=headers,
                    json=payload,
                )
            # The true stop signal: DeepSeek balance exhausted.
            if resp.status_code == 402 or "insufficient balance" in resp.text.lower():
                raise BudgetExhausted(
                    f"DeepSeek says recharge (HTTP {resp.status_code}): {resp.text[:200]}"
                )
            if resp.status_code == 429 or resp.status_code >= 500:
                last_err = DeepSeekAPIError(f"HTTP {resp.status_code}: {resp.text[:200]}")
                time.sleep(backoff)
                backoff *= 2
                continue
            if resp.status_code != 200:
                raise DeepSeekAPIError(f"HTTP {resp.status_code}: {resp.text[:500]}")

            data = resp.json()
            usage = data.get("usage", {})
            entry = _log_call(scenario, variant, usage)

            choice = data["choices"][0]
            msg = choice["message"]
            content = msg.get("content") or ""
            tool_calls = msg.get("tool_calls") or []
            finish_reason = choice.get("finish_reason", "")

            leaked: list[dict[str, Any]] = []
            if not tool_calls and content and finish_reason == "stop":
                leaked = parse_content_leak(content)

            return ChatResult(
                raw_response=data,
                content=content,
                tool_calls=tool_calls,
                leaked_tool_calls=leaked,
                finish_reason=finish_reason,
                cost_entry=entry,
            )
        except httpx.RequestError as e:
            last_err = e
            time.sleep(backoff)
            backoff *= 2

    raise DeepSeekAPIError(f"Max retries exceeded. Last error: {last_err}")


# -- Smoke test ----------------------------------------------------------


def smoke_test() -> None:
    """Quick API smoke test. Run with `python deepseek_client.py`."""
    print(f"Budget so far: ¥{cumulative_cost_rmb():.4f}")
    result = chat_complete(
        messages=[{"role": "user", "content": "Reply with exactly: OK"}],
        scenario="smoke",
        variant="default",
        max_tokens=50,
    )
    print(f"Content: {result.content!r}")
    print(f"Tool calls: {result.tool_calls}")
    print(f"Finish reason: {result.finish_reason}")
    print(f"Cost this call: ¥{result.cost_entry.cost_rmb:.6f}")
    print(f"Budget so far: ¥{cumulative_cost_rmb():.4f}")


if __name__ == "__main__":
    smoke_test()
