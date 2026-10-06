# phase4f-summary — current state

Scenarios complete: SCENARIO-01a (folds 02, 19, 03, 20, 04), SCENARIO-01b (folds 05, 08, 09, 13, 15), SCENARIO-06 (folds 07). Last updated by SCENARIO-06.

## Binding decisions
- One port call per `Server.Summary`: `SummaryReads.Summary` returns Status, Charges and NetWorth from one open (duckstore `readStatus`/`readCharges`/`readNetWorth` over one `ReadDB`). Snapshot, dates and findings render from `Summary.Status` — never a second `Status`/`NetWorth` call; sync renames the store mid-run, and the cli fake's nil `Status` panics on a second read (SCENARIO-01a, 01b)
- `report.Store` stays at 10 interface entries (interfacebloat): new reads join an embedded group (`SummaryReads` holds `Charges` + `Summary`), never Store's top level (SCENARIO-01a)
- `SummaryRequest{Month, Currency}` has no clock: `ParseMonth(value *string, now)` is the only clock reader; recurring's today = the month's last day. The command takes one `now()` per run (local zone, not `.UTC()`) for `ParseMonth` and the Snapshot age; S10's `covers_month` and W3a must use that same value (SCENARIO-01a, 01b)
- Anomalies and recurring share one Charges read through the month end. Anomalies equal `quarry anomalies --since M --until M` (baselines strictly earlier; Checked counts in-window only) (SCENARIO-01a)
- A series is new in M when its listing charge falls in M and the run does not resume an earlier steady run within that run's own `endedAfter` (`newInMonth`, `resumes` in `summary_recurring.go`). Recognized-then-broken series stay omitted later months; a weekly series ended by M's last day is still listed (state `SeriesEnded`); month lower bound `0001-01` (SCENARIO-01a, rulings U2/U3/U4)
- Change: nil only when the first month end has no row in any currency; nil value = `no rate`; converted totals exactly one entry; a converted Change with an empty END day counts that day 0.00 — `no rate` only when a day that HAS rows lacks the reporting-currency entry or holds another (SCENARIO-01a). `renderSummary` appends Change rows to the net-worth history table (shared widths); `Change == nil` prints no Change row (SCENARIO-01b)
- Refusals use command word `summary` (`summary interrupted`); `MonthError{Kind, Value, Example}`, `Error()` is the CLI line without `quarry: ` (SCENARIO-01a)
- Findings counts are not in `Summary`: `report.CountFindings(Summary.Status, ignore, classification)` as status does (SCENARIO-01a, 01b)
- Summary is a third config class: loads the config always (ignore list, classification) but refuses an unreadable one only without `--currency`; with it, W2 `document.CannotTellChoices(config.Problem(err))` is held in `summaryChoice.cannotTell` and printed only after the report is read (a refusal prints alone) and `IgnoreKnown=false`. S12's JSON `findings.ignored` null and S16 mirror it (SCENARIO-01b)
- W1 (config warnings) print before stdout; S10/S14's W3-W6 go after stdout through `emitReport`, in the spec's order (SCENARIO-01b)
- Check order: Args (positional, then `--currency` value) → `--month` → config → store (SCENARIO-01b)
- Empty section bodies are summary-only composers in `render_summary.go` (`summaryAnomaliesSection`, `summaryRecurringSection`, `renderEmptySection`); `renderAnomalies`, `renderRecurringTitled`, `renderNetWorthHistory` and every other command's output stay byte-identical. Empty test is `len(Listed)==0` / `Recurring.Empty()`, not `Checked==0`: all-too-young charges say `No unusually large charges.` above `N charges checked; M had too little history to judge`. An empty month's anomalies section differs from `quarry anomalies` (header row); the byte-identity pin stays on a non-empty month (SCENARIO-06)
- No-change vs no-balance both key on `NetWorth.Change()==nil`; they differ only by whether the last month end has rows. `renderNetWorthWithChange` indexes `Dates[0]`/`Dates[len-1]` with no length guard: `Server.Summary` always holds both month ends (SCENARIO-06)
- Interim, never ships: `summary --json` → `UsageError` `summary --json is not available yet`; a nil Change cell prints `no rate` now (ruled U5) (SCENARIO-01b)

