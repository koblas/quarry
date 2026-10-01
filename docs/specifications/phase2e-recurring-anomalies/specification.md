# Specification: Phase 2e — recurring charges and anomalies

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: Two read-only reports over the store. `quarry recurring` lists charges that repeat on a schedule (weekly, monthly, quarterly, yearly) with their per-year cost, start date, price changes and whether they are still active. `quarry anomalies` lists charges unusually large for their payee or category. Together they answer the PRD's monthly-summary question ("anomalies and new recurring charges") and are the shapes the Phase 3 MCP tools `recurring_charges` / `anomalies` return.

**Out of Scope**: FX / `--currency` (2f); Quicken scheduled transactions as a signal (the real file has none — no `SmartCashFlowTransaction` rows, no `ZRECURRENCEJSON`); persisted series state or "new since last sync" (no tables, no format bump); config keys for thresholds; `--csv` (series are computed in Go, `sql` cannot reach them — `--json` is the machine path); income recurrence; biweekly cadence and variable bills (documented as not covered); a `--new`-only filter (`--json | jq` covers it until Phase 4); duplicates (owned by `findings`).

**User decisions (2026-10-01)**: A — cadences weekly, monthly, quarterly, annual, judged per gap with a tolerance. B — anomalies are large vs the payee's own history, with a category fallback for payees with little history. C — expenses only (spend's scope). D — "new" = first charge inside the window; detection always uses all history; no persisted state.

## Business Rules & Invariants
Every number below is a named Go constant (policy, not config — 2d P2d-8 precedent). Detection is one core-library rule (`internal/report` or a leaf below it); CLI and future MCP pass parameters only. Money is integer cents in Go; no DuckDB `quantile`/DOUBLE.

- P2e-1 **Charge** = one transaction: the sum of its `v_spending.spent` rows, so a split counts once with all its expense splits. Only sums > 0 count. A refund or net-zero transaction is not a charge: it neither counts nor breaks a series. Rows dated after today (local, `env.Now`) are ignored by both commands.
- P2e-2 **Group key** = (`finding.PayeeKey(payee name)`, currency). An empty key falls back to the exact `payee_id`. A NULL payee is never a series and never has a payee baseline. Account and category are not in the key: a card change or category reorganisation does not split a series. CAD and USD charges from one merchant are two series.
- P2e-3 **Order** = date, then transaction source id (numeric). "Earlier" (anomalies) means a strictly earlier date.
- P2e-4 **Cadences**:

  | Cadence | Gap range (days, inclusive) | Min charges | Ended after (days since last) | Per-year factor | Text / JSON |
  |---|---|---|---|---|---|
  | weekly | 6–8 | 4 | 14 | 52 | `week` / `weekly` |
  | monthly | 26–35 | 3 | 45 | 12 | `month` / `monthly` |
  | quarterly | 84–98 | 3 | 120 | 4 | `quarter` / `quarterly` |
  | annual | 350–380 | 2 | 400 | 1 | `year` / `annual` |

