---
id: SCENARIO-02
status: done
---

# SCENARIO-02: status describes the store and the checks sync ran

Size verdict: OWNS A RUN (one `When`, L per sizing; introduces the read side, no seam worth a run of its own).
Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched: read-only open is existing `duckdb.OpenReadOnly`; lockdown is SCENARIO-12)
Acceptance test: `cmd/quarry/run_status_test.go` `Test_run_status_describes_the_store_sync_built`
Narrow loop: `go test ./internal/store/... ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run 'Status|Phrase|Age|Taken|SnapshotID|Help'`
Mutation checks: exactly-one-row check in duckstore `Status` (accept any count) -> `Test_status_refuses_a_store_without_exactly_one_import_run`; deferred Close in `Status` (drop it) -> `Test_status_closes_the_connection_on_success_and_on_a_query_fault`; age bucket bounds (`<` -> `<=` at 1 min / 60 min / 48 h) -> `Test_snapshotAge`
Port naming: `report.Store` + `report.WithStore`, not `Reader`/`WithReader` from the sizing note - the clean-architecture skill names every persistence port `Store` and forbids `Reader`. Orchestrator may overrule before dispatch; the change is a rename.
Copy pending (product-vision, before dispatch): Snapshot line when `snapshot_taken_at` is NULL; Source line when `source_path` is NULL; status's refusal when `$HOME` is unset (`errHomeDirectory` says "run quarry sync again"). Tests named below take their expected text from `## Surface & Copy` once ruled; do not invent it.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_status_test.go` `Test_run_status_describes_the_store_sync_built` - HOME tempdir; v9fixture bundle with transactions on several dates, a never-reconciled account, a brokerage account, one paired and one one-sided transfer (pattern `run_store_info_test.go:21-40`); `run(sync --quicken)` then `run(status)`; exact stdout equality: Store/Snapshot/Source abbreviated via `abbreviated`, id via `snapshotID`, taken time = manifest `taken_at` `.In(time.Local)` formatted `2006-01-02 15:04 MST` + `(just now)`, Dates, Rows, Balances, Splits, Transfers; stderr empty; exit 0. Red: `unknown command "status"`.

