# Specification: Phase 2b — spending (`spend`, `cashflow`)

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: Answer "where did the money go" from quarry's store the way Quicken's own reports do.
`quarry spend` and `quarry cashflow` read two transfer-aware views built into the store (`v_cash_flow`,
`v_spending`), so the CLI, `quarry sql`, the skill and the future MCP server share one set of spending rules.
2b carries Phase 2's gate: quarry's spending and cash-flow numbers match Quicken's reports for two years.

**Out of Scope**: `--currency` / FX conversion (2f); category roll-up flag (use `--json | jq` or sql);
configurable default window (2c candidate `reports.since`); `networth` (Phase 4); recurring/anomalies (2e);
findings (2d); `ZTARGETACCOUNT`/`ZSENDACCOUNT` (probe P2 found 0 affected transactions).

**Business Rules**: native currency only, never summed across currencies; one rule owner (the store views);
exit 2 only when argv plus the clock decide the outcome, anything needing the store exits 1.

## Business Rules & Invariants
- P2b-1 (import): `transactions.excluded_from_reports BOOLEAN NOT NULL` from `ZEXCLUDEFROMREPORTS` (NULL → false).
- P2b-2 (import): `accounts.in_reports BOOLEAN NOT NULL` from `ZACCOUNT.ZUSEDINREPORTS` (NULL → true, Quicken's default). Quicken data, not a quarry decision.
- P2b-2a (import, Gate ruling): `accounts.linked_tracking BOOLEAN NOT NULL` from `COALESCE(ZACCOUNT.ZSIMPLEINVESTING, 0) <> 0` (NULL → false), appended last in `accounts` after `in_reports`; Go field `store.Account.LinkedTracking`. Quicken's "Linked account tracking" (balance only; Quicken shows no transactions). Folds into format 3 (unshipped) — no new bump; dev stores synced before this need `quarry sync` again. Quicken data, not a quarry decision.
- P2b-3 (import): `transactions.date` = UTC calendar day of `ZENTEREDDATE` (the register date — user-confirmed on the real file: entered 2026-06-01 / posted 2026-05-31 shows 2026-06-01), falling back to `ZPOSTEDDATE` only when entered is NULL. Posted kept as nullable `transactions.posted_date DATE`. Balance validation (`checkBalances`, `internal/importer/validate.go:106-133`) sums by reconcile status and never reads transaction dates, so it is unaffected. The flip also moves `status`'s First/Last dates and which rows fall on `v_account_balances`' `date <= current_date` edge — accepted, not separately asserted.
- P2b-4 (import): splits on the system category whose full_path is `Uncategorized` are stored with `category_id NULL` (the quirk is resolved in the importer, never by name-matching in a view). Probe P1 on the real file: Transfer 1,384 splits, Uncategorized 604, Transfer:Credit Card Payment 289, Adjustment 97, Investments:* 0.
- P2b-5 (store): `duckstore.FormatVersion` 2 → 3; `v_cash_flow` and `v_spending` join the `storeRelations()` literal and must return rows from `minimalRows`. A format-2 store gets R2 with its `sync --from <id>` fix.
- P2b-6 (`v_cash_flow`, split grain) columns: `split_id, transaction_id, account_id, date, month (DATE, first of month), currency, category_id, category (full_path; NULL = uncategorized), payee_id, payee, flow ('income'|'expense'), amount (signed native DECIMAL(18,2))`. A split is in the view only when: it is not a transfer leg (`split.id ∈ transfers.from_split_id ∪ transfers.to_split_id` — never `transfer_account_id`, which is NULL on unmatched legs); its transaction is not `excluded_from_reports`; its category is not `system` kind; its account is **reported**. An account is reported when `in_reports = true` and `linked_tracking = false`, as Quicken reports do: Quicken's reports leave out every row a linked-tracking account downloads, even though ZTRANSACTION holds cash-flow rows for it. The flag decides, not account type or data source — other retirement and `ONLINE` accounts match Quicken's reports with their rows in (Gate 2024–25 USD). One owner for "reported": one duckstore SQL definition used by the view predicate and the E1a/E2a range query, and one `store.Account` method (e.g. `LeftOutOfReports() bool { return a.NotInReports || a.LinkedTracking }`) used by the every-named-account-left-out check. Every other account type included, closed accounts too (brokerage/retirement that are not linked contribute only their imported cash-flow rows). Hidden categories included. No date cutoff in the view.
- P2b-7 (`flow`): the split's own category kind wins (never the parent's). An uncategorized split takes flow from its sign: negative → `expense`, positive → `income`; zero-amount uncategorized splits dropped. A positive uncategorized split never nets against spending.
- P2b-8 (`v_spending`): `SELECT split_id, transaction_id, account_id, date, month, currency, category_id, category, payee_id, payee, -amount AS spent FROM v_cash_flow WHERE flow = 'expense'` — it selects from `v_cash_flow`, never with its own predicates. Refunds net; a category can be negative (printed negative, never clamped). Tags joined through `split_tags` at query time.
- P2b-9 (aggregates, owned by the store query; front ends pass parameters only): `income = SUM(amount | income)`, `spent = SUM(spent)`, `net = income − spent`, `savings_rate_pct = round(100·net/income, 1)`, NULL when income ≤ 0.
- P2b-10 (invariant): `spend` Total equals `cashflow` Total Spent for the same window, accounts and currency.
- P2b-11 (window): `--since`/`--until` inclusive; `YYYY`, `YYYY-MM`, `YYYY-MM-DD`; a bare year/month covers all of it. Defaults: January 1 of the current year (local) through today (local), so future-dated rows are out unless `--until` is later than today. A period row is `partial` when the window starts after its first day or ends before its last day. One shared parser for both commands.
- P2b-12 (`--account`): StringArray; exact id first, then exact name ignoring case; closed accounts match; repeats resolving to the same account de-duplicated.
- P2b-13 (ordering): argv checks (S1–S4, U8) run before `openReport`, then account resolution (S5, S6, W2), then the query. All E and W lines go into `warnings[]` without the `quarry: warning: ` prefix and to stderr after stdout succeeds, in both output modes.
- P2b-14 (warning prefix): every `warnings[]`-bearing stderr line uses `quarry: warning: ` (sync and sql already do). 2a's `accounts` all-closed note changes accordingly; its `warnings[]` text is unchanged.
- P2b-15: reuse the 2a read-command shape — `openReport` → store call → `renderResult` → `emit` (O2); `readRefusal` (I1, R1–R3); `noArgs` (U8); U9 via `ExecuteC`; H1 via `resolveHome(command)`; `warnings` always present as `[]`; absolute paths in `--json`; `marshalDocument`; `formatMoney`; `humanize`.

