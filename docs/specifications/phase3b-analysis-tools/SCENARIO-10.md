---
id: SCENARIO-10
status: done
---

# SCENARIO-10: recurring_charges returns the recurring --json document (folds SCENARIO-10b, 11, 11b)

Cadence: code-first (no write guard, atomic adapter or bug fix; no new `report.Store` method)
Acceptance test: `cmd/quarry/run_mcp_recurring_charges_test.go` `Test_run_mcp_recurring_charges_returns_the_recurring_json_document`
Acceptance test (SCENARIO-10b, folded): `cmd/quarry/run_mcp_recurring_charges_test.go` `Test_run_mcp_recurring_charges_refuses_a_future_since_in_its_own_words`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_mcp_anomalies_test.go` `Test_run_mcp_anomalies_returns_the_anomalies_json_document`
Acceptance test (SCENARIO-11b, folded): `cmd/quarry/run_mcp_anomalies_test.go` `Test_run_mcp_anomalies_refuses_a_future_since_in_its_own_words`
Narrow loop: `go test ./internal/mcp/ -run 'recurring|Recurring|anomal|Anomal|Cap|deadline|(?i)window' && go test ./cmd/quarry/ -run 'MCP|mcp'`
Mutation checks: `accountRefusal` at the `Recurring` / `Anomalies` error sites -> `Test_run_mcp_recurring_charges_refuses_an_account_without_its_name_on_stderr` / `Test_run_mcp_anomalies_refuses_an_account_without_its_name_on_stderr`; `capList` call (and its line last) in each handler -> `Test_recurring_charges_cuts_series_to_the_cap_*` / `Test_anomalies_cuts_charges_to_the_cap_*`; twin `"recurring"` passed to `resolveCurrency` -> `Test_recurring_charges_refuses_an_unreadable_config_before_building_the_report`; a second `s.now()` for `Now` -> `Test_recurring_charges_reads_today_once_*` / `Test_anomalies_reads_today_once_*`; tool name passed to `ParseChargeWindow` -> the two future-since acceptance tests
Runs: A (1-3) | B1 (4-6) | B2 (7) | V (8-9)
Size: OWNS A RUN — 4 build batches (recurring handler, recurring window rows, anomalies handler, all-tools rows), 1 feature package (`internal/mcp`); S10b/S11/S11b folded per sizing

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_recurring_charges_test.go` (new) `Test_run_mcp_recurring_charges_returns_the_recurring_json_document` and `cmd/quarry/run_mcp_anomalies_test.go` (new) `Test_run_mcp_anomalies_returns_the_anomalies_json_document`. Use `runBothSurfaces` (`run_mcp_documents_helpers_test.go:40`). Rows: `populatedAnalysisStore` with the four accounts of `run_analysis_documents_test.go:46`; none given; `currency native`; `emptyWindowAnalysisStore`; `unknownKeyConfig` with currency absent. Assert `cliBody == toolBody` and warnings via `inToolWords(.., "recurring", "recurring_charges")` / `(.., "anomalies", "anomalies")`. The recurring accounts row asserts `Contains` the linked line with `so recurring_charges leaves it out` (`linkedLineForCashFlow` precedent, `run_mcp_cash_flow_test.go:15`)
- [x] Step 2: same two files, `Test_run_mcp_recurring_charges_refuses_a_future_since_in_its_own_words` and `Test_run_mcp_anomalies_refuses_a_future_since_in_its_own_words`, shaped like `run_mcp_spending_test.go:128-179`. `since 2099`, no until. Exact isError text from §6.2; stderr `quarry: mcp: <tool>: ` + `windowRefusedLog` (`run_mcp_spending_test.go:25`)
- [x] Step 3: `internal/mcp/tools.go:16-24` adds `toolRecurring`/`toolAnomalies`. Also: `:83-88` descriptions §3.3 verbatim (hard line breaks); `:90-98` six per-param strings §3.4 (`currencySchema()` reused); `:100-130` `recurringInput`/`anomaliesInput` (pointer since/until, accounts, currency, **no `by`**); `:133-159` registration with stub handlers returning a zero `document.Recurring{}`/`document.Anomalies{}`. In the same step, `cmd/quarry/run_mcp_test.go:60-132` gets the two description and input-schema consts and `:148-156` gets the `wantTools` entries, so the 8-tool pin stays green; do not rename the test. Red: equality fails at the body assertion, future-since at `IsError`

