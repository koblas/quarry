# Specification: Phase 4f — monthly summary

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: one command a monthly launchd job runs after `quarry sync` that tells David, on one page, what happened last month — data freshness, findings, unusually large charges, recurring charges that became recurring that month, and net worth at the month end with the change (PRD :69 "Scheduled job | CLI | Monthly sync + summary of anomalies and new recurring charges"; Phase 4 milestone :308).

**Out of Scope**: running sync from summary; notifications; any file other than the job's log; a current-month "to date" summary; `--account`; CSV; stored per-run state.

**Business Rules**: "last month" = calendar month before today in the Mac's local zone; summary is read-only over the store (works with Quicken closed); new recurring = first month `quarry recurring` can list it (judged as of the month's last day), unless it resumes an earlier steady series that had not ended; net worth change computed in core; recipe `quarry sync; quarry summary` from launchd with a private log.

## Business Rules & Invariants
- One store handle per command run and per `monthly_summary` call: snapshot row, dates, findings, anomalies, recurring and net worth all read from it (U1).
- `quarry recurring` output and `--json` stay byte-identical; the recognized-in-month filter is summary-only.
- The anomalies section is byte-identical to `quarry anomalies --since <month> --until <month>` on the same store.
- A rerun of a past month on the same store reproduces every section; only the snapshot age changes.
- Rules live in core `report` (Q4); CLI and MCP render the same document.

User decisions (2026-10-06): new = last calendar month; summary read-only; scope adds freshness, findings counts, net worth; stdout + `--json` + launchd recipe; new recurring = recognized in the month; recipe `sync; summary`; scenario changes from the U-rulings accepted.

---

## Triage Brief

- `quarry sync` (internal/cli/sync.go:38-173): prints findings counts only; nothing about anomalies or recurring; needs Quicken open (sync.go:71); warnings exit 0.
- `quarry anomalies` (internal/cli/anomalies.go:22-84; report internal/report/anomalies.go:90): on demand, window default Jan 1 → today, baselines strictly earlier charges (:88); JSON document/anomalies.go:7-35.
- `quarry recurring` (internal/cli/recurring.go:22-89; report internal/report/recurring.go:199): `New = !First.Before(Since)` (:219); state judged as of today (:204-215); minimum charges :32,37,42,47; late charge restarts a run (:95-102); JSON document/recurring.go:7-47.
- `quarry status` (internal/cli/status.go:14-53; render_status.go:21-34): snapshot line, dates line, findings tally.
- `quarry networth` (Phase 4c): month-end history, `ParseMonthEndWindow` (internal/report/window.go:134).
- Findings New/NewlyFixed = latest build (internal/store/duckstore/findings_read.go:14-15; finding.go:207,218); `import_runs` carried across rebuilds; `store.Status` exposes only the latest run.
- Window parsing YYYY-MM = whole month (window.go:95-107); `Today` (window.go:90); clock seam `cli.Env.Now` (internal/cli/run.go:49-59), wired cmd/quarry/run.go:192.
- Each store read opens the file itself (e.g. internal/store/duckstore/networth.go:37); sync replaces by rename (duckstore.go:379).
- MCP tools anomalies (internal/mcp/anomalies.go:26), recurring_charges (internal/mcp/recurring_charges.go:28); caps via capList (cap.go:11-21).
- No scheduling docs in README or SKILL.

**Already exists — do not re-plan:** sync and its exit-code contract; window parsing for a month; `Server.Anomalies`, `Server.Recurring`, `Server.NetWorth` and their documents; recurring's window-relative `new`; findings counts; status's snapshot/dates lines; MCP anomalies/recurring tools; import_runs carry-forward.

**Callers** (anomalies/recurring entry points): `Server.Anomalies` ← internal/cli/anomalies.go:68, internal/mcp/anomalies.go:26 (grep); `Server.Recurring` ← internal/cli/recurring.go:75, internal/mcp/recurring_charges.go:28 (grep); `document.NewAnomalies`/`NewRecurring` ← internal/cli/json_anomalies.go:10, json_recurring.go:10, internal/mcp/anomalies.go:30, recurring_charges.go:32 (grep).

## Product Verdict

Scoping pass: **SHIP WITH CHANGES**, accepted (user confirmed changes 1 and 3): (1) new recurring = recognized in the month with a resumption exclusion, in core; (2) recurring judged as of the month's last day; (3) recipe `quarry sync; quarry summary`; (4) MCP `monthly_summary`; (5) Findings line names "the last sync"; (6) `umask 077` log; (7) `--month` only for ended months. Unruled-outcome pass (after sizing): **SHIP WITH CHANGES**, accepted — U1–U16 below, incl. U2/U3 rule text, U5 `no rate` Change, U7 year-0000 refusal, and scenario edits (S03 reworded, S19/S20 added, S09 0000-01 row).


## Surface & Copy

Ruled by `product-vision` at scoping (2026-10-06), user decisions folded in (recognized-in-month rule; `sync; summary` recipe). Implement verbatim.

### Command

`quarry summary`. Registered in root.go after `anomalies` (alphabetical root listing).

**Short:** `Summarize a month: unusual charges, new recurring charges, net worth and findings`

