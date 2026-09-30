# phase2d-findings — current state

Scenarios complete: SCENARIO-01..15 (02, 03 delivered by 01; 05 by 04; 07 by 06; 10 by 09; 12, 13 by 11). Last updated by SCENARIO-15.

## Binding decisions
- `internal/finding` is the leaf (stdlib only) owning type names/order, id grammar (`ID`, `PairID`, `NoPayee`), `StatusOf`, the fix table and `Counts`; `store`, `duckstore`, `snapshot`, `report` and the `findings` command (11) import it (SCENARIO-01)
- Detection runs in Go over `DB.QueryRows` on the build connection, inside `build` before `store_info`; any detection or append error is a build error — previous store untouched, exit 1 (P2d-5) (SCENARIO-01)
- `first_found_at`/`fixed_at` are the `builtAt` written to `store_info.built_at`, never `now()`; compared by exact SQL equality with it; sync counts come from `mergeFindings`' branches and `Test_replace_counts_new_and_newly_fixed_as_the_findings_stamped_with_the_build_time` pins both agree — 19's `status` counts reuse that SQL definition (SCENARIO-01, 06)
- `mergeFindings(detected, carried, builtAt)` (`duckstore/findings.go`): detected+carried keeps carried `first_found_at`, `fixed_at` NULL, not new; detected only is new at `builtAt`; carried open not detected is fixed at `builtAt` (NewlyFixed); carried fixed not detected is unchanged (Fixed, not NewlyFixed); items only for detected. Carried timestamps are written back as scanned (SCENARIO-06)
- `FindingsCarried` (`store.Replaced`, `store.Result`) is true iff the previous store's `findings` table was present and read, empty table included; `(M new)` renders only when it is true and `New > 0`; `K fixed since the last sync` is `NewlyFixed` (SCENARIO-06)
- Findings are read in `readHistory` on the SAME read connection, after `readRuns` and even when `readRuns` failed; presence is a `duckdb_columns()` column-list query (zero columns → silent, not carried); findings stay carried on an import_runs-only fault (SCENARIO-06, 08)
- Detectors select only from `transfers` and `v_cash_flow` (P2d-7); uncategorized entity = `payees.id` or `finding.NoPayee`; uncategorized items carry `transaction_id`, `split_id` only (SCENARIO-01)
- Findings line lives in `renderStore` only (after Transfers, before `Pruned`), never in `renderStoreFailure`; ruled strings `none open`, `N open; run quarry findings to list them`, `N open (M new), K fixed since the last sync; ...` pinned in `Test_findingsPhrase`; clauses thousands-grouped (SCENARIO-01, 06)
- Carry faults: `store.Replaced`/`Result` hold `FindingsFault *OpenError` (post-open findings read fault, `OpenFaultOther`, fixed `Reason` `its findings table repeats an id` from the `seen` check in `readFindings`, else `its findings table is incomplete` for any other read error incl. a missing `fixed_at`) and `StoreUnreadable bool` (previous store unopenable; `HistoryFault` holds why; store-level can be `OpenFaultOther`, so never infer it from `Fault`). `snapshot.carryWarnings`: `StoreUnreadable` → one combined line (`cannot carry import history and findings forward ...; both start again with this sync`) in the findings slot; else import_runs fault → CF2 line, findings fault → findings line, both may print (CF2 first). A findings fault leaves `FindingsCarried` false, so nothing is marked fixed and `(M new)`/fixed clause drop (SCENARIO-08)
- `finding.MatchDays = 3` is the shared `findings.match_days`; `duplicateQuery` binds it as `abs(date diff) <= ?` and unlinked-transfer (next detector) must reuse the constant, not a second literal. Duplicates: same account, equal non-zero amount, not both `reconciled`, each pair once, id `finding.PairID` (lower numeric txn id first), two items carrying `transaction_id` only; detector runs first in `detectFindings` (SCENARIO-09)
- `FormatVersion` is 4; `findings` and `finding_items` are in `storeRelations` (SCENARIO-01)
- `Outcome.Warnings()` order is manifest, history restart, findings restart, auto-prune; no one-sided entry in text or `warnings[]` — the count survives as the `Transfers` line, `store.transfers.one_sided` and the Findings count (SCENARIO-04)
- Success block prints no `?` rows; only `renderStoreFailure` prints `writeOneSidedRows` (`oneSidedRows`, `otherAccountLabel` stay live there). V1 tail is `fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry` (SCENARIO-04)
- Findings reads live in `report` (no `internal/findings` package; refusal copy is `report`-private): `Store.Findings`, `Server.Findings(FindingsRequest{Ignore})` -> `FindingsListing{Groups, Counts}`; status only via `finding.StatusOf`; groups in `finding.Types()` order, empty groups omitted; Counts cover all findings. `store.FindingList/Finding/FindingItem` carry `New`/`NewlyFixed` computed in SQL by equality with `store_info.built_at`; 14 and 19 count from them (SCENARIO-11)
- `transfers.other_account` (format 4, NULL for paired and numeric links) + `Transfer.OtherAccount`; `FindingItem.OtherAccount/OtherAccountID` feed 14's JSON `other_account`/`other_account_id` and 22's CSV cell (SCENARIO-11)
- `findings --json` (`internal/cli/json_findings.go`): `{status, type, counts, findings[], warnings[]}`; every item key present, null where n/a; `fix` = `Type.Fix().Sentence`; amounts via `jsonMoney`; `findings`/`warnings`/`items` are `[]`, never null; `counts` reuses `findingsDocument`; config warnings go to stderr AND unprefixed into `warnings[]`. 18 MUST make `status`, `type` follow `--status`/`--type` and each entry's `status`/`fixed_at` follow the finding (`newFindingEntryDocument` hardcodes open/nil); 24-28 fill `category`/`transactions`/`splits` (SCENARIO-14)
- `findings` command: flags validated then config loaded strictly (`loadConfig("findings")` + `printConfigWarnings`) then store; hint shows iff >=1 open and `len(req.Ignore) == 0`; `renderFindings(listing, showHint)` owns the default-view footer (incl. `J ignored`) and both empty forms; 18 adds the other status views (SCENARIO-11)
- Sort: duplicate and one-sided by latest item date desc then id; uncategorized by item count desc, payee case-insensitive, id; other types by id. Duplicate item rows render in pair-id order (lower source id first), not date order — accepted (SCENARIO-11)
- `sync --json` `store.findings` is `{open, ignored, fixed, new, newly_fixed}` ints, null iff `Built` false; 19's `status --json` needs `ignored: null`, so it must not reuse `findingsDocument` as is (SCENARIO-04)

