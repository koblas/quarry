# Review round 3: phase2f-fx (re-gate after fix pass 2)

- **Range:** c4468c9..be5989e.
- **Coverage:** 0 uncovered, plus 1 declared unreachable (`cmd/quarry/main.go`), which the reviewers accepted.
- **Mutation sample:** 2/2 killed.
- **Re-run:** correctness-A, test-A.

## Closed
- **MAJOR, head-span gap.** Built to the invariant "asked ∪ Have = hull(Need ∪ Have)". The exhaustive grid test reddens 140+ cases under either the old head bug or the old tail bug, and the duckstore two-sync property reddens one specific case for each.
  - Partials were checked with 5,856 scratch runs, of which 2,058 had a FetchError. There was no gap, and a positive control on the old rule reported 378 violations.
- **MAJOR, shipped wiring unpinned.** Closed by `Test_the_shipped_sync_fetches_exchange_rates_from_the_valet_series`. Removing the line reddens it.
- **MINORs:** seen-map keyed by date, main_test header, single unreachable comment.

## Follow-ups (MINOR/NIT → STATE.md Open debts)
- **correctness-A** — `internal/fx/fx.go:48` Refresh doc and `duckstore/rates.go:50-51` refreshRates doc still say "dates of Need that Have does not cover". They now also ask bridge days. Doc-only.
- **correctness-A** — no repo test pins partial-failure adjacency. A scratch check passed; test-reviewer to route it.
- **test-A** — `run_sync_wiring_test.go:42` asserts FXUSDCAD only (NIT).
- **test-A** — `refresh_grid_test.go` empty-need test has an inline `DeleteFunc` predicate (NIT).

## Verdict: PASS WITH FOLLOW-UPS