- P2e-5 **Run**: the cadence is the one whose range holds the group's last gap. The run extends backwards while each gap fits that range; any gap outside it ends the run; only the latest run is the series. No range holds the last gap, or too few charges → no series.
- P2e-6 **Price change** = a step between consecutive charges in the run where |to − from| / from > `PriceChangeMinPct` = 5%. Both directions. No noise cap.
- P2e-7 **Steady gate**: the run is listed only if price changes ≤ floor(steps / 4) (`SteadyChangeShare` = 1/4). A 2-charge annual run needs both charges within 5%.
- P2e-8 **State**, as of today: `active` if days since last charge ≤ ended-after; otherwise `ended`. **New** = first charge of the run is in [since, until].
- P2e-9 **Listed** when the run's span [first, today if active else last] overlaps [since, until]. With `--account`, a series is listed only when ≥ 1 charge of its run is in a named account. Amounts and dates always come from the whole run.
- P2e-10 `per_year` = latest amount × factor, active series only (null when ended). `change_pct` / `times` use one decimal, half away from zero (cashflow precedent).
- P2e-11 **Anomaly — payee baseline**: ≥ `AnomalyPayeeMinHistory` = 3 earlier charges in the group; flag when amount > `AnomalyPayeeMultiplier` = 2 × their median.
- P2e-12 **Anomaly — category baseline** (only when P2e-11 has < 3 earlier charges): the charge's single category (every `v_spending` row of the transaction has one non-NULL `category_id`) has ≥ `AnomalyCategoryMinHistory` = 10 earlier charges in that currency; flag when amount > `AnomalyCategoryMultiplier` = 5 × their median. Uncategorized or multi-category transactions get no category baseline. Category cell: `(uncategorized)` only when the transaction's expense rows all have no category; any transaction with more than one expense split — including a NULL plus one category — shows `(split)` (2d unlinked-transfer precedent; ruled at sizing).
- P2e-13 Amount < `AnomalyMinAmount` = 100.00 (native) is never flagged. Median of an even count = mean of the two middles in cents, half away from zero. Charges in recurring series are judged like any other. A charge in the window with no baseline counts as `not_judged`. `checked` = charges in the window (after `--account`). Anomaly `--account` limits listing only; history from every account counts.
- P2e-14 Neither command loads config (added to 2d P2d-10's never-load list).
- P2e-15 Text cells escape `\n \t \r` with the shared text-cell escaper (2d final pass); paths in `--json` warnings are absolute (2d final pass).

---

## Triage Brief
- Reuse, do not re-plan: `report.Server` + `report.Store` pattern (twin `internal/report/cashflow.go:11-70`: request with Window and Accounts, `s.namedAccounts`, `s.readRefusal`, command const); `ParseWindow` (`internal/report/window.go:35-65`); `reportFlags.bind` / `.window` (`internal/cli/window.go:11-47`); command template `internal/cli/cashflow.go:39-92` (`noArgs`, `openReport`, `emitReport`, `runtimeError`); `leftOutWarnings`, `appendEmptyWindowWarning`, `emptyWindowWarning` (`internal/cli/empty_window.go:10-76`); JSON conventions `internal/cli/json.go` (`jsonDateLayout`, one `*Document` per command in `json_<cmd>.go`, `warnings` never null); views `v_spending`/`v_cash_flow` (`internal/store/duckstore/schema.go:123-181`, split grain, reporting scope); duckstore read helpers `filter.go:12-40`, `spending.go:97-135` (`openRead`, `QueryRows`, `openFault`, `transactionRange`); walk-per-payee prior art `findings_mixed.go`; `finding.PayeeKey` (`internal/finding/finding.go:51-69`, contract key).
- Callers for new `report.Store` methods: `internal/store/duckstore` (real), `internal/report/fakes_test.go:35` (`fakeStore`), `internal/cli/fakes_test.go:37` (`fakeReportStore`), `internal/cli/status_test.go:27` (embeds), port guard `cmd/quarry/run.go:29`. Root command list `internal/cli/root.go:28-33` + doc comment `:8`; root help pin in `cmd/quarry/run_status_test.go`. `reportFlags` callers: `internal/cli/cashflow.go`, `internal/cli/spend.go`.
- Data: `transactions` (date, payee_id, amount, currency, source id), `splits` (category_id, transfer_account_id), `payees`, `categories`; `v_spending` already applies expense scope. Investment transactions are not imported. Quicken scheduled transactions are not imported and the real file has none.
- Prior art (`docs/prior-art/hardkoded/plugins/quicken/skills/quicken-spending/sql/`): `recurring_candidates.sql` (n≥3, mean gap 25–35, ±15%, 45-day cancel), `price_increases.sql`, `largest_transactions.sql`.
- Nothing becomes dead; change is additive.

## Product Verdict
**SHIP WITH CHANGES** (scoping pass 2026-10-01), accepted:
1. Detection is a core Go rule, not a view; PRD `v_recurring` removed; `--json` is the machine path; no `--csv`.
2. Recurring requires the steady-amount gate (P2e-7) besides cadence.
3. `--since/--until/--account` filter what is listed, never detection input; window-flag help becomes per command (`reportFlags.bind` takes its help strings; spend/cashflow byte-identical).
4. Anomalies: minimum amount, history-gated category fallback, `not_judged` count.
5. Thresholds are named constants; both commands never load config.

## Surface & Copy

### `quarry recurring`
- Use `recurring`; Short `List charges that repeat every week, month, quarter or year`
- Long:
```
List charges that repeat on a schedule: the same payee and currency every
week, month, quarter or year, at a steady amount. quarry finds them in all
your history, with the rules of quarry spend: expense splits only, without
transfers, refunds or accounts left out of reports. A transaction counts
once, with all its splits. Payees whose names differ only in store or
reference numbers count as one payee.

A charge that comes off schedule starts the series again. A series has
ended when no charge has come for 14 days (weekly), 45 days (monthly), 120
days (quarterly) or 400 days (yearly). Bills whose amount changes most
times, such as hydro, are not listed; see quarry spend --by payee.

--since and --until choose which series to list: those running at any
time in the period. A series whose first charge falls in the period is
marked new. A price change is a step of more than 5% from one charge to
the next. Per year is the latest amount times the charges in a year, for
active series only.
```
- Examples: `  quarry recurring` / `  quarry recurring --since 2026-09 --until 2026-09 --json` / `  quarry recurring --since 2000`
- Flags:
  - `--since`: ``list series running on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)``
  - `--until`: ``list series that started on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today)``
  - `--account`: ``list only series with a charge in the account with this `name` or id; repeat for more``