---

## Triage Brief

- Store: categories.kind ∈ {income, expense, system} (`internal/importer/categories.go:14`), per category; hierarchy `parent_id`/`full_path` ":"-joined (`categories.go:95-112`); `categories.hidden` exists. Uncategorized = `splits.category_id IS NULL` (`internal/importer/splits.go:80-83`).
- Transfers: every ZTRANSFER-linked split has exactly one `transfers` row (`internal/importer/transfers.go:64-98`): paired, name-form one-sided, unmatched numeric (`transfer_account_id` NULL). `Transfer.FromSplitID` is the lower source id, not the money-out leg (`internal/store/store.go:96-105`).
- Sign: negative = money leaving; split sums validated (`internal/importer/validate.go:152-176`). Payee on transaction (nullable), tags many-to-many on splits (`split_tags`).
- Dates: today `transactions.date` = UTC day of `COALESCE(ZPOSTEDDATE, ZENTEREDDATE)` (`internal/importer/transactions.go:121-130`, ORDER BY `:47`) — P2b-3 flips it.
- Not imported today: `ZTRANSACTION.ZEXCLUDEFROMREPORTS`, `ZACCOUNT.ZUSEDINREPORTS` (`internal/quicken/v9/reference.sql:79,83`). No void column.
- Account types: chequing, savings, credit_card, asset, liability, home_equity, brokerage, retirement (`internal/importer/accounts.go:13-24`); `store.IsInvestmentAccount` (`store.go:27-38`).
- Read seam: `duckstore.openRead` + `QueryRows`; `Accounts` (`internal/store/duckstore/accounts.go:12-57`) is the template — one statement, `SUM(DECIMAL)` cast to BIGINT cents in SQL, `openFault(s.Path(), err)` on read faults.
- `report.Store` has 3 implementers — `duckstore` (`accounts.go:23`), cli fake (`internal/cli/fakes_test.go:22`), report fake (`internal/report/fakes_test.go:21`) (LSP); a new port method lands in all three.
- `FormatVersion` const 2 (`internal/store/duckstore/duckstore.go:24`, checked `:235`, written `:428`; tests use the constant except literal `2` at `internal/cli/json_status_internal_test.go:65`, `internal/report/status_test.go:16`) (LSP/grep). View DDL built at `duckstore.go:389` (`schemaDDL+accountBalancesViewDDL()`), `store_info` appended last.
- `storeRelations()` literal (`internal/store/duckstore/query_test.go:28`) + `SELECT *` must return rows (`:53-60`).
- Tests the new commands break: root Available Commands block (`cmd/quarry/run_status_test.go:98-103`); U9 example `unknown command "spend"` (`cmd/quarry/run_usage_test.go:178`, 2a spec U9 row); `root.go:8` doc comment.
- Warning prefixes today: accounts `"quarry: "` (`internal/cli/accounts.go:41`), sql `"quarry: warning: "` (`sql.go:88`), status none.
- Not existing: date/range parsing, `--account` resolution, `--by` grouping, period bucketing, per-currency grouping.
- v9fixture expresses accounts (type, currency, closed, active), categories with kinds/parents/hidden, tags, payees, transactions with posted/entered dates, entries with transfers; it lacks `ZEXCLUDEFROMREPORTS`, `ZUSEDINREPORTS` fields (to add). `store.Rows` + `newBuiltStore` express view-rule tests directly (preferred for date-boundary tests).
- **Already exists — do not re-plan:** the read-command shape and refusal copy (`internal/cli/output.go`, `internal/report/refusal.go`, `noArgs`, `resolveHome`, U9), `marshalDocument`, `formatMoney`, `humanize`, the transfers table and pairing, the view-in-store mechanism, category hierarchy/kinds/tags/payees/currency.

## Product Verdict

**SHIP WITH CHANGES** (accepted by the user 2026-09-29):
1. Importer slice first: `excluded_from_reports` (required; probe P2 found 0 today, kept so quarry stays right if the box is ever ticked), `in_reports` (P2 found 1 account), register-date basis (P2: 150 transactions' posted/entered days differ, 16 in different months; user confirmed the register shows entered), Uncategorized normalisation (P1).
2. `ZTARGETACCOUNT`/`ZSENDACCOUNT`: P2 found 0 → no precondition fix.
3. `FormatVersion` 2 → 3.
4. `v_spending` selects from `v_cash_flow` (one predicate owner); the spend = cashflow-spent invariant has its own scenario.
5. One warning prefix `quarry: warning: `.
6. Not in 2b: `--currency`, roll-up, default-window config, split counts in output.
7. Folds: 2a sql Long layout NIT (blank lines around the indented example); U9 example `spend` → `spending`.

## Surface & Copy

Implement verbatim.

### `spend`
- Use: `spend`; Short: `Show spending by category, payee, tag or month`
- Long:
```
Show how much you spent, grouped by category, payee, tag or month, in each
account's own currency: CAD and USD are listed separately, never added
together.

Spending is every split in an expense category, plus uncategorized splits
that take money out. Refunds in an expense category are netted against it,
so a category can come out negative. Transfers between your own accounts,
splits in Quicken's system categories and transactions marked "exclude from
reports" in Quicken are left out. So are accounts Quicken leaves out of
reports (quarry accounts marks them "not in reports") and accounts that use
Quicken's linked account tracking (marked "linked tracking"). Closed
accounts are included.

The period runs from --since to --until, both included; a bare year or month
covers all of it (--since 2024 --until 2024 is the whole of 2024). Without
them it is this year up to today, so future-dated transactions are left out
unless --until is later than today.

A split with more than one tag counts under each of them, so with --by tag
the rows can add up to more than the total.
```
- Example:
```
  quarry spend
  quarry spend --by payee --since 2025-01 --until 2025-03
  quarry spend --since 2024 --until 2024 --json
  quarry spend --account "Visa Infinite" --account Chequing
```
- Flags (backticked word = placeholder):

| Flag | Help string | Renders as |
|---|---|---|
| `--by` | ``group spending by `group`: category, payee, tag or month`` | `--by group ... (default "category")` |
| `--since` | ``count transactions dated on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)`` | `--since date` |
| `--until` | ``count transactions dated on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today)`` | `--until date` |
| `--account` | ``count only the account with this `name` or id; repeat for more`` | `--account name` |

