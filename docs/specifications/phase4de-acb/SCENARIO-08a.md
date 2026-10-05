---
id: SCENARIO-08a
status: open
---

# SCENARIO-08a: ACB is pooled per security across non-registered accounts (absorbs SCENARIO-09)

Cadence: code-first
Acceptance test: `internal/report/acb_test.go` `Test_acb_pools_each_security_across_non_registered_accounts`
Acceptance test (SCENARIO-09, folded): `internal/report/acb_test.go` `Test_acb_adds_a_reinvested_dividends_cost_and_splits_shares_once`
Narrow loop: `go test ./internal/report/ -run '(?i)acb' && go test ./internal/store/duckstore/ -run '(?i)investment_history|row_reads|reads_'`
Mutation checks: pro-rata half-away rounding → `Test_acb_rounds_the_acb_removed_half_away_from_zero` (ACB 0.05, sell 1 of 2 → 0.03); oversell exact-remainder guard → `Test_acb_removes_the_whole_acb_when_a_sale_exceeds_the_pool`; net-commission arm (proceeds = amount + commission) → acceptance test (2023 sale 804.95); split once per (security, date) → folded acceptance test (ACB removed 156.60); registered exclusion (`Of == false` only) → `Test_acb_pools_only_non_registered_accounts` (registered row and unclassified row); same-day tier order → acceptance test (766.50, not 753.00)
Runs: A (1-2) | B1 (3-4) | B2 (5) | B3 (6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (report; duckstore adapter behind its port)

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `internal/report/acb_test.go` both acceptance tests — `Server.ACB` against `fakeStore`, `Today` 2026-10-05, classification non-registered {acct-1, acct-2 (closed), acct-3 USD}, registered {acct-9}. Hand-computed (CAD cents, half away from zero):
  - XEQT sec-1 CAD: 2023-03-01 acct-1 buy 100, amount -2509.99, commission 9.99 (99900) → pool 100 / 2509.99. 2023-06-15 acct-1 sell 30 (src 10), amount 800.00, commission 4.9450 (49450); acct-2 buy 100 (src 11), amount -2600.00 → buy first: 200 / 5109.99; sale ACB 766.50, proceeds 804.95, outlays 4.95, gain 33.50 → 170 / 4343.49. 2024-02-10 acct-1 sell 50, amount 1600.00 → ACB 1277.50, gain 322.50 → 120 / 3065.99. 2024-09-03 acct-2 sell 100, amount 3300.00, commission 9.99 → proceeds 3309.99, outlays 9.99, ACB 2554.99, gain 745.01 → 20 / 511.00.
  - VTI sec-2 USD in acct-3; rates 2023-12-29 1.320000, 2024-01-03 1.330000, 2024-04-02 1.355000. 2024-01-02 buy 10, amount -2001.37 → 1.32 (latest earlier) → 2641.81. 2024-04-02 sell 4, amount 900.00, commission 1.00 → proceeds 1220.86, outlays 1.36, ACB 1056.72, gain 162.78 → 6 / 1585.09.
  - acct-9 RRSP control: XEQT buy 2023-05-01 50 / -1250.00, sell 2024-03-01 50 / 1500.00 — in no sale, pool or event.
  - Years: 2023 → 1 sale, 804.95 / 4.95 / 766.50 / 33.50. 2024 → 3 sales, 6130.85 / 11.35 / 4889.21 / 1230.29. Securities: VTI 6 sh 1585.09; XEQT 20 sh 511.00.
  - Folded S09 (ZEB sec-5 CAD): 2024-01-10 acct-1 buy 10 / -300.00; acct-2 buy 10 / -310.00 → 20 / 610.00. 2024-03-28 acct-1 reinvest_dividend 0.5, amount 0, cost_basis 16.40 → 20.5 / 626.40. 2024-05-01 split 2:1 recorded in acct-1 AND acct-2 → 41 / 626.40. 2024-06-03 acct-2 sell 10.25, amount 200.00 → ACB 156.60, gain 43.40 → 30.75 / 469.80 (split twice 78.30; no split 313.20; reinvest dropped 156.31).
- [ ] Step 2: `internal/report/store.go:9-15` `ValueReads.InvestmentHistory`; `internal/store/store.go:463-468` new `store.InvestmentHistory` beside `Rate`; new `internal/report/acb.go` `ACBRequest`/`ACB`/`ACBYear`/`ACBSale`/`ACBSecurity`/`ACBEvent` + `(*Server).ACB` signature-only stub; stub `(*duckstore.Store).InvestmentHistory`; `internal/report/fakes_test.go:10-40,99-120` fake field + method (with a reads counter). Let `go vet` list any other implementer (cli/mcp fakes embed `report.Store`). Red at the assertion.

### Build
- [ ] Step 3: new `internal/store/duckstore/investments.go` `(*Store).InvestmentHistory` — one `openRead`, every account via `readAccounts` (`findings_read.go:101-112`), securities (id, name, ticker, currency), every investment transaction with a security (all accounts, registered included; cents/millionths via `CAST(... AS BIGINT)` like `history.go:60`; date then source_id), rates via `readRates` (`history.go:269-308`); faults through `openFault`. `investments_test.go` `Test_investment_history_reads_every_investment_transaction_with_its_cost`: real store (`newStoreWithRates` `views_fx_test.go:42`, `holdingRows` `holdings_view_test.go:22`): NULL vs set commission/cost_basis/shares, split sides, fractional shares, closed account, registered-type account, cash-only row excluded.
- [ ] Step 4: `internal/store/duckstore/read_faults_test.go:21-57` `rowReads` gets `InvestmentHistory` (doc comment lists it) + new `Test_investment_history_returns_each_querys_fault` — one `passQueries` row per query (accounts, securities, transactions, rates column probe, rates) for `queryFault` and for `scanFault` (`fakes_test.go:33-65`).
- [ ] Step 5: `internal/report/acb.go` + new `acb_walk.go` core walk — pool = accounts `Classification.Of == false`; per security: events sorted date, tier (buy/reinvest → split → sell), source_id; buy adds -amount; sell removes round-half-away(ACB × sold ÷ held), sold ≥ held removes the whole ACB and the pool restarts at 0 / 0.00; outlays = commission rounded half away to cents, proceeds = amount + outlays; tax year = `Date.Year()`; events after `Today` dropped; read failure → `s.readRefusal(ctx, "acb", err)`. Tests in `acb_walk_test.go`, all through `Server.ACB`: `Test_acb_rounds_the_acb_removed_half_away_from_zero` (0.05 sell 1 of 2 → 0.03; truncation row), `Test_acb_removes_the_whole_acb_when_a_sale_exceeds_the_pool`, `Test_acb_restarts_from_zero_after_selling_out` (re-buy 10 / 350.00), `Test_acb_pools_only_non_registered_accounts` (registered row, unclassified row, closed non-registered row), `Test_acb_keeps_fractional_shares_exact` (0.5 + 0.25 buys, sell 0.333333), `Test_acb_orders_a_days_buys_before_its_sales` (both src orders), `Test_acb_leaves_out_events_after_today` (Today, Today+1), `Test_acb_counts_a_sale_in_the_calendar_year_of_its_date` (Dec 31 / Jan 1), `Test_acb_reads_the_store_once`, `Test_acb_refuses_a_store_it_cannot_read`, gross-commission control (commission nil sale: outlays 0), `Test_acb_leaves_the_pool_alone_for_actions_it_does_not_walk` (one each of dividend, capital_gain_long, misc_income, add_shares, remove_shares → pool unchanged; S10 flips the last two); nil `Shares` on a buy/sell reads as 0.
- [ ] Step 6: `acb_walk.go` FX, reinvest, split arms + output shape — USD amounts (−amount, proceeds, outlays each) via `money.Convert` at the rate on or latest before the date; reinvest adds `cost_basis` (NULL → units at 0.00, no other mark); split multiplies pool units by new/old once per (security, date) from pool-account rows, ACB unchanged, first ratio by source_id wins when two accounts differ; a nil or non-positive side is `// unreachable:` — the build refuses it (`duckstore.go:460` → `splitRatio` `shares.go:223-240`); `ACBSecurity.PerShare()` nil at 0 shares; securities by name ignoring case then id; sales date, tier, source_id across securities; events carry shares held, ACB and gain after each. Tests: `Test_acb_converts_usd_at_the_rate_on_or_before_the_date` (on-date, latest-earlier, later rate ignored), `Test_acb_splits_and_consolidates_shares_not_acb` (2:1, 1:2, split in a registered account only not applied, buy/split/sell same day), `Test_acb_reinvests_at_quickens_cost` (NULL cost row: units only), `Test_acb_walks_units_of_a_trade_it_cannot_convert` (USD before first rate, EUR account: shares asserted only), `Test_acb_lists_each_securitys_events_and_position` (event fields, per share, zero-share nil, order ties).

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new exported types, `ValueReads` doc names ACB's read, `report/doc.go` package list gains acb

### Verify
- [ ] Step 8: full verification + `spec-check.py phase4de-acb` → tick SCENARIO-08a with its acceptance test; tick SCENARIO-09 with "delivered by SCENARIO-08a" and its folded test; rewrite STATE.md

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `InvestmentHistory` lives on `report.ValueReads`, not a new embed — `report.Store` is at interfacebloat's 10 entries and embeds count as one each.
- One read: `store.InvestmentHistory{Accounts, Securities, Transactions, Rates}` covers every account and every investment transaction with a security, registered included. S12 (file-wide acquisitions and holdings) and S17 (R-6 unclassified refusal) use this read, never a second call.
- Pool = `Classification.Of(a) == false` only. Unclassified accounts never pool (S17 refuses them before the walk).
- Sell: proceeds = amount + commission, outlays = commission, both rounded half away to cents in native currency, then each converted on its own. Gain = CAD proceeds − CAD outlays − ACB removed. Year totals sum the per-sale CAD columns.
- sold ≥ held removes the whole ACB and resets the pool to 0 units / 0.00. This is reachable on the real file until S10 walks add_shares.
- Splits apply once per (security, date) and only from pool-account rows. Reference check (b) compares against non-registered `holdingSpans`.
- Events dated after `ACBRequest.Today` are dropped.
- Ordering:
  - Sales: date, tier, source_id, across securities.
  - Securities: name ignoring case, then id.
  - Events: date, tier, source_id.
- Report amounts are int64 CAD cents and shares are exact `*big.Rat`. Formatting (4-decimal per share, thousands) belongs to 08b's document.

**Left unbuilt** — named so nobody assumes it exists:
- add_shares / remove_shares arms (skipped by the walk; the ordering tier has no add/remove row) — S10
- `incomplete` flag, warning 4, unknown-cost suffix; NULL-cost reinvest currently adds units at 0.00 with no mark — S13a
- No-rate / other-currency arm (units walk, CAD value not pinned), warning 6, out-of-totals — S14
- `ACBRequest` adjustments field (report-owned type, never `config.Adjustment`), ROC/RD events — S11
- Superficial-loss marks, R-6 refusal, `--year` / `--security` selection, CLI/document/JSON — S12, S17, S15/S16, 08b

**Traps** — things that look right and are not:
- With exact rational units, round(ACB × sold ÷ held) at sold == held already equals ACB. The exact-remainder guard is visible only on an oversell row.
- `readRates` probes the fx_rates columns first, so it is two queries. Fault rows index queries with `passQueries`, and a single `rowReads` row faults only the first query.
- cli/mcp fakes embed `report.Store` and compile without the method; only `report/fakes_test.go` needs it.
