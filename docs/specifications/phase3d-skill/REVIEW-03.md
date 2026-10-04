# Review 03 — phase3d-skill (re-gate after the final product-vision fix pass)

## Target
Range `5806ee5..c9cdcb4`. This is copy and tests only; there is no production Go.

## Triggered reviewers
correctness-reviewer and test-reviewer, as product-vision directed.

## BLOCKER
None.

## MAJOR
1. **correctness-reviewer — `README.md:23`.** "sends none of your data anywhere" is false.
   - On a first sync, or when a back-dated transaction appears, the Bank of Canada request's `start_date` is the user's earliest transaction date. The chain is `rates.go:27,57,130-146` → `fx/plan.go:30-36` → `fx/valet.go:58-63`.
   - A copy ruling (product-vision, 2026-10-03) set the replacement paragraph. It follows the `quarry sync` help.
2. **test-reviewer — `recurring-and-anomalies.md:21`.** The "400 days (annual)" copy change has no pin.
   - Fix: add a phrase pin for the full ended-threshold clause.

## Rejected
- **test-reviewer:** "no test runs either recipe with `'native'`". This is refuted by `cmd/quarry/run_skill_recipes_test.go:508` (`Test_spending_trend_lists_each_currency_natively_for_a_currency_other_than_cad_or_usd`) and `:529` (the income twin), which fix pass 1 added.

## NIT
- **correctness-reviewer — `spending.md:45` and `cash-flow.md:36`.** The currency rows open "`'CAD'` or `'USD'`." and then name `'native'`. The copy ruling now lists all three values.
- **test-reviewer.** The phrase pins match anywhere in the file, not only in the table row.

## Verdict: BLOCKED