- stdout (exit 0): caption, blank line, table; columns two spaces apart; Spent right-aligned via `formatMoney`; no trailing spaces.
```
Spending 2026-01-01 to 2026-09-29 in all accounts

Category             Currency      Spent
(uncategorized)      CAD          412.08
Auto:Fuel            CAD        1,204.50
Food:Groceries       CAD        8,412.33
Food:Groceries       USD          312.10
Total                CAD       31,204.18
Total                USD        2,110.00
```
- Caption with `--account`: `Spending 2026-01-01 to 2026-09-29 in Chequing, Visa Infinite` (names in resolved order as given).
- Column 1 header by `--by`: `Category` (full_path; `(uncategorized)` for NULL); `Payee` (`(no payee)` for NULL); `Tag` (`(no tag)` for untagged splits); `Month` (`2026-01`, plus a trailing `Status` column, always present (header-only table included); `partial` or empty, Total rows empty; no trailing spaces; JSON key stays `partial`).
- Sort: category and tag `lower(name), name, currency`, synthetic buckets first; payee `currency, spent DESC, lower(payee)`; month `month, currency`. Total rows last, one per currency present, CAD before USD.
- `--json`:
```
{"since":"2026-01-01","until":"2026-09-29","by":"category","account_filter":[],
 "rows":[{"category":"Food:Groceries","currency":"CAD","spent":"8412.33"}],
 "totals":[{"currency":"CAD","spent":"31204.18"}],"warnings":[]}
```
  The row key is the `--by` value (`category`|`payee`|`tag`|`month`), `null` for synthetic buckets; month rows add `"partial":bool`; `account_filter` is `[{"id","name"}]` of resolved accounts, `[]` = all; `spent` a 2-decimal string; `rows`/`totals` `[]` when empty, never null.

### `cashflow`
- Use: `cashflow`; Short: `Show income, spending and savings rate by month or year`
- Long:
```
Show income, spending and what was left over for each month or year, in
each account's own currency: CAD and USD are listed separately, never added
together.

Income and spending follow the same rules as quarry spend: transfers between
your own accounts, Quicken's system categories and transactions marked
"exclude from reports" are left out, and refunds are netted. Accounts Quicken
leaves out of reports ("not in reports" in quarry accounts) and accounts
that use Quicken's linked account tracking ("linked tracking") are left out
here too. Uncategorized splits count as income when they bring money in and
as spending when they take money out. The Spent column equals quarry spend's
total for the same period and accounts.

Savings rate is net divided by income, and shows n/a when income is zero or
less. A period that --since or --until cuts short is marked partial.
```
- Example:
```
  quarry cashflow
  quarry cashflow --by year --since 2020 --until 2025
  quarry cashflow --account Chequing --json
```
- Flags: `--by` ``group by `period`: month or year`` (default `"month"`); `--since`, `--until`, `--account` — same strings as `spend`. One shared parser.
- stdout (exit 0):
```
Cash flow 2026-01-01 to 2026-09-29 in all accounts

Month    Currency     Income      Spent       Net  Savings rate  Status
2026-01  CAD        9,100.00   6,200.00  2,900.00         31.9%
2026-09  CAD        6,020.00   5,110.40    909.60         15.1%  partial
Total    CAD       61,410.00  44,002.18  17,407.82        28.3%
```
- Copy details (ruling, SCENARIO-20):
  - Column 1 header `Month` (`--by month`) / `Year` (`--by year`); labels `2026-01` / `2026`, same in JSON `"period"`; Total rows show `Total`.
  - `Savings rate`: one decimal + `%`, half away from zero (250/800 → `31.3%`), right-aligned; `n/a` right-aligned, no `%`, wherever that row's income ≤ 0 (incl. empty filled periods and Total rows); JSON `null` wherever text shows `n/a`.
  - Negative rates `-12.5%`; negative zero never appears — a rate rounding to zero prints `0.0%` / JSON `0` even when net < 0; fixed in the store query that owns the rate (P2b-9).
  - Large rates grouped in text (`-9,990.0%`); JSON `savings_rate_pct` plain number (`-9990`, `31.9`).
  - Header row `Month|Year  Currency  Income  Spent  Net  Savings rate  Status`; Status last, no trailing spaces, empty on Total rows.
  - Rates per currency, never across currencies.
  Every period in the window is listed for each currency with any row in the window; empty periods print `0.00` and `n/a`. Status column last. Total rows never partial.
- `--json`:
```
{"since","until","by":"month","account_filter":[],
 "periods":[{"period":"2026-01","currency":"CAD","income":"9100.00","spent":"6200.00","net":"2900.00","savings_rate_pct":31.9,"partial":false}],
 "totals":[{"currency":"CAD","income","spent","net","savings_rate_pct"}],"warnings":[]}
```
  `savings_rate_pct` a JSON number or null; a year period is `"2026"`.

### Refusals and outcomes (both commands)
Read errors reuse `readRefusal`: R1, R2 (with its `--from` fix), R3a/b/c, R3, I1 (`quarry: spend interrupted` / `quarry: cashflow interrupted`), O2, H1 — texts as in the 2a spec.

