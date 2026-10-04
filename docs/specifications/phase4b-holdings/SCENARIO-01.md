---
id: SCENARIO-01
status: open
---

# SCENARIO-01: Sync records each holding's share count over time

Cadence: test-first — new fallible `holding_shares` load inside `Replace`'s temp-then-rename window (atomicity adapter)
Acceptance test: `cmd/quarry/run_holding_shares_test.go` `Test_run_sync_records_each_holdings_share_count_over_time`
Acceptance test (SCENARIO-14, folded): `internal/store/duckstore/rates_test.go` `Test_replace_asks_for_rates_from_the_earliest_investment_transaction`
Narrow loop: `go test ./internal/store/duckstore/ ./cmd/quarry/ -run '(?i)share|holding|needSpan|rates|relations|tables|schema_reference|format'`
Mutation checks: discard the load's error in `build` (`_ = loadHoldingShares(...)`) → `Test_replace_keeps_the_previous_store_when_holding_shares_cannot_be_loaded` | span shares diverge from the final count (truncate instead of `millionthsOf`, or close the open span one date early) → `Test_holding_spans_open_span_is_the_rounded_final_count` | `needSpan` ignores investment dates → `Test_replace_asks_for_rates_from_the_earliest_investment_transaction`
Runs: A (1-3) | B1 (4-6) | B2 (7) | V (8-9)
Size: OWNS A RUN — 4 batches, 1 feature package (`store/duckstore`; `cmd/quarry` tests only), absorbs SCENARIO-14

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_holding_shares_test.go` (new) `Test_run_sync_records_each_holdings_share_count_over_time` — v9fixture bundle as `run_investments_test.go:436-463`: buy, two rows on one day, 1:2 split, sell, buy dated `time.Now()`+1y; lot = final count. Assert `holding_shares` rows (from/to/shares, `to_date` NULL on last), stdout `Shares    1 holding matches…`, `store_info.format_version` = literal `"7"` (not the constant). Read rows with a multi-row helper next to `importRunQuery` (`run_import_runs_test.go:180`)
- [x] Step 2: `internal/store/duckstore/rates_test.go` `Test_replace_asks_for_rates_from_the_earliest_investment_transaction` — `minimalRows()` + one investment transaction dated before the cash row, recording `fakeRates` (`rates_test.go:22-35`); assert `Need.First` = investment date
- [x] Step 3: `internal/store/duckstore/schema.go:102` — stub: `CREATE TABLE holding_shares` (spec H-1 columns, PK `(account_id, security_id, from_date)`) so Step 1 fails at its row assertion, not on a missing table

### Build
- [x] Step 4: `shares.go:27-114` `holdingWalkQuery` (+`date`) / `holdingShares` → `holdingSpans` (one function; every walked holding keyed, even with zero spans), returning per holding the exact final `big.Rat` **and** its spans; `deriveShares` `:57-75` feeds the gate the exact final count, unchanged. Spans: boundary on half-even millionths (`millionthsOf`) at the end of each date; same day netted; `to_date` = change date − 1; rounded zero not stored; negative stored. `shares_internal_test.go:15-29` `walkRow` emits the date. Tests: `Test_holding_spans_open_span_is_the_rounded_final_count` (`shares_internal_test.go`, real scratch DB; rows: plain buy, split-produced 8.3333333…, 1.5µ tie, non-zero exact rounding to 0 → no open span, negative, fully sold) asserts open span shares == `millionthsOf(final)` and no open span ⇔ that is 0; `Test_check_shares_counts_a_fully_sold_holding_as_zero` (still `Checked`, `Quarry 0` vs non-zero lot). Every existing `shares_test.go` row stays as is (`:126` `8.333332` remains a mismatch)
- [x] Step 5: **test-first** `duckstore.go:453-465` `build` → new `loadHoldingShares` (`shares.go`) after `loadRows`, before `loadFindings`, walking the build `DB` and appending via `decimalCell` (shares width/scale `duckstore.go:57`). Red first: `Test_replace_keeps_the_previous_store_when_holding_shares_cannot_be_loaded` rows, each asserting byte-identical store + only `quarry.duckdb` left (pattern `rates_test.go:201-224`): walk query fault (`faultDB.queryFaultOn` = `HoldingWalkQuery`), zero-ratio split through real `Replace`, append fault (`appendFaultTable: "holding_shares"`), split pushing a count past DECIMAL(18,6) (in-bound control at the max fits)
- [x] Step 6: `Test_replace_records_holding_share_spans` grid read from `holding_shares` after `Replace`, each row one variable from its control: date-before-source_id both orders; same-day netting vs same rows on two days; split multiply; sub-millionth change opens no span; rounded-zero stores nothing; sold to zero then rebought (gap); negative stored; consecutive change days → `from = to`; future `from_date`; two holdings / two accounts kept apart; no investment rows → empty table. Then `duckstore.go:25` `FormatVersion` 6→7; re-pin literal `cmd/quarry/run_shared_documents_test.go:132`; `query_test.go:30-35` `storeRelations` + `holding_shares`; regenerate `plugin/skills/quarry/references/schema.md` (`go test ./cmd/quarry/ -run Test_skill_schema_reference_matches_the_committed_file -update`). Step 1 goes green
- [ ] Step 7: `rates.go:26-27` `finishBuild`, `:50-63` `refreshRates` (take `rows`), `:128-146` `needSpan` — earliest of cash and investment dates; prices never. `rates_internal_test.go:38,48,69,78` call sites + rows: investment earlier (head), cash earlier (control), investment only, cash only, both empty, investment future-only → empty, future investment + past cash → cash date, equal dates, price earlier than both ignored. Step 2 goes green

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments: `doc.go:1-17` (holding_shares built by the walk), `holdingSpans`, `loadHoldingShares`, `needSpan`, `refreshRates`, `export_test.go:3` (walk query now also runs in Replace)

### Verify
- [ ] Step 9: full verification + `spec-check.py phase4b-holdings` → tick SCENARIO-01 and SCENARIO-14 (`delivered by SCENARIO-01 —` before its test); write `docs/specifications/phase4b-holdings/STATE.md`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- One walk, `holdingSpans` (`duckstore/shares.go`), feeds both the gate (scratch DB in `CheckShares`) and `holding_shares` (re-walks the build DB inside `build`, after `loadRows`). `store.Rows` / `ImportRun` and the importer are unchanged — 4b readers (S02 `v_holdings`, S03 port) read `holding_shares`, never re-walk
- The 4a gate is not loosened (orchestrator ruling 2026-10-04): `holdingSpans` returns per holding the exact final `big.Rat`, which the gate compares exactly with tolerance 0.000001 as in 4a, plus the spans from the half-even millionths running count. Invariant, pinned by `Test_holding_spans_open_span_is_the_rounded_final_count`: open span shares == `millionthsOf(final)`, and no open span ⇔ that rounds to 0 — so "gate count = the `to_date IS NULL` span's shares, else 0" holds to the millionth that table stores
- Every walked holding stays a gate key even with zero spans — `Checked` and "no lot against zero" rows depend on it
- Span rules: boundary on the rounded count at end of each date; `to_date` inclusive (change date − 1); same-day netted; rounded zero not stored; negative stored; future `from_date` as recorded. PK `(account_id, security_id, from_date)` — S02 relies on spans of one holding never overlapping
- `FormatVersion` 7 is the only 4b bump: S02's view and S15's conventions copy add no bump (no v7 store has shipped)
- `needSpan` starts at the earliest cash or investment transaction date; price dates never extend it (H-4)

**Left unbuilt** — named so nobody assumes it exists:
- `v_holdings`, `(*Store).Holdings`, retiring `phase4ViewPattern` / `v_holdings` pins — SCENARIO-02
- `holding_shares` sentence in `report/sql_conventions.go`, action vocabulary sentence — SCENARIO-15 (schema.md here only gains the table)

**Traps** — things that look right and are not:
- LSP `findReferences` on `FormatVersion` / `refreshRates` misses `rates.go:47`, `rates.go:27` and every test; caller list came from grep. All pins but `run_shared_documents_test.go:132` compare against the constant and cannot see the bump — the acceptance test asserts `"7"` literally
- `HoldingWalkQuery` now runs in `Replace` too: a `faultDB.queryFaultOn` on it fails the build, not just the scratch walk
- DECIMAL(18,6) overflow is reachable only through a split multiply; it surfaces as the generic build failure (previous store kept) — no copy ruled, none invented
- `cmd/quarry/run_investments_test.go` is in Open debts at 480 lines: new tests go in the new file

## Phase report

Run B1 (steps 4-6) done; cadence test-first for Step 5. Acceptance `Test_run_sync_records_each_holdings_share_count_over_time` green; narrow loop green except Step 7's `Test_replace_asks_for_rates_from_the_earliest_investment_transaction` (red by design, B2). Whole `go test ./...` ran: only that test fails.
- `duckstore/shares.go`: `holdingSpans` (walk query now selects `date`) returns `map[holdingKey]*holdingWalk{final *big.Rat, spans []shareSpan}`; `holdingWalker` (`endDayBefore`/`endDay`) rounds at each date end; `deriveShares` maps to finals (gate unchanged); `loadHoldingShares`, `nullableSpanEnd`. `duckstore.go` `build` calls `loadHoldingShares` after `loadRows`; `FormatVersion` 7.
- Step 5 red first (all five rows failed "An error is expected but got nil" / empty scalar), then green. Files: `holding_shares_fault_test.go` (fault grid + `Test_replace_stores_a_share_count_at_the_column_maximum` control), `holding_shares_test.go` (`Test_replace_records_holding_share_spans`, 14 rows).
- `shares_internal_test.go`: `walkRow` emits `walkDay`; `Test_holding_spans_open_span_is_the_rounded_final_count`; `shares_test.go`: `Test_check_shares_counts_a_fully_sold_holding_as_zero`.
- Mutations (red quoted in report): `_ = loadHoldingShares` -> all 4 fault rows red; truncate instead of `millionthsOf` -> `1.5 millionths tie` row red (1 vs 2); drop the final-day flush -> 7 invariant rows red.
- Pins: `run_shared_documents_test.go:132` 7, `query_test.go` `storeRelations` + `holding_shares`, `schema.md` regenerated.
- Plan deviations (existing tests retargeted, not undone): `duckstore_test.go:484-485` `..._largest_investment_amounts_...` split_old was -max, now +max (a non-positive ratio now fails Replace via the walk); `fakes_test.go` `newBuiltStore` appends one buy dated 2026-03-18 so `holding_shares` is non-empty for `Test_query_prints_every_column_of_each_table_and_view` (`minimalRows` alone yields no span: its split follows no buy).
- Not done: Step 7 (`needSpan`), Sweep (lint, doc comments listed in Step 8), Verify.
