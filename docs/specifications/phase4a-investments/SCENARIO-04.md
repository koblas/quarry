---
id: SCENARIO-04
status: open
---

# SCENARIO-04: Sync reports holdings that match Quicken's share counts

Cadence: test-first — no-swap-on-mismatch (`Validation.Failed()` gains shares) is a write-safety guard
Acceptance test: `cmd/quarry/run_investments_test.go` `Test_run_sync_reports_holdings_that_match_quickens_share_counts`
Acceptance test (SCENARIO-05, folded): `cmd/quarry/run_investments_test.go` `Test_run_sync_applies_a_stock_split_in_date_order`
Acceptance test (SCENARIO-08, folded): `cmd/quarry/run_investments_test.go` `Test_run_sync_reports_a_file_with_no_investment_data`
Narrow loop: `go test ./internal/importer/ ./internal/store/duckstore/ ./internal/platform/duckdb/ ./internal/cli/ ./cmd/quarry/ -run '(?i)share|lot|holding|rowsPhrase|investment|validation|_json|shared_documents|import_run'`
Mutation checks: shares clause in `store.Validation.Failed` (`store.go:391-393`) → `Test_run_sync_leaves_the_previous_store_byte_identical_when_share_counts_differ`; `Z_ENT = ?` in the lots query → `Test_import_leaves_out_a_lot_of_another_entity`; `<=` → `<` on the tolerance compare → `Test_check_shares_matches_within_one_millionth` (row "exactly 0.000001")
Runs: A (1-2) | B1 (3-4) | B2 (5-7) | V (8-9)
Size: OWNS A RUN — 5 batches (ceiling), 1 feature package (importer) plus its Store adapter (store/duckstore) as in S01/S02, + cli; S05 and S08 folded

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_investments_test.go` — three tests through `run`: S04 (two holdings in two accounts, one closed, lots equal to Σ shares; plus a cash account): exit 0, empty stderr, Rows line in full, `Shares    2 holdings match Quicken's share counts`, then a `--json` sync of the same bundle decoded: `store.shares` = `{checked:2, mismatched:[]}` (`[]`, not null), `store.rows` has `investment_transactions`/`securities`/`prices`. S05: buy 120, 1:12 split (new 1, old 12), sell 10, lots 0 — rows added out of date order (sell first) so Z_PK order gives a different count; exit 0 and `1 holding matches Quicken's share count`. S08: cash-only file: Rows ends `; 0 investment transactions, 0 securities, 0 prices`, `Shares    no holdings to check`, exit 0
- [x] Step 2: `internal/quicken/v9/v9fixture/builder.go:14-25,179-185,228,237-250` `EntLot`, `LotRow`, `(*Builder).Lot` — signature-level so step 1 compiles; tests red at the Rows/Shares assertions

### Build
- [x] Step 3: **Quicken's reference.** `v9fixture/builder.go:417-425,470-590,596-630` — `Lot` seeds ZLOT (Z_ENT default, ZPOSITION, ZLATESTUNITS text, deleted), `"Lot"` in the Z_PRIMARYKEY list and `maxPKForEntity`; `internal/importer/entities.go:17-25` `lotEntity` optional; new `internal/importer/lots.go` lots query + mapper summing parsed units (S02 `parseShares`, `price.go:40`) per holding over `mapPositions` (`investments.go:68-89`), skipping a position whose security is not in `securityRefs`; `store.go:183-198` non-persisted `Rows.QuickenShares` (quarry account id, security id, millionths); `importer.go:118-141` wiring. Tests (`internal/importer/lots_test.go`): sum of two lots; deleted lot; `Test_import_leaves_out_a_lot_of_another_entity` (+ positive control, same row with default entity); lot under a deleted position, a skipped-account position, a non-imported security; Lot entity missing with no investment rows → empty reference; fault test: lots query error (`QueryRows` fault-table row matching `FROM ZLOT`). **NEEDS RULING before B1:** unreadable `ZLATESTUNITS` (not a number, beyond 6 decimals, too large) and NULL units have no copy/outcome in S.5/S.6 — orchestrator gets a scoped product-vision ruling; developer implements it verbatim with one pin per ruled case
- [x] Step 4: **The I4-5 owner.** `internal/platform/duckdb/duckdb.go:45-69` new in-memory create (same lock-down DSN; no path, no chmod) + its test; `internal/store/duckstore/duckstore.go:82-136` scratch-opener field + `With…` option, default wired in `New`; new `internal/store/duckstore/shares.go`: unexported walk query over `investment_transactions` (security_id NOT NULL, ORDER BY account, security, date, `source_id`) + Go fold (exact rational: add `shares`, NULL adds 0; a `split` row multiplies by new/old and adds nothing) taking a QueryRows-only interface; named tolerance constant 0.000001; `(*Store).CheckShares(ctx, rows)` = scratch db → `schemaDDL` Exec → `appendTable("investment_transactions", investmentTransactionRows(...))` → walk → compare with `rows.QuickenShares`; `store.ShareCheck{Checked, Mismatched []ShareMismatch{AccountID, SecurityID, Quarry, Quicken}}` (millionths, half-even from exact) in `store.go`. Tests (`shares_test.go`): date order beats source_id order; same-date tie by `source_id` 9 vs 10 (string order disagrees); split multiply 1:12; split row with non-zero `shares` (pins "multiply only"); bound rows 0.000001 match / 0.000002 differ / split-produced 100×1/12 vs 8.333333 match and vs 8.333332 differ; holding set: txn-only (Quicken 0), lot-only (quarry 0), neither (not counted), cash-only row excluded, zero-share row still checked; checked count. Fault tests, one per call: scratch create, DDL Exec, append, walk query
- [x] Step 5: **Gate + record.** `store.go:383-393` `Validation.Shares`, `Failed()` includes `len(Shares.Mismatched) > 0` — test-first; `internal/importer/ports.go:21-26` `Store.CheckShares`; `store_fake_test.go:10-41` fake gains configurable check result/error, default = no mismatch (importer tests seeding units stay green); `importer.go:150-155` call `CheckShares` always, after `validate`, before the `Failed()` return (cash failure still carries shares); error → `fmt.Errorf("check share counts: %w")`, no Replace. `store.go:214-226` `ImportRun.SharesChecked` set in `newImportRun` (`importer.go:171-182`); `shares_checked BIGINT` after `investment_transactions_rows` in `schema.go:137-139`, `history.go:41-45` optional + `carriedRun` `history.go:311-345`, `importRunRows` `duckstore.go:652-670`, `status.go:16-30,60-70` COALESCE→0 into `run.SharesChecked`. Tests: `Test_import_does_not_replace_the_store_when_share_counts_differ` (fake mismatch, `replaceCalls == 0`, Built false, `ErrValidationFailed`); `Test_import_reports_share_counts_alongside_a_balance_failure`; CheckShares error never reaches Replace (inject the wrap `CheckShares` itself returns); `Test_run_sync_leaves_the_previous_store_byte_identical_when_share_counts_differ` (cmd/quarry, model `run_validation_test.go:214-303`; cash checks pass; differs from a passing control only in one lot's units; exit 1; no stderr pin); `shares_checked` carried by history (null for an older run) and read by status; regenerate `schema.md` (`-run Test_skill_schema_reference_matches_the_committed_file -update`). Re-fixture real-store holdings that now hit the gate: the S02 fixture `cmd/quarry/run_investments_test.go:92-222` (units at :117,:125,:127, incl. the 1:12 split — lots equal the derived count, within 0.000001 of a millionth) also used by `:224`; S03 refusals (:248,:264) stop before the gate
- [x] Step 6: **Text.** `internal/cli/render.go:264-279` `rowsPhrase(c, n)`: cash clause `; ` investment clause (always; each noun inflects; comma-grouped) then the existing not-imported tail while n > 0; new shares phrase (`N holdings match Quicken's share counts` / `1 holding matches Quicken's share count` / `no holdings to check`); Shares line after Splits, before Transfers in `renderStore` (`render.go:90-101`) and, as its pass line, in `renderStoreFailure` (`render.go:430-460`; DIFFER form is S06); `render_status.go:27` keeps calling `rowsPhrase` (status Shares line is S09). Tests: `Test_rowsPhrase` (`render_internal_test.go:178-218`) rows per noun at 0/1/many + tail; `Test_sharesPhrase` 0/1/2/1,605; balance-failure block shows Shares pass line. Re-pin Rows/Shares at `render_internal_test.go:292,624`, `render_status_internal_test.go:98`, `cmd/quarry/run_status_test.go:52`, `run_validation_test.go:91,165,277,348`, `run_success_test.go:61`, `run_schema_test.go:111`, `run_transfers_test.go:82,176,225,266`, `run_import_test.go:99`
- [x] Step 7: **JSON.** `internal/report/document/common.go:23-51` `Rows` + `NewRows` gain `investment_transactions`, `securities`, `prices` (after `transfers`); `internal/cli/json.go:30-41,148-165` `shares` object after `splits`: `{checked, mismatched}` with `mismatched` always `[]` (entries are S06). Tests: `json_internal_test.go` shares doc (0 checked, N checked, `[]` not null); re-pin `cmd/quarry/run_json_test.go:79`, `run_shared_documents_test.go:135-150`, `run_status_json_test.go:59-70`, `internal/cli/json_status_internal_test.go:23-35`; MCP `sync_status` follows status (`run_mcp_status_test.go:34`)

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `Validation.Failed` (`store.go:389-390`) and `Import` (`importer.go:42-48`), which say "balance or split-sum"; on `CheckShares`, `ShareCheck`, `Rows.QuickenShares`, the walk func (states it is the single share-count owner), in-memory create

### Verify
- [ ] Step 9: full verification + `spec-check.py phase4a-investments` → tick SCENARIO-04, SCENARIO-05 and SCENARIO-08, each folded line naming "delivered by SCENARIO-04" before its test

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Seam C = (i): `importer.Store.CheckShares(ctx, rows) (store.ShareCheck, error)` runs on an in-memory scratch DuckDB built with the store's own `schemaDDL` + `investmentTransactionRows`/`appendTable`; Import calls it always, before `Failed()`, so Replace never runs on any failed check (P1-2 intact) — the guard lives in `Failed()`, not in Replace
- S06/S07 consume `Result.Validation.Shares`: DIFFER block in `renderStoreFailure`, a third clause in `validationFailedRefusal` (`internal/snapshot/import.go:240-252`) after balances, splits; mismatch labels (account name/currency/closed/active, security name/ticker) are resolved from `rows.Accounts`/`rows.Securities` in the importer or added to `ShareMismatch` — S06 decides; `ShareMismatch` carries ids and millionths only
- The walk (duckstore `shares.go`) is the single I4-5 owner; it takes a QueryRows-only interface so 4b runs it on a `ReadDB` of the real store — never re-derive shares elsewhere
- Comparison is exact (rational); only `ShareMismatch.Quarry/Quicken` are rounded half-even to millionths. A split row multiplies and adds nothing (pinned arm; S12 cannot tell the readings apart)
- Lot entity missing → empty reference, holdings with transactions are checked against 0 (STATE: optional entity = empty data, never a refusal); S.6's "no holdings to check" holds only when there are no investment rows either
- `shares_checked` is a nullable BIGINT after `investment_transactions_rows`, carried and COALESCE-read like its siblings; `store.shares` sits after `store.splits` in sync `--json`

**Left unbuilt** — named so nobody assumes it exists:
- `store.shares.mismatched` entries, DIFFER block, stderr shares clause — SCENARIO-06 (until then a mismatch serializes `mismatched: []`)
- Status/MCP `"shares":{"checked":N}` and the status Shares line from `run.SharesChecked` — SCENARIO-09
- Rows tail `; N investment transactions not imported` and its S04 acceptance pin — SCENARIO-09 drops both

**Traps** — things that look right and are not:
- A shares-only failure prints `validation failed: ; …` until S06 adds the clause — do not pin S04 stderr on a mismatch
- The guard test must keep cash checks passing; a balance failure also keeps the store, so the shares mutation would stay green
- `duckdb.Create` stats and chmods its path — it has no in-memory mode; use the new platform create

## Phase report

Run B2 (steps 5-7) done. Narrow loop green (store, duckstore, importer, platform, cli, report, cmd/quarry); the three acceptance tests (S04, S05, S08) pass. Not run yet: lint, full covered suite, `uncovered-diff.py`, spec tick, STATE.md (run V).

Files:
- Gate: `internal/store/store.go` `Validation.Shares`, `Failed()` shares clause, `ImportRun.SharesChecked`; `internal/importer/ports.go` `Store.CheckShares`; `importer.go` calls it always after `validate`, error wrapped `check share counts: %w`, never reaches Replace; `newImportRun` sets `SharesChecked`
- Column: `duckstore/schema.go` `shares_checked BIGINT` (after `investment_transactions_rows`), `history.go` optional + `carriedRun`, `duckstore.go` `importRunRows`, `status.go` COALESCE into `run.SharesChecked`; `plugin/skills/quarry/references/schema.md` regenerated
- Text: `internal/cli/render.go` `rowsPhrase` (investment clause always, then the not-imported tail), `sharesPhrase`, Shares line in `renderStore` and, only while no share mismatch, in `renderStoreFailure`
- JSON: `document/common.go` `Rows`/`NewRows` +3 keys (also status JSON `rows`); `cli/json.go` `sharesDocument{checked, mismatched []any{}}` after `splits`
- Tests: `store_test.go` `Test_Validation_*`; `importer/share_gate_test.go`; `store_fake_test.go` `shareCheck`/`shareErr`; duckstore `history_test.go`/`status_test.go`/`duckstore_test.go` shares_checked; `cmd/quarry/run_investments_test.go` `oneHoldingBundle`, byte-identical guard + control, S02 fixture lot `0.541667`; cli `Test_sharesPhrase`, `Test_rowsPhrase`, failure subtests, `Test_renderJSON_reports_the_shares_checked_*`; Rows/Shares/JSON re-pins across cmd/quarry and cli

Decisions made (not in plan):
- `renderStoreFailure` omits the Shares line while shares mismatch (a "N holdings match" line would be false); S06 adds the DIFFER form. Pinned by `Shares line is left out while a count differs`
- `store.shares.mismatched` is `[]any{}` (no entry type until S06)
- Status text/JSON do not print Shares (S09), but their Rows line/object now carry the investment counts (`rowsPhrase`/`NewRows` shared)

Mutation (backup-copy protocol): `store.go` `Failed()` dropped ` || len(v.Shares.Mismatched) > 0` reddened `Test_Validation_fails_when_a_share_count_differs` ("Should be true") and `Test_run_sync_leaves_the_previous_store_byte_identical_when_share_counts_differ` (exit 0 vs 1, store replaced). Restored, diff clean. The `<=`/`<` tolerance and `Z_ENT` mutations were done in B1.

Run V must know:
- Step 8 doc comments to check: `CheckShares`, `ShareCheck`, `Rows.QuickenShares`, the walk func, `CreateInMemory` (`Failed` and `Import` already say balance, split-sum or share-count)
- Orchestrator-ruled in B1: missing-Lot-entity refusal outranks row-level refusals
- Tick S04 plus folded S05, S08 in `specification.md`; STATE.md `## Left unbuilt` still lists `ZLOT`/Rows keys/Shares line, now built