### Build
- [x] Step 2: `internal/store/store.go:133` (after `ImportRun`) new driver-free `Status` - store path, format version, quarry version, built-at, the `ImportRun` read back, first/last transaction date (absent when no transactions). Doc: zero `TakenAt`/empty `Source` mean NULL.
- [x] Step 3: `internal/store/duckstore/duckstore.go:52-95` read seam + new `internal/store/duckstore/status.go` `(*Store).Status` - small read-DB port (`QueryRows`, `Close`) with `*duckdb.DB` guard, `WithOpenReadOnly` option defaulting to `duckdb.OpenReadOnly`; one unexported `(*Store).openRead` every read method goes through (S12/S15 extend it); one query over `store_info` x `import_runs` + transactions min/max date; exactly one row else error; NULL nullable columns -> zero values; Close deferred on every path. Tests (`status_test.go`): `Test_status_reads_back_what_replace_wrote` (round trip, `duckstore_test.go:23-95` rows); `Test_status_reads_null_taken_at_and_source_as_zero`; `Test_status_reports_no_dates_for_a_store_without_transactions`; `Test_status_refuses_a_store_without_exactly_one_import_run` (0 and 2 ImportRuns via `Replace`, 1 as control); `Test_status_fails_on_a_missing_store_without_creating_it` (no `platform/duckdb` test covers this - establishes it); fakes via `WithOpenReadOnly`: `Test_status_returns_the_open_fault`, `Test_status_closes_the_connection_on_success_and_on_a_query_fault`.
- [x] Step 4: new `internal/report/{doc.go,report.go,store.go}` - `Server`, `Option`, `WithStore`, `WithHome`, `NewServer`, `(*Server).Home`, `(*Server).Status(ctx)`; `Store` port (`Status(ctx) (store.Status, error)`); exported `SnapshotID(path string) string` (basename minus `.sqlite`; S15's R2 builder reuses it). Tests (`status_test.go`, hand-written fake Store, no memory adapter - read-only port): `Test_status_returns_what_the_store_reads`, `Test_status_returns_the_store_fault`, `Test_SnapshotID`.
- [x] Step 5: `internal/cli/render.go:86-205` phrase refactor to counts - `transfersPhrase(paired, oneSided int)`, `balancesPhrase`/`balancesExtrasPhrase` from checked/never-reconciled/investment counts, `splitsPhrase(checked int)`; update callers :92-94, :101, :168, :358, :367; sync output byte-identical (existing cmd tests are the guard). Update tables `render_internal_test.go:220-348`.
- [x] Step 6: new `internal/cli/render_status.go` `renderStatus(st, home, now)`, `snapshotTakenPhrase(now, takenAt)`, `snapshotAge(now, takenAt)` - floor rounding; negative age = `just now`; Splits checked = `Run.Counts.Transactions`; Transfers via `transfersPhrase` only (no `?` rows); zero-transactions `no transactions`; NULL forms per ruling. Tests (`render_status_internal_test.go`): `Test_snapshotAge` (0, 59s | 60s, 1 min singular, 59m59s | 60m, 1 h singular, 47h59m | 48h, negative), `Test_snapshotTakenPhrase` (expectations via `time.Local`), `Test_renderStatus` (full block; no transactions; one-sided 0; `none`; investment clause; NULL taken_at; NULL source).
- [x] Step 7: `internal/cli/run.go:11-40` widen once - `ReportFactory func(ctx) (*report.Server, error)`; `Env{Stdin io.Reader; Stdout, Stderr io.Writer; NewServer ServerFactory; NewReport ReportFactory}`; `Execute(ctx, args, env Env)` also calls `root.SetIn`. `root.go:7-26` passes env through, adds `newStatusCommand`; root Long `root.go:15-17` verbatim from `## Root Long`. New `internal/cli/status.go` - Short/Long verbatim; RunE: factory -> `Status` -> `renderStatus(..., time.Now())` -> write; factory/Status errors as `&runtimeError`; no Args validator (S18).
- [x] Step 8: `cmd/quarry/run.go:21-26,40-67,78-99` - guard `_ report.Store = (*duckstore.Store)(nil)`; shared store-dir helper; `newReportFactory(storeOpts ...duckstore.Option) cli.ReportFactory` (home -> `duckstore.New` -> `report.NewServer`; never MkdirAll); `defaultEnv(stdout, stderr) cli.Env` (Stdin `os.Stdin`); `runWith(ctx, args, env cli.Env)`; `run` signature unchanged. Update `run_store_faults_test.go:135-136` to `defaultEnv` + override. Tests: `Test_run_status_fails_without_creating_a_store_when_none_exists` (exit 1, stdout empty, stderr starts `quarry: ` - S15 pins R1; neither `quarry.duckdb` nor the `quarry/` dir created); `Test_run_status_refuses_when_home_is_unset` (pattern `run_usage_test.go:171-190`, copy per ruling); `Test_run_help_prints_quarrys_description` (root `--help` carries Root Long and lists `status`).
- [x] Step 9: `docs/adr/003-duckstore-owns-the-read-side.md` (new; `adr` skill) - duckstore owns the read-only open, the queries and (later) the format check; `report` declares the `Store` port; read values in `internal/store`; `openRead` is the one door. Link from ADR-001 Consequences.

### Sweep
- [x] Step 10: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `doc.go` of `cli` (:1-4, now report.Server too) and `duckstore` (read side); doc comments on new exported symbols within budget.

### Verify
- [x] Step 11: full verification per `.claude/rules/agent-briefs.md`; `go list -deps -test ./internal/cli ./internal/report | grep duckdb` prints nothing; `.claude/scripts/spec-check.py phase2a-read-foundation` -> tick SCENARIO-02 with its acceptance test.

## Handoff

**Binding decisions**:
- All Phase 2 read commands live in `internal/report`; its port is `report.Store`, implemented by the one `*duckstore.Store` (guard in `cmd/quarry/run.go`). New read methods (S04 `Accounts`, S08 query) go on both, through `(*duckstore.Store).openRead` - S12 lockdown and S15 R1-R3 land there once, not per command.
- `cli.Env` + `Execute(ctx, args, env)` + `runWith(ctx, args, env)` + `defaultEnv` + `newReportFactory(storeOpts...)` are the widened seams; later runs add fields/options, never re-touch callers. `run()` wires `os.Stdin`, so S18's stdin tests go through `runWith` with `env.Stdin` overridden.
- `store.Status` carries everything `status --json` needs (S03 renders only): `Run` is the `import_runs` row as `store.ImportRun`; splits checked = `Run.Counts.Transactions` (import_runs has no splits_checked; `importer/validate.go:162`); snapshot id = `report.SnapshotID(Run.Snapshot.Path)` (S15's R2 reuses it).
- Phase 1 phrase functions take counts, not check slices; `status` and `sync` share them.
- Age: floor per bucket; negative age renders `just now`.

**Left unbuilt**:
- `status --json` (S03 - prints the human block until then); U8 `status takes no arguments` (S18 - extra args accepted until then); U9 matched-path hint (S19 - `status --bogus` hints `quarry sync --help` until then); R1-R3 (S15 - a missing/format-1/corrupt store exits 1 with the wrapped driver error, stdout empty, nothing created); lockdown (S12).

**Traps**:
- A cmd-level `OpenReadOnly` after `status` cannot detect a leaked connection (same config, InstanceCache allows it); only the fake-DB Close test proves Close.
- `QueryRows` with zero rows leaves zero values - hence the exactly-one-row check.
- `Dates` are DuckDB DATEs: format as-is (`2006-01-02`), never `.In(time.Local)` (west of UTC shows the day before). Only `taken_at` converts to local.
- `t.Setenv("TZ")` does not reset `time.Local`; build expected times with `time.Local`.
- If `Test_status_fails_on_a_missing_store_without_creating_it` shows DuckDB creating the file, stop and report: that is a write-safety fault for S15's `os.Stat` guard, not something to paper over.