- `config.Config.Ignore []string` is `findings.ignore` as written: file order, duplicates, `""` and unknown prefixes kept, nil when unset (whole-struct `assert.Equal` tests break on a non-nil empty slice); 16 fills `FindingsRequest.Ignore` from it, 17's W1 needs every element. Validation order: `snapshots.keep`, `quicken.path`, `findings.ignore`, unknown keys; first refusal wins (SCENARIO-15)
- `internal/config/items.go` `arrayItems` splits `entry.value` text only to find C6e's item index and raw text (first non-string item, from 1); values come from the decoded tree. Comments skipped, nesting/multi-line strings honoured (SCENARIO-15)

## Left unbuilt
- `J ignored` Findings-line clause and `Counts.Ignored` (stays 0 from `duckstore`; 16 sets it via a `snapshot` option) — 16
- `findings --csv` and the `--csv --json` line — 22
- `--status`/`--type` filtering (values validated, not passed on), `ignored` marker, `fixed <date>` lines, fixed sort; `findings --json` hardcodes `status` "open", `type` null and `fixed_at` nil, and `items` is `[]` for a fixed finding — 18
- `FindingsRequest.Ignore` filled from `cfg.Ignore` — 16; W1 — 17
- `findings --json` item `category`, `transactions` and `splits` are always null until 24-28 fill them (`findingItemDocument`); rows and sort rules for unlinked-transfer, mixed-categories, payee-variants, similar-categories, unused-category; `findingLines`' `// unreachable` final `return nil` must go when 24-28 add their types; payee-variants / similar-categories headers use `(N groups)` — 24-28

