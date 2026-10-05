---
id: SCENARIO-22
status: open
---

# SCENARIO-22: A sale of more shares than the accounts held leaves a short the next buy covers

Cadence: code-first — ruled rule change from the S21 reference check (`RULING-S22.md`); no reviewer `Failure:`, no write-safety guard, no atomic adapter
Acceptance test: `cmd/quarry/run_acb_short_test.go` `Test_run_acb_counts_shares_sold_beyond_the_pool_at_no_cost_and_lets_the_next_buy_cover_the_short`
Narrow loop: `go test ./internal/report/... ./internal/cli/ ./internal/mcp/ ./cmd/quarry/ -run 'acb|ACB|Acb'`
Mutation checks: cover-first in `(*acbPool).add` (add every bought unit and its full cost while short) → `Test_acb_covers_a_short_before_adding_units_to_the_pool`; pro-rata in `(*acbPool).add` (units beyond the short at full cost) → `Test_acb_adds_the_cost_of_the_units_beyond_the_short_pro_rata_rounded_half_away`; `closeSpanWhenSoldOut` `<= 0` → `== 0` → `Test_acb_ends_the_unknown_cost_span_when_a_disposition_leaves_the_pool_short`; `oversoldWarnings` one line per disposition (return after the first) → `Test_ACBWarnings_names_each_oversold_disposition_in_walk_order`
Runs: A (1) | B1 (2-4) | B2 (5) | V (6-7)
Size: OWNS A RUN — 4 batches, 1 feature package (`internal/report` + its `document` subpackage; cmd tests only)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_acb_short_test.go` (new) `Test_run_acb_counts_shares_sold_beyond_the_pool_at_no_cost_and_lets_the_next_buy_cover_the_short` — SCENARIO-22 Given/Then via `runWith` + `writeConfig` (local `accounts.non-registered`) + `replaceStore`, style of `run_acb_unknown_cost_test.go:83-102`; JSON shares through `document.Shares` (`"-10.000000"`, `"5.000000"`); stderr exact `quarry: warning: ` + 10a line from `RULING-S22.md`; no stubs needed (cmd-level)

### Build
- [x] Step 2: `internal/report/acb_walk.go:393-413` `(*acbPool).add`/`take`, `:258-274` sell/remove arms, `internal/report/acb.go:186-202` new `ACBEvent.Oversold *big.Rat` — `take` leaves shares negative (all ACB out); oversold sale `UnknownCost` true; acquisition covers the short first, units beyond at `roundHalfAway(costCAD × beyond ÷ bought)` (pro-rate only when shares < 0 before the add). Tests in `internal/report/acb_short_test.go` (new): `Test_acb_counts_the_units_sold_beyond_the_pool_at_no_cost_and_leaves_the_pool_short` (gain = proceeds − outlays − ACB removed, commission row; Held < 0; ACB 0), `Test_acb_marks_a_disposition_oversold_only_beyond_what_the_pool_holds` (bound: units = held → nil, shares 0; held + 1 millionth → 0.000001; remove_shares row; second disposition while short), `Test_acb_covers_a_short_before_adding_units_to_the_pool` (cover-only: units < short, units = short; ACB stays 0), `Test_acb_adds_the_cost_of_the_units_beyond_the_short_pro_rata_rounded_half_away` (half-away row; USD row where convert-then-prorate ≠ prorate-then-convert by a cent; reinvest and add_shares with cost; no-short buy control). Re-pin `acb_walk_test.go:91-101` (`"-2"`), `:309-320` (`"-5"`, sale unknown cost), `acb_shares_test.go:53-60` (`"-2"`); change `acb_walk_test.go:103-113` fixture to sell exactly 10 so it keeps pinning edge row "Sold out, re-bought later"
- [x] Step 3: `acb_walk.go:237-239` span opens only when the pool is > 0 after the add (move below the switch), `:278-283` `closeSpanWhenSoldOut` at ≤ 0, `:219` `Incomplete` also while shares < 0, `:292-300` `adjust` not-held at ≤ 0, `acb.go:173-180` `PerShare` nil at ≤ 0; `:55` drop the `// unreachable:` nil-security marker (STATE open debt). Tests in `acb_short_test.go`: `Test_acb_ends_the_unknown_cost_span_when_a_disposition_leaves_the_pool_short` (no-cost add → oversell → buy with cost → sale unmarked; control: partial sale leaving > 0 → next sale marked), `Test_acb_opens_no_unknown_cost_span_for_shares_with_no_cost_that_only_cover_a_short` (cover-only vs partly-beyond no-cost add), `Test_acb_marks_a_short_position_incomplete_until_it_is_covered` (+ `PerShare` nil at < 0 and 0), `Test_acb_skips_an_adjustment_while_the_pool_is_short_as_not_held` (ROC and RD rows: `ACBAdjustmentNotHeld`, no excess), `Test_acb_splits_a_short_pool` (−10 ×2 → −20, ACB 0); `acb_walk_test.go` `Test_acb_skips_a_transaction_with_no_security` (nil security in a pooled account); re-pin `acb_unknown_cost_test.go:105` `wantIncomplete: true`
- [x] Step 4: `internal/report/document/acb_warnings.go:33-45` `ACBWarnings` appends `oversoldWarnings(a)` after `decemberSaleWarnings`; new `oversoldWarnings` after `:264` — 10a (sell) / 10b (remove_shares) verbatim, `<x>` = `humanize.Shares(report.Millionths(event.Oversold))`, every security's events, uncut report. Tests in `acb_warnings_test.go`: `Test_ACBWarnings_names_an_oversold_sale` (grouped `1,122.84`; same line under `ACBAdviceCLI` and `ACBAdviceTool`), `Test_ACBWarnings_names_an_oversold_removal_after_its_removal_line` (5 and 10b both), `Test_ACBWarnings_names_each_oversold_disposition_in_walk_order` (two securities × two dispositions), `Test_ACBWarnings_lists_oversold_lines_after_the_december_sales` (slot 9 then 10); fixtures via `acbPooled`
- [x] Step 5: surface cells in `run_acb_short_test.go` — `Test_run_acb_text_marks_the_oversold_sale_and_lists_no_short_position` (year suffix `sale of shares with unknown cost`, no position row while short, stderr 10a), `Test_run_acb_json_writes_a_short_position_with_negative_shares_and_no_acb_per_share` (`"-1122.840000"`, `"0.00"`, null, incomplete true), `Test_run_acb_year_marks_the_oversold_sale_unknown_cost` (`--year 2017` row suffix `unknown cost`), `Test_run_acb_security_prints_negative_shares_held_grouped` (`-1,122.84`, suffix `unknown cost`); `cmd/quarry/run_mcp_acb_warnings_test.go` `Test_run_mcp_acb_warns_of_a_sale_beyond_the_pool_in_the_cli_words`. Re-pin any cmd/cli/mcp/document exact warning list whose fixture sells into an empty pool (now emits 10a) — say which in the phase report. n/a cells: 10b stderr and ROC-while-short at cmd level (shared composer and existing not-held line, pinned in `document` and Step 3)

