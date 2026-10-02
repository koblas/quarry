# Review round 4: phase2f-fx (re-gate after final product-vision fix pass)

- **Range:** 73611f1..43ad841.
- **Coverage:** 0 uncovered.
- **Mutation sample:** 1/1 killed; the test reviewer ran 3 extra mutations (2 killed, 1 equivalent).
- **Re-run:** correctness-B (PASS, 0 findings), test-B (PASS WITH FOLLOW-UPS).

## Closed
- **Final-pass MAJOR 1:** empty-window zero fill. It is gated on the same predicate as the note, and a control arm is pinned.
- **Final-pass MAJOR 2:** partial-warning copy.
- **Final-pass MAJOR 3:** status Long wrap.
- **Folds:** NativeCurrency tier removed (behaviour-identical); Refresh/refreshRates docs.

## Follow-ups (→ STATE.md Open debts)
- **test-B:**
  - `period.go:63-65` fillSeries doc is 3 lines.
  - `duckstore/rates.go:50-52` refreshRates doc is 3 lines.
  - The new control tests call text and json in one body.
  - `periodKeys`/`spendCurrencies` duplicate `markCurrencies` (NIT).

## Verdict: PASS WITH FOLLOW-UPS
