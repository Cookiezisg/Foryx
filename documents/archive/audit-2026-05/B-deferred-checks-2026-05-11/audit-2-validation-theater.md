# Phase B audit-2 — Validation theater

Scope: HTTP handlers (`internal/transport/httpapi/handlers/`, excluding `_test.go`, `dev*.go` dev-only files, and `forge.go` per skip rule) + app-layer service preflight (`internal/app/*/`, excluding `app/tool/*` LLM-facing tools and `app/forge/`) + domain invariant checks (`internal/domain/*/`).

Method: for each non-trivial validation site, check whether the **frontend / testend / Wails UI** can prevent the bad input AND whether the **downstream service / DB** will naturally fail. Theatre = both yes. Forgify is single-dev local Wails so curl probes from testend exist but no untrusted public traffic.

---

## Per-finding

### [1] AnswerHandler.Submit toolCallId empty check
- **Location**: `backend/internal/transport/httpapi/handlers/answers.go:77-81`
- **Validation code**:
  ```go
  if req.ToolCallID == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
          "toolCallId is required", nil)
      return
  }
  ```
- **Frontend coverage**: yes — UI never displays an Answer form without a known waiting `toolCallId` (the AskUserQuestion tool wires it in via the message stream).
- **Downstream natural error**: yes — `askapp.Service.Resolve("", "")` looks up `s.pending[""]`, miss → returns `askapp.ErrNoPendingQuestion` → errmap → 404 `ASK_NO_PENDING_QUESTION` (`response/errmap.go:179`).
- **Verdict**: EDGE (lean THEATER). The 400 vs 404 wire-code diff is the only daylight — "field missing" is arguably clearer than "no pending question", but downstream is functional and same UX class.
- **If THEATER, recommended action**: delete 5 lines (line 77-81). Downstream `askapp.ErrNoPendingQuestion` lands as `ASK_NO_PENDING_QUESTION` 404 via the existing errmap entry.

### [2] SkillsHandler.Create name empty check
- **Location**: `backend/internal/transport/httpapi/handlers/skills.go:160-164`
- **Validation code**:
  ```go
  if strings.TrimSpace(req.Name) == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
          "name is required", nil)
      return
  }
  ```
- **Frontend coverage**: yes — skill creation form requires a name input.
- **Downstream natural error**: yes — `skillapp.Create` → `validateName("")` → `nameRegexp.MatchString("")` returns false (regex `^[a-z0-9][a-z0-9-]{0,63}$` requires ≥ 1 char) → returns `skilldomain.ErrInvalidName` → errmap → 422 `SKILL_INVALID_NAME` (`response/errmap.go:175`).
- **Verdict**: THEATER (mild — 400 vs 422 is the only daylight; same "your name is broken" UX class). Note: trims whitespace before checking, while `nameRegexp` would reject `" "` outright too — so semantically identical.
- **If THEATER, recommended action**: delete 5 lines (line 160-164). Downstream `skilldomain.ErrInvalidName` lands as `SKILL_INVALID_NAME` 422 via existing errmap entry.

### [3] SandboxHandler.installRuntime KIND_REQUIRED check
- **Location**: `backend/internal/transport/httpapi/handlers/sandbox.go:286-290`
- **Validation code**:
  ```go
  if req.Kind == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "KIND_REQUIRED",
          "kind is required", nil)
      return
  }
  ```
- **Frontend coverage**: yes — installRuntime is only invoked from testend/UI debug panels that pick from a known runtime kind list (python/node/binary/...).
- **Downstream natural error**: yes — `sandboxapp.EnsureRuntime(RuntimeSpec{Kind: ""})` → `s.installers[""]` miss → `ErrRuntimeNotSupported` → errmap → 422 `SANDBOX_RUNTIME_NOT_SUPPORTED` (`response/errmap.go:100`).
- **Verdict**: THEATER. The handler invents an unmapped wire-code `KIND_REQUIRED` (not in `errTable`) where downstream would give the perfectly serviceable `SANDBOX_RUNTIME_NOT_SUPPORTED` 422. The custom code also fragments the error vocabulary (clients now have to handle two codes for the same condition).
- **If THEATER, recommended action**: delete 5 lines (line 286-290). Downstream `sandboxdomain.ErrRuntimeNotSupported` lands as `SANDBOX_RUNTIME_NOT_SUPPORTED` 422 via existing errmap entry.

