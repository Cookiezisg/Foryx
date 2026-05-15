# audit: backend/internal/infra/llm/anthropic.go

LOC: 466
Read: full file (lines 1-466)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | anthropic.go:45-49 | `body, err := buildAnthropicBody(req); if err != nil { yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: build body: %w", err)}) }` | A.4 | OK | §S16 canonical: `llm/anthropic:` prefix + `:` sub-tag + `%w`. Error surfaces to caller via StreamEvent.Err — the "channel" pattern Stream uses (no return error; events carry it). | N-A | — | — | — |
| 2 | anthropic.go:51-56 | `httpReq, err := http.NewRequestWithContext(...); if err != nil { yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/anthropic: new request: %w", err)}) }` | A.4 | OK | §S16 canonical with `: new request:` sub-tag + %w | N-A | — | — | — |
| 3 | anthropic.go:61-67 | `resp, err := c.http.Do(httpReq); if err != nil { if ctx.Err() != nil { return } yield(StreamEvent{...Err: fmt.Errorf("llm/anthropic: do: %w", err)}) }` | A.1/A.4 | OK | ctx.Err() check before yielding error is **correct** — caller cancelled means no event needed (subscriber gone). Otherwise yields wrapped error per §S16. Not §S3 silent — `return` after ctx-cancel detection is the documented exit path for cancelled streams (matches §S9 spec note: "上游 cancel 让终态写失败"). | N-A | — | — | — |
| 4 | anthropic.go:69 | `defer resp.Body.Close()` | A.1 | OK | §S3 spec carve-out: "defer X.Close() 在只读路径（Close 返错对调用方无意义）". Resp body read complete before client returns; Close failure is unrecoverable. | N-A | — | — | — |
| 5 | anthropic.go:71-75 | `if resp.StatusCode != http.StatusOK { raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)); yield(StreamEvent{Err: classifyHTTPError(resp.StatusCode, raw)}) }` | A.1 | OK | `_` discards io.ReadAll error — but the `raw` is best-effort context for classifyHTTPError (which handles empty body). Read-only operation with limit; failure means we report HTTP status without body context (graceful degrade). Not §S3 silent — original status is preserved + non-OK already reported as error. | N-A | — | — | — |
| 6 | anthropic.go:111-114 | `var e anthropicMsgStart; if err := json.Unmarshal([]byte(data), &e); err != nil { yield(StreamEvent{...Err: fmt.Errorf("llm/anthropic: parse message_start: %w", err)}); return }` | A.1/A.4 | OK | parse failure DOES surface (not silent); §S16 canonical wrap format; **terminates parse loop on error** — correct behavior to prevent silent malformed-stream continuation. | N-A | — | — | — |
| 7 | anthropic.go:121-124 | `var e anthropicBlockStart; if err := json.Unmarshal([]byte(data), &e); err != nil { yield(StreamEvent{...Err: fmt.Errorf("llm/anthropic: parse content_block_start: %w", err)}); return }` | A.4 | OK | same canonical pattern as site #6 | N-A | — | — | — |
| 8 | anthropic.go:138-140 | `var e anthropicBlockDelta; if err := json.Unmarshal([]byte(data), &e); err != nil { yield(StreamEvent{...Err: fmt.Errorf("llm/anthropic: parse content_block_delta: %w", err)}); return }` | A.4 | OK | same pattern | N-A | — | — | — |
| 9 | anthropic.go:148-150 | `var e anthropicMsgDelta; if err := json.Unmarshal([]byte(data), &e); err != nil { yield(StreamEvent{...Err: fmt.Errorf("llm/anthropic: parse message_delta: %w", err)}); return }` | A.4 | OK | same pattern | N-A | — | — | — |
| 10 | anthropic.go:168-170 | `if err := scanner.Err(); err != nil && ctx.Err() == nil { yield(StreamEvent{Err: fmt.Errorf("llm/anthropic: scan: %w", err)}) }` | A.1/A.4 | OK | scanner.Err() correctly caught + ctx-cancel check skips yield when caller gone (consistent with site #3 pattern). §S16 canonical. | N-A | — | — | — |
| 11 | anthropic.go:204-208 | `req.Messages = SanitizeMessages(req.Messages); msgs, err := toAnthropicMsgs(req.Messages); if err != nil { return nil, err }` | A.4 | EDGE | bare `return nil, err` — sentinel from toAnthropicMsg (line 267 returns wrapped `llm/anthropic: unexpected role`). §S16: canonical chain depth — buildAnthropicBody is internal, does add own context at outer Stream call site (#1 wraps as "build body:"). Defensible per "innermost sentinel" rule but inconsistent with rest of file's wrap-everywhere style. | LOW | identical UX (caller wraps); style inconsistency only | optional wrap: `return nil, fmt.Errorf("llm/anthropic.buildAnthropicBody: %w", err)` — but outer Stream already adds "build body:" context | FOUND |
| 12 | anthropic.go:248-252 | `am, err := toAnthropicMsg(m); if err != nil { return nil, err }; out = append(out, am)` | A.4 | EDGE | bare-return, same style as site #11. Inner toAnthropicMsg wraps with `llm/anthropic:` prefix. | LOW | identical UX | same as #11 | FOUND |
| 13 | anthropic.go:266-268 | `default: return anthropicMessage{}, fmt.Errorf("llm/anthropic: unexpected role %q in toAnthropicMsg", m.Role)` | A.4 | EDGE | §S16: has prefix `llm/anthropic:` + descriptive context but **NO sentinel + NO %w** (no upstream cause to wrap). Falls into "domain validation but no sentinel" pattern same as `mcp.go:#6` / `apikey.tester.go:#4` (resolved as panic) / `sandbox.go:#22` (resolved as panic). Reachability: only triggers if RoleSystem or future Role* is added without updating this switch — wiring/programming bug. | LOW | only triggers when new RoleX added without updating wire client; would result in unmapped error 500 vs documented role mismatch | could panic (matching the established pattern from earlier audit batches) OR introduce sentinel `llm.ErrUnexpectedRole` + register errmap. Recommend panic — it's a wire-client switch completeness invariant. | FOUND |
| 14 | anthropic.go:332-336 | `if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil { slog.Warn("llm/anthropic: history tool-call arguments are malformed JSON, falling back to {}", "tool_call_id", tc.ID, "tool_name", tc.Name, "raw", tc.Arguments, "err", err); input = json.RawMessage("{}") }` | A.1 | OK | **MODEL §S3 EXAMPLE**: JSON parse failure → fallback to `"{}"` (graceful degrade) **WITH explicit slog.Warn** carrying raw + err for diagnosis. Documented intent at lines 322-329 explicitly explains why (Anthropic tool_use input is required JSON; bad upstream history would otherwise 400). Combined explicit log + structured fields + documentation = compliant best-effort soft-fail per §S3 carve-out for "documented intent with audit trail". | N-A | — | — | — |
| 15 | anthropic.go:271-285 | comment block: `Anthropic enforces a 5MB per-image limit ... Per §S20 deferred WITH justification: (a) structural constraint — fix belongs to a layer that doesn't yet exist; (b) explanation — wire-layer guard would double-process or conflict with the upcoming optimizer.` | A.1 | OK | **MODEL §S20 EXAMPLE**: deferred with both required justifications: (a) structural reason (waiting for context-optimizer layer); (b) explicit consequence explanation. This satisfies the §S20 high-priority "no 留下次 without reason" rule cited verbatim in CLAUDE.md. | N-A | — | — | — |

## Sub-check（必显式，不许 silence）

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: every json.Unmarshal in SSE parser yields EventError + returns; HTTP non-OK reads body best-effort then yields HTTP error; only `_` discard at site #5 is documented graceful-degrade pattern with body-read-fallback. The malformed-history Unmarshal at site #14 is the model §S3-compliant best-effort pattern. ctx-cancel checks at sites #3 + #10 are §S9-aware exits (not silent fallback).

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — anthropic client is stateless wire transport; performs no DB or persistent writes. ctx is consumer-driven (caller cancels = stream ends). Output goes to caller via iter.Seq channel pattern.

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — anthropic.go does not generate business IDs. Tool-call IDs come from upstream LLM (preserved via tc.ID round-trip).

A.4 §S16 错误 wrap 格式:
  - violations: site #11 (toAnthropicMsgs bare-return), #12 (toAnthropicMsgs inner bare-return), #13 (unexpected role no-sentinel — same panic-vs-sentinel question as established pattern).
  - all LOW; sentinel chain functionally preserved through outer wrap at site #1.

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels. All errors are wrapped fmt.Errorf flowing through StreamEvent.Err to caller; caller eventually maps to chatdomain.ErrProviderUnavailable (errmap.go:61).
