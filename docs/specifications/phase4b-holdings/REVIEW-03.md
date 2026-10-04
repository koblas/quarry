# Review Report — phase4b-holdings, round 3 (re-gate after final product-vision fix)

### Target
Range `a89bd9f..69b7023`. The fix implemented the final pass's two MAJORs: the MCP cut note now uses `capList` with date-filtered advice, and holdings sorts names ignoring case, like `quarry accounts`.

### Triggered reviewers
- correctness-reviewer. product-vision asked for the re-gate.
- test-reviewer was folded into round 4.

### Gate inputs
- uncovered-diff vs a89bd9f: 0 added lines.
- `mutation-sample: 0 mutable changed lines since a89bd9f`.

### MAJOR
1. correctness-reviewer, `internal/report/document/holdings.go:133-137`. The slot-2 warning sort uses Go `strings.ToLower`, but the table sorts with DuckDB `lower()`. The two disagree on 55 Unicode-16 capitals; for example U+A7CB gives `["Ωa","Ɤa"]` in DuckDB and `["Ɤa","Ωa"]` in Go. This regressed in 69b7023. **Ruled by product-vision:** slot 2 follows the order the accounts were given, as spend does, so there is only one sort engine.
2. correctness-reviewer, `internal/mcp/holdings.go:35-36`. When accounts are named, the cut-note query has no account filter, so it returns other accounts' rows. **Ruled by product-vision:** add ` and account_id in ('<id>', …)`, with ids in the order given and any `'` doubled.

### Checked clean
- NULL names in `ORDER BY` sort NULLS LAST and are deterministic.
- `listCutWarning` revert is byte-identical to main.
- `capList` and the as-of date in the cut note are correct.

### Verdict: BLOCKED
2 MAJOR, both fixed in fix pass 3 (333690d).
