---
id: SCENARIO-12
status: open
---

# SCENARIO-12: Possible superficial losses are marked

Cadence: code-first — no bug fix, write-safety guard or atomic adapter touched
Acceptance test: `cmd/quarry/run_acb_superficial_test.go` `Test_run_acb_marks_a_loss_sale_rebought_in_a_registered_account_within_30_days`
Narrow loop: `go test ./internal/report/ ./internal/report/document/ ./internal/cli/ ./cmd/quarry/ -run '(?i)superficial|ACBWarnings|renderACB|run_acb'`
Mutation checks: window bound `+30` made exclusive → `Test_acb_marks_a_loss_sale_with_an_acquisition_30_days_after`; bound `−30` made exclusive → `Test_acb_marks_a_loss_sale_with_an_acquisition_30_days_before`; held-at-+30 guard deleted → `Test_acb_does_not_mark_a_loss_sale_when_nothing_is_held_30_days_after`
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (report; document/cli surfaces only)

**Before dispatching B1 — two rulings block it** (orchestrator obtains them; defaults below are what the developer builds if accepted as-is):
- **Copy ruling (product-vision, scoped):** warning 3's `<years>` is unruled. Default: ONE line for the whole report, N = every marked sale, years distinct ascending joined `", "` (`3 possible superficial losses in 2023, 2024: …`); `N possible superficial loss(es)` via `humanize.Count`.
- **Rule ruling — "other than the shares sold":** fixture: RRSP holds the security for years; non-registered holds 0, buys 10 on day −5, sells those 10 at a loss on day 0; nothing re-bought. Per-row rule (any acquisition row of the group in the window + something held at +30) MARKS it; a lot-tracing reading does not. Recommend per-row: CRA treats identical shares as one property, and its denial formula min(acquired, sold, held at +30)/sold denies all of it. Control (not marked under either reading): empty pool, buy 10 and sell 10 same day at a loss, nothing held anywhere at +30.

