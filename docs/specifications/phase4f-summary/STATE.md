# phase4f-summary — current state

Scenarios complete: SCENARIO-01a (folds 02, 19, 03, 20, 04). Last updated by SCENARIO-01a.

## Binding decisions
- One port call per `Server.Summary`: `SummaryReads.Summary` returns Status, Charges and NetWorth from one open (duckstore `readStatus`/`readCharges`/`readNetWorth` over one `ReadDB`). Later scenarios render snapshot, dates and findings from `Summary.Status` — never a second `Status`/`NetWorth` call; sync renames the store mid-run (SCENARIO-01a)
- `report.Store` stays at 10 interface entries (interfacebloat): new reads join an embedded group (`SummaryReads` holds `Charges` + `Summary`), never Store's top level (SCENARIO-01a)
- `SummaryRequest{Month, Currency}` has no clock: `ParseMonth(value *string, now)` is the only clock reader; recurring's today = the month's last day, so a rerun reproduces every section (SCENARIO-01a)
- Anomalies and recurring share one Charges read through the month end. Anomalies equal `quarry anomalies --since M --until M` (baselines strictly earlier; Checked counts in-window only); pinned by `Test_summary_anomalies_equal_the_anomalies_read_of_the_month` (SCENARIO-01a)
- A series is new in M when its listing charge (earliest steady prefix with the rule's minimum charges) falls in M and the run does not resume an earlier steady run within that run's own `endedAfter` (`newInMonth`, `resumes` in `summary_recurring.go`). Recognized-then-broken series stay omitted the following months (SCENARIO-01a, rulings U2/U3/U4)
- Ruled: a weekly series whose listing charge is in M but that ended by M's last day is listed as new (state `SeriesEnded`) (SCENARIO-01a)
- Ruled: month lower bound stays `0001-01` (`--month 0001-01` valid, `0000-01` refused); DuckDB accepts the `0000-12-31` Dates entry, so no 0001-02 change and no new copy (SCENARIO-01a)
- Change: nil only when the first month end has no row in any currency; nil value = `no rate`; converted totals exactly one entry; ruled: a converted Change with an empty END day counts that day 0.00 (a number, not `no rate`) — `no rate` only when a day that HAS rows lacks the reporting-currency entry or holds another (SCENARIO-01a)
- Refusals use command word `summary` (`summary interrupted`); MCP reuses it as recurring_charges reuses `recurring`. `MonthError{Kind, Value, Example}` carries parts, `Error()` is the CLI line without `quarry: `; MCP words its own (SCENARIO-01a)
- Findings counts are not in `Summary`: callers use `report.CountFindings(Summary.Status, ignore, classification)` as status does (SCENARIO-01a)

## Left unbuilt
- `quarry summary` command, heading, Findings composer, Change row rendering — SCENARIO-01b
- `covers_month`, W3a/W3b — SCENARIO-10; `document.Summary` — SCENARIO-12; W4-W6 assembly — SCENARIO-14; `monthly_summary` MCP tool — SCENARIO-16; empty-section copy — SCENARIO-06

## Traps
- `Server.Anomalies`/`Server.Recurring` read Charges through *today*: calling them from Summary costs two full Charges reads and the wrong recurring clock (SCENARIO-01a)
- `TypeNeedsRate` is false when one row of the type converts; Change uses the any-row predicate `typeNeedsAnyRate` (SCENARIO-01a)
- The report fake's `Charges` ignores `Through`; the fake `Summary` filters by `Through`, or the month-end-clock tests are vacuous (SCENARIO-01a)
- The 40-day gap in a late-bill history breaks the run (`ruleForGap(40)` fails) at month 2026-09, so that month alone does not pin the resumption rule; the pin is at 2026-11 (`Test_summary_does_not_list_a_late_bill_of_an_old_subscription_as_new`, `Test_summary_resumption_arms`) (SCENARIO-01a)
- gopls in this worktree may be rooted at `../phase4d-registered`: LSP paths and lines come from that tree; anchor with Read (SCENARIO-01a)

## Open debts
- None recorded by SCENARIO-01a beyond the above; checkpoint MINOR/NIT not folded into V: unowned — dies unless re-opened
