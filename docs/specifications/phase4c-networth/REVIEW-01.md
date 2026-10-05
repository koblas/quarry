# Review Report — gate round 1

### Target
Changed files `7e1a0026f22a..2e5eb22` (162 files; 44 production).

### Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: triggered by `internal/**/*.go`.
- test-reviewer: triggered by `**/*_test.go`.

### Skipped reviewers
- api-reviewer: no HTTP files. pipeline-reviewer: no `.claude/**` change.

### Gate data
- `go test` rc=0; `uncovered-diff.py`: 0 uncovered added lines; lint 0 issues; `spec-check.py --run phase4c-networth` OK.
- test-stats TOTAL 3002 (+280).
- mutation-sample: 20 of 113 sampled — 20 killed, 0 survived, 1135 s.

### BLOCKER
- **correctness** `internal/report/document/holdings_left_out.go:30-45`: a priced CAD/USD holding in an account of the other currency, on a day with no exchange rate (before the first rate, or no rates), is left out of the account's balance with no warning on `networth` (all modes) and on `accounts`. This contradicts N-7. The current pins `holdings_left_out_test.go:125-126` assert the silence. **Ruled:** the new warning item 6 in `specification.md` → Surface & Copy → Warnings (product-vision, gate round 1), with the orchestrator order ruling (holdings kinds 3-6 come before the rate lines on both commands).

### MAJOR
- **correctness** `internal/store/duckstore/accounts.go:14-15`: `Store.Accounts` casts cash, holdings_value, balance and the converted columns ×100 to BIGINT. A holding past 64 bits of cents fails the whole read: `Conversion Error: Failed to cast decimal value 99999999999999999800000000 to type INT64`. That refuses `accounts` and every `--account` command, with the false "run quarry sync" copy. Fix: read HUGEINT into `*big.Int`, as `duckstore/networth.go` does, and carry it through `store.AccountBalance` and the accounts renderers and JSON. Add an Accounts arm beside `Test_net_worth_reads_a_balance_past_64_bits_in_exact_cents`.
- **test** The edge row "investment cash goes negative" is unpinned in every layer. Fix: a `v_balances_daily` case (brokerage cash -100.00, valued holding 30.00 → balance -70.00), a matching `Store.Accounts` case, and one cmd row showing the minus sign.
- **test** Sibling readers of investment cash rows are unpinned: anomalies, recurring, `spend --by payee`. Fix:
  - one cmd test: ten margin-interest charges in one expense category plus one at 5×, asserting `anomalies` lists it against the category baseline, and the below-minimum / `NotJudged` counting;
  - one `recurring` case asserting investment rows (NULL payee) form no series;
  - one `spend --by payee` case asserting the no-payee bucket for margin interest.
  - Spec N-2 wording is amended (orchestrator): margin interest reaches spend and anomalies; it does not reach recurring, which needs a payee.

### MINOR
- **correctness** `internal/cli/render_networth_history.go:95`: `netWorthHistoryCaption` indexes `Dates[0]` with no guard. It is unreachable from the CLI and MCP. Fold: guard `len(n.Dates)==0`.
- **correctness** `internal/store/duckstore/accounts.go:60`: the unvalued query re-reads `current_date`, which can drift past midnight. Fold: read the as-of once and pass it as a parameter.
- **test** `cmd/quarry/run_mcp_net_worth_test.go:29-71`: no CLI↔MCP parity row with a left-out holding. Fold: add an unpriced-security row.
- **test** `cmd/quarry/run_investment_cash_test.go`: no sync-twice pin that cash-row and synthetic split ids, and the `uncategorized` finding's state, survive a re-sync. Fold.
- **test** `cmd/quarry/run_investments_test.go` (`cashID(reinvestPK): "-6.00"`): spec N-1 said "reinvest_dividend: no row". Spec amended (orchestrator): a row exists iff amount ≠ 0, whatever the action. No code change.
- **refactor** `internal/report/networth.go:~92`: extract the bucket-by-day phase of `Server.NetWorth`. Deferred.
- **refactor** `internal/cli/networth.go:~47-90` and `internal/mcp/net_worth.go`: the as-of/history decision is duplicated. Move it to `report.NewNetWorthRequest` with a sentinel. Deferred.
- **refactor** `internal/report/window.go:~110-155`: `parseWindow` and `ParseMonthEndWindow` repeat guard cases. Deferred.
- **refactor** `internal/report/document/networth.go:~46`: extract `newNetWorthDate`. Deferred.
- **refactor** `internal/store/store.go:~200-212` and `internal/report/networth.go:~46-50`: duplicated FirstRate/FirstBalance docs. Fold: one-line pointer on `report.NetWorth`.
- **refactor** `internal/store/duckstore/networth.go:~33`: trim the NetWorth doc (also in STATE debts). Fold.

### NIT
- **arch** `internal/cli/render_networth_history.go:57`: the "no rate" decision for the history total sits in cli. A `report.NetWorth.TotalConverted(date)` would mirror `TypeConverted`. Deferred.
- **refactor** `internal/report/networth.go:~63-134`: method grouping. Deferred.
- **refactor** `internal/mcp/net_worth.go:~46`: `append` onto the callee's slice. Fold: `slices.Concat`.
- **refactor** `internal/report/networth.go:~79`: `position` keyed by a formatted string. Deferred.
- **refactor** `internal/importer/splits.go:~47`: trailing comment repeats the doc. Fold: delete.
- **test** history with empty `Dates` is unpinned. Covered by the caption-guard fold.

### Strengths
- arch: dependency rule clean; delivery layers thin; ports and adapters in the right places.
- correctness: importer ordering, view SQL, month-end math, totals and MCP refusal order all checked clean. DuckDB `current_date` against the Go clock was probed across time zones.
- test: 20/20 mutants killed; the format × mode × edge-row grid is broad.

### Verdict: BLOCKED