- stdout, exit 0. Caption as spend with `Recurring charges` in place of `Spending`. Two-space gaps, money via `formatMoney` right-aligned, no trailing spaces, payee cell escaped:
```
Recurring charges 2026-01-01 to 2026-10-01 in all accounts

Payee        Currency  Every   Amount  Per year  First       Last        Status       Price changes
Rogers       CAD       month    95.00  1,140.00  2019-05-03  2026-09-03  active       1: 85.00 -> 95.00 (+11.8%)
Netflix.com  CAD       month    20.99    251.88  2014-03-12  2026-09-12  active       4: 9.99 -> 20.99 (+110.1%)
Crave        CAD       month    22.59    271.08  2026-03-02  2026-09-02  active, new
Costco       CAD       year    130.00    130.00  2017-11-20  2025-11-21  active
Disney Plus  CAD       month    11.99            2025-02-07  2026-04-07  ended
Total        CAD                        1,792.16
```
- Payee = name on the latest charge. Status = `active` / `ended`, plus `, new`. Price changes = `N: first -> latest (±p%)`, p first-to-latest of the run; `0.0%` unsigned; empty cell when N = 0. Total row: one per currency with any active series, CAD before USD; only Per year filled. Sort: currency, active before ended, per year desc (ended: last desc), lower(payee), key.
- `--json`:
```
{"since","until","account_filter":[],"series":[{"payee":"Netflix.com","payee_key":"netflix-com",
 "payees":[{"id":"payee-12","name":"Netflix.com"}],"currency":"CAD","cadence":"monthly","amount":"20.99",
 "first_amount":"9.99","per_year":"251.88","first_charge":"2014-03-12","last_charge":"2026-09-12",
 "charge_count":151,"state":"active","new":false,"accounts":[{"id","name"}],
 "price_changes":[{"date":"2016-05-12","from":"9.99","to":"11.99","change_pct":20.0}]}],
 "totals":[{"currency":"CAD","per_year":"1792.16"}],"warnings":[]}
```
  `payee_key` null for the payee_id fallback. Arrays `[]`, never null. Money a 2-decimal string. Text and JSON order identical. `price_changes[].date` is the later charge's date; `payees` and `accounts` are distinct by id in first-appearance order within the run (ruled at SCENARIO-07 planning).

### `quarry anomalies`
- Use `anomalies`; Short `List charges unusually large for their payee or category`
- Long:
```
List charges that are unusually large: more than 2 times the median of
the payee's earlier charges, when there are at least 3, or else more than
5 times the median of the category's earlier charges, when there are at
least 10. Charges under 100.00 are never listed. Charges follow the rules
of quarry spend, and a transaction counts once, with all its splits; an
uncategorized or split charge from a payee with little history cannot be
judged. Possible duplicates are listed by quarry findings, not here.

--since and --until choose which charges to list; each is compared with
every earlier charge, however old. --account lists only charges in those
accounts; the payee's charges in other accounts still count as history.
```
- Examples: `  quarry anomalies` / `  quarry anomalies --since 2026-09 --until 2026-09` / `  quarry anomalies --account "Visa Infinite" --json`
- Flags: `--since` ``list charges dated on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)``; `--until` ``list charges dated on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today)``; `--account` ``list only charges in the account with this `name` or id; repeat for more``.
- stdout, exit 0. Account via `accountLabel`; Category `(uncategorized)` / `(split)` (2d precedent); text cells escaped:
```
Unusually large charges 2026-01-01 to 2026-10-01 in all accounts

Date        Account         Payee        Category           Amount   Usual  Times  Compared with
2026-08-14  Visa (CAD)      Home Depot   Home:Repairs     1,842.10  210.40   8.8x  category, 212 earlier
2026-03-02  Chequing (CAD)  Bell Canada  Utilities:Phone    412.00   96.05   4.3x  payee, 38 earlier

1,204 charges checked; 87 had too little history to judge
```
- Sort: date desc, then transaction source id desc. Footer always printed in text mode (`1 charge checked`; `; N had…` omitted when 0; `1 had`). `x`, not `×`.
- `--json`: `{"since","until","account_filter":[],"anomalies":[{"transaction_id","date","account_id","account","currency","payee","category","amount","baseline":"payee"|"category","usual":"210.40","earlier":212,"times":8.8}],"checked":1204,"not_judged":87,"warnings":[]}`. `category` null in both the uncategorized and the split case.