## Left unbuilt
- Suppression of the empty-window lines of `AnomaliesWarnings` (warnings.go:61-68, fires on `Checked==0`), `RecurringWarnings` (:53-59, fires on `Recurring.Empty()`) and `emptyNetWorthWarnings` — SCENARIO-14 wires W4-W6; `summary.go` calls none today, so SCENARIO-06's empty-stderr pins are green on arrival and must stay green when S14 lands (drop only each warning's last empty-window line, keep left-out/unconverted lines) (SCENARIO-06)
- `--json` empty shapes (`{"types":[],"totals":[]}`, `dates` null/null) — SCENARIO-12 (SCENARIO-06)
- `covers_month`, W3a/W3b — SCENARIO-10/11; `document.Summary`, `renderSummaryJSON` (replaces the interim `--json` refusal) — SCENARIO-12; W4-W6 and the no-rate matrix — SCENARIO-14; `monthly_summary` MCP tool — SCENARIO-16; SKILL/README/reference/PRD Decisions + MCP row — SCENARIO-16/17

## Traps
- `Server.Anomalies`/`Server.Recurring` read Charges through *today*: calling them from Summary costs two full Charges reads and the wrong recurring clock (SCENARIO-01a)
- `TypeNeedsRate` is false when one row of the type converts; Change uses the any-row predicate `typeNeedsAnyRate` (SCENARIO-01a)
- The report fake's `Charges` ignores `Through`; the fake `Summary` filters by `Through`, or the month-end-clock tests are vacuous (SCENARIO-01a)
- The 40-day late-bill gap breaks the run at 2026-09; the resumption rule is pinned at 2026-11 (`Test_summary_does_not_list_a_late_bill_of_an_old_subscription_as_new`, `Test_summary_resumption_arms`) (SCENARIO-01a)
- gopls in this worktree may be rooted at `../phase4d-registered`: LSP paths and lines come from that tree; anchor with Read (SCENARIO-01a)
- `spendEnv`'s clock is 2026-09-29, so its default month is August and `--month 2026-09` is refused as not ended; summary tests use `summaryClock` (2026-10-06) (SCENARIO-01b)
- Adding summary to `configAlwaysReadCommands` or `readCommandArgs` makes existing tests (currency_test.go configAlways loops, run_config_test.go empty-stderr) fail for a correct summary (SCENARIO-01b)
- A CLI-level "no ignored clause on W2" assertion is vacuous (an unreadable config already leaves `Ignored` 0); `IgnoreKnown=false` is pinned in `Test_summaryFindingsPhrase_says_ignored_only_when_the_ignore_list_was_read` (SCENARIO-01b)
- Narrow-loop `-run` is case-sensitive: command-level summary tests are `Test_run_summary_*`, so `-run 'Summary'` on `./cmd/quarry/` runs nothing; use lowercase `run_summary` (and `summary` on `internal/cli`) (SCENARIO-06)
- `septemberSummary` (render_summary_internal_test.go) carries both month ends in `NetWorth.Dates`; a fixture without them panics `renderNetWorthWithChange` (SCENARIO-06)
- Native fixture `summaryNativeRows` has no USD-end-day-only account at command level; that arm is pinned in the `changeRows` internal tests (SCENARIO-01b)

## Open debts
- SCENARIO-12 adds `summary` to `genericCurrencyCommands` (currency_test.go:24) and the `--json` loops (:347), and deletes the summary-specific currency tests (`Test_summary_refuses_a_currency_that_is_not_cad_usd_or_native`, `Test_summary_shows_amounts_in_the_currency_it_was_given`, `Test_summary_refuses_a_positional_argument_before_the_currency_flag`, `Test_summary_reads_the_config_once_without_the_currency_flag`) they duplicate, and the interim `--json` refusal test `Test_summary_refuses_json_until_its_document_exists` (SCENARIO-01b)
- No checkpoint MINOR/NIT left unfolded by SCENARIO-01b or SCENARIO-06 (the not-judged footer pin and the doc trim folded into run V)
