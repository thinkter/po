# po Feature Status Report (Verbose)

## Overview

This document summarizes the **current functional state** of the `po` CLI based on the implemented code paths in:

- `cmd/po/main.go`
- `cmd/po/save.go`
- `cmd/po/sync.go`
- `internal/git/git.go`

It classifies behavior into:

- ✅ **Working**
- ⚠️ **Partially working / edge-case dependent**
- ❌ **Not implemented / missing**

---

## 1) Command Surface and Entry Point

## ✅ Working

### Git presence check before command execution
`po` verifies Git is installed before running subcommands.

- Implemented via `git.IsInstalled()`
- Fails early with a helpful message if Git is unavailable

### Subcommand routing
`po` currently routes:

- `po save` → `runSave()`
- `po sync` → `runSync()`

### Fallback usage output
When no recognized command is provided, usage text is shown.

---

## 2) `po save` Feature Status

## ✅ Working

### Reads changed files from Git status
`po save` gets changes via `git status --porcelain` and parses status code + path.

### Interactive file selection UI
Uses `huh` multi-select to choose:

- specific files
- or `"All changed files"` via sentinel `__ALL__`

### Selection normalization and validation
The save flow:

- trims and deduplicates selected paths
- ensures selected values exist in current status set
- aborts safely on empty/invalid selection

### Commit metadata collection
Interactive form collects:

- Conventional-style commit type (`feat`, `fix`, etc.)
- Non-empty commit message (validated)

### New `.gitignore`-priority workflow
If selected files include `.gitignore`, current behavior is:

1. Stage `.gitignore` only
2. Commit with `chore(gitignore): update ignore rules`
3. Push immediately
4. Detect tracked files that are now ignored using:

   `git ls-files -ci --exclude-standard -z`

5. If matches exist:
   - run `git rm --cached -- <files...>`
   - commit with `chore(gitignore): untrack ignored files`
   - push
6. Stage/commit/push remaining selected files with user-provided message

This directly implements your intended safety workflow.

### Handles scenario where only `.gitignore` was selected
If `.gitignore` is the only selected file (and possibly cleanup happened), the tool exits successfully without forcing an extra empty commit.

### Push behavior supports first push/upstream setup
`git.Push()`:

- detects whether upstream exists
- if missing, runs `git push --set-upstream origin <branch>`
- otherwise runs normal `git push`

---

## ⚠️ Partially working / edge-case dependent

### Path parsing from porcelain format may break in some rename/copy scenarios
Current parser uses:

- `code := line[:2]`
- `path := line[3:]`

This is enough for common cases, but porcelain output for renames can include `old -> new`, and special characters can be tricky. It works for many repos but is not the most robust parser possible.

### `.gitignore` detection includes nested `.gitignore` files
Detection checks:

- exact `.gitignore`
- any selected path ending with `/.gitignore`

This is useful, but behavior choice is global cleanup via repo-wide `git ls-files -ci --exclude-standard`, not scoped only to that nested directory. This is usually desirable, but worth noting.

### Cleanup commit triggers only when `.gitignore` is part of selected files
The cleanup check is currently tied to `.gitignore` being selected in that save operation.  
If `.gitignore` changed but user intentionally didn’t select it, cleanup does not run in that invocation.

### No dry-run/preview before destructive index actions
`git rm --cached` only touches index (not local files), which is safe-ish, but there is no explicit “preview these files first and confirm” prompt yet.

### No explicit handling for “nothing to commit” in each phase
The flow generally avoids empty phases by design, but if the index state becomes unusual, commits can still fail with Git’s “nothing to commit” and bubble up as errors.

---

## ❌ Not implemented / missing in `save`

### No configurable commit templates/messages for gitignore phases
Messages are hardcoded:

- `chore(gitignore): update ignore rules`
- `chore(gitignore): untrack ignored files`

No user override yet.

### No batching strategy for very large `git rm --cached` file lists
For very large repos, argument-length limits may be hit. There is no chunked removal strategy currently.