### Refusals and warnings (both commands; reuse verbatim, `<cmd>` = recurring/anomalies)
R1, R2, R3a/b/c, R3, O2, H1 as in 2a. I1 `quarry: <cmd> interrupted` (1). S1, S2, S2d, S3, S5, S6 as in 2b (S1–S3 exit 2; S5, S6 exit 1). U8 `quarry: <cmd> takes no arguments` (2). No `--by`, so no S4. W2/W3 with `<cmd>`, 2b ordering and every-account-left-out rules (0). Empty result: caption + header (anomalies adds its footer, `0 charges checked`), and E1/E2/E1a/E2a via `appendEmptyWindowWarning` with subject `recurring charges` / `unusually large charges`, e.g. `quarry: warning: no recurring charges from 2026-01-01 to 2026-10-01; the store's transactions run 2003-01-04 to 2026-09-26` (0). Warnings go to `warnings[]` without the prefix and to stderr after stdout. `--json` with a refusal: stdout empty.

### Edge cases
| Input | Result |
|---|---|
| merchant billed in CAD and USD | two series |
| subscription moves between cards | one series; `accounts` lists both |
| closed account | in detection; series usually `ended`; matchable by `--account` |
| account not in reports / linked | never in detection (`v_spending`); W2/W3 only when named |
| payee renamed in Quicken, same PayeeKey | one series, latest name shown, `payees` lists both |
| renamed to a different key | old series `ended`, new one `active, new` if it started in the window (accepted; documented) |
| series spanning `--since` | listed, not new |
| paused and resumed (off-schedule gap) | only the resumed run; new if it resumed in the window |
| charged twice one month / extra purchase at the payee | the gap breaks the run; may drop out (accepted) |
| annual, 2 charges within 5% | listed; > 5% apart → not listed |
| biweekly, or a variable bill | not listed (no cadence / steady gate) |
| refund or net ≤ 0 transaction | not a charge; never flagged |
| split transaction | one charge (sum of its expense splits); anomaly category `(split)`, payee baseline only |
| future-dated row | ignored by both commands, even with `--until` past today |
| new payee, uncategorized | `not_judged` |
| stale store | no warning (2a precedent) |
| empty window | E1/E2 |

### Changes to existing surfaces
1. PRD `docs/initial-prd.md:129`: delete `` `v_recurring` (detected series), `` and add "recurring series and anomalies are computed by the core library; `quarry recurring --json` / `anomalies --json` are the shapes the Phase 3 `recurring_charges` / `anomalies` tools return."
2. `reportFlags.bind` (`internal/cli/window.go`): help strings become per command; spend/cashflow strings stay byte-identical.
3. `internal/cli/root.go:8` doc comment lists subcommands: add recurring and anomalies. Root Available Commands pin (`cmd/quarry/run_status_test.go`) gains both, cobra alphabetical order.
4. 2d P2d-10 never-load-config list: add `recurring`, `anomalies`.
5. spend, cashflow, findings and sql Longs: unchanged (still true).

---

## Scenarios (Gherkin)

Build order (sizing pass 2026-10-01); `(was NN)` in `## Sizing` maps the scoping ids.

### recurring

```gherkin
Scenario: SCENARIO-01 — recurring lists a monthly subscription with its yearly cost
  Given a payee charged 20.99 CAD on the 12th of each month for the last year
  When I run quarry recurring
  Then stdout lists it with Every month, Amount 20.99, Per year 251.88, its first and last dates and Status active, and a CAD Total row
```

```gherkin
Scenario Outline: SCENARIO-02 — recurring detects weekly, quarterly and yearly series
  Given a payee charged a steady amount every <gap> days, <n> times, the last one recent
  When I run quarry recurring --since 2000
  Then the series is listed with Every <every>
  Examples:
    | gap | n | every   |
    | 7   | 4 | week    |
    | 91  | 3 | quarter |
    | 365 | 2 | year    |
```

