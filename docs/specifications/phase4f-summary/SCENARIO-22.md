---
id: SCENARIO-22
status: done
---

# SCENARIO-22: A subscription never listed before is new when it first lists

Cadence: test-first (bug fix, ruling U18: `resumes` treats an earlier run `quarry recurring` could never list as a steady series that had not ended)
Acceptance test: `internal/report/summary_recurring_test.go` `Test_summary_lists_a_subscription_as_new_when_its_earlier_series_was_never_listable`
Narrow loop: `go test ./internal/report/ -run 'summary|Summary|recurring'`
Mutation checks: (a) drop the new condition from `resumes` (back to U17) → acceptance + `same-day duplicate on the earlier run's last day` + `duplicate is the run's first charge`; (b) take the prefix from `before` (`group[:len-len(run)]`) instead of the group by date → acceptance and `duplicate is the run's first charge` (the duplicate is `run[0]`, outside `before`; the start-40 row stays green, so (b) is not redundant with (a)); (c) prefix through the earlier last charge's MONTH end, not its day → existing `one stray` and `two strays` rows (strays fall in the same month as the earlier last, 2026-01); (d) prefix strictly BEFORE the earlier last charge's day → control row `no duplicate, run starts 40 days after` and every `Test_summary_resumption_arms` hidden row; (e) refuse any zero gap anywhere in the earlier run → `duplicate earlier in the earlier run` row
Runs: A+B1 (1-2) | V (3-4)
Size: OWNS A RUN — 1 Build batch, 1 feature package (`internal/report`; summary-only, `recurring.go` and `latestRun` untouched)

## Implementation Plan

Premise check: `earlierRun`/`resumes` exist (`summary_recurring.go:26-41`); `resumes` is called only by `newInMonth` (`:12`). Acceptance fixture traced by hand: group 2024-08-15, 2025-08-15 ×2, 2026-08-15 → `latestRun` = `[2025-08-15 b, 2026-08-15]` (the 0-day gap ends the walk-back), `run[0]` is the duplicate, listing charge 2026-08; `before` = `[2024, 2025a]` is ok, steady, gap 0 → resumes today (bug). As of 2025-08 `latestRun` over `[2024, a, b]` is not ok (gap 0), so nothing was listable then.

### Acceptance (red)
- [x] Step 1: `summary_recurring_test.go:15-20` (add `afterAugust` = 2026-09-02) + after `:124` `Test_summary_lists_a_subscription_as_new_when_its_earlier_series_was_never_listable` — dates above; `summaryRecurring(t,"2026-08",afterAugust,…)` lists `Gym`; `summaryRecurring(t,"2025-08",afterAugust,…)` empty; `recurringAt` over 2026-08 still lists `Gym` (`recurring` unchanged). Fails at its assertion today (hidden). No stubs.

