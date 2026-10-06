# Reference check — SCENARIO-18 (2026-10-06)

Run by the orchestrator on copies of snapshot `20261004T184923Z` and the store in a scratch HOME (the user's store, snapshots and config untouched), binary built from the feature branch. Counts only; no names, amounts or ids of the user's data.

The copied store had been built by an earlier quarry and was refused ("built by another version of quarry; run quarry sync --from …"), so the scratch store was rebuilt from the copied snapshot first. The recipe's `quarry sync` does this on the user's first scheduled run.

## Sections against the sibling commands

Months 2025-06, 2025-12 and 2026-01 to 2026-09, each `quarry summary --month M --json`:

| Check | Result |
|---|---|
| `anomalies` = `quarry anomalies --since M --until M` (charges, checked, not_judged) | 11 / 11 equal |
| `anomalies` text section = `quarry anomalies` text | verbatim, 3 / 3 months checked |
| `net_worth.dates` = `quarry networth --since M-1 --until M` dates | 11 / 11 equal |
| Net worth text table | every `quarry networth` line present once column widths are normalised (the Change row widens columns, as ruled); Change row present |
| `changes` arithmetic (per type in the reporting currency, and total) | end − start exact; types whose balance is 0 on both days are omitted, the same `Types()` rule as the networth table |
| `findings` = `quarry status` findings | 11 / 11 equal |
| `snapshot.covers_month` | true for every month (snapshot taken 2026-10-04) |
| Warnings / stderr | 0 / 0 on every month |
| Rerun of 2026-09 | identical document |
| September's new series fields vs `quarry recurring --since 2026-09 --until 2026-09` | identical (1 series) |
| Time per `summary` run | 0.15–0.24 s; default month 0.16 s |

## Recurring "new" rule — two rule gaps found, both fixed before the gate

Probed every month 2024-01 to 2026-09 (33 months), old binary vs fixed.

- **SCENARIO-21 (U17).** A monthly series kept charging, but one off-schedule charge between two runs hid the earlier run from `resumes`, so the series was reported new again 30 days after its last charge (ended-after 45). On the real file one dense monthly payee was reported new in 5 months (2024-03, 2024-06, 2025-03, 2025-12, 2026-03). It is now new only in 2025-03, where the previous run had ended 214 days earlier. Triage reproduced all 4 now-hidden months in a re-implementation that matches the binary. Each hiding run is the same monthly series, steady, 3–9 charges, 20–31 days before the new run.
- **SCENARIO-22 (U18).** The U17 fix then hid an annual series in every month. Its 2025 charge was duplicated on the same day, so the series was never listable at its 2025 month end, and U17 treated that never-listed run as one to resume. A run now resumes only an earlier series `quarry recurring` could list as of that series' last charge day. The annual series is new in 2026-08 again, as before the fix.
- After both fixes: 0 series new in more than one month; 26 series ever new; the only difference from the pre-fix binary is the dense monthly payee above.
- Recognition lag on the real file: monthly series 2 months after the first charge (3rd charge), annual 11–12 months (2nd charge), as ruled.

### Expected differences from today's `quarry recurring`

2 series reported new (2026-06 weekly, 2026-08 monthly) are not listed by today's `quarry recurring --since 2000`. Both were steady at their month end and broke afterwards (a newest gap of 3 and 12 days matches no cadence). Plain `recurring` judges only the group's latest run as of today, so a series broken later is not listed at all. This is existing `recurring` behaviour, which this feature leaves unchanged. Summary's as-of-month-end rule (U2, U4) reports it in the month it was recognized.

## The scheduled job, by hand

The recipe's `sh -c` string was run with `env -i HOME=<scratch> PATH=/usr/bin:/bin` and the scratch binary, with no launchd:

- `$HOME` expands inside `sh -c`; the log directory is created 0700 and `summary.log` is 0600 (umask 077).
- Exit 0. The log holds sync's error line first (scratch HOME has no Quicken file: `quarry: no .quicken file found in ~/Documents or …`), then the full summary. This is the recipe's "Quicken was closed" path, except that the snapshot here covers the month, so no snapshot warning is printed.

## Left to the user (manual)

- `launchctl bootstrap` / `kickstart` on macOS 26 and the TCC prompt for `~/Documents/*.quicken` when launchd runs `quarry sync`. These need a LaunchAgent loaded into the user's own GUI session, which is a persistent change to the account, so it was not done here. Steps 3–5 of `references/monthly-summary.md` are the check.
