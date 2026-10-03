---
id: SCENARIO-01
status: open
---

# SCENARIO-01: CLI output is unchanged after the analysis documents and warnings move to shared code

Cadence: code-first — behaviour-neutral move; no write guard, atomic adapter or bug fix touched
Acceptance test: `cmd/quarry/run_analysis_documents_test.go` `Test_run_prints_spend_cashflow_recurring_and_anomalies_byte_for_byte`
Narrow loop: `go test ./internal/store/ ./internal/platform/money/ ./internal/report/document/ ./internal/cli/ ./cmd/quarry/ -run 'pend|ash|ecurring|nomal|nconverted|FX|fx|eft|mpty|Native|Group|Period|Warning|Status|Baseline|Filter|byte_for_byte'`
Mutation checks: none (code-first). Optional, non-blocking: swap two fields of the moved recurring series struct → the `recurring --json` golden row goes red while `decodeRecurringJSON` pins stay green
Runs: A (1) | B1 (2-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 4 batches, 1 feature package (report/document; store and platform/money get one pure func each; cli is delivery)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_analysis_documents_test.go` (new) `Test_run_prints_spend_cashflow_recurring_and_anomalies_byte_for_byte` — table of setup funcs (shape of `run_shared_documents_test.go:27-93`) through `runWith` with a fixed `Env.Now` (`spendEnvAt`, `run_spend_json_test.go:30`), never `run()` (real clock drifts the default window, recurring/anomalies `Now`). Store via `replaceStoreWithRates` (`run_sql_fx_test.go:32`); charges from `monthlySeries`/`historyWithBigCharge`/`usdRate`. Each row asserts exact stdout and exact stderr with `assert.Equal` (never `JSONEq`, never decode) plus exit 0. Rows, each in `--json` AND text (text pins the `quarry: warning: ` stderr lines): (a) populated, per command — `--account` an included CAD account, an included USD account with amounts before and after the first rate, a linked-tracking account and a not-in-reports account (both left-out lines, before-first-rate line with its noun, command word `spend`/`cashflow`/`recurring`/`anomalies`); (b) empty window, per command, no `--account` (subjects `spending`/`income or spending`/`recurring charges`/`unusually large charges`; anomalies fires on `Checked == 0`); (c) `spend --by tag` with a multi-tag split. 18 rows. **Green on arrival**: write and commit it on the unmodified tree so the literals are pre-move output; no stubs. Report it green, with this reason

### Build
- [x] Step 2: shared primitives — `internal/store/store.go:484-498` `SpendingGroups()` + `(SpendingGroup) String()` and `:560-568` `CashFlowPeriods()` + `(CashFlowPeriod) String()` (words from `internal/cli/spend_grouping.go:12-17`, `cashflow.go:20-23`; today those are arrays, so an out-of-range index panics — `String()` returns `""` instead); `store_test.go` pins order (category, payee, tag, month; month, year), words, and an out-of-range row per enum. `internal/platform/money/money.go` `NativeOf` from `internal/cli/fx_warning.go:52-59` + `money_test.go` rows CAD, USD, Native. `internal/report/document/account_filter.go` (new) `AccountFilter` + `NewAccountFilters` from `json_spend.go:22-33`, test: empty → `[]`, id/name kept. cli: `spend_grouping.go:7-31` and `cashflow.go:15-37` drop `name` (keep `header`/`missing`), parse over the store lists; flag defaults `spend.go:82`, `cashflow.go:97`; `json_spend.go:89`, `json_cashflow.go:62` use `.String()`; `fx_warning.go:66` uses `money.NativeOf`; all four `json_*.go` call `document.NewAccountFilters`; cli copies deleted
- [x] Step 3: warnings — `internal/report/document/warnings.go` (new) from `internal/cli/empty_window.go:9-74` (whole file deleted), `fx_warning.go:13-50` (`noRatesWarning`, `countedNoun`, nouns, `unconvertedWarnings`, `beforeFirstRateWarning`; `accountsFXWarnings` stays) and the composers `spend.go:88-100`, `cashflow.go:103-111`, `recurring.go:90-98`, `anomalies.go:85-93` → exported `SpendingWarnings`, `CashFlowWarnings`, `RecurringWarnings`, `AnomaliesWarnings`, each `(result, word string)`; helpers stay unexported. RunE sites `spend.go:74`, `cashflow.go:91`, `recurring.go:79`, `anomalies.go:72` pass `spendCommand`/`cashFlowCommand`/`recurringCommand`/`anomaliesCommand`. Move `fx_warning_internal_test.go:14-86` to `document/warnings_test.go`; `:87-154` stays. Document tests per composer: word appears only in the linked-tracking and not-in-reports lines (pass a non-CLI word, e.g. `cash_flow`); empty-window arms (unnamed, named, no transactions, all-named-left-out suppresses it); multi-tag only for `SpendByTag` with count > 0; anomalies empty on `Checked == 0` only
- [x] Step 4: spend + cashflow documents — `json_spend.go:9-111` (less account filter) → `document/spending.go` `Spending` + row types + `NewSpending(s, warnings)`; `json_cashflow.go:6-69` → `document/cashflow.go` `CashFlow` + `NewCashFlow(c, warnings)`; warnings via `append([]string{}, warnings...)` like `sql.go:44`. cli `renderSpendingJSON`/`renderCashFlowJSON` stay as `marshalDocument(document.NewX(...))`. Move `json_spend_internal_test.go` (5 tests, :25-215) and `json_cashflow_internal_test.go` (3 tests, :22-118) to `document/spending_test.go`, `cashflow_test.go` with a 2-space-indent test helper (same encoder as `internal/cli/json.go:135-140`) so the literals carry over; plus a read-back test per document (stdlib decode, row count and values equal input)

- [ ] Step 5: recurring + anomalies documents — `json_recurring.go:7-124` (incl. `recurringCadences`, `tenthsPerPercent`) → `document/recurring.go` `Recurring` + `NewRecurring(r, warnings)`; `json_anomalies.go:7-92` (incl. `tenthsPerMultiple`) → `document/anomalies.go` `Anomalies` + `NewAnomalies(a, warnings)`. `render_recurring.go:23-27 recurringStatus` → `document.RecurringStatus(report.SeriesState) string`, used by `render_recurring.go:33-40` and the builder; `render_anomalies.go:17-21 anomaliesBaselineWord` → `document.BaselineWord(report.AnomalyBaseline) string`, used by `render_anomalies.go:46`; each func tested per value plus unknown → `""`. Move `json_recurring_internal_test.go` (:44-324) and `json_anomalies_internal_test.go` (:52-133) to document through the same indent helper; cli wrappers stay one-line

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on every new exported symbol; retarget comments naming moved symbols (`internal/cli/output.go:36-38`, `json.go:140`); `go doc ./internal/report/document` reads as a contract

### Verify
- [ ] Step 7: full verification + `spec-check.py phase3b-analysis-tools` → tick SCENARIO-01 with its acceptance test; write STATE.md (first one for this feature), carrying the Handoff below

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `document.NewSpending/NewCashFlow/NewRecurring/NewAnomalies(result, warnings)` are the only builders of the four documents — S02/S09/S10 render MCP results from them; cli only indents (`marshalDocument`)
- `document.<X>Warnings(result, word)`: `word` feeds ONLY the linked-tracking and not-in-reports lines; subjects, nouns, no-rates, multi-tag lines are fixed — spec §4.1; S02's equality harness substitutes only those two templates. Order: left-out, unconverted, multi-tag, empty window; the caller prepends config lines (cli `withConfigWarnings`, mcp `cfg.WarningsAbsolute`) and mcp appends the cap line
- `store.SpendingGroups()`/`CashFlowPeriods()` + `String()` are the `by` vocabulary in order — S02/S09 schema enums come from them, never hand-typed (`finding.Types()` precedent); cli keeps only header/missing tables
- `money.NativeOf` is the one home (cli `accountsFXWarnings` and document both call it); `document.RecurringStatus`/`BaselineWord` are shared by text and JSON
- document still imports only report, store, finding, platform (money, humanize, tomlstr); never cli, mcp, config

**Left unbuilt** — named so nobody assumes it exists:
- mcp clock (`WithClock`), compact encoding, list caps, MCP currency resolution — S02/S09
- `withConfigWarnings`, `currencyFlag.resolve`, `accountsFXWarnings`, `recurringEvery`, the `*Command` consts stay in cli
- accounts/snapshots/sync documents stay in `internal/cli/json_*.go`

**Traps** — things that look right and are not:
- recurring/anomalies `--json` pins decode into structs (`run_recurring_json_test.go:72 decodeRecurringJSON`, `run_anomalies_json_test.go:49 decodeAnomaliesJSON`): blind to key order and dropped fields. spend/cashflow have literal pins (`run_spend_json_test.go:37`, `run_cashflow_json_test.go:32`). `JSONEq` appears only in `run_json_test.go` (sync, out of scope). The unit key-order tests (`json_recurring_internal_test.go:209`, `json_anomalies_internal_test.go:52`, `json_spend_internal_test.go:195`) move in the same batch as their code, so only step 1's goldens prove pre-move bytes
- Left-out lines appear only for `--account`-named accounts; naming only left-out accounts suppresses the empty-window line
- `cashFlowCommand` is `cashflow`, its empty subject is `income or spending` — two different strings
- Adding `String()` to the enums is safe only because duckstore formats them with `%d` (`duckstore/spending.go:115`, `cashflow.go:64`); a `%v` would change error text
- `append([]string{}, warnings...)` is neutral only because `withConfigWarnings` (`currency.go:71`) never returns nil

## Phase report

Run B1 (steps 2-4) done and committed (`ddaffa4` steps 2-3, step 4 in the next commit). Acceptance `Test_run_prints_spend_cashflow_recurring_and_anomalies_byte_for_byte` green after the move; `cmd/quarry/run_analysis_documents_*_test.go` untouched (`git diff f388acf -- cmd` empty). Narrow loop green (store, money, document, cli, cmd/quarry); `golangci-lint run ./internal/report/... ./internal/cli/... ./internal/store/... ./internal/platform/...` 0 issues.

Built: `store.SpendingGroups/CashFlowPeriods` + `String()` (`store.go`); `money.NativeOf`; `document/account_filter.go`, `warnings.go` (`SpendingWarnings`/`CashFlowWarnings`/`RecurringWarnings`/`AnomaliesWarnings`), `spending.go`, `cashflow.go`; cli `empty_window.go`, `json_spend.go`/`json_cashflow.go` bodies deleted/one-line wrappers; cli `accountFilterDocument` gone (recurring/anomalies json now use `document.AccountFilter`/`NewAccountFilters`).

Tests moved: `fx_warning_internal_test.go:14-86` -> `document/warnings_internal_test.go` (renamed from plan's `warnings_test.go`: testpackage lint wants `_internal_test.go` for white-box); new black-box `document/warnings_composers_test.go`; spend/cashflow JSON tests -> `document/spending_test.go`, `cashflow_test.go` (+read-back and warnings-copy tests); shared helpers `document/helpers_test.go` (`indented`, `topLevelKeys`). cli keeps `topLevelKeys` and `spendWindow` in `json_internal_test.go` (accounts/recurring/anomalies/render_table tests still use them; recurring/anomalies move in step 5 - the `topLevelKeys` in `helpers_test.go` is ready for them).

Next run (B2, step 5): move `json_recurring.go`/`json_anomalies.go` + `recurringStatus`/`anomaliesBaselineWord` per plan; recurring/anomalies json already use `document.AccountFilter`. Use `indented(t, ...)` and `window`-style fixtures (package-level `window` var lives in `warnings_composers_test.go`). Not yet done: step 6 comment retargets (`output.go:36-38`, `json.go:140` - `json.go:140` marshalDocument comment still names moved docs), doc comment review, `go doc`.