### Build
- [x] Step 4: new `internal/mcp/recurring_charges.go` `(*Server).recurringCharges`, mirroring `cash_flow.go:16-43`. Order: one `s.now()`; `report.ParseChargeWindow(toolRecurring, ..)` -> `windowRefusal`; `resolveCurrency(in.Currency, "recurring")`; `newReport(ctx, commandName)`; `srv.Recurring(ctx, RecurringRequest{.., Now: <same value>})` -> `accountRefusal`; `document.NewRecurring(rec, append(configWarnings, document.RecurringWarnings(rec, toolRecurring)...))`; `capList(doc.Series, doc.Warnings, toolRecurring, "series", "totals count every series; pass a shorter period or fewer accounts")`. Support in `query_helpers_test.go:35-63`: `fakeStore` gains a `charges store.Charges` field and a `Charges` method that records `ChargeParams.Through`; `harness.recurringCharges` beside `:158`. Unit tests in `internal/mcp/recurring_charges_test.go`, mirroring `cash_flow_test.go:19-175`:
  - reads today once (stepped clock, `Through` = that call's date);
  - `until 2099` is not refused and `Through` is still today;
  - unreadable config -> `run quarry recurring to see why`;
  - config not read when currency is given; config read as `mcp`;
  - factory failure -> generic line;
  - store refusal verbatim; plain fault -> generic line;
  - schema-rejected `by`, `cad`, null since, non-list accounts, with no config read and no build;
  - `Test_recurring_charges_cuts_series_to_the_cap_*`: a `Charges` generator gives 501 series; cap line last after a config line and a document warning; `totals` equal the uncut result's. The 500 control is `cap_internal_test.go:18`.

  Step 2's recurring test is expected green on arrival here. Report it.
- [x] Step 5: `recurring_charges_test.go` window rows, through the harness, each with the class line on stderr and no config read or build: not a date, `""`, since after until, until before the default since, and the charge future-since line in `recurring_charges` words
- [x] Step 6: new `internal/mcp/anomalies.go` `(*Server).anomalies`, the same shape as step 4. Differences: `ParseChargeWindow(toolAnomalies, ..)`, twin `"anomalies"`, `AnomaliesWarnings(.., toolAnomalies)`, `capList(doc.Anomalies, doc.Warnings, toolAnomalies, "charges", "pass a shorter period or fewer accounts")`. Unit tests in `internal/mcp/anomalies_test.go`: the same rows as step 4, plus window rows incl. the anomalies future-since line. Add `Test_anomalies_cuts_charges_to_the_cap_*`: 501 anomalies, generated from `report.AnomalyPayeeMinHistory`/`AnomalyMinAmount`/`AnomalyPayeeMultiplier`, not literals. Assert `checked`/`not_judged` equal the uncut result's and the line is last
- [x] Step 7: rows for both tools in the four remaining all-tools tables:
  - `cmd/quarry/run_mcp_no_store_test.go:22-30`;
  - `internal/mcp/timeout_test.go:101-108`, plus `stallingStore.Charges` beside `:66-75`, recording the deadline and stalling like `CashFlow`;
  - `cmd/quarry/run_mcp_store_faults_test.go:62-66,84-86`: `directoryStore` open-time, and `brokenStore("DROP VIEW v_spending")` statement-time with reason `Table with name v_spending does not exist!` (`chargesQuery` reads `v_spending`; confirm the text on the first run);
  - new `Test_run_mcp_recurring_charges_refuses_an_account_without_its_name_on_stderr` / `Test_run_mcp_anomalies_refuses_an_account_without_its_name_on_stderr`, copying the three cases of `run_mcp_cash_flow_test.go:63-115`.

### Sweep
- [x] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on both handlers, twin consts and input types

### Verify
- [x] Step 9: full verification block + `.claude/scripts/spec-check.py phase3b-analysis-tools` -> tick SCENARIO-10, 10b, 11, 11b (folded lines "delivered by SCENARIO-10", test last); rewrite STATE.md

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- recurring_charges and anomalies read the clock once and pass that one value to `ParseChargeWindow` and `Request.Now`. Rule 12: two reads straddle midnight, and the charge window's today must equal the `Through` the report reads.
- The tool name, not the CLI twin, goes to `ParseChargeWindow`. The charge refusal speaks it through `WindowError.Command` (spec §6.2 rows).
- recurring passes twin `"recurring"` to `resolveCurrency`. anomalies passes `"anomalies"`, the same string as the tool name.
- The anomalies cap noun is `charges` and its list is `doc.Anomalies`; the advice has no totals clause. `checked`/`not_judged` stay uncut (spec §5, Rule 11).
- The tools/list pin (`run_mcp_test.go:134`) now pins eight tools. S13 renames it and adds the 3a per-param descriptions.

**Left unbuilt** — named so nobody assumes it exists:
- 3a per-param descriptions, instructions, query description, `quarry mcp` Long, and the rename of `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc`: owned by S13
- A `by` default row in `server_test.go:181` for these tools: not needed, they have no `by`

**Traps** — things that look right and are not:
- STATE says "S10 passes `series`/`anomalies` nouns". The anomalies noun is `charges`; `anomalies` is the list field.
- `fakeStore` embeds a nil `report.Store`. `Recurring`/`Anomalies` call `namedAccounts`, so a unit test passing `accounts` panics. The account rows live at cmd level.
- For anomalies, tool name == twin == CLI word. `inToolWords` maps nothing, and a twin-swap mutation is invisible. Only recurring's twin is mutation-pinned.
- The harness map must substitute on `, so recurring leaves it out`, never on bare `recurring`, which sits inside `recurring charges`.
- S10b and S11b go green as soon as their handler passes the tool name. Nothing new is worded in `window.go`.

## Phase report

Run V (steps 8-9) done; scenario complete.
- Sweep: `golangci-lint run ./...` 0 issues; one `prealloc` hit fixed (`monthlyCharges`, `internal/mcp/recurring_charges_test.go:212`). `dupl` did not fire on the account tests. Doc comments on both handlers, twin const and input types already within budget; no edit.
- Verify (start 2322c52): `go test rc=0` (covered full suite), `uncovered-diff.py` 0 uncovered added lines, `-race` green on `./internal/mcp/...` and `./cmd/quarry -run 'MCP|mcp'`.
- test-stats: cmd/quarry 547 (+6), internal/mcp 132 (+24), TOTAL 679 (+30); tempdir 480 (+4), disk 436 (+4).
- specification.md: SCENARIO-10, 10b, 11, 11b ticked (folded ones "delivered by SCENARIO-10"). `spec-check.py` rc=0, `spec-check.py --run` rc=0.
- STATE.md rewritten. Next: S13 (eight-tool descriptions, rename of the tools/list pin test).
