# Review Report — round 2 (re-gate after fix pass 1)

### Target
8676a1a4897a89736c7a1c8f780c07592d9415be..b4384206 (fix pass 1).

### Triggered reviewers
- correctness-reviewer: blocked round 1
- test-reviewer: fix changed tests
- refactor-advisor: had findings the fix claims to close

### Skipped reviewers
- arch-reviewer: fix changed no imports, package placement or wiring
- api-reviewer, pipeline-reviewer: no matching files

### Gate inputs
- uncovered-diff: 0 uncovered added lines
- test-stats: cmd/quarry 981 (+1), internal/importer 233 (+2), internal/snapshot 348 (+2), TOTAL (+5)
- mutation-sample: 7 sampled of 7 — 6 killed, 1 survived (equivalent: `snapshot/resolve.go:17` keepName ok), 0 non-viable, 0 timed out; 563s

### Round-1 findings
All closed: BLOCKER duplicated looked-up entity (importer refusal, ruled copy); resolve.go column dedup; splits.go `ok` fallback; snapshot identity default; resolvedEntities struct; schemaNumbered list; renumberSchema compose; tb.Context; shiftFrom doc; slices.Sorted; control assertion.

### BLOCKER
none

### MAJOR
none — correctness raised one (duplicated entity on a renumbered file refuses at the schema stage with mismatch copy); product-vision mid-feature ruling chose option A (scoping BR-R3 outcome stands, spec Surface & Copy updated), downgraded to MINOR, recorded in STATE.md Open debts.

### MINOR
- correctness-reviewer: duplicated entity on renumbered file → schema-stage mismatch copy (ruled, deferred, STATE.md).
- correctness-reviewer: `WithEntityResolver(nil)` panics on first use (no caller passes nil; STATE.md).

### NIT
- refactor-advisor: `keepName` defined in both packages (covered by EntityResolver duplication decision); `joinNames` case-0 comment accurate; nil-option note (same as MINOR).
- test-reviewer: STATE.md records only importer keepName as equivalent — added snapshot one.

### Verdict: PASS WITH FOLLOW-UPS