```gherkin
Scenario: SCENARIO-03 — a charge off schedule starts the series again
  Given a payee charged monthly, then a 70-day gap, then charged monthly three more times
  When I run quarry recurring --since 2000
  Then the series' first date is the first charge after the gap and its charge count is 3
```

```gherkin
Scenario: SCENARIO-04 — a split transaction counts once and a refund does not break the series
  Given a monthly series where one month's charge is split across two expense categories and a refund from the payee falls mid-series
  When I run quarry recurring --since 2000
  Then the series is listed with every month counted once and the refund left out
```

```gherkin
Scenario Outline: SCENARIO-05 — a series ends after its cadence's quiet period
  Given a <cadence> series whose last charge was <days> days ago
  When I run quarry recurring --since 2000
  Then its Status is <status>
  Examples:
    | cadence   | days | status |
    | weekly    | 14   | active, new |
    | weekly    | 15   | ended, new  |
    | monthly   | 45   | active, new |
    | monthly   | 46   | ended, new  |
    | quarterly | 121  | ended, new  |
    | annual    | 400  | active, new |
```

```gherkin
Scenario: SCENARIO-06 — a series first charged in the window is marked new
  Given one monthly series that started in March of this year and one that started in 2019
  When I run quarry recurring
  Then the March series reads "active, new" and the 2019 series reads "active"
```

```gherkin
Scenario: SCENARIO-07 — price changes are listed both ways with first to latest
  Given a monthly series 9.99 x8, 11.99 x8, 10.99 x8
  When I run quarry recurring --since 2000
  Then Price changes reads "2: 9.99 -> 10.99 (+10.0%)" and --json lists both steps with their dates and change_pct
```

```gherkin
Scenario: SCENARIO-08 — recurring --json returns the series document
  Given one active monthly series with one price change
  When I run quarry recurring --since 2000 --json
  Then stdout is the ruled document with since, until, account_filter, series (payee, payee_key, payees, currency, cadence, amount, first_amount, per_year, first_charge, last_charge, charge_count, state, new, accounts, price_changes), totals and warnings
```

```gherkin
Scenario: SCENARIO-09 — a bill whose amount changes most months is not listed
  Given a payee charged monthly with an amount more than 5% different from the previous one in most months
  When I run quarry recurring --since 2000
  Then the payee is not listed
```

```gherkin
Scenario: SCENARIO-10 — payees differing in store numbers are one series, currencies are two
  Given monthly charges from "NETFLIX.COM 1234" and "Netflix.com" in CAD, and from "Netflix.com" in USD
  When I run quarry recurring --since 2000
  Then one CAD series and one USD series are listed, and the CAD series' --json payees lists both names
```

```gherkin
Scenario: SCENARIO-11 — recurring --account lists only series charged in that account
  Given one monthly series charged on Visa and another charged on Chequing
  When I run quarry recurring --since 2000 --account Visa
  Then only the Visa series is listed
```

```gherkin
Scenario: SCENARIO-12 — no series in the window says so
  Given a store with no recurring charges
  When I run quarry recurring
  Then stdout is the caption and header only, stderr has the "no recurring charges from … to …" warning, and the exit code is 0
```

```gherkin
Scenario Outline: SCENARIO-13 — recurring refuses usage and store problems
  Given <state>
  When I run quarry recurring <args>
  Then stderr is <line> and the exit code is <code>
  Examples:
    | state        | args              | line                                         | code |
    | any store    | extra             | "quarry: recurring takes no arguments"       | 2    |
    | any store    | --since 2026-13   | the S1 bad-date line                         | 2    |
    | a store      | --account Nope    | the unknown-account refusal naming recurring | 1    |
    | no store     |                   | the R1 no-store line naming recurring        | 1    |
```

### anomalies (SCENARIO-16 covers all four reports' window-flag help; it lands with the anomalies command)

```gherkin
Scenario: SCENARIO-14 — a charge over twice the payee's usual is listed
  Given a payee charged about 96.05 three or more times before, then 412.00 this year
  When I run quarry anomalies
  Then the 412.00 charge is listed with Usual 96.05, Times 4.3x and "payee, N earlier"
```

```gherkin
Scenario Outline: SCENARIO-15 — anomaly thresholds hold at their boundaries
  Given a payee whose earlier charges have median <median> and a new charge of <amount>
  When I run quarry anomalies
  Then the charge is <listed>
  Examples:
    | median | amount | listed     |
    | 40.00  | 99.99  | not listed |
    | 60.00  | 120.00 | not listed |
    | 60.00  | 120.01 | listed     |
```