### No explicit “run this check every time `.gitignore` changes in repo history”
Current behavior is runtime behavior during `po save`, not a Git hook installed globally (like pre-push hook).  
So enforcement happens when using `po save`, not automatically on all Git operations done outside `po`.

### No unit/integration tests for new flow
`go test ./...` passes, but there are no dedicated tests validating:

- `.gitignore` first commit ordering
- cleanup detection behavior
- multi-commit sequencing

---

## 3) `po sync` Feature Status

## ✅ Working

### Fetch + user-selected pull strategy
`po sync` does:

1. `git fetch`
2. asks user for mode:
   - merge
   - rebase
   - fast-forward only
3. runs `git pull` with matching flags

### User-facing guidance strings
Before pulling, prints mode-specific explanation text.

### Abort handling
If user aborts the `huh` form, sync exits gracefully.

---

## ⚠️ Partially working / edge-case dependent

### Conflict resolution delegated entirely to Git
If merge/rebase conflicts occur, command surfaces error and exits.  
No guided conflict workflow is provided (which is acceptable for MVP, but still a limitation).

### No branch cleanliness pre-check
`po sync` does not enforce clean working tree before pull/rebase. Git will handle/deny as needed depending on local state.

---

## ❌ Not implemented / missing in `sync`

### No “sync plan preview”
No ahead/behind summary (`git status -sb`, etc.) before executing pull.

### No auto-stash mode
No built-in option like “stash local changes, sync, pop stash.”

### No post-sync summary
No structured report of what changed after successful sync.

---

## 4) Git Helper Layer (`internal/git`) Status

## ✅ Working

- Generic command execution in current working directory
- `status` parsing for changed files
- add/commit/push/fetch/pull wrappers
- upstream detection and first-push setup
- tracked check (`ls-files --error-unmatch`)
- tracked-but-ignored discovery (`ls-files -ci --exclude-standard -z`)
- cached removal (`rm --cached --`)

---

## ⚠️ Partially working / edge-case dependent

### Error strings are raw git output
Useful for debugging, but UX is not normalized. Some messages can be noisy.

### `runGitCommand` uses current process directory
This is expected, but assumes `po` is run from inside a git repository. There is no explicit repo-root validation helper yet.

---

## ❌ Not implemented / missing in git helper layer

- No explicit helper for “am I in a git repo?”
- No retry/backoff logic for transient network push failures
- No structured typed errors (everything is wrapped string error)

---

## 5) UX Quality and Product Behavior Summary

## ✅ Strong points right now

1. Practical interactive UX for save/sync
2. Good guardrails against empty selections
3. `.gitignore`-first commit sequencing now implemented
4. Automatic untracking of newly ignored tracked files
5. Push behavior works for both existing and new upstreams

## ⚠️ Current friction points

1. Multi-commit save flow can surprise users without preview
2. Hardcoded gitignore commit messages
3. Edge cases around porcelain parsing and huge file lists
4. No test coverage for behavior regressions

---

## 6) Recommended Next Improvements (Priority Order)

1. Add pre-execution summary for `po save` when `.gitignore` is involved:
   - “This will create up to 3 commits”
   - list affected cleanup files (with confirm)

2. Add robust status parsing:
   - consider `git status --porcelain=v1 -z` parser for safer path handling

3. Add chunked `RemoveCached` for very large file lists.

4. Add tests for:
   - `.gitignore` commit precedence
   - cleanup commit creation/non-creation
   - final commit behavior when no remaining files

5. Add optional config:
   - custom commit messages
   - toggle auto-push after each internal phase

6. Optional: add hook installer command (future):
   - e.g., `po install-hooks` to run similar checks on `pre-push` even outside `po save`.

---

## Final Verdict

`po` is currently in a **solid functional MVP state** for its two core workflows (`save`, `sync`), and your requested `.gitignore` safety behavior is now integrated into `save` with meaningful sequencing and automation.

The biggest remaining gaps are mostly around **hardening and polish** (tests, previews, edge-case parsing), not missing core logic.

