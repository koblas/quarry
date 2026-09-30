# Review Report — REVIEW-07

### Target
Final product-vision SHIP WITH CHANGES fix pass, range `a9b46d7..550f0d0` (C3 key quoting, UTF-8 BOM, sole discovered bundle names `quicken.path`), plus the orchestrator's follow-up test rows.

Gate on `550f0d0`: `go test rc=0` (covered full suite); `uncovered-diff: 0 uncovered added line(s) in 0 run(s) since a9b46d7a32e7`; `golangci-lint run ./...` 0 issues, rc=0; mutation sample 4 sampled of 4 — 4 killed.

### Triggered reviewers
- test-reviewer (new pins); product-vision narrow re-check (SHIP).
- correctness-reviewer not re-run: the key-quoting helper escapes exactly the ruled set (`\"`, `\\`, `\n`, `\t`, `\uXXXX` for `unicode.IsControl`).

### MAJOR (both closed in this round)
1. test-reviewer — `Test_load_names_a_table_once_when_its_children_need_quotes` pins `unknown key foo` for `[foo]` then `"x.y" = 1`, contradicting the ruling's example. Resolved: product-vision confirmed its example was wrong (a `[foo]` header warns once as a table; children are not listed); spec corrected in `58ff3a2` to `foo."x.y" = 1` → `unknown key foo."x.y"`.
2. test-reviewer — `internal/config/parse.go` control-character arm: mutants `unicode.IsControl(r) && r < 0x20` and `%04X` → `%04x` survived. Resolved by the orchestrator: rows `"a\u007Fb"` (DEL, upper-case hex) and `"a\u0085b"` (C1) added to `Test_load_names_an_unknown_key_as_toml_would_write_it`; both mutants now redden it; `parse.go` restored byte-identical.

### NIT
- U+2028/U+2029 (not `unicode.IsControl`) print raw in an unknown key; the ruling names control characters only. Left.

### Verdict: PASS WITH FOLLOW-UPS
