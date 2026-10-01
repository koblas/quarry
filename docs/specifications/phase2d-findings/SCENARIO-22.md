---
id: SCENARIO-22
status: done
---

# SCENARIO-22: findings --csv prints one row per item

Cadence: code-first (read-only rendering; no write guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_findings_csv_test.go` `Test_run_findings_status_all_csv_prints_a_row_per_duplicate_item_and_one_row_for_the_fixed_finding`
Acceptance test (SCENARIO-23, folded): `cmd/quarry/run_findings_usage_test.go` `Test_run_findings_rejects_usage_it_cannot_use`
Narrow loop: `go test ./internal/cli/` (whole package) and `go test ./cmd/quarry/ -run 'findings'`
Mutation checks: fixed finding = one row with NULL item cells → acceptance test and `Test_findings_csv_applies_the_status_and_type_filters`; `--csv --json` refused → outline row `--csv --json` and `Test_findings_rejects_bad_usage`; nil payee is NULL not `""` → `Test_findings_csv_leaves_the_payee_of_a_no_payee_item_...`; CSV has no footer/hint → exact-stdout tests
Runs: L | V
Size: LIGHT — 3 steps, cli

Pinned (unruled, decided): `date` = `2006-01-02`; `amount` = JSON's plain string (`-1200.50`); `status` = `open|ignored|fixed`; `payee` empty → NULL (as JSON); `other_account`, `*_id` NULL when nil; `category`, `transactions`, `splits` NULL until types 24-28 fill them; `fix` = `Type.Fix().Sentence` on every row incl. fixed; cells built from `newFindingEntryDocument` (one item converter for JSON and CSV); `--csv --json` checked in `Args` after the positional check.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_findings_csv_test.go` (new) acceptance test over a synced store (Hydro One duplicate open, Netflix duplicate fixed by a second sync); stub `--csv` flag + empty `renderFindingsCSV` in `internal/cli/findings.go`, new `csv_findings.go`

### Build
- [x] Step 2: `internal/cli/csv_findings.go` `renderFindingsCSV(listing)` + `csvOptional`; `findings.go` `--csv` flag wired after `--json` renders; fold `csv.go:14-17` doc trim; tests `internal/cli/findings_csv_test.go` (rows of open/ignored/no-payee item, fixed row, header only when none, quoting, footer/hint absent, config warning on stderr, stdout write fault, report fault)
- [x] Step 3: `--csv --json` in `findings.go` `Args` (after positional); help line pin for `--csv`; `cmd/quarry/run_findings_usage_test.go` outline over the four ruled rows

## Handoff
- `findings --csv` cells come from `newFindingEntryDocument`; 24-28 that fill `category`/`transactions`/`splits` in `findingItemDocument` fill the CSV for free (`csvOptionalInt`)

## Phase report

Run L done (start b69fe46): Acceptance red quoted (stdout was the text view, not CSV), Build green.
- `internal/cli/csv_findings.go` (new): `renderFindingsCSV`, `findingCSVRows`, `findingItemCSVCells`, `csvOptional`, `csvOptionalCount`; cells come from `newFindingEntryDocument` (same converter as `--json`).
- `internal/cli/findings.go:20,67-69,96-104,116`: `--csv` flag (ruled help), `Args` conflict after the positional check, `renderText` swapped to CSV; `csv.go:14-16` doc trimmed to 2 lines (STATE debt closed).
- Tests: `cmd/quarry/run_findings_csv_test.go` (acceptance), `run_findings_usage_test.go` (outline, four ruled rows; first three were green on arrival, existing code), `internal/cli/findings_csv_test.go` (6 tests), `csv_findings_internal_test.go` (white-box: no type fills category/transactions/splits yet), `findings_test.go` (+`--csv` help row, +2 conflict rows).
- Mutations reddened: drop conflict check (cli + run rows); fixed finding emits no row (filters test, acceptance); nil payee as empty (panic in the row test, red); trailing newline (three exact-stdout tests).
- Pinned: date `2006-01-02`; amount plain `-1200.50`; status `open|ignored|fixed`; empty payee NULL; `fix` on every row incl. fixed; `transactions`/`splits`/`category` NULL until 24-28.
- V still to do: lint (`golangci-lint run ./...`), full covered suite + `uncovered-diff.py` (`csvOptionalCount` non-nil arm is covered only by the white-box test), `test-stats.py --base b69fe46`, tick SCENARIO-22 and SCENARIO-23 (delivered by SCENARIO-22) in the spec, `spec-check.py`, STATE.md rewrite (Left unbuilt: drop `findings --csv`; close the `csv.go:14-17` debt), `status: done`.

Run V done: lint 0 issues; covered suite rc=0, uncovered-diff 0 lines; -race cli + cmd/quarry ok; spec ticks (22, 23 delivered by 22), spec-check OK, STATE.md rewritten.