| # | Condition | stderr | Exit |
|---|---|---|---|
| S1 | bad date | `quarry: --since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` (same for `--until`) | 2 |
| S2 | since after until (resolved dates compared; values printed as given) | `quarry: --since 2025 is after --until 2024` | 2 |
| S2d | `--until` given, `--since` not, and resolved until is before the default since | `quarry: --until 2024 is before the default --since 2026-01-01; pass --since too` (until as given, default since resolved YYYY-MM-DD). Check order S1 → S3 → S2/S2d; no mirror case (S3 owns since-after-today) | 2 |
| S3 | since after today and no `--until` | `quarry: --since 2027 is after today; pass --until to include future-dated transactions` | 2 |
| S4 | bad `--by` | spend: `quarry: --by must be category, payee, tag or month`; cashflow: `quarry: --by must be month or year` | 2 |
| S5 | unknown account | `quarry: no account named "Chequeing"; run quarry accounts --all to list them` | 1 |
| S6 | ambiguous name | `quarry: 2 accounts are named "Visa"; pass one of their ids instead: acct-812, acct-977` (ids sorted) | 1 |
| U8 | positional args | `quarry: spend takes no arguments` / `quarry: cashflow takes no arguments` | 2 |
| E1 | nothing in the window, store has transactions | exit 0; stdout caption, blank line, header only (JSON `rows`/`periods` `[]` for spend; cashflow per its fill rule); stderr `quarry: warning: no spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2026-09-26` (cashflow: `no income or spending from …`) | 0 |
| E2 | store has zero transactions | as E1, tail `; the store has no transactions` | 0 |
| E1a | as E1 but `--account` given and some named account is reported (P2b-6) | `quarry: warning: no spending from 2026-01-01 to 2026-09-29 in the named accounts; their transactions run 2019-03-02 to 2024-11-30` (cashflow: `no income or spending from …`); range covers the reported named accounts only | 0 |
| E2a | as E1a but none of them has any transactions | `…in the named accounts; they have no transactions` | 0 |
| W1 | `--by tag` and > 0 splits in the window carry more than one tag | `quarry: warning: 12 splits carry more than one tag, so the rows add up to more than the total` (`humanize.Count`; singular `1 split carries`) | 0 |
| W2 | an `--account` resolves to an account with `in_reports = false` (once per such account) | `quarry: warning: account "<name>" is not used in reports in Quicken, so <cmd> leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync` (`<cmd>` = `spend`/`cashflow`); if every named account is excluded the result is empty plus W2 | 0 |
| W3 | an `--account` resolves to an account with `linked_tracking = true` (once per such account) | `quarry: warning: account "<name>" uses linked account tracking in Quicken, so <cmd> leaves it out, as Quicken's reports do` (`<cmd>` = `spend`/`cashflow`; `<name>` = stored name). No fix clause. An account both `in_reports = false` and linked gets W3 only, never W2. W2/W3 lines interleave in argv order, one per account, before W1 and E | 0 |

### Edge-case rows
| Input class | Result |
|---|---|
| no store / format-2 store | R1 / R2 |
| stale store | no warning (2a precedent) |
| category, tag or payee row nets to 0.00 (void, full refund) | omitted from rows; Total unaffected |
| month/year row with no activity | printed `0.00` (series filled); `spend --by month` fills too |
| positive uncategorized split | cashflow Income only; never in `spend` |
| net-refund category | printed negative |
| USD-only window | only USD rows and a USD Total |
| closed account | included; matchable by `--account` |
| account not in reports, no `--account` | absent, no warning |
| account not in reports named by `--account` (incl. closed) | W2 only |
| linked account, no `--account` | absent, no warning |
| linked account named by `--account` (incl. closed) | W3 only |
| named account both not in reports and linked | W3 only (no W2) |
| future-dated, default window | excluded; `--until` past today includes it |
| split with two tags | both tag rows, once in Total; W1 |
| window on day boundaries (`--since 2026-01-15`) | first period `partial` |
| S1–S6 / U8 with W2 or W3 | the refusal only; no W2/W3 |
| every `--account` left out — not in reports or linked, any mix (after dedupe) | W2/W3 lines only (argv order); no E1/E2/E1a/E2a (even with zero transactions); exit 0 |
| some `--account` left out, rest empty in window | W2/W3 lines (argv order, each account once) then E1a/E2a over the reported named accounts; same order in `warnings[]` |

### Changes to 2a surfaces
- `accounts` all-closed note (2a Surface & Copy): stderr becomes `quarry: warning: all 3 accounts are closed; pass --all to list them` / `quarry: warning: the only account is closed; pass --all to list it`; `warnings[]` text unchanged.
- `accounts` Status column: append `not in reports` for `in_reports = false`, joined with `, ` — open+active `not in reports`; inactive `inactive, not in reports`; closed (with `--all`) `closed, not in reports`. `--json` adds `"in_reports": true|false` after `"active"`.
- `accounts` Status column (Gate ruling): parts joined with `, ` in fixed order state (`closed`/`inactive`), `not in reports`, `linked tracking` — e.g. `linked tracking`, `inactive, linked tracking`, `closed, linked tracking`, `not in reports, linked tracking`, `closed, not in reports, linked tracking`. `--json` adds `"linked_tracking": true|false` directly after `"in_reports"`.
- U9 example: `unknown command "spend"` → `unknown command "spending" for "quarry"; Run 'quarry --help' for usage.`; root Available Commands gains `cashflow` and `spend` (cobra alphabetical order).
- `sql` Long: blank lines around the indented example:
```
that starts with - (such as a -- comment) goes after --:

  quarry sql -- "-- monthly totals
  SELECT ..."

Amounts are DECIMAL(18,2) ...
```
- `v_cash_flow` column/relation note for `describe_schema` (future MCP): "excludes accounts where accounts.in_reports is false or accounts.linked_tracking is true, as Quicken reports do." The `COMMENT ON VIEW v_cash_flow` matches.

---

## Gate

Runs after `/run-reviewers` passes and before the final product-vision pass; the final pass cannot SHIP with Gate rows empty.

