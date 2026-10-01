# Review round 1 — phase2d-findings

Range `5b420e1..HEAD` (HEAD 976300b at fix dispatch). Coverage gate: 0 uncovered added lines, 5 declared unreachable. Mutation sample: 20/226, 20 killed, 636s. Full suite rc=0, lint 0.

## Triggered reviewers
- arch-reviewer, correctness-reviewer ×2 (A: store/importer/finding/snapshot/config/quicken; B: cli/report/cmd), test-reviewer ×2 (same split), refactor-advisor — all on 132 `.go` files.
## Skipped
- api-reviewer (no HTTP), pipeline-reviewer (no `.claude/` change).

## BLOCKER
none

## MAJOR
1. correctness-B — `internal/cli/csv.go:17,27` via `render_sql.go:87`: a one-column NULL row prints as a blank line; Go `encoding/csv` and Python `DictReader` skip it (rows silently lost). **Ruled at the gate** (spec P2d-12 + sql Long amended, 976300b): a record whose only field is NULL is written as `""`. Fix code + Long `internal/cli/sql.go:55-57` + pins (`sql_test.go:224-226`; invert `csv_internal_test.go:63` "a one-column NULL row is a blank line"); add a test reading the output back with `encoding/csv` (row count equal).
2. correctness-B — `internal/cli/status.go:30,36`: `status` opens the store twice (`srv.Status`, then `srv.FindingCounts`); a sync rename between them mixes builds. Fix: one read — e.g. `Store.Status` also returns finding states (or one port call returning both), classified in `report`; `FindingCounts` no longer opens separately. Test: a fake store that changes between calls cannot produce mixed output (status and counts from one call).
3. test-A — `internal/store/duckstore/findings_unused_category_test.go:~125`: hidden guard pinned one level only; one-step `markBlocked` mutant survives (reports a category with a hidden grandchild — destructive advice). Add rows: hidden grandchild under an unused visible chain → nothing reported; used hidden grandparent + used middle + unused leaf → leaf not reported; (optional) used visible parent, unused child, hidden sibling → unused child still reported.
4. test-B — `internal/cli/render_findings.go:397` `uncategorizedSpan`: first/last not pinned for items out of date order (positional mutant survives). Add a row test with dates Mar 5, Mar 1, Mar 3 → `2026-03-01 to 2026-03-05`.
5. test-B — `internal/cli/json_findings.go:95`: `other_account_id` never asserted non-null in findings `--json`. Add a one-sided item with `OtherAccount` and `OtherAccountID` set, full item map asserted.
6. test-B — `internal/cli/findings_test.go:243,266,272`: `fixed <date>` expectations depend on machine TZ (fails under Pacific/Auckland, Kiritimati). Pin `time.Local` (shared helper, `t.Cleanup`).
7. test-B — `internal/cli/sql.go:55-57` / `sql_test.go:224-226`: Long + pin carry pre-amendment copy (fold with item 1).
8. test-B — `internal/cli/sql_csv_test.go:149`: `--csv --json` precedence only partly pinned. Add rows `{"--csv","--json","  "}` → blank-query message; `{"--csv","--json","SELECT 1","SELECT 2"}` → `sql takes one query; quote it as one argument`.

## MINOR (fold — cheap)
- correctness-A — `duckstore.go:329`/`snapshot/import.go:150`: carried findings of an unknown type are counted (fixed) by sync but not by `findings`. Fix: leave unknown-type carried rows out of `states` (one rule with `knownFindings`), with a test.
- correctness-B + test-B — `internal/report/findings.go:218`, `internal/cli/render_findings.go:346` unreachable reasons: cite where "only fixed findings have no items" is established (`mergeFindings`, every detector emits ≥1 item); `render_findings.go:139`/`report/findings.go:183` reasons may name the `exhaustive` linter.
- test-A — `config/ignore_test.go:~25`: `assert.Nil(cfg.Ignore)` not `Empty`.
- test-A — `finding/finding_test.go:~28`: add `PairID("txn-b","txn-9")` mixed numeric/non-numeric case.
- test-A — `importer/category_refs_test.go:~104`: move "a split under an imported transaction" into its own test (`..._for_a_split_the_store_keeps`).
- test-A — `importer/import_faults_test.go:~89,~173`: add `want` per row, `ErrorContains` the `readCategoryRefs` wrap.
- test-B — `report/findings_test.go:244`: add generic store-fault-returned-unchanged test for `Findings`.
- test-B — `cmd/quarry`: one `run()` `sql --csv` test over a real synced store (NULL, "", DECIMAL) + `sql --csv --json` exit 2.
- test-B — delete `Test_similarCategoryRows_labels_an_income_group_by_its_prefixed_id` (echoes fixture).
- test-B — header comments on `render_findings_similar_internal_test.go`, `render_findings_variants_internal_test.go`; merge `inZone`/`useZone`; merge `Test_findingsFooter` into `Test_findingsFooter_by_view`; split multi-When hint/help tests.
- correctness-A NIT — `store.go:126` `Transfer.OtherAccount` doc ("nil for a pair and for a numeric link"); `store.go:139` `ReferencedCategoryIDs` doc add split entries under non-imported transactions.
- refactor — `duckstore/findings.go`: extract `detectPairs` (duplicate/unlinked); table-driven `detectFindings` in `finding.Types` order; `render_findings.go` `idWidthOf` helper; one `FindingItem.CategoryPath()` replacing the three NOT-NULL arguments and `cmp.Or(…, new(string))`; `store.Finding` methods (`Payee()`, `Transactions()`) replacing duplicated `payeeOf`/`transactionsOf`; doc on `unusedCategoryItem`, `newCategoryRefs`, `categoryRefSources`; name the `markBlocked` closure.
- test-A NITs — test names for `BasicString`, `PayeeKey`, `CategoryKey` behaviour-named.

## Deferred to STATE Open debts (not this pass)
- arch MINOR: detection rules live in duckstore SQL (no change now).
- arch NIT: compile-time guard `*duckstore.Store` satisfies `report.Store`.
- correctness-A: runs-only carry fault still carries findings and marks them fixed (planner decision, SCENARIO-06) — confirm at final product-vision pass.
- refactor NITs: `stringEnd` doc, `Findings` compose split, `findingOrder` doc.

## Verdict: BLOCKED
