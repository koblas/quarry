---
id: SCENARIO-26
status: done
---

# SCENARIO-26: payees that differ only in case, punctuation or numbers are variants

Cadence: code-first (read-only detection and rendering; no write guard, atomic adapter or bug fix)
Acceptance test: `cmd/quarry/run_sync_payee_variants_test.go` `Test_run_sync_records_tim_hortons_variants_and_not_unrelated_payees`
Narrow loop: `go test ./internal/finding/` | `go test ./internal/store/duckstore/ -run 'replace|findings|Findings'` | `go test ./internal/report/ ./internal/cli/` (whole packages) | `go test ./cmd/quarry/ -run 'variants|findings|sync'`
Mutation checks: key cut at first `*`/`#` → `Test_PayeeKey` rows `TIM HORTONS #1234`, `AMZN Mktp CA*1A2B3`, `A#B*C`; digit-token drop (token, not whole name) → `Test_PayeeKey` rows `Store 24`, `7-Eleven #123`; `>= 2 payees` threshold → `Test_replace_flags_payee_variants_only_with_two_payees_sharing_a_key`; payee used by >= 1 transaction → `Test_replace_ignores_a_payee_no_transaction_uses`; read gate `f.type = 'payee-variants'` → `Test_Findings_gives_only_a_payee_variants_item_the_payee_total` (store row hand-built so a dropped gate changes a non-variants item)
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 3 Build batches (key; detector + count read; sort/text/JSON-CSV), `finding` leaf + duckstore adapter + `report` + `cli`

Survey (step 5): no new port; `store.Store.Findings` unchanged; `store.FindingItem.Transactions` (0 = n/a) is reused, so `json_findings.go:92` and `csv_findings.go:72` already emit `payee`, `payee_id`, `transactions` and null date/account/amount/category for an item with neither `transaction_id` nor `split_id`: **no production edit in JSON/CSV, tests only**. `findingItem` (`findings.go:48-50`) already carries `payeeID`; `mergeFindings` (`:207`) already writes it. Callers of changed symbols: `findingOrder` (one caller, `report/findings.go`), `findingsGroupCount` (`render_findings.go`, one caller). LSP not needed (no signature changes).

