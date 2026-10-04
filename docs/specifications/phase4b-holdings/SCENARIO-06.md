---
id: SCENARIO-06
status: open
---

# SCENARIO-06: A holding with no price is listed without value (folds SCENARIO-12: Holdings as JSON)

Cadence: code-first (no mandatory test-first item: no write guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_holdings_no_price_test.go` `Test_run_holdings_lists_a_holding_with_no_price_without_value`
Acceptance test (SCENARIO-12, folded): `cmd/quarry/run_holdings_no_price_test.go` `Test_run_holdings_json_lists_the_unpriced_holding_with_nulls_and_a_warning`
Narrow loop: `go test ./internal/report/... ./internal/cli/ -run 'Holdings|holdings'` and `go test ./cmd/quarry/ -run 'run_holdings'`
Mutation checks: no-price predicate `Price == nil` in `noPriceWarning` → `Test_HoldingsWarnings_does_not_count_a_zero_price_holding`; singular/plural arm → the 1 and 2 rows of `Test_HoldingsWarnings_say_how_many_holdings_have_no_price`; as-of date in the line → acceptance test (clock day 12 differs from real date); left-out-of-total → the mixed priced + unpriced total in the acceptance test
Runs: A (1-2) | B1 (3-4) | V (5-6)
Size: OWNS A RUN — 2 Build batches, 1 feature package (`report` + `document`); production edit is one composer, everything else is pins. Wiring exists: `cli/holdings.go:65-70` already feeds `document.HoldingsWarnings` to both stderr (`emitReport`) and `warnings[]` (`withConfigWarnings`), so no cli/`cmd` production edit.

## Existing surface (survey; nothing to re-plan)
- Text cells already render the unpriced row: `render_holdings.go:106-135` (`holdingPrice` "no price", blank Priced on / Value; pinned `render_holdings_internal_test.go:154`). JSON nulls exist: `document/holdings.go:60-86`, pinned `document/holdings_test.go:90`. Total already skips nil values: `report/holdings.go:61-77`, native `:79+`.
- Missing: `document/holdings.go:106-109` `HoldingsWarnings` returns `[]string{}`.
- Composer owner: `document` (`HoldingsWarnings` beside `SpendingWarnings`, `warnings.go:29`), one function feeding stderr and JSON; not `report` (warnings are rendered copy, sibling precedent). Count `h.Rows` with `Price == nil`; date `h.AsOf.Format(DateLayout)`; counting via `humanize.Count`-style 1/N like `beforeFirstRateWarning` (`warnings.go:103`).

## Copy gap — orchestrator rules before dispatching `A`
S.3 rules only the singular line and "`N holdings have`". The plural tail is not ruled. Plan implements: `N holdings have no price on or before <d>, so they have no value and are left out of the total; enter a price for each in Quicken, then run quarry sync` (N thousands-grouped by `humanize.Count`). If product-vision rules other wording, step 3 and the plural test row change only.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_holdings_test.go:32-64` `seedHoldingsStore` — extract the rows so a new file can add one unpriced security (no price on or before the clock's day 12; give it a price dated day 13 so "on or before" is pinned) and a `--as-of` run; behaviour-neutral, `Test_run_holdings_*` stay green
- [ ] Step 2: `cmd/quarry/run_holdings_no_price_test.go` (new) both acceptance tests via `runWith` + `spendEnvAt(…, holdingsClock())`: text — row has shares, `no price`, blank Priced on / Value / In, `Total` excludes it (mixed priced + unpriced), stderr is exactly `quarry: warning: 1 holding has no price on or before 2026-03-12, so it has no value and is left out of the total; enter a price for it in Quicken, then run quarry sync`, exit 0; JSON — 6-decimal `shares`/`price`, null `price`/`price_date`/`value`/`converted_value`/for the unpriced row, `holdings`/`totals`/`account_filter` present, `warnings` equals that line (no prefix), stdout read back with a tagged struct (`musttag`). Price column widens to 8 ("no price"): `holdingsLine` (price `%6s`) cannot be reused, give the new file its own line helper. Both red at their assertion (stderr/warnings empty)

### Build
- [ ] Step 3: `internal/report/document/holdings.go:106-109` `HoldingsWarnings` + unexported `noPriceWarning` (singular arm `holding has … it has … for it`, plural arm per the copy gap) + `document/holdings_test.go:185`: `Test_HoldingsWarnings_say_how_many_holdings_have_no_price` (0 → `[]string{}` kept; 1; 2; 1,000 grouped), `Test_HoldingsWarnings_does_not_count_a_zero_price_holding` (priced at 0: no warning, control beside a nil-price row), the date is `h.AsOf` not the clock, unpriced counted whatever its currency (nil, CAD, EUR), account closed, or shares sign, and the same in native mode
- [ ] Step 4: `internal/cli/holdings_test.go:276-334` pins at the command boundary on `fakeReportStore`: config warning precedes the no-price line on stderr and in `warnings[]` (R3 slot 1 before slot 4; later slots are S07/S08's pins); mixed priced + unpriced text total excludes the unpriced row in converted and `--currency native` modes (native: a per-currency total exists only from priced rows, and an all-unpriced listing has no `Total` row); `holdingsDoc` (`:45-62`) gains `price_date` for the null read-back

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comment on `HoldingsWarnings` says the slot it fills (4) and the rest are the later scenarios'

### Verify
- [ ] Step 6: full verification block + `.claude/scripts/spec-check.py phase4b-holdings`; tick SCENARIO-06 and SCENARIO-12 (line "delivered by SCENARIO-06", test reference last) in `specification.md`; rewrite STATE.md (move "no price warning and JSON row pin" out of Left unbuilt; add the plural wording and composer-owner decisions)

## Handoff

**Binding decisions:**
- `document.HoldingsWarnings` stays the single composer owner and takes `report.Holdings`; S07/S08/S09/S10 each add one helper and append in their R3 slot (4 is built: no price, then 5 no rates / before first rate, 6 no currency, 7 other currency, with 2 and 3 prepended). The row set is the order within a kind — table sort
- "No price" means `store.Holding.Price == nil`; a priced zero is not unpriced (value `0.00`, in total, no warning), so S07's "no currency" and S08's "no rate" must not reclassify on `Value == nil`
- The date in the line is the as-of day (`h.AsOf`), never the clock

**Left unbuilt:** `not converted` cell, no-currency and other-currency warnings — S07; before-first-rate and no-rates warnings, `no rate` cell, unconverted totals — S08; empty-result warnings and non-investment `--account` warnings — S09/S10; MCP `holdings` — S13.

**Traps:**
- No-price cell makes the Price column 8 wide: helpers with a fixed `%6s` price column (`run_holdings_test.go:16-25`) misalign a fixture holding one; give the new tests their own widths
- Unpriced row also has a nil `Value`, so a total or warning keyed on `Value == nil` conflates unpriced with nothing; key on `Price`
- Test clock day 12 vs `current_date`: the "price dated after as-of" fixture row only proves `on or before` if the view's through-today arm reaches that date (real date is later than 2026-03-13)