**Long** (words ruled; wrap and indent the developer's):
```
Summarize one month, last month unless --month names another: how fresh
the store is and how many findings are open, the month's unusually large
charges, the recurring charges new in the month, and net worth at the end
of the month beside the end of the month before. It is meant to run once
a month after quarry sync, for example from launchd:

  quarry sync; quarry summary

summary only reads quarry's store; it never runs sync and never looks at
Quicken, so it works with Quicken closed. When the store was built from a
snapshot taken before the month ended, summary warns, since transactions
from the rest of the month are missing; open your Quicken file, run quarry
sync, then run summary again.

Unusually large charges are those quarry anomalies lists for the month,
for example quarry anomalies --since 2026-09 --until 2026-09.

A recurring charge is new in the first month quarry recurring can list
it: usually the month of its third monthly or quarterly charge, fourth
weekly charge or second yearly charge, later when its amount changed too
often before then. A recurring charge that starts again after a charge
off schedule is new only if it had ended first, with no charge for 14
days (weekly), 45 days (monthly), 120 days (quarterly) or 400 days
(yearly). Recurring charges are judged as of the month's last day, so
later charges never change a past month's summary.

Net worth is what quarry networth lists at the two month ends, for
example quarry networth --since 2026-08 --until 2026-09; Change is the
difference. In CAD or USD it includes changes in the exchange rate.

Findings counts the findings open in the store; new and fixed are what
the last sync found, whenever it ran.

Months begin and end at midnight in this Mac's time zone. Amounts are in
CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
With --currency native, CAD and USD are listed separately, never added
together.
```
Example months in the Long are literal text, not computed.

**Example:**
```
  quarry summary
  quarry summary --month 2026-08 --currency USD
  quarry summary --json
```

**Flags as rendered:**
```
      --currency code    show amounts in currency code: CAD, USD, or native for each account's own (default reporting.currency in the config file, else CAD)
      --month YYYY-MM    summarize month YYYY-MM instead of last month; it must have ended
```
`--currency` reuses `reportCurrencyHelp` verbatim (currency.go:15). `--month` source: "summarize month `YYYY-MM` instead of last month; it must have ended". `--json` is the root persistent flag. No `--account`, no `--csv`.

### What "last month" means

Default month = the calendar month before `Today(now)`'s month, in now's own zone (the Mac's local zone; window.go:90 `Today`, cmd/quarry/run.go:192 `time.Now`). Oct 1 00:05 or Oct 31 23:59 → September 2026; Jan 3 2027 → December 2026. Month start = local midnight on the 1st. "Ended" = `Today(now)` is in a later month. A snapshot covers the month when its `taken_at` instant is at or after local midnight beginning the 1st of the next month.

### Rules owned by core `report` (Q4)

- **New recurring charge:** a series listed by recurring as of the month's last day is new in the month when its listing charge (the earliest charge, at or past its cadence's minimum count, at which the run so far is steady) falls in the month, unless the group's charges before the run's first charge form a steady series (`latestRun` ok, `steady()`) whose last charge is no more than that series' ended-after days (14/45/120/400) before the run's first charge. Cites: recurring.go:32,37,42,47 (minimums), :88-103 (run), :274 (ended comparison), pricechange.go:51 (steady). (U2/U3)
- **Recurring judged as of the month's last day:** the recurring read for the summary uses the month's last day as its clock (removes the today-dependence at recurring.go:204-215), so a rerun on the same store reproduces the section.
- **Net worth Change** = end-month-end minus start-month-end, computed in core.

### Text layout, stdout (CAD example)

```
Summary of September 2026 (2026-09-01 to 2026-09-30), amounts in CAD

Snapshot  20261001T130512Z, taken 2026-10-01 09:05 EDT (5 days ago)
Dates     2003-01-02 to 2026-09-30
Findings  14 open, 5 ignored; the last sync found 3 new and 2 fixed; run quarry findings to list them

Unusually large charges 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD

Date        Account        Payee       Category        Amount   Usual  Times  Compared with
2026-09-14  Visa Infinite  Home Depot  Home:Repairs  1,284.00  212.50   6.0x  payee, 41 earlier

412 charges checked; 37 had too little history to judge

Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD

Payee    Currency  Every  Amount  Per year  First       Last        Status  Price changes
Crave    CAD       month   22.59    271.08  2026-07-03  2026-09-03  active
Total    CAD                        271.08

Net worth at each month end 2026-08-31 to 2026-09-30, amounts in CAD

Month end   chequing   credit card    brokerage         Total
2026-08-31  12,345.67   -2,100.00   410,000.00   420,245.67
2026-09-30  13,000.00   -1,880.40   415,312.50   426,432.10
Change        +654.33     +219.60    +5,312.50    +6,186.43
```

- **Heading:** `Summary of <Month YYYY> (<since> to <until>)` then `, amounts in <CUR>` in CAD/USD, nothing in native (`windowCaption` precedent, render_table.go:54-61). Month name from `time.Month.String()` (English).
- **Snapshot / Dates rows:** reuse status's `snapshotLine` and `datesLine` (render_status.go) verbatim, same `%-10s` label column.
- **Findings row (new composer; status's and sync's phrases untouched):** `<N> open` or `none open`; then `, <J> ignored` when the ignore list was read and J>0; then `; the last sync found <a> new and <b> fixed` / `; the last sync found <a> new` / `; the last sync found <b> fixed` (whichever nonzero; nothing when both 0); then `; run quarry findings to list them` when open>0. Ignored unknown (config unreadable) → no ignored clause (status.go:46-50).
- **Anomalies section:** exactly `renderAnomalies` for the month window, no account filter, same currency — byte-identical to `quarry anomalies --since <month> --until <month>` on the same store. Empty: header row replaced by `No unusually large charges.`, then the blank line and the footer (e.g. `0 charges checked`).
- **Recurring section:** `renderRecurring` rows over the core-filtered list, Total row per currency (core `yearlyTotals` over the filtered series). Caption `windowCaption("Recurring charges new", …)` → `Recurring charges new 2026-09-01 to 2026-09-30 in all accounts, amounts in CAD`. Status cell keeps the existing `, new` suffix for series first charged in the month (recurring.go:219 against the month window). Empty: caption, blank line, `No new recurring charges.`
- **Net worth section:** exactly `renderNetWorthHistory` for month ends since = month before, until = month (`ParseMonthEndWindow`, window.go:134), plus a Change row from core. Converted mode: `Change` in the Month end column, a signed cell per type and the signed total. Native: one `Change` row per currency with Currency column filled. Sign: `+6,186.43`, `-219.60`, `0.00` (`signedTenths` precedent, render_recurring.go:60). A type with no row on one day counts 0.00 that day. A cell is `no rate` when either day's cell is `no rate`. Empty (no row either day): caption, blank line, `No account has a balance on 2026-08-31 or 2026-09-30.` Start day empty, end day not (first month of data): Change row omitted, then blank line and `No change shown: no account has a balance on 2026-08-31.`
- **Separators:** one blank line between sections; output ends with exactly one newline.
- **Data-bearing text:** text cells through `escapeCell` (render_table.go:76-79); `--json` carries strings raw; NULL payee as in anomalies (`payeeLabel("")`), JSON `payee` null. No CSV.
- **Paths:** none in stdout text; stderr `~` form, `--json` warnings absolute.

### `--json` document (key order as listed)

```json
{
  "month": "2026-09",
  "since": "2026-09-01",
  "until": "2026-09-30",
  "currency": "CAD",
  "snapshot": {"id": "20261001T130512Z", "taken_at": "2026-10-01T13:05:12Z", "covers_month": true},
  "dates": {"first": "2003-01-02", "last": "2026-09-30"},
  "findings": {"open": 14, "ignored": 5, "fixed": 2, "new": 3, "newly_fixed": 2},
  "anomalies": {"checked": 412, "not_judged": 37, "charges": [ /* document.Anomaly, unchanged */ ]},
  "recurring": {"series": [ /* document.RecurringSeries, unchanged */ ], "totals": [ /* document.RecurringTotal */ ]},
  "net_worth": {
    "dates": [ /* document.NetWorthDate x2, unchanged */ ],
    "changes": {
      "types": [{"type": "chequing", "currency": "CAD", "value": "654.33"}],
      "totals": [{"currency": "CAD", "value": "6186.43"}]
    }
  },
  "warnings": []
}
```
`taken_at` via `nullTimestamp` (status.go:155-166); `covers_month` null when `taken_at` null. `findings` = `document.StatusFindings` verbatim (`ignored` null when config unreadable). Arrays `[]`, never null. `changes.value` null where text says `no rate`; `types[].currency` = reporting currency (CAD/USD) or account currency (native); no start balance → `{"types":[],"totals":[]}`. Money as string. Same document with and without a TTY.

### Exit codes and refusals (stderr one line, `quarry: ` prefix)

Check order: args → `--month` → config/currency → store.

| Outcome | Line | Exit |
|---|---|---|
| Positional arg | `quarry: summary takes no arguments` (errors.go:17) | 2 |
| `--currency EUR` | `quarry: --currency must be CAD, USD or native` | 2 |
| `--month` not exactly YYYY-MM (`2026-9`, `2026`, `2026-09-15`, `""`, `2026-13`) | `quarry: --month "2026-9" is not a month; use YYYY-MM, such as 2026-09` — example = the default month, value %q | 2 |
| `--month` current or later | `quarry: --month 2026-10 has not ended; summary covers whole months, so pass 2026-09 or earlier` | 2 |
| Unknown flag | cobra's existing line | 2 |
| Config unreadable, no `--currency` | existing `quarry: cannot read ~/…/config.toml: <reason>; fix the file and run the command again` | 1 |
| Config value bad, no `--currency` | existing `reporting.currency must be CAD, USD or native, got "EUR"; fix the file and run the command again` line | 1 |
| Config unreadable, `--currency` given | no refusal; warning W2; output printed | 0 |
| No store | existing `quarry: no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it` | 1 |
| Store built by another version / unreadable | existing refusal.go:81 / :83 lines | 1 |
| stdout write fails | `quarry: cannot write the result to stdout: <err>` (output.go:54-59) | 1 |
| Interrupt | as every read command | 1 |
| Success, with or without warnings | stdout document, then warnings | 0 |

### Warnings (stderr `quarry: warning: <line>`; `--json` `warnings[]` in this order; identical lines deduplicated)

1. W1 config warnings: existing, printed before stdout (`printConfigWarnings`), absolute paths in JSON.
2. W2: `CannotTellChoices(problem)` existing text (status.go:99-102), only when `--currency` given and config unreadable.
3. W3a snapshot predates month end: `the store was built from a snapshot taken 2026-09-28 14:02 EDT, before September 2026 ended, so transactions from the rest of the month are missing; open your Quicken file, run quarry sync, then run quarry summary again` — with `--month` the tail is `then run quarry summary --month 2026-08 again`. Time via `takenLayout`, local zone (render_status.go:16).
4. W3b taken time unknown: `cannot tell whether the store holds all of September 2026: its snapshot's manifest does not record when it was taken; open your Quicken file and run quarry sync to take a new snapshot`
5. W4 anomalies: unconverted line from `AnomaliesWarnings` (charges noun, or `noRatesWarning`).
6. W5 recurring: unconverted line from `RecurringWarnings` (series noun, or `noRatesWarning`, deduplicated against W4).
7. W6 net worth: `rateWarnings` with `NativeFlag` (networth_rate_warnings.go).

Suppressed (body says it): the empty-window lines of `AnomaliesWarnings` and `RecurringWarnings`; `emptyNetWorthWarnings`. `leftOutWarnings` cannot fire (no account filter). Names from the user's file inside any new warning go through `%q`.

### Edge-case rows

| Input | Heading/rows | Anomalies | Recurring | Net worth | Warnings | Exit |
|---|---|---|---|---|---|---|
| No store | none | – | – | – | no-store refusal | 1 |
| Quicken closed (summary itself) | normal | normal | normal | normal | none | 0 |
| Job: sync failed, store from Sept 28 | normal; Snapshot row Sept 28 | from store | from store | from store | W3a | 0 |
| Snapshot `taken_at` null | `Snapshot <id>, time taken not recorded in its manifest` | normal | normal | normal | W3b | 0 |
| Snapshot older than whole month | normal | likely `No unusually large charges.` / `0 charges checked` | likely none | Sept 30 = Aug 31 balances | W3a | 0 |
| Store has no transactions | `Dates     no transactions` | `No unusually large charges.` + `0 charges checked` | `No new recurring charges.` | `No account has a balance on … or ….` | none | 0 |
| First month of data | normal | mostly not judged | none | table, no Change row, `No change shown: …` | none | 0 |
| `--month` before all data | normal | empty line | empty line | empty-balance line | none | 0 |
| Empty month | normal | `No unusually large charges.` / `0 charges checked` | `No new recurring charges.` | normal | none | 0 |
| Late bill restarts an old series | – | – | not listed (resumption) | – | none | 0 |
| No investment accounts | normal | normal | normal | bank/credit columns only | none | 0 |
| No rates, CAD | normal | USD rows own currency, code-prefixed | ditto | `no rate` cells / Change | W4/W5 (dedup), W6 | 0 |
| Month ends before first rate | normal | normal | normal | `no rate` cells, Change `no rate` | W6 (`2 month ends` form) | 0 |
| `--currency USD` | `, amounts in USD` | USD | USD | USD + FX in Change | as applicable | 0 |
| `--currency native` | no amounts clause | own currency | own currency | Currency column, Change per currency | none for rates | 0 |
| Findings none open, none new/fixed | `Findings  none open` | | | | | 0 |
| Findings none open, 2 fixed | `Findings  none open; the last sync found 2 fixed` | | | | | 0 |
| Config unreadable with `--currency` | no ignored clause; JSON `ignored` null | | | | W2 | 0 |
| Closed accounts | `accountLabel` | included | included | counted with balance | none | 0 |
| Accounts not in reports | | left out | left out | left out | none | 0 |
| Run on the 1st at 00:05 local | September | | | | W3a unless snapshot after 00:00 Oct 1 | 0 |
| `--month` current/future | refusal | | | | | 2 |

### MCP tool `monthly_summary`

- Input: `month` (string `YYYY-MM`, optional, default last month at call time via `s.now()`), `currency` (`CAD`/`USD`/`native`, exact case, optional, config default as other tools).
- Description: `Summarize one month (default last month): data freshness, findings counts, unusually large charges, recurring charges new in the month, and net worth at the month end beside the month before, with the change. The same document quarry summary --json prints. Read-only; never syncs.`
- isError: `month "2026-9" is not a month; use YYYY-MM, such as 2026-09` and `month 2026-10 has not ended; monthly_summary covers whole months, so pass 2026-09 or earlier`. Config and store refusals use the existing MCP wording.
- Warnings as above; W6 advice `NativeParameter`; W3a/W3b tail `then call monthly_summary again`.
- Caps: `anomalies.charges` and `recurring.series` through `capList` (cap.go:11-21), nouns `charges` / `series`, advice `pass an earlier or later month`.

### launchd recipe

Single source: new `plugin/skills/quarry/references/monthly-summary.md`; README gets a short section linking to it.

```
# Run quarry summary every month

1. Find quarry's full path with `command -v quarry` (for example /Users/you/go/bin/quarry).
   launchd does not use your shell's PATH.
2. Save this as ~/Library/LaunchAgents/com.github.koblas.quarry.summary.plist, with both
   /Users/you/go/bin/quarry replaced by that path:

<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.github.koblas.quarry.summary</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/sh</string>
    <string>-c</string>
    <string>umask 077; mkdir -p "$HOME/Library/Logs/quarry"; { /Users/you/go/bin/quarry sync; /Users/you/go/bin/quarry summary; } >>"$HOME/Library/Logs/quarry/summary.log" 2>&amp;1</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Day</key><integer>1</integer>
    <key>Hour</key><integer>9</integer>
    <key>Minute</key><integer>0</integer>
  </dict>
</dict>
</plist>

3. Load it:      launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.github.koblas.quarry.summary.plist
4. Try it now:   launchctl kickstart gui/$(id -u)/com.github.koblas.quarry.summary
   then read ~/Library/Logs/quarry/summary.log.
5. To stop it:   launchctl bootout gui/$(id -u)/com.github.koblas.quarry.summary

quarry sync needs Quicken running with your file open. If Quicken was closed when the job ran,
the log shows sync's error line, then a summary that warns its snapshot was taken before the
month ended; open your Quicken file and run `quarry sync; quarry summary` in a terminal.
If the Mac is asleep at 9:00 on the 1st, launchd runs the job when it wakes.
The log holds your payees, amounts and net worth; umask 077 keeps it readable only by you.
```
In the XML `&` is `&amp;`. Verify by hand in the reference scenario before shipping as claims: `$HOME` set in a LaunchAgent's `sh -c`; whether TCC blocks reading `~/Documents/*.quicken` from a launchd job; the `bootstrap`/`kickstart` lines on macOS 26.

### Changes to existing surfaces

- PRD CLI table (:160-178): add `| quarry summary | One month (default last month): freshness, findings counts, unusually large charges, recurring charges new in it, net worth at its end and the change; read-only, for a monthly job after sync |`.
- PRD MCP table (:196-): add `monthly_summary`.
- PRD Decisions: add `Monthly summary: quarry summary reads the store only; "last month" is the calendar month before today in local time; a recurring charge is new in the first month quarry recurring can list it, unless it starts again after an off-schedule charge before its earlier series had ended; recurring is judged as of the month's last day; the job is quarry sync; quarry summary from launchd; no stored state.`
- PRD :69 unchanged.
- root.go:7-10 doc comment: add `summary`.
- SKILL.md §4 row: `| What happened last month; a monthly summary | quarry summary --json (--month YYYY-MM for an earlier month) |`.
- SKILL.md §9: add `monthly_summary` to the tool list.
- SKILL.md §10: `- [Monthly summary job](references/monthly-summary.md): a launchd job that runs quarry sync and quarry summary on the 1st of each month.`
- README: new `## Run a monthly summary`: `quarry summary prints last month's unusual charges, new recurring charges, net worth change and findings. To get it every month, follow plugin/skills/quarry/references/monthly-summary.md.`
- Nothing becomes false: `quarry recurring` Long, `recurring --json` and SKILL's "new marks series that started in the period" unchanged.

### Behaviour citations

Read-only: `openReport` (output.go:10-17) only store access; acceptance test pins that no Quicken path is resolved. Anomalies stable on rerun: anomalies.go:88, :96. Minimum charges: recurring.go:32,37,42,47. Restart on a late charge: recurring.go:95-102. Today-dependence removed: recurring.go:204-215. Month ends: window.go:134. Local day: window.go:90, run.go:192. Findings new/fixed = latest build: finding.go:207,218; findings.go:111. Mac-asleep sentence: `man launchd.plist`, StartCalendarInterval.

### Rulings after the sizing pass (U1–U16, product-vision 2026-10-06)

- **U1** One store handle per command run and per `monthly_summary` call; all sections read from it. No copy; failure to deliver is a correctness defect.
- **U2** Resumption: a run R resumes an earlier series (so is NOT new) only when (1) `latestRun` over the group's charges dated before `R[0]` is ok (recurring.go:88), (2) that earlier run is `steady()` (pricechange.go:51), and (3) `daysBetween(prev.Last, R[0].Date) <= prev.rule.endedAfter` (recurring.go:274 comparison) using the earlier run's cadence rule.
- **U3** A run R is new in month M when (a) recurring as of M's last day lists R (`latestRun` ok, `steady()`, runs during M, not a resumption) and (b) R's listing charge falls in M — the **earliest** `R[k]` with `k+1 >= rule.minCharges` whose prefix `R[0..k]` is `steady()`.
- **U4** Recognized in M, run broken later in M → not listed in M, and not later while it resumes within its ended-after days. Edge row: `Recognized mid-month, run broken by month end | – | – | not listed this month or later while it resumes within its ended-after days | – | none | 0`.
- **U5** Converted Change type cell = `no rate` (JSON null) when either day has **any** row of that type needing a rate (new core predicate, e.g. `TypeNeedsAnyRate`; `TypeNeedsRate` networth.go:58-70 is not enough). Total cell `no rate` (null) when either day's `Totals` (networth.go:183-203) lacks the reporting-currency entry or holds any other entry. No warning beyond W6. Converted `changes.totals[]` is exactly one entry `{"currency": "<reporting>", "value": … | null}`. Native has no rate case.
- **U6** End day without a row while start day has one: unreachable — `v_balances_daily` carries each account to `current_date` (internal/store/duckstore/schema.go:322) and summary months have ended. No copy; a branch, if any, marked `// unreachable: v_balances_daily carries each account to current_date and summary months have ended`.
- **U7** `--month` year must be 0001 or later. `0000-01` → CLI `quarry: --month "0000-01" is not a month; use YYYY-MM, such as 2026-09` (exit 2); MCP isError `month "0000-01" is not a month; use YYYY-MM, such as 2026-09`. Heading month formatted with `time.Format("January 2006")`.
- **U8** JSON `dates` with no transactions = `document.StatusDates` verbatim (document/status.go:43-47): `{"first": null, "last": null}`; text `Dates     no transactions`.
- **U9** `changes.types[]`: currency first (CAD, USD, then the rest alphabetically — `nativeNetWorthTotals` order, networth.go:207), then type in `NetWorth.Types()` order. Converted: one currency, every type. Native: an entry only when that currency has a row of that type on either day; text cell blank for an absent entry. `changes.totals[]`: converted one entry (U5); native one per currency present on either day, same order.
- **U10** Native: a currency on only one month end counts the other day 0.00 — e.g. `Change      USD     +1,500.00   +1,500.00`. "No change shown: …" fires only when the start day has no row in any currency.
- **U11** MCP `month` description: `Month to summarize: YYYY-MM, a month that has ended. Defaults to last month.`
- **U12** `mcp --help` Tools line (internal/cli/mcp.go:38-40), tool registered last:
  ```
  Tools: describe_schema, query, sync_status, data_quality, spending,
  cash_flow, recurring_charges, anomalies, search_transactions, holdings,
  net_worth, acb, monthly_summary.
  ```
- **U13** SKILL §9 (SKILL.md:99): `` `sync_status` for section 1, `spending`, `cash_flow`, `recurring_charges`, `anomalies`, `search_transactions`, `holdings`, `net_worth`, `acb`, `monthly_summary`, `data_quality` for the commands in section 4, `describe_schema` and `query` for section 5. `` (rest unchanged).
- **U14** PRD MCP table row, after `holdings`, before `sync_status`: `| \`monthly_summary\` | One month (month, default last month): freshness, findings counts, unusually large charges, recurring charges new in it, and net worth at its end beside the month before with the change; the document \`quarry summary --json\` prints |`
- **U15** references/monthly-summary.md line 3: `Use this when the user wants last month's summary every month without asking: a launchd job that runs \`quarry sync\`, then \`quarry summary\`, on the 1st and writes both to a log only they can read. Show the user these steps; write or load the LaunchAgent only when they ask you to.` Test row (cmd/quarry/run_skill_references_test.go:60): `{"monthly-summary.md", []string{"quarry sync", "quarry summary", "launchd", "StartCalendarInterval", "umask 077", "launchctl bootstrap", "launchctl bootout", "command -v quarry", "only when they ask you to"}}`.
- **U17** (orchestrator, from the SCENARIO-18 reference check, 2026-10-06) U2 condition (1) is the intent "an earlier steady series that had not ended", not the exact prefix: the earlier series is the latest `steady()` run `latestRun` finds over the group's charges before `R[0]` with any trailing off-schedule charges dropped (search back no further than that run's ended-after days before `R[0]`); (2) and (3) unchanged. Off-schedule charges between the runs do not end the earlier series. Real-file case: a monthly run whose last charge was 30 days before `R[0]` (ended-after 45), with one off-schedule charge between, was listed new. No copy change.
- **U16** README `## Run a monthly summary` between `## Use quarry with Claude Code` and `## Credits`, path as a link `[plugin/skills/quarry/references/monthly-summary.md](plugin/skills/quarry/references/monthly-summary.md)`.
- Recurring section rows keep recurring's own per-row `new` / `, new` suffix (series first charged in the month), as ruled at scoping.

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01a The report server summarizes a month
  Given the store from SCENARIO-01b
  When the September 2026 summary is read from the report server
  Then it holds the month's anomalies, the recurring series recognized in September, and net worth at both month ends with the change