### Build
- [x] Step 2: tests `summary_recurring_arms_test.go:102-150` `Test_summary_resumption_skips_off_schedule_charges` (rename allowed; nothing cites it) red first, then `summary_recurring.go:26-30` `resumes` + one new unexported helper (e.g. `listableAt(group, day)`) — one batch.
  - Code: `resumes` gains a fourth condition after (2) steady and (3) gap: `latestRun` over the GROUP's charges dated on or before the earlier run's last charge's day is ok. "Ends at that charge" needs no branch: the prefix ends on that day, and a second charge that day is a 0-day gap, never ok. No-same-day-charge groups are unaffected by construction (prefix equals `before[:end]`). `earlierRun` unchanged: still stops at the first ok run; on U18 failure `resumes` returns false (no further search). 1-2 line doc; helper finds the prefix end by binary or linear scan, no whole-group copy.
  - Tests (rows in the S21 table; `strays` offsets already allow 0 = same-day duplicate and negatives): `same-day duplicate on the earlier run's last day, run starts 40 days after` (strays `{0}`, start 40; listed: `earlierRun` drops the duplicate and finds the steady run, so this is the fix); control `no duplicate, run starts 40 days after` (strays none, start 40; hidden, U17 holds; green on arrival); `duplicate earlier in the earlier run, run starts 40 days after` (strays `{-60}`, start 40; hidden: `[D-60b, D-30, D]` is still listable); `duplicate is the run's first charge` (strays `{0}`, start 30, listed — the monthly variant of the acceptance shape). Existing `one stray` (`{21}`, start 30), `two strays` (`{10,21}`), the 45/46, annual and "over a year" rows stay unedited, hidden/listed as now: they hold no same-day charge, so U18 is vacuous for them.
  - Harness: that last row's run is `[dup, R0, R1]`, listing at R1 = 30 days after `start`, not 60; add a `runCharges` column (0 = 3) so `runEnd = onDay(start + 30*(runCharges-1))` and `monthlyEndingOn(runEnd, runCharges)`; the row sets 2. Existing rows leave it 0 and are unchanged. Start 30 with a duplicate and 3 run charges lists a month early: the trap in Handoff.
  - Real file: the 4 earlier runs recognized at their own month end (key A) satisfy U18 by construction (month-end recognition means `latestRun` over everything through that month end is ok and ends at the run's last charge; the charge-day prefix is that same slice or shorter-to-same). They stay hidden; confirm only by re-running SCENARIO-18.
  - End of step, once: narrow loop green, then `go test ./cmd/quarry/ -run 'run_summary|run_mcp_summary'`, `go test ./internal/cli/ -run summary`, `go test ./internal/mcp/ -run 'summary|Summary'`.

### Sweep
- [x] Step 3: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc ./internal/report` unchanged (nothing exported).

### Verify
- [x] Step 4: `.claude/scripts/verify.sh <start> ./internal/report/...`; `spec-check.py phase4f-summary`; tick SCENARIO-22 in `specification.md:529` (test last on the line); rewrite STATE.md (decision at `:10` gains the U18 condition; add the same-day trap below); `status: done`.

## Handoff

**Binding decisions:**
- The earlier run counts only if `latestRun` over the group's charges dated on or before its last charge's day is ok (U18); the prefix comes from the whole group by date, not from `before`, because the duplicate may be `run[0]`. On failure `resumes` is false and the search does not continue to an older run.
- `latestRun`, `earlierRun`'s drop loop and `recurring.go` stay untouched: `quarry recurring` still lets a same-day duplicate break a run, pinned by the acceptance test's `recurringAt` assertion.

**Left unbuilt:** nothing; SCENARIO-18 re-runs on the real file after this scenario (yearly series with a duplicated 2025 charge must list new in 2026-08, the 4 key-A resumptions stay hidden).

**Traps:**
- A same-day duplicate joins the run when the next gap is 30 (monthly): `[Da, Db, R0..]` makes `latestRun` return `[Db, R0, R1, R2]`, listing one month earlier than the harness's `runEnd` month; keep start 40 for rows where the duplicate must be dropped as a stray, and start 30 only with `runCharges` 2.
- The prefix must not stop at the month end: a stray later in the earlier run's own month would then un-resume a series U17 hides.

## Phase report

Run A+B1 (steps 1-2) and V (steps 3-4) done.

- `internal/report/summary_recurring_test.go:16-21,126-138` `afterAugust` + acceptance test. Red at its first assertion (`expected: []string{"Gym"} actual: []string{}`); its `2025-08` empty and `recurringAt` Gym assertions were green on arrival.
- `internal/report/summary_recurring_arms_test.go` `Test_summary_resumption_skips_off_schedule_charges`: new `runCharges` column (0 = 3), 4 new rows. Red before the fix: `same-day duplicate on the earlier run's last day, run starts 40 days after` and `duplicate is the run's first charge`; `no duplicate, run starts 40 days after` and `duplicate earlier in the earlier run, run starts 40 days after` green on arrival (controls). Renamed in V to `Test_summary_resumption_with_off_schedule_and_same_day_charges`.
- `internal/report/summary_recurring.go:26-43` `resumes` gains `listableAt(group, last)` (sort.Search prefix through the earlier last charge's day, then `latestRun` ok).
- Mutations (a)-(e) all redden the planned rows (see report). Narrow loops green: report, cmd/quarry `run_summary|run_mcp_summary`, cli `summary`, mcp `summary|Summary`.
- V: test renamed `Test_summary_resumption_with_off_schedule_and_same_day_charges`; lint 0 issues; `verify.sh` all rc=0, internal/report 710 (+1); spec ticked, `spec-check` OK; STATE.md rewritten.
