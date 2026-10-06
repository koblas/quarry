# Review Report — round 03 (re-gate after fix pass 3, cap reached)

### Target
Range 80396c3..b6b736b (fix pass 3 for REVIEW-02). Full suite green, 0 uncovered added lines, lint 0.

### Triggered reviewers
- correctness-reviewer: PASS WITH FOLLOW-UPS — prior MAJOR (split moves held() across 0) CLOSED; probe shapes 1-3 pinned and green.
- test-reviewer: prior MAJOR (span-open / cover pro-rate guards) CLOSED; MINORs closed or accepted.

### MAJOR (test-reviewer — missing pins, production correct on every probe; NOT eligible for a 4th fix round: none introduced a behaviour change to an exit code or written file → STATE.md Open debts)
- `internal/report/acb_walk.go:300` settle's acquisition arm unpinned (mutant `== acbDisposition` survives). Pin: buy 1µ for 100, sell 5µ, split 1:3, buy 1µ, split 3:1 → Shares "0", Incomplete false, no Oversold on last event.
- `acb_walk.go:241` `wasShort` raw-Sign mutant survives. Pin: buy 1µ, sell 2µ, split 1:3, buy 10 for 1,000,000,000 cents → ACB 1,000,000,000.
- `acb_walk.go:322` `adjust` raw-Sign mutant survives. Pin: buy 1µ for 100, split 1:3, ROC 50 → not-held issue, ACB 100.
- `acb_walk.go:160` `Incomplete` raw-Sign mutant survives. Pin: short −1µ then split 1:3 → Incomplete false.

### MINOR
- correctness: STATE.md "Short pool" line still says "shares <= 0 ⇒ ACB 0"; qualify it with HELD INVARIANT (3).
- test: no-cost span closed by a split is not reopened by a restoring split (1-millionth input) — unruled residual, already in Open debts; final product-vision pass may rule.
- test: `acbPool` doc comment 4 lines (budget 1-2).

### NIT
- correctness: `acbPool` doc says shares stay exact; settle() discards sub-half-millionth remainders.
- test: `acbSplitThenSell` lost its doc line; `run_config_test.go:386` still lists `{"accounts","acb"}` by hand.

### Verdict: BLOCKED by contract, proceeds by cap
Fix passes capped at 3 (CLAUDE.md): every finding above goes to STATE.md `## Open debts`; the feature proceeds to the final product-vision pass.
