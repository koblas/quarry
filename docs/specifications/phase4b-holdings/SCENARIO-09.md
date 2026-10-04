---
id: SCENARIO-09
status: open
---

# SCENARIO-09: Nothing held on the day

Cadence: code-first (no write-safety guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_holdings_empty_test.go` `Test_run_holdings_before_the_first_investment_transaction_warns_where_they_start_and_prints_no_total`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/... ./internal/cli/ -run 'Holdings'` and `go test ./cmd/quarry/ -run 'run_holdings'`
Mutation checks: span query's `account_id IN` scope in `holdingsSpan` → `Test_holdings_reads_the_transaction_span_of_only_the_named_accounts`; `len(h.Rows) == 0` gate in `emptyWarnings` → `Test_HoldingsWarnings_say_nothing_about_an_empty_result_when_something_is_held`; variant key `h.Accounts != nil` → the named-accounts rows of `Test_HoldingsWarnings_word_an_empty_result_by_what_the_store_holds`; slot 3 placed before slot 2 → `Test_run_holdings_of_only_a_non_investment_account_prints_both_warnings_in_order`
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 batches (port span, composer, cross-pins + stale tests), 1 feature package; renderer needs no production edit (header-only table, no Total, `[]` JSON already pinned: `internal/cli/holdings_test.go:466-476`, `cmd/quarry/run_holdings_account_test.go:110-116`)

Surveyed (grep, production): `store.Holdings` is built in one place, `duckstore/holdings.go:26`; read by `report/holdings.go:78` only; two fakes return it (`report/fakes_test.go:96`, `cli/fakes_test.go:70`), neither breaks on new fields. Slot 3 goes in `document.HoldingsWarnings` (`document/holdings.go:114-122`) between `nonInvestmentWarnings` and `noPriceWarning`.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_holdings_empty_test.go` `Test_run_holdings_before_the_first_investment_transaction_warns_where_they_start_and_prints_no_total` — `holdingsRows()` (`run_holdings_test.go:51`) with `inv-maple` moved to `holdingsDay(5)` so the span is `2026-03-02 to 2026-03-05`; `--as-of 2026-03-01`, clock `holdingsClock()`; stdout exactly caption `Holdings on 2026-03-01 in all accounts, amounts in CAD; cash not included` + blank line + header-only line, no Total; stderr exactly `quarry: warning: no holdings on 2026-03-01; the store's investment transactions run 2026-03-02 to 2026-03-05\n`; exit 0. Red at the stderr assertion

### Build
- [ ] Step 2: `store/store.go:165-171` `Holdings` + `duckstore/holdings.go:26-60` `(*Store).Holdings` + new `holdingsSpan` beside it (pattern `charges.go:75` `firstRate`) — add `FirstTransaction, LastTransaction time.Time` (UTC midnight, zero when none in scope) read by `SELECT min(date), max(date) FROM investment_transactions` plus ` WHERE account_id IN (marks(1))` when `params.AccountIDs` is non-empty (no day parameter here, ids start at `$1`; never `accountFilter.and`, which hard-codes `$3`), on the same handle after `firstRate`. Every investment-transaction row counts, whatever its action or security; future-dated ones too. Tests in `duckstore/holdings_test.go` (fixtures `holdingRows`, `buy`, `newStoreWith`, `marchDay`) and `holdings_account_test.go`: span of all accounts; span with none (both zero); `Test_holdings_reads_the_transaction_span_of_only_the_named_accounts` (other account's earlier and later rows excluded; id naming no account gives zero); span independent of the as-of day (a day before the first row still returns it). Fault tests: span query fault (`spyReadDB{passQueries: 2, ...}`) and span scan fault, each `assertOtherFault`; the existing `passQueries: 1` fault tests still hit the first-rate query
- [ ] Step 3: `report/holdings.go:29-39,78` `Holdings`, `(*Server).Holdings` — carry both span fields from the one port read into `report.Holdings`; `internal/report/holdings_test.go` test with the fake store returning spans, one read (`holdingsReads`). `document/holdings.go:112-142` new unexported `emptyWarnings(h)` appended after `nonInvestmentWarnings`, before `noPriceWarning`: none unless `len(h.Rows) == 0`; `h.Accounts == nil` store-wide wording, else the `in the named accounts` wording; `FirstTransaction.IsZero()` selects the no-transactions tail (store-wide: `no holdings on <d>; the store has no investment transactions`; named: `no holdings on <d> in the named accounts; they have no investment transactions`), else the span tail with `DateLayout` dates (`the store's investment transactions run <a> to <b>` / `their investment transactions run <a> to <b>`); `<d>` is `h.AsOf`, never the clock. Update the slot list in the `HoldingsWarnings` doc. Tests in `document/holdings_empty_test.go`: `Test_HoldingsWarnings_word_an_empty_result_by_what_the_store_holds` table (store-wide span; store-wide none; named span; named none; `a == b` span prints `a to a`; a day after everything was sold, same span line); `Test_HoldingsWarnings_say_nothing_about_an_empty_result_when_something_is_held`; named non-investment account only gives its slot 2 line then slot 3 line
- [ ] Step 4: cross-pins and stale tests. New `cmd/quarry/run_holdings_empty_test.go` rows (each its own test, helper-free header-only expectation): no investment data at all (`rows.InvestmentTransactions = nil`, `--as-of 2026-03-01`) gives `no holdings on 2026-03-01; the store has no investment transactions`; `--account Brokerage --as-of 2026-03-01` gives the named wording with only Brokerage's span (`2026-03-02 to 2026-03-02`, scope-discriminating because Maple is on day 5); `--currency native` empty (caption without `, amounts in`, no In column, no Total, same stderr line); `--json` empty (`holdings` `[]`, `totals` `[]`, `warnings` equal to the stderr line, `account_filter` `[]`); `Test_run_holdings_of_only_a_non_investment_account_prints_both_warnings_in_order` (`--account Chequing`: slot 2 line then `no holdings on 2026-03-12 in the named accounts; they have no investment transactions`). Fix the stale tests: `run_holdings_account_test.go:110-116` (assert both stderr lines, not the `^` regexp) and `:119-125` (assert exact stderr: only the slot 3 named line, no `not a brokerage`); `internal/cli/holdings_test.go:278-284` config-warning test gives `fakeReportStore` `brokerageHolding()` so stderr stays the one config line (separate cli test for config line first, empty line second); `document/holdings_test.go:186-188` `Test_HoldingsWarnings_is_empty_not_nil` uses a holding row; `document/holdings_account_test.go:36-58` (table wants the slot 3 line appended) and `:70-83` (give it a priced row so `Empty` still holds). Grep `report.Holdings{` and `store.Holdings{` across tests for any other empty-rows case asserting exact warnings

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on the new fields (one line each) and `emptyWarnings`

### Verify
- [ ] Step 6: full verification per `.claude/rules/agent-briefs.md`, `spec-check.py phase4b-holdings`, tick SCENARIO-09 with its acceptance test, rewrite STATE.md (remove slot 3 and span from Left unbuilt, drop the stale-stderr trap)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- "Empty" means `len(h.Rows) == 0`, whatever the reason (before first trade, all sold, wrong account, non-investment account) — S.6 gives one row for it, and one composer owns it (`emptyWarnings`, slot 3)
- Variant is keyed on `h.Accounts == nil` (store-wide) versus named, never on account types; the tail is keyed on `FirstTransaction.IsZero()`. So `--account` with no investment data at all uses `they have no investment transactions`, and a named non-investment account with no transactions prints slot 2 then slot 3 — S.3 ruled both warnings print
- Span = min and max `investment_transactions.date` of every row in scope (all actions, cash-only rows included, future-dated included), scoped by the same `AccountIDs`, not the holding spans and not price dates — it describes the data the store has, so it does not move with `--as-of`
- `store.Holdings.FirstTransaction/LastTransaction` ride the one existing port call; S13's MCP reuses them via `report.Holdings`, no second read
- A one-day span prints `<a> to <a>`; no special wording

**Left unbuilt** — named so nobody assumes it exists:
- MCP `holdings` and its `as_of` / `accounts` arguments — SCENARIO-13
- `holding_shares` / `v_holdings` SQL-conventions sentences, SKILL.md and `--account` reference copy — SCENARIO-15

**Traps** — things that look right and are not:
- The span query text also begins `SELECT min`: fault tests key on `passQueries` (rows 0, first rate 1, span 2), not on the SQL substring
- `accountFilter.and` hard-codes `$3` and `holdingsQueryFor` binds the day as `$1`; the span query has no day, so its ids start at `$1`: build it with `marks(1)`
- Exact-empty stderr tests with an empty-rows fixture go stale (list in Step 4); a fixture meaning "no warning" needs a priced row