**Unruled edges — default, pending ruling (architect recommendation):**
1. Window not closed (sale day +30 after Today): rows dated after Today ignored, as the walk does (`acb_walk.go:56`); "held" measured at end of min(day+30, Today). Mark stands as "possible".
2. "Held at +30" across accounts: per holding (account, security in the group), signed stored shares summed, each account's own split row scales that account's count, rounded to millionths (`report.Millionths`) at end of the day; held = SOME holding > 0. A negative holding never cancels a positive one.
3. RD adjustment (config): NOT an acquisition (no units; spec's list is buy, reinvested dividend, added shares).
4. Same-day re-buy: counts (day 0 is inside the window); a sell or remove_shares is never an acquisition.
5. Break-even: not marked (`Gain < 0` strictly).
6. Same ticker: exact, case-sensitive equality of a non-nil, non-empty `Security.Ticker`; nil/empty ticker groups only with its own id. Binding for S14's warning 7.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_acb_superficial_test.go` `Test_run_acb_marks_a_loss_sale_rebought_in_a_registered_account_within_30_days` — config `[accounts]` registered `acct-rrsp`, non-registered `acct-cad`; buy in acct-cad, loss sale, re-buy in acct-rrsp within 30 days, held (`acbTrade`, `replaceStore`, `runWith` + `holdingsClock`, pattern `run_acb_adjustments_test.go:16-60`); sale and its day +30 well before `holdingsClock()` 2026-03-12, so the window is closed (edge 1 not in play). Text run: year row ends `1 possible superficial loss`, Gain column = unadjusted loss, stderr warning 3 verbatim. `--json` run: sale `possible_superficial_loss` true, `possible_superficial_losses` 1, `gain` unchanged, `warnings[]` carries warning 3. No stubs needed (`ACBSale.PossibleSuperficialLoss` `acb.go:84` and its JSON count `document/acb.go:136-139` exist); red at the suffix assertion.

### Build
- [ ] Step 2: new `internal/report/acb_superficial.go` (marking func), called in `walkACB` `acb_walk.go:78-89` on the `[]acbSale` BEFORE `acbYears` — acquisition window over the unfiltered `history.Transactions` (every account, rows ≤ Today), group = same id or same ticker (edge 6), loss only. Tests `internal/report/acb_superficial_test.go` through `Server.ACB` (`acbWalkRequest` `acb_walk_test.go:33-46`, holding kept in acct-9 so held is true): registered re-buy marked; non-registered re-buy in the same pool account and in another pool account marked; `Test_acb_marks_a_loss_sale_with_an_acquisition_30_days_before` / `…_30_days_after` plus −31 / +31 rows; reinvest and add_shares count; zero-unit add_shares does not; sell in another account does not; RD adjustment does not (edge 3); gain sale with re-buy not; break-even not; same ticker marks, different ticker not, nil-ticker pair not; acquisition after Today ignored; same-day re-buy (edge 4); the "other than the shares sold" fixture and its control (ruling above); ROC excess never marked
- [ ] Step 3: same file — held at end of day +30 (edge 2, edge 1). Rows: all sold by +30 → not marked; group's last units sold ON +30 → not held; sold on +31 → held; held only in the registered account; held only via a same-ticker sibling; split in the holding account scales its count, split recorded in two accounts not doubled; negative holding in one account beside a positive one; window open with +30 = Today vs +30 = Today+1
- [ ] Step 4: ONE count site: new method `(ACBYear).PossibleSuperficialLosses() int` in `acb.go` (test in `acb_superficial_test.go`); repoint `document/acb.go:138-139` at it; `internal/cli/render_acb.go:41-48` `acbYearSuffixes` — count suffix FIRST via `humanize.Count` of that method, before the ROC suffix; `render_acb_internal_test.go` rows: 1 vs 2 (singular/plural), year with both suffixes in order, 0 → no suffix. `internal/report/document/acb_warnings.go:15-20` `ACBWarnings` — warning 3 (N and years summed from that method) after `adjustmentWarnings`, before `removalWarnings` (per the copy ruling); `acb_warnings_test.go` rows: one sale, two sales over two years, none → silent; extend the order test `:147-178` to adjustment, warning 3, removal, ROC. Edge × arm: RRSP re-buy row → text + JSON in Step 1; not-held, gain, break-even and bound rows → `Server.ACB` only, text/JSON n/a (both read the flag through the one count method); `--year`/`--security` n/a (unregistered until S15/S16)

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comment on the marking func; `ACBSale` doc `acb.go:75-76` names what marks it; `ACBWarnings` doc `acb_warnings.go:12-14` lists warning 3's slot. No golden re-pins expected: S08b/S10/S11 cmd fixtures and `acb_events_test.go:114-121` have no loss sale with an acquisition inside ±30 days — any re-pin is a finding to report, not to absorb

### Verify
- [ ] Step 6: full verification + `spec-check.py phase4de-acb` → tick SCENARIO-12 with its acceptance test; STATE.md rewrite

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- Detection reads `InvestmentHistory.Transactions` file-wide (registered, closed, every account), never the pool map `bySecurity` — the CRA rule counts own-RRSP/TFSA re-buys; S21's mark count depends on it
- `report` keeps its own per-holding share count for "held at +30", mirroring `duckstore.holdingSpans` (`shares.go:100-147`) semantics (signed shares, per-account split, millionths at end of day) — a deliberate second derivation: `report` cannot import duckstore, and adding `holding_shares` to `InvestmentHistory` costs a duckstore batch plus fault tests. Revisit only if S21 finds a divergence
- Same-ticker rule (edge 6, as ruled) is the ONE grouping rule; S14's warning 7 reuses it
- Flag set on `acbSale.row` before `acbYears`; `(ACBYear).PossibleSuperficialLosses()` is the ONE count (JSON, text suffix, warning 3; S15's `--year` and S19 reuse it); year suffix first in `acbYearSuffixes`; warning 3 slot between adjustment lines and slot 5 in `ACBWarnings`; S13a's warning 4 goes after warning 3, before slot 5

**Left unbuilt** — named so nobody assumes it exists:
- `--year` per-row suffix `possible superficial loss` — S15; MCP `acb` — S19
- Whether a no-rate USD sale (CAD 0 INTERIM, a spurious "loss") is marked — S14 decides when it cuts that security from totals

**Traps** — things that look right and are not:
- `history.Transactions` is unfiltered: repeat the walk's `Date.After(req.Today)` cut (`acb_walk.go:56`) or future-dated Quicken rows create marks
- `acbYears` copies sales by value into `Year.Sales` (`acb_walk.go:432`): a flag set after it is lost
- Find sales by `Action`/the `acbSale` list, never `Realized` — an ROC excess is `Realized` too
- `acbSecurity` test helper (`acb_test.go:35-37`) sets Ticker = name: two report-test securities with one name now cross-match
- The walk's split rule (once per day, pool rows) is NOT the holding rule (each account's own row scales that account)

## Phase report

**Orchestrator rulings 2026-10-05 (SCENARIO-12 plan defaults): warning 3 is one line for the report, N = every marked sale, `<years>` distinct ascending ", "-joined; "other than the shares sold" is per-row (any qualifying acquisition row in the window counts, even one whose units were the ones sold — over-flagging a *possible* loss is the safe side; CRA min(acquired, sold, held)/sold denies the whole loss); window still open (day +30 after today): rows after today ignored, held measured at the earlier of day +30 and today; held = some (account, security) holding > 0 at that point, signed shares, each account's own split rows, millionths, a negative holding never cancels a positive one; a reinvested-distribution adjustment is not an acquisition; a same-day re-buy counts, a sell/remove never does; break-even (gain 0) not marked; same ticker = exact case-sensitive match on a non-empty ticker (binding for S14 warning 7); report keeps its own per-holding count mirroring duckstore.holdingSpans (no store change; S21 cross-checks it). Recorded in spec Part B CRA rules; all plan defaults stand.**

**Run A (acceptance, red) — done.** Added `cmd/quarry/run_acb_superficial_test.go` (fixture `acbSuperficialRows`, const `superficialLossWarning`, config `acbSuperficialConfig`); no production code, no stubs. Fixture: acct-cad (non-registered) buys 20 Acme 2025-01-10 for 2,000.00, sells 10 on 2025-03-01 for 600.00 (gain -400.00, ACB removed 1,000.00); acct-rrsp (registered) buys 5 on 2025-03-15; clock 2026-03-12 so the window is closed.
- Red at assertions, expected reason (nothing marks yet), run `go test ./cmd/quarry/ -run Test_run_acb_marks_a_loss_sale`: text year row lacks suffix (`-400.00` ends the line, want `  1 possible superficial loss`); stderr `""` want `quarry: warning: 1 possible superficial loss in 2025: ...`; JSON `possible_superficial_losses` 0 want 1; sale `possible_superficial_loss` false want true; `warnings` `[]` want warning 3. Table widths, Gain `-400.00`, position table, JSON gain all already match (they pass).
- One test, two runs (text, `--json`) per plan; both assert warning 3 on stderr and in `warnings[]`. `golangci-lint run ./cmd/quarry/`: 0 issues.
- B1 must not redo: the fixture/pins above. Warning text is `N possible superficial loss(es) in <years>: ...` (ruling: one line, years distinct ascending). Year suffix goes in `acbYearSuffixes` first, before ROC.

