---
id: SCENARIO-06
status: open
---

# SCENARIO-06: Sync fails when holdings' share counts differ from Quicken (folds SCENARIO-07)

Cadence: code-first — nothing on the mandatory set: S04's write-safety guard (`CheckShares` → `Failed()` → no `Replace`, `importer.go:158-167`) is left unchanged
Acceptance test: `cmd/quarry/run_share_gate_test.go` `Test_run_sync_fails_when_holdings_share_counts_differ_from_quicken`
Acceptance test (SCENARIO-07, folded): `cmd/quarry/run_share_gate_test.go` `Test_run_sync_joins_a_share_failure_to_a_balance_failure_in_one_line`
Narrow loop: `go test ./internal/importer/ ./internal/cli/ ./internal/snapshot/ ./internal/report/document/ -run '(?i)share|validation|StoreFailure|refusal' && go test ./cmd/quarry/ -run 'share_failure|share_counts_differ'`
Mutation checks: shares-only tail predicate in `(*Server).validationFailedRefusal` (make it "shares failed" instead of "only shares failed") → `Test_sync_and_import_reports_a_failed_check_in_the_validation_refusal` row "balances and shares joined" + `Test_run_sync_joins_a_share_failure_to_a_balance_failure_in_one_line`
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 4 batches; importer (labels) + snapshot (stderr) + `store` type, cli not counted. Sizing pass ruled OWNS A RUN; strict SPLIT seam named in Handoff
Developer model: sonnet suffices — display and copy over a settled seam.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_share_gate_test.go` (new) `Test_run_sync_fails_when_holdings_share_counts_differ_from_quicken` — fixture: closed CAD account "RRSP" holding "iShares Core Equity ETF" ticker XEQT, derived 120.5 vs lots 110.5; an open account's holding with transactions and no lots; a third holding that matches; cash checks pass; previous-store sentinel. Text run: Store `NOT REBUILT` line, the contiguous `Shares    DIFFER for 2 of 3 holdings` block (S.2 row verbatim), exact S.3 shares-only stderr, exit 1, sentinel byte-identical. `--json` run: `built:false`, `store.shares.mismatched` decoded — S.4 key set and order, one `ticker` null and one set, raw text, 6-decimal strings. Do NOT pin the Rows line (its `not imported` tail lives until S09)
- [x] Step 2: same file `Test_run_sync_joins_a_share_failure_to_a_balance_failure_in_one_line` — cash account with a mismatched reconciled balance + one mismatched holding; exact stderr `validation failed: 1 of 1 account does not match Quicken's last reconciled balance and 1 of 1 holding does not match Quicken's share count; <store> was not changed; each difference is listed on stdout; fix them in Quicken and run quarry sync, or run quarry sync --from <id> after updating quarry`, exit 1. No stubs needed; both must fail at their assertions

### Build
- [x] Step 3: `internal/store/store.go:456-462` `ShareMismatch` + `internal/importer/validate.go` new `describeShareMismatches` called at `importer.go:159-163` right after `CheckShares` (do not move `CheckShares`, do not change `validate`'s signature) — add account name/currency/closed/active/source id, security name/ticker (`*string`)/source id, `Difference` = saturating `Quarry − Quicken` on the int64s; small account+security maps, not `newRowIndex`; four-tier sort per S.2 with numeric source-id tie-break (`byNameThenSourceID`, `validate.go:138-148`). Tests in `internal/importer/share_gate_test.go` through `Import` + fake `CheckShares` returning real fixture ids: labels joined (closed, inactive), one row per sort tier differing in one variable (account name, account source id, security name, security source id), difference sign negative, saturated operands (MaxInt64 − negative, MinInt64 − positive) clamp instead of wrapping
- [x] Step 4: `internal/cli/render.go:299-312` new `formatShares` beside `formatMoney`, new `securityLabel` beside `accountLabel:315-325`, new `shareMismatchRows` beside `balanceMismatchRows:330-352`, `renderStoreFailure:447-481` DIFFER branch (`xOfYPhrase(n, checked, "holding", "holdings")`, `render.go:229-236`). Tests in `render_internal_test.go`: `formatShares` table (`0.000001`, `-0.000001`, `0`, `10`, `120.5`, `1,200`, MinInt64, MaxInt64); `securityLabel` (nil ticker, ticker == name, ticker differs, control chars escaped in name and ticker); rows padded to widest account label, security label and each figure column (two rows of different widths); `Test_renderStoreFailure:624-744` new case shares differ (`DIFFER for 1 of 1 holding` and plural), plus shares pass line while balances fail stays
- [x] Step 5: `internal/report/document/common.go:68-78` new `Shares(millionths)` beside `Money` (exactly 6 decimals); `internal/cli/json.go:89-94` `sharesDocument` + new `shareMismatchDocument` (S.4 field order) + converter at `json.go:168`, `mismatched` never nil. Tests: `document.Shares` table (`-0.000001`, `0`, `1200.000000`, MinInt64, MaxInt64); `json_internal_test.go:160-178` keeps the empty-array case and gains a populated case read back with `encoding/json` (ticker null vs set, raw name with a control char, difference string)
- [x] Step 6: `internal/snapshot/import.go:237-252` `validationFailedRefusal` + new `shareMismatchClause` beside `splitMismatchClause:268-275` — clause third after balances, splits; shares-only failure uses the S.3 share-specific tail, any cash clause present keeps the existing V1 tail. `sync_and_import_test.go:415-508` gains a want-tail column and rows: shares singular `1 of 145 holdings does not match Quicken's share count`, plural `N of M holdings do not match Quicken's share counts`, `1 of 1 holding`, thousands-grouped, balances+shares, splits+shares, all three in order. These rows run with no store present — that pins the first-run form ("was not changed" unchanged, phase1 V1) with no code change

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments now false: `validationFailedRefusal` ("balance or split-sum"), `renderStoreFailure` ("Shares … left out while a count differs"), `sharesDocument` ("always an empty array"), `ShareMismatch` (ids only)

### Verify
- [ ] Step 8: full verification + `spec-check.py phase4a-investments` → tick SCENARIO-06 and SCENARIO-07 (`— delivered by SCENARIO-06 —` before its test ref); STATE.md: drop line 21 (transitional `validation failed: ; …`, `[]`, omitted Shares line) and the S06/07 Left-unbuilt entry

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Display labels are joined in the importer after `CheckShares`, never in duckstore — the walk (`holdingShares`/`compareShares`) stays the id-only share-count owner 4b reuses on a `ReadDB`
- `ShareMismatch.Difference` = saturating `Quarry − Quicken` on the rounded int64s, not a rounding of the exact difference — the printed columns must subtract to the printed difference (half-even ties flip parity otherwise)
- Mismatch order is fixed in the importer (account name, account source id, security name, security source id); duckstore's id-string order is not display order
- Stderr tail: share-specific only when shares is the sole failing check; any balance/split clause keeps the V1 tail. First run keeps "was not changed"
- `document.Shares` (6 decimals) is the `--json` share formatter; `formatShares` (trimmed, comma-grouped) the text one — 4b reuses both rather than adding a third

**Left unbuilt** — named so nobody assumes it exists:
- Status Shares line / `"shares":{"checked":N}` on status and MCP, dropping the Rows `not imported` tail — SCENARIO-09

**Traps** — things that look right and are not:
- Negating `math.MinInt64` overflows — both share formatters and the difference subtraction must handle the clamped extremes
- Integer division loses the sign of `-0.000001` — carry a negative flag as `formatMoney` does
- Pinning whole stdout in the acceptance test breaks at S09 when the Rows tail goes — pin the Store line and the Shares block only
- Existing V1 tail in code is `fix them in Quicken…`, not phase1 spec's `fix the account…` — pin the code's
- `importer/share_gate_test.go` fake ids `acct-1`/`sec-1` resolve to nothing; zero-value labels must not panic

**Strict-SPLIT seam (if the orchestrator applies the >1-feature-package rule):** (a) Steps 1, 3-5 — labels, text, `--json` (importer + cli); (b) Steps 2, 6 — stderr clause and tail (snapshot), absorbing SCENARIO-07.

## Phase report

Run B2 (steps 5-6) done; Steps 1-6 ticked. Both acceptance tests green; narrow loop green; `golangci-lint run ./...` prints `0 issues`.

Files:
- `internal/report/document/common.go` `Shares(millionths)` (6 decimals; negates quotient and remainder, not the int64) + `Test_Shares` in `common_test.go`.
- `internal/cli/json.go` `sharesDocument.Mismatched []shareMismatchDocument`, `shareMismatchDocument` (S.4 order), `newSharesDocument`; `json_internal_test.go` `Test_renderJSON_reports_each_share_mismatch_with_six_decimal_counts_and_raw_text` (ticker null vs set, control char, negative difference).
- `internal/snapshot/import.go` `validationFailedRefusal` (shares clause third; shares-only gets `shareOnlyTail`, singular "the holding's" at X=1, "those holdings'" at X>1; any cash clause keeps V1 tail), `shareMismatchClause`, `shareOnlyTail`.
- `sync_and_import_test.go` table gained `wantTail` plus 7 share rows; `cmd/quarry/run_share_gate_test.go` acceptance re-pinned to the plural tail.

Mutation (tail predicate `len(clauses) == 0` -> `n > 0`, i.e. "shares failed"): reddened `.../balances_and_shares_joined`, `.../splits_and_shares_joined`, `.../balances,_splits_and_shares_in_order` and `Test_run_sync_joins_a_share_failure_to_a_balance_failure_in_one_line` (expected the V1 `fix them in Quicken…` tail, got the share tail). File restored byte-identical.

For V: sweep owes stale docs on `ShareMismatch` check, `renderStoreFailure` ("Shares … left out while a count differs"), `validationFailedRefusal` now updated. Run full Verify, `spec-check.py`, tick S06 + S07 in specification.md, STATE.md rewrite (drop line 21 and the S06/07 Left-unbuilt entry). The `--json` `store.shares.mismatched` is never nil (`make` of len 0).
