---
id: SCENARIO-14
status: open
---

# SCENARIO-14: Missing exchange rates are warned about once

Cadence: code-first (no mandatory test-first item: no bug fix, write-safety guard or atomic adapter)
Acceptance test: `cmd/quarry/run_summary_warnings_test.go` `Test_run_summary_warns_once_per_kind_when_the_store_has_no_exchange_rates`
Narrow loop: `go test ./internal/report/document/ -run 'SummaryWarnings|NewSummary'` then `go test ./internal/cli/ -run 'summary'` then `go test ./cmd/quarry/ -run 'run_summary'` (lowercase)
Mutation checks: dedupe removed → no-rates-once row and acceptance test; W4/W5/W6 order swapped → order row (document) and cmd order pin; `unconvertedWarnings` swapped for `AnomaliesWarnings` (empty-window line leaks) → existing empty-stderr pins in `run_summary_empty_test.go`; advice hard-coded to `NativeFlag` → `NativeParameter` row; W6 call dropped → acceptance test; `rateWarnings` swapped for `NetWorthWarnings` → no-transactions empty-stderr pin (`run_summary_empty_test.go:133`)
Runs: A (1) | B1 (2-3) | V (4-5)
Size: OWNS A RUN — 2 batches (production in batch 1, ~15 lines; batch 2 is the command-level matrix), 1 feature package (report/document; cli one-line wiring and cmd/quarry tests do not count). Matches sizing; not FOLD (nothing to absorb it, S16 is another package).

Surface surveyed (grep + Read; gopls rooted at ../phase4d-registered): `SummaryWarnings` callers = `internal/cli/summary.go:104` and `internal/report/document/summary_test.go:411` only. Reused unexported builders in `document`, no new rule text: `unconvertedWarnings` (warnings.go:90-101), `chargesNoun`/`seriesNoun` (:23-25), `rateWarnings` (networth_rate_warnings.go:24-41, `NativeAdvice` consts :14-19). `Anomalies`/`Recurring` `Unconverted` count listed items only (anomalies.go:126-136; recurring.go:252-257, after `keep`), so a series not new in the month never warns; `NetWorth.Window` is always set by `Server.Summary`, so W6 always takes the `N month end(s)` form. Change cells and JSON null are built (S01a/S12): S14 only pins them at command level.

