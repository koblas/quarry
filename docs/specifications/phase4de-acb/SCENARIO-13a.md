---
id: SCENARIO-13a
status: open
---

# SCENARIO-13a: Shares added with no cost leave ACB incomplete

Cadence: code-first — no bug fix, write-safety guard or atomic adapter touched
Acceptance test: `cmd/quarry/run_acb_unknown_cost_test.go` `Test_run_acb_marks_an_incomplete_security_and_its_sales_after_shares_added_with_no_cost`
Narrow loop: `go test ./internal/report/... ./internal/cli/ ./cmd/quarry/ -run '(?i)acb'` (B1: drop `./cmd/quarry/`, see Step 3)
Mutation checks: span reset at pool 0 deleted in `(*securityWalk).apply` → `Test_acb_stops_marking_sales_once_the_pool_sells_out`; both-kinds arm of `noCostWarnings` falling to 4a → `Test_ACBWarnings_names_shares_added_and_dividends_reinvested_with_no_cost_in_one_line`
Runs: A (1) | B1 (2-3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 4 batches, 1 feature package (report; `report/document`, cli render and cmd pins ride it)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_acb_unknown_cost_test.go` (new) `Test_run_acb_marks_an_incomplete_security_and_its_sales_after_shares_added_with_no_cost` — one non-registered CAD brokerage: buy with cost, add_shares > 0 with nil `CostBasis`, a later partial sell; `runWith` + `replaceStore` (precedent `run_acb_shares_test.go:28-60`). Asserts exact stdout (year suffix `1 sale of shares with unknown cost`; position row suffix `incomplete`), exact stderr warning 4a, exit 0. No stubs: `ACBSale.UnknownCost`/`ACBSecurity.Incomplete` exist; compiles today and fails at the stdout assertion

### Build
- [x] Step 2: report span — `internal/report/acb_walk.go:317-327` add unexported `noCostAcquisition(tx)` beside `movesNoUnits` (add_shares units > 0 with nil cost, or reinvest_dividend with nil cost: the ONE predicate); `:172-182` `securityWalk.unknownCost`; `:214-252` `apply`: no-cost acquisition sets the event mark and opens the span; sell and remove_shares copy the span onto their event, sell sets `sale.row.UnknownCost` from that same value; after the take, pool shares 0 clears the span; `:208` `position.Incomplete = w.unknownCost`; `internal/report/acb.go:124-140` `ACBEvent.UnknownCost` (doc: the shares this event moved have no recorded cost); `acb.go:85` `(ACBYear).UnknownCostSales()` mirroring `PossibleSuperficialLosses`. Tests: new `internal/report/acb_unknown_cost_test.go` rows — no-cost add then sale (marked, incomplete); with-cost add control (unmarked, complete); reinvest nil cost (marked, incomplete) vs with cost; same-day no-cost add + sale (marked); sale BEFORE the no-cost add (unmarked); two partial sales (both marked, still incomplete); `Test_acb_stops_marking_sales_once_the_pool_sells_out` (sell-out sale marked, re-buy with cost + later sale unmarked, not incomplete); remove_shares in span (event marked) and remove_shares emptying the pool clears; oversell clears; later no-cost add reopens; zero-unit add_shares nil cost opens nothing; registered-account no-cost add AND registered no-cost reinvest (no mark, complete); after-`Today` no-cost add (no mark); span per security (sec-A's no-cost add never marks sec-B's sale); `UnknownCostSales()` counts 0/1/2. Fold `Test_acb_leaves_the_flags_the_later_rules_set_unset` (`acb_events_test.go:112-123`) in as the with-cost control row (rename or delete; its superficial assert stays somewhere)
- [x] Step 3: document — `internal/report/document/acb.go:138-140` inline count → `year.UnknownCostSales()` at `:125`; `acb_warnings.go:17-23` add slot 4 `noCostWarnings(a)` after `superficialLossWarnings`, before `removalWarnings`; new func after `:64`: one line per security in `a.Securities` order, variant from ITS ACQUISITION events only (`add_shares`/`reinvest_dividend` with `UnknownCost`) — never from `Incomplete` (S14's rate cause sets it; a sold-out security has it false and still warns). Name quoted `"%s"` raw like warnings 5/8, id bare. Tests `acb_warnings_test.go`: 4a, 4b, 4c verbatim (`spec :281-282`); two no-cost adds → one 4a line; two securities → two lines; sold-out security with no-cost add (`Incomplete` false) still warns; only a disposition event marked → no line; `acbDocumentFixture`'s `Incomplete: true` sold-out security (`document/acb_test.go:52`) → no line (extend `Test_ACBWarnings_is_empty_…` `:105-112`); extend the slot-order test `:191-224` with a 4 line between the superficial line and the removal (rename to include no-cost). `document/acb_test.go:99-126`: year with 2 unknown-cost sales → `unknown_cost_sales` 2. `./cmd/quarry` `run_acb_shares*` tests go red here (fixture has a no-cost add); re-pinned in Step 5 — expected, not a regression
- [x] Step 4: cli render — `internal/cli/render_acb.go:41-51` `acbYearSuffixes`: `humanize.Count(year.UnknownCostSales(), "sale of shares with unknown cost", "sales of shares with unknown cost")` between the superficial and ROC entries; `:54-72` `renderACBPositions`: sixth unheaded left-aligned column, `incomplete` when `s.Incomplete` (aligns slice gains `alignLeft`; header cell ""). Tests `render_acb_internal_test.go` (near `:73-98`): suffix singular and plural; all three suffixes in ruled order `superficial, unknown cost, ROC`; `incomplete` row beside a complete control row (control line byte-identical to before: trailing trim); header line unchanged
- [x] Step 5: cmd pins — re-pin `cmd/quarry/run_acb_shares_test.go:43-60` (S10's ticked acceptance test), `:62`, `:78`, `:93`: warning 4a for `Acme Corp` before the removal line on stderr and in JSON `warnings[]`, position row `incomplete`. KEEP `inv-add-free` without cost: it is S10's only cmd-level "counts at 0.00" coverage. New tests in `run_acb_unknown_cost_test.go`: `--json` (`sales[].unknown_cost` true, `years[].unknown_cost_sales` 1, `securities[].incomplete` true, `warnings[]` 4a); slot-4 order (three no-cost securities: `beta`, `Alpha`, and a second `Alpha` with a lower id stored after it → `Alpha`(low id), `Alpha`, `beta`); edge row "no-cost add, sold out, re-bought with cost" (year suffix counts only pre-sell-out sales, no `incomplete`, warning 4a still prints). Cells n/a: `--year`/`--security` (unregistered until S15/S16), `--account` (acb has none); 4b/4c forms ride the same `ACBWarnings` call pinned in both forms by `:93`

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; docs: `ACBSale` two-flags sentence (`acb.go:87-89`), `ACBWarnings` slot list (`acb_warnings.go:14-16`), `renderACBPositions` doc; grep `internal/report/*acb*_test.go`, `cmd/quarry/run_acb*_test.go` for any other no-cost add/reinvest fixture with an exact `warnings[]`/position pin (found: `acb_arms_test.go:121`, `acb_walk_test.go:352,369`, `acb_test.go:169` assert projections only — confirm)

### Verify
- [ ] Step 7: full verification + `spec-check.py phase4de-acb` → tick SCENARIO-13a with its acceptance test; STATE.md: drop the first Open debt's re-assert clause (done), keep its S16/S21 clauses

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `noCostAcquisition(tx)` (`acb_walk.go`) is the ONE "no-cost acquisition" predicate — S13b's detector calls it (add_shares arm) inside `readTimeFindings`, never restates it, or warning 4a and the finding disagree on which rows count
- Span lives in `securityWalk.unknownCost`, opened by a no-cost acquisition, cleared when a disposition leaves pool shares 0; `ACBSecurity.Incomplete` = span open after the walk. S14 ORs its rate cause into `Incomplete`, never replaces the assignment
- `ACBEvent.UnknownCost` = "the shares this event moved have no recorded cost": set on no-cost acquisitions and on sell/remove_shares inside the span; `ACBSale.UnknownCost` is copied from its sell event's value in `apply` (one source)
- Warning 4 variant comes from acquisition events (`Action` + `UnknownCost`), never from `Incomplete`; slot 4 iterates `a.Securities` (walk's name-ignoring-case-then-id sort) — S14 warning 6 and S16 must not reorder `Securities`
- `(ACBYear).UnknownCostSales()` is the ONE count: JSON `unknown_cost_sales`, text year suffix (second in `acbYearSuffixes`), S15's `--year` and S19's MCP reuse it
- Position text suffix is a sixth unheaded column holding `incomplete`; S14's no-rate cause reuses the same cell and word

**Left unbuilt** — named so nobody assumes it exists:
- A removal's mark (`ACBEvent.UnknownCost` on remove_shares) has NO output: event JSON key order is ruled without it. Surfacing it is unruled — S16 or final product-vision pass
- `--year` row suffix `unknown cost` and its stacking after `possible superficial loss` — S15
- Warnings 4a/4c print `quarry findings --type shares-without-cost`, which `findings` refuses until S13b adds the type
- No-rate cause of `Incomplete` and warning 6 — S14

**Traps** — things that look right and are not:
- `ACBWarnings` reads `a.Securities` events for slots 4, 5 and 8: S15's `--year` cut must compose warnings from the UNCUT report, or warning 4 vanishes for a security unsold that year
- A reinvest with 0 or negative units opens no span (`noCostAcquisition` needs units > 0, pinned); reinvest still has no zero-unit EVENT rule (it walks as an event and adds nothing)
- Find sales by `Action`, never `Realized` (ROC excess events are `Realized`)

**Orchestrator rulings 2026-10-05 (pre-dispatch):** (1) `noCostAcquisition` requires units > 0 for reinvest_dividend too — a reinvest with 0 or negative units moves nothing and opens no span (consistent with the S10 zero-unit ruling); pin it with a row. (2) Slot-4 order is security name ignoring case, then security_id, as ruled; if `a.Securities` order is not exactly that, sort in `noCostWarnings`; pin with two securities whose names differ only in case/order at cmd or document level. (3) Removal-event `unknown_cost` output stays unbuilt (S16/final pass), as planned.

## Phase report

Runs B1 (steps 2-3) and B2 (steps 4-5) done, steps ticked. Acceptance test `Test_run_acb_marks_an_incomplete_security_and_its_sales_after_shares_added_with_no_cost` is GREEN. `go test ./cmd/quarry/ ./internal/cli/ ./internal/report/...` all ok; `golangci-lint run ./cmd/quarry/... ./internal/cli/...` 0 issues. V still owns the full covered suite, Sweep docs, spec tick, STATE.md.

B1 production (unchanged): `internal/report/acb_walk.go` `securityWalk.unknownCost`, `noCostAcquisition`, `closeSpanWhenSoldOut`; `acb.go` `ACBEvent.UnknownCost`, `(ACBYear).UnknownCostSales()`; `document/acb.go` `unknown_cost_sales`; `document/acb_warnings.go` `noCostWarnings` (slot 4, iterates `a.Securities`, no composer sort: the walk's security sort orders it).

B2 production: `internal/cli/render_acb.go` `acbYearSuffixes` adds `N sale(s) of shares with unknown cost` between superficial and ROC; `renderACBPositions` has a sixth unheaded left-aligned column ("" header) holding `incomplete`.
B2 tests: `internal/cli/render_acb_internal_test.go` (count singular/plural, ruled three-suffix order, incomplete row beside a complete control; header and complete-row lines unchanged by trailing trim); re-pinned `cmd/quarry/run_acb_shares_test.go` (4 tests: `acmeNoCostWarnings` = 4a then removal on stderr and `warnings[]`, position row `incomplete`; `acbSharesPositionLine` gained a suffix arg and trims) and `run_acb_adjustments_test.go:102` (4a between adjustment and removal lines; renamed `..._then_no_cost_then_removal_...`); `run_acb_unknown_cost_test.go` new `--json` pin, slot-4 order pin, sold-out/re-bought edge row; `run_acb_surface_test.go` `acbDoc` gained `unknown_cost_sales`, `sales[].unknown_cost`, `securities[].incomplete`.

Order pin deviates from plan: names `Beta`/`alpha`/`alpha` (lower id stored last) so case-insensitive order differs from byte order (the plan's `beta`/`Alpha` sort the same either way); the two `alpha` warning lines are identical, so the id tiebreak is pinned via `securities[].security_id` order `sec-a1, sec-a2, sec-b`. Both orders are the walk's, so no composer sort is needed.

V owes: doc comments (`ACBSale` two-flags sentence `acb.go:87-89`, `ACBWarnings` slot list `acb_warnings.go:14-16`), the sweep grep for other no-cost fixtures with exact pins (cmd `run_acb*` now all green), full covered suite, `spec-check.py`, tick, STATE.md (drop first Open debt's re-assert clause; keep S16/S21). `acmeNoCostWarnings` is a package-level read-only test slice in `run_acb_shares_test.go`.
