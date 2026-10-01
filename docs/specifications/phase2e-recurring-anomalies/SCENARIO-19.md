---
id: SCENARIO-19
status: done
---

# SCENARIO-19: anomalies --json returns the anomalies document

Cadence: code-first (read-only report; no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_anomalies_json_test.go` `Test_run_anomalies_json_returns_the_anomalies_document`
Narrow loop: `go test ./internal/cli/ ./cmd/quarry/ -run 'anomal|Anomal'`
Mutation checks: `category` always the path (nil guard dropped) -> `..._category_null_for_uncategorized_and_split_charges`; payee deref without nil guard -> `..._payee_null_for_a_charge_with_no_payee`; `times` emitted as string -> acceptance + `..._times_is_a_number`; `anomalies` left nil -> `..._empty_arrays_rather_null`
Runs: L | V
Size: LIGHT — 3 steps, cli

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_anomalies_json_test.go` (new) — five earlier Bell Canada charges (2025) plus 412.00 on 2026-03-02, and an uncategorized 150.00 first-time charge in window; `anomalies --json` read back with `DisallowUnknownFields`: one entry (payee baseline, usual "96.05", earlier 5, times 4.3), `checked` 2, `not_judged` 1, `account_filter` and `warnings` `[]`

### Build
- [x] Step 2: `internal/cli/json_anomalies.go` (new) `renderAnomaliesJSON(a)`; `anomalies.go:22,45,57` takes `jsonOut *bool`, passes it with `renderAnomaliesJSON`; `root.go:34` passes `jsonOut`. Tests `internal/cli/anomalies_json_test.go` (new, fake store): every ruled key present and `[]` not null (raw map), uncategorized and split charges both `category` null, NULL payee `payee` null with `baseline` "category", money 2-decimal strings, `times` a number, no listed charge gives `anomalies` `[]`, no table on stdout
- [x] Step 3: `internal/cli/anomalies_test.go`, `recurring_test.go` — failed stdout write test for each (`failingWriter`, mirror `spend_test.go:144`)

### Sweep
- [x] Step 4 (run V): `go build ./... && golangci-lint run ./...` to `0 issues`; doc comments

### Verify
- [x] Step 5 (run V): full verification + `spec-check.py phase2e-recurring-anomalies` -> tick SCENARIO-19; rewrite STATE.md

## Handoff

`account_filter` and `warnings` stay `[]` until SCENARIO-20 (`--account`, empty-window warnings); `renderAnomaliesJSON(a, warnings)` already takes the slice S20 fills.

## Phase report

Run L done (`<start>` 4da2f1fc79fd0141ef0abddf8e9c66df2c1ecd2c). Steps 1-3 green on the narrow loop; `golangci-lint` on `internal/cli` and `cmd` prints `0 issues`. Steps 4-5 are run V's (full verify, tick, spec-check, STATE.md, `status: done`).
- Red: `Test_run_anomalies_json_returns_the_anomalies_document` failed at its decode assertion (`invalid character 'U'`, stdout was the text table).
- `internal/cli/json_anomalies.go` (new): `anomaliesDocument`, `anomalyDocument`, `renderAnomaliesJSON(a, warnings)`, `anomalyEntry`, const `tenthsPerMultiple`. `category` is null when `Category == nil` or `ExpenseSplits > 1` (two splits of one category read `(split)` in text, so null in JSON). `account_filter` is `accountFilterDocuments(nil)` -> `[]` until S20; `warnings` is the `[]string{}` the command passes.
- `anomalies.go` takes `jsonOut *bool` and calls `emitReport(cmd, *jsonOut, warnings, renderAnomaliesJSON, renderAnomalies)`; `root.go:34` passes it. `json.go`/`output.go` unreachable-comments name anomalies' `times`.
- Tests: `cmd/quarry/run_anomalies_json_test.go` (acceptance; types `anomaliesJSONDoc`, `anomalyJSON`), `internal/cli/anomalies_json_test.go` (6 tests, fake store, raw-map reads), failed-stdout-write tests `Test_anomalies_reports_a_failed_stdout_write` / `Test_recurring_reports_a_failed_stdout_write` (STATE MINOR closed).
- Mutations (all reddened, restored byte-identical): category ignores `ExpenseSplits` -> `..._null_category_for_uncategorized_and_split_charges` (`Expected nil, but got: "Fitness"`); category always nil -> `..._category_path_of_a_single_category_charge` and the acceptance test; NULL payee -> `""` -> `..._null_payee_for_a_charge_judged_against_its_category`; `anomalies` nil -> `..._empty_arrays_rather_than_null_when_nothing_is_listed`; `times,string` -> `..._times_as_a_number` and the acceptance test; `usual` from Amount -> `..._two_decimal_strings...` and the acceptance test.
- Left for S20: `account_filter` content, `warnings` content (both still `[]`).
