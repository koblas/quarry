---
id: SCENARIO-06
status: open
---

# SCENARIO-06: Status shows rate coverage

Cadence: code-first — reads and renders only; no write-safety guard, bug fix or atomic adapter touched
Acceptance test: `internal/cli/status_rates_test.go` `Test_status_prints_the_rates_line_for_each_coverage_state`
Narrow loop: `go test ./internal/store/duckstore/ -run 'Status' && go test ./internal/cli/ -run '(?i)status'` then `go test ./cmd/quarry/ -run '(?i)status'`
Mutation checks: none (code-first)
Runs: L | V
Size: LIGHT — 3 steps, internal/store/duckstore + internal/cli

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/cli/status_rates_test.go` (new, black-box) `Test_status_prints_the_rates_line_for_each_coverage_state` — `cli.Execute` over a `statusStore` fake (status_test.go:20) with fixed `Env.Now` and `time.Local` pinned to a zone whose date differs from UTC's. Each row asserts the text line and the decoded `rates` object. Rows: age today / 1 day ago / 2 days ago; first rate after first transaction; last fetch failed; both clauses; none; none + failure. Stubs: `store.StatusRates` type + `Status.Rates` field (store.go:191-199), `newStatusCommand` gains `now` (status.go:13, root.go:29) — red at the missing `Rates` line

### Build
- [x] Step 2: store read, one query. `duckstore/status.go:16-27` statusQuery adds `(SELECT min(date) FROM fx_rates), (SELECT max(date) FROM fx_rates), r.rates_fetch_error`, scanned at :53-60 into `st.Rates` (NULL error = ""); store.go `StatusRates{First, Last, FetchError}`. v4/older store: `openRead` → `checkFormat` refuses it before the query (duckstore.go:177), so a status read of one is n/a — not a new branch. Tests (status_test.go): reads min/max fx_rates and the latest run's `rates_fetch_error`; empty fx_rates and NULL error read as zero/""; an older run's error is not read when a later run has none (`addImportRun`-style, ids 2 and 3)
- [x] Step 3: Rates line. `cli/render_status.go:28-37` `ratesLine(st, now)` after Findings: covered `USD/CAD from the Bank of Canada, <first> to <last> (<age>)` (age by civil local date of `now` vs `last`, ≤0 → `today`), `; transactions before <first> are not converted` when `Rates.First` is after `st.FirstDate` (not when no transactions), `; the last sync could not fetch new rates: <reason>`, none arm `none, so amounts are not converted; run quarry sync to fetch them from the Bank of Canada` + failure clause; `status.go:42` renders with `now()`. Acceptance goes green. Re-point `render_status_internal_test.go` full-block want, `cmd/quarry/run_status_test.go:48` want (after sync, `editStore DELETE FROM fx_rates` so the line is the none arm)
- [x] Step 4: JSON and Long. `cli/json_status.go:11-21,66-96` `Rates statusRatesDocument{first,last,fetch_error}` between findings and not_imported (null forms); `status.go:17-23` Long append `Rates shows the Bank of Canada USD/CAD rates the store holds, and why the last sync could not fetch new ones if it could not.`; pins: `json_status_internal_test.go` fullStatusJSON + null row (empty store, error set), `cmd/quarry/run_status_json_test.go:~96` want, `run_status_test.go:149` Long, each arm × `--json` in `status_rates_test.go`

### Sweep
- [ ] Step 5: `go build ./... && golangci-lint run ./...` to `0 issues`; doc comments (`Status`, `StatusRates`, `renderStatus`, `statusQuery`)

### Verify
- [ ] Step 6: full verification; `spec-check.py phase2f-fx`; tick SCENARIO-06; rewrite STATE.md (drop "Status Rates line" from Left unbuilt)

## Handoff

- Status JSON puts `rates` right after `findings` (mirrors text order), before `not_imported`; spec rules the keys, not the position.
- Age is the civil-date difference in `time.Local` between `Env.Now()` and `Rates.Last`; a last rate dated after today reads `today`.

## Phase report

Run L. Acceptance, Build 2-4 done; 5-6 left for V.

Red (before Step 3), `internal/cli/status_rates_test.go` `Test_status_prints_the_rates_line_for_each_coverage_state`:
`"Store ~ ... Findings  none open\n" does not contain "\nRates     USD/CAD from the Bank of Canada, 2026-01-01 to 2026-10-01 (today)\n"`; JSON rows: `Input ('') needs to be valid json` (no `rates` key).
Green now: that test (text and `--json`, 6 rows each), `Test_status_rates_line_edge_rows`, `Test_status_json_carries_the_fetch_error_when_there_are_no_rates`, 4 duckstore `Test_status_reads_*` rate tests, cmd `-run '(?i)status'`. `golangci-lint run ./...` already 0 issues; full suite, coverage gate, test-stats, `spec-check.py`, tick, STATE.md, `status: done` still V's.

Changed: `internal/store/store.go:191-208` `Status.Rates`, `StatusRates`; `duckstore/status.go:16-30,48-64,80` one query (`min/max(fx_rates.date)`, `r.rates_fetch_error`); `cli/status.go:13,17-25,43` `now` param + Long paragraph; `cli/root.go:29`; `cli/render_status.go` `statusRatesPhrase`, `rateAge`; `cli/json_status.go` `statusRatesDocument`, between findings and not_imported.
Tests re-pointed: `render_status_internal_test.go` (fixture Rates, full block, Contains not HasSuffix), `json_status_internal_test.go` fullStatusJSON, `status_test.go` (Env.Now), cmd `run_status_test.go` (`DELETE FROM fx_rates` so the line is the none arm: the age follows the real clock; Long pin), `run_status_json_test.go` (real rate 2026-01-02, kept end to end), `run_status_findings_test.go` (HasSuffix -> Contains).
v4/older store: n/a. `Status` reads through `openRead` -> `checkFormat`, which refuses a non-current format before the query (`open_test.go:180` `Test_open_read_refuses_a_store_of_another_format`, "an older format"); no new branch.
One read per command: the rates ride `statusQuery`; no second port call.
Do not redo: mutation checks (none, code-first). Paper mutations: dropping `.In(time.Local)` in `rateAge` reddens "last rate today" (ratesNow is 03:30 UTC, local date a day earlier); `<= 0` -> `< 0` reddens "last rate after today reads today" and the today row; `After` -> `!Before` reddens the same-date acceptance rows.