```gherkin
Scenario: SCENARIO-16 — each report's window flags describe what that report does with them
  Given the quarry binary
  When I run quarry spend --help, quarry cashflow --help, quarry recurring --help and quarry anomalies --help
  Then spend and cashflow show their existing --since, --until and --account help unchanged and recurring and anomalies show their ruled help
```

```gherkin
Scenario: SCENARIO-17 — a payee with little history is judged against its category
  Given a first-time payee charged 1,842.10 in a category with at least 10 earlier charges whose median is 210.40
  When I run quarry anomalies
  Then the charge is listed with Usual 210.40, Times 8.8x and "category, N earlier"
```

```gherkin
Scenario: SCENARIO-18 — a charge with no usable history is counted as not judged
  Given an uncategorized charge from a first-time payee this year
  When I run quarry anomalies
  Then it is not listed and the footer reads "N charges checked; 1 had too little history to judge"
```

```gherkin
Scenario: SCENARIO-19 — anomalies --json returns the anomalies document
  Given one anomaly and one charge too new to judge in the window
  When I run quarry anomalies --json
  Then stdout is the ruled document with since, until, account_filter, anomalies (transaction_id, date, account_id, account, currency, payee, category, amount, baseline, usual, earlier, times), checked, not_judged and warnings
```

```gherkin
Scenario: SCENARIO-20 — history outside the window and other accounts still counts
  Given a payee's three earlier charges were last year on Chequing and this year's large charge is on Visa
  When I run quarry anomalies --account Visa
  Then the Visa charge is listed with "payee, 3 earlier"
```

```gherkin
Scenario: SCENARIO-21 — no charges in the window says so
  Given a store with no charges in the window
  When I run quarry anomalies
  Then stdout is the caption, header and "0 charges checked", stderr has the "no unusually large charges from … to …" warning, and the exit code is 0
```

```gherkin
Scenario Outline: SCENARIO-22 — anomalies refuses usage and store problems
  Given <state>
  When I run quarry anomalies <args>
  Then stderr is <line> and the exit code is <code>
  Examples:
    | state     | args            | line                                         | code |
    | any store | extra           | "quarry: anomalies takes no arguments"       | 2    |
    | any store | --until 2026-02-30 | the S1 bad-date line                      | 2    |
    | a store   | --account Nope  | the unknown-account refusal naming anomalies | 1    |
    | no store  |                 | the R1 no-store line naming anomalies        | 1    |
```

---

