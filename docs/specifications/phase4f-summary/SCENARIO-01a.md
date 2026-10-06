---
id: SCENARIO-01a
status: open
---

# SCENARIO-01a: The report server summarizes a month

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `internal/report/summary_test.go` `Test_summary_holds_the_months_anomalies_new_recurring_and_net_worth_change`
Acceptance test (SCENARIO-02, folded): `internal/report/summary_recurring_test.go` `Test_summary_lists_a_subscription_as_new_in_the_month_of_its_third_charge`
Acceptance test (SCENARIO-19, folded): `internal/report/summary_recurring_test.go` `Test_summary_lists_a_subscription_with_an_early_price_change_as_new_in_its_first_steady_month`
Acceptance test (SCENARIO-03, folded): `internal/report/summary_recurring_test.go` `Test_summary_does_not_list_a_late_bill_of_an_old_subscription_as_new`
Acceptance test (SCENARIO-20, folded): `internal/report/summary_recurring_test.go` `Test_summary_lists_a_subscription_resumed_after_it_ended_as_new`
Acceptance test (SCENARIO-04, folded): `internal/report/summary_recurring_test.go` `Test_summary_recurring_ignores_charges_after_the_month`
Narrow loop: `go test ./internal/report/ ./internal/store/duckstore/ -run 'Summary|Month|Recurring|Anomal|NetWorth|reads'`
Mutation checks: resumption bound `<= endedAfter` → `<` in the summary's resumption test → the 45-day row of `Test_summary_resumption_arms` (month 2026-11) | port call in `(*Server).Summary` replaced by `s.store.Charges` → `Test_summary_reads_the_store_once`
Runs: A (1-2) | B1 (3-4) | B2 (5) | B3 (6-7) | V (8-9)
Size: OWNS A RUN — 5 batches, 1 feature package (report) + its store adapter (duckstore)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `internal/report/summary_test.go` (new) `Test_summary_holds_the_months_anomalies_new_recurring_and_net_worth_change` — fake store: one Sept anomaly, a monthly series whose 3rd charge is in Sept, balances Aug 31 + Sep 30; month from `ParseMonth("2026-09", 2026-10-06)`, CAD; asserts anomalies listed, the series in Recurring, two NetWorth dates, Change values
- [x] Step 2: signature-only stubs — `internal/report/month.go` (new) `Month`, `ParseMonth`, `MonthError`/`MonthErrorKind`; `internal/report/summary.go` (new) `SummaryRequest{Month, Currency}` (no `Now`), `Summary`, `(*Server).Summary`; `internal/report/store.go:22-44` new embedded `SummaryReads{Charges, Summary}` with `Charges` (:30-32) moved out of `Store`'s top level; `internal/store/store.go:902-950` `SummaryParams{Through, Dates}` + `Summary{Status, Charges, NetWorth}`; `internal/store/duckstore/summary.go` (new) `(*Store).Summary` stub; `internal/report/fakes_test.go:9-43,124-132` real `Summary` (filters charges by `Through`, records `gotSummary`, `summaryReads`); stub every other implementer the build lists

