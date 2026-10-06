---
id: SCENARIO-10
status: open
---

# SCENARIO-10: Snapshot taken before the month ended is warned about (folds SCENARIO-11)

Cadence: code-first (no mandatory test-first item: no bug fix, write-safety guard or atomic adapter)
Acceptance test: `cmd/quarry/run_summary_snapshot_test.go` `Test_run_summary_warns_when_the_snapshot_was_taken_before_the_month_ended`
Acceptance test (SCENARIO-11, folded): `cmd/quarry/run_summary_snapshot_test.go` `Test_run_summary_warns_when_the_snapshot_records_no_time`
Narrow loop: `go test ./internal/report/... -run 'summary|month'` then `go test ./internal/cli/ -run 'summary|Summary'` then `go test ./cmd/quarry/ -run 'run_summary'` (lowercase; `-run Summary` on cmd/quarry runs nothing)
Mutation checks: `!taken.Before(closes)` boundary in `coverageOf` → exact-midnight and 1 ns-before rows; month's zone in `closes` → America/Toronto DST rows; zero-`TakenAt` arm → Unknown row; `--month` tail choice in `summaryAgain` → both tail tests
Runs: A (1) | B1 (2-4) | V (5-6)
Size: OWNS A RUN — 3 batches, 1 feature package (report + report/document; cli and cmd/quarry wiring do not count). Sizing said 2; the Month.Zone seam made it 3. S11 FOLDED: same composer, one arm.

Existing, not re-planned: Snapshot row for null `taken_at` (`snapshotLine`, render_status.go:82-88) already says `time taken not recorded in its manifest`; S10 only pins it at command level.
Surface surveyed (grep, gopls rooted at ../phase4d-registered): `Summary` callers = internal/cli/summary.go:97 only (MCP is S16); `ParseMonth` callers = summary.go:82 and tests; `Month{` literals = render_summary_internal_test.go:21 only (nil Zone must keep working). No new port method: `Status.Run.Snapshot.TakenAt` already arrives in `Summary.Status`.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: new `cmd/quarry/run_summary_snapshot_test.go` — both tests through `runWith`. Seed with `summaryRows(true)` then override `rows.ImportRuns[0].Snapshot.TakenAt` and `replaceStore` (zero `TakenAt` writes NULL, duckstore.go:706); `pinLocalZone` plus a `now` built in the same EDT zone (not `summaryClock`, which is UTC and would print the W3a time in UTC). S10: taken 2026-09-28 14:02 EDT, `quarry summary`: exit 0, stdout heading + Snapshot row, stderr exactly the W3a line with tail `then run quarry summary again`. S11: zero taken: Snapshot row `20261001T130512Z, time taken not recorded in its manifest`, stderr exactly W3b. Control (empty stderr, snapshot Oct 1 09:05 EDT): existing `Test_run_summary_prints_last_months_summary` (run_summary_test.go:84). No stubs: compiles on today's surface; red at the missing stderr line.

