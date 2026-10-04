# Review Report — phase4b-holdings, round 4 (re-gate after fix pass 3)

### Target
- correctness: `69b7023..333690d`
- test: `a89bd9f..333690d`

### Triggered reviewers
- correctness-reviewer: its own round-3 MAJORs
- test-reviewer: pins from both final-pass fixes

### Gate inputs
- uncovered-diff vs 69b7023: 0 added lines
- test-stats: mcp +1

### Round-3 findings
- MAJOR slot-2 sort engine mismatch: closed. Slot 2 now uses the given order and the Go sort is deleted.
- MAJOR unscoped cut-note query: closed. `accountsPredicate` was verified on DuckDB for valid SQL, quote doubling and injection attempts.

### MINOR
- correctness, `internal/mcp/holdings.go:46`: `accountsPredicate` guards `== nil`, not `len == 0`. An empty non-nil slice would render `in ()`. Unreachable today because report returns nil when no accounts are named.
- test, `internal/report/document/holdings_account_test.go:35-48`: the slot-2 rows give ids in the same order as the given order, so a re-sort by ID survives. Add a row with ids descending against the given order.
- test, `internal/mcp/holdings_test.go:60,82-83,277`: the cut-note date is pinned only with a full-date `as_of`. Add `as_of: "2026-03"` (expect `'2026-03-31'`) or an omitted `as_of`.
- test, `internal/store/duckstore/holdings_test.go:172-200`: the case-insensitive sort rows are ASCII only (optional).
- test, `cmd/quarry/run_holdings_sort_test.go:15-22`: the seed helper sets the name the assertion depends on (follows `seedHoldingsStore` precedent).

### NIT
- The JSON sort test overlaps the table sort test.
- The single-account cut note still says "pass fewer accounts" (ruled copy).

### Verdict: PASS WITH FOLLOW-UPS
Fix-pass cap (3) reached; MINORs go to STATE.md.
