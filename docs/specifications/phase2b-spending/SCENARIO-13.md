---
id: SCENARIO-13
status: open
---

# SCENARIO-13: spend counts the whole period it is given

Cadence: code-first (no bug fix, write-safety or atomicity item touched)
Acceptance test: `cmd/quarry/run_spend_window_test.go` `Test_run_spend_counts_the_whole_period_it_is_given`
Narrow loop: `go test ./internal/report/ -run 'Window|Spend' && go test ./internal/cli/ -run 'pend' && go test ./cmd/quarry/ -run 'Test_run_spend'`
Mutation checks: month end +1 (`2024-02` until -> 02-28, or Dec -> Dec 30) -> `Test_parse_window_resolves_a_bare_year_or_month_to_its_last_day`; until not inclusive (`<`) -> same test + acceptance row `2024-12-31 / 2024-12-31`; S1 grammar loosened (time layouts swapped for a prefix/regexp match, `2024-1`/`24`/trailing text accepted) -> `Test_parse_window_refuses_a_value_that_is_not_a_date`; S2 `>` -> `>=` (equal days refused) -> `Test_parse_window_accepts_equal_days_and_refuses_since_after_until`; S3 `>` -> `>=` (since == today refused) -> `Test_parse_window_refuses_a_since_after_today_only_when_until_is_absent`; S2d `<` -> `<=` (until == default since refused) -> `Test_parse_window_refuses_an_until_before_the_default_since`; check order (S3 before S1, or S2 before S3) -> `Test_parse_window_checks_bad_dates_before_since_after_today_before_since_after_until`; flag read as set when empty (drop `Changed`) -> `Test_spend_refuses_an_empty_since_as_a_bad_date`; window resolved after `openReport` -> `Test_spend_refuses_a_bad_period_before_opening_the_report`
Runs: A (1) | B1 (2-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 4 Build batches, 1 feature package (report; cli + cmd wiring do not count)

## Decisions this plan makes
- **Report owns the rule and the copy** (2a `RefusalError` precedent: message excludes `quarry: `). `report.ParseWindow(since, until *string, now time.Time) (store.Window, error)` is a package func (no `Server`: it runs before `openReport`); nil = flag not given. It returns `report.WindowError{msg}`; cli wraps any error from it as `UsageError{msg: err.Error()}` (exit 2 via `cmd/quarry/run.go:139`). `report` cannot import `cli`, so this is the only way copy stays in one place for spend and cashflow.
- **`SpendRequest.Now` is replaced by `SpendRequest.Window`** (`spending.go:13-16`): the window is resolved in cli before the store opens (P2b-13), so `Spend` no longer derives it. `DefaultWindow` stays exported; `ParseWindow` builds on it (today = `DefaultWindow(now).Until`, default since = `.Since`, both in `now`'s own zone).
- Resolution: a value's first day is the window `Since`, its last day the `Until` (year -> Dec 31, month -> first of next month minus a day). Grammar is exact via `time.Parse` layouts `2006`, `2006-01`, `2006-01-02` tried in turn; no trimming.
- Check order: parse `--since` (S1) then `--until` (S1); S3 (since given, until nil, since first day > today); S2 (both given, since first day > until last day); S2d (until only, until last day < default since). Copy: S1 `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` (`%q`-quoted, as spec); S2/S2d/S3 print values as given, unquoted, per Surface & Copy.
- `cli/window.go` holds `windowFlags{since, until}` with `bind(cmd)` (the two help strings from Surface & Copy, placeholder `date`) and `window(cmd, now)` using `cmd.Flags().Changed`, so cashflow (S20) reuses it verbatim. `--by` (S4) stays checked first; the window check follows it, both before `openReport`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_spend_window_test.go` `Test_run_spend_counts_the_whole_period_it_is_given` — table over the three spec rows through `runWith`: CAD account with distinct-amount splits on 2024-01-01, 2024-12-31, 2025-01-01 (`spendRows`/`spendSplit`/`day`, `run_spend_test.go:23-75`), `env.Now` 2026-09-29; asserts the caption (`Spending 2024-12-01 to 2025-01-31 ...`) and the Total per row. Compiles today (no stubs); red at exit code 2, cobra "unknown flag: --since".

### Build
- [ ] Step 2: `internal/report/window.go` (new), `window_test.go` (new, `report_test`) `ParseWindow`, `WindowError` — grammar and resolution, nil flags = default window. Tests: `Test_parse_window_resolves_a_bare_year_or_month_to_its_last_day` (2024 -> Dec 31, `2024-02` -> 29, `2023-02` -> 28, `2024-12` -> Dec 31, day value both bounds); `Test_parse_window_refuses_a_value_that_is_not_a_date` (table: `2024-13`, `2024-00`, `2024-02-30`, `2023-02-29`, `2024-1`, `2024-01-1`, `24`, `20240101`, `2024/01`, `2024-01-01x`, ` 2024`, ``, `yesterday`; both flags name themselves in the message; control `2024-02-29`); nil/nil equals `DefaultWindow(now)` in a non-UTC zone.
- [ ] Step 3: `window.go` `ParseWindow` refusals S3/S2/S2d — Tests: `Test_parse_window_accepts_equal_days_and_refuses_since_after_until` (`--since 2024-12-31 --until 2024-12-31` ok; `--since 2025 --until 2024` refused, message with values as given; `2024-12-15` since vs `2024-12` until ok); `Test_parse_window_refuses_a_since_after_today_only_when_until_is_absent` (bounds: since == today ok, today+1 refused, `2027` message; `--since 2099 --until 2100` ok); `Test_parse_window_refuses_an_until_before_the_default_since` (bounds: `2025-12-31` refused with exact S2d copy incl. default `2026-01-01`, `2026-01-01` ok, `2027` ok); `Test_parse_window_checks_bad_dates_before_since_after_today_before_since_after_until` (bad `--until` with `--since 2099` -> S1; `--since 2099 --until 2024` -> S2 not S3; bad `--since` and bad `--until` -> the `--since` message).
- [ ] Step 4: `internal/report/spending.go:10-16,37-45` `SpendRequest`/`(*Server).Spend` — `Now` -> `Window store.Window`; update `spending_test.go:33-73` (`Now:` at the three `SpendRequest` literals become `Window: report.DefaultWindow(...)` or a literal); `Test_spend_reads_the_requested_window` (a non-default window reaches `store.SpendingParams` and comes back on `report.Spending.Window`).
- [ ] Step 5: `internal/cli/window.go` (new), `internal/cli/spend.go:13-56,68` — `windowFlags` bound next to `--by` (`spend.go:68`), `window(cmd, now())` between `parseSpendGrouping` (`:43`) and `openReport` (`:48`), result into `report.SpendRequest{Window: ...}` (`:53`). Tests in `internal/cli/spend_window_test.go` (new, `cli_test`, reuse `executeSpend` `spend_test.go:20-30`): `--since 2025-03 --until 2025-05` -> `got.Window` {2025-03-01, 2025-05-31}; only `--since` -> until today; only `--until` -> since Jan 1; refusal table S1 (each flag) / S2 / S2d / S3 asserts `cli.UsageError`, exact message, `fakeReportStore{err: errStoreRead}` never read, stdout and stderr empty; `Test_spend_refuses_a_bad_period_before_opening_the_report` (`NewReport` returns error, cf. `spend_test.go:83-100`); `Test_spend_refuses_an_empty_since_as_a_bad_date`; `--by vendor --since 2024-13` -> S4 message; help output carries both flag strings verbatim (fault: none, no fallible call added in cli beyond the parse).

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (short, no scenario ids) on `ParseWindow`, `WindowError`, `windowFlags`; `internal/cli/doc.go`/`go doc ./internal/report` need no edit unless the package summary names spend's window.

### Verify
- [ ] Step 7: full verification block + `.claude/scripts/spec-check.py phase2b-spending`; tick SCENARIO-13 with its acceptance test; rewrite STATE.md (`Left unbuilt` drops `--since`/`--until`; Spend window bullet says `SpendRequest.Window`, resolved by `report.ParseWindow`).

## Handoff

**Binding decisions:**
- `report.ParseWindow(since, until *string, now)` is the one parser for spend, cashflow (S20), month partial (S12), account (S14) and E1 (S17); it returns `store.Window` (civil days, UTC midnight, both inclusive) or `report.WindowError` (copy without `quarry: `). S12's `partial` compares a period against this Window; S17's E1 message prints `Window` dates, it does not re-parse. Callers pass a pointer only when the flag was given (`Changed`); an explicit empty value is S1, mirroring `--by ""`.
- `report.SpendRequest` carries `Window`, not `Now`: S14 adds `AccountIDs`, S20's `CashFlowRequest` takes `Window` the same way; nothing below cli reads the clock.
- `cli/window.go` `windowFlags` is the shared flag pair; cashflow binds it with the same help strings (spec: "same strings as `spend`").

**Left unbuilt:**
- cmd-level exit-2 test for S1-S4/U8 through `runWith` (`Test_run_spend_rejects_a_period_it_cannot_use`-style) — SCENARIO-12 absorbs 18 and ticks it once `--by month` exists; S13 pins the refusals at `cli.Execute` and `report` only.
- `cashflow` command and its S2d/S3 wording ("default --since") — SCENARIO-20 reuses `ParseWindow` unchanged.
- Empty-window E1/E2 — SCENARIO-17.

**Traps:**
- `time.Parse` "2006-01" needs two-digit months and rejects `2024-1` and trailing text, but accepts year `0000`: spec is silent, leave it (not a finding).
- S2d compares the until's *last* day with the default since; S3 compares the since's *first* day with today; S2 compares first day of since with last day of until. Comparing raw parse results (first day for both) makes `--since 2024 --until 2024` an S2.
- Old spend tests set `Now` on `report.SpendRequest`; they stop compiling in Step 4 — update them, do not keep a `Now` shim.
- Copy gaps (all ruled here, no product-vision run needed: each is an instance of a spec row): S1 quotes with `%q`, S2/S2d/S3 do not; empty value = S1; both flags bad reports `--since`; `--by` refusal outranks a bad window (argv order in code, spec silent).

## Phase report

Run A: PARTIAL — test written, red NOT yet observed (Bash tool's auto-mode classifier returned no verdict on every call, so `go test` never ran).
- Written: `cmd/quarry/run_spend_window_test.go` `Test_run_spend_counts_the_whole_period_it_is_given` (3 subtests over `--since/--until`; CAD Auto:Fuel splits 10.00 on 2024-01-01, 20.00 on 2024-12-31, 40.00 on 2025-01-01; `env.Now` 2026-09-29 UTC; asserts full stdout: caption, header, Auto:Fuel row, Total row; totals 30.00 / 60.00 / 20.00).
- Next run must: `go test ./cmd/quarry/ -run 'Test_run_spend_counts_the_whole'`, confirm it fails at `require.Equal(0, exitCode)` (cobra "unknown flag: --since", exit 2) and not at a compile error or a column-width mismatch; then tick Step 1. No stubs needed (compiles today). Nothing committed.
