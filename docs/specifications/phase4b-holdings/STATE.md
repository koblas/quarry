# phase4b-holdings — current state

Scenarios complete: SCENARIO-01 (folds 14), SCENARIO-02. Last updated by SCENARIO-02.

Inherited, still binding: `../phase4a-investments/STATE.md` — read its Binding decisions and Traps for the importer, the share gate (`CheckShares`, tolerance 0.000001), share formatters, `store.Action*`, DECIMAL(18,6) shares, `schema.md` regeneration and the `Z_ENT` / `CAST(... AS REAL)` fixture traps. Only what 4b changed or added is below.

## Binding decisions
- One walk, `holdingSpans` (`duckstore/shares.go`), feeds both the gate (scratch DB in `CheckShares`) and `holding_shares` (`loadHoldingShares` re-walks the build DB inside `build`, after `loadRows`, before `loadFindings`). `store.Rows`, `ImportRun` and the importer are unchanged — `v_holdings` and the S03 port read `holding_shares`, never re-walk (SCENARIO-01)
- The gate stays exact (orchestrator ruling 2026-10-04): `holdingSpans` returns per holding the exact final `big.Rat` (what `compareShares` uses, unchanged) plus spans from the half-even millionths running count. Invariant pinned by `Test_holding_spans_open_span_is_the_rounded_final_count`: open span shares == `millionthsOf(final)`, and no open span ⇔ that rounds to 0 (SCENARIO-01)
- Every walked holding stays a gate key even with zero spans — `Checked` and "no lot against zero" rows depend on it (SCENARIO-01)
- Span rules: boundary on the rounded count at the end of each date; same day netted; `to_date` inclusive (change date − 1, `NULL` on the open span); rounded zero not stored; negative stored; future `from_date` as recorded. PK `(account_id, security_id, from_date)` — spans of one holding never overlap, `v_holdings` relies on it (SCENARIO-01)
- `FormatVersion` 7 is the only 4b bump: `v_holdings` (built at every `build`, `duckstore.go:454`) and S15's copy add none; no v7 store has shipped, so a dev store built at v7 before S02 lacks `v_holdings` until re-synced. Literal `"7"` pinned in `cmd/quarry/run_shared_documents_test.go:132` and the acceptance test; every other pin compares against the constant (SCENARIO-01, SCENARIO-02)
- `split_old_shares` must be positive through `Replace`: a zero-ratio split fails the build in `holdingSpans`, previous store kept byte-identical (`Test_replace_keeps_the_previous_store_when_holding_shares_cannot_be_loaded`) (SCENARIO-01)
- `needSpan` starts at the earliest cash or investment transaction date; price dates never extend it; future-only → empty span (SCENARIO-01, folded 14)
- A split overflowing DECIMAL(18,6) surfaces as the generic build failure with the previous store kept — the 4a precedent; no copy ruled, none invented. Both bounds pinned in `holding_shares_fault_test.go` (SCENARIO-01)
- `v_holdings` `value`, `value_cad`, `value_usd` are DECIMAL(38,2) (orchestrator ruling), so no overflow failure exists. `convertedTo` stays the 18,2 wrapper for the four existing views; `convertedToWide(target, amount, currency, rate, digits)` owns the half-away-from-zero rule and the view uses 38 (SCENARIO-02)
- Reader `(*duckstore.Store).Holdings(ctx, store.HoldingsParams{AsOf}) (store.Holdings, error)` reads only `v_holdings`; S03's `report.Store.Holdings` copies the signature; S08/S09/S10 extend params and result, never add a second call (SCENARIO-02)
- Go value types: `Value`, `ValueCAD`, `ValueUSD` are `*big.Int` cents, nil when NULL; S03 renderers, `document.Holdings` and the H-3 totals sum and format `*big.Int`, never int64. `Shares`/`Price` int64 millionths. `Holdings.Holdings` is a nil slice when empty — S03 must emit `[]` in `--json`. `USDCAD` is `money.Rate` zero when no rate; a missing account reads as "" / 0 (SCENARIO-02)
- Same-currency conversion needs no rate: a CAD row has `value_cad = value`, a USD row `value_usd = value`, even before the first rate. S08's `no rate` cell and warning must test that a conversion needed a rate, not `value_cad IS NULL` (NULL also covers no price, NULL currency, EUR). `usd_cad` is the rate in force on `date` on every row, NULL before the first rate (SCENARIO-02)
- Sort (S.2 tiers): plain, not case-folded, names — `a.name, a.source_id, v.account_id, v.security, s.source_id, v.security_id` (SCENARIO-02)
- H-2 timing: warm `WHERE date = <recent>` ~50 ms on 145 holdings x 3 spans, 500k prices, 3.6k rates; the filter sits above the day expansion (~723k rows, 0.02 s) and below both ASOF joins (SCENARIO-02)

