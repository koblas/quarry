# phase2d-findings — current state

Scenarios complete: SCENARIO-01..05 (02, 03 delivered by 01; 05 by 04). Last updated by SCENARIO-04.

## Binding decisions
- `internal/finding` is the leaf (stdlib only) owning type names/order, id grammar (`ID`, `PairID`, `NoPayee`), `StatusOf`, the fix table and `Counts`; `store`, `duckstore`, `snapshot`, `report` and the `findings` command (11) import it (SCENARIO-01)
- Detection runs in Go over `DB.QueryRows` on the build connection, inside `build` before `store_info`; any detection or append error is a build error — previous store untouched, exit 1 (P2d-5) (SCENARIO-01)
- `findings.first_found_at` = the `builtAt` written to `store_info.built_at`, never a fresh `time.Now()`; 06/16/19 compare `new`/`newly_fixed` by equality (P2d-4) (SCENARIO-01)
- `mergeFindings(detected, builtAt)` (`duckstore/findings.go`) is the merge seam: 06 adds the carried-findings parameter (read in `history.go` beside `readRuns`) and sets fixed_at/reopen (SCENARIO-01)
- Detectors select only from `transfers` and `v_cash_flow` (P2d-7); uncategorized entity = `payees.id` or `finding.NoPayee`; uncategorized items carry `transaction_id`, `split_id` only (SCENARIO-01)
- Findings line lives in `renderStore` only (after Transfers, before `Pruned`), never in `renderStoreFailure`; text is `none open` or `N open; run quarry findings to list them` (SCENARIO-01)
- `FormatVersion` is 4; `findings` and `finding_items` are in `storeRelations` (SCENARIO-01)
- `Outcome.Warnings()` order is manifest, history restart, auto-prune; no one-sided entry in text or `warnings[]` — the count survives as the `Transfers` line, `store.transfers.one_sided` and the Findings count (SCENARIO-04)
- Success block prints no `?` rows; only `renderStoreFailure` prints `writeOneSidedRows` (`oneSidedRows`, `otherAccountLabel` stay live there). V1 tail is `fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry` (SCENARIO-04)
- `sync --json` `store.findings` is `{open, ignored, fixed, new, newly_fixed}` ints, null iff `Built` false; 19's `status --json` needs `ignored: null`, so it must not reuse `findingsDocument` as is (SCENARIO-04)

## Left unbuilt
- `(M new)`, `K fixed since the last sync`, `J ignored` Findings-line clauses and the "history was carried" flag — 06 (new/fixed), 16 (ignored); `Counts.New == Counts.Open` today, so rendering `New` before 06 adds the flag prints `(N new)` on every first sync
- `Counts.Ignored` stays 0 from `duckstore` — 16 sets it via a `snapshot` option
- Carry of `findings` from the previous store — 06; carry faults — 08
- Text rows for one-sided legs (`one-sided-transfer:xfer-N  date  account  payee  amount  other account: ...`) — 11 reuses `oneSidedRows` columns minus the `?` (SCENARIO-04)
- Exported finding row type / read port for reads — 11 designs it

## Traps
- A duplicated EMPTY append does not fail: `duplicateTable: "findings"` fixtures need >= 1 finding (SCENARIO-01)
- Uncategorized is per payee: a fixture whose splits all lack a payee is `1 open`, not the split count (SCENARIO-01)
- A one-sided transfer's from-split is a transfer leg, so `v_cash_flow` never lists it as uncategorized (SCENARIO-01)
- Comparing uncategorized to cashflow goes through totals (`quarry cashflow` prints no uncategorized count); zero-amount splits are invisible there; `--since/--until` must cover future-dated splits (SCENARIO-01)
- `faultDB` in `duckstore_test.go` injects `queryFault` or `scanFault` keyed by query text (`queryFaultOn`); `cmd/quarry/run_store_faults_test.go` `faultDB` embeds `duckstore.DB` too (SCENARIO-01)
- The six `run_config_test.go` history-restart pins (one const), `run_sync_prune_json_test.go:179` and snapshot `historyRestartLine` all pin the CF2 history line; 08 replaces it for the store-level case, so they change again there (SCENARIO-04)
- `sync --json` `new` equals `open` on a first sync (`Counts.New == Counts.Open`) until 06 carries history (SCENARIO-04)

## Open debts
- Orchestrator: the findings `Long` line "the splits quarry cashflow counts as uncategorized" needs a product-vision copy ruling before SCENARIO-11 — cashflow prints no uncategorized count (SCENARIO-11)
- Orchestrator: the Reference check (real-file review of heuristic findings) runs after SCENARIO-28, before the gate round (unowned until then)
- Checkpoint 01 MINOR (comment budget): `internal/store/duckstore/findings.go:12-13` const doc is 2 lines with a "how" clause — cut to one line
- Checkpoint 01 MINOR (comment budget): `internal/store/duckstore/duckstore.go:391-393` `build` doc is 3 lines — trim to 2
- Checkpoint 01 MINOR (comment): `internal/store/duckstore/findings.go:84-85` "findings come in the query's payee order" is unobservable how — drop; `internal/store/duckstore/doc.go:6` ~89 columns — re-wrap