Scenario: SCENARIO-01b Summary of last month
  Given a store synced after September 2026 ended, with charges, a recurring subscription and balances
  When the user runs `quarry summary` on 2026-10-06
  Then stdout shows the September heading, snapshot/dates/findings rows, the month's unusually large charges, the recurring charges new in September and net worth at Aug 31 and Sep 30 with the change, and exit 0

Scenario: SCENARIO-02 A subscription is new in the month quarry first recognizes it
  Given a monthly subscription first charged in July whose third charge falls in September
  When the user runs `quarry summary --month 2026-09`
  Then it is listed under recurring charges new in September, and not in the August or October summary

Scenario: SCENARIO-19 A price change before quarry can list a subscription moves the month it is new
  Given a monthly subscription first charged in July 2026 whose second charge was 10% higher
  When the user runs `quarry summary --month 2026-11`
  Then it is listed as new in November, and the September summary does not list it

Scenario: SCENARIO-03 A late bill does not make an old subscription new
  Given a monthly subscription charged for two years whose September charge came 40 days after August's
  When the user runs `quarry summary --month 2026-09`
  Then it is not listed as a new recurring charge

Scenario: SCENARIO-20 A subscription resumed after it ended is new again
  Given a monthly subscription last charged January 2025 and charged again from July 2026
  When the user runs `quarry summary --month 2026-09`
  Then it is listed under recurring charges new in September