### Build
- [ ] Step 2: `internal/report/month.go:10-12,58-73`, `summary.go:20-29,36-53` — `Month.Zone *time.Location` set by `ParseMonth` from `now.Location()` (nil reads as UTC; doc on the field); unexported `(Month).closes()` = local midnight beginning the next month in that zone via `time.Date`; `SnapshotCoverage` (`SnapshotCovers`, `SnapshotPredatesMonthEnd`, `SnapshotTimeUnknown`); `Summary.Coverage` set in `Server.Summary` from `read.Status.Run.Snapshot.TakenAt`: zero → unknown, `!taken.Before(closes)` → covers, else predates. Tests (`summary_coverage_test.go`, through `Server.Summary` with `fakeStore.status`; assert on `got.Coverage`; control row per bound): exactly local midnight → covers; 1 ns before → predates; zero → unknown; zones UTC, fixed EDT, America/Toronto (`time.LoadLocation`, `require.NoError`) with month 2026-10 whose boundary is 2026-11-01T04:00Z (an EST-fixed impl lands on 05:00Z); month 2026-12 (year rollover); leap February; run on the 1st at 00:05 (taken 00:01 covers, 23:59 the day before predates); nil Zone = UTC. `month_test.go`: `Test_a_parsed_month_keeps_the_zone_of_the_clock`. Fault tests: n/a (no fallible call; the one store call is already faulted by the S01a refusal tests).
- [ ] Step 3: new `internal/report/document/summary_warnings.go` (+ `_test.go`) `SnapshotWarning(s report.Summary, again string) string` — "" when covers; W3a verbatim with `s.Month.Name()`, time `Month.Zone` formatted `2006-01-02 15:04 MST`, tail `then ` + `again`; W3b verbatim for unknown (no tail). `again` is the surface's phrase (`run quarry summary again`, `run quarry summary --month 2026-08 again`, S16's `call monthly_summary again`). Rows: each coverage arm; default tail, `--month` tail, MCP tail (all three through the one function); another month and year in the name (`0001` row: `January 0001`); zone label in text (EDT, UTC).
- [ ] Step 4: `internal/cli/summary.go:72-110` — W2 stays before `writeResult` (STATE binding), so replace `writeResult` with `emit(cmd, []byte(renderSummary(...)), "quarry: warning: ", warnings)`: W3 lands on stderr after stdout and after W2, and only once the stdout write succeeded. `summaryAgain(month, given bool)` builds the tail from `cmd.Flags().Changed` (line 79 already computes it). Tests in `internal/cli/summary_test.go` via `executeSummary` with `fakeReportStore.summary.Status` TakenAt set: default month tail; `--month 2026-08` tail; covered snapshot → empty stderr; W2 then W3a order (unreadable config + `--currency`, line 182 pattern); stdout write failure → refusal, no W3 line (control: warning present when the write succeeds); `--json` stays the interim refusal (S12).

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`. Expected churn: fake-based tests that assert empty stderr or exact stderr at `summary_test.go:~182-260` now get W3b because the fake's zero `TakenAt` is unknown; give `executeSummary`'s default fake a `TakenAt` after the month end rather than loosening any pin. Doc comments on `SnapshotCoverage`, `Month.Zone`, `SnapshotWarning`.

### Verify
- [ ] Step 6: full verification per agent-briefs.md plus `.claude/scripts/spec-check.py phase4f-summary`; tick SCENARIO-10 with its acceptance test and SCENARIO-11 `delivered by SCENARIO-10` with its test (reference last on the line); rewrite STATE.md (move `covers_month`/W3a/W3b out of Left unbuilt).

## Handoff

**Binding decisions:**
- `Summary.Coverage` (three-state) lives on `report.Summary`; S12 maps covers→`true`, predates→`false`, unknown→`null` for `covers_month`. Q4: the rule is in core, not in cli or document.
- `Month.Zone` (set only by `ParseMonth`; nil = UTC) is the one zone for the month's bounds and the W3a time. `SummaryRequest` still has no clock. Production: `now.Location()` is the Mac's local zone, so it equals the Snapshot row's `time.Local`; they differ only in tests that pass a UTC `now` with a pinned `time.Local`.
- The W3 text is `document.SnapshotWarning(summary, again)`, surface-parameterized by the `again` phrase. S12 folds it into `warnings[]` after W1/W2; S14 appends W4-W6 after it; S16 passes `call monthly_summary again`.
- W3 is printed after stdout through `emit`; W2 stays before stdout. Do not move W2 to after.

**Defaults pending ruling** (unruled outcomes; developer implements these, orchestrator may overrule):
- MCP tail with an explicit `month`: same `then call monthly_summary again` (spec rules one tail only); S16 decides.
- Zone of the W3a time: `Month.Zone`, not `time.Local` (see above).
- Month end falling in a zone whose local midnight does not exist (DST at 00:00): `time.Date`'s normalization, unpinned.
- Snapshot taken after the month ended by any amount, or in the future: covers, no warning.
- W3b fires for any zero `TakenAt`, with or without dates in the store.

**Left unbuilt:** `document.Summary`, `renderSummaryJSON`, `covers_month` key (S12); W4-W6 and empty-window suppression (S14); `monthly_summary` (S16).

**Traps:**
- `summaryClock` is UTC: with `pinLocalZone` the W3a time would print in UTC while the Snapshot row prints EDT. Build the acceptance `now` in EDT.
- Fake-based cli summary tests with zero `TakenAt` start printing W3b (Step 5); a cmd/quarry store always has `summaryTakenAt` (Oct 1 09:05 EDT), which covers September.
- `Month.Start/End` are UTC midnights of calendar days, not instants: never compare `taken_at` to `End` or `Start.AddDate` in UTC; the bound is `closes()`.

**Orchestrator rulings 2026-10-06 (pre-dispatch):** all five defaults stand — (1) MCP tail for an explicit month decided at S16; (2) W3a time formatted in Month.Zone (equals time.Local in production; acceptance builds now in EDT + pinLocalZone); (3) nonexistent local midnight follows time.Date normalization, unpinned; (4) taken after the month ended by any amount, incl. future, covers — no warning; (5) W3b for any zero TakenAt.

## Phase report

Run A (acceptance, red). Start commit `fe7d11a`.

- New `cmd/quarry/run_summary_snapshot_test.go`: `Test_run_summary_warns_when_the_snapshot_was_taken_before_the_month_ended` and `Test_run_summary_warns_when_the_snapshot_records_no_time`, both through `runWith`. No production code touched, no stubs (compiles on today's surface).
- Helper `summaryClockEDT()` (same file): 2026-10-06 12:00 in a fixed `EDT` (-4h) zone; with `pinLocalZone` the W3a time and the Snapshot row print in the same zone. Do not reuse `summaryClock` (UTC) for these tests.
- Seed: `summaryRows(true)` with `ImportRuns[0].Snapshot.TakenAt` overridden (2026-09-28 14:02 EDT, or zero), one `replaceStore`. The Findings line differs from `seedSummaryStore` (single build), so stdout is asserted with `Contains` on heading + Snapshot row (`(7 days ago)`), not whole-document equality.
- Red, both at the stderr assertion (stdout Contains passes): `run_summary_snapshot_test.go:33` expected the W3a line, actual `""`; `:51` expected the W3b line, actual `""`.
- Next (B1): Steps 2-4 as planned; nothing here to undo.