### [4] SandboxHandler.ListEnvs OWNER_KIND_REQUIRED check
- **Location**: `backend/internal/transport/httpapi/handlers/sandbox.go:103-109`
- **Validation code**:
  ```go
  ownerKind := r.URL.Query().Get("ownerKind")
  if ownerKind == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "OWNER_KIND_REQUIRED",
          "ownerKind query parameter is required", nil)
      return
  }
  ```
- **Frontend coverage**: yes — testend/UI debug pages always pass ownerKind (one of 4 enum values).
- **Downstream natural error**: no — `ListEnvsByOwnerKind("")` issues `WHERE owner_kind = ""` which legitimately returns empty list (no error). Without the handler check, a curl probe with no param would get `{"data": []}` instead of a 400. Different UX class than usual theatre because there's no sentinel that fires.
- **Verdict**: NECESSARY. Without it, callers silently see "no envs" with no hint that they failed to scope. The handler check forces explicit scoping. The downstream returning empty list ≠ an error.
- Note: the wire code `OWNER_KIND_REQUIRED` is not in errmap; consider folding it into the standard `INVALID_REQUEST` vocabulary for consistency with other "missing query param" errors (e.g. eventlog.go uses `INVALID_REQUEST` for the same shape).

### [5] EventLogHandler.Stream conversationId empty check
- **Location**: `backend/internal/transport/httpapi/handlers/eventlog.go:87-91`
- **Validation code**:
  ```go
  conversationID := r.URL.Query().Get("conversationId")
  if conversationID == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "conversationId is required", nil)
      return
  }
  ```
- **Frontend coverage**: yes — frontend always passes the conv id when opening the SSE stream.
- **Downstream natural error**: partial — `bridge.Subscribe("", 0)` returns `fmt.Errorf("%w: empty conversationID", eventlogdomain.ErrInvalidEvent)`, but `eventlogdomain.ErrInvalidEvent` is **NOT registered in errmap** → would land as 500 INTERNAL_ERROR + "unmapped domain error" warning (`response/errmap.go:245-249`). So downstream naturally errors but ugly.
- **Verdict**: NECESSARY — without the early 400, this becomes a 500 + ERROR-level log noise. Alternative: register `eventlogdomain.ErrInvalidEvent` in errmap and delete the early check. Either path closes the gap; current state with early check is functionally fine.

### [6] EventLogHandler.History conversationId empty check
- **Location**: `backend/internal/transport/httpapi/handlers/eventlog.go:148-152`
- **Validation code**:
  ```go
  conversationID := r.PathValue("id")
  if conversationID == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "conversationId is required", nil)
      return
  }
  ```
- **Frontend coverage**: n/a — `r.PathValue("id")` cannot be empty because Go 1.22 mux pattern `/api/v1/conversations/{id}/eventlog` requires a non-empty segment for `{id}` (would not match if missing).
- **Downstream natural error**: n/a — code is unreachable. `r.PathValue` returns empty only when the route doesn't match the pattern, in which case this handler wouldn't be invoked.
- **Verdict**: THEATER (dead code variant). The check is unreachable.
- **If THEATER, recommended action**: delete 5 lines (line 148-152). No downstream sentinel needed since branch is dead.

### [7] MCPHandler.PutServer command empty check
- **Location**: `backend/internal/transport/httpapi/handlers/mcp.go:172-176`
- **Validation code**:
  ```go
  if strings.TrimSpace(body.Command) == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "MCP_COMMAND_REQUIRED",
          "command field is required", nil)
      return
  }
  ```
- **Frontend coverage**: yes — UI form for MCP server config has a required command input.
- **Downstream natural error**: partial — `mcpapp.AddServer` writes `mcp.json` then calls `connectOne` → `exec.Command("", ...)` from `infra/mcp/client.go:137`. `cmd.Start()` would return an error like `"exec: no command"`, which gets wrapped as `mcpapp.AddServer: connect: ...` → no sentinel match → 500 INTERNAL_ERROR + "unmapped domain error" warning.
- **Verdict**: NECESSARY. Without the handler check, this lands as 500 + noisy log. The 400 + clear "command field is required" wire is the right UX. Note: wire code `MCP_COMMAND_REQUIRED` is also not in errmap (literal Error call), but this is consistent with other ad-hoc "missing field" codes in this codebase — separate question.

