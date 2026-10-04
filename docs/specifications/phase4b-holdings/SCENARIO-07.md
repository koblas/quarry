---
id: SCENARIO-07
status: open
---

# SCENARIO-07: A security quarry cannot convert is left out of the total

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_holdings_not_converted_test.go` `Test_run_holdings_leaves_a_security_it_cannot_convert_out_of_the_total`
Narrow loop: `go test ./internal/report/... ./internal/cli/ -run 'Holdings|holdings'` and `go test ./cmd/quarry/ -run 'run_holdings'`
Mutation checks: EUR-warning mode guard in `otherCurrencyWarnings` (`h.Currency == money.Native`) → `Test_HoldingsWarnings_say_which_securities_have_no_conversion` native row; one-line-per-security dedup → same test, two-account row; `Price == nil` arm of `holdingInCell` → no-price NULL row in `Test_holdings_in_column_says_not_converted_for_a_priced_security_it_cannot_convert`
Runs: A (1) | B1 (2-3) | V (4-5)
Size: OWNS A RUN — 2 batches, 1 feature package (`report`; `cli`/`document` are its renderers)

Surveyed (existing, no change): H-3 `report/holdings.go:61-77` `total` already skips rows whose `Converted` is nil, and `nativeTotals` `:81-103` skips NULL currency and totals EUR in its own entry (CAD, USD, rest alphabetical) — S07 only pins them. `holdingCurrency` already prints `none`.

Rulings made here (binding below): security named by stored name via `%q` (not the S.2 label: `securityLabel` lives in `cli`, `document` cannot import it); no-price NULL/EUR row; one line per distinct `SecurityID`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_holdings_not_converted_test.go` `Test_run_holdings_leaves_a_security_it_cannot_convert_out_of_the_total` — seed `holdingsRows()` plus a NULL-currency and an EUR-priced security (each bought and priced, `replaceStoreWithRates`, `holdingsClock`); CAD mode: both rows `not converted`, Total unchanged, stderr exactly the two ruled lines NULL then EUR. Also in this file, same fixture: `--json` (`converted_value` null, `warnings[]` same two lines in that order, `totals` one CAD entry); `--currency native` (no In column; `Total` rows CAD, USD, then EUR; NULL never totalled; stderr only the NULL line). Fails at the In-cell and stderr assertions

### Build
- [ ] Step 2: `internal/report/holdings.go` (new exported predicate beside `Converted`, `:35-45`) + `internal/cli/render_holdings.go:138-144` `holdingInCell`, const beside `:17` `holdingNoPriceCell` — batch 1. One owner for "currency is CAD or USD" (exact match) used by renderer and composer. In-cell arms, each its own row: unpriced row → blank (`Price == nil` wins, row facts only, never `Converted == nil`); priced and currency NULL → `not converted`; priced and EUR → `not converted`; priced CAD/USD → converted cents. Tests in `internal/cli/holdings_test.go` (`Test_holdings_in_column_says_not_converted_for_a_priced_security_it_cannot_convert`: CAD and USD modes, NULL, EUR, unpriced NULL, unpriced EUR, native has no In column) and `internal/report/holdings_test.go` (predicate classes; CAD-mode total with a NULL and an EUR row summed with a CAD row excludes both — bound: total unchanged when they are added; native EUR own total after USD, NULL never — extend existing `:43`/`:69` rather than duplicate)
- [ ] Step 3: `internal/report/document/holdings.go:107-135` `HoldingsWarnings` — batch 2. Add unexported `noCurrencyWarnings(h)` (slot 6) and `otherCurrencyWarnings(h)` (slot 7) returning `[]string`, appended after `noPriceWarning`; rewrite the doc comment to the contract only (drop "so far"; list the slot order once). NULL line: every mode, fires for a priced or unpriced row. EUR line: converted modes only, priced rows only (an unpriced row is not "priced in" anything), currency code as stored. Name: `*Security`, falling back to `SecurityID` when nil. One line per distinct `SecurityID` among qualifying rows, first appearance in table order; several lines keep `h.Rows` order. Wording verbatim from spec S.3. Tests in `internal/report/document/holdings_test.go`: `Test_HoldingsWarnings_say_which_securities_have_no_conversion` rows — NULL in CAD/USD/native (line each); EUR in CAD/USD (line), native (none); CAD and USD rows (none); two accounts, one security (one line); two NULLs then two EUR codes (table order, codes named as stored); no-price + NULL + EUR together (slot order 4, 6, 7); unpriced NULL (no-price line and NULL line); unpriced EUR (no-price line only); nil `Security` (id fallback); name with a quote (`%q`)

