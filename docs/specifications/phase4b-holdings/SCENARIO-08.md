---
id: SCENARIO-08
status: open
---

# SCENARIO-08: A day with no exchange rate totals each currency separately

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_holdings_no_rate_test.go` `Test_run_holdings_before_the_first_rate_shows_no_rate_and_totals_usd_separately`
Narrow loop: `go test ./internal/report/... ./internal/store/duckstore/ ./internal/cli/ -run 'Holdings|NeedsRate'` and `go test ./cmd/quarry/ -run 'run_holdings'`
Mutation checks: same-currency exemption in `(Holdings).NeedsRate` → `Test_holdings_needs_rate_is_false_for_a_row_in_the_reporting_currency`; `report.Convertible` guard in `NeedsRate` → `Test_holdings_needs_rate_is_false_for_a_security_quarry_does_not_convert`; no-rates-wins branch in `noRateWarnings` → `Test_HoldingsWarnings_say_the_store_has_no_rates_when_it_holds_none`; `Total` row picked by currency, not `Totals[0]` (`renderHoldings`) → `Test_holdings_total_rows_list_the_unconverted_total_when_nothing_converts`
Runs: A (1) | B1 (2-3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 3 batches (facts + decision + totals, warning slot 5, renderer), 1 feature package (`report`; `duckstore` adapter, `cli`, `cmd/quarry` follow it)

## Implementation Plan

Decisions (binding for this plan; sources: spec S.2/S.3/S.6, STATE.md):
- **A row "needs a rate"** (new `(Holdings).NeedsRate(store.Holding) bool`, `report/holdings.go`): converted mode (`Currency != money.Native`), `Price != nil`, `Convertible(h)`, `*h.Currency != l.Currency.String()`, and `l.Converted(h) == nil`. Never `value_cad IS NULL`/`USDCAD == 0` alone: CAD in CAD mode and USD in USD mode need no rate; NULL/EUR/unpriced are other arms. One owner, shared by cell, composer and totals.
- **One fact added to the port call:** `store.Holdings.FirstRate time.Time` (zero = store has no rates; "store has rates" is `!FirstRate.IsZero()`), read by `firstRate(ctx, db)` (`duckstore/charges.go:75`) on the same `openRead` handle after the rows query. No second port call.
- **Warning choice (slot 5):** zero rows need a rate → none. Else `FirstRate.IsZero()` → no-rates line (wins); else the before-first-rate line. N = rows needing a rate (not distinct securities). Native mode, rate gap, as-of after last rate: no row needs a rate, so silent by construction (no extra branch).
- **Copy:** before-first-rate singular `1 holding valued on <d>, before <first>, the first exchange rate in the store, is not converted to <reporting> and is totalled in <other>`; plural `N holdings valued on …, are not converted to <reporting> and are totalled in <other>` (`humanize.Count(n, "holding", "holdings")`, other = `money.NativeOf(l.Currency)`). No-rates line verbatim from S.3 (`…listed in each security's own currency; run quarry sync to fetch them`). New consts; do not reuse `beforeFirstRateWarning` (`document/warnings.go:103`) or `noRatesWarning` (`:14`): wording differs.
- **Totals:** `total()` returns converted total (reporting currency, only if a row converts) then, appended, one total per unconverted currency summing `row.Value` of NeedsRate rows (a priced zero counts). Text and `--json` `totals` follow that order.

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_holdings_no_rate_test.go` (new) `Test_run_holdings_before_the_first_rate_shows_no_rate_and_totals_usd_separately` — `seedHoldingsStore` (`run_holdings_test.go:43`, first rate day 10), `--as-of 2026-03-09`; asserts exact stdout (VTI In cell `no rate`; Acme and Maple are CAD rows and convert, so the converted `Total` row, then a `USD` Total row below it carrying 24,659.35) and the exact stderr singular warning line, exit 0. Own line helper (trap: In column width). Compiles against today's code; must fail at the stdout assertion.

### Build
- [x] Step 2 (B1, batch 1: facts + decision + totals): `internal/store/store.go:164-168` `Holdings.FirstRate` (+ doc); `internal/store/duckstore/holdings.go:29-51` `(*Store).Holdings` read `firstRate` after the rows; `internal/report/holdings.go:26-31,58-84` `Holdings.FirstRate`, `NeedsRate`, `total()` appends unconverted totals (update `HoldingsTotal`/`total` docs). Tests: `duckstore/holdings_test.go` first-rate returned / zero with no rates, fault tests for the `SELECT min` query (`spyReadDB{passQueries: 1, queryFault}` per `charges_test.go:297`) and its scan; `report/holdings_test.go` NeedsRate table (USD-in-CAD true; CAD-in-CAD, USD-in-USD, EUR, NULL currency, unpriced, native, rate present each false), totals (USD-only total when nothing converts; converted-then-unconverted order; priced zero makes a total; unpriced does not; USD reporting mode gives a CAD total); fake store whose second `Holdings` call differs cannot mix (one call).
- [x] Step 3 (B1, batch 2: slot 5): `internal/report/document/holdings.go:109-118` `HoldingsWarnings` + new unexported `noRateWarnings(h)` appended after `noPriceWarning`, before `noCurrencyWarnings` (fix the doc comment's slot list). Tests in `document/holdings_test.go`: singular, plural (N=2), swapped currencies in USD mode, no-rates line when `FirstRate` zero (wins), silent when no row needs a rate (CAD row in CAD mode before first rate; rate gap and as-of after last rate = `FirstRate` ≤ as-of with converted rows; native mode), EUR/NULL rows not counted, order pin with no-price + no-rate + NULL-currency rows, JSON `warnings[]` equals stderr list.
- [x] Step 4 (B2, batch 3: renderer): `internal/cli/render_holdings.go:16-20,31-58,139-151` `holdingNoRateCell` (own const; `noRateCell` is taken), `holdingInCell` arm after the `Convertible` arm using `l.NeedsRate(h)`; `renderHoldings` picks the converted Total by `Currency == h.Currency` (not `Totals[0]`) and renders an unconverted total row with Currency at `width-3` and Value at `width-2` (In cell blank), generalising `holdingsNativeTotalRow` (native stays `width-2`/`width-1`). The old blank fallback arm becomes unreachable: mark `// unreachable:` with reason or remove it. Tests in `internal/cli/holdings_test.go` via `fakeReportStore`: In-cell arms (no price blank, not converted, no rate, converted), converted + USD Total rows, only-unconverted Total with no converted Total, USD-mode CAD Total row, native unchanged.
- [x] Step 5 (B2, tests only: edge cells crossed with formats; `cmd/quarry/run_holdings_no_rate_test.go`): `--json` (`totals` order, `converted_value` null, `warnings[]` line), `--currency USD --as-of` before first rate (CAD row `no rate`, CAD total, warning names `USD`/`CAD`), `--currency native` before first rate (no `no rate`, no warning, per-currency totals), store with no rates (no-rates line, `no rate`), rate gap (rates day 10 and 12, `--as-of 2026-03-11`: converted, silent), CAD-only holding before first rate in CAD mode (no `no rate`, no warning). After-last-rate silence is already pinned by `Test_run_holdings_lists_todays_holdings_in_the_reporting_currency`; do not duplicate.

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; refresh doc comments naming the old totals/slots (`renderHoldings`, `HoldingsWarnings`, `store.Holdings`); the full suite may redden an older fixture holding a USD row with no rate.