## Left unbuilt
- `report.Store.Holdings`, `Server.Holdings`, the `quarry holdings` command and document — SCENARIO-03
- `holding_shares` / `v_holdings` sentence in `report/sql_conventions.go`, action-vocabulary sentence, SKILL.md and hand copies — SCENARIO-15 (`schema.md` already carries `v_holdings`, regenerated in S02)

## Traps
- `minimalRows()` carries investment rows, which start the rate span: tests meaning "no transactions" use `noTransactionRows()` (nils `InvestmentTransactions` too), `futureRows()` nils them as well; the narrow loop missed this, only the full suite caught it (SCENARIO-01)
- `newBuiltStore` / `minimalRows` buy fixture: a holding's lot must equal the derived count or the gate refuses — keep the buy and its lot in step (SCENARIO-01)
- `HoldingWalkQuery` runs in `Replace` too: a `faultDB.queryFaultOn` on it fails the build (SCENARIO-01)
- LSP `findReferences` on `FormatVersion` / `refreshRates` misses `rates.go` call sites and every test; use grep (SCENARIO-01)
- Worktree-isolation hook refuses `go test` / `uncovered-diff.py` whose path comes from a shell variable, and any compound command (`cd ... &&`, heredoc, `> file`): pass the coverage profile as a literal absolute path under the scratchpad, one plain command per call; write files with Edit/Write (SCENARIO-01, SCENARIO-02)
- `cmd/quarry/run_investments_test.go` is at 480 lines: new tests go in a new file (`run_holding_shares_test.go`, `run_holdings_view_test.go` are the 4b homes) (SCENARIO-01)
- `v_holdings` operands are cast to DECIMAL(19,6) before multiplying: two DECIMAL(18,6) operands overflow 64 bits at the largest holdings, and `Holdings` reads shares and price through DECIMAL(38,6) for the same reason; do not simplify either cast (SCENARIO-02)
- "Through today" arms use `localToday()` (DuckDB's `current_date`); fixed dates in tests must not be after the real date (SCENARIO-02)
- `phase4ViewPattern` (`run_skill_references_test.go:21`) and the schema.md `Len` pin are shared; a new view's schema.md regen, the `Len` bump and pin retirement must land together (SCENARIO-02)

## Open debts
- Sort of `Holdings` differs from `quarry accounts`: `accountsQuery` (`accounts.go:16`) sorts `lower(name), name`, `holdingsQuery` plain `a.name` (S.2 ruling), so mixed-case account names can order differently across the two commands — S03/final product-vision pass rules on it (SCENARIO-02)
- 4a final-pass follow-ups owned by 4b, closed by SCENARIO-15 unless re-opened: "N investment accounts not checked" wording at `internal/cli/render.go:221`; action vocabulary sentence in SQL conventions / `schema.md`; byte-vs-rune column width (`widestLen`) (see `../phase4a-investments/STATE.md`)
- 4a gate-round-1 deferrals (test-file splits, duplicated half-even `QuoRem` in `price.go` `roundHalfEven` and `shares.go` `millionthsOf`, etc.) stay unowned — dies unless re-opened
