# D2 Doc-Sync Audit — model

Scope:
- Doc: `documents/version-1.2/service-design-documents/model.md`
- Code: `backend/internal/{domain,app,infra/store}/model/` + `backend/internal/transport/httpapi/handlers/model.go`

D1 already covered contract documents. Below: design-doc-vs-code drifts only.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| Service uses `RequireUserID` (returns `(string, error)` + sentinel `reqctxpkg.ErrMissingUserID`) — doc §3 + §15.2 say `reqctx.GetUserID(ctx)` | `internal/pkg/reqctx/reqctx.go:48`, `internal/app/model/model.go:78`, `internal/infra/store/model/model.go:45,66` | LOW |
| Service.Upsert applies `strings.TrimSpace` to **both Provider and ModelID** before persisting (lines 93-94). Doc §8 Upsert 流程 step 4 doesn't show TrimSpace. | `internal/app/model/model.go:93-94` | LOW |
| **Pre-flight TrimSpace check ordering**: code rejects empty Provider/ModelID *before* `RequireUserID`; doc §8 step 1-3 sequence shows scenario→provider→modelid→uid (matches code) | `internal/app/model/model.go:69-78` | OK |
| `ScenarioWebSummary = "web_summary"` is included in `ModelPicker.PickForWebSummary` interface and `ListScenarios` output. ✅ matches doc §4 + §6.3. | — | OK |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| **§5 + §11 — partial UNIQUE index `idx_mc_user_scenario WHERE deleted_at IS NULL` claimed in `schema_extras.go`**. Reality: `infra/db/schema_extras.go` has only ONE group (`forges`); **no `model_configs` entry**. Active index is the GORM-emitted full `UNIQUE(user_id, scenario)` — which **DOES include soft-deleted rows**. Effect: if a row gets soft-deleted, you cannot insert a new (same user, same scenario) row — you'd hit UNIQUE collision against the dead row. (Side note: §17 line 622 even acknowledges this as `partial UNIQUE 暂缓 — GORM 全索引在当前 Upsert 模式下等价（无 delete+recreate 路径）`. So §17 is consistent; **§5 + §11 are the stale parts.**) | model.md:147-158, 467-471 | **HIGH** |
| **§7 Store 实现 — `GetByScenario`** doc says SQL filters with `deleted_at IS NULL`. Code uses `s.db.WithContext(ctx).Where("user_id = ? AND scenario = ?", uid, scenario).First(&m)` — relies on GORM auto-soft-delete filter via `gorm.DeletedAt` field. Implicit, not explicit. ✅ behaviourally equivalent; just description-drift. | model.md:277 | LOW |
| **§7 Store 实现 — `List`** doc says `WHERE user_id=? AND deleted_at IS NULL ORDER BY scenario`. Code: `WHERE user_id=?` (relies on GORM auto-soft-delete) `Order("scenario")`. Same as above. | model.md:278 | LOW |
| **§7 Store 实现 — `Upsert`**: doc says "尝试 `WHERE user_id=? AND scenario=?` 拿现有行 → 有则更新 ID 保持 + 字段改 + `Save()`；无则 `INSERT`". Code reality: store `Upsert` is just `s.db.Save(m)` — **no GetByScenario** in the store; the get-then-decide logic is in **app layer Service.Upsert** (which the doc itself says correctly at §8 + §17). Doc §7 attributes the get-then-decide flow to the **store**, which is wrong (it's in Service). | model.md:278-280 | **MED** |
| **§7 store Upsert "或者走 GORM 的 `ON CONFLICT DO UPDATE` 语法"** — neither path is taken; code uses plain `Save()`. Doc speculation; remove. | model.md:280 | LOW |
| **§8 PickForChat 流程** lists step `1. m, err := repo.GetByScenario(ctx, ScenarioChat); err == ErrNotConfigured → 向上抛 ErrNotConfigured`. Code matches exactly. ✅ But doc §8 does **not list `PickForWebSummary` flow** — code has it at lines 127-133 (mirrors PickForChat for ScenarioWebSummary). | model.md:352-358 | LOW |
| **§13 错误码 status column** all 4 rows show `⬜` (未实现 marker). Reality: all 4 sentinels are mapped in `errmap.go:71-74`, all wired into Service.Upsert + repo.GetByScenario + repo.List paths. Should be `✅`. | model.md:486-491 | LOW |
| **§17 实现清单**: `internal/app/model/model.go — Service ... PickForChat + nil logger 守护`. Match (file exists, single-file Service). ✅ | — | OK |
| **§17 line 626 — `modelpicker.go 取消`** — accurate; merged into `model.go`. ✅ | — | OK |
| **§14 chat domain 调 LLM 时 — Phase 5 Forge 复用同一套** — reality: Forge (Phase 5) is implemented in `app/forge/` and indeed consumes ModelPicker via DI. Out of scope for this audit; flagged not. | — | — |

## Mismatched (different details)

| Item | Code | Doc | Severity |
|---|---|---|---|
| **Repository.List signature** | `List(ctx) ([]*ModelConfig, error)` (no pagination) | doc §7 same | OK |
| **§7 store implementation details** describes `GetByScenario` doing `WHERE user_id=? AND scenario=? AND deleted_at IS NULL`; code uses GORM auto-soft-delete via the model's `gorm.DeletedAt` field, no explicit clause | model.md:277-278 vs model.go:50-52, 65-78 | LOW |
| **§7 Store — describes Upsert as a get-then-update inside store** | Code: store.Upsert is one-liner `s.db.Save(m)`; the orchestration is in Service.Upsert (`app/model/model.go:82-97`). Description placed at the wrong layer. | **MED** |
| **§13 status column** | Code: all sentinels live + mapped in errmap; status should reflect | Doc: ⬜ on all 4 rows (stale flag) | LOW |
| **§5 SQL block (line 152-158) and §11 (line 467-471)** | No partial UNIQUE in schema_extras; GORM tag-only full UNIQUE active | Doc explicitly prescribes the partial UNIQUE statement | **HIGH** |
| **§10.2 PUT request — `scenario`-path validation gap** | Code: handler reads `r.PathValue("scenario")` raw, passes to Service.Upsert which checks `IsValidScenario` and returns `ErrInvalidScenario`. Path **wildcard accepts arbitrary strings**, so `PUT /api/v1/model-configs/badname` falls through to the Service check → 400 INVALID_SCENARIO. Doc §10.2 says "Path param: `scenario` ∈ `{"chat"}` (Phase 2 白名单)". Doc still claims Phase 2 whitelist = `{chat}` only; code whitelist also includes `web_summary`. | model.md:413 | **MED** |
| **§4 Scenario 白名单** rendered table shows 2 entries (`chat`, `web_summary`) — line 70-72. ✅ in sync with code. | — | OK |
| **§13 errmap entry list at the bottom (lines 495-499)** | Doc: `modeldomain.ErrNotConfigured: {http.StatusUnprocessableEntity, "MODEL_NOT_CONFIGURED"}` etc. | Code `errmap.go:71-74` matches verbatim. ✅ | OK |
| **§6.3 ModelPicker comment** (lines 213-217) | Doc: `PickForWebSummary returns ... callers (the WebFetch tool) MUST fall back to PickForChat so summarisation works out of the box`. Code interface comment (`model.go:107-112`) matches. ✅ | OK |

## Sub-check

- **Entities aligned**: **Yes** — `ModelConfig` struct fields + GORM tags match doc §5 (lines 117-128). `UserID` is `json:"-"` in both.
- **Service methods aligned**: **Yes** — 4 public methods (List / Upsert / PickForChat / PickForWebSummary) match doc §8 signatures.
- **Endpoints aligned**: **Yes** — 2 endpoints `GET /api/v1/model-configs` + `PUT /api/v1/model-configs/{scenario}` registered (`handlers/model.go:36-37`). D1 contract-doc audit covered.
- **Sentinels aligned**: **Yes** — 4 sentinels (`ErrNotConfigured / ErrInvalidScenario / ErrProviderRequired / ErrModelIDRequired`) defined in `domain/model/model.go:69-74`; all mapped in `errmap.go:71-74`. Doc §5 + §13 list same 4.
- **端到端推演 valid**: **Mostly** — §15 chains accurate at the layer-flow level. Drifts:
  - (a) `RequireUserID` not `GetUserID` (minor naming);
  - (b) §15.2 Upsert step `repo.Upsert(ctx, m)` matches actual code (Service deciding new vs existing via prior GetByScenario);
  - (c) §15.3 chat-side `PickForChat` chain matches; OK.

---

## Critical drift summary

The biggest finding is the **partial UNIQUE index that doesn't exist**: doc §5 + §11 treat it as a hard schema fact, but `schema_extras.go` has zero `model_configs` entries. Any future code path that soft-deletes a model config and then tries to insert a new one for the same `(user_id, scenario)` will collide — the doc misleads readers about both the live schema and the deletion semantics. Note that §17 line 622 itself acknowledges "partial UNIQUE 暂缓"; the inconsistency is **between §5/§11 (prescribing) and §17 (acknowledging)**. The §5/§11 sections need to be reconciled with §17 — currently the doc contradicts itself.

The §7 store-vs-service responsibility split is also misdescribed (doc puts get-then-decide logic in store; code has it in Service).

---

**Totals:** 1 HIGH / 3 MED / 6 LOW