Scenario: SCENARIO-21 A subscription that kept charging through a stray charge is not new
  Given a monthly subscription charged October to December 2025, one off-schedule charge from the same payee 21 days after the December charge, and the subscription charged monthly again from 30 days after the December charge at a new price
  When the user runs `quarry summary --month 2026-03`
  Then it is not listed under recurring charges new in March

Scenario: SCENARIO-04 A past month's recurring charges do not change with later charges
  Given a store holding charges after September 2026
  When the user runs `quarry summary --month 2026-09`
  Then the recurring section is the same as on a store holding only charges up to September 30

Scenario: SCENARIO-05 Net worth change between month ends
  Given balances at the ends of August and September 2026, one account type with no balance in August
  When the user runs `quarry summary --month 2026-09`
  Then the net worth table shows both month ends and a signed Change row per type and in total, the missing day counting as 0.00

Scenario: SCENARIO-06 First month of data shows no change
  Given a store whose first balances are in September 2026
  When the user runs `quarry summary --month 2026-09`
  Then the net worth table shows September 30 only and the line "No change shown: no account has a balance on 2026-08-31."

Scenario: SCENARIO-07 A quiet month
  Given a month with no unusually large charge and no new recurring charge
  When the user runs `quarry summary`
  Then the sections say "No unusually large charges." and "No new recurring charges." and exit 0

