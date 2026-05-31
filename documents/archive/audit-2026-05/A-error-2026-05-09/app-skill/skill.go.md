# audit: backend/internal/app/skill/skill.go

LOC: 168
Read: full file (lines 1-168)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | skill.go:112-114 | `if log == nil { panic("skill.New: logger is nil") }` | A.1 | OK | wiring-time invariant; same pattern as apikey.NewService / mcp.New / sandbox panics — caught at boot, never runtime | N-A | — | — | — |
| 2 | skill.go:115-117 | `if notif == nil { notif = notificationspkg.New(nil, log) }` (defensive default to no-op publisher) | A.1 | OK | nil → no-op; intentional graceful fallback to suppress publishes when no events bridge wired (e.g. unit tests). Doc comment in notifications package confirms `New(nil, log)` returns noopPublisher. | N-A | — | — | — |
| 3 | skill.go:143-152 | `Get: ...; if !ok { return nil, skilldomain.ErrSkillNotFound }; cp := *sk; return &cp, nil` | A.4 | OK | direct sentinel return at innermost layer (§S16 spec: "sentinel 在最里层"); registered errmap.go:149 → 404 SKILL_NOT_FOUND. Returns copy not pointer to internal map entry → safe concurrent read. | N-A | — | — | — |
| 4 | skill.go:157-167 | `List: take RLock; copy each *Skill; sort by Name; return` | A.1/A.4 | OK | no error path; copies internal pointers to defend against caller mutation. RLock is held for full iteration — ~1-50 skills, microsecond cost. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none in this file (Get/List are read-side; mutations live in mutate.go)
  - 各自 ctx 来源: N/A
  - violations: N/A — file is constructor + read-side accessors only

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — skill names are user-supplied (frontmatter.name or dir basename), not idgen-generated business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (only error return is direct sentinel at site #3)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none (sentinels live in domain/skill/skill.go)
  - 已登记 errmap (consumed): `skilldomain.ErrSkillNotFound` → errmap.go:149 ✓
  - missing: N/A — file defines no sentinels
