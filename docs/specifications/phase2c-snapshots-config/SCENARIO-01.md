---
id: SCENARIO-01
status: open
---

# SCENARIO-01: A malformed config file is refused before Quicken is touched

Cadence: code-first
Acceptance test: `cmd/quarry/run_config_test.go` `Test_run_sync_refuses_a_malformed_config_before_taking_a_snapshot`
Acceptance test (SCENARIO-04, folded): `cmd/quarry/run_config_test.go` `Test_run_read_commands_ignore_a_malformed_config`
Acceptance test (SCENARIO-09, folded): `cmd/quarry/run_config_test.go` `Test_run_sync_refuses_a_relative_quicken_path`
Acceptance test (SCENARIO-10, folded): `cmd/quarry/run_config_test.go` `Test_run_sync_refuses_a_quicken_path_that_is_not_a_string`
Narrow loop: `go test ./internal/config/ ./internal/platform/osreason/ ./internal/cli/ && go test ./cmd/quarry/ -run 'config|quicken_path|home|usage|sql'` (case-sensitive; every new cmd test name contains `config` or `quicken_path`)
Mutation checks: config load ordering in sync `RunE` (move the loader call below `resolveBundle`/`SyncAndImport`) → `Test_run_sync_refuses_a_malformed_config_before_taking_a_snapshot`
Runs: A (1-2) | B1 (3-5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 new package (`internal/config`) + `internal/platform/osreason` + cli + cmd

Contract (Surface & Copy C1–C4, C2q, C2r, verbatim; `runWith` adds `quarry: `): every refusal → stdout empty, one stderr line, exit 1, no snapshot taken. C3 → `quarry: warning: …` on stderr at load time, command proceeds, exit 0. `status`/`accounts`/`spend`/`cashflow`/`sql` never load the file. Config path = `storeDirUnder(home)/config.toml`; home from `resolveHome("sync")`.

Dependency: `github.com/pelletier/go-toml/v2 v2.4.3` (zero deps). Run B1 fetches it first: `go get github.com/pelletier/go-toml/v2@v2.4.3` **outside the sandbox** (proxy.golang.org, sum.golang.org). Run A's stubs don't import it.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_config_test.go` `Test_run_sync_refuses_a_malformed_config_before_taking_a_snapshot` — `writeStatusFixtureBundle` (`run_status_test.go:126`, lands in `~/Documents`), one good `quarry sync`, then config `[snapshots` on line 1, bare `quarry sync`: exact C1 for line 1, exit 1, stdout empty, snapshot `.sqlite` count still 1, store file bytes unchanged — count and bytes checked with `assert` before any exit-code `require`, so run A's quoted red shows the second snapshot; it is the control arm for "no new snapshot"
- [ ] Step 2: stubs — `internal/config/doc.go`, `internal/config/config.go` `Config{Path, Keep, QuickenPath, Warnings}`, `DefaultKeep`, `Load(home, path)` returning defaults; `internal/cli/run.go:13-32` `ConfigLoader func(command string) (config.Config, error)` + `Env.LoadConfig`; `internal/cli/root.go:27` pass it to `newSyncCommand` (`sync.go:35`); first call in `RunE` (`sync.go:77-81`), before `newServer`; `cmd/quarry/run.go` `newConfigLoader` (beside `:96-113`, uses `resolveHome(command)` + `storeDirUnder`) wired in `defaultEnv` `:115-126`

### Build
- [ ] Step 3: loader read + parse — `go get` (above); `internal/platform/osreason/doc.go` + `Reason(err)` (G1: `*fs.PathError` → `Err.Error()`, else first line, empty → `unknown error`) moved down from `internal/cli/sql.go:144-155` `osReason`, `sql.go:136` repointed; `config.Load`: missing / empty → defaults silently; C4 through `osreason.Reason`; C1 from a shape-agnostic decode (`*toml.DecodeError` line + message minus `toml: `, first line). Tests: `osreason_test.go` `Test_reason_*` (path error, multi-line, empty); `config_test.go` `Test_load_returns_defaults_for_a_missing_or_empty_file`, `Test_load_refuses_a_file_it_cannot_read` (directory; chmod 000 with a root skip), `Test_load_refuses_malformed_toml_naming_the_line` (line 1 `[snapshots`, error on line 3, duplicate key, integer overflow in `keep` — pin what the decode yields)
- [ ] Step 4: `config.Load` values — strict decode (`DisallowUnknownFields().EnableUnmarshalerInterface()`, `unstable.RawMessage` fields); `got` = raw text; `snapshots.keep` checked before `quicken.path`; `quicken.path` expanded with `homepath.Expand` only. Tests: `Test_load_reads_a_key_from_a_table_or_a_dotted_key` (`[snapshots] keep = 24` ≡ `snapshots.keep = 24`, `Path` set), `Test_load_refuses_a_snapshots_keep_below_one_or_not_an_integer` (1 in; 0, -1, 2.5, 12.0, `"twelve"`), `Test_load_refuses_a_quicken_path_that_is_not_a_string` (12, true, array, inline table), `Test_load_refuses_a_relative_or_empty_quicken_path` (`"Home.quicken"`, `""`, `"~"`, `"~user/x"`, `'Home.quicken'` keeps its quotes), `Test_load_expands_a_home_relative_quicken_path` (`~/…`, absolute, trailing slash kept), `Test_load_warns_about_unknown_keys_in_file_order` (`snapshot.keep`, `[foo]` once, known values still loaded), `Test_load_checks_snapshots_keep_before_quicken_path`, `Test_load_never_reports_a_shape_mismatch_as_malformed` (`snapshots = 3`, `quicken = "x"`, header table `[quicken.path]`, multi-line array — copy per the product-vision ruling; never C1, never Go type names)
- [ ] Step 5: sync wiring — `sync.go:77-124` print `cfg.Warnings` as `quarry: warning: ` lines right after load; `json.go:119-128` `renderJSON` takes the config warnings, `warnings[]` = config then `outcome.Warnings()`; `cmd/quarry/run.go` loader finished. Tests in `run_config_test.go`: `Test_run_sync_refuses_a_relative_quicken_path` (bare sync; `--quicken <bundle>` with `quicken.path = "Home.quicken"` → C2r), `Test_run_sync_refuses_a_quicken_path_that_is_not_a_string` (bare sync; `--from <id>` with `quicken.path = 12` → C2q), `Test_run_read_commands_ignore_a_malformed_config` (status, accounts, spend, cashflow, `sql`: stdout equal to the run before the file broke, stderr empty, exit 0; `spendEnv` clock), `Test_run_sync_warns_about_unknown_config_keys_before_its_own` (status fixture's one-sided warning: C3 line first then it, on stderr and in `--json` `warnings[]` without prefix). Must stay green unchanged: `Test_run_reports_exit_1_when_home_directory_cannot_be_resolved`, `Test_run_help_and_usage_errors_do_not_need_home` (`run_usage_test.go:202-225`)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go mod tidy`; doc comments on `config` package, `Load`, `Config`, `osreason.Reason`, `ConfigLoader`, `Env.LoadConfig`

### Verify
- [ ] Step 7: full verification + `spec-check.py phase2c-snapshots-config` → tick SCENARIO-01, then 04, 09 and 10 each with its own "delivered by SCENARIO-01" note before its acceptance test; STATE.md rewrite

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- TOML lib `github.com/pelletier/go-toml/v2 v2.4.3` — on SCENARIO-01's input it reports line 1 (BurntSushi v1.6.0 says line 2); `unstable.RawMessage` gives the value exactly as written for C2/C2q
- `internal/config` is a leaf package imported only by `internal/cli` and `cmd/quarry`; `snapshot` never imports it — `keep` and the configured path reach `snapshot` as values or options (05, 24, 29)
- `cli.Env.LoadConfig` (`ConfigLoader`) is the one seam; 16 and 24 call it at the top of `RunE`, after `Args`, so `--help` and usage errors need no home or config
- `config.Config`: `Keep` defaults to `DefaultKeep` (12) when unset; `QuickenPath` is `""` when unset, else `~/`-expanded only (no Clean, no EvalSymlinks), so 05 resolves it the way it resolves the flag; `Path` is the absolute config path for K1/K2 copy
- C3 lines go to stderr at load time; `warnings[]` = config warnings, then the command's own
- An unknown table warns once by its own key (`foo`), not once per child; `*toml.StrictMissingError` means warnings, not a refusal (decoded values are still filled in)
- `snapshots.keep` is validated before `quicken.path`; a refusal drops the C3 warnings
- G1 reasons come from `internal/platform/osreason.Reason`; 16 and 21 (folder unreadable, delete failure) call it rather than copy it

**Left unbuilt**:
- `quicken.path` precedence, K1/K2, sync Long / `--quicken` help — SCENARIO-05
- `snapshots.keep` consumers (`prune`, auto-prune) — SCENARIO-24, SCENARIO-29

**Traps**:
- Every `*toml.DecodeError` looks alike: a struct type mismatch (`snapshots = 3`) comes back as one naming Go types. C1 must come from a decode that accepts any shape
- `RawMessage` for a header table (`[quicken.path]`) is the table body, and for a multi-line array it has newlines — neither is a one-line `got` value
- `unstable` is go-toml's unstable API: no version bump without re-running `internal/config` tests
