# phase4b-holdings — current state

Scenarios complete: SCENARIO-01 (folds 14). Last updated by SCENARIO-01.

Inherited, still binding: `../phase4a-investments/STATE.md` — read its Binding decisions and Traps for the importer, the share gate (`CheckShares`, tolerance 0.000001), share formatters, `store.Action*`, DECIMAL(18,6) shares, `schema.md` regeneration and the `Z_ENT` / `CAST(... AS REAL)` fixture traps. Only what 4b changed or added is below.

## Binding decisions
- One walk, `holdingSpans` (`duckstore/shares.go`), feeds both the gate (scratch DB in `CheckShares`) and `holding_shares` (`loadHoldingShares` re-walks the build DB inside `build`, after `loadRows`, before `loadFindings`). `store.Rows`, `ImportRun` and the importer are unchanged — S02 `v_holdings` and the S03 port read `holding_shares`, never re-walk (SCENARIO-01)
- The gate stays exact (orchestrator ruling 2026-10-04): `holdingSpans` returns per holding the exact final `big.Rat` (what `compareShares` uses, unchanged) plus spans from the half-even millionths running count. Invariant pinned by `Test_holding_spans_open_span_is_the_rounded_final_count`: open span shares == `millionthsOf(final)`, and no open span ⇔ that rounds to 0 (SCENARIO-01)
- Every walked holding stays a gate key even with zero spans — `Checked` and "no lot against zero" rows depend on it (SCENARIO-01)
- Span rules: boundary on the rounded count at the end of each date; same day netted; `to_date` inclusive (change date − 1, `NULL` on the open span); rounded zero not stored; negative stored; future `from_date` as recorded. PK `(account_id, security_id, from_date)` — spans of one holding never overlap, S02 relies on it (SCENARIO-01)
- `FormatVersion` 7 is the only 4b bump: S02's view and S15's copy add none (no v7 store has shipped). Literal `"7"` pinned in `cmd/quarry/run_shared_documents_test.go:132` and the acceptance test; every other pin compares against the constant (SCENARIO-01)
- `split_old_shares` must be positive through `Replace`: a zero-ratio split fails the build in `holdingSpans`, previous store kept byte-identical (`Test_replace_keeps_the_previous_store_when_holding_shares_cannot_be_loaded`) (SCENARIO-01)
- `needSpan` starts at the earliest cash or investment transaction date; price dates never extend it; future-only → empty span (SCENARIO-01, folded 14)
- A split overflowing DECIMAL(18,6) (±999999999999.999999, needs ~10^12 shares, only reachable through a split multiply) surfaces as the generic build failure with the previous store kept — the 4a precedent for out-of-range investment columns; no copy ruled, none invented. Both bounds pinned in `holding_shares_fault_test.go` (SCENARIO-01)

## Left unbuilt
- `v_holdings`, `(*Store).Holdings`, retiring the `phase4ViewPattern` / `v_holdings` pins — SCENARIO-02
- `holding_shares` sentence in `report/sql_conventions.go`, action-vocabulary sentence, hand copies — SCENARIO-15 (`schema.md` here only gained the table)

## Traps
- `minimalRows()` carries investment rows, which now start the rate span: tests meaning "no transactions" use `noTransactionRows()` (nils `InvestmentTransactions` too), `futureRows()` nils them as well; the narrow loop missed this, only the full suite caught it (SCENARIO-01)
- `newBuiltStore` / `minimalRows` buy fixture: a holding's lot must equal the derived count or the gate refuses — keep the buy and its lot in step when adding fixtures (SCENARIO-01)
- `HoldingWalkQuery` runs in `Replace` too, not only the scratch walk: a `faultDB.queryFaultOn` on it fails the build (SCENARIO-01)
- LSP `findReferences` on `FormatVersion` / `refreshRates` misses `rates.go` call sites and every test; use grep (SCENARIO-01)
- Worktree-isolation hook refuses `go test` / `uncovered-diff.py` whose path comes from a shell variable (`$COVER`, `$(mktemp ...)`): pass the coverage profile as a literal absolute path under the scratchpad, one plain command per call (SCENARIO-01)
- `cmd/quarry/run_investments_test.go` is in Open debts at 480 lines: new tests go in a new file (`run_holding_shares_test.go` is the 4b home) (SCENARIO-01)

## Open debts
- 4a final-pass follow-ups owned by 4b, closed by SCENARIO-15 unless re-opened: "N investment accounts not checked" wording at `internal/cli/render.go:221`; action vocabulary sentence in SQL conventions / `schema.md`; byte-vs-rune column width (`widestLen`) (see `../phase4a-investments/STATE.md`)
- 4a gate-round-1 deferrals (test-file splits, duplicated half-even `QuoRem` in `price.go` `roundHalfEven` and `shares.go` `millionthsOf`, etc.) stay unowned — dies unless re-opened
