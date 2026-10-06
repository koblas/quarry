---
id: SCENARIO-01b
status: open
---

# SCENARIO-01b: Summary of last month

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_summary_test.go` `Test_run_summary_prints_last_months_summary`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_summary_test.go` `Test_run_summary_counts_a_type_without_a_balance_on_the_first_month_end_as_zero_in_the_change`
Acceptance test (SCENARIO-08, folded): `internal/cli/summary_test.go` `Test_summary_findings_row_names_the_last_sync`
Acceptance test (SCENARIO-09, folded): `cmd/quarry/run_summary_refusals_test.go` `Test_run_summary_refuses_a_month_it_cannot_summarize`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_summary_test.go` `Test_run_summary_lists_cad_and_usd_separately_with_native`
Acceptance test (SCENARIO-15, folded): `cmd/quarry/run_read_refusals_test.go` `Test_run_read_commands_refuse_when_there_is_no_store`
Narrow loop: `go test ./internal/cli/ ./cmd/quarry/ -run 'ummary|read_commands|currency|usage_hint|run_help_prints|each_report|reads_the_config'`
Mutation checks: `cmd.Flags().Changed(currencyFlagName)` arm of the config failure in `summaryChoices` (W2, not refusal) → `Test_summary_warns_and_prints_when_the_config_is_unreadable_and_currency_is_given` | `IgnoreKnown` guard on the ignored clause in `summaryFindingsPhrase` → `Test_summary_findings_row_names_the_last_sync` (row `Ignored>0, IgnoreKnown=false`)
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 0 feature packages (internal/cli + cmd/quarry tests; `report.Server.Summary` exists from 01a)

Contract: `quarry summary [--month YYYY-MM] [--currency CAD|USD|native]` → text on stdout, exit 0; W1 config warnings / W2 on stderr **before** stdout (status.go:41 precedent — default, pending ruling for W2's position). Refusals per spec *Exit codes and refusals*. Check order as implemented: Args (positional, then `--currency` value, cobra) → `--month` → config → store; reading the spec's "config/currency" as the config-file currency (currency_test.go:150 precedent puts the flag value in Args).

**Deviations (orchestrator: rule before dispatch):** (1) `--json` is a root persistent flag and cannot be unregistered; until S12, `summary --json` → `UsageError` `summary --json is not available yet`, exit 2 — interim copy, default, pending ruling, never ships (S12 deletes it and `Test_summary_refuses_json_until_its_document_exists`). (2) A nil Change value renders `noRateCell` now (ruled copy, spec :158/U5) — otherwise the nil branch is untested or panics; S14 still owns the no-rate matrix, W4–W6 and JSON null.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_summary_test.go` (new) `Test_run_summary_prints_last_months_summary` — `replaceStore` fixture (run_status_findings_test.go / run_networth_history_test.go precedents): snapshot taken 2026-10-01, one Sept anomaly, a monthly series whose 3rd charge is in Sept, balances Aug 31 + Sep 30, open/new/fixed findings; `spendEnvAt` 2026-10-06, zone pinned as run_status_test.go:49; `env.NewServer` fails the test if called (no Quicken path resolved, spec :316); whole stdout pinned verbatim, stderr empty, exit 0
- [x] Step 2: `internal/cli/summary.go` (new) `newSummaryCommand(newReport, loadConfig, now, jsonOut)` — Use/Short only, RunE returns nil; registered at `root.go:37` after anomalies; `internal/cli/fakes_test.go:17-37` `fakeReportStore` gains `summary store.Summary` + `gotSummary` and a `Summary` method (Status stays nil-embedded so any second read panics)

