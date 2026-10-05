---
id: SCENARIO-14
status: open
---

# SCENARIO-14: ACB data-quality warnings

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_acb_warnings_test.go` `Test_run_acb_warns_of_a_no_rate_trade_a_shared_ticker_and_a_december_sale`
Narrow loop: `go test ./internal/report/... && go test ./cmd/quarry/ -run 'Test_run_acb'`
Mutation checks: year-totals exclusion (`walkACB` keeps a no-rate security's sales) → `Test_acb_leaves_a_no_rate_securitys_sales_and_excess_out_of_the_years`; Dec 24 lower bound (`>= 24` → `>= 25`) → `Test_ACBWarnings_dates_a_december_sale_warning_by_the_24th_to_the_31st`; shared-ticker predicate (case-fold) → `Test_acb_shared_tickers_match_exactly_and_ignore_an_empty_ticker`
Runs: A (1) | B1 (2-3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 4 batches (spec sized 3; the shared-ticker extraction is the 4th), report + report/document (one feature tree), cli/cmd tests only, no new production edit outside `internal/report`

## Implementation Plan

Survey: `toCAD` (`acb_walk.go:412-418`) drops `money.Convert`'s `ok`; that bool is the single "cannot value" signal (USD before the first rate, USD with no rates, non-CAD/USD, empty currency). Callers: `apply` :224,:234,:236 and `sell` :387-388. Re-pin inventory (grep, control `acb_arms_test.go:137-139` found): no existing year/totals pin has a trade before its first rate (`run_acb_test.go:50-55` rates from 2024-01-02, VTI buy 2024-05-01; `acb_test.go:119-135` first rate 2023-12-29, VTI buy 2024-01-02; `acb_events_test.go:62-71` pins a no-rate event, events stay untouched). Misses: literal `store.InvestmentTransaction{}` fixtures with no Currency now become a cause; `go test` will list them.

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_acb_warnings_test.go` new `Test_run_acb_warns_of_a_no_rate_trade_a_shared_ticker_and_a_december_sale` — `runWith` + `replaceStoreWithRates` (`run_sql_fx_test.go:32`), `acbTrade` (`run_acb_test.go:28`), `usdRate`, `holdingsClock()`. Fixture: non-registered USD brokerage; VTI buys (no-rate trade before the first rate) and a part-sale so it is still held; a second security with ticker VTI; a counted CAD sale; a sale dated Dec 28. Asserts stdout (year table without the no-rate sale, kept sale counted, position row `incomplete`) and stderr warnings 6, 7, 9 in that order, exit 0. Keep one counted sale (empty-years output is S15/S18). Fails at its assertion: no code needed first.

