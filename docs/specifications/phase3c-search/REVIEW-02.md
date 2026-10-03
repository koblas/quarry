# Review 02 — phase3c-search (re-gate after fix pass 1)

## Target
Range `69db5dd..69d8173`. Mutation sample: `0 mutable changed lines since 69db5dd37d15`.

## Triggered reviewers
- test-reviewer: it raised the round-1 MAJORs.
- correctness-reviewer: the fix pass changed the production `AmountError.Error()` and `amountWording` switches.

## Skipped reviewers
- arch-reviewer: no import, placement or wiring change.
- refactor-advisor: its findings are deferred in STATE.md, and the fix claims none closed.

## BLOCKER
None.

## MAJOR
None. The three round-1 MAJORs are closed. test-reviewer confirmed each one, and correctness-reviewer confirmed the ruled lines are byte-identical and that `exhaustive` guards both switches.

## MINOR
- test-reviewer — `internal/report/amount.go:44` and `internal/mcp/search.go:87`: for an out-of-range `AmountErrorKind` built by hand, `Error()` and `amountWording` return `""`. No production constructor builds one (correctness-reviewer checked), and a newly declared kind fails lint. Optional fix: a non-empty `default:` arm with a test row.

## NIT
- test-reviewer — the white-box headers in `internal/cli/render_search_internal_test.go` and `search_internal_test.go` are about 140 characters long.

## Verdict: PASS WITH FOLLOW-UPS
