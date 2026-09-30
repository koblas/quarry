# phase2d-findings — current state

Scenarios complete: SCENARIO-01..07 (02, 03 delivered by 01; 05 by 04; 07 by 06). Last updated by SCENARIO-06.

## Binding decisions
- `internal/finding` is the leaf (stdlib only) owning type names/order, id grammar (`ID`, `PairID`, `NoPayee`), `StatusOf`, the fix table and `Counts`; `store`, `duckstore`, `snapshot`, `report` and the `findings` command (11) import it (SCENARIO-01)
- Detection runs in Go over `DB.QueryRows` on the build connection, inside `build` before `store_info`; any detection or append error is a build error — previous store untouched, exit 1 (P2d-5) (SCENARIO-01)
- `first_found_at`/`fixed_at` are the `builtAt` written to `store_info.built_at`, never `now()`; compared by exact SQL equality with it; sync counts come from `mergeFindings`' branches and `Test_replace_counts_new_and_newly_fixed_as_the_findings_stamped_with_the_build_time` pins both agree — 19's `status` counts reuse that SQL definition (SCENARIO-01, 06)
- `mergeFindings(detected, carried, builtAt)` (`duckstore/findings.go`): detected+carried keeps carried `first_found_at`, `fixed_at` NULL, not new; detected only is new at `builtAt`; carried open not detected is fixed at `builtAt` (NewlyFixed); carried fixed not detected is unchanged (Fixed, not NewlyFixed); items only for detected. Carried timestamps are written back as scanned (SCENARIO-06)
- `FindingsCarried` (`store.Replaced`, `store.Result`) is true iff the previous store's `findings` table was present and read, empty table included; `(M new)` renders only when it is true and `New > 0`; `K fixed since the last sync` is `NewlyFixed` (SCENARIO-06)
- Findings are read in `readHistory` on the SAME read connection, after `readRuns` and even when `readRuns` failed; presence is a `duckdb_columns()` column-list query (zero columns → silent, not carried). 08 must keep findings carried on an import_runs-only fault and reads the same column set for "incomplete" (SCENARIO-06)
- Detectors select only from `transfers` and `v_cash_flow` (P2d-7); uncategorized entity = `payees.id` or `finding.NoPayee`; uncategorized items carry `transaction_id`, `split_id` only (SCENARIO-01)
- Findings line lives in `renderStore` only (after Transfers, before `Pruned`), never in `renderStoreFailure`; ruled strings `none open`, `N open; run quarry findings to list them`, `N open (M new), K fixed since the last sync; ...` pinned in `Test_findingsPhrase`; clauses thousands-grouped (SCENARIO-01, 06)
- `FormatVersion` is 4; `findings` and `finding_items` are in `storeRelations` (SCENARIO-01)
- `Outcome.Warnings()` order is manifest, history restart, auto-prune; no one-sided entry in text or `warnings[]` — the count survives as the `Transfers` line, `store.transfers.one_sided` and the Findings count (SCENARIO-04)
- Success block prints no `?` rows; only `renderStoreFailure` prints `writeOneSidedRows` (`oneSidedRows`, `otherAccountLabel` stay live there). V1 tail is `fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry` (SCENARIO-04)
- `sync --json` `store.findings` is `{open, ignored, fixed, new, newly_fixed}` ints, null iff `Built` false; 19's `status --json` needs `ignored: null`, so it must not reuse `findingsDocument` as is (SCENARIO-04)

## Left unbuilt
- `J ignored` Findings-line clause and `Counts.Ignored` (stays 0 from `duckstore`; 16 sets it via a `snapshot` option) — 16
- Findings carry fault reasons (`its findings table repeats an id`, `its findings table is incomplete`), `Replaced.FindingsFault`, findings-only and store-level warnings — 08; until then a findings read fault silently starts findings history
- Text rows for one-sided legs (`one-sided-transfer:xfer-N  date  account  payee  amount  other account: ...`) — 11 reuses `oneSidedRows` columns minus the `?` (SCENARIO-04)
- Exported finding row type / read port for reads — 11 designs it

## Traps
- A duplicated EMPTY append does not fail: `duplicateTable: "findings"` fixtures need >= 1 finding (SCENARIO-01)
- Uncategorized is per payee: a fixture whose splits all lack a payee is `1 open`, not the split count (SCENARIO-01)
- A one-sided transfer's from-split is a transfer leg, so `v_cash_flow` never lists it as uncategorized (SCENARIO-01)
- Comparing uncategorized to cashflow goes through totals (`quarry cashflow` prints no uncategorized count); zero-amount splits are invisible there; `--since/--until` must cover future-dated splits (SCENARIO-01)
- `faultDB` in `duckstore_test.go` injects `queryFault` or `scanFault` keyed by query text (`queryFaultOn`); `cmd/quarry/run_store_faults_test.go` `faultDB` embeds `duckstore.DB` too (SCENARIO-01)
- The six `run_config_test.go` history-restart pins (one const), `run_sync_prune_json_test.go:179` and snapshot `historyRestartLine` all pin the CF2 history line; 08 replaces it for the store-level case, so they change again there (SCENARIO-04)
- Go `builtAt` carries nanoseconds, DuckDB TIMESTAMP microseconds: never `Equal()` a read-back time against `builtAt` in Go (SCENARIO-06)
- `findings.id` is PRIMARY KEY: a repeated carried id reaching `appendTable` fails the whole build (exit 1) — 08 adds a `seen` check at read time that turns it into a warning (SCENARIO-06)
- `spyReadDB` fails EVERY query after `passQueries`, so a runs-fault spy test also fails the findings read; only the DDL fixture (broken `import_runs`, intact `findings`) shows findings survive a runs fault (SCENARIO-06)
- `newBuiltStore`'s previous store holds `one-sided-transfer:xfer-3`, so every `Replace` over it carries findings; count tests on fresh dirs keep `New == Open` (SCENARIO-06)
- Test names for `-run` patterns are matched lowercase-first in this repo's narrow loops; capitalised patterns match nothing (SCENARIO-06)

## Open debts
- Orchestrator: the findings `Long` line "the splits quarry cashflow counts as uncategorized" needs a product-vision copy ruling before SCENARIO-11 — cashflow prints no uncategorized count (SCENARIO-11)
- Orchestrator: the Reference check (real-file review of heuristic findings) runs after SCENARIO-28, before the gate round (unowned until then)