## Traps
- `report` counts `New` over open-status findings only (ignored new ones are not New), but sync's `Counts.New` from duckstore does not exclude ignored: 16 must make the sync line's `(M new)` and `open` exclude ignored so sync, status and findings agree (SCENARIO-11)
- A v4 store built on this branch before `transfers.other_account` makes `Findings` fail with a read refusal; re-sync (SCENARIO-11)
- `report/fakes_test.go` `fakeStore` lists methods explicitly; `cli` `fakeReportStore` embeds `report.Store`, so an unset `Findings` panics. Duplicate item amount is the transaction's; one-sided item amount is the leg's split (SCENARIO-11)
- `cmd/quarry` `Test_run_help_prints_quarrys_description` pins the exact command list: a new subcommand needs its row (SCENARIO-11)
- A duplicated EMPTY append does not fail: `duplicateTable: "findings"` fixtures need >= 1 finding (SCENARIO-01)
- Uncategorized is per payee: a fixture whose splits all lack a payee is `1 open`, not the split count (SCENARIO-01)
- A one-sided transfer's from-split is a transfer leg, so `v_cash_flow` never lists it as uncategorized (SCENARIO-01)
- Comparing uncategorized to cashflow goes through totals (`quarry cashflow` prints no uncategorized count); zero-amount splits are invisible there; `--since/--until` must cover future-dated splits (SCENARIO-01)
- `faultDB` in `duckstore_test.go` injects `queryFault` or `scanFault` keyed by query text (`queryFaultOn`); `cmd/quarry/run_store_faults_test.go` `faultDB` embeds `duckstore.DB` too (SCENARIO-01)
- Go `builtAt` carries nanoseconds, DuckDB TIMESTAMP microseconds: never `Equal()` a read-back time against `builtAt` in Go (SCENARIO-06)
- `findings.id` is PRIMARY KEY: a repeated carried id reaching `appendTable` would fail the whole build (exit 1); the `seen` check in `readFindings` is what turns it into a warning — keep it (SCENARIO-06, 08)
- `editStore` cannot insert a duplicate into a format-4 `findings` (PK): `DROP TABLE findings` and recreate without the PK (no FK from `finding_items`); `spyReadDB.passQueries` 2 fails the findings columns query, 3 the rows query (SCENARIO-08)
- `spyReadDB` fails EVERY query after `passQueries`, so a runs-fault spy test also fails the findings read; only the DDL fixture (broken `import_runs`, intact `findings`) shows findings survive a runs fault (SCENARIO-06)
- `newBuiltStore`'s previous store holds `one-sided-transfer:xfer-3`, so every `Replace` over it carries findings; count tests on fresh dirs keep `New == Open` (SCENARIO-06)
- Test names for `-run` patterns are matched lowercase-first in this repo's narrow loops; capitalised patterns match nothing (SCENARIO-06)

- A sync fixture with two equal-amount transactions in one account within 3 days now raises a `duplicate` finding: give look-alikes distinct amounts (`twoPayeeBundle` second txn is -11.00) — never weaken the detector (SCENARIO-09)
- Duplicate test names in `duckstore/findings_duplicate_test.go` do not all contain `duplicate`: use `-run 'flag|duplicate'` (SCENARIO-09)

- go-toml `unstable` array/inline-table nodes carry no usable `Raw`, so compound item text comes from splitting `entry.value`; `[[findings.ignore]]` (`ArrayTable`) must go to C6 `got a list of tables`, never C6e — `written` only accepts KeyValue entries (SCENARIO-15)

## Open debts
- Orchestrator: the Reference check (real-file review of heuristic findings) runs after SCENARIO-28, before the gate round (unowned until then)