### Sweep
- [ ] Step 4: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; new test file under 480 lines; `holdingsRows` fixture reused, not re-seeded

### Verify
- [ ] Step 5: full verification block + `.claude/scripts/spec-check.py phase4b-holdings` → tick SCENARIO-07 with its acceptance test; rewrite STATE.md (fold Handoff; retire the `HoldingsWarnings` "so far" debt; add slots 6, 7 to the built list; remove `not converted` from Left unbuilt)

## Handoff

**Binding decisions:**
- In cell: `Price == nil` → blank first, then not-CAD/USD currency (NULL or other) → `not converted`, else the converted cents (S08 adds `no rate` for a CAD/USD priced row whose `Converted` is nil, after these arms) — S.2 table row order
- One exported predicate in `report` decides "CAD or USD"; renderer and composers share it — S08's before-first-rate warning must count only rows that pass it, or EUR/NULL leak into a rate warning
- NULL-currency warning keys on `Currency == nil` alone (priced or not, every mode); EUR warning needs `Price != nil` and a converted mode. A no-price NULL row therefore gets two lines (no price, then no currency); a no-price EUR row only the no-price line — its text says "priced in"
- Security named by stored name with `%q`, fallback `SecurityID`; the table's `Name (TICKER)` label is not used (it lives in `cli`). Two distinct securities sharing a name give two identical lines — accepted, no ticker disambiguation
- One line per distinct `SecurityID`, not per row: a security held in two accounts warns once

**Left unbuilt:** `no rate` In cell, before-first-rate and no-rates warnings (slot 5) and unconverted totals — SCENARIO-08; slots 2 and 3 of `HoldingsWarnings` — SCENARIO-10 / SCENARIO-09

**Traps:**
- A row is excluded from the converted total because the store leaves `ValueCAD`/`ValueUSD` NULL, so `Converted == nil` also covers no rate and no price — the In cell and warnings must use row facts (`Currency`, `Price`)
- Native mode has no In column: `holdingInCell` is not called there; EUR is totalled, so no EUR warning, but the NULL warning still fires
- Fixed `%6s`-style helpers in `run_holdings_test.go:16-25` misalign once a row has a wide cell (`not converted` is 13 wide): use a local line helper as `run_holdings_no_price_test.go:21` does

## Phase report

Run A (step 1) done. New file `cmd/quarry/run_holdings_not_converted_test.go` (~110 lines): consts `holdingsNoCurrencyLine`, `holdingsOtherCurrencyLine` (unprefixed), local helper `holdingsNotConvertedLine` (In column 13 wide), fixture `seedHoldingsStoreWithUnconvertible` (Euro Fund EUR 20 sh @12.50 = 250.00; Mystery Fund NULL currency 5 sh @10.00 = 50.00, both in Brokerage, priced day 9; sorted Acme, Euro, Mystery). Three tests, all red at assertions today:
- acceptance `Test_run_holdings_leaves_a_security_it_cannot_convert_out_of_the_total`: stderr got "" (want the two lines NULL then EUR); In cells blank (want `not converted`), In header 9 wide (want 13).
- `Test_run_holdings_json_lists_an_unconvertible_security_with_a_null_converted_value`: only `warnings[]` assertion red (`[]` vs two lines); null converted_value and the single CAD total already green (H-3 existing).
- `Test_run_holdings_native_totals_a_security_priced_in_another_currency_and_never_one_with_none`: only stderr red (NULL line missing); native table with CAD, USD, EUR totals already green.
No production code touched. Next (B1): step 2 (predicate + `holdingInCell`), step 3 (`HoldingsWarnings` slots 6, 7). Do not re-seed; reuse the fixture and consts.
