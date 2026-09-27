---
paths:
  - "**/*.go"
---

- `quarry` is a single Go module rooted at the repo root (`github.com/koblas/quarry`). There
  is no nested `go/` directory, no `go.work`, no generated-code tree. Run every Go command
  from the repo root.
- All Go code should be idiomatic, with interfaces defined by the consumer to support
  testing.
- Comments are written for `go doc`: a `doc.go` per package, function docs that state the
  contract (cite the standard — `RFC 6749 §5.1`, `OIDC Core §3.1.2.1` — where the behaviour
  is one), no history or change narrative anywhere, and no in-function comments restating
  what the code already says. Full rules in `.claude/skills/clean-architecture/SKILL.md`
  (_Documentation & comments_). Check with `go doc ./internal/<pkg>`.
- **Doc comments are short.** Budget: exported func/type ~4 lines; unexported func/type
  1–2 lines; `const`, `var`, sentinel error 1 line. Over budget means the extra text is
  _how_, and _how_ belongs in the body at the line it explains
  ([go.dev/doc/comment](https://go.dev/doc/comment), _Funcs_: doc comments "should not
  explain internal details such as the algorithm used").
  - The doc says what the symbol does, returns and refuses — not how it decides.
  - **Say each fact once, where the code enforces it.** An ordering constraint is one line
    beside the literal that sets the order, not repeated on every symbol that relies on it.
  - **No spec or finding ids** (`R7`, `BR-3`, `SCENARIO-04`) — they point outside `go doc`.
    State the rule itself.
  - **Fixing a comment makes it shorter or truer, never longer.** Appending a correction to
    a wrong comment is how a doc longer than its function happens.
- Testing always uses `testify`; `suite` is optional.
- The toolchain is **Go 1.27.1** (pinned in `devenv.nix` and `go.mod`). `testing/synctest`
  is available as a stable package — use it for deterministic concurrency tests instead of
  sleeps and polling.

## Toolchain

Bash tool calls already run inside the pinned nix/devenv environment (`IN_NIX_SHELL` is
set), so `go` and `golangci-lint` are the pinned versions. Run them directly:

```bash
go build ./...
```

**Confirm what you got before trusting a result**: `go version` must report the version
pinned in `devenv.nix` (currently `go1.27.1`). A mismatch means the command ran outside the
environment — not that PATH is broken.

**Never hardcode a `/nix/store/...` path.** They go stale on every rebuild, silently pointing at a
garbage-collectable store entry after a version bump.

If a command fails with `operation not permitted`, the shell was **sandboxed**. Re-run with
`dangerouslyDisableSandbox: true` and check `echo $DEVENV_ROOT` plus `which -a go` before
concluding anything about PATH.

## Verification

From the repo root:

```bash
go build ./...
go test ./...
go test -race ./internal/<touched>/...
golangci-lint run ./...
```

**Never `| tail -N` / `| head -N` a verification command.** `$?` becomes the pipe's
status — `go build ./nonexistent 2>&1 | tail -2` reports **exit 0** for a failed build.
Never quote an exit code captured that way as proof anything passed. If you must pipe,
prefix with `set -o pipefail`.

A green `go test ./...` line is the evidence; the exit code of an unpiped command is
trustworthy (1 on failure, 0 on success).

**A green summary does not mean everything ran.** A suite that skips on a missing
precondition **skips green**. When a claim depends on one, count the skips:

```bash
go test -v ./internal/<pkg>/... 2>&1 | grep -c -- "--- SKIP"
```

Before declaring a scenario or task complete, run the full suite once, unpiped:

```bash
go test ./...
```