**User supplies**, for each of 2024 and 2025 (Jan 1 – Dec 31), twice — once with only CAD accounts selected, once with only USD accounts (Quicken shows USD accounts in USD — user-confirmed):
- (a) Quicken "Spending by Category": custom date range for that year; closed accounts included; transfers excluded; subcategories shown; default "exclude from reports" handling.
- (b) Quicken income/expense (cash flow) by month, same settings.
Exported as CSV, kept outside the repo; no private data is checked in.

**Pass rule**: per currency, `quarry spend --since Y --until Y` rows equal (a) to the cent for every leaf category and the total; `quarry cashflow --since Y --until Y` Income and Spent equal (b) for each month. Every difference is recorded below (year, currency, row, quarry, Quicken, cause) and classified as a quarry defect (fix pass), a rule change (product-vision amends P2b-n), or a Quicken data fix (2d finding, re-checked after next sync). The Gate passes when no difference is unexplained.

| Year | Currency | Row | quarry | Quicken | Cause |
|---|---|---|---|---|---|
| 2024–25 | CAD | every month × category (547 cells) | = | = | Match to the cent once Quicken's CAD selection includes RBC Cash Back Mastercard. The CAD export (7 accounts) left the card out. Its own export (the first USD attempt) equals quarry's RBC rows cell for cell. Export setup, not a difference. |
| 2024–25 | CAD | Investments:Dividend Income (8 months, 3,131.36) | 0.00 | 3,131.36 | Investment transactions are not imported (Phase 1 scope; `sync` counts them as not imported). Rule: out of scope until investment transactions are imported. |
| 2024–25 | USD | Outflows, every month | = | = | Total Outflows equals `cashflow` Spent in all 24 months. |
| 2024–25 | USD | Investments:Dividend Income / Realized Gain/Loss / Buy / Sell | 0.00 | 86,412.60 / 918.69 / −378,999.14 / 37,817.86 | Same cause: investment transactions are not imported. Buy/Sell sit under Quicken's "Other" group and are outside Inflows/Outflows. |
| 2025 | USD | Uncategorized income, Netskope 401(k) (Mar 20,187.15; Jun 511.81; Sep 419.29; Dec 1,415.47) | 22,533.72 | 0.00 | **Rule change.** Netskope 401(k) uses Quicken's linked account tracking (`ZACCOUNT.ZSIMPLEINVESTING = 1`, the only such account; the user can't see its transactions in Quicken). Quicken's reports leave out the cash rows it downloads. With the account excluded, USD matches in every month × category cell except the investment-action rows. Needs a product-vision ruling on P2b-6, then a fix pass. |

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — sync records which transactions Quicken leaves out of reports
  Given a Quicken bundle with one transaction marked exclude-from-reports and one not
  When I run quarry sync
  Then transactions.excluded_from_reports is true for the first and false for the second
```

```gherkin
Scenario: SCENARIO-02 — sync records which accounts Quicken uses in reports
  Given a Quicken bundle with one account whose use-in-reports setting is off, one on, and one unset
  When I run quarry sync
  Then accounts.in_reports is false, true and true respectively
```

```gherkin
Scenario: SCENARIO-03 — sync dates each transaction by its register date
  Given a Quicken bundle with a transaction entered 2026-06-01 and posted 2026-05-31, and one with no posted date
  When I run quarry sync
  Then transactions.date is 2026-06-01 with posted_date 2026-05-31 for the first, and the entered date with posted_date NULL for the second
```

```gherkin
Scenario: SCENARIO-04 — sync stores splits on Quicken's Uncategorized category as uncategorized
  Given a Quicken bundle with a split on the system category "Uncategorized"
  When I run quarry sync
  Then that split's category_id is NULL
```

```gherkin
Scenario: SCENARIO-05 — spend refuses a store built by an older quarry
  Given a store at format version 2
  When I run quarry spend
  Then stderr is R2 naming "quarry sync --from <id>" and the exit code is 1
```

```gherkin
Scenario: SCENARIO-06 — v_cash_flow keeps only real income and spending
  Given a store with an expense split, an income split, both legs of a transfer, an unmatched transfer leg, a split in a system category, a split on an excluded transaction, and positive and negative uncategorized splits
  When I query v_cash_flow with quarry sql
  Then it holds the expense split as expense, the income split and the positive uncategorized split as income, the negative uncategorized split as expense, and none of the others
```

```gherkin
Scenario: SCENARIO-07 — spending leaves out accounts Quicken does not use in reports
  Given a store with spending in an account whose in_reports is false and in one whose in_reports is true
  When I run quarry spend
  Then only the in-reports account's spending is counted
```

```gherkin
Scenario: SCENARIO-08 — v_spending nets refunds against their category
  Given a store with a 100.00 purchase and a 30.00 refund in the same expense category, and a category whose refunds exceed its purchases
  When I query v_spending with quarry sql
  Then the first category sums to 70.00 spent and the second to a negative amount
```

```gherkin
Scenario: SCENARIO-09 — spend shows this year's spending by category in each currency
  Given a store with CAD and USD spending this year, spending last year, and a future-dated purchase
  When I run quarry spend
  Then stdout shows the caption, one row per category and currency, and one Total row per currency, excluding last year and the future-dated purchase
```

```gherkin
Scenario: SCENARIO-10 — spend groups by payee
  Given a store with spending at several payees and a transaction with no payee
  When I run quarry spend --by payee
  Then rows are per payee and currency sorted by currency then spent descending, with "(no payee)" for the missing one
```

```gherkin
Scenario: SCENARIO-11 — spend groups by tag and warns about multi-tagged splits
  Given a store with a split carrying two tags and an untagged split
  When I run quarry spend --by tag
  Then the two-tag split counts under both tags and once in the Total, the untagged one is "(no tag)", and stderr carries W1
```

```gherkin
Scenario: SCENARIO-12 — spend groups by month and fills empty months
  Given a store with spending in January and March only
  When I run quarry spend --by month --since 2026-01-15 --until 2026-03
  Then rows cover 2026-01, 2026-02 (0.00) and 2026-03, with 2026-01 marked partial
