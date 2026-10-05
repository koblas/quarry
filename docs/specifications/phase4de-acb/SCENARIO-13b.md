---
id: SCENARIO-13b
status: open
---

# SCENARIO-13b: Shares added with no cost are findings

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_findings_shares_without_cost_test.go` `Test_run_findings_lists_shares_added_with_no_cost_as_status_sync_and_mcp_count_them`
Narrow loop: `go test ./internal/report/... ./internal/store/... ./internal/importer/ ./internal/snapshot/ ./internal/cli/ ./internal/finding/ ./cmd/quarry/ -run '(?i)shares.?without|no.?cost|readtime|read_time|findings|status'`
Mutation checks: `c.Of(a)` non-registered test in the detector → `Test_findings_lists_shares_without_cost_only_in_a_non_registered_account`; add_shares-only action test → `Test_findings_does_not_list_a_reinvest_with_no_cost`; `noCostAcquisition(tx)` replaced by `tx.CostBasis == nil` → `Test_findings_does_not_list_an_add_that_moves_no_units`
Runs: A (1-2) | B1 (3-5) | B2 (6) | V (7-8)
Size: OWNS A RUN, 4 batches, 1 feature package (report). `finding`, the duckstore adapter, the one importer `Result` field and the snapshot call site are plumbing under the sizing note, as in S05.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_findings_shares_without_cost_test.go` (new) `Test_run_findings_lists_shares_added_with_no_cost_as_status_sync_and_mcp_count_them`. Pattern from `run_unclassified_agreement_test.go:14-38` (`newStatusPeer` and `syncFindingsBundleIn`). v9fixture brokerage listed in `accounts.non-registered` plus one registered brokerage. The security and add_shares rows are built as in `run_cost_basis_test.go`: one add with `CostBasis: ""` and one with `"0"` (both NULL, `importer/investments.go:293-304`), one add with a cost, and one no-cost add in the registered account. Assert: the `findings` text group (heading, GroupClause and two rows, ruled copy at spec :334-336) equals the sync line count, `status`, `sync_status` `Findings.Open` and `data_quality` counts. `findings --type shares-without-cost` exits 0.
- [ ] Step 2: `internal/finding/finding.go:26` `SharesWithoutCost` const, signature-only (not yet in `Types()`), so the test compiles. Red at the findings text assertion.

### Build
- [ ] Step 3: inputs in all three sources. `internal/store/store.go`: new `Investments{Securities []Security; Transactions []InvestmentTransaction}`, and an `Investments` field on `FindingList`, `Status` and `Result`, with doc comments updated.
  - `duckstore/findings_read.go:92-98` (`Findings`) and `duckstore/status.go:95-98` (`Status`) gain one `readInvestments` helper. It reuses `readSecurities` and `readInvestmentTransactions` (`investments.go:49-85`) and adds no new SQL. Call it AFTER `readAccounts` so existing `passQueries` fault indices do not shift.
  - Importer: `importer/importer.go:176-181` sets `Investments{securities, investments}`.
  - Snapshot: `snapshot/import.go:194` passes `result.Investments` into the `FindingList`.
  - Report: `report/findings.go:99` (`CountFindings`) maps `st.Investments`.
  - Tests: one readback test each for `Findings` and `Status`. Fault tests (`findings_read_test.go:473-495` and `status_test.go:318-340` style): a query fault and a scan fault for the securities read and for the transactions read, on both methods (8 rows). One snapshot test that the read-time func receives `Result.Investments`.
- [ ] Step 4: type and detector.
  - `finding/finding.go:26-38`: add to `Types()` after `UnclassifiedAccount`, set `ReadTime()` true, and add a `fixes` entry with Sentence, Heading and GroupClause verbatim from spec :334-335. Update `finding_test.go:13,19,216`.
  - `store.FindingItem` (`store/store.go:~640-660`) gains the investment transaction id, security id, security name and shares (millionths).
  - `report/readtime.go:12-15`: `readTimeFindings` concatenates a new `sharesWithoutCostFindings(list, c)`. It lists a row when action is `store.ActionAddShares`, `SecurityID != nil`, `noCostAcquisition(tx)` (`acb_walk.go:348`, never restated) holds, and the account's `c.Of` is non-nil false. The item carries date, account id and name, the ACCOUNT's currency (sibling precedent, `duckstore/findings_read.go:84`), and the four new fields.
  - `report/findings.go:178-182`: a `findingOrder` arm sorting by date ASCENDING, then id (do not reuse descending `latestDate`).
  - Tests in `report/readtime_test.go`, one row per arm:
    - account classes: registered, unclassified and non-registered, with a closed non-registered account listed (`..._only_in_a_non_registered_account`)
    - excluded by `noCostAcquisition`: units 0, negative and nil (`..._an_add_that_moves_no_units`)
    - excluded by action: a reinvest with no cost (`..._a_reinvest_with_no_cost`)
    - an add with a cost, and an add with a nil security
    - a future-dated add, which is listed
    - the empty Classification (unreadable config), which lists none
    - ignored by id; never new or fixed; `ReadTimeStates` included; a `--type` count
    - order rows: date both orders, plus an id tie-break
