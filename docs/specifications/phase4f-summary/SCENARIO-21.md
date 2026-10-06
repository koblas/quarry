---
id: SCENARIO-21
status: done
---

# SCENARIO-21: A subscription that kept charging through a stray charge is not new

Cadence: test-first (bug fix: `resumes` calls the exact-prefix `latestRun`, a stray charge makes it not ok, so a continuing series is listed new)
Acceptance test: `internal/report/summary_recurring_test.go` `Test_summary_does_not_list_a_subscription_that_kept_charging_through_a_stray_charge_as_new`
Narrow loop: `go test ./internal/report/ -run 'summary|Summary|recurring'`
Mutation checks: (a) revert `earlierRun` to one `latestRun` over the whole prefix → acceptance test + one-stray row; (b) drop at most one trailing charge → `two strays` row; (c) measure the gap from the last dropped stray instead of the earlier run's last charge → `stray, R[0] 46 days after the earlier run's last charge` row (listed); (d) keep dropping past an ok-but-unsteady run until a steady prefix → `earlier run not steady` row (listed); (e) cap the drop loop at `monthlyEndedAfterDays` instead of the longest ended-after → `annual earlier series 400 days before, with a stray` row (hidden)
Runs: A+B1 (1-2) | V (3-4)
Size: OWNS A RUN — 1 Build batch, 1 feature package (`internal/report`; summary-only, `recurring.go` untouched)

## Implementation Plan

Premise check: SCENARIO-19/20's acceptance tests are `Server.Summary` tests in `internal/report` (`summary_recurring_test.go:52,102`), not `cmd/quarry`; this one follows them. Callers of `resumes`: only `newInMonth` (`summary_recurring.go:8`), reached only through `recurringFrom`'s `scope.keep`; `latestRun` (`recurring.go:88-103`) stays unchanged, so `quarry recurring` output is byte-identical by construction.

### Acceptance (red)
- [x] Step 1: `summary_recurring_test.go:15-19` (add `afterMarch`, 2026-04-02 or later) + after `:108` `Test_summary_does_not_list_a_subscription_that_kept_charging_through_a_stray_charge_as_new` — Oct 15/Nov 14/Dec 14 2025 at 1000, stray 2026-01-04 (D+21), then 1500 on Jan 13/Feb 12/Mar 14 (R[0]=D+30); `summaryRecurring(t,"2026-03",afterMarch,…)` empty; `recurringAt` for March still lists `Gym` (`recurring` unchanged). Fails at its assertion (listed new today). No stubs needed.

### Build
- [x] Step 2: `summary_recurring_arms_test.go` after `:100` `Test_summary_resumption_skips_off_schedule_charges` red first, then `summary_recurring.go:22-26` `resumes` + new unexported `earlierRun(before []store.Charge, first time.Time) ([]store.Charge, cadenceRule, bool)` — one batch.
  - Tests: table through `summaryRecurring`, one variable per row against the control (R = 3 monthly charges, listing in the summarized month; strays placed so no gap to a neighbour falls in a cadence range 6-8/26-35/84-98/350-380): one stray; stray, R[0] 45 days after (hidden) and 46 days after (listed, U3 holds); two strays (D+10, D+21; hidden); earlier run not steady = 4 charges 1000,1000,1000,1500 ending D, stray D+3, R[0] = D+14 (listed; mutation d needs the first three steady and R[0] within 45 days of the third); earlier run too short (2 charges) + stray (listed); annual earlier series (2 charges 365 apart) + stray, R[0] 400 days after (hidden) and 401 (listed); stray with the earlier series over a year old (listed; runs the 400-day cap). Existing `Test_summary_resumption_arms` and the S19/S20 tests stay unedited and green.
  - Code: drop trailing charges while `latestRun(before[:end])` is not ok, stopping at the first ok run (steady or not: U2 (2) then judges it) and at the first charge more than the longest ended-after (`annualEndedAfterDays`, `recurring.go:48`) before `first`; `resumes` keeps (2) `steadyCharges` and (3) `daysBetween(earlier last, run[0]) <= rule.endedAfter` from the earlier run's own last charge, not the dropped tail's. Doc comments 1-2 lines.
  - End of step, once: narrow loop green, then `go test ./cmd/quarry/ -run 'run_summary|run_mcp_summary'`, `go test ./internal/cli/ -run summary`, `go test ./internal/mcp/ -run 'summary|Summary'` (lowercase on cmd/quarry: case-sensitive trap).

### Sweep
- [x] Step 3: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc ./internal/report` unchanged (nothing exported).