### [8] MCPHandler.serverNameAction empty name check
- **Location**: `backend/internal/transport/httpapi/handlers/mcp.go:236-242`
- **Validation code**:
  ```go
  name, action := splitAction(r.PathValue("nameAction"))
  if name == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
          "missing server name in path", nil)
      return
  }
  ```
- **Frontend coverage**: n/a — `splitAction` returns empty name only when the path tail has no colon (`splitAction` returns `("", input)` for no-colon — see `sandbox.go:358-363`). The mux pattern `{nameAction}` requires non-empty match, but the **post-split** name can be empty if path is `:reconnect` (literally just colon-prefixed action). This is a malformed-path defense, not user-input defense.
- **Downstream natural error**: yes if `mcpapp.Reconnect("")` or `HealthCheck("")` — both look up `s.clients[""]` / `s.states[""]` → return `ErrServerNotFound` → errmap → 404 `MCP_SERVER_NOT_FOUND`.
- **Verdict**: EDGE. Theoretically theatre, but defends against a malformed URL `/api/v1/mcp-servers/:reconnect` which won't reach the same downstream sentinel cleanly (returns 404 instead of 400). The early 400 is more accurate. Keep.

### [9] MCPHandler.registryNameAction empty name check
- **Location**: `backend/internal/transport/httpapi/handlers/mcp.go:415-421`
- **Validation code**: same pattern as [8] — `if name == ""` from splitAction.
- **Frontend coverage**: n/a — same dead-branch logic as [8].
- **Downstream natural error**: `InstallFromRegistry("", ...)` → `GetRegistryEntry("")` → `ErrRegistryEntryNotFound` → 404 `MCP_REGISTRY_ENTRY_NOT_FOUND`.
- **Verdict**: EDGE. Same as [8] — defends a malformed URL where path is `/api/v1/mcp-registry/:install`. Keep.

### [10] SkillsHandler.NameAction empty name/action check
- **Location**: `backend/internal/transport/httpapi/handlers/skills.go:347-351`
- **Validation code**:
  ```go
  name, action := splitAction(r.PathValue("nameAction"))
  if name == "" || action == "" {
      responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
          "path must be {name}:{action}", nil)
      return
  }
  ```
- **Frontend coverage**: n/a — same splitAction post-split semantics as [8] / [9].
- **Downstream natural error**: yes — `Activate("", args)` would lookup in `s.skills[""]` → `ErrSkillNotFound` → 404; but action="" would fall through to the `default` case below and return 400 INVALID_REQUEST "unknown action:" anyway.
- **Verdict**: EDGE-lean-THEATER. The check is partially redundant given the default-case fallback for unknown action. Keep it together with [8]/[9] for consistency or delete all three together.

### [11] apikey.Service Create provider whitelist check
- **Location**: `backend/internal/app/apikey/apikey.go:118-121` (via `validateCreate`)
- **Validation code**:
  ```go
  if !isValidProvider(in.Provider) {
      return fmt.Errorf("apikey.validateCreate: provider %q: %w", in.Provider, apikeydomain.ErrInvalidProvider)
  }
  ```
- **Frontend coverage**: yes — providers come from a dropdown driven by `GET /api/v1/providers`.
- **Downstream natural error**: **NO**. DB has no CHECK constraint on provider column. `repo.Save` would persist a garbage `Provider` value with no rejection. The error would surface only when the user later tries to test or use the key (tester.go line 76 returns `ErrInvalidProvider`), and the bad row stays in the DB.
- **Verdict**: NECESSARY. Prevents persisted garbage. Keep.

### [12] apikey.Service Create key non-empty check
- **Location**: `backend/internal/app/apikey/apikey.go:122-124`
- **Validation code**: `if strings.TrimSpace(in.Key) == "" { return apikeydomain.ErrKeyRequired }`
- **Frontend coverage**: yes — UI form requires key input.
- **Downstream natural error**: **NO**. `encryptor.Encrypt([]byte(""))` produces a non-empty ciphertext for empty plaintext (AES-GCM authenticates an empty payload). KeyEncrypted column is NOT NULL but ciphertext-of-empty is still non-null, so DB doesn't reject. Row persists as a "valid" but useless credential.
- **Verdict**: NECESSARY. Keep — prevents storing useless empty creds.