### Sweep
- [ ] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments: `take` (`acb_walk.go:399-400`, describes the short), `unknownCost` field `:189-190`, `closeSpanWhenSoldOut`, `adjust` `:291-292`, `ACBEvent.Oversold`, `PerShare`; spec edits verbatim from `RULING-S22.md` → *Changes to existing surfaces*: `specification.md:85`, `:206`, `:399` item (c), four edge rows appended to the table ending `:392`

### Verify
- [ ] Step 7: full verification + `spec-check.py phase4de-acb` → tick SCENARIO-22 with its acceptance test; rewrite STATE.md per Handoff (supersede, not append)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Pool invariants: shares ≤ 0 ⇒ ACB 0; `unknownCost` ⇒ shares > 0. Every "is it held" check is `Sign() > 0` / `<= 0`, never `== 0` — `adjust`, span close, `PerShare`, text positions all depend on it
- `ACBEvent.Oversold *big.Rat` — nil unless the disposition left the pool below 0; value = the short after it (−`Held`). Never serialized (ruling: no new JSON field). Slot 10 reads it from the UNCUT report like slots 4/5, so it fires for NoRate, unselected and other-year securities
- Slot 10 is last in `ACBWarnings`, no advice substitution; a later slot goes after it
- Oversold sale: `UnknownCost` true whatever the span; ACB removed = whole pool ACB (0 when already short); gain never adjusted later. Covering acquisition: event `CAD` full cost; ACB += pro-rated CAD cost of units beyond the short, converted first
- Superficial: `heldAt` (`acb_superficial.go:101-130`) counts per account, file-wide, not the pool — a short pool changes nothing. `acquiredBetween` counts a covering buy within ±30 days, rule unchanged
- STATE lines superseded: :23 `take` "sold >= held takes all and resets to 0" (now shares go negative); :27 span "clears when a disposition leaves 0" (now ≤ 0); :28 not-held "pool 0" (now ≤ 0); `PerShare` nil at ≤ 0. Open debt `acb_walk.go:~55` nil-security closed by Step 3

