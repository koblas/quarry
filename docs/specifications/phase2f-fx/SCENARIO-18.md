---
id: SCENARIO-18
status: open
---

# SCENARIO-18: Anomalies are judged in native currency and shown converted

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_anomalies_fx_test.go` `Test_run_anomalies_judges_in_native_currency_and_shows_converted_amounts`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run '(?i)anomal|charges|help|fx_warning|unconverted'`
Mutation checks: native judging in `judge` (`report/anomalies.go:158`) → `Test_anomalies_finds_none_when_only_the_rate_moves` + `Test_anomalies_never_list_a_usd_charge_under_100_in_its_own_currency`; Usual at the charge's own rate → `Test_anomalies_convert_usual_at_the_charges_rate_not_the_baselines`; count of listed-only charges → `Test_anomalies_count_only_listed_unconverted_charges`
Runs: A (1-2) | B1 (3-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 3 batches, 1 feature package (`report`; duckstore test-only) + cli. Port reused unchanged (`Charge.AmountCAD/AmountUSD/USDCAD`, `Charges.FirstRate`; frozen by 17).

Survey (existing, reused): `report.chargeIn` (`recurring.go:235`) picks the cell; detection already native (`chargeKey`, `categoryKey` carry currency); `unconvertedWarnings(currency, u, noun)` (`cli/fx_warning.go:251`); `windowCaption`; `withConfigWarnings`. Needs `anomalies.go` to keep `resolve`'s value (today `_`, `:50`).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_anomalies_fx_test.go` (new) `Test_run_anomalies_judges_in_native_currency_and_shows_converted_amounts` — on `replaceStoreWithRates` (rate on the big charge's date differs from the earlier charges' dates and from the newest rate): USD payee history, a USD 90.00 charge above 2x the payee's USD median (CAD ~123), so the native 100.00 floor is the only gate that keeps it out, a USD 250.00-ish charge far above the baseline (listed). Text: caption `, amounts in CAD`, Amount/Usual converted at the charge-date rate, Times and footer native. `--json`: `currency`, `native_currency`, `native_amount`, `native_usual`, `times` native.
- [x] Step 2: `internal/report/anomalies.go:43-70` `AnomaliesRequest.Currency`, `Anomaly` (converted amount and usual, native currency, unconverted marker), `Anomalies.Currency` + `Unconverted store.Unconverted` — signature-only; red at the caption/cell assertion. Design: a zero-value `Anomaly` must keep rendering native (the render and json tests build bare fixtures).

### Build
- [x] Step 3: `internal/report/anomalies.go:78-112` `(*Server).Anomalies`, `:158-190` `judge` — judging untouched (native `Amount`, native median, floor). Convert after the verdict: Amount via `chargeIn`; Usual via `money.Convert(median, parsed c.Currency, target, c.USDCAD)` (R1's single Go conversion). Decide once per anomaly: Amount and Usual convert only when BOTH `chargeIn` and `money.Convert` succeed, else both stay native and the charge counts if it is CAD/USD (mirrors `Series.listedIn`); a fake-only arm (cell present, `USDCAD` 0) is a row. EUR stays native and uncounted; a charge in the target is identity and needs no rate. `Unconverted.Transactions` counts LISTED unconverted charges; `FirstRate` from `charges.FirstRate` unless native. Tests `anomalies_fx_test.go` (new, against the fake), each arm a row: converted USD→CAD; CAD→USD (`AmountUSD`); native target; identity with nil cells (not counted); no rate (native, counted); category baseline converted the same way (`Baseline` category); `Test_anomalies_finds_none_when_only_the_rate_moves` + reverse (native jumps, converted flat → listed); USD 90.00 not listed while CAD-equivalent ≥100.00 (bound: 99.99 / 100.00 native); `not_judged` and `Checked` identical in all three currencies; Usual rate is the charge's own; counting: filtered by window/`--account` or not unusual adds nothing; one `Charges` read (`chargesReads == 1`). Same batch: duckstore `charges_test.go` `Test_charges_cells_match_money_convert_for_single_split_charges` (Charges-side counterpart of the view-level pin `views_fx_test.go:202` `Test_money_convert_matches_every_spending_rows_converted_columns` (do not rewrite that one): `Convert(Amount, cur, CAD|USD, USDCAD)` == the cell, half-cent cases ±, rates 1.25/1.249999/1.6/1.600001); a split row pins that the cell is the per-split sum, not `Convert(Amount)`.
- [x] Step 4: `internal/cli/anomalies.go:50,60,65,78-86`, `render_anomalies.go:109-136`, `fx_warning.go:243-247` — thread `resolve`'s value into the request; `anomaliesWarnings` order: config, left-out, FX (`unconvertedWarnings(a.Currency, a.Unconverted, chargesNoun)`, new `chargesNoun`), empty-window (an empty window has no listed charge so no FX line). Render: caption `windowCaption(..., a.Currency)` replaces the `money.Native` placeholder at `:134`; Amount/Usual cells converted; an unconverted charge prefixes `USD 250.00` in both cells (thousands kept); native mode byte-identical; Times and footer native.
  - Tests: `fx_warning_internal_test.go` charges arm at 1 (`is`) and 2 (`are`), no-rates, zero. `render_anomalies_internal_test.go`: converted row, `USD 1,250.00` unconverted cell, `CAD 250.00` in USD mode, native, zero-value fixture unchanged. cli test: left-out-account warning before FX line (text and `--json` order pin); config USD via a USD `LoadConfig` (`cadConfig` shape, `currency_test.go`) → `amounts in USD`.
  - Repoint CAD-mode captions to `, amounts in CAD`: `cmd/quarry/run_anomalies_test.go:67`, `_category_test.go:30,50`, `_account_test.go:34,61`, `_empty_test.go:27,98,122` (:98 is the `wantCaption` table, 6 cases), `run_charges_edges_test.go:54,74,90`, `internal/cli/anomalies_test.go:126`, `anomalies_account_test.go:51`. `render_anomalies_internal_test.go:14` stays (zero currency = native).
  - cmd edge rows in `run_anomalies_fx_edges_test.go` (new): Step 4 owns each row's TEXT cells and stderr, Step 5 adds the `--json` cell of the same row (the keys exist only then); each row in both; silent rows `assert.Empty(stderr)`: USD in USD mode (identity, silent); before-first-rate USD in CAD (`USD 250.00` cells, "N charges dated before…" line, [json, Step 5: row currency ≠ `.currency`, native fields = row, `warnings[]`]); CAD before-first-rate in USD (`CAD …`); unrated USD in CAD (no-rates line, native cells); unrated all-CAD in CAD (silent); `--currency native` on a RATED USD store (literal pre-18 text, json `currency:"native"`, `native_*` == row [json, Step 5]); category-baseline anomaly (null payee) converted; `not_judged` footer unchanged in CAD/USD/native; one USD `--account` default CAD (caption names it + `amounts in CAD`); closed account converts; two unconverted listed charges (`are`) vs one (`is`); empty window on an unrated USD store in CAD/USD/native (empty note only, no FX line, json `currency` set, `anomalies []`).
- [x] Step 5: `internal/cli/json_anomalies.go:5-13,26-41,51-74` `currency` right after `until`; `native_currency, native_amount, native_usual` per entry; `currency`/`amount`/`usual` converted; `times` native. `internal/cli/anomalies.go:27-38` Long. Tests: new `json_anomalies_internal_test.go` key-ORDER pin with `topLevelKeys` (`json_spend_internal_test.go:177`) for top level and an entry, converted + unconverted + native; add the new keys to `anomalies_json_test.go:62` key-set pin; read-back test with `encoding/json` (count, converted vs native values, `anomalies []` not null, one entry for a no-payee category-baseline charge, `warnings []`); `cmd/quarry/run_anomalies_json_test.go:54` typed doc gains the new fields. Long: edit `Charges under 100.00 …` to the ruled sentence, add the ruled paragraph, re-pin verbatim at `anomalies_test.go:81` (`Test_anomalies_help_says_what_anomalies_lists`).

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new fields.

### Verify
- [ ] Step 7: full verification + `spec-check.py phase2f-fx` → tick SCENARIO-18 with its acceptance test; rewrite STATE.md.

## Handoff

**Binding decisions:**
- Detection, floor, `Times`, `NotJudged`, `Checked` stay native (R3); only `Amount`/`Usual` display converts. Amount uses the charge's cell (`chargeIn`, per-split sum), Usual uses `money.Convert` at `c.USDCAD`, so for a split charge Amount may differ by a cent from `Convert(Amount)`; 17's trap, unchanged.
- Unconverted = CAD/USD charge for which Amount or Usual cannot convert (both then stay native, never one converted and one not); counted only when LISTED; native target → no `FirstRate`, no warning. Third currencies stay plain and uncounted.
- 19 reuses nothing from here; 11 stays folded into 19.

**Left unbuilt:** accounts' reporting-currency balances (19).

**Traps:** `anomalyOf` fixtures with zero converted fields would render `0.00` if the design reads converted fields unconditionally. CAD-mode captions on every existing cmd/cli anomalies pin change (Step 4 list). Existing anomalies tests hold no USD data (grepped `inUSD(`, `usdChequingAccount`, `"USD"`, positive control on `run_recurring_fx_test.go`), so those goldens gain no FX line; never seed rates there.

**Ruled by the spec's edge row:** no Currency column; a converted cell is a plain number and the Account label keeps `(USD)`.

**Unruled copy (orchestrator: one copy ruling before run A):**
- Anomalies Long wrapping: spec gives the sentence and paragraph unwrapped. Recurring's ruling wrapped at <=74 columns; ruled text needed for paragraph 1's reflow (line 4 grows) and for the new paragraph's position (proposed: after paragraph 1, as recurring).
- JSON positions: top-level `currency` after `until` (spec); entry `native_currency, native_amount, native_usual` proposed right after `usual` (before `earlier`), mirroring recurring's after-`per_year`.
- Cell prefix when a CAD charge is unconverted in USD mode: proposed `CAD 250.00`, by symmetry with the ruled `USD 250.00`. Native mode adds no prefix.
- Warning noun: `charge`/`charges` (spec: "the same sentence with `charges`").

## Phase report

Run B2 done; steps 1-5 ticked. Sweep (6) and Verify (7) are V's. Already green at B2's end: `go build ./...`, `golangci-lint run ./...` 0 issues, full covered suite rc=0, `uncovered-diff.py` 0 uncovered lines since `<start>` 6c75b4a, `go test -race ./internal/cli/` ok. Test counts vs `<start>` (cumulative A-B2): cmd/quarry 489 (+12), internal/cli 409 (+12), internal/report 272 (+16), duckstore 468 (+2), total 1638 (+42).

- `internal/cli/json_anomalies.go`: top-level `currency` after `until` (`a.Currency.String()`, "native" in native mode); entry `native_currency, native_amount, native_usual` after `usual`, before `earlier`; entry `currency/amount/usual` come from `ListedCurrency/ListedAmount/ListedUsual` when `ListedCurrency != ""`, else the native fields; `times` native. In native mode native_* equal their twins.
- `internal/cli/anomalies.go`: Long verbatim from the spec's Anomalies ruling (paragraph 1 reflowed, new paragraph after it); pin repointed in `anomalies_test.go` `Test_anomalies_help_says_what_anomalies_lists`.
- Tests: new `internal/cli/json_anomalies_internal_test.go` (key-order pin for top level and entry in converted/unconverted/native using `topLevelKeys`; read-back with `encoding/json` incl. no-payee category-baseline entry, `warnings []`, native-mode twins, `anomalies []` not null); key-set pin extended in `anomalies_json_test.go`; `run_anomalies_json_test.go` expected doc gained `currency` and native_* (typed doc already had the fields); every json subtest in `run_anomalies_fx_edges_test.go` now asserts `.currency` and the six cells via `jsonCells` (USD identity, before-first-rate USD and CAD, two unconverted, unrated USD, unrated CAD, native rated, category baseline, `--account`, closed, not-judged per currency, empty window per currency). All store-backed, no network.
- Mutations (each restored, each reddened tests): ignore ListedCurrency in the entry; native_amount from the converted value; native_currency from the listed currency; blank top-level `currency` (killed by the read-back, key-order, edge-row and run tests); Long without "in their account's own currency" (help pin).
- Nothing deferred. V: spec-check, tick SCENARIO-18 with its acceptance test, STATE.md rewrite (STATE.md untouched by B1/B2).