```

```gherkin
Scenario Outline: SCENARIO-13 — spend counts the whole period it is given
  Given a store with spending on 2024-01-01, 2024-12-31 and 2025-01-01
  When I run quarry spend --since <since> --until <until>
  Then the Total is <total>

  Examples:
    | since      | until      | total                         |
    | 2024       | 2024       | 2024-01-01 + 2024-12-31       |
    | 2024-12    | 2025-01    | 2024-12-31 + 2025-01-01       |
    | 2024-12-31 | 2024-12-31 | 2024-12-31 only               |
```

```gherkin
Scenario: SCENARIO-14 — spend counts only the accounts it is given
  Given a store with spending in Chequing, Visa Infinite (closed) and Savings
  When I run quarry spend --account chequing --account acct-<visa id> --account Chequing
  Then only Chequing and Visa Infinite are counted and the caption names them once each
```

```gherkin
Scenario: SCENARIO-15 — spend says why a named account shows nothing
  Given a store whose account "Old Card" has in_reports false
  When I run quarry spend --account "Old Card"
  Then the result is empty, stderr carries W2 naming "Old Card", and the exit code is 0
```

```gherkin
Scenario: SCENARIO-16 — spend --json returns spending as a document
  Given a store with CAD spending in two categories
  When I run quarry spend --json
  Then stdout has since, until, by, account_filter, rows keyed by "category", totals and warnings, with amounts as 2-decimal strings
```

```gherkin
Scenario Outline: SCENARIO-17 — spend says when the period holds nothing
  Given <store>
  When I run quarry spend --since 2026-01 --until 2026-02
  Then stdout is the caption and header only, stderr carries <warning>, and the exit code is 0

  Examples:
    | store                                           | warning |
    | a store with transactions only in 2003–2025     | E1      |
    | a store with no transactions                    | E2      |
```

```gherkin
Scenario Outline: SCENARIO-18 — spend rejects a period it cannot use
  Given any state
  When I run quarry spend <args>
  Then stderr is <refusal>, stdout is empty and the exit code is 2

  Examples:
    | args                            | refusal |
    | --since 2024-13                 | S1      |
    | --until yesterday               | S1      |
    | --since 2025 --until 2024       | S2      |
    | --since 2099                    | S3      |
    | --by vendor                     | S4      |
    | extra                           | U8      |
```

```gherkin
Scenario Outline: SCENARIO-19 — spend refuses an account it cannot pick
  Given a store with two accounts named "Visa" and one named "Chequing"
  When I run quarry spend --account <name>
  Then stderr is <refusal>, stdout is empty and the exit code is 1

  Examples:
    | name       | refusal |
    | Chequeing  | S5      |
    | Visa       | S6      |
```

```gherkin
Scenario: SCENARIO-20 — cashflow shows income, spending and savings rate by month
  Given a store with CAD income and spending this year and a month with income but no spending
  When I run quarry cashflow
  Then stdout shows one row per month and currency with Income, Spent, Net and Savings rate, empty months filled, the current month partial, and a Total per currency
```

```gherkin
Scenario: SCENARIO-21 — cashflow groups by year
  Given a store with income and spending across 2020–2025 and a year with no income
  When I run quarry cashflow --by year --since 2020 --until 2025
  Then there is one row per year and currency, and the no-income year shows n/a for savings rate
```

```gherkin
Scenario: SCENARIO-22 — cashflow --json returns cash flow as a document
  Given a store with CAD income and spending
  When I run quarry cashflow --json
  Then stdout has since, until, by, account_filter, periods, totals and warnings, with savings_rate_pct a number or null
```

```gherkin
Scenario: SCENARIO-23 — cashflow's spending equals spend's total
  Given a store with CAD and USD spending, refunds, transfers, uncategorized splits and an excluded account
  When I run quarry spend and quarry cashflow for the same period and accounts
  Then each currency's cashflow Total Spent equals spend's Total for that currency
```

```gherkin
Scenario Outline: SCENARIO-24 — cashflow refuses and reports empty periods like spend
  Given <state>
  When I run quarry cashflow <args>
  Then stderr is <outcome> and the exit code is <exit>

  Examples:
    | state                              | args                        | outcome | exit |
    | any state                          | --since 2024-13             | S1      | 2    |
    | any state                          | --by week                   | S4      | 2    |
    | a store with two "Visa" accounts   | --account Visa              | S6      | 1    |
    | a store with transactions to 2025  | --since 2026-01 --until 2026-02 | E1  | 0    |
```

```gherkin
Scenario: SCENARIO-25 — accounts marks its all-closed note as a warning
  Given a store whose accounts are all closed
  When I run quarry accounts
  Then stderr is "quarry: warning: all 3 accounts are closed; pass --all to list them" and warnings[] under --json is unchanged
```

```gherkin
Scenario: SCENARIO-26 — accounts marks accounts Quicken leaves out of reports
  Given a store with an open account, an inactive account and a closed account, each with in_reports false
  When I run quarry accounts --all
  Then their Status reads "not in reports", "inactive, not in reports" and "closed, not in reports", and --json carries "in_reports": false
```

Added after the Gate (linked account tracking, user-approved 2026-09-29):

```gherkin
Scenario: SCENARIO-27 — spending leaves out accounts that use Quicken's linked account tracking
  Given a store with an expense and an uncategorized deposit in an account whose linked_tracking is true, and spending in one whose linked_tracking is false
  When I run quarry cashflow for that period
  Then only the unlinked account's rows are counted, and quarry spend's total equals cashflow's Spent
```

```gherkin
Scenario: SCENARIO-28 — naming a linked-tracking account warns that it is left out
  Given a store whose account "Netskope 401(k)" has linked_tracking true and in_reports false
  When I run quarry spend --account "Netskope 401(k)"
  Then the result is empty, stderr carries only W3 naming "Netskope 401(k)" (no W2, no E line), and the exit code is 0
