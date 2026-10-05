# Review Report — gate round 3 (re-gate after fix pass 2)

### Target
`95a779d..90770b0`: the 5 copy changes from REVIEW-03 (final product-vision SHIP WITH CHANGES).

### Triggered reviewers
- correctness-reviewer: production logic changed (`NativeAdvice` threading).
- test-reviewer: re-pinned copy and the new MCP pins.

### Gate data
- `go test` rc=0; `uncovered-diff.py` 0 since 95a779d; lint 0; `spec-check --run` OK.
- test-stats TOTAL 2777 (+3).
- mutation-sample: 0 mutable changed lines.

### MAJOR
- **test** `plugin/skills/quarry/references/findings.md:17`: the ruled bullet `` `duplicate` and `unlinked-transfer` compare register entries only, not buys, sells, dividends or other investment transactions. `` has no pin. `Test_references_carry_…` (`cmd/quarry/run_skill_references_test.go:79-81`) reads findings.md only through the type bullets and four phrases, so deleting the line keeps every test green. Fix: add a distinctive span of the sentence to that test's findings.md phrase list.

### MINOR
- **test** `specification.md` Surface & Copy does not record the findings.md bullet; only the findings Long sentence is listed. Fold: add it.

### NIT
- **test** `internal/report/document/networth_rate_warnings_test.go`: test name `..._advise_the_surface_s_own_way_...` has a stray `s_`.

### Prior findings
- correctness: PASS. `NativeAdvice` is passed correctly by both callers, the warning order is unchanged, and the COMMENT SQL is safe.
- test: REVIEW-03 items 1-4 are pinned verbatim; the EUR-account test rename is fine.

### Verdict: BLOCKED
