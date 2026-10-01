---
id: SCENARIO-07
status: open
---

# SCENARIO-07: price changes are listed both ways with first to latest (folds 08, 09, 10)

Cadence: code-first (no mandatory test-first item: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_recurring_price_test.go` `Test_run_recurring_lists_price_changes_both_ways_from_first_to_latest`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_recurring_json_test.go` `Test_run_recurring_json_returns_the_series_document`
Acceptance test (SCENARIO-09, folded): `cmd/quarry/run_recurring_price_test.go` `Test_run_recurring_leaves_out_a_bill_whose_amount_changes_most_months`
Acceptance test (SCENARIO-10, folded): `cmd/quarry/run_recurring_json_test.go` `Test_run_recurring_merges_payees_differing_in_store_numbers_and_splits_currencies`
Narrow loop: `go test ./internal/report/ ./internal/cli/ -run 'Recurring|PriceChange'` then `go test ./cmd/quarry/ -run 'Recurring'`
Mutation checks (code-first, judged load-bearing; owned by B1 for the first three, B2 for the last, `proof.md` protocol): 5% comparison `>` → `>=` → `Test_recurring_treats_exactly_5_percent_as_no_price_change`; gate `floor(steps/4)` → `steps/4+1` or `<` → `Test_recurring_lists_a_run_with_floor_steps_over_4_changes_and_drops_one_more`; half-away rounding → truncation → `Test_recurring_rounds_change_pct_half_away_from_zero`; `PayeeKey` tag read → always set → `Test_renderRecurringJSON_writes_a_null_payee_key_for_the_payee_id_fallback`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches, 1 feature package (`report`; `cli` + `cmd/quarry` wiring/e2e do not count). Sizing row 07; FOLDs 08, 09, 10 per `## Sizing`.

Existing surface surveyed (`go doc ./internal/report`, LSP): `Series{Payee, Currency, Cadence, Amount, PerYear *int64, First, Last, ChargeCount, State, New, key}` has none of FirstAmount / PriceChanges / Payees / Accounts / PayeeKey. `latestRun` already returns the run slice, so every new field derives from `run` in `seriesOf`. `store.Charge` already carries PayeeID, Payee, Account (ID/Name) — no port change, no fake change. `jsonOut` is not wired for recurring (`root.go:33`).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_recurring_price_test.go` (new) — 07 acceptance (9.99 x8, 11.99 x8, 10.99 x8 monthly, `--since 2000`: row ends `2: 9.99 -> 10.99 (+10.0%)`; a second `--json` run of the same fixture, decoded with `encoding/json`: two `price_changes` entries `9.99`→`11.99` and `11.99`→`10.99`, each with the later charge's date and `change_pct` 20.0 and -8.3) and 09 (a monthly payee whose amount moves >5% in most months is absent, caption+header only); `cmd/quarry/run_recurring_json_test.go` (new) — 08 (one active monthly series with one price change, `--json`, decoded with `encoding/json`: all ruled keys, values, `payee_key`, `payees`, `accounts`, `totals`, `warnings: []`) and 10 (`NETFLIX.COM 1234` + `Netflix.com` CAD, `Netflix.com` USD: one CAD series whose `payees` lists both names, one USD series). Reuse `chargeRows`/`groceryCharge`/`recurringTable`/`day` helpers (`run_helpers_test.go:176-219`, `run_recurring_test.go:26-59`)
- [x] Step 2: signature-only stubs so they compile and fail at their assertions: none needed beyond existing symbols (tests decode into local structs) — confirm red is at assertion (today `--json` prints text, price-change cell is empty, no gate). Quote failures in phase report

### Build
- [ ] Step 3: `internal/report/recurring.go:122-140,161-206` `Series`, `Recurring`, `seriesOf`; new `internal/report/pricechange.go` — batch 1, price changes and steady gate. Add `Series.FirstAmount`, `Series.PriceChanges []PriceChange{Date, From, To, Tenths}`, `Series.ChangeTenths` (first to latest); named consts `PriceChangeMinPct` = 5 and `SteadyChangeShare` = 1/4 (as the spec: a numerator/denominator pair or a divisor named `steadyChangeDivisor`); integer-only 5% test (`|to-from|*100 > 5*from`, no float); `tenthsOfPercent` half away from zero in integers; gate in `Server.Recurring` after `latestRun`, before listing (`changes <= steps/4`). Tests in `internal/report/recurring_price_test.go` via `recurringOf`, long runs so the gate does not hide the bound: `Test_recurring_treats_exactly_5_percent_as_no_price_change`: exactly 5% up and down (10000 to 10500 / 9500) → no change, 10501 / 9499 → change; `Test_recurring_lists_a_run_with_floor_steps_over_4_changes_and_drops_one_more`: steps 3 (4 charges, 0 allowed) 1 change → dropped; steps 4 (1 allowed) 1 listed / 2 dropped; steps 8 (2 allowed) 2 listed / 3 dropped; annual 2 charges within 5% listed, past 5% dropped; both-orders (up-then-down, down-then-up); `Test_recurring_rounds_change_pct_half_away_from_zero`: `Tenths` pins 2000→2101 = 51 and 2000→1899 = -51 (step >5%, half away both signs), overall first-to-latest 100.00→110.00→99.99 = 0 not negative; price changes carry the charge date they land on (the later charge); dropped group neither lists nor totals. Existing fixtures checked safe: `recurring_state_test.go:36-38` (1000,1000,1040 = 4%), `recurring_order_test.go:20` and `recurring_cadence_test.go:88` constant amounts, e2e `groceryCharge` series constant, split fixture `run_recurring_test.go:121-136` sums 2099 each month
- [ ] Step 4: `internal/report/charges.go:18-24`, `recurring.go:122-140,185-206` — batch 2, series identity. Add `Series.PayeeKey *string` (nil when `key.kind == keyByPayeeID`), `Series.Payees []SeriesPayee{ID, Name}` and `Series.Accounts []store.Account`: distinct over the run's charges, in order of first appearance in the run, never nil. Tests in `internal/report/recurring_identity_test.go`: renamed payee (two ids, one PayeeKey) lists both and `Payee` is the latest name; card change lists both accounts, one series; an account used only before the run's start is not listed; `PayeeKey` nil on payee_id fallback (use the `#4411` payee of `recurring_groups_test.go:105-109`) and set for a normal name; same name in CAD and USD = two series, each with only its own payees

- [ ] Step 5: `internal/cli/render_recurring.go:9-12,43-57` `renderRecurring`, `recurringAligns` — batch 3, Price changes cell. `priceChangesCell(s)`: empty when `len(PriceChanges)==0`, else `N: <formatMoney first> -> <formatMoney latest> (<signed pct>)`; sign `+` positive, `-` negative, none at 0 (`0.0%`, also for a -0.04 overall that rounds to 0). Tests in `render_recurring_internal_test.go` (white-box, layout rule): N=0 empty cell and no trailing space; `+11.8%`; negative; `0.0%` unsigned; 1,000-plus amounts grouped like the Amount column;  the ruled sample row 1 `1: 85.00 -> 95.00 (+11.8%)` verbatim
- [ ] Step 6: new `internal/cli/json_recurring.go` `recurringDocument`, `renderRecurringJSON(r report.Recurring, warnings []string)`; `internal/cli/recurring.go:18,55-63,66` and `internal/cli/root.go:33` — batch 4, `--json`. `newRecurringCommand(newReport, now, jsonOut *bool)`; `emitReport(cmd, *jsonOut, []string{}, renderJSON, renderText)`; delete the `// unreachable:` closure at `recurring.go:61-62`; `root.go:33` passes `jsonOut`. Document per spec `## Surface & Copy`: money via `jsonMoney`, dates `jsonDateLayout`, `account_filter` via `accountFilterDocuments(nil)` (always `[]` until S11 adds `RecurringRequest.Accounts`), `per_year` `*string` (null when ended), `payee_key` `*string`, `change_pct` `float64(Tenths)/10`, `new`, `state` `active`/`ended`, `cadence` `weekly|monthly|quarterly|annual` (new map beside `recurringEvery`), `charge_count`, `first_amount`; `payees`, `accounts`, `price_changes`, `series`, `totals`, `account_filter`, `warnings` built with `make(..., 0)`/len so never null. Text and JSON iterate `r.Series` once, same order. Tests in `json_recurring_internal_test.go` (read back with `encoding/json` into `map[string]any`): empty `report.Recurring{}` → every array is `[]` not null; ended-new series → `state:"ended"`, `per_year: null`, `new:true`, absent from `totals`; `Test_renderRecurringJSON_writes_a_null_payee_key_for_the_payee_id_fallback` (`payee_key` null on fallback series); one-payee one-account series still arrays; values round-trip (series count, amounts, change_pct 20.0 and -8.3 for the 07 fixture, dates, `charge_count`); two currencies' totals in order given. CLI-slice test in `recurring_test.go`: `--json` prints the document on stdout, no table, stderr empty; a store fault with `--json` leaves stdout empty (`ErrorIs` precedent). Update `emitReport`'s `// unreachable:` note (`output.go:36-37`) to name recurring's document (strings, bools, ints, finite float)

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new exported symbols (short, per `go-code.md`); `report/doc.go` untouched

### Verify
- [ ] Step 8: full verification per `.claude/rules/agent-briefs.md`, `spec-check.py phase2e-recurring-anomalies`; tick SCENARIO-07 with its acceptance test and 08, 09, 10 as `delivered by SCENARIO-07: <test>` (reference last on the line); rewrite `STATE.md` (drop the "Interim until S07/S11" bullet; `--account` still ignored until S11); `status: done`

## Handoff

**Binding decisions:**
- Price change and steady gate use integer cents, never floats — a float `> 0.05` misfires at exactly 5%; `Series.PriceChanges[i].Tenths` and `Series.ChangeTenths` are tenths of a percent, half away from zero; JSON `change_pct` is `float64(tenths)/10`, text is the same tenths with a `+`/`-` sign and none at 0 — S11/S14 do not touch these.
- The steady gate sits in `Server.Recurring` before listing, so a dropped group is in neither `Series` nor `Totals`, and window/`--account` filters (S11) apply to survivors only.
- `Series.Payees`/`Accounts` are the run's distinct values in first-appearance order; `Series.PayeeKey` is nil only for the payee_id fallback — S11's `--account` match and the JSON `accounts` both read `Series.Accounts`.
- `recurringDocument.account_filter` stays `[]` until S11 adds `RecurringRequest.Accounts`; S11 then passes `accountFilterDocuments(rec.Accounts)` and fills `warnings` from `recurringWarnings`, replacing the `[]string{}` arguments here.

**Left unbuilt:** `RecurringRequest.Accounts`, `namedAccounts` call, `recurringWarnings`, E1/E2, refusal outline, I1 — S11. `anomalies` — S14.

**Traps:**
- `payees` order is not ruled in the spec (only that renamed payees are both listed): first-appearance is this plan's choice; say so in the phase report.
- A price-change fixture needs enough charges that the gate keeps the series (steps/4 >= changes), else the series vanishes and the bound test reads "not listed" for the wrong reason — assert the series is present in each "change" arm.
- A 3-charge run has floor(2/4)=0 changes allowed: any new fixture with a >5% step in a short run vanishes.
- Unruled, for the orchestrator before B1: `price_changes[].date` is the later charge's date; `payees`/`accounts` distinct by id, first-appearance order in the run.
- `jsonOut` is the persistent `--json` flag already registered at `root.go:26`; no new flag.

## Phase report

Run A (steps 1-2) done. Acceptance (red) written; no production code touched, no stubs needed.

Files (new): `cmd/quarry/run_recurring_price_test.go` (07 acceptance `Test_run_recurring_lists_price_changes_both_ways_from_first_to_latest`; 09 `Test_run_recurring_leaves_out_a_bill_whose_amount_changes_most_months`), `cmd/quarry/run_recurring_json_test.go` (08 `Test_run_recurring_json_returns_the_series_document`; 10 `Test_run_recurring_merges_payees_differing_in_store_numbers_and_splits_currencies`; plus shared local decode types `recurringJSONDoc`/`recurringSeriesJSON`/`recurringPriceChangeJSON`/`recurringIDName`/`recurringTotalJSON`, `decodeRecurringJSON` (DisallowUnknownFields), `monthlySeries`, `inUSD`).

Red (all four fail at their assertions, `go test ./cmd/quarry/ -run 'Test_run_recurring_(json|lists_price|leaves_out|merges)'`):
- 07 text arm: expected row ending `active, new  2: 9.99 -> 10.99 (+10.0%)`, got row ending `active, new` (empty cell); its `--json` arm then fails `decodeRecurringJSON` (`invalid character 'R'`: stdout is the text table).
- 08, 10: `require.NoError` on decode, `invalid character 'R'` (--json prints text).
- 09: expected caption+header only, got `Hydro CAD month 12.00 144.00 ... active, new` plus Total row (no gate).
Build and `golangci-lint run ./cmd/quarry/...` clean (0 issues).

Next runs must not redo: fixtures are final. 07 series = Netflix.com 24 monthly charges from 2024-10-12 (9.99 x8, 11.99 x8, 10.99 x8), price changes dated 2025-06-12 (20.0) and 2026-02-12 (-8.3). 08 = Netflix.com Feb-Sep 2026 (9.99 x4, 11.99 x4), `payee_key` `netflix-com`, payee id `payee-Netflix.com`, account `acct-cad`/`Chequing`, `new:true`. 10 = CAD Apr-Sep 2026 (`NETFLIX.COM 1234` x3 then `Netflix.com` x3, 15.00; payee ids `payee-NETFLIX.COM 1234`, `payee-Netflix.com`) and USD `Netflix.com` Jun-Sep 12.00 x4 on `acct-usd`; the USD series lists only `payee-Netflix.com` although the id is shared with the CAD payee. 09 = Hydro Feb-Sep 2026 alternating 10.00/12.00 (7 changes in 7 steps).
Green-by-B: 07 text arm needs steps 3+5 (changes, cell); json arms need 3, 4, 6 (document, `jsonOut` wiring); 09 needs the gate (step 3); 10 needs identity (step 4) and 6.
Note for B1: `payees` order is first-appearance within the run (ruled in spec); `price_changes[].date` = later charge's date (ruled).
