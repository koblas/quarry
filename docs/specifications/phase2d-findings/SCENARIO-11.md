---
id: SCENARIO-11
status: open
---

# SCENARIO-11: findings lists open findings by type with their fix

Cadence: code-first (no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_findings_test.go` `Test_run_findings_lists_open_findings_by_type_with_their_fix`
Acceptance test (SCENARIO-12, folded): `cmd/quarry/run_findings_test.go` `Test_run_findings_says_no_open_findings_when_the_store_has_none`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_findings_test.go` `Test_run_findings_refuses_a_store_built_before_findings_existed`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/importer/ ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run 'findings|name_form|reads|usage|refuse|interrupt|config'`
Mutation checks: none
Runs: A (1-2) | B1 (3-5) | B2 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (`report`) + duckstore adapter + `store` row types; one importer field (`nameFormLeg`), see Handoff

Departures from the sizing line, ruled here:
- **No `internal/findings` package.** The store/interrupt refusal copy is unexported in `report` (`refusal.go:28-60`) and a sibling feature cannot import it; `report` already owns every read command, its factory and the `run.go:29` port guard, and 19 reads findings counts through the same port. Everything lands in `report`.
- **`transfers.other_account` column.** The name a name-form one-sided leg records exists only in memory (`importer/transfers.go:130-137`); `store.Rows` does not carry validation legs, so the ruled row `other account: Savings (not in this file)` is unrenderable from the store without it. `main` is FormatVersion 3, so format 4 absorbs the column, no bump.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_findings_test.go` (new) — three tests above. SCENARIO-11: v9fixture sync (pattern `run_sync_findings_test.go:22-43`) with a duplicate pair built by `categorizedTxn` (`run_sync_duplicates_test.go:18-22`), a name-form transfer leg to "Savings" (not in the file), one payee with uncategorized splits; every other amount distinct; exactly 3 findings; assert full stdout (three groups in order, headers, rows, `3 open findings`, hint line), empty stderr (also pins "stale store silent"), exit 0. SCENARIO-12: `replaceStore` (`run_helpers_test.go:76-80`) with rows raising no finding → stdout `No open findings\n`, exit 0. SCENARIO-13: `writeStoreFixture` + `store_info` format 3, mirroring `run_read_refusals_test.go:93-108` → R2 line with `--from` fix, exit 1, empty stdout
- [ ] Step 2: signature-only stubs so it compiles: `report/store.go:11-22` `Store.Findings(ctx) (store.FindingList, error)`; `(*report.Server).Findings`; `(*duckstore.Store).Findings`; `store.FindingList`/`store.Finding`/`store.FindingItem` types; `report/fakes_test.go:11-57` `fakeStore.Findings`; `cli/findings.go` `newFindingsCommand` registered in `cli/root.go:27-33`

### Build
- [ ] Step 3: row types + persistence of the recorded name — `store/store.go:121-126` `Transfer.OtherAccount *string`; `importer/transfers.go:130-137` `nameFormLeg` sets it; `duckstore/schema.go:67-72` `other_account VARCHAR`; `duckstore.go:554-560` `transferRows`; `store/store.go` near `:328` `FindingList{Findings []Finding}`, `Finding{ID, Type, FirstFoundAt, FixedAt *time.Time, New, NewlyFixed bool, Items}`, `FindingItem{TransactionID, SplitID, PayeeID, CategoryID *string; Date; AccountID, Account, Currency; Closed, Active; Payee; Amount; OtherAccount, OtherAccountID *string}`. Tests: `importer/transfers_test.go:144-172` pins `OtherAccount`; `duckstore_test.go:90` shape gains the column; numeric-link leg keeps NULL
- [ ] Step 4: `duckstore/findings_read.go` (new) `(*Store).Findings` — `openRead`, ONE query: `findings` LEFT JOIN `finding_items` → `transactions`, `accounts`, `payees`, `splits`, `transfers` (on `from_split_id = item.split_id`), `New`/`NewlyFixed` as SQL equality with `store_info.built_at`; amount = the item split's amount when `split_id` set, else the transaction's; `OtherAccountID` = the from-split's `transfer_account_id`; a fixed finding has zero items; errors via `openFault`. Tests `duckstore/findings_read_test.go`: each of the three types' items; other-account arms (name not in file, linked, numeric nil); fixed finding no items; New/NewlyFixed after a second `Replace`; closed + USD account fields. Faults: add `Findings` to `rowReads()` `read_faults_test.go:21-33` (open, query, scan, close tests run over it)
- [ ] Step 5: `report/findings.go` (new) `FindingsRequest{Ignore []string}`, `FindingsListing{Groups []FindingsGroup{Type, Findings}, Counts finding.Counts}`, `(*Server).Findings` — status only via `finding.StatusOf(FixedAt != nil, id in Ignore)`; only open findings listed; groups in `finding.Types()` order, empty groups omitted; Counts Open/Ignored/Fixed/New/NewlyFixed over all findings; sorts per Surface & Copy: duplicate (later item date desc, id), one-sided (date desc, id), uncategorized (item count desc, payee case-insensitive, id), any other type by id; store error → `s.readRefusal(ctx, "findings", err)`. Tests `report/findings_test.go`: each sort's tie-break just past the first key (equal later date → id; equal count → payee ci; equal payee → id), ignored id counted not listed, fixed counted not listed, store fault → `RefusalError` naming no store, cancelled ctx → `findings interrupted`

### Build (B2)
- [ ] Step 6: `cli/findings.go` (new) `newFindingsCommand` — Use/Short/Long/Example verbatim (Surface & Copy `quarry findings`; Long includes the uncategorized row "splits with no category, one finding per payee; / quarry cashflow counts them as income or spending"); `--status` (default `open`) and `--type` with ruled help sources; Args refusal line (positional) ending `Run 'quarry findings --help' for usage.`; `--status`/`--type` validated to `UsageError` lines (type list joined from `finding.Types()`); RunE order: flags → `loadConfig("findings")` + `printConfigWarnings` (`snapshots.go:43-47`) → `openReport` → `srv.Findings(ctx, report.FindingsRequest{})` → `emit`. Tests: `cli/findings_test.go` help Long/Examples/flag lines verbatim (pattern `report_help_test.go`); `run_read_usage_test.go:24-45` rows positional / bad `--status` / bad `--type`, plus bad `--status` with no store still exit 2; `run_read_refusals_test.go:47-56` and `:160-166` `findings` rows (no store; interrupt); `run_config_test.go` findings: one C row exit 1 empty stdout, one C3 warning then the listing exit 0
- [ ] Step 7: `cli/render_findings.go` (new) `renderFindings(listing, showHint)` — headers `Heading (N): GroupClause` from `finding.Fix` (uncategorized `(N payees, M splits)` via `humanize.Count`); duplicate id line + 4-space item lines padded per finding; one-sided rows reusing `render.go:333-353` columns without `?` (extract the shared column builder, `renderStoreFailure` unchanged); uncategorized `id  payee  N splits  first to last` (single date when equal), blank line between groups; footer `N open findings`/`1 open finding` + `; J ignored and K fixed not shown (--status all)` (zero clauses omitted); hint when ≥1 open and `showHint` (cli passes `len(req.Ignore) == 0`); `No open findings` and its `; … not shown` form. Tests: `render_findings_internal_test.go` spec example rows, `(CAD, closed)`, `(USD)` with native amount, `(no payee)`, other-account three arms, footer matrix (1/2 open; ignored only; fixed only; both; neither), both empty forms, hint on/off

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new exported symbols; `report` package doc and `newRootCommand` doc (`root.go:7-9`) name `findings`

### Verify
- [ ] Step 9: full verification + `spec-check.py phase2d-findings` → tick SCENARIO-11, and SCENARIO-12/13 as "delivered by SCENARIO-11" with their acceptance tests

## Handoff

**Binding decisions:**
- Findings reads live in `report` (`Store.Findings`, `Server.Findings`, `FindingsRequest`, `FindingsListing`); no `internal/findings` package — refusal copy is `report`-private and 19 reads counts through the same port method.
- `transfers.other_account` (NULL for paired and numeric links) + `store.Transfer.OtherAccount`; `FindingItem.OtherAccount/OtherAccountID` feed 14's JSON `other_account`/`other_account_id` and 22's CSV cell.
- `Finding.New`/`NewlyFixed` are computed in SQL by equality with `store_info.built_at`; 14 and 19 count from them, never from Go time comparison.
- `FindingsRequest.Ignore []string` is the ignore input; status only via `finding.StatusOf`; hint shows iff ≥1 open and `len(Ignore) == 0`. 16 fills `Ignore` from config.
- Config: `findings` loads strictly (P2d-10) in RunE after flag validation, before the store; 15 adds the key, 16 threads it.
- `renderFindings` owns the default-view footer (incl. `J ignored` clause) and both empty-state forms; 18 adds the other status views.

**Left unbuilt:**
- `findings --json` document — 14 (until then RunE does not branch on `--json`); `--csv` flag and `--csv --json` line — 22.
- Filtering by `--status`/`--type`, `ignored` marker, `fixed <date>` lines, fixed sort — 18 (values validated here, not passed on).
- `Config` ignore field, C6/C6e — 15; W1 — 17.
- Rows and sort rules for unlinked-transfer, mixed-categories, payee-variants, similar-categories, unused-category — 24-28.

**Traps:**
- A v4 store built on this branch before `transfers.other_account` makes `Findings` fail with a read refusal; re-sync.
- `report/fakes_test.go` `fakeStore` lists methods explicitly; `cli` `fakeReportStore` embeds `report.Store`, so an unset `Findings` panics.
- Duplicate item amount is the transaction's; one-sided item amount is the leg's split.
