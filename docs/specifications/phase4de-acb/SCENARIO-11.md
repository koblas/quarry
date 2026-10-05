---
id: SCENARIO-11
status: open
---

# SCENARIO-11: Return of capital and reinvested distributions

Cadence: code-first (no mandatory test-first item: no bug fix, write-safety guard or atomic adapter)
Acceptance test: `cmd/quarry/run_acb_adjustments_test.go` `Test_run_acb_lowers_and_raises_the_acb_by_the_adjustments_and_counts_return_of_capital_above_it_as_a_gain`
Narrow loop: `go test ./internal/report/... ./internal/platform/humanize/ ./internal/cli/ ./cmd/quarry/ -run '(?i)acb|adjust|Money'`
Mutation checks: ROC clamp `>` → `>=` in the ROC arm → `Test_acb_counts_only_return_of_capital_above_the_acb_as_a_gain`; swap RD/ROC kind order → `Test_acb_applies_a_days_reinvested_distributions_before_its_returns_of_capital_in_item_order`; drop the not-held check → `Test_acb_skips_an_adjustment_no_non_registered_account_holds`
Runs: A (1) | B1 (2-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 5 batches, 1 feature package (`report`; `document`, `humanize` hoist and cli wiring don't count)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_acb_adjustments_test.go` (new) `Test_run_acb_lowers_and_raises_the_acb_by_the_adjustments_and_counts_return_of_capital_above_it_as_a_gain` — `runWith` + `writeConfig` with `[[acb.adjustment]]` items (one RD, one ROC under ACB, one ROC above ACB in a year with no sale) over a one-security fixture (copy `acbSharesRows` shape, `run_acb_shares_test.go:26-41`); asserts stdout year row + suffix, position ACB, stderr warning 8. No stubs needed (config already parses adjustments): must fail at the stdout assertion

### Build
- [ ] Step 2: `internal/report/acb.go:12-16` `ACBRequest.Adjustments []ACBAdjustment` (new report-owned type: security id, date, ROC/RD cents, request order = config item order) + action consts `return of capital` / `reinvested distribution`; `internal/cli/acb.go:57` map `cfg.Adjustments` via an unexported `acbAdjustmentsOf(cfg)` beside the call; `internal/report/acb_walk.go:43-78,104-160` merge adjustments into `walkSecurity`'s day at tier 1: split, every RD, every ROC, config order within a kind; an item with both amounts yields an RD and an ROC event; dated after `Today` dropped. RD raises ACB, ROC (<= ACB) lowers it; units unchanged; event `ID`/`AccountID`/`Account` empty, `Amount` nil, `Shares` 0, `Held`/`ACB` after. Tests (new `internal/report/acb_adjustments_test.go`, `acbWalkOf` from `acb_walk_test.go:16`): `Test_acb_raises_the_acb_by_a_reinvested_distribution`, `Test_acb_lowers_the_acb_by_a_return_of_capital`, `Test_acb_applies_both_amounts_of_one_item`, `Test_acb_orders_a_days_buy_split_distribution_return_of_capital_then_sale_whatever_their_source_ids` (sibling of `acb_shares_test.go:121`), `Test_acb_applies_a_days_reinvested_distributions_before_its_returns_of_capital_in_item_order` (one day, ACB 50: item 1 ROC 100, item 2 RD 80 + ROC 30 → events RD(2), ROC(1), ROC(2), no excess on ROC(1); kinds swapped would give an excess), `Test_acb_leaves_out_an_adjustment_after_today`; cli `internal/cli/acb_test.go` `Test_acb_applies_the_configs_adjustments` (sibling of `:102`)
- [ ] Step 3: `acb_walk.go` ROC arm + `acbYears` `:254-275` — ROC above ACB: ACB 0, excess = event `Gain`, `Realized` true, `Outlays` nil; `ACBYear.ReturnOfCapitalGain int64` (`acb.go:26-31`); excess entries flow beside sales into `acbYears` so a year with only an excess gets a row (no sales, sale totals 0). Tests: `Test_acb_counts_only_return_of_capital_above_the_acb_as_a_gain` (bound rows ACB-0.01, ACB, ACB+0.01: at ACB no gain, `Realized` false), `Test_acb_counts_a_whole_return_of_capital_as_a_gain_on_shares_with_no_acb`, `Test_acb_lists_a_year_with_only_a_return_of_capital_gain`, `Test_acb_keeps_a_years_sales_gain_apart_from_its_return_of_capital_gain`
- [ ] Step 4: `acb_walk.go:60-68` + `acb.go:20-24` — new `ACB` field listing adjustment issues by 1-based item: unknown security (id not in `history.Securities`), not held (pool units 0 at its tier-1 point; also for known securities the loop at `:62-64` skips — registered-only, or no pool row yet), repeat (same security+date as an earlier APPLIED item; each later item pairs with the first). Skipped items make no event. Tests: `Test_acb_skips_an_adjustment_for_a_security_not_in_the_store`, `Test_acb_skips_an_adjustment_no_non_registered_account_holds` (rows: before first buy; registered-only security → no `Securities` entry; same-day buy → applied; same-day sell-out → applied), `Test_acb_names_a_repeated_adjustment_with_the_first_item` (pair; triple → (1,2),(1,3); both skipped → no repeat)
- [ ] Step 5: `internal/platform/humanize` new grouped-cents func hoisted from `internal/cli/render.go:303-313` `formatMoney` (cli `formatMoney` delegates; no caller moves) + behaviour classes pinned in `humanize_test.go` (negative, zero, < 1.00, grouping); `internal/report/document/acb.go:30-40,118-124` `ReturnOfCapitalGain` `json:"return_of_capital_gain"` after `Gain`; `document/acb_warnings.go:11-14` `ACBWarnings(a, configShown string)`: slot 1 the three ruled adjustment lines (spec :323-325) in item order (repeat line at its later item), ids via `tomlstr.BasicString`, names `"%s"`, dates `DateLayout`; then slot 5; then warning 8 (spec :282) by security then event order, `<x>` grouped. Tests: re-pin `document/acb_test.go:83` key order; `Test_NewACB_writes_a_sale_only_year_return_of_capital_gain_as_zero`; `Test_NewACB_writes_a_year_with_only_a_return_of_capital_gain`; `acb_warnings_test.go` one test per ruled line verbatim + `Test_ACBWarnings_lists_adjustment_lines_then_removals_then_returns_of_capital_above_the_acb`; existing `:18-63` calls gain the path arg
- [ ] Step 6: `internal/cli/render_acb.go:23-36` `renderACBYears` trailing left-aligned suffix column, empty header, suffixes an ordered slice joined ", ": `<x> return of capital above ACB, a capital gain`; `internal/cli/acb.go:62-68` `ACBWarnings` twice — stderr `homepath.Abbreviate(srv.Home(), cfg.Path)`, JSON `cfg.Path` (precedent `accounts.go:55,66`). Tests: `render_acb_internal_test.go` `Test_renderACB_suffixes_a_years_return_of_capital_gain` + ROC-only year row (`0` sales, zeros); existing goldens `:30-110` pass unchanged (control); `cmd/quarry/run_acb_adjustments_test.go` `Test_run_acb_lists_config_then_adjustment_then_removal_then_return_of_capital_warnings_in_both_forms` (sibling of `run_acb_shares_test.go:93`; `~` path stderr, absolute JSON), `Test_run_acb_writes_return_of_capital_gain_in_json` (ROC-only year `sale_count` 0, `"0.00"` on a sale-only year). No new fallible call: no fault tests

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `ACBAdjustment`, the new `ACB`/`ACBYear` fields, `ACBWarnings`' new param, the humanize func

### Verify
- [ ] Step 8: full verification + `.claude/scripts/spec-check.py phase4de-acb` → tick SCENARIO-11 with its acceptance test; STATE.md: REWRITE Open debt :71, never close it — default `years[]` pin (`document/acb_test.go:83`) and RD-before-ROC pin done; S15 `--year` and S19 MCP `years[]` pins still owe `return_of_capital_gain` after `gain`, `"0.00"` when none

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `ACBRequest.Adjustments []report.ACBAdjustment`, mapped in cli only; `report` never imports `config` — dependency rule; S19's MCP maps the same way
- Day order: tier 0 acquisitions; tier 1 split, every RD, every ROC (config item order within a kind; a both-amounts item splits into two events); tier 2 dispositions — spec Part B :200
- Not-held = pool units 0 at the item's tier-1 point; unknown = id not in `InvestmentHistory.Securities` (every security in the store, registered-only included). Skipped items make no event and no `securities[]` entry
- Repeat warning only between applied items; each later item pairs with the first; emitted at the later item's position. Slot 1 = cfg's own warnings, then adjustment lines in item order
- `ACBWarnings(a, configShown)` is still the one composer; callers pass `~` path for stderr, absolute for JSON (S19: absolute)
- Adjustment events: `Amount` nil, `Shares` 0, `ID`/account empty; `CAD` = effect on ACB (RD +amount, ROC −amount), matching `document/acb_test.go:189` (default, pending ruling)
- Adjustments dated after `Today` dropped silently, like transactions (default, pending ruling)
- Warning 8 `<x>` grouped like the year suffix, via the hoisted humanize func (default, pending ruling)
- Year-row suffixes are an ordered slice; S12's and S13a's go before the ROC suffix (spec :250)

**Left unbuilt** — named so nobody assumes it exists:
- `--year` text row after `Total` (`Return of capital above ACB` in Security, amount in Gain or loss, other cells blank, only when > 0) and warning 2's `--year` form firing only when the year has no sales AND `ReturnOfCapitalGain` is 0 — S15
- No-rate USD cut: S14's "left out of year totals" must drop the security's ROC excess from `ReturnOfCapitalGain` too, warning 8 still fires; zero-cost (warning 4) excess stays — S14
- `--security` text action names `return of capital` / `reinvested distribution` — S16
- `years[]` key-order pins with `return_of_capital_gain` for `--json --year` — S15; for MCP `acb` — S19

**Traps** — things that look right and are not:
- RD/ROC have no source_id: adding them to `acbTiers` at 1 and sorting by source_id breaks config order and the both-amounts split
- `walkACB` skips a security with no pool rows (`acb_walk.go:62-64`): not-held for such a security must be checked outside that loop
- `Realized` no longer means "sale": an ROC-excess event has `Realized`, `Gain`, nil `Outlays`; S12/13a/14/16 must find sales by `Action`
- ROC exactly equal to ACB is no gain: `Realized` stays false (JSON `gain` null), no warning 8

**Orchestrator rulings 2026-10-05 (SCENARIO-11 plan defaults): (1) adjustment event `cad` follows the column's cash-flow sign: return of capital +amount (cash to you), reinvested distribution −amount (like a reinvest/buy); `internal/report/document/acb_test.go:189` fixture flips to `CAD: 250`. (2) adjustments dated after today are dropped with no warning, as transactions are. (3) three or more items on one security+date: each later item pairs with the first, warning printed at the later item's position. (4) a repeat whose items are both skipped (unknown/not held) gets no repeat warning; each keeps its own line. (5) warning 8 `<x>` is thousands-grouped, 2 decimals, like the year suffix. Recorded in spec Part B CRA rules. Defaults 2-5 stand as planned; default 1 overruled.**

## Phase report

Run A (step 1) done. Acceptance test red, committed.
- `cmd/quarry/run_acb_adjustments_test.go` (new): consts `acbAdjustmentsConfig`, `acmeReturnOfCapitalWarning`, helper `acbAdjustmentsRows` (one non-registered CAD brokerage, Acme Corp 10 shares bought 2024-02-01 for 1,000.00, never sold) and the acceptance test. Config: RD 200.00 on 2024-06-30, ROC 150.00 on 2024-12-31 (under ACB, no gain), ROC 2300.00 on 2025-12-31 (ACB 1050.00, excess 1,250.00). Expect: one year row 2025 (0 sales, zeros, suffix `1,250.00 return of capital above ACB, a capital gain`), position ACB `0.00` / `0.0000`, stderr warning 8 only.
- Red at the stdout assertion (no year row, position ACB `1,000.00` / `100.0000` because adjustments are ignored) and the stderr assertion (empty); exit 0 passes. No production code or stubs touched.
- Year row column widths in the expectation: ACB column is 4 wide (`0.00`), suffix column left-aligned after a two-space gap, header's empty seventh cell trimmed. Step 6 must produce exactly that.
- The B runs add the other tests in the plan to this same file (steps 5-6); `year` suffix text and warning 8 text are asserted verbatim here.