Scenario: SCENARIO-08 Findings line names the last sync
  Given open, ignored, new and fixed findings in the store
  When the user runs `quarry summary`
  Then the Findings row reads "<n> open, <j> ignored; the last sync found <a> new and <b> fixed; run quarry findings to list them" in each ruled variant

Scenario Outline: SCENARIO-09 --month that cannot be summarized is refused
  Given today is 2026-10-06
  When the user runs `quarry summary --month <value>`
  Then stderr is "<line>" and the exit code is 2
  Examples:
    | value      | line |
    | 2026-9     | quarry: --month "2026-9" is not a month; use YYYY-MM, such as 2026-09 |
    | 2026-09-15 | quarry: --month "2026-09-15" is not a month; use YYYY-MM, such as 2026-09 |
    | 0000-01    | quarry: --month "0000-01" is not a month; use YYYY-MM, such as 2026-09 |
    | 2026-10    | quarry: --month 2026-10 has not ended; summary covers whole months, so pass 2026-09 or earlier |
    | 2027-01    | quarry: --month 2027-01 has not ended; summary covers whole months, so pass 2026-09 or earlier |

Scenario: SCENARIO-10 Snapshot taken before the month ended is warned about
  Given the store was built from a snapshot taken 2026-09-28
  When the user runs `quarry summary` on 2026-10-06
  Then the summary prints and stderr warns the snapshot predates the end of September, telling the user to sync and run summary again, exit 0