### Verify
- [x] Step 4: `.claude/scripts/verify.sh <start> ./internal/report/...`; `spec-check.py phase4f-summary`; tick SCENARIO-21 in `specification.md` (`BDD Acceptance Progress`, test last on the line); rewrite STATE.md (decision at `:10` becomes "latest steady run with trailing off-schedule charges dropped, U17"; trap `:35` still true); `status: done`.

## Handoff

**Binding decisions:**
- The fix lives only in `resumes`/`earlierRun` in `summary_recurring.go`; `latestRun` and `recurring.go` stay untouched — `quarry recurring` and every non-summary surface must stay byte-identical, and `Test_recurring_still_lists_a_resumed_series` pins it.
- Search stops at the FIRST ok run found while dropping trailing charges, steady or not — dropping further to reach a steady prefix would treat an unsteady series (with a price change in it) as steady (U2 (2)), and hides a genuinely new series.
- The gap in (3) is measured from the earlier run's last charge, never from a dropped stray — a stray inside the quiet period would otherwise keep a long-dead series alive.
- Cost: `resumes` runs only for a series whose listing charge is in the month (`inWindow` short-circuits first). The drop loop walks only charges within 400 days of `R[0]`, each `latestRun` call O(minCharges) when not ok: worst case a few hundred cheap iterations on a 272-charge group, once per candidate series.

**Risk (for product-vision/gate):** same rule, larger reachable population for dense payees. `latestRun` finds only runs of consecutive charges in the group, so an earlier series interleaved with other purchases from the same payee is never found, which is what limits dense payees; before the fix a dense payee's prefix nearly always ended off-schedule, so it was always reported new. Residual: a coincidental consecutive cluster that is steady (e.g. 4 charges 6-8 days apart) and ends within 14 (weekly) or 45 (monthly) days of `R[0]` now hides a genuinely new series even with strays between. Condition (3), not the 400-day cap (cost only), bounds the reach. A cadence-blind match (weekly cluster hiding a monthly series) is pre-existing.

**Left unbuilt:** nothing new; SCENARIO-18 (reference check on the real file) re-runs after this scenario to confirm the real-file case.

**Traps:**
- The 400-day cap is observable only if too small (mutation e) or too large-with-no-effect: a cap above 400 is equivalent behaviour, a pure cost bound — do not chase an equivalent mutant.
- A stray at exactly 6-8 / 26-35 days from a neighbour is a cadence gap and changes which run `latestRun` finds: pick stray dates by the gap table, not by feel (D+21 and R[0]-stray 9 days are outside every range).
- An existing summary test flipping from listed to hidden is a fixture with a stray before a run: confirm it is a U17 case before editing its expectation; never flip it blind.

## Phase report

Runs A+B1 (steps 1-2) and V (steps 3-4) done.

- `internal/report/summary_recurring.go:27-41`: `resumes` now calls new `earlierRun(before, first)` (drops trailing charges until `latestRun` is ok, stops at charges more than `annualEndedAfterDays` before `first`); the gap in (3) stays measured from the earlier run's own last charge. `latestRun`/`recurring.go` untouched.
- `internal/report/summary_recurring_test.go`: `afterMarch` clock plus the acceptance test (green).
- `internal/report/summary_recurring_arms_test.go`: `Test_summary_resumption_skips_off_schedule_charges`, 9 rows built from `earlierEnd = 2026-01-10` and day offsets (green).
- Red before fix: acceptance + rows `one stray`, `two strays`, `stray, run starts 45 days...`, `annual earlier series, run starts 400 days after`; the five `listed` rows were green on arrival (controls).
- Mutations (a)-(e) each reddened their named row; file restored byte-identical.
- Narrow loops green: `internal/report`, `cmd/quarry` (`run_summary|run_mcp_summary`), `internal/cli` (`summary`), `internal/mcp` (`summary|Summary`).
- V: `earlierRun` doc reworded (checkpoint NIT); lint `0 issues`; `verify.sh` rc=0, internal/report 709 (+2), 0 uncovered added lines; only unexported symbols changed so `go doc ./internal/report` is unchanged; spec ticked, STATE.md rewritten.