### [13] apikey.Service Create baseURL conditional check
- **Location**: `backend/internal/app/apikey/apikey.go:125-128`
- **Validation code**: `if meta.BaseURLRequired && strings.TrimSpace(in.BaseURL) == "" { return apikeydomain.ErrBaseURLRequired }`
- **Frontend coverage**: yes — UI conditionally requires baseURL for ollama/custom.
- **Downstream natural error**: partial — for `meta.BaseURLRequired=true` providers, tester.go:79-84 returns `ErrBaseURLRequired` at probe time. But Create itself wouldn't fail — bad row would persist and only error at the first Test.
- **Verdict**: NECESSARY. Same reason as [11] — prevents garbage row.

### [14] apikey.Service Create custom-provider apiFormat check
- **Location**: `backend/internal/app/apikey/apikey.go:129-131`
- **Validation code**: `if in.Provider == "custom" && strings.TrimSpace(in.APIFormat) == "" { return apikeydomain.ErrAPIFormatRequired }`
- **Frontend coverage**: yes — UI form for "custom" provider shows apiFormat field with validation.
- **Downstream natural error**: no — same garbage-row pattern as [13].
- **Verdict**: NECESSARY. Keep.

### [15] model.Service Upsert scenario whitelist check
- **Location**: `backend/internal/app/model/model.go:69-71`
- **Validation code**: `if !modeldomain.IsValidScenario(scenario) { return nil, modeldomain.ErrInvalidScenario }`
- **Frontend coverage**: yes — testend/UI dropdown limits to known scenarios.
- **Downstream natural error**: **NO**. DB has no CHECK constraint on `scenario` (intentional — `domain/model/model.go:43-47` comment: "App-layer validation, not DB CHECK, so adding new scenarios needs no migration"). `repo.Upsert` would persist garbage. Worse: the partial UNIQUE index on (user_id, scenario) means the garbage entry would prevent the user from configuring a real scenario later.
- **Verdict**: NECESSARY. Keep — explicitly documented as the right place for this check.

### [16] model.Service Upsert provider/modelID required checks
- **Location**: `backend/internal/app/model/model.go:72-77`
- **Validation code**:
  ```go
  if strings.TrimSpace(in.Provider) == "" { return nil, modeldomain.ErrProviderRequired }
  if strings.TrimSpace(in.ModelID) == "" { return nil, modeldomain.ErrModelIDRequired }
  ```
- **Frontend coverage**: yes — UI form requires both.
- **Downstream natural error**: partial — DB columns are `not null` but empty string is not null, so DB wouldn't reject. Downstream `PickForChat` returning empty `provider, modelID` would then cause chat to fail with a less clear error.
- **Verdict**: NECESSARY. Empty-string + NOT-NULL is a common Go/DB trap; explicit app-layer check gives a clean error.

### [17] todo.Service Create subject required check
- **Location**: `backend/internal/app/todo/todo.go:92-94`
- **Validation code**:
  ```go
  subject := strings.TrimSpace(in.Subject)
  if subject == "" { return nil, tododomain.ErrSubjectRequired }
  ```
- **Frontend coverage**: n/a — todo is tool-driven (LLM), not user-driven (no HTTP handler).
- **Downstream natural error**: partial — Subject column is `not null` but empty-string passes (same trap as [16]).
- **Verdict**: NECESSARY. Tool-callers (LLMs) can absolutely send empty subjects via JSON args; the clean sentinel + clear error message is the right pattern for tool feedback.

### [18] todo.Service Update status whitelist check
- **Location**: `backend/internal/app/todo/todo.go:182-186`
- **Validation code**:
  ```go
  if in.Status != nil {
      if !tododomain.IsValidStatus(*in.Status) {
          return nil, tododomain.ErrInvalidStatus
      }
  ```
- **Frontend coverage**: n/a — tool-driven.
- **Downstream natural error**: **NO**. DB has no CHECK constraint on todo.status (explicit choice — same pattern as model.scenario, see `domain/todo/todo.go:36-37`).
- **Verdict**: NECESSARY. Same reason as [15].

