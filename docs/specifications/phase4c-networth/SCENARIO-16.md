---
id: SCENARIO-16
status: open
---

# SCENARIO-16: Net worth before any data

Cadence: code-first
Acceptance test: `cmd/quarry/run_networth_before_data_test.go` `Test_run_networth_before_the_first_transaction_prints_the_caption_and_the_first_balance_warning`
Narrow loop: `go test ./internal/store/duckstore/ -run 'NetWorth|net_worth' && go test ./internal/report/... ./internal/cli/ -run 'NetWorth|Networth|networth' && go test ./cmd/quarry/ -run 'networth'`
Mutation checks: empty = "no `Rows` on any listed date", not "all balances zero" (drop the zero-row guard) → `Test_NetWorthWarnings_says_nothing_for_a_date_whose_rows_all_sum_to_zero`; first-transaction scope (`reportedAccount`) → `Test_net_worth_first_transaction_ignores_accounts_left_out_of_reports`
Runs: A (1) | B1 (2-3) | B2 (4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (report; store/duckstore/document/cli/cmd wiring)

## Implementation Plan

Inventory (read, not Glob): `Server.NetWorth` is the only producer of `report.NetWorth`; `cli/networth.go:84` the only caller of `NetWorthWarnings`; renderers (`render_networth.go:26`, `render_networth_history.go:98`) already print caption and header with no Total on empty `Dates[*].Rows`, so no renderer change. Empty-rendered text and JSON are already ruled by S10/S12; S16 adds only the warning and the fact behind it.

**Definitions (binding).** Empty = `len(n.Dates) > 0` and every `Dates[i].Rows` is empty (no store row on any listed date). A zero-balance row IS a row, so a zero-only date is not empty and warns nothing. `Dates` empty (zero value, public empty `Window`) warns nothing and must not index `Dates[0]`. Same in native, CAD and USD listings. First transaction = earliest `transactions.date` among counted accounts (`reportedAccount`: in reports, not linked tracking), the same accounts `v_net_worth` counts; zero = none.

**Copy needing a ruling before B1 (orchestrator: scoped product-vision copy ruling; plan below assumes the answers):**
1. No-data history form is unruled (spec has only snapshot): assumed `no account has a balance at any month end from <s> to <u>; the store has no transactions`.
2. `<s>`/`<u>`: spec is ambiguous (caption uses first/last listed month ends; JSON `since`/`until` are resolved since / clamped until). Assumed first and last LISTED month ends (`Dates[0].Date`, `Dates[len-1].Date`), as the caption, because the line says "month end". Dates in `DateLayout`.
3. Scope of "the store's transactions": assumed counted accounts only (a not-in-reports account's earlier transaction must not make "start <d>" false).

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_networth_before_data_test.go` `Test_run_networth_before_the_first_transaction_prints_the_caption_and_the_first_balance_warning` — `seedNetWorthStore` (`run_networth_test.go:26`, first transaction 2026-03-02), `holdingsClock()`, `--as-of 2026-03-01`: exit 0, stdout caption plus header only (no Total), stderr `quarry: warning: no account has a balance on 2026-03-01; the first balance is on 2026-03-02`. Compiles unstubbed; red at the stderr assertion.

### Build
- [x] Step 2 (B1, store fact, one read): `store/store.go:~657-666` add `NetWorth.FirstTransaction time.Time` (zero when none; doc one line, beside `FirstRate`); `duckstore/networth.go:65-70` read it as the fourth query in the same open after `firstRate` (new unexported `firstTransaction(ctx, db)` modelled on `charges.go:74-81`, SQL `min(t.date)` over `transactions t JOIN accounts a` WHERE `reportedAccount`); `report/networth.go:36-49,96` `NetWorth.FirstTransaction`, copied from the read. Tests: `duckstore/net_worth_read_test.go` (beside :127-168) — gives the earliest date; zero for a store with no transactions; ignores not-in-reports and linked-tracking accounts (`Test_net_worth_first_transaction_ignores_accounts_left_out_of_reports`, their earlier transaction does not win; control: counted account's later one does); query fault and scan fault with `passQueries: 3` and a fault text that matches only the new query; bump `unvalued_holdings_test.go:214` `Test_net_worth_for_no_dates_runs_no_query` count 3 -> 4 and its `idle` stays 0 (empty dates still opens and queries nothing); `report/networth_test.go` — fake read's `FirstTransaction` reaches `report.NetWorth` for snapshot and history.
- [x] Step 3 (B1, composer): new `report/document/networth_empty_warnings.go` `emptyNetWorthWarnings(n)` (one line or none), called in `holdings_left_out.go:17-19` `NetWorthWarnings` BEFORE `unvaluedWarnings` (order: empty, unvalued, rates; update its doc line). Tests in new `networth_empty_warnings_test.go` through `NetWorthWarnings`, one row per arm, each verbatim: snapshot with a first transaction; history with a first transaction (`<s>`/`<u>` = first/last listed, window-since mid-month differs from first listed end: pins listed-not-resolved); snapshot no data; history no data; zero-row-only date -> none (`Test_NetWorthWarnings_says_nothing_for_a_date_whose_rows_all_sum_to_zero`); one non-empty date among empties (history) -> none; zero value `report.NetWorth{}` and `Dates` empty -> `[]string{}` (extends `holdings_left_out_test.go:216-217`); native and USD-reporting listings -> same line; ordering vs an unvalued line on hand-built input (empty first).
- [ ] Step 4 (B2, command matrix, `cmd/quarry/run_networth_before_data_test.go` plus one `internal/cli/networth_test.go` row): cmd rows reached through the real store: history `--since 2026-01 --until 2026-02` (all listed month ends empty: caption, header, one blank-Total line per month end, exit 0, stderr history line); `--json` snapshot and history (`dates` has the entries with `balances`/`totals` `[]`, `warnings[]` carries the line, `as_of`/`since`/`until` per existing rules); `--currency native` snapshot; store with no transactions at all (the "has no transactions" lines, snapshot and history); the seed's own `--as-of 2026-03-02` (rows exist) warns nothing (control one day later). `internal/cli/networth_test.go` (fake store, beside :60-100): config warning precedes the empty line in `warnings[]` and on stderr. n/a: `--account` (networth has none).

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments (`FirstTransaction`, `emptyNetWorthWarnings`); existing tests that hand-build an empty `store.NetWorth` through the CLI or `Server` now get the "no transactions" line: update their stderr expectations, not the code.

### Verify
- [ ] Step 6: full verification per `.claude/rules/agent-briefs.md` + `spec-check.py phase4c-networth` -> tick SCENARIO-16 with its acceptance test; rewrite `STATE.md` (drop "Left unbuilt" empty-history item; composer order; the fourth query).

## Handoff

**Orchestrator: copy ruling 2026-10-04 (product-vision) — recorded in `specification.md` Warnings item 2; it CHANGES this plan.** Copy is now `…; the first balance is on <f>` and the no-data form `…; no account in Quicken's reports has transactions or holdings` (snapshot and history). Rename `FirstTransaction` → `FirstBalance` (fields and test names `Test_net_worth_first_balance_…`): SQL = `min` over the union of `transactions.date` and `holding_shares.from_date` joined to `accounts` under `reportedAccount` (mirror the `firsts` CTE in `v_balances_daily`). Add a duckstore test where a holding date precedes the first transaction and wins, plus a control. `<s>`/`<u>` = first/last listed month ends (assumption 2 confirmed). Guard: print the `<f>` form only when `<f>` is after `<u>` (snapshot: after `<d>`); else no empty line — pin with one composer row. Data only in uncounted accounts → no-data form. Acceptance test stderr line becomes `…; the first balance is on 2026-03-02` (if the seed has no earlier holding); rename the acceptance test to `…_prints_the_caption_and_the_first_balance_warning`.


**Binding decisions:**
- Empty = at least one listed date and no `Rows` on any; zero-balance rows count as rows (not empty) — S17 MCP shares `NetWorthWarnings`, so the same line reaches `warnings[]` there.
- `store.NetWorth.FirstTransaction` / `report.NetWorth.FirstTransaction` read in the same open as rows, unvalued and `FirstRate`; always read (four queries), scoped to `reportedAccount` accounts, not the whole `transactions` table.
- Warning order: config, empty-result, unvalued (no price, no currency, other currency), rates.
- Empty-history `<s>`/`<u>` = first/last listed month end (pending ruling, see copy list above).

**Left unbuilt:** MCP `net_worth` (S17); no renderer change was needed.

**Traps:**
- Fakes and cmd fixtures with no transactions now print `...the store has no transactions` where they printed nothing; `Test_net_worth_for_no_dates_runs_no_query` and the `read_faults_test.go:44` NetWorth row follow the query count.
- The first-rate fault test (`net_worth_read_test.go:149`, `passQueries: 2`) stays on query 3; its fault text `query rows "SELECT min"` also fits the new `min(...)` query, so the new fault tests need `passQueries: 3`.
- `v_balances_daily` also starts an account at its first holding date, so the line's "transactions start" is the register start; a date before both is still empty and true.

## Phase report

Run A: acceptance test `cmd/quarry/run_networth_before_data_test.go` was red at its stderr assertion (seed's first balance 2026-03-02, `--as-of 2026-03-01`).
Run B1 done (steps 2-3, code-first); acceptance GREEN; `go build` and `golangci-lint run ./...` 0 issues; narrow loop green.
- Store fact: `store.NetWorth.FirstBalance` (`internal/store/store.go`); `duckstore/networth.go` `firstBalanceQuery` (union of `transactions.date` and `holding_shares.from_date` joined to `accounts` under `reportedAccount`) and `firstBalance(ctx, db)` as the fourth query after `firstRate`; `report.NetWorth.FirstBalance` copied in `Server.NetWorth` (`internal/report/networth.go`).
- Composer: `report/document/networth_empty_warnings.go` `emptyNetWorthWarnings(n)`, first in `NetWorthWarnings` (`holdings_left_out.go`), which seeds `[]string{}` so the result is never nil. `<s>`/`<u>`/`<d>` = `Dates[0]`/`Dates[last]`; the `<f>` form only when `FirstBalance` is after the last listed day, else no line; zero `FirstBalance` = no-data form.
- Tests: `duckstore/net_worth_read_test.go` (`Test_net_worth_first_balance_*`; query and scan faults with `passQueries: 3`), `unvalued_holdings_test.go` count 3 -> 4, `report/networth_test.go`, `document/networth_empty_warnings_test.go` (every arm, boundary f == last, zero value, order vs unvalued).
- Existing tests touched: `document/networth_rate_warnings_test.go` dropped its "no rows" case (an empty listing now gets the empty line); `internal/cli/networth_test.go` `leftOutNetWorth` fake got one chequing row so its stderr stays the holdings line only.
- Mutations, all red then reverted: zero row counts as empty -> `..._says_nothing_for_a_date_whose_rows_all_sum_to_zero`; guard inverted, `Before`, or against first listed -> `..._not_after_the_last_day_listed` and the arm tests; dates-empty guard dropped -> panic, `..._says_nothing_when_no_day_is_listed`; first=last, window ignored -> history arms; store: `reportedAccount` dropped -> `..._ignores_accounts_left_out_of_reports` and `..._ignores_a_holding_of_an_account_left_out_of_reports`; holdings out of union -> `..._is_a_holding_that_starts_before_the_first_transaction`; transactions out of union, max for min, fact not stored -> the earliest-date tests.
- Next: B2 owns step 4 (cmd matrix, one cli row for config-before-empty order). V owes the full suite: other cmd goldens that hand-build an empty net worth may now print the empty line (step 5).