### Build
- [ ] Step 3: `summary.go` command — Short/Long/Example/flags verbatim from spec; `Args: currency.args`; `--month` via `report.ParseMonth` (`*string` only when Changed) → `UsageError{msg: err.Error()}`; `summaryChoices` (status.go:55-63 + currency.go:73-107 shape): config always loaded once; failure → W2 `document.CannotTellChoices(config.Problem(err))` when `--currency` given, else `runtimeError`; W1 via `printConfigWarnings`; currency = flag or `cfg.Currency`; ignore + `classificationOf(cfg)`; `openReport` → `srv.Summary` → `runtimeError`; one `now()` read feeds ParseMonth and the renderer; `--json` interim refusal. Tests `internal/cli/summary_test.go`: help Long pinned at its wrap, Example, both flag lines (`report_help_test.go:182-197` row for --currency); `Test_summary_warns_and_prints_when_the_config_is_unreadable_and_currency_is_given` (W2 line, stdout, no ignored clause, exit 0); config loaded once with the flag; unreadable config without flag → runtime error; `gotSummary` Month/Currency; Server.Summary error → not a UsageError; `failingWriter` → `cannot write the result to stdout`; `report_clock_test.go:27` `executeAtAdvancingClock` row `Test_summary_reads_the_clock_once`; `Test_summary_refuses_json_until_its_document_exists`
- [ ] Step 4: `internal/cli/render_summary.go` (new) `renderSummary(report.Summary, document.FindingsTally, now)` — heading (`Month.Name()`, since/until, `, amounts in <CUR>` unless native, `windowCaption` render_table.go:54-61 precedent); Snapshot/Dates via `snapshotLine`/`datesLine` (render_status.go:80-88, 99-106), `%-10s`; `summaryFindingsPhrase` (new composer; `findingsPhrase` render.go:138-164 untouched); sections: `renderAnomalies` verbatim, recurring through a title parameter split out of `renderRecurring` (render_recurring.go:71-90; caller recurring.go:84 and render_recurring_internal_test.go unchanged in output — anchor by grep, gopls is rooted elsewhere), `renderNetWorthHistory` (Change rows in Step 5); one blank line between sections, exactly one trailing newline. Tests: `Test_summary_findings_row_names_the_last_sync` table, one arm per variable: none open / N open (1,234 thousands), ignored J>0 known, J>0 with `IgnoreKnown=false` (no clause), J=0; new+fixed, new only, fixed only, neither; run-findings clause only when open>0; heading CAD / USD / native; Snapshot `taken_at` zero row (existing copy); `cmd/quarry` `Test_run_summary_findings_count_a_classified_investment_account_and_an_ignored_id` (real `findings.ignore` id and a config classifying an investment account, vs status's counts on the same store)
- [ ] Step 5: `render_networth_history.go:11-34` extract header/rows so the summary appends Change rows to the same table; new `signedMoney(*big.Int)` (`+6,186.43`, `-219.60`, `0.00`, nil → `noRateCell`) beside `signedTenths` (render_recurring.go:59). Converted: `Change` in Month end column, cell per `Types()`, signed total (`Totals[0]`); native: one `Change` row per `Change.Totals` currency, Currency column filled, blank cell for an absent type (U9/U10); `Change == nil` → no row (S06 adds its line). Tests `render_summary_internal_test.go`: `signedMoney` positive/negative/zero/≥1,000/nil; converted all types, one nil type cell; native CAD+USD, USD on end day only (`+1,500.00` both cells), type present in one currency only; `cmd/quarry` folded `Test_run_summary_counts_a_type_without_a_balance_on_the_first_month_end_as_zero_in_the_change` (account opened in Sept) and `Test_run_summary_lists_cad_and_usd_separately_with_native` (each section CAD and USD apart, one Change row per currency); `Test_run_summary_anomalies_section_equals_quarry_anomalies_of_the_month` (byte-equal to `anomalies --since 2026-09 --until 2026-09` on the same store)
- [ ] Step 6: refusals + all-commands rows — `cmd/quarry/run_summary_refusals_test.go` (new) folded `Test_run_summary_refuses_a_month_it_cannot_summarize` (Outline rows + `2026-13`, `""`, `2026`, control `2026-09` accepted; stdout empty, exit 2); `Test_run_summary_checks_arguments_then_month_then_config_then_store` (`--currency EUR --month 2026-9` → currency line; `--month 2026-10` + malformed config + no store → month line; malformed config + no store → config line, exit 1; bad `reporting.currency` → existing line); `--month` with no value → cobra line; `Test_run_summary_refuses_a_store_built_by_an_older_quarry` (run_read_refusals_test.go:168-180). Rows: run_read_refusals_test.go:45-62 `summary` (S15), :87-110 `summary`, :236-247 `summary interrupted`; run_read_usage_test.go:16-51 `summary takes no arguments`, :83 list; run_usage_test.go:212 list, :248-275 unknown flag row; run_status_test.go:145-162 `  summary     Summarize a month: …` between status and sync; internal/cli/currency_test.go:24 `genericCurrencyCommands` (so :211 loop 1 and :318 apply), :150-160 row `--month 2026-9`, :378 row `summary --month 2026-9`. **Not** in currency_test.go:31 `flagSkipsConfigCommands`, :247 `configAlwaysReadCommands`, nor run_config_test.go:238 `readCommandArgs` (its :366 asserts empty stderr; summary prints W2)

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new symbols; `root.go:7-10` doc comment adds summary; `docs/initial-prd.md:171` CLI row after anomalies (spec *Changes to existing surfaces*, verbatim)

### Verify
- [ ] Step 8: full verification + `spec-check.py phase4f-summary` → tick SCENARIO-01b and 05, 08, 09, 13, 15 ("delivered by SCENARIO-01b" before the test reference); rewrite `STATE.md`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `renderSummary` renders every section from the one `report.Summary` the command reads; S06/S10/S12/S14 extend it and its warnings list, never add a second Server read — U1, and the fake's nil `Status` panics on one.
- Summary is a third config class: loads the config always (ignore list, classification) but refuses only without `--currency`; with it, W2 and `IgnoreKnown=false`. S12's JSON `findings.ignored` null comes from the same `IgnoreKnown`; S16 mirrors it.
- One `now()` read per run feeds `ParseMonth` and the Snapshot age; S10's `covers_month` and W3a must use that same value.
- Change rows are appended to the net-worth history table (shared column widths); `Change == nil` prints no Change row — S06 adds `No change shown: …` after the table.
- W1/W2 print before stdout; S10/S14's W3–W6 go after stdout through `emitReport`, in the spec's order.

**Left unbuilt** — named so nobody assumes it exists:
- Empty-section lines `No unusually large charges.`, `No new recurring charges.`, `No account has a balance on …`, `No change shown: …` — SCENARIO-06
- `covers_month`, W3a/W3b — SCENARIO-10; `document.Summary`, `renderSummaryJSON` (replaces the interim `--json` refusal) — SCENARIO-12; W4–W6 and the no-rate matrix — SCENARIO-14; `monthly_summary` — SCENARIO-16; SKILL/README/reference/PRD Decisions + MCP row — SCENARIO-16/17

**Traps** — things that look right and are not:
- `spendEnv`'s clock is 2026-09-29, so the default month there is August and `--month 2026-09` is refused as not ended.
- Joining `configAlwaysReadCommands` or `readCommandArgs` makes existing tests (currency_test.go:268, run_config_test.go:366) fail for a correct summary.
- A CLI-level "no ignored clause on W2" assertion is vacuous: an unreadable config already leaves `Ignored` 0; the arm is pinned with `Ignored>0, IgnoreKnown=false`.
- The root help pin (run_status_test.go:145) goes red from run A until Step 6.

**Orchestrator rulings 2026-10-06 (pre-dispatch):** (1) interim `summary --json is not available yet` (UsageError, exit 2) until SCENARIO-12 deletes the guard and its test — never ships; (2) nil Change cell renders `no rate` now (ruled copy, U5); S14 keeps the matrix, W4–W6, JSON null; (3) W2 prints before stdout (status.go:41 precedent); (4) `--currency` value refused in cobra Args before `--month` (currency_test.go:150 precedent) — the spec's "config/currency" step means the config file's currency.

## Phase report

Run A (steps 1-2) done; commit follows start ed61870.

Files:
- `cmd/quarry/run_summary_test.go` (new): `summaryRows(kioskFixed)`, `seedSummaryStore` (two `replaceStore` builds so the second build has 1 new, 1 newly fixed, Shell carried), `Test_run_summary_prints_last_months_summary`. Whole stdout pinned; anomalies/recurring/net-worth sections composed from the existing test oracles `anomaliesTable`, `recurringTable`, `netWorthHistoryLine`; `env.NewServer` errors the test if called; zone pinned with `pinLocalZone`; clock `summaryClock` 2026-10-06 12:00Z.
- `internal/cli/summary.go` (new): `summaryCommand` const + `newSummaryCommand` stub (Use/Short, RunE returns nil; params unused until Step 3). Registered in `internal/cli/root.go` before `search`, after `anomalies`.
- `internal/cli/fakes_test.go`: `fakeReportStore` gains `summary store.Summary`, `gotSummary *store.SummaryParams`, `Summary` method.

Red (expected, assertion): `Test_run_summary_prints_last_months_summary` fails at run_summary_test.go:99 `assert.Equal` — expected the full text, actual `""` (stub prints nothing, exit 0).

Fixture facts verified against the real commands on this store (scratch test, deleted): `status` Snapshot/Dates/`Findings 2 open`, `status --json` findings open 2 / new 1 / newly_fixed 1, `anomalies --since 2026-09 --until 2026-09` (2 charges checked), `recurring` Crave row, `networth --since 2026-08 --until 2026-09` rows. Expected Change row `+588.00 / -22.59 / +565.41` is hand-derived (not yet producible).

Next runs must not redo: do not change the fixture amounts. Shell 10.00 / Kiosk 11.00 / Pharmacy 12.00 differ on purpose: equal amounts on nearby days raise `duplicate` findings (the first draft had 5 open). Snapshot age prints `4 days ago` (now - taken_at = 4d 22h). The default-month `NewServer` guard is a t.Error, not t.Fatal.
Root help pin (run_status_test.go:145) is expected red until Step 6; `go test ./internal/cli/` is green now.
Not run: golangci-lint (Sweep); the stub's unused parameters go away in Step 3.
