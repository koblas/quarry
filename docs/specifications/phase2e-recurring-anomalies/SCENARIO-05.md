---
id: SCENARIO-05
status: done
---

# SCENARIO-05: a series ends after its cadence's quiet period

Cadence: code-first — read-only derivation and a text cell; no bug fix, write-safety guard or atomic adapter
Acceptance test: `cmd/quarry/run_recurring_state_test.go` `Test_run_recurring_marks_a_series_ended_one_day_past_its_cadences_quiet_period`
Acceptance test (SCENARIO-06, folded): `cmd/quarry/run_recurring_state_test.go` `Test_run_recurring_marks_a_series_first_charged_in_the_window_as_new`
Narrow loop: `go test ./internal/report/ ./internal/cli/ ./cmd/quarry/ -run '(?i)recurring'`
Mutation checks: `> rule.endedAfter` in `seriesOf` (`>=`) → `Test_run_recurring_keeps_a_series_active_on_the_last_day_of_its_cadences_quiet_period` (all four rows); each `*EndedAfterDays` const off by one → its cadence's active and ended rows; `!First.Before(Since)` (`After`, or `Before` dropped) → `Test_recurring_marks_a_series_new_only_when_its_first_charge_is_on_or_after_the_window_start`; `, new` suffix dropped → the folded e2e test and `Test_renderRecurring_adds_new_to_the_status_of_a_new_series`
Runs: L | V
Size: LIGHT — 3 steps, report + cli

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_recurring_state_test.go` (new): bound outline as two tables (active at `ended-after` days: weekly 14, monthly 45, quarterly 120, annual 400; ended one day past: 15, 46, 121, 401 with no Total row), each case differing from its twin by the one day; and folded 06 (series started in March + one started the December before, default window, `active, new` vs `active`). Ended-after bounds are green on arrival (S01 built them; pinned only at one far control) — mutation checks prove the pins; folded 06 is the red (no `, new` yet). `--since 2000` makes every series new, so the bound rows and the S01 `--since 2000` tests read `active, new` / `ended, new`

### Build
- [x] Step 2: `internal/report/recurring.go` `Series.New`, set in `(*Server).Recurring` after the listing filter as `!First.Before(Window.Since)` (a listed series already starts by `Until`, so no upper check); `internal/report/recurring_new_test.go` (new): first on `since` (new), first one day before `since` (not new, series active so still listed), first on `until` (new, listed)
- [x] Step 3: `internal/cli/render_recurring.go` Status cell = state word plus `, new`; `render_recurring_internal_test.go` pins `active, new` and `ended, new` (an ended series can be new) with no trailing space in either row

### Sweep and Verify (run V)
- [x] Step 4: lint 0 issues, full covered suite, `uncovered-diff.py`, `test-stats.py`; tick SCENARIO-05 and 06 ("delivered by SCENARIO-05") in `specification.md`, `spec-check.py`, rewrite `STATE.md`, `status: done`

## Handoff

- `Series.New` is `!First.Before(Window.Since)` and relies on `runsDuring` having dropped series starting after `Until`; S07's `new` JSON field and its ended/new pin read it.
- The local-date pin for the ended bound is `Test_recurring_counts_days_since_the_last_charge_from_the_local_date` (monthly 45 under `windowNow`).

## Phase report

Run L done (steps 1-3); V (step 4) left.

**Files**
- `cmd/quarry/run_recurring_state_test.go` (new): active-at-bound table (weekly 14, monthly 45, quarterly 120, annual 400), ended-past-bound table (15, 46, 121, 401), folded 06 e2e. `cmd/quarry/run_recurring_test.go:81-135`: the two `--since 2000` tests now expect `active, new` (every series is new when the window starts in 2000).
- `internal/report/recurring.go`: `Series.New`, set after the `runsDuring` filter. `internal/report/recurring_new_test.go` (new): first on since / day before since / on until.
- `internal/cli/render_recurring.go`: `statusCell`, `recurringNewSuffix`. `render_recurring_internal_test.go`: `active, new` and `ended, new`.

**State**: narrow loop `-run recurring` green in report, cli, cmd/quarry. Acceptance red seen: folded 06 failed at its table assertion (`active` where `active, new` expected); bound tables green on arrival (built in S01).
**Mutations** (all reverted, each reddened only its own subtests): `>=` in `seriesOf` -> four active-at-bound rows; each `*EndedAfterDays` +1/-1 -> that cadence's ended / active row only; `First.After(Since)`, `First.Before(Since)` negation, `Since -1 day`, `Since +1 day` -> the new table's matching rows; `if s.New` -> `if false` -> folded e2e and the render pins.
**Next run**: V only (sweep, covered full suite, uncovered-diff, ticks 05 and 06, spec-check, STATE.md, `status: done`). Do not redo the pins. Steps 1-3 ticked.