**Left unbuilt / default, pending ruling** — orchestrator gets a product-vision ruling before run B1 (Steps 2-3 build both defaults):
- `<x>` for a disposition while already short: default = the short after it (pool −10, sell 5 → 15), which keeps "more than held" and "next <x> … back to 0" literally true
- Cover-only no-cost acquisition: default keeps `event.UnknownCost` true (the transaction has no cost; the finding lists it), so warning 4 still fires though no span opens. Acceptance test reaches neither

**Traps** — things that look right and are not:
- JSON shares are 6-decimal `document.Shares`; Gherkin `"-10"`/`"5"` mean `"-10.000000"`/`"5.000000"`. Do not change the formatter
- Sell/remove arms read `w.unknownCost` before `take`; moving the span-open below the switch must keep that order
- Pro-rate only when shares < 0 before the add; a held or empty pool adds the full cost unrounded, as today (the no-short control row pins it)
- `take`'s else branch must never divide: shares ≤ 0 reaches it with units > 0
- `formatPerShare(nil)` panics; `renderACBPositions:118` filter is the only thing keeping a short off it — keep it

**Orchestrator rulings 2026-10-05 (pre-dispatch):** (1) warning 10a/10b `<x>` = the short after the disposition (pool at -10 selling 5 shows 15) — keeps both clauses literally true; (2) a no-cost acquisition that only covers a short keeps its event `UnknownCost` true (opens no span, as ruled), so warning 4 and the shares-without-cost finding stay consistent. Both defaults stand.

## Phase report

Runs A (Step 1), B1 (Steps 2-4) and B2 (Step 5) done; Steps 6-7 (sweep, verify, tick, STATE rewrite, `status: done`) untouched. Acceptance test GREEN; narrow loop green; `golangci-lint run ./cmd/quarry/...` 0 issues. Full verification not run (V owns it).

- B1 production (unchanged by B2): `internal/report/acb_walk.go` (`add` cover-first and pro-rata, `short()`, `take` negative shares, oversold marking, span opens after the switch only when > 0, `closeSpanWhenSoldOut`/`adjust` at `<= 0`, nil-security `// unreachable:` dropped), `acb.go` (`ACBEvent.Oversold`, `PerShare` nil at `<= 0`, doc comments), `document/acb_warnings.go` (`oversoldWarnings`, slot 10 last). B1 tests: `acb_short_test.go` (9), re-pins in `acb_walk_test.go`, `acb_shares_test.go`, `acb_unknown_cost_test.go`, `document/acb_warnings_test.go` (4 new).
- B2 (tests only, no production): `cmd/quarry/run_acb_short_test.go` adds `shortOpenRows` (buy 100, sell 1,222.84: short open at as-of), `moneyFundShortWarning`, `ACBPerShare` on `shortDoc`, and 6 tests: `Test_run_acb_text_marks_the_oversold_sale_and_lists_no_short_position`, `Test_run_acb_json_writes_a_short_position_with_negative_shares_and_no_acb_per_share`, `Test_run_acb_year_marks_the_oversold_sale_unknown_cost`, `Test_run_acb_security_prints_negative_shares_held_grouped`, plus two cells the plan called n/a but the dispatch named: `Test_run_acb_names_a_removal_beyond_the_pool_after_its_removal_line` (warning 5 then 10b on stderr) and `Test_run_acb_skips_a_return_of_capital_while_the_pool_is_short_as_not_held` (adjustment not-held line, then 10a, no ROC suffix). `cmd/quarry/run_mcp_acb_warnings_test.go` adds `Test_run_mcp_acb_warns_of_a_sale_beyond_the_pool_in_the_cli_words` (tool warnings = the one 10a line = CLI warnings; body = CLI body).
- All 7 new cmd tests GREEN ON ARRIVAL: B1 production already satisfies them; they are surface cells, not new behaviour. Counts: `cmd/quarry` 904 (+7).
- No existing cmd/cli/mcp/document warning list needed re-pinning (full `internal/cli`, `internal/mcp`, `cmd/quarry` green since B1; no fixture sells into an empty pool).
- V must do: Step 6 sweep (doc comments `take`, `unknownCost` field, `closeSpanWhenSoldOut`, `adjust`; spec edits verbatim from `RULING-S22.md` -> *Changes to existing surfaces*: `specification.md:85`, `:206`, `:399` item (c), four edge rows after the table ending `:392`); Step 7 full verify, `spec-check.py phase4de-acb`, tick SCENARIO-22 in `specification.md` with `Test_run_acb_counts_shares_sold_beyond_the_pool_at_no_cost_and_lets_the_next_buy_cover_the_short`, STATE.md rewrite (supersede lines :23, :27, :28; close the `acb_walk.go:~55` open debt; add the slot-10 and short-pool decisions from the Handoff).