## Sizing
Ruled (sizing pass 2026-10-01):
- **Placement.** Detection lives in the `report` feature package (new files such as `charges.go`, `recurring.go`, `anomalies.go`): unexported funcs over driver-free charge values, thresholds as named constants there, `Server.Recurring` / `Server.Anomalies` as the only entry points (Phase 3 MCP calls the same methods). There is no leaf package. `internal/finding` is a leaf because `snapshot`, `report` and duckstore all read it. Here `report.Server` is the only consumer, and nothing is persisted (decision D). A separate feature package would have `cli` call two servers and duplicate the loader, and feature packages never import each other. `finding.PayeeKey` comes from the existing leaf.
- **One port method for both commands**, fixed in SCENARIO-01 so anomalies never reopens the port, the three fakes or the `cmd/quarry/run.go:29` guard: `report.Store.Charges(ctx, store.ChargeParams{Through: today})` returns `store.Charges{Rows, Transactions store.TransactionRange}`.
  - `Through` is today, passed in by the caller from `env.Now`. The adapter computes no date.
  - The adapter takes no window and no account ids. This deliberately departs from the `Spending`/`CashFlow` twin: detection and baselines use all history, and `--since/--until/--account` filter only what is listed, in Go.
  - `Transactions` feeds E1/E2 from the same read.
  - Each row has: transaction id, numeric source id, date, account id, name and currency; payee id and name (nullable); currency; amount in cents (sum of the transaction's `v_spending.spent`, sums > 0 only); category id and path when there is exactly one; and a kind that tells `(uncategorized)` from `(split)`. Rows are ordered by date, then source id.
  - `--account` names resolve through `s.namedAccounts` (cashflow precedent). That call resolves names and renders nothing. Every rendered value comes from the one `Charges` read.
- **Acceptance tests at the real store.** Rules that live in adapter SQL (split counted once, refund and net-zero dropped, future-dated rows dropped, single versus multi category) get their acceptance tests in `cmd/quarry` end-to-end tests (`run_recurring_*_test.go`, `run_anomalies_*_test.go`). A `cli` test against `fakeReportStore` proves nothing about them.
- **Later scenarios pin earlier rules in the outputs they add.** SCENARIO-07 pins `state:"ended"`, `per_year: null` and `new: true` in `--json`. SCENARIO-11 and SCENARIO-20 pin W2/W3/E1/E2 in `warnings[]` as well as on stderr.
- Each command's first scenario (01, 14) adds the command to the P2d-10 never-load-config pin, the root doc comment and the root Available Commands pin. PRD change 1 rides SCENARIO-01's Sweep.

| Scenario (was) | Verdict — numbers |
| --- | --- |
| 01 (01) | **OWNS A RUN, 6 batches — orchestrator overruled SPLIT** (no new Gherkin; two run groups: B1–B2 = the 01a batches, B3–B4 = the 01b batches; architect may plan `Runs: A | B1 | B2 | B3 | B4 | V`). Sizing rationale: twins 2d SCENARIO-11 at 2,594k and 2b SCENARIO-20 at 2,445k, each a read port, adapter, command and renderer, are above the p90 of the 32 pooled 2b+2d units, about 2,400k). **01a** — `report` + duckstore: the `Charges` port and adapter with its arms (split summed once, refund and net-zero dropped, future-dated dropped, single, uncategorized and multi-category, including a transaction mixing a NULL and one category, which is not single (P2e-12), transfer excluded by `v_spending`) and the fakes and port guard; detection: group key arms (PayeeKey merge, currency split, empty-key `payee_id` fallback, NULL payee), the full cadence table, latest run; `Server.Recurring`: window overlap, `active`, per_year, per-currency totals, sort. Acceptance test is a `Server` method test. 3 batches. **01b** — `cli` + `cmd/quarry`: `reportFlags.bind` takes per-command help (spend/cashflow byte-identical, with `report_help_test.go:11-104` as control); `recurring` command with Long, Examples and flags; root registration and pins; text renderer with caption, header, rows and Total. Acceptance test is this scenario's e2e test, plus the folded 02/03/04 tests. 3 batches |
| 02 (02) | FOLD into 01 — cadence table rows; pinned in 01a, acceptance test ticks with 01b |
| 03 (03) | FOLD into 01 — the gap-outside-range arm of 01a's run walk |
| 04 (09) | FOLD into 01 — adapter SQL arms of 01a (sum per transaction, > 0); e2e acceptance test |
| 05 (06) | LIGHT — 3 steps: `ended` per cadence at each ended-after bound and one day past it; per_year empty and Total skips ended; sort tier active before ended, then ended by last date descending; status cell with `, new` (folded 06), `report` + `cli` |
| 06 (07) | FOLD into 05 — `new` is one comparison (first charge on `since` and one day before it) plus the `, new` suffix |
| 07 (05) | OWNS A RUN — 4 batches: price-change steps and steady gate (5% bound both directions; floor(steps/4) at the bound and +1; annual 2-charge within and past 5%; change_pct half away from zero); Price changes cell (`0.0%` unsigned, empty when N = 0); per-series `payees`/`accounts` and `payee_key` null on fallback; recurring `--json` document with read-back test and the ended/new pins. `report` + `cli`. Merge forced: 05's Then needs `--json` and 08's Given needs a price change |
| 08 (11) | FOLD into 07 — the JSON document is 07's batch 4 |
| 09 (04) | FOLD into 07 — the steady gate counts 07's price changes |
| 10 (08) | FOLD into 07 — group-key arms built in 01a; its `payees` assertion needs 07's JSON |
| 11 (10) | OWNS A RUN — 3 batches (Sonnet-size): `--account` lists a series with at least one run charge in a named account, amounts from the whole run; W2/W3; empty window E1/E2/E1a/E2a with subject `recurring charges` from the `Charges` read's `Transactions`; refusal outline (U8, S1, unknown account, R1) plus I1. `report` + `cli` |
| 12 (12) | FOLD into 11 — the empty-window branch |
| 13 (13) | FOLD into 11 — reused refusal helpers; outline rows pinned there |
| 14 (14) | **OWNS A RUN, 4 batches — orchestrator overruled SPLIT** (no new Gherkin; B1 = 14a, B2 = 14b). Sizing rationale: twin 2b SCENARIO-20, cashflow's first command scenario at 2,445k, is above the pooled p90 of about 2,400k). **14a** — `report` only, no port change: median (even count, half away from zero), payee baseline, the 100.00 minimum, times; `Server.Anomalies` over the same `Charges` read (window listing, `checked`/`not_judged`, sort by date then source id, both descending); the 15 threshold arms. Acceptance test is a `Server` method test. 2 batches. **14b** — `cli` + `cmd/quarry`: `anomalies` command, help and root pins; text renderer with footer (`1 charge`, `1 had`, clause omitted at 0) and category cell `(uncategorized)`/`(split)`. Acceptance test is this scenario's e2e test, plus the folded 15/16 tests. 2 batches |
| 15 (16) | FOLD into 14 — threshold arms of 14's batch 1 |
| 16 (22) | FOLD into 14 — anomalies registers the fourth help, so this acceptance test can first pass in 14. The `bind` refactor itself lands in 01b |
| 17 (15) | LIGHT — 2 steps: category fallback (9 and 10 earlier; 5× and just past; another currency's charges not counted; uncategorized and multi-category get none; a payee with 3 earlier is judged on the payee, never the category) and the `category, N earlier` cell. `report` + `cli` |
| 18 (17) | FOLD into 17 — `not_judged` is the arm where neither baseline applies |
| 19 (19) | LIGHT — 2 steps: anomalies `--json` document (category null in both cases, both `baseline` values, `times` number) and a read-back test, `cli` |
| 20 (18) | OWNS A RUN — 3 batches (Sonnet-size): `--account` limits listing and `checked` while history from every account still counts; W2/W3 and the empty window with footer `0 charges checked`; refusal outline plus I1. `report` + `cli` |
| 21 (20) | FOLD into 20 — the empty-window branch |
| 22 (21) | FOLD into 20 — reused refusal helpers |

## BDD Acceptance Progress
- [x] SCENARIO-01: recurring lists a monthly subscription with its yearly cost — `cmd/quarry/run_recurring_test.go` `Test_run_recurring_lists_a_monthly_subscription_with_its_yearly_cost`
- [x] SCENARIO-02: recurring detects weekly, quarterly and yearly series — delivered by SCENARIO-01: `cmd/quarry/run_recurring_test.go` `Test_run_recurring_detects_weekly_quarterly_and_yearly_series`
- [x] SCENARIO-03: a charge off schedule starts the series again — delivered by SCENARIO-01: `internal/report/recurring_test.go` `Test_recurring_starts_the_series_again_after_a_charge_off_schedule`
- [x] SCENARIO-04: a split transaction counts once and a refund does not break the series — delivered by SCENARIO-01: `cmd/quarry/run_recurring_test.go` `Test_run_recurring_counts_a_split_charge_once_and_leaves_a_refund_out`
- [x] SCENARIO-05: a series ends after its cadence's quiet period — `cmd/quarry/run_recurring_state_test.go` `Test_run_recurring_marks_a_series_ended_one_day_past_its_cadences_quiet_period`
- [x] SCENARIO-06: a series first charged in the window is marked new — delivered by SCENARIO-05: `cmd/quarry/run_recurring_state_test.go` `Test_run_recurring_marks_a_series_first_charged_in_the_window_as_new`
- [ ] SCENARIO-07: price changes are listed both ways with first to latest
- [ ] SCENARIO-08: recurring --json returns the series document
- [ ] SCENARIO-09: a bill whose amount changes most months is not listed
- [ ] SCENARIO-10: payees differing in store numbers are one series, currencies are two
- [ ] SCENARIO-11: recurring --account lists only series charged in that account
- [ ] SCENARIO-12: no series in the window says so
- [ ] SCENARIO-13: recurring refuses usage and store problems
- [ ] SCENARIO-14: a charge over twice the payee's usual is listed
- [ ] SCENARIO-15: anomaly thresholds hold at their boundaries
- [ ] SCENARIO-16: each report's window flags describe what that report does with them
- [ ] SCENARIO-17: a payee with little history is judged against its category
- [ ] SCENARIO-18: a charge with no usable history is counted as not judged
- [ ] SCENARIO-19: anomalies --json returns the anomalies document
- [ ] SCENARIO-20: history outside the window and other accounts still counts
- [ ] SCENARIO-21: no charges in the window says so
- [ ] SCENARIO-22: anomalies refuses usage and store problems

## Reference check
Last step before the gate round (step 7). Outcome recorded in `REFERENCE-CHECK.md` beside this file.

- [ ] Series and anomalies on the real Quicken file hold up to review: run `quarry recurring --since 2000` and `quarry anomalies --since 2000` on David's file (scratch HOME from the latest snapshot); David reviews up to 20 series and 20 anomalies. A command with more than half false positives has its rule re-ruled (mid-feature product-vision ruling) and the fix appended as a new scenario before the gate round.
