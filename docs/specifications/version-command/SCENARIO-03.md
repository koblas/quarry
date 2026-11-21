---
id: SCENARIO-03
status: done
---

# SCENARIO-03: The root --version flag prints the same line

Cadence: code-first (no mandatory test-first item: no write-safety guard, atomic adapter or bug fix)
Acceptance test: `internal/cli/version_test.go` `Test_version_flag_prints_the_same_bytes_as_the_version_command`
Narrow loop: `go test ./internal/cli/ -run 'Test_version' && go test ./cmd/quarry/ -run 'Test_run_version|Test_run_help|Test_run_rejects_usage_errors'`
Mutation checks: delete the root `RunE` version branch; make `--version` persistent
Runs: L | V
Size: LIGHT — 3 steps, internal/cli

Surface & Copy delivered: root flag help line (Flags block in `Test_run_help_prints_quarrys_description`, `cmd/quarry/run_status_test.go`); refusal rows (`Test_run_version_flag_refusals`, exact stderr, exit 2); `--version --help` (`Test_run_version_flag_with_help_prints_the_root_help`); regression pins bare `quarry` / `quarry --json` (`Test_run_root_without_a_subcommand_prints_the_help_it_always_has`) and `quarry bogus` (`Test_run_root_refuses_an_unknown_command_naming_it`); stdout write failure in the `--version` arm (`Test_version_flag_reports_a_failed_stdout_write`).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `Test_version_flag_prints_the_same_bytes_as_the_version_command` in `version_test.go` (table over build infos; `--version` == `version` bytes; `--version --json` and `--json --version` == `version --json`). Red: root today fails on unknown flag.

### Build
- [x] Step 2 (batch 1): `root.go` local non-persistent `Bool("version")`, root `RunE`: flag set -> `emitReport` via a shared helper with `newVersionCommand`, else `cmd.Help()`; `Args` nil; rewrite `root.go:7-14` comments. Tests: write-failure in `--version` arm (`version_test.go`); `cmd/quarry/run_version_test.go` refusals, help-flag line, regression pins (bare, `--json`, `--version --help`, `bogus`).

### Sweep
- [x] Step 3: lint `0 issues`, doc comments.

## Handoff
- Root `--version` is local to root and `RunE` stays the only root behaviour: `Args` nil keeps cobra's legacy unknown-command refusal.

## Phase report
Run L done (start 3555c80a). Files:
- `internal/cli/root.go`: `showVersion` local bool, `root.Flags().BoolVar(&showVersion, "version", ...)` (local, non-persistent), root `RunE`: flag set -> `printVersion(cmd, env.BuildInfo, *jsonOut)`, else `cmd.Help()`; `Args` nil; doc comment and `Args`-nil comment rewritten.
- `internal/cli/version.go`: `printVersion(cmd, info, jsonOut)` extracted; `version` and root both call it (one emit path).
- `internal/cli/version_test.go`: +`Test_version_flag_prints_the_same_bytes_as_the_version_command` (acceptance: `--version`, `--version --json`, `--json --version`), `Test_version_flag_reports_a_failed_stdout_write`.
- `cmd/quarry/run_version_test.go`: +`Test_run_version_flag_refusals` (4 rows), `..._prints_the_version_line_with_exit_0`, `..._with_help_prints_the_root_help`, `Test_run_root_without_a_subcommand_prints_the_help_it_always_has` (bare, `--json`), `Test_run_root_refuses_an_unknown_command_naming_it`.
- `cmd/quarry/run_status_test.go`: root help pin gains the Flags block (`-h, --help`, `--json`, `--version   print quarry's version`).
Red (before code): acceptance failed at `require.NoError`, `unknown flag: --version; Run 'quarry --help' for usage.` Cobra dispatch for bare/bogus unchanged; the regression and refusal pins were green on arrival (they pin existing behaviour the new root RunE could have changed).
Mutations (all red, restored byte-identical): `if showVersion {` -> `if false && showVersion {`: acceptance x3 subtests, `Test_version_flag_reports_a_failed_stdout_write`, `Test_run_version_flag_prints_the_version_line_with_exit_0`. Flag made persistent (`root.PersistentFlags().BoolVar`): `Test_run_version_flag_refusals` before_a_subcommand / after_a_subcommand / on_the_version_command (help pin does NOT redden: root lists its own persistent flags under Flags).
Narrow loops green; `go build ./...` and `golangci-lint run ./...` 0 issues.
V must: `verify.sh 3555c80a... ./internal/cli/... ./cmd/quarry/...`, tick Sweep/Verify, tick spec SCENARIO-03 (acceptance test last on line), spec-check, STATE.md (remove root-`--version` item from Left unbuilt), `status: done`.

Run V done (start 3555c80a). Checkpoint pins applied: root-help test asserts `Usage:\n  quarry [flags]\n  quarry [command]\n`; `--version=false` row added to the bare-root help pin; `newRootCommand` doc cut to 2 lines. Mutations (all red, restored byte-identical): drop root `RunE` -> `Test_run_help_prints_quarrys_description`, `Test_run_version_flag_prints_the_version_line_with_exit_0`, `..._with_help_prints_the_root_help`; `if showVersion` -> `cmd.Flags().Changed("version")` -> `Test_run_root_without_a_subcommand_prints_the_help_it_always_has/quarry_--version=false`. verify.sh rc=0 on every step.