Scenario: SCENARIO-11 Snapshot with no recorded time is warned about
  Given the store's snapshot manifest does not record when it was taken
  When the user runs `quarry summary`
  Then the Snapshot row says the time was not recorded and stderr carries the ruled cannot-tell warning, exit 0

Scenario: SCENARIO-12 Machine-readable summary
  Given the store from SCENARIO-01b
  When the user runs `quarry summary --json`
  Then stdout is the ruled document with keys month, since, until, currency, snapshot, dates, findings, anomalies, recurring, net_worth, warnings, in that order

Scenario: SCENARIO-13 Summary in another currency
  Given CAD and USD accounts and exchange rates
  When the user runs `quarry summary --currency native`
  Then each section lists CAD and USD separately and net worth shows one Change row per currency

Scenario: SCENARIO-14 Missing exchange rates are warned about once
  Given USD accounts and no exchange rates in the store
  When the user runs `quarry summary`
  Then unconvertible amounts show "no rate" and stderr carries one rates warning per kind, deduplicated, in the ruled order

Scenario: SCENARIO-15 Summary with no store refuses
  Given no store exists
  When the user runs `quarry summary`
  Then stderr is the existing no-store line and the exit code is 1

Scenario: SCENARIO-16 Claude asks for last month's summary
  Given the store from SCENARIO-01b
  When an MCP client calls `monthly_summary`
  Then it gets the same document `quarry summary --json` prints