### Verify
- [ ] Step 7: full verification + `spec-check.py phase4b-holdings` → tick SCENARIO-08 with its acceptance test; rewrite STATE.md.

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `store.Holdings.FirstRate` (zero = no rates) is the only new fact on the one port call — S09's empty-result warning and S13 MCP read the same call; no second read.
- `(Holdings).NeedsRate` is the single "no rate" predicate (cell, slot 5, unconverted totals) — S10's `--account` and S13 reuse it.
- `HoldingsTotal` order is converted first, then unconverted (reporting-currency total absent when nothing converts) — `--json` `totals` and text rows both depend on it; no new JSON key.
- Slot 5 sits after no-price, before no-currency (R3); the no-rates line beats before-first-rate when `FirstRate` is zero.

**Left unbuilt** — named so nobody assumes it exists:
- empty-result warnings / "no holdings on" (S09), `--account` caption + `account_filter` (S10), MCP `holdings` (S13), SKILL/schema.md copy (S15).

**Traps** — things that look right and are not:
- `renderHoldings` takes `h.Totals[0]` as the converted total today — wrong once the first total can be the unconverted one.
- In a converted table the Value column is `width-2` (In is last); `holdingsNativeTotalRow` offsets (`width-2`/`width-1`) are native-only.
- `value_cad IS NULL` / `USDCAD == 0` also mean no price, NULL/EUR currency or same-currency USD row: never use them alone as "no rate".
- `beforeFirstRateWarning`/`noRatesWarning` (`document/warnings.go`) and `accountsFXWarnings` (`cli/fx_warning.go`) have different wording and subjects; copying them ships unruled copy.

