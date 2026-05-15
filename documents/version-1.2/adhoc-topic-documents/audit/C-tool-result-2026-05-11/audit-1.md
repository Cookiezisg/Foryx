# Phase C audit-1 — read/exec/network tools

Scope: `internal/app/tool/{filesystem,search,shell,web}/`. forge sub-package skipped per request. forge is being rewritten by user.

Method: read each `.go` file's `Description()` / `Parameters()` / `Execute()` happy + error paths. Track every LLM-facing string. Identify AP1/AP2/AP3/AP4/AP5/AP6/AP7 hits.

How validation errors reach the LLM: `loop/tools.go:191` wraps `ValidateInput` errors as `fmt.Sprintf("input validation failed: %s", err.Error())` — so any `fmt.Errorf("Read.ValidateInput: %w", ...)` etc. inside ValidateInput surfaces verbatim to the LLM with a `Read.ValidateInput:` prefix. This is **AP4 backend leak** at the framework level (every tool that wraps with `<Tool>.<Method>:` per S16 leaks the internal sentinel chain into LLM context). Documented as cross-cutting #1.

How Execute errors reach the LLM: `loop/tools.go:241` returns `output != "" → output` else `err.Error()`. Most filesystem/web/shell tools return errors as `(string, nil)` strings, so they bypass the err.Error fallback and present whatever string they composed. No additional wrapping applied.

---

## Per-tool findings

### filesystem.Read (`filesystem/read.go`)

#### Description (line 80-91)

```
Reads a file from the local filesystem.

Assume this tool is able to read most files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error message will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default, it reads up to 2000 lines starting from the beginning of the file
- When you already know which part of the file you need, only read that part using offset and limit. This can be important for larger files
- Results are returned using cat -n format (5-digit right-padded line number, tab, content), with line numbers starting at 1
- This tool can only read files, not directories. To list files in a directory, use the Glob tool with pattern "*"
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents
- Some sensitive paths (system directories, credential locations like ~/.ssh) are blocked for safety; you will receive a denial message if you try to read one
```

- Length: 12 lines, well-controlled. Imperative voice. No AP1/AP3.
- AP7 (very LOW, optional): "Assume this tool is able to read most files…" — slight hedge in user-facing prose. The "if the User provides a path to a file assume that path is valid" sentence is borrowed from CC's prompt and is half-redundant given the next line. Consider trimming. **LOW**.

#### Parameters (line 98-115)

- `file_path`: "The absolute path to the file to read" — clean.
- `offset`: "The line number to start reading from (1-based; default 1). Only provide if the file is too large to read at once." — clean, MED density of advice but not a violation.
- `limit`: same shape as offset. Clean.

No findings.

#### Execute (line 198-290)

| Line | Return string | Anti-pattern |
|---|---|---|
| 205 | `fmt.Errorf("Read.Execute: %w", err)` (Go err) | AP4 cross-cutting #1 (S16 wrapper leaks sentinel chain when returned as Go err — `loop/tools.go:246` exposes `err.Error()` to LLM)
| 220 | `pathGuard.Allow(...)` returns `reason` raw | depends on PathGuard wording; quote in cross-cutting #2 |
| 227 | `statErrorMessage(cleaned, err)` | exposes full host path in `cleaned` (e.g. `/Users/sunweilin/private/.../file`) — **AP4 LOW**
| 230 | `"Path is a directory, not a file: %s. Use Glob with pattern \"*\" to list a directory."` | full host path in result — **AP4 LOW**; also helpful follow-up suggestion (good)
| 237 | `"<system-reminder>File exists but has empty contents.</system-reminder>"` | clean. Good use of system-reminder convention.
| 242 | `statErrorMessage(cleaned, err)` | second site (open-after-stat); same AP4 LOW |
| 268 | `fmt.Sprintf("Failed to read %s: %v", cleaned, err)` | full path + raw stdlib err.Error() — **AP4 LOW**; the `%v` may include "bufio.Scanner: token too long" with internal stack-ish context |
| 274 | `"... [truncated at line %d; use offset+limit to read more]\n"` | clean.

`statErrorMessage` (line 297-306):
- `"File not found: " + path` — **AP4 LOW** (full path)
- `"Permission denied: " + path` — **AP4 LOW** (full path)
- `fmt.Sprintf("Cannot access %s: %v", path, err)` — **AP4 LOW** (full path + raw err)

**Note**: in this tool path-leak is essentially intentional — the LLM gave the path in the args and needs it in the response so it can correlate. Redacting would reduce utility. But the audit category still flags it because the cross-cutting pattern (every tool reflects the host path back) creates LLM training signal "absolute paths show up in tool results, treat as normal." **Severity: LOW** for Read individually; **MED** as a class — see cross-cutting #3.

---

### filesystem.Write (`filesystem/write.go`)

#### Description (line 58-67)

```
Writes a file to the local filesystem. Overwrites if the file exists.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- If the file already exists, you must Read it first in this conversation or the call will fail (must-Read-first guard prevents accidental clobbering)
- Prefer the Edit tool for modifying existing files — Edit only sends the diff and is far less risky
- The parent directory must already exist; this tool does NOT create directories. Use Bash mkdir -p first if needed
- The file is written atomically (staged to a tmp file, then renamed); readers never see a half-written file
- Some sensitive paths (system directories, credential locations like ~/.ssh, ~/.aws) are blocked for safety; you will receive a denial message if denied
- Do NOT create documentation files (*.md, README) or files outside the user's working scope unless explicitly requested
```

- Length: 8 lines. Imperative voice.
- "Edit only sends the diff and is far less risky" — slight LLM training nudge ("Prefer Edit"). Helpful. No violation.
- AP3 LOW (optional): "Do NOT create documentation files…" — borrowed from CC; technically LLM-facing instructions. Not a violation per se but worth noting if Forgify wants different defaults for forge/skill creation flows.

#### Parameters (line 74-87)

- `file_path` description: "The absolute path to the file to write (must be absolute)" — minor redundancy ("absolute path…must be absolute") but harmless.
- `content` description: "The content to write to the file (may be empty to create an empty file)" — clean.

#### Execute (line 173-277)