Pinned decisions (unruled in spec, decided here):
- Normalisation owner: `finding.PayeeKey(name string) string` in `internal/finding/finding.go` (beside `ID`, `PairID`, `MixedMin`) returns the id entity (tokens joined `-`), `""` = skipped. Id = `finding.ID(finding.PayeeVariants, key)`. Steps in spec order: cut at first `*` or `#`; `strings.ToLower`; each rune that is not `unicode.IsLetter`/`unicode.IsDigit` is a space; split on spaces; drop every token with any `unicode.IsDigit` rune; join `-`. Literal reading, pinned: no diacritic folding or NFC (`Café` -> `café`, distinct from `cafe`); a combining mark is a non letter/digit so a decomposed `é` ends a token; `½` (not `Nd`) is a space; `7-Eleven` -> `eleven` (hyphen splits first), `7Eleven` -> `` (one token with a digit).
- Detector: one query `payees p JOIN (payeeTransactions) pc ON pc.payee_id = p.id` ordered by `p.id`; `payeeTransactions` is a SHARED const (`SELECT payee_id, count(*) AS n FROM transactions WHERE payee_id IS NOT NULL GROUP BY payee_id`: every account, a multi-split transaction counts once, no `v_cash_flow`) that the read also uses (count not stored, P2d-1). Grouping in Go by `PayeeKey`; groups with >= 2 payees only; findings emitted in key order, items in count desc, name case-insensitive, payee id order; item = `payee_id` only. Two payees with the same exact name share a key and are flagged (rows repeat the name). A payee no transaction uses is not in the join.
- Read: new `pc` join on `pc.payee_id = fi.payee_id AND f.type = 'payee-variants'`; count column becomes `COALESCE(mc.n, pc.n, 0)`. Mixed items carry `payee_id` too, so the gate is load-bearing.
- Sort (`findingOrder`): sum of item `Transactions` desc, then id string compare. Items are never re-sorted by reader/`report`.
- Text: header `Payee variants (N groups)` / `(1 group)` via `humanize.Count` in `findingsGroupCount` (other types unchanged), fix clause only when the group lists an open finding; id line `  <id>  N payees, M transactions` (id padded to widest across the group's findings, `humanize.Count`, `ignoredMarker` at the end), then four-space rows `<payee name>  <N transactions>` (names padded to widest within the finding, counts right-aligned, `humanize.Count`, widths in runes).
- Test retarget: the two "no row layout" tests move to `finding.UnusedCategory` (not SimilarCategories: 27 changes similar-categories' header to `(N groups)`, so UnusedCategory keeps 27 free of edits here; 28 deletes them).

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_sync_payee_variants_test.go` `Test_run_sync_records_tim_hortons_variants_and_not_unrelated_payees` — v9fixture as `run_sync_mixed_test.go:14-45`: "TIM HORTONS #1234" (2 txns) and "Tim Hortons" (1) plus control "Shell" (1) and "Tim Hortons Cafe" (1, different key); `syncFindingsBundle`; `stringMap` asserts exactly `payee-variants:tim-hortons` in `findings`, its items are the two payee ids (`payee_id` set, `transaction_id`/`category_id` NULL), no finding for Shell
- [x] Step 2: no stub needed; run red at its assertion, plus `go test ./cmd/quarry/ -run 'sync|findings'` once to list fixtures the detector will now hit (payees equal under the key: give look-alikes distinct names, never weaken the rule)

### Build
- [x] Step 3 (B1, key): `internal/finding/finding.go:41` `PayeeKey` + doc (id contract: users write the id into config.toml); `finding_test.go` `Test_PayeeKey` table: `TIM HORTONS #1234`/`Tim Hortons`/`TIM-HORTONS` -> `tim-hortons`, `AMZN Mktp CA*1A2B3` -> `amzn-mktp-ca`, `A#B*C` -> `a`, `Café` -> `café`, `Cafe` -> `cafe`, `Tim's Hortons.` -> `tim-s-hortons`, `  Tim   Hortons  ` -> `tim-hortons`, `Store 24` -> `store`, `7-Eleven #123` -> `eleven`, `7Eleven`, `#1234`, `12345`, `*`, `#`, empty and all-punctuation -> `` (skipped), `東京` -> `東京`, Arabic-Indic digit token dropped, `½` split not dropped
- [x] Step 4 (B1, detector): new `internal/store/duckstore/findings_payee_variants.go` (`payeeTransactions` const, `payeeVariantsQuery`, `detectPayeeVariants`, unexported grouping func); `findings.go:77-99` `detectFindings` between mixed and `slices.Concat` (order = `finding.Types`); `export_test.go:13-17` `PayeeVariantsQuery`; new `findings_payee_variants_test.go`: control pair flagged; single payee no (`Test_replace_flags_payee_variants_only_with_two_payees_sharing_a_key`); three payees one finding; two keys two findings; payee with no transaction not counted (control: it gains one and is flagged; `Test_replace_ignores_a_payee_no_transaction_uses`); transaction in a closed / not-reported account counts; two payees whose keys are empty (`#1`, `#2`) never flagged; `Shop 1`/`Shop 2` flagged `shop`; identical names flagged; item order count desc, name ci, id (tie rows); fault rows in `findings_test.go:107-112` for the query and its scan (`detect payee-variants findings`)

### Build (B2)
- [x] Step 5 (B2, read + sort + text): `findings_read.go:19,31-32,80` `pc` join (type-gated) and `COALESCE(mc.n, pc.n, 0)`; `findings_read_test.go`: variants items carry payee name, `Transactions`, nil category, zero date/account; a mixed item is unchanged; `Test_Findings_gives_only_a_payee_variants_item_the_payee_total` (gate). `report/findings.go:145-163` `findingOrder` case `PayeeVariants` + doc line; test beside `findings_test.go:139` (`variants(id, counts...)` helper: sum desc, id tie, `payee-variants:b` after `:a` on equal sums). `cli/render_findings.go:72-87,117-135` `findingsGroupCount` group form for `PayeeVariants`, `liveFindingLines` case + new `payeeVariantRows(findings, view)`; exact-output tests in `render_findings_internal_test.go` (spec example with its ruled header, fix clause, id line and rows; two findings padding ids; `1 transaction` row; `1,200 transactions` total; ignored marker on the id line under `--status all`; `(1 group)` and `(2 groups)` headers; ignored-only group has no `: fix` clause; non-ASCII names pad by runes); retarget the two "no row layout" tests (`render_findings_internal_test.go:238-261`, `render_findings_status_internal_test.go:117-138`) to `UnusedCategory` with its header text; end-to-end `cmd/quarry/run_findings_payee_variants_test.go` `Test_run_findings_lists_payee_variants_with_a_row_per_payee` (stdout block incl. footer)
- [x] Step 6 (B2, JSON/CSV tests only): `json_findings_internal_test.go` (variants item: `payee` name, `payee_id`, `transactions`; `category`, `category_id`, `splits`, date, account, currency, amount null); `findings_csv_test.go` (row: payee, transactions, payee_id filled; date/account/currency/category/amount NULL unquoted); one `--json` slice in `run_findings_payee_variants_test.go`; no production edit expected, so run green on arrival and say so

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on new symbols (`findingItem`/`FindingItem.Transactions` docs now name payee-variants too); `liveFindingLines` `//nolint:exhaustive` and `findingLines`' `// unreachable` return stay (27, 28 remain)

### Verify
- [x] Step 8: full verification + `spec-check.py phase2d-findings` -> tick SCENARIO-26 with its acceptance test

## Handoff

**Binding decisions**
- `finding.PayeeKey` is the only owner of the payee-variants key and of the id entity; the detector never re-derives it, and a changed rule needs a mid-feature ruling (ids are in users' config.toml). Rule reading above (no diacritic folding / NFC, `unicode.IsDigit` only, hyphen splits before the digit drop).
- Count is derived on read from the SAME `payeeTransactions` const the detector selects from; `FindingItem.Transactions` (0 = n/a) stays the one count field; 27's `Splits` per category follows the same route and must extend the unlinked-only gate (`ts` join), never reuse `pc`/`mc`.
- `findingsGroupCount` group form (`N groups`) is per type; 27 adds `SimilarCategories` to it.
- Variants items: reader and `report` never re-sort items (detector-owned: count desc, name ci, payee id).

**Left unbuilt**
- Detectors, rows, sort for `similar-categories`, `unused-category`; `--json` `splits` stays null; `findingLines`' `// unreachable` return and `liveFindingLines`' `//nolint:exhaustive` go after 28 — 27, 28. The two "no row layout" tests use `UnusedCategory` after 26: 28 deletes them.

**Traps**
- Mixed items carry `payee_id`: an ungated `pc` join would give a mixed item the payee total whenever its `mc` row is absent; dropping the gate is invisible with a detector-fed fixture, hence the hand-built row.
- `payees.name` is `NOT NULL`, but a name can normalise to empty: skip it, do not treat as a group of its own.
- Fixtures with two payees equal under the key (`Amazon`/`AMAZON`, `Store 1`/`Store 2`) now raise a finding: rename them, never weaken the rule.
- `-run 'mixed'`-style patterns miss duckstore `Test_replace_*` tests: use `-run 'replace|findings|Findings'`.

## Phase report

Run V (steps 7-8) done; all steps ticked, `status: done`, SCENARIO-26 ticked in `specification.md`, `spec-check.py phase2d-findings` OK, STATE.md rewritten.
- Sweep: doc comments only (`store.FindingItem.Transactions` names payee-variants; `duckstore.findingItem` notes a variants item holds `payeeID` only). `golangci-lint run ./...` 0 issues, `go build ./...` ok.
- Verify: full covered suite `go test rc=0`; `uncovered-diff.py` 0 uncovered added lines; `go test -race` on finding, store, report, cli, cmd/quarry rc=0.
- Counts (`test-stats.py --base 49c88e5 --changed`): cmd/quarry 344 (+3), internal/cli 242 (+7), internal/finding 13 (+1), internal/report 110 (+1), duckstore 279 (+8); total 988 (+20), tempdir 434 (+4), disk 404 (+3).
