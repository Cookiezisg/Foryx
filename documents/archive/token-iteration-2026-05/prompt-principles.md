# Prompt Principles (Forgify V1.2 §18)

> All LLM-facing prompt strings in this codebase obey these 6 principles.
> `make lint-prompts` enforces a subset; the rest are review-time checks.
> §18 Prompt Governance puts inventory + preview behind dev endpoints.

---

## The 6 principles

### 1. Examples beat rules

LLMs imitate examples far better than they derive behaviour from abstract rules. One concrete example is worth 5 paragraphs of "be careful to..." prose. The `multiAgentForgingPromptSection` (`backend/internal/app/chat/multi_agent_prompt.go`) is the gold reference — every step is a concrete action sequence the model can mimic, not an aspiration.

**Apply when**: writing tool descriptions, system prompts, agent identity blocks.

---

### 2. "What NOT" beats "What"

LLMs default to "I can use it, I use it." If a tool has a footgun (e.g. `delete_document` is destructive, `eval_expression` is non-deterministic), spell out **when not to use** explicitly. One line — "Don't use for X (use Y instead)" — prevents 90% of misuse.

**Apply when**: every tool with a more-specific sibling. Most tool descriptions in `app/tool/*/` already include this hint; new tools must.

---

### 3. Static-first, dynamic-last (cache-friendliness)

Anthropic prompt cache has a 5-minute TTL on the **prefix** of the system prompt. Sections that never change between turns (multi-agent forging teaching, base identity, catalog summary) belong at the **front** to maximize cache hits → 90% savings. Sections that change per-request (current time, per-conv memory, attached docs) belong at the **back**.

The `chat.SystemPromptSections` ordering encodes this: `base` → `multi_agent_forging` → `catalog` → `memory` → `documents` → `user_systemPrompt` → `locale_hint`.

**Apply when**: composing multi-section prompts, especially `buildSystemPrompt`-style assemblers.

---

### 4. No first-person voice

"I will create..." vs "Create..." — the imperative is shorter, perspective-neutral, and doesn't confuse the model about who's speaking. First-person ("I'll", "I am", "I need to") in tool descriptions causes the LLM to roleplay the tool itself rather than just call it.

**Lint enforced**: `cmd/lintprompts` flags `i will / i'll / i am / i need to` in any prompt constant.

---

### 5. No weasel words

"Be careful to..." / "Try to..." / "When in doubt..." — these directives are unintelligible to LLMs. The model doesn't have introspection for "am I in doubt?" Replace every weasel phrase with a **concrete trigger + action**:

- ❌ "When in doubt, ask the user."
- ✅ "If `arg.scope` is ambiguous, call `AskUserQuestion` with options=[file, directory, project]."

**Lint enforced**: `cmd/lintprompts` flags `be careful / try to / when in doubt / as much as possible`.

---

### 6. 50-800 char sweet spot

- **< 50 chars**: LLM has to guess intent. Common in early-stage tool descs ("Searches stuff."). Expand.
- **50-300 chars**: ideal for tool descriptions.
- **300-800 chars**: ideal for system prompt sections with examples.
- **> 800 chars**: attention dilutes; LLM skims. Split into multiple sections, or move bulk content to fetched-on-demand resources.

**Lint enforced**: `cmd/lintprompts` flags both bounds.

---

## Anti-patterns we've audited out

| Pattern | Example | Why bad | Fix |
|---|---|---|---|
| Emoji in tool descs | `📝 Edit a file` | Eats tokens, no LLM value | Drop |
| Generic role intros | "You are a helpful AI..." | Adds 0 information | Drop or replace with specific identity ("You are Forgify, an agent that...") |
| Apologetic deferral | "I will try my best to..." | First-person + weasel | Imperative ("Forge the requested entity") |
| Implementation noise | "Internally we use Phase 3 Tool framework v2..." | LLM doesn't care about your architecture | Drop |
| Pre-flight checklists in description | "Before calling: ① check X, ② check Y, ③..." | Tool description ≠ recipe; that belongs in the call instructions in conversation history | Move to caller's prompt |
| Self-reference | "This tool searches..." → "This tool" wastes 2 tokens | Just say "Search..." | Drop "This tool" |

---

## Section markers (XML-style)

`chat.AssemblePromptSections` wraps each segment in `<section name="...">` so:

1. **The model** can refer to a section by name when reasoning ("Per the `multi_agent_forging` section...")
2. **The testend preview** (`/conversations/{id}/system-prompt-preview`) can render section boundaries
3. **Future prompt-guided debugging** (e.g. "ignore everything but the `memory` section") becomes possible

The marker overhead is ~30 chars per section — worth it.

---

## Inventory + preview endpoints (§18 governance)

- `GET /api/v1/dev/prompts` (dev only) — every LLM-facing prompt in one place
- `GET /api/v1/conversations/{id}/system-prompt-preview` — per-conv assembled prompt with section breakdown

testend mirrors:
- `/dev/prompts` — searchable inventory
- chat header **📋 prompt** button — preview modal

---

## When you write a new prompt

1. Pull up `/dev/prompts` in testend; check if a similar one exists you can extend.
2. Write it short — aim for 50-300 chars on first pass.
3. Run `make lint-prompts` — fix any violations.
4. If you added a non-tool prompt (new const), export a public getter and register it in `handlers/prompts.go::PromptsHandler.List` so it shows up in the inventory.
5. Commit with the prompt change + lint pass + inventory wiring as one atom.
