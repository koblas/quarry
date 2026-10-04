---
id: SCENARIO-09
status: done
---

# SCENARIO-09: Status reports the share check without "not imported" (folds SCENARIO-10)

Cadence: code-first (nothing on the mandatory test-first set: no write-safety guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_status_shares_test.go` `Test_run_status_reports_the_share_check`
Acceptance test (SCENARIO-10, folded): `cmd/quarry/run_sync_pre4a_store_test.go` `Test_run_sync_twice_over_a_version_5_store_carries_import_history_forward`
Narrow loop: `go test ./cmd/quarry/ ./internal/cli/ ./internal/report/... ./internal/importer/ ./internal/store/... -run '(?i)status|shares|import_runs|history|replace|renderStore|rowsPhrase|json|pre4a|SchemaReference|skill_schema'`
Mutation checks: `investment_transactions_not_imported` out of `requiredRunColumns` (`history.go:34-38`) → `Test_run_sync_twice_over_a_version_5_store_carries_import_history_forward`; status Shares line reads `run.SharesChecked` (`render_status.go`) → `Test_run_status_reports_the_share_check`; status document has no `not_imported` key → `run_status_json_test.go` full-document pin
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN, absorbs SCENARIO-10 — 3 Build batches. Only `internal/importer` is a feature package; `store`, `store/duckstore`, `report/document`, `cli` are shared types, adapter, renderers, so the >1-feature-package SPLIT trigger does not fire.

Resolved against the code: I4-9 / S10 do not conflict. `checkFormat` runs only from `openRead` (status, accounts, spend, `BuiltFrom`; `BuiltFrom`'s callers `snapshot/list.go:193`, `prune.go` run only for `snapshots`/`prune` and for auto-prune after `Replace`). Sync reaches the old store through `Replace` -> `readHistory` (`duckstore.go:330`, `history.go:92`), which calls `openReadOnly` with no format check, so a v5 store is carried and rebuilt as v6; only the readers refuse it. S10's Then is reachable as written. It is already true today (the column exists in both v5 and v6), so its test is green on arrival; it turns red during step 4 and green in step 5.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_status_shares_test.go` (new) `Test_run_status_reports_the_share_check` — `holdingsBundle` (`run_investments_test.go:307`), `sync` then `status` and `status --json`: `Shares    2 holdings match Quicken's share counts` on stdout, no `not imported` in Rows, JSON `"shares": {"checked": 2}`, `store.rows` `investment_transactions` 3 / `securities` 2 / `prices` 3, no `not_imported` key. The MCP `sync_status` arm of the Then is carried by step 3's `run_shared_documents_test.go` pin (MCP document equals `status --json`), not by this test. Red at the missing Shares line. No stubs needed.
- [x] Step 2: `cmd/quarry/run_sync_pre4a_store_test.go` (new) `Test_run_sync_twice_over_a_version_5_store_carries_import_history_forward` — `writeStoreFixture` (`run_read_refusals_test.go:22`) with a v5 store: `store_info` format_version 5 plus `import_runs` with the 19 Phase 1 columns (`id` .. `investment_transactions_not_imported`) and the nine optional columns `snapshot_taken_at` .. `rates_fetch_error`, none of `securities_rows`, `prices_rows`, `investment_transactions_rows`, `shares_checked`; one run (id 7, `investment_transactions_not_imported` 13); then `sync --quicken` twice. Each exit 0, stderr empty (no import-history warning); ids 7, 8, 9; run 7's snapshot_path and counts intact and its four 4a columns NULL; `store_info.format_version` = `duckstore.FormatVersion`; `import_runs` has no `investment_transactions_not_imported` column. Green on arrival except the last assertion: say so in the report.

### Build
- [x] Step 3 (B1, status + render + JSON surfaces; all pins re-pinned once here):
  - `internal/report/document/status.go:12-23` `Status` add `Shares StatusShares` `json:"shares"` right after `Splits`, delete `NotImported`; `:142` `NewStatus` fills `Shares` from `run.SharesChecked`; new `StatusShares{Checked int}` beside `StatusSplits` (~`:55`). `common.go:39-42` delete `NotImported`.
  - `internal/cli/render.go:272-289` `rowsPhrase(c)` loses its second parameter and the `not imported` tail; callers `:93`, `:512`. `render_status.go:27` same, and a `Shares` line after Splits from `sharesPhrase(run.SharesChecked)` (`render.go:293`, the sync formatter, never a copy); fix `renderStatus` doc. `json.go:40,186` delete `storeDocument.NotImported`.
  - Tests: `render_status_internal_test.go:74,100` (drop count, add Shares line; table over `shares_checked` 0 / 1 / N, three variants); `render_internal_test.go:183-212` (drop the `notImported` column and its two rows), `:286-295`, `:709-715`; `json_status_internal_test.go:73`, `json_internal_test.go:157` (drop `not_imported` from `storeKeys`); cmd re-pins `run_shared_documents_test.go:~132-190` (MCP `sync_status` equals `status --json`; add `shares`), `run_status_json_test.go:~56-115` (checked 0 is the control beside step 1's 2), `run_json_test.go:101`, `run_investments_test.go:352`, `run_transfers_test.go:178` (Rows tail gone; pin Store/Rows/Shares lines only, STATE trap). Not applicable: `--account`, no fault path added.
- [x] Step 4 (B2a, delete `not_imported` store-side):
  - `internal/importer/importer.go:86-90` delete the survey call (keep the `investmentEnt, hasInvestment` lookup, `mapInvestmentTransactions` reads it), `:156,166,176` `notImported`, `:185-190` `newImportRun` loses `n` and the field. `transactions.go:51-74` delete `transactionSurveyQuery` and `surveyTransactions`.
  - `internal/store/store.go:234` delete `ImportRun.InvestmentTransactionsNotImported`; `:283-299` `Result.NotImported` and its doc; `:375-379` delete `store.NotImported`.
  - `duckstore/schema.go:127` drop the column; `status.go:22,66` drop select and scan; `duckstore.go:694` drop the cell in `importRunRows`.
  - Tests: delete `importer/not_imported_test.go`; `import_faults_test.go:89,187` drop the `ZTRANSACTION ids` rows; `import_runs_test.go:41`; `duckstore_test.go:78,117`; `run_import_runs_test.go:70` (drop the column, `"1 0 0 1"`).
  - Ends deliberately red: `history.go` still requires the column, so every Replace-over-a-v6-store test and step 2's second sync fail. That is the S10 red; do not patch around it.
- [x] Step 5 (B2b, tolerant carry, S10 green):
  - `duckstore/history.go:34-38` `requiredRunColumns` drops `investment_transactions_not_imported` (the 18 columns every format has); `:311-337` `carriedRun.counts` `[13]` -> `[12]`; fix the comment.
  - Tests: `history_test.go:16-24,106` the Phase 1 DDL stays as the older-format fixture, drop the not-imported predicate from the carried-row assertion and add column-absent scalar on the rebuilt `import_runs`; `history_faults_test.go:50` drop the row from `importRunsColumns` and add a case "a store whose import_runs lacks the column is carried" (positive control: a store lacking `transfers_one_sided` stays `reasonRunsIncomplete`); `rates_floor_test.go:25` stays as is. Mutation: put the column back into `requiredRunColumns`, see step 2 and the new case fail.
  - Regenerate `plugin/skills/quarry/references/schema.md` (row `:109` goes) with `go test ./cmd/quarry/ -run Test_skill_schema_reference_matches_the_committed_file -update`.

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `rowsPhrase`, `Result`, `ImportRun`, `requiredRunColumns` say what is now true; grep `not_imported|NotImported|not imported` for leftovers outside S11's accounts copy (`render_accounts.go:13`, `accounts.go:17`, `json_accounts.go:17`, `store.go:52`, fx and accounts tests: SCENARIO-11). Add the one-line superseded note to `docs/specifications/phase1-import-store/specification.md:216-228` (S.7).

### Verify
- [x] Step 7: full verification per `.claude/rules/agent-briefs.md`, `spec-check.py phase4a-investments`; tick SCENARIO-09 with its acceptance test, tick SCENARIO-10 as `delivered by SCENARIO-09` with its own test last on the line; roll `STATE.md` (drop the `not_imported` Left-unbuilt and Traps lines).

## Handoff

**Binding decisions**:
- Status `shares` sits right after `splits` in `document.Status` — mirrors sync's order (balances, splits, shares, transfers); S.4 does not pin it, and 4b's reuse and the three status pins depend on it. Text Shares line sits after Splits, before Transfers.
- `shares_checked` stays the only persisted share fact; there is no `shares_mismatched` (I4-8).
- Sync over an older-format store works by design: `Replace`/`readHistory` never run `checkFormat`; only readers refuse (I4-9). Keep that: a format check in `readHistory` would break S10.
- `import_runs` has no `investment_transactions_not_imported`; history tolerates it present (older store) or absent (v6).
- `importer.Result.NotImported`, `store.NotImported`, `document.NotImported`, `ImportRun.InvestmentTransactionsNotImported` no longer exist.

**Left unbuilt**: accounts `not valued` cell and help, sync Long, SKILL.md, `sql_conventions.go`, `store.IsInvestmentAccount` comment (`store.go:52`) — SCENARIO-11. `docs/adr/001-shared-store-package.md:22` still names `NotImported`; unowned, historical.

**Traps**:
- The orchestrator's brief listed `investmentEntity` for deletion: it stays (`entities.go:18,26`, `importer.go:85`); only its survey use goes.
- The column is `schema.go:127`, not `:98`; `history.go` also holds `counts [13]int64`, which must become 12.
- `rowsPhrase` is shared by sync success, sync failure and status; step 3 must change all three call sites together.
- Rows-tail pins also sit outside the four named tests (`run_investments_test.go:352`, `run_transfers_test.go:178`, `render_internal_test.go`); a narrow loop over only the four misses them.

## Phase report

Run V done; steps 1-7 ticked, `status: done`. Nothing left for this scenario.

Sweep: `go build ./...` ok, `golangci-lint run ./...` 0 issues. Doc comments on `rowsPhrase`, `requiredRunColumns` already state the current contract (no edit). The superseded note for phase1's `not_imported` bullet was already in place (`phase1-import-store/specification.md:224`, plus the S21 tick line); no further edit. Leftover `not imported` grep: production hits are SCENARIO-11's accounts copy only (`render_accounts.go:13,79`, `accounts.go:17`, `json_accounts.go:17`, `store.go:52`); test hits are Phase 1 DDL fixtures, the NotContains pins in `run_status_shares_test.go`, and accounts tests.

Verify: full suite `go test rc=0` (one covered run); `uncovered-diff.py --profile ... ad2fbac`: 0 uncovered added lines; `go test -race` on cli, importer, store/..., report/... ok. `test-stats.py --base ad2fbac --changed`: cmd/quarry 676 (+2), internal/cli 423 (+0), internal/importer 240 (-5), internal/store/duckstore 560 (+1), TOTAL 1899 (-2) / tempdir 830 (-3) / disk 752 (-3). The -5 is the deleted `not_imported_test.go`.

Spec: SCENARIO-09 and SCENARIO-10 ticked (10 as delivered by 09, own test last); `spec-check.py phase4a-investments` and `phase1-import-store` run, see the report.
