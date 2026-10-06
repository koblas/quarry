# Review Report — round 02 (re-gate)

### Target
Range 3a9e459..f1cd6e4 (fix passes 1 and 2 for REVIEW-01). Full suite green, 0 uncovered added lines, lint 0.

### Triggered reviewers
- correctness-reviewer (blocked round 01; production logic changed)
- test-reviewer (blocked round 01; tests changed)

### Skipped reviewers
- arch-reviewer, refactor-advisor: PASS WITH FOLLOW-UPS in round 01; fixes touched nothing they own beyond folds already listed.

### Prior findings
- correctness MAJOR acb_walk.go:255 (split leftover): closed for dispositions; reopened for splits (below).
- test MAJOR acb_cap_test.go:69 (`events: []`): CLOSED. Cross-scenario MINORs: CLOSED.

### MAJOR
- correctness-reviewer: `internal/report/acb_walk.go:254-260` — the split arm can move `held()` across 0 with no disposition (exact dust kept). Probes: (1) buy 1,000 / split 1:7 / sell 142.857143 / split 7:1 → short −0.000001, incomplete true, no warning (no ruled copy); (2) buy 0.000001 for 1.00 / split 1:3 → shares 0, ACB 1.00 hidden; (3) no-cost add 0.000001 / split 1:3 → flat pool incomplete. Fix: orchestrator ruling appended to REVIEW-01 ("after fix pass 2"): zero exact shares at any flat point after an acquisition or disposition; span closes whenever `held() <= 0` after any pool change incl. splits; a split rounding a held pool to 0 keeps its ACB.
- test-reviewer: `internal/report/acb_walk.go:282` (no-cost span open `held() > 0`) and `:411` (`add` cover pro-rate `case p.held() > 0`) survive mutation to raw `Sign() > 0`. Fix: rows on `acbSplitThenSell(t, 100*acbMillion, acbMillion, 3*acbMillion, 33_333_334)` + one more transaction: (a) a 1µ costed buy → position `{Shares: "0", ACB: 0}`; (b) a 1µ `acbNoCostAdd` → Incomplete false, add event UnknownCost true. Each red under its mutant.

### MINOR
- test-reviewer: `acb_short_test.go:~335` `Test_acb_position_holds_nothing_below_half_a_millionth_of_a_share` feeds `Shares: 1/3_000_000`, which the walk never emits — keep only if `Holds` is a deliberate contract (say so in the name), else drop.
- test-reviewer: `cmd/quarry/run_config_test.go:350-352` hand-copied command list — derive from `readCommandArgs()` minus a named `currencyIgnored` set, or add a length guard.

### NIT
- test-reviewer: `acbSplitThenSell` four positional int64 params — use a struct or named constants.

### Verdict: BLOCKED
Two MAJORs. Fix pass 3 (last allowed for this surface) builds to the invariant ruled in REVIEW-01 "after fix pass 2".