### Build
- [ ] Step 3: `month.go` `ParseMonth`, `Month` (window, `YYYY-MM`, `time.Format("January 2006")`), `MonthError.Error()` CLI wording (WindowError precedent `window.go:29-60`; default via `Today` `window.go:89-93`) + `month_test.go` — default month: Oct 1 00:05 and Oct 31 23:59 Toronto → 2026-09, Sep 30 23:59 Toronto (Oct 1 UTC) → 2026-08, Jan 3 2027 → 2026-12; not-a-month rows `2026-9`, `2026`, `2026-09-15`, `""`, `2026-13`, `0000-01` (bound control `0001-01` accepted); not-ended `2026-10`, `2027-01`; bound: 2026-09 refused at Sep 30 23:59:59 local, accepted at Oct 1 00:00 local; example = default month in both kinds
- [ ] Step 4: `summary.go` `(*Server).Summary` — one `s.store.Summary` call (Through = month end, Dates = {prev month end, month end}); extract `anomalies.go:90-133` into a from-charges core and `networth.go:86-120` into a from-read core so `Server.Anomalies`/`Server.NetWorth` call them unchanged; refusal via `readRefusal(ctx, "summary", err)` (`refusal.go:56-62`). Tests: `Test_summary_reads_the_store_once` (fake whose Status/Charges/NetWorth return store B and whose `Summary` returns A then B: output all A, `summaryReads == 1`, other read counters 0); `gotSummary` pins Through 2026-09-30 and Dates {2026-08-31, 2026-09-30}; `Test_summary_anomalies_equal_the_anomalies_read_of_the_month` (store with charges after Sept, vs `Server.Anomalies{Window: month, Now: 2026-10-06}`); `Test_summary_net_worth_equals_the_month_end_read` (vs `Server.NetWorth` over `ParseMonthEndWindow("2026-08","2026-09")`); fault rows: missing store `*store.OpenError` → RefusalError, interrupted ctx → `summary interrupted`, other error unchanged
- [ ] Step 5: `summary.go` recognized-in-month rule (U2/U3/U4) — extract `recurring.go:199-231` into a from-charges core taking today and a keep func over (group charges, run, rule); `Server.Recurring` passes keep-all (existing `recurring_*_test.go` are the byte-identity control); summary passes today = month's last day, window = month, `Series.New` vs month start, `yearlyTotals` over the kept series. Uses `latestRun` `:86-103`, `seriesOf` `:249-281`, `steady` `pricechange.go:50-54`. Folded acceptance tests 02 (Aug/Sept/Oct summaries, now ≥ 2026-11-01), 19 (Nov new, Sept not, now ≥ 2026-12-01), 03 verbatim (Sept), 20, 04 (same section with and without Oct–Nov charges). Arms: `Test_summary_lists_a_series_new_in_the_month_of_its_listing_charge` one row per cadence (weekly 4th, monthly 3rd, quarterly 3rd, annual 2nd), listing charge on M's 1st (new) vs prev month's last day (not), on M's last day (new); `Test_summary_resumption_arms` at month 2026-11, now ≥ 2026-12-01, run Sep/Oct/Nov: prior gap 40 (not new), 45 (not new), 46 (new), prior charges too short for `latestRun` (new), prior run not `steady()` (new), prior run weekly then monthly run (prior run's own `endedAfter` 14 decides); U4 `Test_summary_omits_a_series_recognized_then_broken_in_the_month` (off-cadence charge after the 3rd, then resumes within 45 days next month: not new there either); `Test_recurring_still_lists_a_resumed_series` (control: `Server.Recurring` lists the gap-40 series the summary omits); weekly series ended by month end with listing charge in M → listed (default, pending ruling)
- [ ] Step 6: `networth.go` Change (U5/U9/U10) — `NetWorth` change over its first and last dates, absent when the first date has no row in any currency; value nil = `no rate`; any-row-needs-rate predicate beside `TypeNeedsRate` (`:57-70`); totals from `total`/`nativeNetWorthTotals` (`:182-223`). `summary_change_test.go`: converted — type missing on start counts 0, type `no rate` on start only / end only, type with one converted row and one needing a rate (`TypeNeedsRate` false, any-rate true) → nil, total nil when a day lacks the reporting entry / holds another entry, all-convertible control, exactly one totals entry; native — currency on one day only (`+1,500.00` shape, U10), type entry only where the currency has that type on either day, order CAD, USD, then alphabetical (a GBP row as out-of-domain), type order = `Types()`; start day empty → no Change, start USD-only native → Change shown; `--currency USD` row
- [ ] Step 7: duckstore one-handle read — extract `status.go:45-109`, `charges.go:46-72`, `networth.go:34-82` bodies into helpers over one `ReadDB`; `summary.go` `(*Store).Summary` opens once (`openRead` `duckstore.go:203-219`), runs all three, closes. Tests: `Summary` op added to `rowReads()` (`read_faults_test.go:22-62`; open/query/scan fault and `closes == 1` rows come free); `Test_summary_returns_each_querys_fault` with a `summaryQueries` constant (`:146-164` pattern, `spyReadDB.passQueries` `fakes_test.go:33-65`); contract `summary_read_test.go` `Test_summary_equals_the_separate_reads` (one built store: `Summary` == {`Status`, `Charges(Through)`, `NetWorth(Dates)`}, a charge after Through excluded); Dates row with `0000-12-31` (month 0001-01) — if DuckDB refuses the date, record "default, pending ruling" and stop

### Sweep
- [ ] Step 8: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on every new exported symbol; `internal/report/doc.go` command list adds summary; `go doc ./internal/report Summary` reads as the contract

### Verify
- [ ] Step 9: full verification + `spec-check.py phase4f-summary` → tick SCENARIO-01a, 02, 19, 03, 20, 04 (folds: "delivered by SCENARIO-01a" before the test reference), write `STATE.md`

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- One port call per `Server.Summary`: `SummaryReads.Summary` returns Status, Charges and NetWorth from one open (U1). 01b/10/12/16 render snapshot, dates and findings from `Summary.Status` — never a second `Status`/`NetWorth` call; sync renames the store mid-run.
- `report.Store` stays at 10 interface entries (interfacebloat): new reads join an embedded group (`SummaryReads` holds `Charges` + `Summary`), never Store's top level.
- `SummaryRequest` has no clock: ParseMonth is the only clock reader; recurring's today = the month's last day, so a rerun reproduces every section (U3/U4, S04).
- Anomalies and recurring share one Charges read through the month end. Anomalies equal `quarry anomalies --since M --until M` because baselines are strictly earlier and Checked counts only in-window charges; pinned by an equality test, keep it.
- Change: absent only when the first month end has no row in any currency; nil value = `no rate`; converted totals exactly one entry.
- Refusals use command word `summary` (`summary interrupted`); MCP reuses it as recurring_charges reuses `recurring`.
- Findings counts are not in `Summary`: callers use `report.CountFindings(Summary.Status, ignore, classification)` as status does.
- `MonthError{Kind, Value, Example}` carries parts; `Error()` is the CLI line without `quarry: `; MCP words its own (S16).

**Left unbuilt** — named so nobody assumes it exists:
- `quarry summary` command, heading, Findings composer, Change row rendering — SCENARIO-01b
- `covers_month`, W3a/W3b — SCENARIO-10; `document.Summary` — SCENARIO-12; W4–W6 assembly — SCENARIO-14; `monthly_summary` — SCENARIO-16; empty-section copy — SCENARIO-06

**Traps** — things that look right and are not:
- `Server.Anomalies`/`Server.Recurring` read Charges through *today*: calling them from Summary costs two full Charges reads and the wrong recurring clock.
- `TypeNeedsRate` is false when one row of the type converts; U5 needs the any-row predicate.
- The report fake's `Charges` ignores `Through`; the fake `Summary` must filter, or S04 is vacuous.
- SCENARIO-03 at month 2026-09 is satisfied by the 40-day gap breaking the run (`ruleForGap(40)` fails), not by U2; the resumption arm is pinned at 2026-11.
- gopls here is rooted at `../phase4d-registered`: LSP paths and lines come from that tree. Anchor with Read in this worktree.
- Default, pending ruling: a weekly series whose listing charge is in M but which has ended by M's last day is listed as new (U3 literal).

## Phase report

**Run A (steps 1-2) done; start commit 909cd60.** Acceptance red, planned reason. Failure (assertions, not compile): `summary_test.go:43` expected `[]int64{20000}` actual `[]int64{}` (anomalies); `:44` expected `[]string{"Streaming"}` actual `[]string{}` (recurring); `:45` `"[]" should have 2 item(s), but has 0` (NetWorth.Dates, `require.Len`, stops the test).

Files (all new unless noted):
- `internal/report/summary_test.go` — acceptance test + `summaryNow` (2026-10-06 noon UTC); fixture: Hardware 3 earlier charges 6000 + Sep-14 20000 (anomaly), Streaming monthly ending 2026-09-03 at 2259 (3rd charge in Sept), chequing CAD 100000 (Aug 31) → 150000 (Sep 30). Uses `ParseMonth(new("2026-09"), summaryNow)`.
- `internal/report/month.go` — stubs: `Month{Start, End}` (UTC midnights of first/last day), `MonthErrorKind` (`MonthNotAMonth`, `MonthNotEnded`), `MonthError{Kind, Value, Example}` (`Error()` returns ""), `ParseMonth(*string, time.Time)` (nil = default month). All zero-bodied: step 3 builds them.
- `internal/report/summary.go` — `SummaryRequest{Month, Currency}`, `Summary{Month, Currency, Status, Anomalies, Recurring, NetWorth, Change *NetWorthChange}`, stub `(*Server).Summary`.
- `internal/report/networth.go` — type decls only: `NetWorthChange{Types []NetWorthChangeType, Totals []NetWorthChangeTotal}`; `Value *big.Int` nil = no rate. Pinned by the acceptance test: B3 builds the Change logic, must not reshape these.
- `internal/report/store.go` — new embedded `SummaryReads{Charges, Summary}`; `Charges` left Store's top level (Store still 10 entries). `internal/store/store.go` — `SummaryParams{Through, Dates}`, `Summary{Status, Charges, NetWorth}`.
- `internal/store/duckstore/summary.go` — stub `(*Store).Summary`. `internal/report/fakes_test.go` — real fake `Summary` (filters charges by `Through`, `gotSummary`, `summaryReads`; returns `f.status`, `f.charges`, `f.netWorth`, `f.err`) — B1's once-test needs a fake whose Summary differs from its other reads (add a field then).

Verified: `go build ./...` and `go vet ./internal/... ./cmd/...` clean (cli/mcp fakes embed `report.Store`, so they needed no stub). No lint run yet (Sweep).
Next: B1 = steps 3-4 (ParseMonth/Month; `Server.Summary` + anomalies/networth core extraction). Doc comments on stubs are final contract text; keep.

**Orchestrator rulings 2026-10-06 (pre-dispatch):** (1) a weekly series whose listing charge falls in the month but which has ended by the month's last day IS new in that month (U3 literal) — pin as planned. (2) if DuckDB rejects 0000-12-31 for `--month 0001-01`, the lower bound becomes 0001-02: 0001-01 is refused with the existing not-a-month line (`--month "0001-01" is not a month; use YYYY-MM, such as <default>`), no new copy; record which held in the Phase report.
