<!-- surface: examples (gold example + architecture rules) | 被优化面 | 复杂建提分(few-shot ~+11pt / 架构守则 ~+10pt) -->
[architecture decisions] (when building a workflow, choose by these)
- agent vs function: needs reasoning/judgment/extraction → agent; deterministic compute → function.
- polling vs cron: data arrives continuously / external source you must watch → polling function;
  fixed schedule → cron trigger (then fetch in the first node).
- case is a dealer, not an analyst: it only routes on fields ALREADY computed upstream; never put
  compute/LLM/HTTP in a guard.
- every path has an action; every case has a final `when:"true"` default; no dangling branch.
- multi-field gate: combine with `&&` in one `when:` guard.

[gold example] a correct workflow (cron → fetch → classify → route with when: + retry + approval)
```yaml
nodes:
  - {id: t, type: trigger, config: {kind: cron, cron: "0 9 * * *"}}
  - {id: fetch, type: tool, config: {ref: fn_fetch_unread, args: {}}}
  - {id: clf, type: agent, config: {ref: ag_email_classifier}}     # outputSchema=enum
  - {id: route, type: case, config: {branches:
        invoice: {when: "payload.category == 'invoice'", to: pay}
        normal:  {when: "true", to: appr}}}                         # default
  - {id: pay, type: tool, config: {ref: fn_process_invoice, args: {}}}
  - {id: appr, type: approval, config: {prompt: "Review this email", branches: {...}}}
edges: [[t,fetch],[fetch,clf],[clf,route]]
```