### Build
- [ ] Step 2: report walk — `acb.go:35-43` `ACB` gains `FirstRate time.Time` (zero = no rates, from `history.Rates[0]`); `acb.go:110-118` `ACBSecurity` gains `NoRate *ACBNoRate{Date, Currency}` (earliest trade the walk could not convert; fix :115 doc); `acb_walk.go:171-185` `securityWalk` field, `:217-262` `apply` sets it from a `needsConversion(tx)` predicate (buy, sell, and add_shares/reinvest with non-nil cost; split, remove_shares and a nil-cost add never) and `convertible`; `:211` ORs the cause into `Incomplete` (never reassigns); `:75-80` `walkACB` appends `walk.sales`/`walk.excesses` only when `NoRate == nil`, BEFORE `markSuperficialLosses` :89 and `acbYears` :90. Tests `internal/report/acb_no_rate_test.go` new + extend `acb_arms_test.go:131-152` (keep EUR row): sale and ROC excess of the security leave `Years` and `ReturnOfCapitalGain` while the ROC event keeps `Realized`; another security's sale stays; no rates at all; trade on the first rate date is NOT a cause, day before is; EUR trade is a cause; nil-cost USD add before first rate is not; remove_shares/split not; no-rate sale never superficial-marked even at a spurious loss, with a same-ticker re-buy fixture; sold-out-then-rebought security stays out (security-wide); `Incomplete` true while held.
- [ ] Step 3: `document/acb_warnings.go:18-25`, new `noRateWarnings` after `removalWarnings` (:105-122), before `returnOfCapitalWarnings` (:124) — one line per security (walk order), date = `NoRate.Date`, `DateLayout`. Variant with rates: `"<security>" has a USD trade on <d>, before <first>, the first exchange rate in the store, so its ACB is incomplete and its gains are left out of the year totals`. Variant `FirstRate` zero (EXPANSION of the spec's ellipsis, orchestrator to confirm): `"<security>" has a USD trade on <d>, and the store has no exchange rates, so its ACB is incomplete and its gains are left out of the year totals; run quarry sync to fetch rates`. `"%s"` quoting like warnings 5 and 8. A non-USD `NoRate.Currency` emits NO line (no copy ruled; do not invent). Tests `acb_warnings_test.go`: both variants verbatim, two securities in name order, a security with no `NoRate` silent. Plus `cmd/quarry/run_acb_warnings_test.go` `Test_run_acb_json_leaves_a_no_rate_security_out_of_the_years_and_warns` (`--json`: `years` without the sale, `sale_count`, `return_of_capital_gain` without the excess, `incomplete` true, `warnings[]` carries line 6).
- [ ] Step 4: shared tickers — `acb_superficial.go:44-50` extract the one predicate (exact, case-sensitive, non-empty ticker) used by `newSuperficialIndex` and a new `(ACB).SharedTickers() []ACBSharedTicker{Ticker, Securities}` over `a.Securities` (groups of >=2, ordered by first member in walk order, members in walk order). `document/acb_warnings.go` `sameTickerWarnings` after `noRateWarnings`: `"<ticker>" is <n> securities in Quicken (<name1>, <name2>); quarry keeps a separate ACB for each; if they are the same, merge them in Quicken` (names ", "-joined, unquoted). Tests `internal/report/acb_tickers_test.go` new (`Test_acb_shared_tickers_match_exactly_and_ignore_an_empty_ticker`: `vti` vs `VTI` no group, nil and empty ticker none, three members n=3, two groups ordered, a security held only in a registered account is not in `a.Securities` so not counted) and `acb_warnings_test.go` (line copy, n=3).
- [ ] Step 5: `document/acb_warnings.go` `decemberSaleWarnings` last in `ACBWarnings`, over `a.Years[].Sales` (excluded sales are absent, so never warned): one line per year ascending, `humanize.Count(n, "sale", "sales")`, `N sale(s) dated December 24–31, <year>: a sale settles a day or two after its trade date and counts for tax in the year it settles; check its date on your T5008` (en dash U+2013). Tests: `Test_ACBWarnings_dates_a_december_sale_warning_by_the_24th_to_the_31st` rows Dec 23 no, Dec 24 yes, Dec 31 yes, Jan 1 no, Nov 30 no, 2 sales one year plural, two years two lines ascending; `Test_ACBWarnings_orders_slots_1_and_3_to_9` hand-built report with every slot present asserting exact sequence (adjustment, 3, 4, 5, 6, 7, 8, 9); slot 2 (S15) goes between adjustment lines and 3 at `acb_warnings.go:19-20`. Control: `Test_ACBWarnings_is_empty_..._and_no_loss_is_marked` and `acbDocumentFixture` (`document/acb_test.go:20-60`, sales Mar 4 and Apr 5, no shared ticker) must stay empty.

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `NoRate`, `FirstRate`, `SharedTickers`; re-pin any literal fixture without a Currency that `go test` lists.

### Verify
- [ ] Step 7: full verification per `.claude/rules/agent-briefs.md`, `.claude/scripts/spec-check.py phase4de-acb`, tick SCENARIO-14 (`specification.md:578`) with its acceptance test, rewrite STATE.md (Left unbuilt: no-rate arm done; add the open items below), `status: done`.

## Defaults pending ruling (orchestrator: rule 1 BEFORE B1; the rest may ride)
1. A non-USD trade `Convert` cannot value (EUR; empty currency): default incomplete + out of totals, NO warning line (copy 6 says "before the first exchange rate", false for it). Recommend a scoped product-vision copy ruling; if it rules USD-only, only `needsConversion` changes.
2. Warning 6 one line per security, earliest trade date; `<USD>` = the trade's currency code.
3. Exclusion is security-wide and permanent (all years, ROC excess, no clearing on sell-out like S13a's span); `Incomplete` follows it. Sell-out/re-buy row unruled.
4. Excluded sales leave `sales[]`, `sale_count`, superficial marks (closes S12's open item) and warning 9. Their events stay in `securities[].events` with `cad` `"0.00"`, `usd_cad` null, `gain` as computed (shape is ruled non-null); recommend final pass reviews.
5. Warning 9 one line per year, ascending. Warning 7 one line per shared ticker, first-member order; slot order 6, 7, 8, 9 after 5.

## Handoff

**Binding decisions:**
- `ACBSecurity.NoRate *ACBNoRate` and `ACB.FirstRate` are additive; `Incomplete` = unknown-cost span OR `NoRate != nil` — S15/S16/S19 read these, never recompute.
- A no-rate security's sales and ROC excess never reach `Years`; warning 8 reads events and still fires.
- The ticker predicate behind `(ACB).SharedTickers` (exact, case-sensitive, non-empty) is the one definition, shared with `newSuperficialIndex` — S12's rule, binding.
- `ACBWarnings` stays the one composer; slots 6, 7, 9 and the slot-2 insertion point as in Step 5.

**Left unbuilt:** warning 2 and the `--year` forms — S15/S18; `--security` text of a no-rate event's blank Rate — S16; any non-USD warning line — pending ruling 1.

**Traps:**
- S15's `--year` cut must build warnings 4, 6, 7, 9 from the UNCUT report, and warning 9's lines are per year.
- Years can now be empty while `Securities` is not (all USD pre-rate): S18's emptiness rule must test years, not securities.
- `acbSecurity` helper (`acb_test.go:35-37`) sets Ticker = name: two same-name test securities trigger warning 7.
- `acbYears` copies sales by value, so the exclusion must happen before it, in `walkACB`.

**Orchestrator: product-vision ruling 2026-10-05 recorded in spec warning 6 — supersedes this plan: `needsConversion` = trade currency is not CAD (confirm a trade's currency column copies its account's, `internal/store/duckstore/investments.go:86`); Step 3 adds variant 6c (EUR and empty-code rows), drops "emits NO line"; unconverted event JSON `cad` null and `gain` null (document `CAD *string`), report side carries it as an ADDITIVE field (e.g. `ACBEvent.Unvalued bool`) — never retype `ACBEvent.CAD` (STATE binding: 08a fields never retyped); `Test_acb_gives_a_usd_event_with_no_rate_on_file_a_zero_rate` (`internal/report/acb_events_test.go:62-71`) re-asserts not-converted and is renamed; sell-out/re-buy row stays excluded; defaults 2, 3, 5, 6 confirmed; 6b text confirmed.**

## Phase report

Run A (step 1) done; acceptance test red, nothing else touched (no production code, no stubs needed).

- `cmd/quarry/run_acb_warnings_test.go` new: `Test_run_acb_warns_of_a_no_rate_trade_a_shared_ticker_and_a_december_sale`, helper `warningsPositionLine`, `noRateSharedTickerRows`, consts for warnings 6a, 7, 9.
- Fixture: USD + CAD non-registered brokerages; `Vanguard Total Stock` (USD, VTI) bought 2023-12-01 (before first rate 2024-01-02), part-sold 2025-12-29; `Vanguard Total Stock CAD` (CAD, ticker VTI) bought 2025-03-03, part-sold 2025-12-28. The no-rate sale is in December ON PURPOSE: warning 9 must say `1 sale` (excluded sale not counted).
- Red (assertion, both non-fatal asserts): stdout year row actual `2025  2  1,225.00  0.00  400.00  825.00` vs expected `2025  1  600.00  0.00  400.00  200.00`; position row lacks suffix `incomplete`; stderr actual empty vs the three warnings 6a, 7, 9 in that order. Exit 0 holds.
- Pinned by me, unruled: the no-rate position row shows ACB `0.00` and per share `0.0000` (buy unconverted, so pool ACB 0; matches today's output). Orchestrator confirm.
- Next (B1 = steps 2-3): `ACBNoRate`, `FirstRate`, exclusion in `walkACB`, `noRateWarnings` (6a/6b/6c per ruling block, `needsConversion` = non-CAD); additive `ACBEvent.Unvalued`.

