# phase2d-findings — current state

Scenarios complete: SCENARIO-01..03 (02, 03 delivered by 01). Last updated by SCENARIO-01.

## Binding decisions
- `internal/finding` is the leaf (stdlib only) owning type names/order, id grammar (`ID`, `PairID`, `NoPayee`), `StatusOf`, the fix table and `Counts`; `store`, `duckstore`, `snapshot`, `report` and the `findings` command (11) import it (SCENARIO-01)
- Detection runs in Go over `DB.QueryRows` on the build connection, inside `build` before `store_info`; any detection or append error is a build error — previous store untouched, exit 1 (P2d-5) (SCENARIO-01)
- `findings.first_found_at` = the `builtAt` written to `store_info.built_at`, never a fresh `time.Now()`; 06/16/19 compare `new`/`newly_fixed` by equality (P2d-4) (SCENARIO-01)
- `mergeFindings(detected, builtAt)` (`duckstore/findings.go`) is the merge seam: 06 adds the carried-findings parameter (read in `history.go` beside `readRuns`) and sets fixed_at/reopen (SCENARIO-01)
- Detectors select only from `transfers` and `v_cash_flow` (P2d-7); uncategorized entity = `payees.id` or `finding.NoPayee`; uncategorized items carry `transaction_id`, `split_id` only (SCENARIO-01)
- Findings line lives in `renderStore` only (after Transfers, before `Pruned`), never in `renderStoreFailure`; text is `none open` or `N open; run quarry findings to list them` (SCENARIO-01)
- `FormatVersion` is 4; `findings` and `finding_items` are in `storeRelations` (SCENARIO-01)

## Left unbuilt
- `(M new)`, `K fixed since the last sync`, `J ignored` Findings-line clauses and the "history was carried" flag — 06 (new/fixed), 16 (ignored); `Counts.New == Counts.Open` today, so rendering `New` before 06 adds the flag prints `(N new)` on every first sync
- `Counts.Ignored` stays 0 from `duckstore` — 16 sets it via a `snapshot` option
- Carry of `findings` from the previous store — 06; carry faults — 08
- `sync --json` `store.findings`, removal of the one-sided warning and the success `?` rows — 04
- Exported finding row type / read port for reads — 11 designs it

## Traps
- A duplicated EMPTY append does not fail: `duplicateTable: "findings"` fixtures need >= 1 finding (SCENARIO-01)
- Uncategorized is per payee: a fixture whose splits all lack a payee is `1 open`, not the split count (SCENARIO-01)
- A one-sided transfer's from-split is a transfer leg, so `v_cash_flow` never lists it as uncategorized (SCENARIO-01)
- Comparing uncategorized to cashflow goes through totals (`quarry cashflow` prints no uncategorized count); zero-amount splits are invisible there; `--since/--until` must cover future-dated splits (SCENARIO-01)
- `faultDB` in `duckstore_test.go` injects `queryFault` or `scanFault` keyed by query text (`queryFaultOn`); `cmd/quarry/run_store_faults_test.go` `faultDB` embeds `duckstore.DB` too (SCENARIO-01)

## Open debts
- Orchestrator: the findings `Long` line "the splits quarry cashflow counts as uncategorized" needs a product-vision copy ruling before SCENARIO-11 — cashflow prints no uncategorized count (SCENARIO-11)
- Orchestrator: the Reference check (real-file review of heuristic findings) runs after SCENARIO-28, before the gate round (unowned until then)