## Phase report

Runs B1 (steps 2-3) and B2 (steps 4-5) done. Steps 6-7 (V) remain.

B2 production: `internal/cli/render_holdings.go` const `holdingNoRateCell`, `holdingCurrencyColumn`/`holdingValueColumn` (5/6, same in native and converted); `renderHoldings` loops every `Totals` entry — the one whose currency is the reporting currency is the In-column `holdingsTotalRow`, every other is `holdingsCurrencyTotalRow` (replaces `holdingsNativeTotalRow`); `holdingInCell` gains the `NeedsRate` arm, its trailing `return ""` is marked `// unreachable:`.
B2 tests: `internal/cli/holdings_no_rate_test.go` (no-rate cell arms, no-`no rate` arms, converted + USD total order, only-unconverted total, USD-mode CAD total; native Total shape stays pinned by the existing native tests); `cmd/quarry/run_holdings_no_rate_test.go` adds JSON (totals order, null `converted_value`, `warnings[]` == stderr line), `--currency USD`, `--currency native`, store with no rates, rate gap, CAD-only in CAD. Steps 5's cmd tests were green on arrival: the behaviour came with B1 + step 4.
B2 mutation (restored): `total.Currency == h.Currency.String()` -> `== h.Totals[0].Currency` in `renderHoldings`: `Test_holdings_total_rows_list_the_unconverted_total_when_nothing_converts` red (both assertions). All four plan mutation checks now run.
B2 state: acceptance test and narrow loops green (`go test -count=1 ./cmd/quarry/ -run run_holdings`, `./internal/report/... ./internal/store/duckstore/ ./internal/cli/` Holdings subset); `golangci-lint run ./internal/cli/... ./cmd/quarry/...` 0 issues. Full suite, coverage gate, `go build ./...`, spec-check, tick and STATE.md remain for V.

B1 report:

Production:
- `internal/store/store.go:164-168` `Holdings.FirstRate`; `duckstore/holdings.go` reads `firstRate(ctx, db)` after the rows on the same handle.
- `internal/report/holdings.go` `Holdings.FirstRate`, `(Holdings).NeedsRate`, `total()` = converted total then one unconverted total (`money.NativeOf(l.Currency)`), each only if a row feeds it.
- `internal/report/document/holdings.go` `noRateWarnings` (slot 5, after no-price, before no-currency), const `holdingsNoRatesWarning`; `HoldingsWarnings` doc slot list updated.

Tests: `duckstore/holdings_test.go` (first rate, none, query fault, scan fault); `report/holdings_needs_rate_test.go` (NeedsRate arms, totals, FirstRate pass-through; one-call pin is the existing reads==1 test); `document/holdings_no_rate_test.go` (singular, plural by rows, USD mode, no-rates wins, silent cases, order pin with EUR/NULL not counted). Existing fixture changed: `document/holdings_test.go` "CAD and USD securities have no line" now gives its USD row a `ValueCAD` (an unconverted USD row now earns the rate line).

Mutations (all restored, byte-identical):
- drop `*h.Currency != l.Currency.String()` in NeedsRate: `Test_holdings_needs_rate_is_false_for_a_row_in_the_reporting_currency` red (both subtests).
- `Convertible(h)` -> `h.Currency != nil`: `Test_holdings_needs_rate_is_false_for_a_security_quarry_does_not_convert/another_currency` red.
- `h.FirstRate.IsZero() && false` in noRateWarnings: `Test_HoldingsWarnings_say_the_store_has_no_rates_when_it_holds_none` red.

Do not redo B1 files.