- [ ] Step 5: document and CSV.
  - `report/document/findings.go:33-50`: `FindingItem` gains `investment_transaction_id`, `security_id`, `security` and `shares` (`document.Shares`) AFTER `splits`, null on every other type. A `NewFindingEntry` arm (`:68-83`) sets date, account_id, account and currency for this type. `NewFindingItem` returns early, so the arm must live in `NewFindingEntry`.
  - `cli/csv_findings.go:12-80`: the same four columns go AFTER `fix`, and `findingItemCSVColumns` covers them. The empty-item row (fixed finding) and every other type's row get four trailing NULLs.
  - Tests: an `encoding/csv` read-back where every record is header width, covering a fixed row, a non-shares row and a shares row. Re-pin the raw JSON and CSV goldens: `cmd/quarry/run_shared_documents_test.go:231-329`, `run_findings_json_test.go:~59`, `run_findings_csv_test.go:60-63`, `document/findings_test.go:112`, `cli/json_findings_internal_test.go:165-250`, `cli/findings_csv_test.go:~295`.
- [ ] Step 6: text and copy, in `cli/render_findings.go:138` and `:351-380`.
  - A new `sharesWithoutCostRows` arm: id, date, `escapeCell(account)` and `escapeCell(security)`, each padded to the widest, then `humanize.Shares(n) + " shares"` and the ignored marker. Pin it in `render_findings_unclassified_internal_test.go` style: widths, escaping and the ignored marker.
  - `cli/findings.go`: the Long type row (20-rune column, `:50-52`), the Long sentence (RULED copy, see Handoff), the `--type` literal (`:141`) and the `--csv` help (`:142`, Part B wording spec :354).
  - Re-pin the `--type` and `--csv` literals: `cli/findings_test.go:141-143`, `cmd/quarry/run_findings_usage_test.go:14` and `run_read_usage_test.go:62`. Re-pin the MCP enum at `run_mcp_descriptions_test.go:212`. Update `schema.md` only if a test says so (`-update`).
  - `plugin/skills/quarry/references/findings.md:23`: add the bullet (spec :339). Change `:43` to "prints one row per transaction, split, payee, category, account or investment transaction". Re-pin `cmd/quarry/run_skill_references_test.go:81-85`.

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Update the doc comments on `FindingList`, `Status`, `Result`, `readTimeFindings`, `findingOrder` and `NewFindingEntry`. Bump the exact-count assertions the suite names.

### Verify
- [ ] Step 8: run the full verification, then `spec-check.py phase4de-acb`. Tick SCENARIO-13b with its acceptance test, rewrite STATE.md (drop the S13b items from Left unbuilt and the `:43` open debt), and set `status: done`.

## Handoff

**Binding decisions:**
- Read-time investment inputs are ONE composite `store.Investments{Securities, Transactions}` carried as `Investments` on `FindingList`, `Status` and `Result`, and filled from the existing duckstore readers. A later read-time investment type reads this field. It never adds a parallel field or a second option, because `Result` and `Status` would then silently disagree with `findings`.
- The detector is: action `add_shares`, `SecurityID != nil`, `noCostAcquisition(tx)`, and `Classification.Of(account)` non-nil false. Spec :333 rules "non-registered accounts only", so unclassified and registered adds never count. With an unreadable config the shares-without-cost count is therefore 0.
- The item currency is the account's currency, as for every other item.
- The four new JSON keys sit after `splits`. The four CSV columns sit after `fix` ("appended to the header end").

**Left unbuilt:**
- The spec sizing row's "excludes 10's pairs" is dead: P2a pairing never shipped.
- The future-dated add is listed (findings have no clock). This is a default, pending ruling.

**Copy needing a ruling BEFORE run B2:**
- Spec :338 gives the gist of the findings Long sentence ("never marked fixed; leaves the list on the sync after the cost is entered"), not verbatim text, and does not say where it goes. Proposed default: `shares-without-cost is never marked fixed: it leaves the list on the sync after its cost is entered in Quicken.`, as its own paragraph after the unclassified-account TOML block.
- `1 shares` versus `1 share` in the text row is unruled.

**Traps:**
- `noCostAcquisition` also accepts reinvests. The detector must AND it with add_shares, because a no-cost reinvest is warning 4b only, never a finding.
- ZCOSTBASIS 0 becomes nil in the importer (`optionalMoney`), not in duckstore, so sync and status agree. Keep both `""` and `"0"` adds in the acceptance fixture.
- Fixtures with no `[accounts]` raise no shares-without-cost finding (the account is unclassified), so the blast radius is only fixtures that list a non-registered account AND hold a no-cost add.

**Orchestrator: product-vision copy ruling 2026-10-05 recorded in spec `#### Finding shares-without-cost` — supersedes this plan where they differ: Long paragraph wording (not the proposed one); sort NEWEST first via `latestDate` (add to the Duplicate/UnlinkedTransfer/OneSidedTransfer arm in `internal/report/findings.go:148-151`, update `findingOrder` doc, pin descending in Step 4); `1 share` at exactly 1 else `shares`; future-dated add listed; JSON/CSV placement as planned; ALSO fix SKILL.md:70 and findings.md:27 per spec and re-pin their skill-text tests. Unclassified/registered add is not a finding (spec :333).**
