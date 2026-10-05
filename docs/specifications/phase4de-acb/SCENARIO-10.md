---
id: SCENARIO-10
status: open
---

# SCENARIO-10: Shares added or removed without a trade

Cadence: code-first
Acceptance test: `cmd/quarry/run_acb_shares_test.go` `Test_run_acb_takes_removed_shares_out_of_the_acb_and_adds_added_shares_at_their_cost`
Narrow loop: `go test ./internal/report/... ./internal/platform/humanize/ -run '(?i)acb|shares' && go test ./internal/cli/ ./cmd/quarry/ -run '(?i)acb|formatShares'`
Mutation checks: abs on remove (`Abs` of a remove_shares row's stored shares); add_shares cost (the `CostBasis` added for an add_shares row); warning wiring (the `document.ACBWarnings` call in the acb RunE)
Runs: L | V
Size: LIGHT — 3 steps, report (+ document, cli wiring)

Contract: add_shares adds its units plus `cost_basis` converted (NULL: units at 0.00; the `incomplete` mark, warning 4 and the finding are 13a/13b). Every remove_shares (stored negative; units = magnitude) removes pro-rata ACB, makes no sale and no gain, and raises warning 5. Both sort with `acbTiers`: add_shares 0 (acquisition), remove_shares 2 (disposition). Registered and unclassified accounts stay out of the pool, so no warning from them.

Surface & Copy delivered: warning 5 (verbatim, spec Part B warnings list), asserted on stderr in the acceptance test and in `warnings[]` by `Test_run_acb_lists_a_removal_warning_in_json`. `<shares>` is `humanize.Shares` (thousands-grouped, trailing zeros trimmed), `<date>` is `2006-01-02`, names quoted `"…"` verbatim.

Interim: warning 5 is the only warning `document.ACBWarnings` composes; slots 2-4 and 6-9 slot in around it. It is built from the full report, one per remove_shares event, in securities then event order.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_acb_shares_test.go` `Test_run_acb_takes_removed_shares_out_of_the_acb_and_adds_added_shares_at_their_cost` through `runWith`: one non-registered account, buy 10 for 1,000.00, add_shares 5 with cost_basis 600.00, add_shares 5 with no cost, remove_shares -4 (stored negative). Expect position 16 shares, ACB 1,280.00, 80.0000 per share, no sale rows, stderr warning 5 verbatim.

### Build (code-first, 3 batches)
- [x] Step 2: `internal/report/acb_walk.go:21-26,124-144` — `acbTiers` gains add_shares 0, remove_shares 2; add_shares joins the reinvest arm (cost from `CostBasis`, NULL 0); remove_shares arm takes the shares' `Abs` on the event, removes pro-rata ACB through a `take` method shared with `sell` (`acb_walk.go:168-179`), no sale, no `Realized`. Tests in `internal/report/acb_arms_test.go`: add with cost, add without cost, remove pro-rata, remove after oversell (whole ACB, pool 0), same-day order (add, then remove, with a sale), registered account's remove skipped; replaces `Test_acb_leaves_the_pool_alone_for_actions_it_does_not_walk` rows for add/remove.
- [x] Step 3: `internal/report/document/acb_warnings.go` `ACBWarnings(report.ACB) []string` (warning 5, one per remove_shares event); `humanize.Shares` hoisted from `internal/cli/render.go:317` (callers `render.go`, `render_acb.go`, `render_holdings.go`; test moves to `internal/platform/humanize`). Tests: `acb_warnings_test.go` (shape, two removals in order, none when no removal), `humanize` test.
- [x] Step 4: `internal/cli/acb.go` — `warnings := document.ACBWarnings(acb)`; `emitReport(cmd, *jsonOut, warnings, renderACBJSON(acb, withConfigWarnings(cfg.WarningsAbsolute, warnings)), …)`. Tests: acceptance (stderr), `Test_run_acb_lists_a_removal_warning_in_json` in `cmd/quarry/run_acb_shares_test.go`.

## Handoff
- `document.ACBWarnings` is the one composer for acb; later warning scenarios add their slot in ruled order around warning 5 and keep it one function.
- `ACBEvent` for add_shares/remove_shares carries `Amount` = the row's own amount (precedent: reinvest), `Shares` positive magnitude for remove_shares.

## Phase report
Run L done; V next.
- Red (Acceptance): `Test_run_acb_takes_removed_shares_out_of_the_acb_and_adds_added_shares_at_their_cost` failed at its assertions (position `10 / 1,000.00 / 100.0000` for `16 / 1,280.00 / 80.0000`; stderr `""` for warning 5). Now green with `Test_run_acb_lists_a_removal_warning_in_json`.
- Changed: `internal/report/acb_walk.go:21-30` (tiers), `:124-155` (add_shares joins reinvest arm; remove_shares arm, `Abs` + `pool.take`), `:172-190` (`take` extracted from `sell`); `internal/report/document/acb_warnings.go` (`ACBWarnings`); `internal/cli/acb.go:57-64` (warnings to stderr and `--json`); `humanize.Shares` hoisted from `cli.formatShares` (`internal/platform/humanize/humanize.go`, its test moved to `humanize_test.go`, callers `render.go`, `render_acb.go`, `render_holdings.go`).
- Tests: `internal/report/acb_shares_test.go` (5), `internal/report/document/acb_warnings_test.go` (3), `cmd/quarry/run_acb_shares_test.go` (2); `Test_acb_leaves_the_pool_alone_for_actions_it_does_not_walk` lost its add/remove rows.
- Mutations (each reverted, `diff` clean): drop `Abs` on remove -> `Test_acb_removes_a_removals_share_of_the_acb_with_no_sale_and_no_gain` (`expected "6" / actual "14"`); add_shares cost -> `0` -> `Test_acb_adds_added_shares_at_their_cost_or_at_none/with_a_cost_adds_the_units_and_the_cost` and the acceptance test (`1,280.00` vs `800.00`); `ACBWarnings(acb)[:0]` -> acceptance (stderr `""`) and `Test_run_acb_lists_a_removal_warning_in_json` (`warnings []`).
- `golangci-lint run ./...`: 0 issues. Full covered suite, `uncovered-diff.py`, `test-stats.py`, spec tick, STATE.md rewrite NOT done (run V).
- V must not undo: warning 5 text; the `humanize.Shares` hoist (STATE trap: shares text follows the holdings precedent, now one helper).