| Line | Return string | Anti-pattern |
|---|---|---|
| 184 | `pathGuard.Allow` reason | cross-cutting #2 |
| 194 | `"Parent directory does not exist: " + parent + ". Use Bash 'mkdir -p' to create it first."` | **AP4 LOW** (parent path leaked); actionable next step (good)
| 196 | `fmt.Sprintf("Cannot access parent directory %s: %v", parent, err)` | **AP4 LOW** (path + raw err.Error())
| 199 | `"Parent path exists but is not a directory: " + parent` | **AP4 LOW** (parent path)
| 206 | `"Path is a directory, not a file: " + cleaned` | **AP4 LOW** (full path)
| 217 | `"Cannot verify Read-first guard: agent state missing. Read the file first."` | clean — internal terminology "agent state missing" is acceptable since this is a Forgify-specific guard the LLM should learn; could be tightened to "Read the file first." alone. **LOW (AP3 elem)**
| 220 | `"File must be read first before overwriting: " + cleaned + ". Use the Read tool first."` | **AP4 LOW** (full path); actionable
| 228 | `fmt.Sprintf("Cannot create temp file in %s: %v", parent, err)` | **AP4 MED** — leaks `parent` directory + os.CreateTemp internal err.Error() (which may include "permission denied: /Users/sunweilin/.forgify/...") — that path is **server internal**, not user-supplied
| 240 | `fmt.Sprintf("Write failed (writing temp): %v", err)` | **AP4 MED** — `%v` on os IO err may include filesystem internal paths (the tmp file path Forgify staged) — leaks `parent + "/.forgify-write-XXXXXX"` to LLM
| 244 | `fmt.Sprintf("Write failed (closing temp): %v", err)` | same MED as 240
| 259 | `fmt.Sprintf("Write failed (chmod temp): %v", err)` | same MED as 240
| 264 | `fmt.Sprintf("Write failed (rename to target): %v", err)` | **AP4 MED** — `%v` may include "rename /tmp/.forgify-write-abc123 /Users/x/y.txt: cross-device link" exposing both internal tmp path AND target — same severity
| 276 | `"File successfully written to " + cleaned` | **AP5 LOW** + AP4 LOW — slight verbose pattern ("File successfully written to" vs "Wrote /path/to/file"). Actually fine — concise enough. But `cleaned` leaks full path. LLM learns to reflect target path; acceptable for filesystem tool semantics.

**Cross-cutting impact for Write**: lines 228/240/244/259/264 all dump raw `err.Error()` from os tmp-file operations. These error strings can contain Forgify's internal staging path (`<dir>/.forgify-write-NNN`). The user's LLM context will accumulate references to internal staging files — **AP4 MED**. Suggested fix: a `formatWriteErr(stage, err)` helper that classifies stdlib errors (permission, ENOSPC, EXDEV) into clean human messages without raw err embedding.

---

### filesystem.Edit (`filesystem/edit.go`)

#### Description (line 64-78)

Length: 14 lines. Same shape as Write.
- AP3 (very LOW, optional): "Use replace_all: true to rename a string everywhere in the file (e.g. variable rename); only do this when you have verified all occurrences are intended replacements" — soft "should/must" guidance, fine for a description.
- "On success, the result message reports the actual number of replacements performed" — meta description of result format. Could be argued as AP3 ("note: …"), but it's actually concrete contract info — LLM benefits from knowing the post-condition shape. **No finding**.

#### Parameters (line 85-107)

- All field descriptions tight and concrete. No violations.

#### Execute (line 212-334)

| Line | Return string | Anti-pattern |
|---|---|---|
| 220 | `fmt.Errorf("Edit.Execute: %w", err)` | cross-cutting #1 (Go err path)
| 225 | pathGuard reason | cross-cutting #2
| 234 | `"File not found: " + cleaned + ". Edit can only modify existing files; use Write to create new ones."` | **AP4 LOW** (full path); actionable suggestion (good)
| 236 | `fmt.Sprintf("Cannot access %s: %v", cleaned, err)` | **AP4 LOW** (path + raw err)
| 239 | `"Path is a directory, not a file: " + cleaned` | **AP4 LOW**
| 245 | `"Cannot verify Read-first guard: agent state missing. Read the file first."` | LOW — same as Write line 217
| 249 | `"File must be read first before editing: " + cleaned + ". Use the Read tool first."` | **AP4 LOW** (path); actionable
| 259-262 | `"File has been modified since last read (current size %d, expected %d): %s. Read it again before editing."` | **AP4 LOW** (path); also an internal implementation detail leak ("size mismatch detection") — but this is by design (LLM needs to know to re-Read). MED for the design intent leak; LOW for the actionable wording
| 268 | `fmt.Sprintf("Cannot read %s: %v", cleaned, err)` | **AP4 LOW**
| 276 | `"old_string not found in the file. Verify the exact text (whitespace and case matter)."` | clean. Actionable.
| 278-281 | `"Found %d matches of old_string in %s, but replace_all is false. Either provide more surrounding context to make old_string unique, or set replace_all: true."` | **AP4 LOW** (path); good actionable wording (provides 2 options). MED-density verbosity but useful — net **LOW**
| 301 | `fmt.Sprintf("Edit failed (cannot create temp): %v", err)` | **AP4 MED** — leaks tmp path / internal err same as Write
| 309 | `"Edit failed (writing temp): %v"` | **AP4 MED** same
| 313 | `"Edit failed (closing temp): %v"` | **AP4 MED** same
| 317 | `"Edit failed (chmod temp): %v"` | **AP4 MED** same
| 321 | `"Edit failed (rename to target): %v"` | **AP4 MED** same
| 331 | `"Successfully replaced 1 occurrence in %s."` | **AP4 LOW** (path); **AP5 LOW** (could be "Replaced 1 occurrence in <path>." — `Successfully` is a tiny bit of verbose-success padding but minimal)
| 333 | `"Successfully replaced %d occurrences in %s."` | **AP4 LOW** + **AP5 LOW** same as 331

---

### search.Grep (`search/grep.go` + `grep_rg.go` + `grep_stdlib.go`)

#### Description (`grep.go` line 58-69)

