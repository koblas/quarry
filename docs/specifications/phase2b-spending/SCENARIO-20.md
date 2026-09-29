---
id: SCENARIO-20
status: open
---

# SCENARIO-20: cashflow shows income, spending and savings rate by month

Cadence: code-first — read-only command; no bug fix, write-safety guard or atomic adapter touched
Acceptance test: `cmd/quarry/run_cashflow_test.go` `Test_run_cashflow_shows_income_spending_and_savings_rate_by_month`
Acceptance test (SCENARIO-21, folded): `cmd/quarry/run_cashflow_test.go` `Test_run_cashflow_by_year_shows_one_row_per_year_and_na_without_income`
Acceptance test (SCENARIO-23, folded): `cmd/quarry/run_cashflow_invariant_test.go` `Test_run_cashflow_total_spent_equals_spend_total_per_currency`
Acceptance test (SCENARIO-24, folded): `cmd/quarry/run_cashflow_refusals_test.go` `Test_run_cashflow_refuses_and_reports_empty_periods_like_spend`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run 'CashFlow|cashflow|Cashflow|spend|Spend|Available|refuse|interrupted|takes_no'`
Mutation checks: cashflow query spent sums only negative expense amounts (refund netting dropped, `duckstore/cashflow.go`) → `Test_run_cashflow_total_spent_equals_spend_total_per_currency`; cashflow query spent also counts positive uncategorized splits (`duckstore/cashflow.go`) → same test; `v_spending` DDL adds `AND category_id IS NOT NULL` (negative uncategorized dropped from spend, `duckstore/schema.go`) → same test; savings-rate `income <= 0 → NULL` guard flipped to `< 0` → `Test_cash_flow_has_no_savings_rate_when_income_is_zero_or_less`
Runs: A (1-3) | B1 (4-5) | B2 (6) | V (7-8)
Size: OWNS A RUN — 3 batches, 1 feature package (report; duckstore adapter + cli delivery)

**Copy gaps — orchestrator: get a `product-vision` copy ruling before dispatching B2** (proposed defaults, UNRULED; B1 does not depend on them):
- `--by year` column-1 header — proposed `Year` (Surface & Copy shows only `Month`); year label `2026` as JSON.
- `n/a` in the right-aligned Savings rate column — proposed right-aligned like numbers.
- Negative rate and minus zero — proposed `-12.5%`; a rate rounding to −0.0 prints `0.0%` (JSON `0`), never `-0.0%`.
- Large-magnitude rates (net far below −income) — proposed thousands grouping, `-9,990.0%`.
- Interim `cashflow --json` until SCENARIO-22 — proposed S09 precedent: prints the text table.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_cashflow_test.go` (new) — S20: exact stdout via `runWith` + `spendEnv` (clock 2026-09-29); CAD income and spending in several months, one month income-only, one empty month (`0.00`/`n/a`), September `partial`, Total; fixture via `spendRows` (`run_spend_test.go:31-72`) plus an income category. S21 test in same file: `--by year --since 2020 --until 2025`, one row per year, a no-income year `n/a`.
- [ ] Step 2: `cmd/quarry/run_cashflow_invariant_test.go` (new) — S23: same store, `spend` and `cashflow` with identical `--since/--until`, two arms (no `--account`; `--account` naming an in-reports and a not-in-reports account); fixture: expense refund, positive and negative uncategorized, transfer pair (append `store.Transfer` to the rows), a `NotInReports` account with spending, CAD and USD. Per currency assert both Totals equal a value derived by hand from the fixture — never from code (a `v_cash_flow` mutation moves both sides together; only the pin catches it). `cmd/quarry/run_cashflow_refusals_test.go` (new) — S24 outline rows S1, S4 (`quarry: --by must be month or year`), S6, E1 (`quarry: warning: no income or spending from 2026-01-01 to 2026-02-28; the store's transactions run …`, stdout caption + header only) with stdout, stderr and exit per row.
- [ ] Step 3: stubs — `internal/store/store.go:338-380` cash-flow param/row/total/result types + period enum; `internal/report/store.go:11-20` `Store.CashFlow`; zero-value `CashFlow` on `*duckstore.Store`, `internal/report/fakes_test.go:11-37` `fakeStore`, `internal/cli/fakes_test.go:13-34` `fakeReportStore`; `(*report.Server).CashFlow` stub; `internal/cli/cashflow.go` (new) command with verbatim Use/Short/Long/Example and all four flags, inert RunE (no output, nil); `internal/cli/root.go:30` AddCommand; `cmd/quarry/run_status_test.go:97-103` gains the `cashflow` line. All four acceptance tests red at an assertion (S24's S1/S4/S6 at their stderr asserts, not green on arrival).

### Build
- [ ] Step 4: `internal/store/duckstore/cashflow.go` (new) `(*Store).CashFlow` — one statement over `v_cash_flow`, month or year key as SQL text (byte-equal to Go `2006-01`/`2006`), per-period rows + per-currency totals via `GROUPING SETS`; income, spent, net in BIGINT cents and savings rate in SQL (P2b-9); account filter via `accountFilter` (`spending.go:14-33`); `transactionRangeQuery` (`spending.go:38-45`) LAST, only when no totals; extract the window+account args builder shared with `spending.go:145-150`. Tests `cashflow_test.go` (new, `newBuiltStore`): income/spent/net per period and total; income-only period spent `0`; refund nets; positive uncategorized = income; transfer, excluded txn, not-in-reports account absent; account filter; year key; CAD before USD; `Test_cash_flow_has_no_savings_rate_when_income_is_zero_or_less` (income 0 with spending, income negative; 0.01 control); tie 250/800 → 31.3; negative net; net −3.00 on income 10,000.00 → not minus zero (per copy ruling); unknown period → its own sentinel; faults mirroring `spending_test.go:155-209` (open, query, scan, close) and `spending_range_test.go:89-99` (`passQueries: 1`) for the range statement.
- [ ] Step 5: `internal/report/period.go:21-34` add the year unit beside months (`fillMonths` behaviour unchanged); `internal/report/cashflow.go` (new) `CashFlowRequest`, `CashFlow` result (+ `Empty()` = no Totals), `(*Server).CashFlow` — `resolveAccounts(ctx, "cashflow", …)`, `readRefusal(ctx, "cashflow", err)`, fill every period × Totals currency (store row else zero row with nil rate), Partial on first/last only; extract the names→ids step shared with `spending.go:65-75`. Tests `cashflow_test.go` (new, `fakeStore`): month and year fill, partial first/last with in-bound controls, currencies in Totals order, empty → no periods, params passed (window, period, ids), S5/S6 copy, store fault → `cashflow interrupted`/R-refusal, Accounts fault.
- [ ] Step 6: `internal/cli/cashflow.go` RunE per `spend.go:45-74` (S4 parse before `windowFlags.window` before `openReport`); `internal/cli/render_cashflow.go` (new) caption `Cash flow … in <spendingAccountsCaption>`, columns per Surface & Copy, Status last and unpadded, Total rows empty Status, rate format per copy ruling; interim JSON = text table, the uncovered `return err` after `renderResult` marked `// unreachable:` citing `output.go:26`. Warnings: extract W2 loop + all-named-excluded suppression from `spend.go:84-103` into one helper used by both; cashflow passes `"cashflow"` and subject `income or spending` — `internal/cli/spend_account_test.go`, `spend_empty_test.go`, `cmd/quarry/run_spend_empty_test.go`, `run_spend_account_test.go` stay green. Tests `cashflow_test.go`/`render_cashflow_internal_test.go` (new): S4 (`month`, `year` controls; `week`, `""`), S4 before store open, W2 → E order, all-named-excluded → W2 only, report fault → exit 1, factory fault, failed stdout write, interim `--json`, render widths from cells, `n/a`, Year header. cmd rows: R1 `run_read_refusals_test.go:53`, I1 `:165` (`quarry: cashflow interrupted`), U8 `run_read_usage_test.go:40` (`quarry: cashflow takes no arguments`).

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new exported symbols; `internal/cli/root.go:7-9` doc lists cashflow; `internal/cli/run.go:24-25` Env doc names cashflow as a `Now` reader; clear STATE open debts only if touched.

### Verify
- [ ] Step 8: full verification + `spec-check.py phase2b-spending` → tick SCENARIO-20, and 21, 23, 24 as "delivered by SCENARIO-20" with their acceptance tests.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `report.Store.CashFlow(ctx, store.CashFlowParams{Window, By, AccountIDs})` is the one cash-flow read; the period is a param value, never a second method — SCENARIO-22 and the Gate read it as is.
- Income, spent, net and savings rate are computed in the duckstore query (P2b-9); `report` only fills missing periods with zero rows and a nil rate — the `income <= 0 → NULL` rule has one owner.
- Cashflow spent = `SUM(-amount)` over `flow = 'expense'` of `v_cash_flow`, the rows `v_spending` selects — P2b-10 holds by construction and is pinned by the S23 test.
- Cashflow empty = no Totals: an income-only window is not empty; a currency netting to zero keeps its Total (same as spend). Empty → no periods, header only, E line subject `income or spending`.
- Fill = every period of the window × each Totals currency in Totals order; Total rows never partial; Status column always present (SCENARIO-12 ruling).
- One W2/E helper for spend and cashflow; order W2 → (W1, spend only) → E.

**Left unbuilt** — named so nobody assumes it exists:
- `renderCashFlowJSON`, the `periods`/`totals` document — SCENARIO-22.

**Traps** — things that look right and are not:
- Interim `--json` prints the text table; SCENARIO-22 must remove the `// unreachable:` marker in `cashflow.go` and replace the interim cli test.
- `SUM(...) FILTER`/`CASE` over a period with no expense is NULL, not 0: COALESCE before the cents cast.
- A small negative net rounds to −0.0: Go `%.1f` prints `-0.0%`, `encoding/json` prints `-0` — normalise once, where the copy ruling says.
- Year/month label text must match Go's layout byte for byte, or every period prints `0.00` beside lost rows; only a DuckDB-backed test sees it (SCENARIO-12 trap).
- A mutation inside `v_cash_flow` (e.g. transfer exclusion) keeps spend = cashflow; only the S23 test's hand-derived values catch it.