Scenario: SCENARIO-17 Docs describe the monthly job
  Given the shipped skill, references and README
  When a reader looks for how to run a monthly summary
  Then SKILL §4/§9/§10, references/monthly-summary.md (launchd recipe) and the README section carry the ruled copy

Scenario: SCENARIO-18 Reference check on a real month
  Given a scratch copy of the newest real snapshot
  When the orchestrator runs `quarry summary` for a past month and the launchd job by hand
  Then each section matches `quarry anomalies`, `quarry recurring`, `quarry networth` for that month and the job writes a 0600 log
```

---

## Sizing

Architect sizing pass (2026-10-06). Order: 01a → 01b → 06 → 10 → 12 → 14 → 16 → 17 → 18.

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01a | OWNS A RUN (opus), 3–4 batches, report (+ duckstore one-handle read): month model and parse (not a month / not ended, year ≥ 0001); `Server.Summary` composing status, anomalies, recurring and two month-end net worth reads from ONE store handle (U1; embedded port interface, fakes, contract test; fake-store test where a second read returns different data); recognized-in-month rule (U2/U3/U4; month-end clock); Change both modes (U5, U9, U10). Absorbs 02, 19, 03, 20, 04 (acceptance tests at `Server.Summary`; their clocks ≥ the month after). |
| SCENARIO-01b | OWNS A RUN (opus), 4 batches, cli + cmd/quarry tables: command, Short/Long/Example, `--month` (both refusal kinds) and `--currency`; root registration and every all-commands table row (run_read_refusals_test.go:45,87; run_read_usage_test.go:16,82; run_usage_test.go:211,248; root Available Commands pin); heading, Snapshot/Dates rows; Findings composer (all clauses, W2, unknown ignored); the three sections; Change row both modes. Absorbs 05, 08, 09, 13, 15. Sweep: PRD CLI row, root.go doc comment. |
| SCENARIO-02 | FOLD into 01a (Nth-charge arms). |
| SCENARIO-19 | FOLD into 01a (listing charge = earliest steady prefix, U3). |
| SCENARIO-03 | FOLD into 01a (resumption arm within ended-after, U2). |
| SCENARIO-20 | FOLD into 01a (resumption past ended-after is new, U2). |
| SCENARIO-04 | FOLD into 01a (month-end clock). |
| SCENARIO-05 | FOLD into 01b. |
| SCENARIO-06 | OWNS A RUN (sonnet), 2 batches, cli (+ report if needed): empty lines (no change shown, no balance on either, no unusually large charges + footer, no new recurring); edge rows no transactions, `--month` before all data, empty month. Absorbs 07. |
| SCENARIO-07 | FOLD into 06. |
| SCENARIO-08 | FOLD into 01b. |
| SCENARIO-09 | FOLD into 01b (+ out-of-domain rows `2026-13`, `""`, `2026`, check-order pins). |
| SCENARIO-10 | OWNS A RUN (sonnet), 2 batches, report + document + cli: `covers_month` (local midnight bounds, 1 ns before, zero taken_at, DST zone); W3a both tails (surface-parameterized tail for MCP); W3b; slot after W1/W2. Absorbs 11. |
| SCENARIO-11 | FOLD into 10. |
| SCENARIO-12 | OWNS A RUN (sonnet), 3 batches, document + cli: `document.Summary` key order, nested documents reused, `changes`, `covers_month`, findings verbatim, read-back test, arrays never null, absolute paths in `warnings[]`; crosses cells from 06/08/11/13. |
| SCENARIO-13 | FOLD into 01b. |
| SCENARIO-14 | OWNS A RUN (sonnet), 3 batches, document + cli: `no rate` Change cell/null (U5); W4/W5 dedupe; W6 "2 month ends"; full W1–W6 order in text and `--json`. |
| SCENARIO-15 | FOLD into 01b (row in run_read_refusals_test.go:45). |
| SCENARIO-16 | OWNS A RUN (sonnet), 2–3 batches, mcp: tool, description, `month` (U11), isError lines, W3 tail, W6 `NativeParameter`, caps; SKILL §9 (U13), `mcp --help` Tools line (U12), PRD MCP row (U14). |
| SCENARIO-17 | OWNS A RUN (sonnet), 2 batches, docs + tests: references/monthly-summary.md (U15 + recipe), SKILL §4 and §10, README (U16), PRD Decisions. |
| SCENARIO-21 | OWNS A RUN (sonnet) — added by the SCENARIO-18 reference check (U17); one core function (`resumes`) plus its unit and acceptance tests. |
| SCENARIO-18 | Orchestrator reference check (no architect/developer); also verifies the launchd claims ($HOME in `sh -c`, TCC on ~/Documents, bootstrap/kickstart on macOS 26) and times summary on the real store. |

**Traps:** one store handle (sync renames the store mid-run); two full Charges reads (share or time them); `quarry recurring` must not change; test clocks for later months (02 needs now ≥ 2026-11-01, 19 ≥ 2026-12-01); all-commands tables listed under 01b.

## BDD Acceptance Progress
- [x] SCENARIO-01a: The report server summarizes a month — `internal/report/summary_test.go` `Test_summary_holds_the_months_anomalies_new_recurring_and_net_worth_change`
- [x] SCENARIO-02: A subscription is new in the month quarry first recognizes it — delivered by SCENARIO-01a — `internal/report/summary_recurring_test.go` `Test_summary_lists_a_subscription_as_new_in_the_month_of_its_third_charge`
- [x] SCENARIO-19: A price change before quarry can list a subscription moves the month it is new — delivered by SCENARIO-01a — `internal/report/summary_recurring_test.go` `Test_summary_lists_a_subscription_with_an_early_price_change_as_new_in_its_first_steady_month`
- [x] SCENARIO-03: A late bill does not make an old subscription new — delivered by SCENARIO-01a — `internal/report/summary_recurring_test.go` `Test_summary_does_not_list_a_late_bill_of_an_old_subscription_as_new`
- [x] SCENARIO-20: A subscription resumed after it ended is new again — delivered by SCENARIO-01a — `internal/report/summary_recurring_test.go` `Test_summary_lists_a_subscription_resumed_after_it_ended_as_new`
- [x] SCENARIO-04: A past month's recurring charges do not change with later charges — delivered by SCENARIO-01a — `internal/report/summary_recurring_test.go` `Test_summary_recurring_ignores_charges_after_the_month`
- [x] SCENARIO-01b: Summary of last month — `cmd/quarry/run_summary_test.go` `Test_run_summary_prints_last_months_summary`
- [x] SCENARIO-05: Net worth change between month ends — delivered by SCENARIO-01b — `cmd/quarry/run_summary_change_test.go` `Test_run_summary_counts_a_type_without_a_balance_on_the_first_month_end_as_zero_in_the_change`
- [x] SCENARIO-08: Findings line names the last sync — delivered by SCENARIO-01b — `internal/cli/summary_test.go` `Test_summary_findings_row_names_the_last_sync`
- [x] SCENARIO-09: --month that cannot be summarized is refused — delivered by SCENARIO-01b — `cmd/quarry/run_summary_refusals_test.go` `Test_run_summary_refuses_a_month_it_cannot_summarize`
- [x] SCENARIO-13: Summary in another currency — delivered by SCENARIO-01b — `cmd/quarry/run_summary_change_test.go` `Test_run_summary_lists_cad_and_usd_separately_with_native`
- [x] SCENARIO-15: Summary with no store refuses — delivered by SCENARIO-01b — `cmd/quarry/run_read_refusals_test.go` `Test_run_read_commands_refuse_when_there_is_no_store`
- [x] SCENARIO-06: First month of data shows no change — `cmd/quarry/run_summary_empty_test.go` `Test_run_summary_shows_no_change_in_the_first_month_of_data_and_warns_when_the_snapshot_time_is_unknown`
- [x] SCENARIO-07: A quiet month — delivered by SCENARIO-06 — `cmd/quarry/run_summary_empty_test.go` `Test_run_summary_says_so_in_a_month_with_no_unusual_charge_and_no_new_recurring_charge`
- [x] SCENARIO-10: Snapshot taken before the month ended is warned about — `cmd/quarry/run_summary_snapshot_test.go` `Test_run_summary_warns_when_the_snapshot_was_taken_before_the_month_ended`
- [x] SCENARIO-11: Snapshot with no recorded time is warned about — delivered by SCENARIO-10 — `cmd/quarry/run_summary_snapshot_test.go` `Test_run_summary_warns_when_the_snapshot_records_no_time`
- [x] SCENARIO-12: Machine-readable summary — `cmd/quarry/run_summary_json_test.go` `Test_run_summary_json_prints_the_ruled_document`
- [x] SCENARIO-14: Missing exchange rates are warned about once — `cmd/quarry/run_summary_warnings_test.go` `Test_run_summary_warns_once_per_kind_when_the_store_has_no_exchange_rates`
- [x] SCENARIO-16: Claude asks for last month's summary — `cmd/quarry/run_mcp_summary_test.go` `Test_run_mcp_monthly_summary_returns_the_summary_json_document`
- [x] SCENARIO-17: Docs describe the monthly job — `cmd/quarry/run_skill_monthly_summary_test.go` `Test_monthly_summary_job_is_documented_where_a_reader_looks`
- [x] SCENARIO-21: A subscription that kept charging through a stray charge is not new — `internal/report/summary_recurring_test.go` `Test_summary_does_not_list_a_subscription_that_kept_charging_through_a_stray_charge_as_new`
- [ ] SCENARIO-18: Reference check on a real month

SCENARIO-18 is run by the orchestrator after SCENARIO-17 and before the gate, on a scratch HOME holding copies of the newest snapshot; results go in `REFERENCE-CHECK.md`.