```
A powerful content search tool, backed by ripgrep when available (fast) or a stdlib regex fallback when not.

Usage:
- ALWAYS use Grep for content search tasks. NEVER invoke `grep` or `rg` as a Bash command — Grep is optimized for safe path access.
- Supports full regex syntax (e.g. "log.*Error", "function\s+\w+")
- Filter files with the `glob` parameter (e.g. "*.go", "**/*.tsx") or the `type` parameter (e.g. "go", "py", "js")
- Output modes: "content" shows matching lines (use -n for line numbers, -A/-B/-C for context), "files_with_matches" returns matching file paths only (default; cheapest), "count" returns one path:N line per matching file.
- Pattern syntax: full RE2 (Go) / PCRE-ish (ripgrep). Literal braces need escaping: `interface\{\}` to find `interface{}` in Go code.
- Multiline: by default patterns match within single lines only. For cross-line patterns like `struct \{[\s\S]*?field`, set `multiline: true`.
- `-i` for case-insensitive. `head_limit` caps the result list.
- The `path` parameter (file or directory) must be absolute when provided. Defaults to current working directory.
- Sensitive paths (system dirs, credential locations like ~/.ssh) are blocked for safety.
```

- AP7 LOW: "A powerful content search tool" — marketing fluff opener ("powerful"). LLM doesn't benefit from the puffery; just say "Search file contents using regex." Same finding pattern as `globDescription`. **LOW**.
- AP3 LOW (caps shouting, but useful in this case): "ALWAYS use Grep…NEVER invoke grep or rg as a Bash command" — these caps are deliberate to stop the LLM from routing through Bash and bypassing PathGuard. Defensible. **NOT a finding** — the tone is functional.
- AP2 (very LOW): backticks-quoted parameter names is fine for code-formatting in markdown; LLM handles either.

#### Parameters (`grep.go` line 78-134)

Field descriptions are dense but fine. No violations.

- `pattern` description: "The regex pattern to search for. Use full regex syntax. Literal braces need escaping (e.g. \"interface\\\\{\\\\}\")." — escaping example is helpful.
- `output_mode` description: "How to present matches: content (lines), files_with_matches (paths only, default, cheapest), count (path:N per file)." — clear.

No findings.

#### Execute paths

`grep.go::Execute` (line 251-293):
| Line | Return string | Anti-pattern |
|---|---|---|
| 254 | `fmt.Errorf("Grep.Execute: %w", err)` | cross-cutting #1
| 259 | pathGuard reason | cross-cutting #2
| 266 | `"Search root not found: " + cleaned` | **AP4 LOW** (full host path)
| 268 | `fmt.Sprintf("Cannot access %s: %v", cleaned, err)` | **AP4 LOW**

`grep_rg.go::execRg` (line 38-72):
| Line | Return string | Anti-pattern |
|---|---|---|
| 56 | `fmt.Errorf("Grep.execRg: %w (stderr: %s)", err, stderrSnippet(ee.Stderr))` | **AP4 HIGH** — leaks rg's raw stderr (truncated to 512 bytes) which can include rg's internal version messages, file system error details, and (most importantly) the full PATH (`/Users/.../`) of files rg was scanning. Rg in error scenarios prints full absolute paths. This is a Go err returning to caller; if `t.execRg` returns err, line 285 (in `Execute`) logs warn AND falls back to stdlib **without surfacing the err to LLM** — so LLM doesn't actually see this string. **Effective: NOT exposed to LLM**. Lower to **MED** because the path of `loop/tools.go` returning err.Error if execRg's err were ever propagated still exists; and stderr-snippet is a footgun pattern.
| 58 | `fmt.Errorf("Grep.execRg: %w", err)` | as above; not currently exposed to LLM (fallthrough), but still cross-cutting #1 footgun
| 65 | `noMatchesMessage(args)` → `fmt.Sprintf("No matches for %q in %s.", args.Pattern, root)` (line 134) | **AP4 LOW** (full host path in `root`); the message itself is concise and useful — ✅ good shape

`grep_rg.go::capLines` (line 152): `"... [truncated at %d lines; raise head_limit to see more]\n"` — clean. Actionable.

`grep_stdlib.go::execStdlib` (line 117-137):
| Line | Return string | Anti-pattern |
|---|---|---|
| 120 | `fmt.Sprintf("Invalid regex pattern: %v", err)` | **AP4 LOW** — leaks Go regexp's internal err.Error() which uses `regexp/syntax` package's verbose message format ("error parsing regexp: missing argument to repetition operator: `*`"). Useful for LLM to fix the pattern. Borderline; LOW.
| 126 | `fmt.Errorf("Grep.execStdlib: %w", err)` (Go err) | cross-cutting #1

`grep_stdlib.go::searchContent / searchFilesWithMatches / searchCount`: all output `<path>:<line>:<content>` or similar — by definition exposes host paths (this is the contract). **Not flagged** — paths are the value.

`grep_stdlib.go::searchFilesWithMatches` line 272 — `"... [truncated at %d files; raise head_limit to see more]\n"` — clean.

#### Cross-cutting Grep finding

`grep_rg.go:56` `stderrSnippet` pattern: even though current dispatcher in grep.go:285-289 falls back to stdlib (silently swallowing the rg error from LLM), the err message would be exposed if anyone ever tightens the fallback policy or adds a "no rg fallback" branch. Rotted footgun. **MED structural** (cross-cutting #4).

---

### search.Glob (`search/glob.go`)

#### Description (line 61-71)

```
Fast file finder: matches glob patterns and returns JSON enriched with type / size / mtime per entry.

Usage:
- Supports any glob pattern, including `**` for recursive descent (e.g. "**/*.go", "src/**/*.tsx", "*.md").
- Pass pattern "*" with a directory `path` to list immediate children — Glob fully replaces a separate LS tool.
- Output is JSON: {"root", "matches": [{"path","type","size","mtime"}], "total", "truncated"}.
- Each match's type is one of "file", "dir", or "symlink"; mtime is RFC 3339.
- Matches are sorted by mtime descending (newest first) so recently-edited files surface at the top.
- `path` (search root) defaults to the current working directory; must be absolute when provided.
- `limit` caps the result count (default 100, hard max 1000); the JSON `truncated` flag tells you whether more matches exist.
- Sensitive paths (system dirs, ~/.ssh, ~/.aws, etc.) are blocked for safety.
```

- AP7 LOW: "Fast file finder" — same fluff as Grep's "powerful content search tool". Just say "File finder backed by glob patterns." **LOW**.
- AP6 (cross-cutting): output shape declared as JSON. Grep returns plain `<path>:<line>:<content>` lines (not JSON). **AP6 MED** — same family of search tools, different output formats. LLM learns "search tool may be JSON or plain"; for grep it can't be JSON without breaking ripgrep parity (rg gives line-format), but Glob's JSON-vs-plain inconsistency is a real format-discoverability friction. See cross-cutting #5.

#### Parameters (line 73-90)

- All clean.

#### Execute (line 201-282)

| Line | Return string | Anti-pattern |
|---|---|---|
| 204 | `fmt.Errorf("Glob.Execute: %w", err)` | cross-cutting #1
| 209 | pathGuard reason | cross-cutting #2
| 216 | `"Search root not found: " + root` | **AP4 LOW**
| 218 | `fmt.Sprintf("Cannot access %s: %v", root, err)` | **AP4 LOW**
| 221 | `"Search root must be a directory: " + root` | **AP4 LOW**
| 231 | `fmt.Sprintf("Invalid glob pattern %q: %v", args.Pattern, err)` | **AP4 LOW** — `err.Error()` from doublestar is brief and friendly; minimal leak
| 279 | `fmt.Errorf("Glob.Execute: marshal result: %w", err)` (Go err) | cross-cutting #1; in practice marshal can't fail here (struct shape is closed) so dead path

JSON-shaped success output is good — clean, predictable for LLM.

---

### shell.Bash (`shell/bash.go` + `bash_route.go`)

#### Description (`bash.go` line 87-101)

```
Run a shell command on the user's machine.

Usage:
- `command` is the shell command. On macOS/Linux it runs via `/bin/sh -c`; on Windows it runs via `cmd.exe /c`. Use shell-portable syntax when possible. Examples: "ls -la" (unix) / "dir" (windows), "git status", "go test ./...".
- `description` is a one-line note for the human reader (e.g. "List repo files").
- `run_in_background: true` spawns the command without waiting and returns a bash_id; use BashOutput to poll for new output and KillShell to terminate.
- `timeout` (milliseconds, foreground only) defaults to 120000 (2 min); hard max 600000 (10 min). For longer-running tasks use background mode.
- The conversation has a tracked working directory: `cd <path>` as the entire command updates it; subsequent commands run there. Chained 'cd ... && ...' does not update the tracked cwd (matches normal subshell semantics).
- Combined stdout+stderr is returned, capped at 256 KB. Exit code appears in a status footer.
- This is a local single-user app — there is no banned-command list. Be careful with destructive commands; the user sees what you propose to run.

Sandbox auto-routing (Python and Node only — other languages run on the host system):
- Commands that invoke `python`, `pip`, `uv`, `virtualenv`, `pipenv`, `poetry`, `node`, `npm`, `npx`, `yarn`, or `pnpm` automatically execute inside a per-conversation isolated environment so packages do not pollute the host. Detection covers nested forms — `bash -c "pip install ..."`, `env VAR=val python ...`, `/usr/bin/python3 ...`, `cd /tmp && python ...` chains, subshells, and `which python3`.
- Other languages (Rust, Go, Ruby, PHP, Java, .NET, etc.) currently run on the host system — install them yourself if needed; isolation is not provided.
- The router cannot see through `eval "..."`, `source ./script.sh`, or commands hidden inside `$(<dynamic-string>)` substitutions — those run on the host system and pollute it. When installing packages or running scripts, write the runtime command directly (e.g. `pip install pandas`, not `eval "pip install pandas"`).
```

- Length: 14 lines main + 4 lines auto-route. Comprehensive.
- AP7 (very LOW): the "description" parameter docs say "one-line note for the human reader". Note: this `description` field is the **non-standard** Bash-specific field — it conflicts conceptually with the framework's standard injected `summary` (which is for the same purpose: LLM-supplied human description per-call). Probable redundancy. The framework guarantees `summary` is required; this `description` is optional and unused except for human display. **AP7 MED** — the `description` field probably should be removed or merged into `summary` to avoid confusing the LLM into populating both (which it sometimes does). See cross-cutting #6.
- AP3 LOW: "Be careful with destructive commands; the user sees what you propose to run." — slight elem of meta-narration, but in this case justified for safety. Borderline. **LOW**.
- "Sandbox auto-routing (Python and Node only — other languages run on the host system)" — clear, but heavy. Could collapse to bullet form. **LOW (verbosity)**.

#### Parameters (`bash.go` line 103-125)

- `command`: "Shell command to execute (POSIX sh)." — fine.
- `description`: "One-line human-readable description of what this command does." — see AP7 MED above (duplicates `summary`'s purpose).
- `run_in_background`: clean.
- `timeout`: "Foreground timeout in milliseconds (default 120000, hard max 600000). Ignored in background mode." — clean.

#### Execute (`bash.go` line 205-246)

| Line | Return string | Anti-pattern |
|---|---|---|
| 208 | `fmt.Errorf("Bash.Execute: %w", err)` | cross-cutting #1
| 234 | `formatAutoRouteError(autoRouteErr)` (line 260-268) | **AP3 MED** — message body: `"Sandbox auto-route could not prepare the runtime for this command. The command was NOT executed (running on the system shell would return misleading data — e.g. system Python 3.9.6 instead of the conversation's isolated 3.12 venv). Please retry, or have the user check the sandbox status in testend."`. **AP3** because: (a) "Please retry" is meta-narration ("Please" elem), (b) "have the user check the sandbox status in testend" instructs the LLM to delegate to the user — which can be useful but is rather verbose; (c) the parenthetical "running on the system shell would return misleading data — e.g. system Python 3.9.6" is explanatory noise (LLM doesn't need to know the rationale to recover). **MED**.
| 234 | same line, "Reason: " + err.Error() — **AP4 MED** — the err.Error() will be from `maybeAutoRoute`, which wraps with `shelltool.Bash.maybeAutoRoute: ...`. So LLM sees `Reason: shelltool.Bash.maybeAutoRoute: sandbox not ready (bootstrap incomplete) — python commands cannot run safely on the system shell` — internal symbol leak.

`bash.go::handleCD` (line 391-421):
| Line | Return string | Anti-pattern |
|---|---|---|
| 396 | `"Cannot resolve home directory: " + err.Error()` | **AP4 LOW** — exposes raw os.UserHomeDir err (rare path, low leak)
| 409 | `fmt.Sprintf("cd: %s: %v", target, err)` | **AP4 LOW** (full path; raw err.Error() — but minimal — like "no such file or directory")
| 411 | `fmt.Sprintf("cd: not a directory: %s", target)` | **AP4 LOW**
| 417 | `"cd: agent state missing — cwd not persisted across calls. Subsequent commands will use the process default cwd."` | **AP3 LOW** — internal "agent state missing" leak; the LLM doesn't really need to know what's "missing", just "cwd won't persist". Slightly meta. **LOW**
| 420 | `"Changed working directory to " + target` | clean. Path is by-design (LLM asked for cd). ✅

`bash.go::runForeground` (line 446-480):
| Line | Return string | Anti-pattern |
|---|---|---|
| 460 | `formatForegroundResult(output, -1, fmt.Sprintf("command timed out after %s", timeout))` | clean: status footer "[command timed out after X]" + "[exit code: -1]"
| 471 | `formatForegroundResult(output, -1, "cancelled")` | clean
| 475 | `formatForegroundResult(output, exitErr.ExitCode(), "")` | clean — output + status footer
| 477 | `formatForegroundResult(output, -1, "exec failed: "+err.Error())` | **AP4 LOW** — raw os/exec err.Error() leaked into footer (e.g. "exec failed: fork/exec /bin/sh: cannot allocate memory"). Clean enough but could classify

`bash.go::formatForegroundResult` (line 488-500): emits `[exit code: N]` and optional `[<note>]` — clean stable format.

`bash.go::capOutput` (line 506-512): leading note `"...[truncated %d bytes from start]\n"` — clean. Note: drops *from the start*, which loses "the failing command's first line" but keeps the recent traceback — sensible default.

`bash.go::runBackground` (line 522-595):
| Line | Return string | Anti-pattern |
|---|---|---|
| 538 | `fmt.Sprintf("Failed to open stdout pipe: %v", err)` | **AP4 LOW** raw err
| 542 | `fmt.Sprintf("Failed to open stderr pipe: %v", err)` | **AP4 LOW**
| 546 | `fmt.Sprintf("Failed to start background command: %v", err)` | **AP4 LOW** — raw exec err
| 591-593 | `"Started background command (bash_id=%s): %s\nUse BashOutput with this bash_id to poll new output, or KillShell to terminate."` | clean. Actionable. ✅ Echoes user's command back which is fine

`maybeAutoRoute` returns wrapped errors which surface via `formatAutoRouteError`:
- line 292: `fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox service not wired (this is a server build / config issue — please report)")` — **AP3 HIGH**! "(please report)" is direct meta-narration aimed at humans, not LLM-actionable. Also "shelltool.Bash.maybeAutoRoute:" leaks internal package symbol. The LLM sees this as `Reason: shelltool.Bash.maybeAutoRoute: sandbox service not wired (this is a server build / config issue — please report)` — both **AP3** ("please report") and **AP4** (symbol leak).
- line 300: `fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox not ready (%s) — %s commands cannot run safely on the system shell", reason, kind)` — **AP4 MED** symbol leak; "cannot run safely on the system shell" is fine wording.
- line 304: `fmt.Errorf("shelltool.Bash.maybeAutoRoute: no conversation context — %s commands need a conversation-scoped sandbox env", kind)` — **AP4 MED** symbol leak; rest fine.
- line 350: `fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox env install failed (%s for %s): %w", kind, convID, err)` — **AP4 MED** symbol leak + the wrapped `%w` chain may include sandbox-internal paths/errs

#### Cross-cutting Bash finding

The `description` parameter (separate from framework `summary`) duplicates intent. **AP7 MED** (cross-cutting #6).

The auto-route error path (`formatAutoRouteError` body + the wrapped `maybeAutoRoute` errors it embeds) is the **single highest-density LLM noise** in the audited tools — long explanation, internal symbol leaks, "please report" meta-narration, parenthetical rationale. **AP3+AP4 HIGH** (cross-cutting #7). LLM seeing this output will train to treat verbose multi-paragraph error explanations + symbol prefixes as acceptable in the tool family. Suggest replacing with a 2-line "Sandbox unavailable for <runtime>. Reason: <classified-reason>. Try: <action>."

---

### shell.BashOutput (`shell/output.go`)

#### Description (line 32-39)

```
Read new stdout/stderr from a background shell process started by Bash.

Usage:
- `bash_id` is the ID returned by a Bash call with run_in_background:true.
- Returns only output APPEARED SINCE THE LAST BashOutput call for that ID — successive polls don't repeat what you've already seen.
- Includes a status footer: "running", "exited (code N)", "killed", or "errored: ...".
- `filter` (optional regex) keeps only matching lines from the new output.  Useful for grepping a noisy log stream.
- Returns ErrProcessNotFound if the bash_id is unknown (never started or already removed via KillShell).
```

- AP4 (HIGH for the description itself): "Returns ErrProcessNotFound if the bash_id is unknown" — **leaks a Go sentinel name** into the description text. The LLM should not see `ErrProcessNotFound` (an internal Go identifier); it should see "Returns 'not found' if bash_id is unknown" or similar. **AP4 HIGH** in description prose.
- "(never started or already removed via KillShell)" — concise + actionable. Fine.

#### Parameters (line 41-54)

- All clean.

#### Execute (line 103-128)

| Line | Return string | Anti-pattern |
|---|---|---|
| 109 | `fmt.Errorf("BashOutput.Execute: %w", err)` | cross-cutting #1
| 114 | `fmt.Sprintf("Background shell process not found: %s", args.BashID)` | clean — actionable, low-leak (bash_id is just an opaque ID); ✅
| 127 | `formatOutputResult(...)` | see below

`formatOutputResult` (line 150-177): emits body + `(no new output since last poll)` if empty + optional `[note: %d bytes dropped...]` + `[status: %s]` footer. **Clean stable format**. ✅

#### Compared to Bash error format

Status footer format `[status: running]` / `[status: exited (code N)]` / `[status: killed]` / `[status: errored]` — note **AP6 micro-inconsistency**: Bash foreground footer uses `[exit code: N]` separately from `[<note>]`; BashOutput footer is `[status: ...]` with everything inline. Different family member emits different shape. **LOW (AP6)**.

---

### shell.KillShell (`shell/kill.go`)

#### Description (line 21-26)

```
Terminate a background shell process started by Bash.

Usage:
- `shell_id` is the bash_id returned by a Bash call with run_in_background:true.
- Sends SIGKILL on Unix; the process is removed from the registry whether or not it was still running.
- Idempotent: killing an already-finished or unknown ID returns a clear message instead of failing.
```

- Clean and concise.
- **AP6 MED** — parameter is named `shell_id` here but Bash returns `bash_id` and BashOutput accepts `bash_id`. Naming inconsistency within the same family. The schema description tries to bridge ("the bash_id returned by Bash…") but the schema field name itself is `shell_id`. **AP6 MED** — confusing for LLM; either rename param to `bash_id` or document the rename more prominently.

#### Parameters (line 28-37)

- `shell_id` description: "ID of the background shell process to terminate (the bash_id returned by Bash with run_in_background:true)." — bridges the rename, but the rename itself is the issue. See AP6 MED above.

#### Execute (line 78-106)

| Line | Return string | Anti-pattern |
|---|---|---|
| 83 | `fmt.Errorf("KillShell.Execute: %w", err)` | cross-cutting #1
| 88 | `fmt.Sprintf("Background shell process not found: %s", args.ShellID)` | clean ✅
| 103 | `fmt.Sprintf("Killed background shell %s.", args.ShellID)` | clean ✅
| 105 | `fmt.Sprintf("Background shell %s already finished; removed from registry.", args.ShellID)` | clean ✅

---

### web.WebFetch (`web/fetch.go`)

#### Description (line 99-107)

```
Fetches a URL and returns an LLM-generated summary tailored to your prompt.

Usage:
- `url` must be an absolute http or https URL.
- `prompt` describes what to extract or summarise from the page (e.g. "What does this paper conclude?", "List every API endpoint mentioned").
- The tool fetches the URL (Jina reader for clean markdown when available, direct HTTP GET fallback), caps content at 1 MB, then asks the configured summary model to answer your prompt against that content.
- Summarisation uses the user's "web_summary" model scenario if configured; otherwise it falls back to the main "chat" scenario, so this works out of the box.
- Private / loopback / link-local addresses are blocked for safety (no fetching localhost or RFC 1918 ranges).
- Each fetch is capped at 30 seconds.
```

- Clean. Mentions Jina (third-party) which is fine — the LLM doesn't need to abstract over it.
- AP7 LOW: "so this works out of the box" — slight ad-copy phrasing; the LLM doesn't benefit from the marketing tone. Just say "...falls back to the main chat scenario." **LOW**.

#### Parameters (line 109-122)

- `url`: clean.
- `prompt`: clean.

#### Execute (line 190-224)

| Line | Return string | Anti-pattern |
|---|---|---|
| 196 | `fmt.Errorf("WebFetch.Execute: %w", err)` | cross-cutting #1
| 201 | `fmt.Sprintf("Invalid URL %q: %v", args.URL, err)` | **AP4 LOW** — raw url.Parse err
| 203 | `guardHostname(...) → "Refusing to fetch loopback host: " + host` etc | clean wording; ✅
| 209 | `fmt.Sprintf("Failed to fetch %s: %v", args.URL, err)` | **AP4 LOW** — leaks raw http transport err.Error() (e.g. "Get \"https://...\": dial tcp: lookup foo.com: no such host"). Borderline — actionable for LLM (knows DNS failed). **LOW**.
| 212 | `fmt.Sprintf("Fetched %s but body was empty.", args.URL)` | clean ✅
| 220-221 | `fmt.Sprintf("Summarisation failed (%v). Raw content (first 4 KB):\n\n%s", err, truncate(content, 4096))` | **AP4 LOW** — leaks raw LLM err (could include "anthropic: HTTP 400: invalid_request"); minor leak; balanced by being actionable. **LOW**.

`guardHostname` strings (line 358-401):
- All "Refusing to fetch <kind> address: " + ip.String() — clean wording. Slightly verbose ("Refusing to fetch" vs "Blocked: ") but consistent. ✅

#### `summarise` flow (line 412-428): result is the LLM-generated summary text (controlled by another LLM, not Forgify). Not in audit scope (LLM output is the LLM's job).

`buildSummaryPrompt` (line 434-449): the *prompt sent to the summary LLM* (not LLM-facing). Out of scope for AP analysis (this is what Forgify sends to summary model, not what summary model returns). However, note: the prompt mentions "fetched on the user's behalf" and uses `<<<CONTENT_BEGIN>>>` delimiters. Reasonable.

---

### web.WebSearch (`web/search.go` + `search_byok.go` + `search_mcp.go`)

#### Description (`search.go` line 99-106)

```
Web search. Routes to the first available source: configured BYOK provider (Brave / Serper / Tavily / Bocha), then duckduckgo-search MCP server (if installed). When neither is available the tool returns a clear hint — call list_mcp_marketplace to discover the duckduckgo-search backend, then install_mcp_server({name:"duckduckgo-search"}) to add it (~30s, no key needed).

Usage:
- `query` is the search string (treated as one phrase by the upstream engine).
- Returns JSON: {"query","source","results":[{"title","url","snippet"}],"truncated"}.
- `source` tells you which backend produced the results: "brave" / "serper" / "tavily" / "bocha" / "mcp".
- `limit` caps the result count (default 10, hard max 30).
- Each backend has a 10-second budget; the tool falls through if a backend returns no results or errors.
```

- Length: long opening sentence (3 sub-clauses + tooling hint). Could split.
- AP3 (LOW): the description itself instructs the LLM to "call list_mcp_marketplace to discover…then install_mcp_server({name:\"duckduckgo-search\"})" — this is **chained tool-call guidance baked into the description**. While useful (tells LLM what to do when no backend is available), it's a mild form of meta-narration ("when X fails, do Y"). Better in the *result message* (line 255-263) rather than the description. **LOW**.
- The duckduckgo-search MCP server installation hint is also repeated in the result error message (line 255-263) — that's the **right place** for it. The description hint is partially redundant. **LOW (AP3)**.

#### Parameters (line 108-121)

- All clean.

#### Execute (line 221-264)

| Line | Return string | Anti-pattern |
|---|---|---|
| 224 | `fmt.Errorf("WebSearch.Execute: %w", err)` | cross-cutting #1
| 236 | `marshalSearchResponse(args, source, results)` → JSON | clean ✅
| 251 | `marshalSearchResponse(args, "mcp", results)` → JSON | clean ✅
| 255-263 | the long "no backend available" message | see below

The "no backend available" message:

```
No results for %q. No search backend is currently available.

To enable web search, do ONE of the following:
  • Configure a search-category API key in Settings → API Keys (Brave / Serper / Tavily — international; Bocha — China). All have free tiers.
  • Install the duckduckgo-search MCP server from the marketplace (no API key needed; ~30s install). The user can do this from the MCP tab.

(The previous Bing CN HTML scrape fallback was removed because Bing now renders results via JavaScript, making server-side HTML scraping return 0 results.)
```

- AP3 MED: "(The previous Bing CN HTML scrape fallback was removed because Bing now renders results via JavaScript, making server-side HTML scraping return 0 results.)" — this is **historical implementation context** that the LLM does not need. Pure noise from LLM's perspective; LLM is not debugging Forgify's history. **MED**. Suggest deleting.
- AP3 LOW: "(no API key needed; ~30s install)" — implementation detail that helps LLM decide which option to recommend; defensible. **LOW**.
- AP3 LOW: "The user can do this from the MCP tab." — instructs LLM to delegate to user's UI; clear next step but slightly meta. **LOW**.
- AP2 (LOW): bullets render fine but the message is heavy markdown for what could be 3 lines.

`marshalSearchResponse` (line 385-401): JSON output with `query / source / results / truncated`. Clean stable format. ✅

#### `search_byok.go` BYOK clients

`searchBrave` / `searchSerper` / `searchTavily` / `searchBocha` — all have `fmt.Errorf("<provider>: build: %w", err)`, `fmt.Errorf("<provider>: parse: %w", err)`, `fmt.Errorf("<provider>: connection: %w", err)`. These errors return up to `tryBYOKProvider` (search.go:325) which logs them at warn and falls through to next tier — **NOT exposed to LLM** in current dispatch. ✅

`doSearchHTTP` (line 179-220):
- line 217: `fmt.Errorf("%s: HTTP %d: %w: %s", provider, resp.StatusCode, sentinel, snippet(body, 200))` — leaks raw response body snippet (200 bytes from `provider.URL`) into err. **AP4 MED** if ever surfaced — currently it's logged at warn, not LLM-facing, so net **LOW** in practice.

#### `search_mcp.go` MCP shim

`runMCPSearch` (line 54-67): `fmt.Errorf("mcp: parse: %w", perr)` → returned to Execute (search.go:243), which checks `err != nil` and logs at warn but does NOT surface to LLM. ✅

`parseMCPSearchResults` (line 77-136): line 133 — `searchResult{Title: "MCP search result", Snippet: raw}` — fallback wraps unknown shape as a single result. Clean. ✅

---

## Cross-cutting patterns

### #1 — `<Tool>.<Method>:` wrapper leaks via S16 → ValidateInput → loop/tools.go:191

**Severity: HIGH**

Every tool's `ValidateInput` and `Execute` Go-error path uses S16 wrap format `fmt.Errorf("<Tool>.<Method>: %w", err)`. When ValidateInput fails, `loop/tools.go:191` exposes `err.Error()` to LLM via `"input validation failed: %s"`. So LLM sees verbatim:
- `input validation failed: Read.ValidateInput: invalid character '}' after top-level value`
- `input validation failed: WebFetch.ValidateInput: parse "ht!tp://example.com": net/url: invalid control character in URL`
- `input validation failed: Bash.ValidateInput: invalid character …`

The `Read.ValidateInput:` / `WebFetch.ValidateInput:` etc. prefixes are **internal Go symbol references** — meaningless to the LLM and pollute its training context. The LLM doesn't need to know which package or which method; it needs to know what's wrong with its input.

**Affected sites** (each `fmt.Errorf("<Tool>.<Method>: %w", err)` inside a ValidateInput or Execute that gets exposed):
- `filesystem/read.go:154, 205`
- `filesystem/write.go:126, 179`
- `filesystem/edit.go:145, 220`
- `search/grep.go:215, 254`
- `search/glob.go:170, 204`
- `shell/bash.go:176, 208`
- `shell/output.go:81, 89, 109`
- `shell/kill.go:62, 83`
- `web/fetch.go:159, 169, 196`
- `web/search.go:195, 224, 398`

Suggested fix: framework-level shim in `loop/tools.go:191` that strips `<Pkg>.<Method>: ` prefix before exposing to LLM, OR change tool implementations to wrap with `userMsg + ": " + err` (where `userMsg` is LLM-facing) instead of S16 sentinel format. The S16 sentinel format is correct for **internal Go telemetry / log** but wrong for **LLM-facing tool_result**.

### #2 — PathGuard.Allow() reason strings (not audited; assumed by reference)

Multiple tools (Read 219, Write 184, Edit 224, Grep 259, Glob 209) return PathGuard's reason verbatim. PathGuard.Allow is in `pkg/pathguard` (not in audit scope per the 4-subpackage filter), but its strings reach LLM through these tools. Recommendation: confirm a future audit covers `pkg/pathguard` reason wording. Best-case its reasons are short ("Path /etc/passwd is denied: system directory") — but if they're verbose or include internal terminology, every audited tool inherits the leak. **MED structural** (relies on external file).

### #3 — Host-path reflection in success and error messages

**Severity: MED (class-level)**

Filesystem / search / shell tools reflect host path back into LLM context on every success and error. Per call, this is **necessary information** (LLM needs to correlate the path it asked about). **Across many calls**, the LLM accumulates "absolute paths in tool_results are normal" training signal — including paths that contain user identity (`/Users/sunweilin/...`), project names, system directories.

This is largely **unavoidable** for filesystem tools (the path is the value). But concrete cleanup wins:
- (a) **error messages** that wrap `%v` of an os err often leak *internal* paths the LLM didn't ask for (e.g. tmp file paths, parent dirs). See AP4 MED in Write 228/240/244/259/264 and Edit 301/309/313/317/321. Fix: classify common os errs (`fs.ErrNotExist` / `fs.ErrPermission` / `os.PathError.Op`) and emit clean messages without the raw err string.
- (b) Specifically `os.CreateTemp` / `os.Rename` errors leak the **intermediate** path Forgify staged (`<dir>/.forgify-write-XYZ`), which the LLM never knew about. Different from "reflecting LLM's input"; this is an **internal staging leak**.

### #4 — `stderrSnippet` footgun in grep_rg.go

**Severity: LOW** (currently dead path); **MED** structural

`grep_rg.go:56` returns `fmt.Errorf("Grep.execRg: %w (stderr: %s)", err, stderrSnippet(ee.Stderr))` which embeds rg's raw stderr (truncated 512B). Currently the dispatcher silently falls through to stdlib so this err never reaches LLM. But if anyone ever hardens the fallback ("don't fallback on user-side regex error"), this rg-stderr leak becomes LLM-facing. The 512B can include rg's version, scanned paths, and internal hints. Suggest classifying rg exit codes upfront and emitting clean messages, dropping `stderrSnippet`.

### #5 — Output format inconsistency within the search tool family

**Severity: MED**

- `Glob` returns JSON: `{root, matches:[…], total, truncated}`.
- `Grep` returns plain text lines: `<path>:<lineno>:<text>` (mirrors ripgrep CLI format).

Both are "find things" tools, both in `search/` sub-package, both routed through the same `SearchTools(...)` factory. The LLM has to learn two output discoverability conventions. While Grep's plain-line format is constrained by ripgrep parity (shellable into next stage), Glob could plausibly emit a similar plain format ("path tab type tab size tab mtime\n") for consistency. **AP6 MED**.

### #6 — Bash's `description` parameter conflicts with framework's `summary`

**Severity: MED**

`bash.go::bashSchema` (line 110-114) declares a `description` field for "one-line note for the human reader". The framework injects a standard `summary` field for the same purpose. The LLM may populate both, fail to populate one consistently, or pick the wrong one for display. **AP7 MED** + **AP6 MED** — recommend removing `description` from Bash schema and routing display through `summary`.

### #7 — Sandbox auto-route error path: highest-density LLM noise in audited tools

**Severity: HIGH**

`bash.go::formatAutoRouteError` (line 260-268) + the wrapped `maybeAutoRoute` errors it embeds:

```go
body := "Sandbox auto-route could not prepare the runtime for this command. " +
    "The command was NOT executed (running on the system shell would " +
    "return misleading data — e.g. system Python 3.9.6 instead of " +
    "the conversation's isolated 3.12 venv). Please retry, or have " +
    "the user check the sandbox status in testend.\n\n" +
    "Reason: " + err.Error() + "\n"
```

Composed with one of:
- `shelltool.Bash.maybeAutoRoute: sandbox service not wired (this is a server build / config issue — please report)` (AP3 + AP4: "please report" + symbol leak)
- `shelltool.Bash.maybeAutoRoute: sandbox not ready (bootstrap incomplete) — python commands cannot run safely on the system shell` (AP4: symbol leak)
- `shelltool.Bash.maybeAutoRoute: no conversation context — python commands need a conversation-scoped sandbox env` (AP4: symbol leak)
- `shelltool.Bash.maybeAutoRoute: sandbox env install failed (python for cv_xxx): %w` (AP4: symbol + convID leak + wrapped chain)

The LLM sees a multi-paragraph explanation including:
- Implementation rationale ("running on the system shell would return misleading data")
- A specific version detail ("system Python 3.9.6 instead of the conversation's isolated 3.12 venv") that's **made up** — not derived from runtime state
- A meta-instruction directed at the human ("Please retry, or have the user check the sandbox status in testend") + ("please report")
- A `Reason:` prefix carrying the symbol-leaked wrapper chain

**Suggested fix**: replace with a 2-line classifier:

```
Sandbox unavailable for python: bootstrap incomplete.
Run `make resources` to install runtime dependencies, or the user can check status in testend.
```

This is the single highest-payoff cleanup target in the audit.

### #8 — Verbose-success padding (low magnitude, system-wide)

**Severity: LOW**

Many success messages use "Successfully <verb>" pattern:
- Edit: `Successfully replaced 1 occurrence in /path/to/file.` (line 331/333)
- Write: `File successfully written to /path` (line 276)
- Bash background: `Started background command (bash_id=...)` (no "Successfully" — actually clean)
- KillShell: `Killed background shell <id>.` (clean)

CC's pattern from training data leaks the "Successfully" word. Could trim:
- "Replaced 1 occurrence in <path>." (Edit)
- "Wrote <path>." (Write)

The current wording is consistent and fine; just noting **LOW (AP5)** that "Successfully" is mild padding on every successful Edit/Write.

### #9 — Marketing fluff in tool descriptions

**Severity: LOW**

- `grepDescription` (grep.go:58): "A powerful content search tool…" — drop "powerful".
- `globDescription` (glob.go:61): "Fast file finder…" — drop "Fast".
- `webFetchDescription` (fetch.go:99): "...so this works out of the box." — drop "so this works out of the box."

These are mild AP7 cases — the LLM doesn't benefit from adjectives like "powerful/fast"; they just train it to use similar inflated language in its responses.

---

## 总计

| AP class | Count | HIGH | MED | LOW |
|---|---|---|---|---|
| AP1 教学 | 0 | 0 | 0 | 0 |
| AP2 markdown | 1 | 0 | 0 | 1 |
| AP3 元话术 | 7 | 1 | 2 | 4 |
| AP4 backend leak | 33 | 2 | 11 | 20 |
| AP5 verbose success | 3 | 0 | 0 | 3 |
| AP6 format inconsistency | 4 | 0 | 3 | 1 |
| AP7 description脱钩 | 9 | 0 | 2 | 7 |
| **Total findings** | **57** | **3** | **18** | **36** |

Cross-cutting patterns (counted once above, listed for prioritization):
- **#1 HIGH** — `<Tool>.<Method>:` symbol prefix exposed via ValidateInput surface (~20 sites)
- **#7 HIGH** — Bash sandbox auto-route error message: AP3 meta-narration + AP4 symbol leak + made-up version data
- **#3 MED** — class-level host-path reflection (intentional for filesystem tools, leak for staging tmp paths)
- **#5 MED** — search family format split (JSON vs plain text)
- **#6 MED** — Bash's `description` field conflicts with framework `summary`
- **#2 MED** — PathGuard.Allow() reason strings inherited (out-of-audit; needs cross-package check)
- **#4 LOW (currently)** — rg stderr footgun

Highest-payoff fixes:
1. **#7** — rewrite `formatAutoRouteError` body + classify `maybeAutoRoute` errors. One file, ~10 lines of logic, eliminates ~5 HIGH-density leak sites and removes the made-up Python 3.9.6 vs 3.12 fabrication.
2. **#1** — strip `<Tool>.<Method>:` prefix in `loop/tools.go:191` (one-line `strings.SplitN(err.Error(), ": ", 2)` after `<word>.<word>:` regex check). Cleans ~20 sites at once.
3. **Cluster (Write 228/240/244/259/264 + Edit 301/309/313/317/321)** — internal staging-path leak. Add a `formatTempFsErr(stage, err)` helper.
4. **#9** — delete "powerful" / "Fast" / "so this works out of the box" from descriptions (3 deletions).
5. **#8** — drop "Successfully" prefix from Write/Edit messages (2 lines).