```

```gherkin
Scenario: SCENARIO-29 — accounts marks linked-tracking accounts
  Given a store with an open linked account, an inactive linked account and a closed account that is linked and not in reports
  When I run quarry accounts --all
  Then their Status reads "linked tracking", "inactive, linked tracking" and "closed, not in reports, linked tracking", and --json carries "linked_tracking": true after "in_reports"
```

---

## Sizing
| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN (run 1) — 4 batches (store fields + schema columns + FormatVersion 2→3 + v9fixture fields / report flags import / register-date basis + posted_date / Uncategorized → NULL), 1 feature package (importer); absorbs 02, 03, 04; owns the only 2b format bump |
| SCENARIO-02 | FOLD into SCENARIO-01 — same column-import shape as 01, a handful of lines |
| SCENARIO-03 | FOLD into SCENARIO-01 — date pick + ORDER BY + one nullable column; no existing test sets `EnteredDate` |
| SCENARIO-04 | FOLD into SCENARIO-01 — a few lines in the split category mapping |
| SCENARIO-05 | FOLD into SCENARIO-09 — test only: R2 comes free once spend calls `readRefusal` over the format check |
| SCENARIO-06 | OWNS A RUN (run 3) — 2 batches (v_cash_flow DDL + rule tests / v_spending + `storeRelations` + `minimalRows`), duckstore only; absorbs 08; owns every spending rule, so mutations must be named |
| SCENARIO-07 | FOLD into SCENARIO-09 — the `in_reports` predicate is built in 06's view; 07's spend-level test is coverage only |
| SCENARIO-08 | FOLD into SCENARIO-06 — one `CREATE VIEW` selecting from v_cash_flow |
| SCENARIO-09 | OWNS A RUN (run 4) — 4 batches (clock seam + default window / `Spending` port + by-category query + 3 implementers / `(*report.Server).Spend` / cli spend command + text renderer), 1 feature package (report); absorbs 05, 07 and the U9-example fold |
| SCENARIO-10 | LIGHT (run 6) — 2 steps (payee grouping in the Spending query / `--by` flag + Payee header + `(no payee)` + S4 check), report + cli |
| SCENARIO-11 | OWNS A RUN (run 7) — 3 batches (tag grouping with a per-split Total / multi-tag count on the read / W1 phrase + warnings emit), 1 feature package (report); first grouping where rows ≠ Total |
| SCENARIO-12 | OWNS A RUN (run 9) — 3 batches (period series + partial seam reused by cashflow / month grouping + fill / Month + Status render and JSON `partial`), 1 feature package (report); absorbs 18 |
| SCENARIO-13 | OWNS A RUN (run 8) — 2 batches (shared window parser incl. S1–S3 in report / cli `--since`/`--until` before `openReport`), 1 feature package (report); seam reused by 12, 14, 17, 20 |
| SCENARIO-14 | OWNS A RUN (run 10) — 4 batches (account resolver + S5/S6 / account filter on the query / caption + `account_filter` / W2), 1 feature package (report); absorbs 15, 19 |
| SCENARIO-15 | FOLD into SCENARIO-14 — W2 is a few lines on the resolver's result |
| SCENARIO-16 | LIGHT (run 5) — 2 steps (spend JSON document / `renderResult` branch), cli only |
| SCENARIO-17 | LIGHT (run 11) — 2 steps (store date range on the Spending read / E1+E2 phrase + emit, shared with cashflow), report + cli |
| SCENARIO-18 | FOLD into SCENARIO-12 — outline over 13's parser, 10's S4 check and `noArgs`; ticked only once every `--by` value S4 names exists (month lands in 12) |
| SCENARIO-19 | FOLD into SCENARIO-14 — S5/S6 are the resolver's error branches |
| SCENARIO-20 | OWNS A RUN (run 12) — 4 batches (`CashFlow` port + period query + 3 implementers / `(*report.Server).CashFlow` over the shared seams / cli cashflow command + text renderer / cashflow S4 + E1 copy), 1 feature package (report); absorbs 21, 23, 24 |
| SCENARIO-21 | FOLD into SCENARIO-20 — year is the period parameter of the same query and fill; n/a is already needed for 20's empty months |
| SCENARIO-22 | LIGHT (run 13) — 2 steps (cashflow JSON document / `renderResult` branch), cli only |
| SCENARIO-23 | FOLD into SCENARIO-20 — test only; the architect names the fixture cases and the mutations the equality must catch |
| SCENARIO-24 | FOLD into SCENARIO-20 — reuses the parser, resolver and E1 seams; only the cashflow S4/E1 copy is new |
| SCENARIO-25 | FOLD into SCENARIO-26 — one prefix string plus its tests |
| SCENARIO-26 | LIGHT (run 2) — 3 steps (Accounts read carries `in_reports` / Status + JSON `in_reports` / S25 prefix + sql Long NIT), duckstore + cli; absorbs 25 |
| SCENARIO-27 | OWNS A RUN (post-Gate) — 4 batches (import `ZSIMPLEINVESTING` → `linked_tracking` through store + schema + fixture / one SQL "reported" definition shared by the `v_cash_flow` predicate, the E1a/E2a range query and the COMMENT / Accounts read + `Account.LeftOutOfReports` + W3 / accounts Status + JSON + spend/cashflow Long), 1 feature package (importer) plus store/duckstore/cli; absorbs 28, 29; owns the "reported" rule, so mutations must be named |
| SCENARIO-28 | FOLD into SCENARIO-27 — its Accounts-read column and `LeftOutOfReports` are what 27's W3 batch builds; W3 is one branch in `leftOutWarnings` |
| SCENARIO-29 | FOLD into SCENARIO-27 — a status part and one JSON field over the Accounts read 27 already widens; the new Long text says "linked tracking" and that is only true once 29's Status lands |

## BDD Acceptance Progress
- [x] SCENARIO-01: sync records which transactions Quicken leaves out of reports — `cmd/quarry/run_sync_reports_test.go` `Test_run_sync_records_which_transactions_are_excluded_from_reports`
- [x] SCENARIO-02: sync records which accounts Quicken uses in reports — delivered by SCENARIO-01 — `cmd/quarry/run_sync_reports_test.go` `Test_run_sync_records_which_accounts_are_used_in_reports`
- [x] SCENARIO-03: sync dates each transaction by its register date — delivered by SCENARIO-01 — `cmd/quarry/run_sync_reports_test.go` `Test_run_sync_dates_each_transaction_by_its_register_date`
- [x] SCENARIO-04: sync stores splits on Quicken's Uncategorized category as uncategorized — delivered by SCENARIO-01 — `cmd/quarry/run_sync_reports_test.go` `Test_run_sync_stores_uncategorized_splits_with_no_category`
- [x] SCENARIO-05: spend refuses a store built by an older quarry — delivered by SCENARIO-09 — `cmd/quarry/run_read_refusals_test.go` `Test_run_spend_refuses_a_store_built_by_an_older_quarry`
- [x] SCENARIO-06: v_cash_flow keeps only real income and spending — `cmd/quarry/run_sql_views_test.go` `Test_run_sql_cash_flow_keeps_only_real_income_and_spending`
- [x] SCENARIO-07: spending leaves out accounts Quicken does not use in reports — delivered by SCENARIO-09 — `cmd/quarry/run_spend_test.go` `Test_run_spend_leaves_out_accounts_quicken_does_not_use_in_reports`
- [x] SCENARIO-08: v_spending nets refunds against their category — delivered by SCENARIO-06 — `cmd/quarry/run_sql_views_test.go` `Test_run_sql_spending_nets_refunds_against_their_category`
- [x] SCENARIO-09: spend shows this year's spending by category in each currency — `cmd/quarry/run_spend_test.go` `Test_run_spend_shows_this_years_spending_by_category_in_each_currency`
- [x] SCENARIO-10: spend groups by payee — `cmd/quarry/run_spend_by_test.go` `Test_run_spend_by_payee_groups_spending_by_payee_and_currency_biggest_first`
- [x] SCENARIO-11: spend groups by tag and warns about multi-tagged splits — `cmd/quarry/run_spend_by_test.go` `Test_run_spend_by_tag_counts_a_two_tag_split_under_both_tags_once_in_the_total_and_warns`
- [x] SCENARIO-12: spend groups by month and fills empty months — `cmd/quarry/run_spend_by_test.go` `Test_run_spend_by_month_fills_empty_months_and_marks_a_cut_short_month_partial`
- [x] SCENARIO-13: spend counts the whole period it is given — `cmd/quarry/run_spend_window_test.go` `Test_run_spend_counts_the_whole_period_it_is_given`
- [x] SCENARIO-14: spend counts only the accounts it is given — `cmd/quarry/run_spend_account_test.go` `Test_run_spend_counts_only_the_accounts_it_is_given`
- [x] SCENARIO-15: spend says why a named account shows nothing — delivered by SCENARIO-14 — `cmd/quarry/run_spend_account_test.go` `Test_run_spend_warns_that_a_named_account_is_left_out_of_reports`
- [x] SCENARIO-16: spend --json returns spending as a document — `cmd/quarry/run_spend_json_test.go` `Test_run_spend_json_returns_spending_as_a_document`
- [x] SCENARIO-17: spend says when the period holds nothing — `cmd/quarry/run_spend_empty_test.go` `Test_run_spend_says_when_the_period_holds_nothing`
- [x] SCENARIO-18: spend rejects a period it cannot use — delivered by SCENARIO-12 — `cmd/quarry/run_spend_refusals_test.go` `Test_run_spend_rejects_a_period_it_cannot_use`
- [x] SCENARIO-19: spend refuses an account it cannot pick — delivered by SCENARIO-14 — `cmd/quarry/run_spend_account_test.go` `Test_run_spend_refuses_an_account_it_cannot_pick`
- [x] SCENARIO-20: cashflow shows income, spending and savings rate by month — `cmd/quarry/run_cashflow_test.go` `Test_run_cashflow_shows_income_spending_and_savings_rate_by_month`
- [x] SCENARIO-21: cashflow groups by year — delivered by SCENARIO-20 — `cmd/quarry/run_cashflow_test.go` `Test_run_cashflow_by_year_shows_one_row_per_year_and_na_without_income`
- [x] SCENARIO-22: cashflow --json returns cash flow as a document — `cmd/quarry/run_cashflow_json_test.go` `Test_run_cashflow_json_returns_cash_flow_as_a_document`
- [x] SCENARIO-23: cashflow's spending equals spend's total — delivered by SCENARIO-20 — `cmd/quarry/run_cashflow_invariant_test.go` `Test_run_cashflow_total_spent_equals_spend_total_per_currency`
- [x] SCENARIO-24: cashflow refuses and reports empty periods like spend — delivered by SCENARIO-20 — `cmd/quarry/run_cashflow_refusals_test.go` `Test_run_cashflow_refuses_and_reports_empty_periods_like_spend`
- [x] SCENARIO-25: accounts marks its all-closed note as a warning — delivered by SCENARIO-26 — `cmd/quarry/run_accounts_test.go` `Test_run_accounts_says_how_to_list_them_when_every_account_is_closed`
- [x] SCENARIO-26: accounts marks accounts Quicken leaves out of reports — `cmd/quarry/run_accounts_test.go` `Test_run_accounts_all_marks_accounts_left_out_of_reports`
- [x] SCENARIO-27: spending leaves out accounts that use Quicken's linked account tracking — `cmd/quarry/run_cashflow_linked_test.go` `Test_run_cashflow_leaves_out_accounts_that_use_linked_account_tracking`
- [x] SCENARIO-28: naming a linked-tracking account warns that it is left out — delivered by SCENARIO-27 — `cmd/quarry/run_spend_account_test.go` `Test_run_spend_warns_that_a_named_linked_tracking_account_is_left_out`
- [x] SCENARIO-29: accounts marks linked-tracking accounts — delivered by SCENARIO-27 — `cmd/quarry/run_accounts_test.go` `Test_run_accounts_all_marks_accounts_that_use_linked_account_tracking`