### [19] mcp.Service AddServer cfg.Name panic guard
- **Location**: `backend/internal/app/mcp/mcp.go:252-270`
- **Validation code**:
  ```go
  if cfg.Name == "" {
      panic("mcpapp.Service.AddServer: cfg.Name is empty — caller wiring bug; ...")
  }
  ```
- **Frontend coverage**: n/a — this is a programmer-bug guard (panic, not error). Comment explicitly says "wiring bug, not user input".
- **Downstream natural error**: n/a — every caller already sets cfg.Name from path or registry, by design.
- **Verdict**: NECESSARY. Panic-on-wiring-bug pattern, not user-input validation. Keep.

---

## Cross-cutting patterns

### Pattern 1 — Handler empty-field 400s where downstream service does the same check

Sites [1], [2], [3], [5], [7]. Common shape: handler does an early `if x == "" return 400` and downstream service has its own sentinel that would fire on the same input. The differences cluster into:
- **Wire code drift** ([1], [2]: 400 INVALID_REQUEST vs downstream 404 / 422) — UX-clarity tradeoff.
- **Unmapped sentinels** ([5], [7]): downstream sentinel exists but isn't in errmap, so removing the handler check would expose 500 + "unmapped domain error" warning. Two ways to fix: (a) keep handler check (current state, defensible); (b) register sentinel in errmap and remove handler check (cleaner, per §S17).
- **Real theatre** ([3]): handler invents an unmapped wire code where downstream gives a perfectly serviceable sentinel.

Recommendation: only [3] is clearly THEATER and clearly worth deleting (~5 lines, 0 UX regression). [1] and [2] are EDGE — small UX clarity loss if removed, but legitimate cleanup if you want to enforce "handlers don't duplicate service checks" rule.

### Pattern 2 — splitAction post-split empty-name guards

Sites [8], [9], [10]. The mux pattern `{nameAction}` guarantees the segment is non-empty, but `splitAction` can return `("", "")` for input like `:reconnect`. These guards defend against a narrow malformed-URL case (`/foo/:reconnect` — leading colon).

Recommendation: leave as is. The malformed-URL path is rare but real (e.g. testend bug, JS templating bug producing `/${name}:action` with empty name). Removing them costs little but the early 400 vs downstream 404 nuance is appropriate.

### Pattern 3 — App-layer "enum without DB CHECK" checks

Sites [11], [15], [18]. All three are `IsValidX(value)` checks at app-layer where the corresponding DB column has no CHECK constraint, intentionally (so adding new enum values doesn't require migration). These are explicitly NOT theatre — they're the only defense against garbage data.

Recommendation: leave all. Documented design pattern (see `domain/model/model.go:43-47`, `domain/todo/todo.go:36-37`).

### Pattern 4 — App-layer "empty-string + NOT-NULL" guards

Sites [12], [13], [14], [16], [17]. SQLite NOT NULL doesn't reject empty strings; without app-layer guards, garbage rows would persist with clean DB inserts but broken semantics. These are necessary.

Recommendation: leave all. This is the canonical "value is not just non-null, must be non-empty" pattern.

---

## Summary

| Classification | Count |
|---|---|
| THEATER | 3 |
| NECESSARY | 12 |
| EDGE | 4 |

Total: 19 validation sites examined.

## Top THEATER findings (易修 + 高 ROI)

1. **[3] sandbox.go:286-290 `KIND_REQUIRED`** — handler invents custom unmapped wire code where downstream gives `SANDBOX_RUNTIME_NOT_SUPPORTED` 422 (in errmap). Pure win to delete: -5 lines, clearer error vocabulary, no UX regression. Downstream sentinel: `sandboxdomain.ErrRuntimeNotSupported` → `errmap:100` → `SANDBOX_RUNTIME_NOT_SUPPORTED` 422.

2. **[6] eventlog.go:148-152 `History` empty conversationId** — dead branch. `r.PathValue("id")` can't be empty for the pattern `/api/v1/conversations/{id}/eventlog`. -5 lines, no functional change. No downstream sentinel needed because the branch is unreachable.

3. **[2] skills.go:160-164 `Create` name empty check** — duplicates the `nameRegexp` check in `validateName`. -5 lines, 0 functional change beyond wire-code (400 INVALID_REQUEST → 422 SKILL_INVALID_NAME). Downstream sentinel: `skilldomain.ErrInvalidName` → `errmap:175` → `SKILL_INVALID_NAME` 422.
