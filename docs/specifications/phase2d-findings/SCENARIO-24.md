---
id: SCENARIO-24
status: done
---

# SCENARIO-24: opposite amounts in two accounts not linked as a transfer are an unlinked transfer

Cadence: code-first (read-only detection and rendering; no write guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_sync_unlinked_test.go` `Test_run_sync_records_opposite_amounts_in_two_cad_accounts_as_an_unlinked_transfer_and_not_the_cad_usd_pair`
Narrow loop: `go test ./internal/store/duckstore/ -run 'unlinked|findings|Findings|replace'` then `go test ./internal/report/ ./internal/cli/` (whole packages) and `go test ./cmd/quarry/ -run 'unlinked|findings'`
Mutation checks: none (code-first)
Runs: A (1-2) | B1 (3) | B2 (4-5) | V (6-7)
Size: OWNS A RUN — 3 Build batches (detector; store read + text rows + sort; JSON/CSV cells), 1 feature package (`report`) + duckstore adapter + cli

Survey (step 5): no new port. `store.Store.Findings` keeps its signature; `store.FindingItem` (`internal/store/store.go:368-382`) gains two fields (LSP `findReferences` on `FindingItem`: duckstore reader, `report` sorts, `cli` renderers/JSON; every fake builds it by keyed literal, so no implementer breaks).

Pinned decisions (unruled in spec, decided here):
- Same currency = `accounts.currency` of each side (the label the row prints); `transactions.currency` is not compared.
- Item `Category` (store) = the `full_path` of the transaction's sole split when it has exactly one split and that split has a category; else nil. `Splits` = the transaction's split count. Both filled ONLY for `unlinked-transfer` items (gate in the read SQL on `f.type`); duplicate/one-sided/uncategorized items keep `Category` nil, `Splits` 0, so the duplicate `--json` document is unchanged (`category: null`).
- Text category cell: `Splits > 1` → `(split)`; else nil `Category` → `(uncategorized)` (a 0-split transaction too); else the path. `(split)`/`(uncategorized)` are text-only.
- `--json`/`--csv` `category` = the path, or null for uncategorized AND split (a consumer cannot tell them apart; accepted, flagged to orchestrator). `category_id`, `split_id`, `payee_id`, `transactions`, `splits` stay null for these items (`finding_items` holds `transaction_id` only).
- Items in pair-id order (lower source id first, as duplicates); text row order inside a pair is therefore not date order.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_unlinked_test.go` `Test_run_sync_records_opposite_amounts_in_two_cad_accounts_as_an_unlinked_transfer_and_not_the_cad_usd_pair` — v9fixture (template `run_sync_duplicates_test.go:17-40`, `syncFindingsBundle`): -500.00 Chequing CAD 07-02 + 500.00 Visa CAD 07-03 (categorized, no transfer), plus the same pair across a CAD and a USD account; assert `findings` ids/items hold the CAD pair only (`stringMap`)
- [x] Step 2: no stub needed (no new symbol); run it red at its assertion (empty findings map). Also run the whole `cmd/quarry` narrow loop once to list fixtures the new detector will now hit

### Build
- [x] Step 3 (B1, detector): `internal/store/duckstore/findings.go:13-19,62-97` new `unlinkedTransferQuery` (joins `transactions` a/b with `accounts` for currency; `a.account_id <> b.account_id`, equal currency, `a.amount = -b.amount`, `a.amount <> 0`, `abs(a.date - b.date) <= ?` bound to `finding.MatchDays`, `(a.source_id, a.id) < (b.source_id, b.id)`, NOT EXISTS a split of either in `transfers.from_split_id`/`to_split_id` or with `transfer_account_id` set) + `detectUnlinkedTransfers` (template `detectDuplicates`, `finding.PairID`, two `transaction_id` items) wired in `detectFindings` between one-sided and uncategorized; `export_test.go:13-17` `UnlinkedTransferQuery`; new `findings_unlinked_test.go` (template `findings_duplicate_test.go:21-47`), each clause in and out with a control arm: same account never (duplicate's domain); zero amounts; `-500`/`+500.01`; same sign; 3 days in / 4 days out, both with the lower-id txn dated earlier and later; CAD vs USD never (same-currency control); one leg in `transfers` only, one via `transfer_account_id` only, a second split that is a leg; closed and not-in-reports accounts flagged; 3 matches (A-, B+, C+ in three accounts) = two findings; ids 9 and 10 give `txn-9+txn-10` with items in that order; fault rows `findings_test.go:107-112` for the unlinked query and its scan (`detect unlinked-transfer findings`)
- [x] Step 4 (B2, store read + sort + text): `internal/store/store.go:368-382` `FindingItem.Category`, `.Splits`; `duckstore/findings_read.go:13-27,52-53,68-74` read them (same single query; type-gated); test in `findings_read_test.go` (path, `(split)` source = `Splits 2`, NULL category, 0 splits, and a duplicate item stays nil/0). `report/findings.go:145-163` `findingOrder`: `UnlinkedTransfer` joins Duplicate's case (pair's later date desc, then id) + doc line; test beside `findings_test.go:69` (later date first, tie by id, a reversed-date pair). `cli/render_findings.go:115-138,140-159` `liveFindingLines` case `UnlinkedTransfer`: id line with `ignoredMarker`, then four-space item rows = date, account label, payee, amount right-aligned, two spaces, category cell (no trailing pad; columns padded by `widestRunes`); exact-output tests in `render_findings_internal_test.go` (spec example pair, the `(split)` and `(uncategorized)` cells, ignored marker under `--status all`, closed/USD labels, no-payee) and the ruled header/fix via `Fix()` with hint; end-to-end `cmd/quarry/run_findings_unlinked_test.go` `Test_run_findings_lists_an_unlinked_transfer_pair_with_its_category_cells`
- [x] Step 5 (B2, JSON/CSV): `internal/cli/json_findings.go:83-91` `newFindingItemDocument` sets `Category: item.Category`; update the doc comment (category now filled for this type); CSV needs no code (`csv_findings.go:66-76` reads the document) — keep `csv_findings_internal_test.go` (counts arm still unreachable). Tests: `json_findings_internal_test.go` (path present, `(split)`/`(uncategorized)` both null, `category_id` null), `findings_csv_test.go` (category cell path; NULL unquoted for split/uncategorized), and one `cmd/quarry` `--json` slice in `run_findings_unlinked_test.go`

### Sweep
- [x] Step 6: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; the new detector will raise findings in existing fixtures with opposite amounts in two accounts: give look-alikes distinct amounts in the fixture, never weaken the detector; update exact finding-count assertions that move; drop the `//nolint:exhaustive` reason text in `liveFindingLines` if it names unlinked-transfer; doc comments on new symbols

### Verify
- [x] Step 7: full verification + `spec-check.py phase2d-findings` → tick SCENARIO-24 with its acceptance test

## Handoff

**Binding decisions**
- `unlinkedTransferQuery` binds `finding.MatchDays`; detector order in `detectFindings` is finding.Types order (duplicate, one-sided, unlinked, uncategorized) — the later mixed/payee/similar/unused detectors append in that order.
- `store.FindingItem.Category`/`Splits` are type-gated in the read SQL: a later type that wants a category cell must extend the gate, not drop it, or the duplicate `--json` document changes.
- `--json`/`--csv` `category` is null for uncategorized and split alike; `(split)`/`(uncategorized)` are text-only.
- Unlinked-transfer sort is Duplicate's (`findingOrder` shared case); items render in pair-id order.

**Left unbuilt**
- Rows/sorts/detectors for `mixed-categories`, `payee-variants`, `similar-categories`, `unused-category`, and `--json` `transactions`/`splits` — 25-28.

**Traps**
- `detectFindings` now also flags look-alike fixtures (opposite amounts, two accounts, 3 days): existing `cmd/quarry` and duckstore fixtures may gain an open finding.
- A transfer-linked leg is excluded by EITHER `transfers` membership or `transfer_account_id`: test each alone.
- `PairID` orders numerically (`txn-9` before `txn-10`); the SQL tuple `(source_id, id)` must give the same item order or ids and item rows disagree.

## Phase report

Run V done; steps 6-7 ticked, scenario done. Nothing further to build in this scenario.

- Sweep: `go build ./...` ok, `golangci-lint run ./...` 0 issues; no existing fixture needed changes.
- Verify: covered suite rc=0, `uncovered-diff.py` 0 uncovered added lines; `-race` on duckstore, report, cli, cmd/quarry ok; `test-stats.py --base c8aa14f --changed` TOTAL 931 (+23).
- `specification.md` SCENARIO-24 ticked with its acceptance test; `spec-check.py phase2d-findings` OK; `STATE.md` rewritten.