Expected values for the fixture (`summaryNativeRows`, default CAD, no rates; Hulu 150.00 USD anomaly and Spotify USD series unconvertible): Change row chequing `no rate`, credit_card `-22.59`, Total `no rate`; JSON `changes.types` chequing null, credit_card `"-22.59"`, `totals` `[{CAD, null}]`; stderr = W3b, `noRatesWarning` once, W6 no-rates line with `pass --currency native`. If the actual store output differs from this, stop and report; do not bend the assertion.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_summary_warnings_test.go` `Test_run_summary_warns_once_per_kind_when_the_store_has_no_exchange_rates` — `replaceStore(summaryNativeRows())`, `spendEnvAt(..., summaryClock)`, `quarry summary`; stdout `Contains` the USD Hulu anomaly row and USD Spotify series row, code-prefixed (precondition: W4 and W5 both fire, so the dedupe is not vacuous), and the Change row (`no rate` / `-22.59` / `no rate`), stderr equals `septemberTimeUnknownWarning` + the no-rates line once + the W6 line. `replaceStore` seeds no rates (checked: run_helpers_test.go:76, no `WithRates`). Red today at the stderr assertion (only W3b prints). No stubs needed.

### Build
- [x] Step 2 (batch 1, document + wiring): `internal/report/document/summary_warnings.go:12-20` `SummaryWarnings(s, again, advice NativeAdvice)` — after W3 append W4 `unconvertedWarnings(s.Anomalies.Currency, s.Anomalies.Unconverted, chargesNoun)`, W5 same for `s.Recurring` with `seriesNoun`, W6 `rateWarnings(s.NetWorth, advice)`; drop repeats keeping first (unexported helper, whole list); still non-nil. Call `unconvertedWarnings` directly, never `AnomaliesWarnings`/`RecurringWarnings`/`emptyNetWorthWarnings`/`leftOutWarnings`, so the empty-window and left-out lines cannot leak. `internal/cli/summary.go:104` passes `document.NativeFlag`. New `document/summary_rate_warnings_test.go` over hand-built `report.Summary` (`summaryOf`, summary_warnings_test.go:15), one case per arm: no rates, anomalies and recurring both unconvertible → one `noRatesWarning` (control: only one of them → still once); before first rate → W4 (`charge`/`charges` singular and plural) and W5 (series noun) differ, both kept, in order W3, W4, W5, W6; W6 `1 month end` vs `2 month ends` (first rate between the ends vs after both); native → no rate lines; zero `Unconverted` and no row needing a rate → none; `--currency USD` summary → currencies swapped; advice rows `NativeFlag` / `NativeParameter` in W6; empty-window anomalies (`Checked==0`), empty recurring, empty net worth, and an account left out → no line (these are the suppression pins); existing W3 rows (summary_test.go:389-415) updated for the new parameter. Acceptance goes green here.
- [x] Step 3 (batch 2, command cells): same new cmd file, one case per cell, each text and `--json` where it exists. (a) `--json` of the acceptance store: `warnings` equal the three stderr lines unprefixed, in order; `changes.types` chequing `null`, credit_card `"-22.59"`, `totals` `[{CAD, null}]`; stderr identical with and without `--json`. (b) before-first-rate with charges (`replaceStoreWithRates(summaryNativeRows(), usdRate(day(2026, time.October, 2), …))`): W4 and W5 each print (different nouns, no dedupe), W6 `2 month ends`; text and JSON. (b2) the ruled edge row "Month ends before first rate": new small store, a USD chequing balance on Aug 31 and Sep 30, no USD charge and no new USD series in September, first rate Oct 2: stderr exactly W3b then the W6 `2 month ends` line, anomalies and recurring sections normal, Change `no rate`; text and `--json` (`warnings`, null `changes` cells). (c) order with a config warning (`writeConfig(home, "colour = \"red\"\n")`, as run_summary_json_cells_test.go:133): stderr lines in order W1 (`~` form), W3b, rates lines (order within stderr; stdout is a separate buffer); JSON `warnings` = W1 absolute, W3b, rates lines. (d) order with W2 (unreadable config via `writeConfig` bad value + `--currency CAD`, as :114): W2, W3b, rates lines. (e) `--currency native` on the acceptance store: stderr is W3b only, no rate line, text and JSON. (e2) `--currency USD` on the acceptance store (CAD rows unconvertible, currencies swapped): stderr lines equal `--json` `warnings`; exact text is whatever the sibling builders print, asserted from `anomalies --currency USD` / `networth` output on the same store, not typed from memory. (f) controls already green, re-run: CAD-only `seedSummaryStore` prints no rate lines; the five empty-stderr pins in `run_summary_empty_test.go` (`septemberTimeUnknownWarning` alone) stay green. Fault: unreadable-config row (d) covers the `emitReport` path with write failure unchanged (S12 pin `internal/cli/summary_json_test.go`); no new fallible call.

### Sweep
- [ ] Step 4: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comment on `SummaryWarnings` states the slot order W3, W4, W5, W6 and that the empty-window and left-out lines are not part of it.

### Verify
- [ ] Step 5: full verification per agent-briefs.md plus `.claude/scripts/spec-check.py phase4f-summary`; tick SCENARIO-14 with its acceptance test in specification.md; rewrite STATE.md (move W4-W6/dedupe and the suppression item out of Left unbuilt; keep the NativeParameter note for S16); `status: done`.

## Handoff

**Binding decisions:**
- `SummaryWarnings(s, again, advice)` is the one list; text stderr and `--json` `warnings[]` read it, W6 advice and W3 tail are both surface parameters. S16 passes `call monthly_summary again` and `document.NativeParameter`. W4-W6 come from `unconvertedWarnings`/`rateWarnings` directly, never from the commands' `*Warnings` composers, so suppression of empty-window/left-out lines holds by construction.
- Order W1, W2 (stderr before stdout, mutually exclusive), then W3, W4, W5, W6 after stdout; repeats dropped keeping the first.

**Defaults pending ruling** (implemented as stated, orchestrator may overrule):
- Dedupe is over the whole list, not only W4/W5; in practice only the no-rates line repeats. Before-the-first-rate lines never repeat (different nouns) and both print.
- W6 shows once however many types need a rate (existing `rateWarnings`); `--currency USD` swaps the currencies in each line as the sibling commands do; native has no rate warnings.
- W4-W6 text and `--json` strings are the sibling commands' lines verbatim, with no path and no `again` tail.

**Left unbuilt:** `monthly_summary` MCP tool, its `NativeParameter` call site and caps (S16); `declaredFields` row for `summary --json` (S17); `readCommandArgs` summary row debt (STATE Open debts).

**Traps:**
- `AnomaliesWarnings` fires its empty-window line on `Checked==0` and `RecurringWarnings` on `Recurring.Empty()`: using them for W4/W5 breaks the empty-stderr pins in `run_summary_empty_test.go`.
- chargeRows-based cmd stores print W3b first; every stderr assertion in this scenario starts with `septemberTimeUnknownWarning`.
- `Unconverted.FirstRate` is zero for native, so no rate line there; do not add a native special case.

**Orchestrator rulings 2026-10-06 (pre-dispatch):** defaults 1-3 stand (dedupe over the whole list keeping the first; W4-W6 sibling lines verbatim in both formats, W6 once, none in native, USD swaps as siblings; `SummaryWarnings(s, again, advice NativeAdvice)` — S16 passes NativeParameter).

## Phase report

Run B1 (steps 2-3, start commit 747f3c6). Acceptance test green; lint 0; whole cmd/quarry, internal/cli, internal/report/... green.
- `internal/report/document/summary_warnings.go:17` `SummaryWarnings(s, again, advice NativeAdvice)`: W3, W4 `unconvertedWarnings(Anomalies, chargesNoun)`, W5 same for Recurring/`seriesNoun`, W6 `rateWarnings(NetWorth, advice)`, then `withoutRepeats` (whole list, keeps first, never nil). `internal/cli/summary.go:104` passes `document.NativeFlag`. The doc comment on `SummaryWarnings` already states the slot order and the suppression (step 4 only needs lint).
- Document tests: new `internal/report/document/summary_rate_warnings_test.go` (9 tests incl. 4 suppression rows, advice rows, USD swap, native, order, month-end count); existing W3 table in `summary_test.go:411` takes the new parameter.
- Command tests appended to `cmd/quarry/run_summary_warnings_test.go` (a, b, b2, c, d, e, e2, each text and --json). Real output matched the plan, except b2's JSON change cells read currency CAD (the reporting currency), not USD.
- Two existing tests retargeted: `run_summary_findings_test.go:72` and `run_summary_json_cells_test.go:120` ran `--currency USD` on the CAD no-rates store, which now correctly prints the rate lines after W2; they now pass `--currency CAD` (same bad-config path, no rate lines).
- Mutations run (each restored, diff clean): dedupe removed -> `Test_SummaryWarnings_says_the_store_has_no_rates_once_...` + acceptance; W4/W5 swapped and W6 moved before W5 -> order rows (document) + `Test_run_summary_warns_about_each_kind_dated_before_the_first_rate` (+ json); W4 -> `AnomaliesWarnings` -> `no_charge_checked_in_the_month` and `an_account_named_and_left_out` rows + empty-stderr pins in `run_summary_empty_test.go`; W5 -> `RecurringWarnings` -> `no_series_listed` row + empty pins; advice hard-coded `NativeFlag` -> the two `parameter` rows; W6 dropped -> acceptance; W6 -> `NetWorthWarnings` -> `no_balance_on_either_month_end` row + `Test_run_summary_prints_the_ruled_empty_lines_...`.
- Next (V): step 4 sweep (lint already 0), step 5 full covered suite + uncovered-diff, spec tick + spec-check, STATE.md rewrite, `status: done`. STATE note: `NativeParameter` is S16's; the two retargeted tests belong in STATE Traps (USD on a no-rates CAD store prints rate lines).
