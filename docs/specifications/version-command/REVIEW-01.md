# Review Report — round 1

### Target
260d0d29f940be7890624936e5a3e27a59c525cf..HEAD (version-command + release-please config + README).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

### Skipped reviewers
- api-reviewer: no HTTP files
- pipeline-reviewer: no `.claude/**` files

### Gate inputs
- uncovered-diff: 0 uncovered added lines
- test-stats: cmd/quarry 991 (+10), internal/cli 704 (+10), internal/version 1 (+1), TOTAL 1696 (+21)
- mutation-sample: 15 sampled of 15 — 14 killed, 1 survived (equivalent: `internal/version/version.go:80` `setting` absent-key ok), 0 non-viable, 0 timed out; 469s
- spec-check --run: OK

### BLOCKER
none

### MAJOR
none

### MINOR
- correctness-reviewer: `internal/cli/root.go:23` — root RunE ignores args; `quarry --version -- extra` prints the version rc=0 while `quarry --version extra` refuses rc=2. Spec has no ruling on `--`.
- refactor-advisor: `internal/version/version.go:36-65` — `Resolve` mixes settings parsing and display rule (Compose method).
- refactor-advisor: `cmd/quarry/run.go:84-88` — `buildVersion` is a one-line pass-through to `version.Resolve(info).Version`.
- test-reviewer: `internal/cli/version_test.go:150-152` — wrap-width loop measures the test's own constant; proves nothing.
- test-reviewer: `internal/cli/version_test.go:251-273` — token-decoding read-back test is logic-in-test and redundant with byte-exact pins.

### NIT
- refactor-advisor: `cmd/quarry/run.go:53-55` `newServerFactory` doc lost "storeOpts go to the store"; `defaultEnv`/`envFor` doc wording.
- refactor-advisor: `internal/cli/version.go:13-27` Long text doesn't name the `(devel) <rev>` form (ruled copy — final pass).
- test-reviewer: `errVersionDiskFull` duplicates `errNoSpace`; consts mid-file; `Test_Resolve` name lacks behaviour.

### Strengths
- One owner for the version rule; `info` read once and passed down (arch).
- Real binary run through every ruled Refusal and Regression pin matched the spec (correctness).
- Byte-exact equivalence of `--version` vs `version` across flag orders; MCP wiring pinned through `envFor` (test).

### Verdict: PASS WITH FOLLOW-UPS
