# Package audit summary: internal/infra/llm

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. `_ = err` requires inline justification. `defer X.Close()` on read-only resources is fine. `if ctx.Err() != nil { return }` after Stream cancel is correct §S9-aware exit (not silent fallback). Documented soft-fail with audit log (e.g. anthropic.go:#14 malformed history JSON → fallback `{}` + slog.Warn) is the canonical compliant pattern.
- **§S9 detached ctx 终态写**: applies to terminal-state DB writes that must persist regardless of caller cancel. **N/A in this package** — infra/llm is a stateless wire transport (HTTP client + SSE parser + protocol invariant sanitizer + in-memory trace recorder). No DB writes; ctx is consumer-driven (caller cancels = stream ends). Trace recorder writes to in-memory ring (--dev observability, by design loss-on-exit).
- **§S15 ID 生成**: package generates NO business IDs. Tool-call IDs (LLMToolCall.ID) come from upstream LLM and are preserved via round-trip — not Forgify-issued. §S15 §N/A.
- **§S16 错误 wrap 格式**: this is the **dominant finding cluster**. infra/llm uses `llm/openai:` / `llm/anthropic:` slash-separated prefixes throughout instead of canonical `infra/llm.<Type>.<Method>:`. Sentinel chain via `%w` is consistent in most places. The biggest concrete impact is **classifyHTTPError + in-stream Error chunk handling** (openai.go:#8, #12, #18) — these produce sentinel-less errors that prevent `errors.Is(err, apikeydomain.ErrInvalid)` from working in chat runner, which is the upstream cause of why apikey.MarkInvalid (recently §S9-fixed) rarely actually gets called.
- **§S17 errmap 单一事实源**: infra/llm declares zero sentinels currently. The classifyHTTPError set should introduce 4-5 new sentinels (auth/rate-limit/bad-request/model-not-found/generic-provider) — this is the single most consequential improvement for end-to-end error classification.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| adapter.go | 300 | 3 | 3 | 0 | 0 | 0 |
| anthropic.go | 466 | 15 | 13 | 0 | 0 | 2 |
| factory.go | 140 | 3 | 1 | 0 | 0 | 2 |
| llm.go | 216 | 1 | 0 | 0 | 0 | 1 |
| mock.go | 192 | 3 | 2 | 0 | 0 | 1 |
| openai.go | 637 | 18 | 4 | 0 | 3 | 11 |
| sanitizer.go | 114 | 3 | 3 | 0 | 0 | 0 |
| trace.go | 202 | 3 | 3 | 0 | 0 | 0 |
| **TOTAL** | **2267** | **49** | **29** | **0** | **3** | **17** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 3 | openai.go:#8, #12 (provider error `%s` not `%w`, no sentinel — affects in-stream + non-stream), openai.go:#18 (classifyHTTPError set — 5 cases all sentinel-less, blocks apikey.MarkInvalid path) | FOUND |
| LOW (§S16 prefix style) | 11 | adapter ×0; anthropic ×2 (#11, #12 bare-return); factory ×2 (#1, #3 incl no-sentinel); llm ×1 (#1 Generate); mock ×1 (#1); openai ×4 (#1, #2, #6, #7, #10 prefix); openai #13/#14 (bare-return); openai #16a (panic-or-sentinel wiring) | FOUND |
| LOW (§S3 silent without comment) | 1 | openai.go:#17 (jsonString silent json.Marshal — basic-type unfailable but missing inline comment) | FOUND |
| LOW (defensive-validation no sentinel) | 3 | factory.go:#3 (resolveBaseURL — no default for custom provider); openai.go:#15 (toOpenAIMsg unknown role — same pattern as apikey/mcp/sandbox panic decision); anthropic.go:#13 (toAnthropicMsg unexpected role — same pattern); openai #16a similar | FOUND |
| LOW (other) | 2 | openai.go:#9 (no choices — empty response), openai.go:#5b (cluster cross-ref to #18) | FOUND |

## Cross-cutting

### Sentinel chain integrity (§S17) — most important finding

**infra/llm declares zero sentinels.** All errors are produced via `fmt.Errorf("...", err)` chains that preserve %w when present, but lack discriminative sentinels.

**Critical gap**: HTTP 401/429/400/404 paths in classifyHTTPError (openai.go:486-503) produce errors with NO sentinel chain. This means:

1. chat runner's `errors.Is(err, apikeydomain.ErrInvalid)` check (line 6 of chat/runner.go: `case errors.Is(err, llmclientpkg.ErrPickModel): code = "MODEL_NOT_CONFIGURED"; case errors.Is(err, llmclientpkg.ErrResolveCreds): code = "API_KEY_PROVIDER_NOT_FOUND"`) handles only the two pre-LLM-call errors but never the post-call 401/429/etc.
2. **apikey.MarkInvalid (§S9 fixed in commit e36f890) rarely fires in practice** because the 401-detection signal never reaches it — the chain is broken at this layer.

This is the same defect class as the resolved-via-`%w: %w` issue in mcp install.go:#5 (commit 505d6e3), but here at the LLM transport layer where it matters most for credential lifecycle.

**Recommendation**: introduce sentinels in a new `llmdomain` package (or directly in this file as `var Err...`):
- `llm.ErrAuthFailed` (or reuse `apikeydomain.ErrInvalid` directly so MarkInvalid path lights up)
- `llm.ErrRateLimited`
- `llm.ErrBadRequest`
- `llm.ErrModelNotFound`
- `llm.ErrProviderError` (catch-all)

Then wrap classifyHTTPError + in-stream chunk.Error paths with `%w`, register all in errmap.go.

### 3-client error-handling style comparison (adapter / openai / anthropic / mock)

| Aspect | adapter.go | openai.go | anthropic.go | mock.go |
|---|---|---|---|---|
| Error vehicle | StreamEvent.Err (channel pattern, consistent) | StreamEvent.Err | StreamEvent.Err | StreamEvent.Err |
| `%w` usage | N/A (no errors) | yes throughout | yes throughout | N/A (errors.New for queue-empty) |
| Pkg.Method prefix | N/A | `llm/openai:` slash-style | `llm/anthropic:` slash-style | `mock-llm:` (descriptive) |
| Canonical `<pkg>.<Type>.<Method>:` form | N/A | NO (consistent slash-prefix style) | NO (same) | NO (descriptive prefix) |
| HTTP status classification | N/A | classifyHTTPError (no sentinels) | classifyHTTPError (shared, no sentinels) | N/A |
| In-stream upstream error detection | N/A | TE-23 OpenRouter chunk.Error | (does not surface in-stream provider errors as separate path) | N/A |
| Documented soft-fail with audit log | N/A | classifyHTTPError body-read graceful-degrade | tool-call args malformed → `{}` + slog.Warn (model §S3 example) | N/A |

**Style consistency**: openai.go and anthropic.go are remarkably consistent with each other — same prefix scheme, same wrap-everywhere discipline, same ctx.Err() exit paths, both use the shared classifyHTTPError. The factory + sanitizer + adapter + llm + mock + trace files are stylistically aligned too. **The package's internal style is consistent**; the issue is that the consistent style deviates from the §S16 spec literal.

### `infra/llm.<Type>.<Method>:` migration impact

If we standardize on §S16 spec literal (e.g. `llm.openAIClient.Stream:`), this is a global rename across ~25 fmt.Errorf call sites. Low risk (no behavior change), bounded grep effort. Could be a single sweep commit. Alternatively: WAIVE since project-internal consistency is high and `llm/openai:` is unambiguous.

### Anthropic file is exemplar; OpenAI file has more noise

anthropic.go has 13/15 OK + 2 LOW (both bare-return style), no MED.
openai.go has 4/18 OK + 3 MED + 11 LOW. The MED concentrate around classifyHTTPError + in-stream Error handling.

This asymmetry isn't because openai.go is sloppy — it's because OpenAI-compat covers more failure modes (OpenRouter mid-stream errors / DeepSeek reasoning fallback / Ollama tool-call quirks / non-streaming dual-shape) and has more places where sentinels would help.

### Recommended fix priorities

1. **openai.go:#18 + #8 + #12 (MED)** — introduce HTTP-class sentinels + propagate to errmap. **Highest impact**: unblocks apikey.MarkInvalid lifecycle, lets chat/loop discriminate retry-worthy from terminal errors. Single commit ~30 lines.
2. **openai.go:#15 + anthropic.go:#13 (LOW defensive-validation)** — apply the panic-vs-sentinel decision consistently with apikey/mcp/sandbox audits. Quick.
3. **§S16 prefix sweep (LOW × 11)** — global rename `llm/openai:` → `llm.openAIClient.<Method>:` (and similar for anthropic). Or WAIVE if project-internal consistency is acceptable.
4. **factory.go:#3 (LOW)** — resolveBaseURL "no default" path needs a sentinel for the custom-provider error case.
5. **openai.go:#17 + #16b (LOW)** — Marshal-discard polish comments (matches loop fork's resolved approach).
6. **anthropic.go:#11 + #12 (LOW)** — bare-return style cleanup.
7. **factory.go:#1, llm.go:#1, mock.go:#1 (LOW)** — bare-return / dev-only error style; consider WAIVE for mock dev-only path.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

Random seed: 7 sites picked from `OK`/`POST-FIX OK` set across 5 files:

1. **anthropic.go:#3** (ctx.Err() pre-check before yielding error): verified — `if ctx.Err() != nil { return }` is the canonical §S9-aware Stream cancel detection. Caller already knows about cancel via ctx; yielding a duplicate StreamEvent would be noise. Compliance literal.
2. **anthropic.go:#14** (malformed tool-call JSON → `{}` fallback + slog.Warn): verified — file's docstring at lines 322-329 explicitly documents the 400-trap recovery rationale. slog.Warn carries tool_call_id + tool_name + raw + err. This is the model §S3-compliant best-effort pattern; future audits should cite it as exemplar.
3. **adapter.go:#2** (lookupAdapter unknown provider → openaiAdapter fallback): verified — documented at lines 257-263 as explicit design choice ("user typos / new untested providers stay functional"). Returns concrete value not error; downstream wire client surfaces real provider mismatch as HTTP error. §S3 doesn't apply.
4. **sanitizer.go:#1** (drop stray RoleTool message): verified — file header lines 1-25 + inline comment 64-68 thoroughly document why silent drop is the recovery path (no anchor to repair against). The whole sanitizer's purpose is precisely this — broken-history recovery before it 400s the conversation. §S3 documented-intent carve-out applies.
5. **openai.go:#11** (scanner.Err with ctx.Err()==nil guard): verified — surfaces real scan errors but skips ctx-cancelled reads (since cancel-induced read failures are not the cause we want to report). §S3 + §S9 aware. Wrap with %w. Compliance literal.
6. **trace.go:#3** (Recorder writes to in-memory map): verified — comment at lines 8-10 explicitly carves out "loss on process exit is by design (--dev observability, not persistent audit)". Not a §S9 terminal write; in-memory ring loses on restart by design.
7. **mock.go:#3** (ErrAfter propagated as EventError): verified — file's API contract at line 49 promises this exact behavior; tester-supplied error flows unchanged through StreamEvent. Not §S3 silent — matches tester's explicit script-time choice.

All 7 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping. Audit's primary findings (the sentinel-less classifyHTTPError cluster + provider in-stream error chunks) survive spot-check pressure: the model-correct OK sites (anthropic.go #3 / #14, openai.go #11) prove the §S3+§S9 pattern is achievable in this package; the deviations at openai.go #8/#12/#18 are real gaps in **sentinel taxonomy** rather than style noise.

## Out-of-scope notes (parent should verify)

1. **apikey.MarkInvalid lifecycle** depends on this layer producing apikeydomain.ErrInvalid-compatible errors. Cross-fork concern: the §S9 fix to MarkInvalid (commit e36f890) made the function safe to call on cancel races, but the function may rarely actually be called because the LLM-layer 401 detection chain is broken. Both fixes together = complete repair.

2. **chat/runner.go #6 manual code switch** (LOW WAIVED in chat audit) is partially redundant with what classifyHTTPError + sentinel registration would enable. If we add llm.ErrAuthFailed sentinels, the runner switch shrinks. May want to revisit chat audit's WAIVE decision after this fix lands.

3. **infra/llm has no service-design-document** (file paths grep'd; 4-step required-reads list noted infra/llm has no specific doc). Suggests Phase B (§S14 doc-sync audit) should consider whether to create one or fold into chat.md.

4. **No sentinel registration in errmap.go from this package** — not strictly a §S17 violation today (since no sentinels exist) but the recommended fix #1 above would introduce 4-5 new errmap rows. Should batch with the registration.
