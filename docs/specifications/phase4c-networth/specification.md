# Specification: Phase 4c — net worth

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: the user can ask "what am I worth today, on a day, or month by month?" and get one answer from `quarry networth`, MCP `net_worth`, `v_net_worth` and `v_balances_daily`. Investment accounts are valued as their cash plus their holdings, computed by quarry, and that cash also feeds cash flow.

**Secondary Goals**:
- One ledger: cash moved by investment transactions gets rows in `transactions`/`splits` (PRD L125), so `sum(transactions.amount)` is an account's cash in every reader.
- One balance owner: `v_balances_daily`. `v_account_balances` is that view at `current_date`, so `quarry accounts` and `quarry networth` cannot disagree per account.
- The Phase-4 view pin and SKILL.md's net-worth "not covered" line retire.

**Out of Scope**:
- A sync-time investment-cash gate. Quicken keeps no cash balance of its own (`ZAVAILCASH`/`ZONLINEBANKINGLEDGERBALANCEAMOUNT` are the broker's figures); investment reconcile records stay skipped (`statements.go:54`) until their meaning is known.
- `--account`/`--type` on `networth` (net worth filtered by account is not net worth).
- Registered-account classification (4d), `quarry acb` (4e), MCP `acb` and the `net-worth.md`/`investments.md` skill references (4f), monthly summary (4g).

**User decisions (2026-10-04)**:
1. Probe the snapshot: yes (results in *Triage Brief*).
2. Cash flow: dividends, interest and capital-gain distributions count as income; buys, sells and share moves never count as spending or income. Cash-flow income totals rise.
3. `quarry networth`: default today's net worth by account type and currency; `--since/--until` gives month-end history.
4. One spec for all of 4c.
5. Net worth leaves out accounts Quicken marks "not in reports" or "linked tracking" (same rule as cash flow).
6. MCP `net_worth` moves from 4f into 4c (LIGHT).
7. The reference check includes one manual comparison: the user compares quarry's computed cash for the 9 investment accounts with Quicken.
8. Scenarios approved 2026-10-04; S01 and S14 splits approved the same day.
10. Reinvested dividends (0.00 in Quicken) stay out of income; no estimated amount (N-3 (b), 2026-10-04).
9. Investment account value = holdings + cash, computed by quarry (4b decision 1).

## Business Rules & Invariants

- **N-1 Investment cash rows.** Each investment transaction with amount ≠ 0 gets a row in `transactions`:
  - new column `transactions.investment_transaction_id` = the investment transaction's id (`itxn-<Z_PK>`); NULL for a register entry;
  - ids are deterministic from source ids (`ZTRANSACTION.Z_PK` is unique across entities, so the existing transaction id format holds; a split id derives from the entry Z_PK);
  - `date` = `investment_transactions.date`; status from `ZRECONCILESTATUS`; `excluded_from_reports` from `ZEXCLUDEFROMREPORTS`; payee NULL; memo copied;
  - one split per Quicken entry, with the entry's category, amount and transfer target; splits go through the existing transfer pairing and splits-sum check;
  - add_shares, remove_shares, split (amount 0) get no row;
  - reinvest_dividend: no row (N-3 outcome (b)).
  - `FormatVersion` 7 → 8; an older store gets the existing format refusal; a re-sync rebuilds it.
- **N-2 Cash-flow class = the entry category's kind** (the existing rule, `schema.go:225-227`); no action table. Expected per action (asserted on the real file in the reference scenario):

  | action | cash | row? | flow |
  | --- | --- | --- | --- |
  | buy | − (system) | yes | neither |
  | sell | + (system) | yes | neither |
  | add_shares, remove_shares, split | 0 | no | — |
  | dividend, interest, capital_gain_long, capital_gain_short | + (income) | yes | income |
  | reinvest_dividend | 0 | no (N-3 (b)) | — (not income: Quicken records 0.00) |
  | margin_interest | − (expense) | yes | spending (reaches `quarry spend`, recurring, anomalies) |
  | misc_expense | − | yes | by entry: expense → spending, system → neither |
  | misc_income | + | yes | by entry: income → income, system → neither |
  | commission | inside the buy/sell amount | — | never spending (part of the trade) |

  - Entries with no category count by sign, like any uncategorized split, and raise the existing `uncategorized` finding.
  - Entries with a transfer target pair as transfers and leave cash flow.
  - An investment transaction with no entry gets one NULL-category split.
- **N-3 Reinvest probe (architect runs first, counts only).**
  - (a) Entry amount X ≠ 0: a 0.00 row with splits +X (the entry's income category) and −X (the system category the file's buy entries carry). It counts as income.
  - (b) X = 0, or no single system category: no row. The cashflow sentence drops "reinvested dividends included", and the orchestrator tells the user before building.
  - **Outcome (2026-10-04 probe): (b).** All 5 reinvest entries are 0.00 (shares non-zero); buys carry exactly 1 system category; 0 investment entries name a transfer account; 0 investment transactions lack an entry; 0 entries lack a category; every entry amount equals its transaction's amount; commission is inside the buy amount (22 of 22). User decision 10: reinvested dividends stay out of income.
- **N-4 Findings.** `duplicateQuery` and `unlinkedTransferQuery` add `investment_transaction_id IS NULL` on both sides. The Rows line keeps counting table rows.
- **N-5 `v_balances_daily`.**
  - Grain: one row per account per calendar day, from its first transaction or holding date through today. Closed and left-out accounts included.
  - Columns: `date, account_id, account, type, currency, cash, holdings_value, holdings_unvalued, balance, balance_cad, balance_usd, usd_cad`.
  - `cash` = sum of `transactions.amount` dated on or before `date`, all transactions including `excluded_from_reports` (`reportedTransaction` not used).
  - `holdings_value` is NULL outside brokerage and retirement accounts; otherwise the sum of that day's `v_holdings.value` in the account's currency: own currency as is; the other of CAD/USD converted at the date's rate and rounded; no price, NULL or other currency, or no rate → left out and counted in `holdings_unvalued`; 0.00 when nothing is held.
  - `balance` = cash + coalesce(holdings_value, 0). `balance_cad`/`balance_usd` via `convertedTo` at the ASOF rate for `date`, per account.
  - `COMMENT ON VIEW`: `one row per account per day from its first transaction through today; cash is the sum of its transactions to that day, holdings_value its holdings' value in its own currency (NULL outside brokerage and retirement accounts), balance is cash plus holdings_value, as quarry accounts and quarry networth use; filter by date.`
  - `v_account_balances` = this view at `current_date` (LEFT JOIN, so an account with no rows is 0.00). It gains `cash` and `holdings_value`; its investment CASE (`schema.go:186-187`) is deleted.
- **N-6 `v_net_worth`.**
  - Grain: day × type × currency over accounts where `in_reports AND NOT linked_tracking` (the `reportedAccount` constant, `schema.go:201`).
  - Columns: `date, type, currency, accounts, balance, balance_cad, balance_usd`. Converted columns are sums of the per-account rounded values; NULL when any account in the row has no rate.
  - `COMMENT`: `net worth by day, account type and currency over the accounts Quicken's reports count, as quarry networth does; sum balance_cad or balance_usd over one date for the total; a NULL there means no exchange rate for that day.`
  - **Architect precondition** (like 4b H-2): `WHERE date IN (<≤200 month ends>)` runs in under 1 s on the real store. Fallback: a Go reader under the same column names, after a scoped product re-ruling.
- **N-7 Net-worth definition.** An account counts when `in_reports AND NOT linked_tracking`. Closed accounts count with their balance on the day. Liabilities count with the stored sign (negative = owed), no flip. `home_equity` counts as recorded. Unpriced holdings and unconvertible currencies are left out of the account's balance and warned. Before the first FX rate or with no rates, a USD balance is left out of the CAD total and warned; snapshot mode gives it its own `Total USD` row; native mode is unaffected.
- **N-8 Totals** are sums of the rounded per-account converted values, never added across currencies in native mode.
- **N-9 Dates.** `--as-of`/`--since`/`--until` accept YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day (as-of) per `report.ParseAsOf`/`ResolveAsOf`. History lists each month end in [since, until]; `until` is clamped to today silently; the last row is `until` itself when it is not a month end. A future-dated transaction is not in a balance before its date.

---

## Triage Brief

Triage agent report (2026-10-04) plus the orchestrator probe.

**Builds on (do not re-plan):**
- 4b: `v_holdings` (`internal/store/duckstore/schema.go:256-287`), `holding_shares`, `report.Holdings` totals/`NeedsRate`/`Convertible`, `document.HoldingsWarnings`, `report.ParseAsOf`/`ResolveAsOf` (`internal/report/asof.go`), `quarry holdings` (`internal/cli/{holdings,render_holdings,json_holdings}.go`), MCP `holdings` (`internal/mcp/holdings.go`, `capList` cap 500), `convertedTo`/`convertedToWide` (`internal/store/duckstore/convert_sql.go`), `needSpan` (`rates.go`).
- `accountBalancesViewDDL` (`schema.go:174-208`), `v_cash_flow`/`v_spending` (`schema.go:216-253`; uncategorized splits classified by sign at `:225-227`), view build order (`duckstore.go:454`).
- Importer: `investmentsQuery` (`internal/importer/investments.go:44-52`) reads no reconcile status, payee or entries; cash twin `internal/importer/transactions.go:42-48`; `checkBalances` skips investment accounts (`validate.go:104-127`); `statements.go:54` drops investment reconcile records.
- CLI: `notValuedBalance` (`internal/cli/render_accounts.go:13,80`); Balances clause (`internal/cli/render.go:223`); `reportCurrencyHelp` (`currency.go:13`); `WindowError` (`internal/report/window.go`).
- Phase-4 view pin: `cmd/quarry/run_skill_references_test.go:21,57,100-101` (`phase4ViewPattern`), `run_skill_schema_reference_test.go:182`, `run_skill_drift_names_test.go:222-224`; SKILL.md:73.
- Conventions text hand copies: `internal/report/sql_conventions.go:10`, `internal/cli/sql_test.go:213`, `cmd/quarry/run_shared_documents_test.go:354`.
- Readers of `transactions` affected by new rows: findings, search, filter, status, recurring, anomalies, MCP spending/cash_flow.

**Probe (snapshot 20260930T072052Z, counts only):**
- 9 investment accounts (3 closed). `ZSTATEMENTCLOSINGBALANCE` non-null on 0; `ZONLINEBANKINGLEDGERBALANCEAMOUNT` on 5; `ZNOCASHTRANSACTIONS` on 0.
- `ZFISTATEMENT` covers 6 accounts (6 rows); `ZAVAILCASH` non-null on 6, non-zero on 2.
- Reconcile records on investment accounts: 0.
- Register (Z_ENT 79) transactions in investment accounts: 422 (24 reconciled, 337 cleared, 61 uncleared).
- Investment transactions (Z_ENT 81): 0 reconciled, 0 excluded, 0 with payee. Signs: buy −; sell +; dividend/cap gains/interest/misc_income +; margin_interest/misc_expense −; add/remove/reinvest/split 0. Exactly 1 entry each. Entry category kinds: buy/sell/add/remove/split system; margin_interest expense; dividend/cap gains/interest/reinvest income; misc_expense 1 system + 1 expense; misc_income 1 system + 2 income. 22 buys carry a commission.
- Running cash (register + investment amounts): ends positive in 5 accounts, ~0 in 4; 5 go negative at some point. In all 6 FI-statement accounts the computed final cash is within 1.00 of the latest `ZAVAILCASH`.
- Future-dated investment transactions: 0.

## Product Verdict

**SHIP WITH CHANGES** (product-vision Phase 1, 2026-10-04). Accepted changes:
1. Investment cash goes into `transactions`/`splits` with `investment_transaction_id`; FormatVersion 7 → 8 (N-1).
2. One balance owner: `v_account_balances` = `v_balances_daily` at `current_date` (N-5).
3. Cash-flow class follows the entry's category kind; the user's action rule is asserted by the reference scenario (N-2, N-3).
4. No investment-cash sync gate; Balances clause reworded to what is true.
5. `duplicate` and `unlinked-transfer` exclude investment-cash rows (N-4).
6. MCP `net_worth` in 4c, LIGHT (user decision 6).
7. No `--account`/`--type` on `networth`.

## Surface & Copy

### `quarry networth`

- Use: `networth`
- Short: `Show net worth today or at each month end, by account type and currency`
- Long (verbatim):
```
Show net worth on one day (--as-of, default today), or at the end of each
month from --since to --until: account balances added up by account type
and currency. A balance is the sum of the account's transactions dated
that day or earlier; a brokerage or retirement account adds the value of
its holdings that day, each at the latest price Quicken recorded on or
before it (quarry holdings lists them). Credit card, loan and other
liability balances are negative, so they reduce the total. Closed
accounts count with their balance on the day. Accounts Quicken leaves out
of reports ("not in reports" or "linked tracking" in quarry accounts) are
left out, as Quicken's reports do.

Amounts are in CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
Each account's balance converts at the Bank of Canada rate for the day it
is valued on, or the latest earlier rate, and is rounded to the cent
before it is added. With --currency native, CAD and USD are listed
separately, never added together.

Month ends after today are not listed; a history that reaches this month
ends with today.
```
- Example:
```
  quarry networth
  quarry networth --as-of 2025-12-31
  quarry networth --since 2020 --currency native --json
```
- Flags (backticked word is the placeholder):
  - `--as-of`: `value net worth on `date` (YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day; default today)`
  - `--since`: `list net worth at each month end on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year when --until is given)`
  - `--until`: `list net worth at each month end on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today; a later date means today)`
  - `--currency`: reuses `reportCurrencyHelp` (`currency.go:13`).
  - `--json`: the global flag.

**Text, snapshot (default or `--as-of`):**
```
Net worth on 2026-10-04, amounts in CAD

Type         Currency     Balance      In CAD
brokerage    USD       150,000.00  205,680.00
chequing     CAD        12,345.67   12,345.67
chequing     USD         1,000.00    1,371.20
credit_card  CAD        -2,104.33   -2,104.33
Total                              217,292.54
```
- No "in all accounts" in the caption. Native mode drops ", amounts in X".
- One row per type×currency with at least one counted account; rows whose balance is 0.00 are omitted.
- Sort: type ascending (stored values, as the `quarry accounts` Type column), then CAD, USD.
- Native mode: no In column; one `Total` row per currency with Currency and Balance cells filled, CAD first.
- Converted mode: a row with no rate shows `no rate` in the In column, is left out of the In total, and gets its own `Total <CUR>` row below it (holdings S.2 precedent).

**Text, history (`--since` and/or `--until`):**
```
Net worth at each month end 2026-01-31 to 2026-10-04, amounts in CAD

Month end    brokerage   chequing  credit_card       Total
2026-01-31  198,000.00  10,000.00    -1,500.00  206,500.00
...
2026-10-04  205,680.00  13,716.87    -2,104.33  217,292.54
```
- One column per type non-zero on any listed date, alphabetical; each cell is the converted sum across currencies.
- Native mode: one row per month end per currency: `Month end  Currency  <types…>  Total`.
- Dates per N-9.
- Copy ruling (product-vision, 2026-10-04, SCENARIO-12):
  - A cell with no row for that type (converted) or type × currency (native) on that date is **blank**; a date with no rows has a blank Total, never `0.00`. A cell whose rows sum to zero shows `0.00` (history keeps zero cells; only snapshot text drops zero rows).
  - A type gets a column when its native balance is non-zero on any listed date, in any currency.
  - A converted history cell with no rate is `no rate` (as `convertedCell`); blank always means "no row".
  - Every listed month end gets a line. Native: one line per currency with a row that date, CAD first; a date with no row in any currency gets one line with the date only. A native line's Total is that currency's sum.
  - The caption uses the first and last listed month ends while JSON `since`/`until` carry the resolved since and clamped until.

**`--json`** (one shape for both modes):
```json
{"as_of":"2026-10-04"|null,"since":null|"2026-01-01","until":null|"2026-10-04","currency":"CAD",
 "dates":[{"date":"2026-10-04",
   "balances":[{"type":"chequing","currency":"CAD","balance":"12345.67","converted_balance":"12345.67"|null}],
   "totals":[{"currency":"CAD","value":"217292.54"}]}],
 "warnings":[]}
```
- `as_of` null in history mode; `since`/`until` null in snapshot mode.
- `dates`, `balances`, `totals` are `[]`, never null. `balances` keeps zero rows (only text omits them).
- `converted_balance` null in native mode and when there is no rate.
- `totals`: the reporting-currency total of rows that convert (absent if none), then one total per currency of rows with no rate; native: one per currency, CAD first. Snapshot text prints each as a Total row; history text prints only the reporting-currency one, in the Total column (copy ruling 2026-10-04, SCENARIO-14b).

**Warnings** (stderr, prefix `quarry: warning: `, exit 0, also in `warnings[]`), in this order:
1. Config warnings.
2. Empty result:
   - Copy ruling (product-vision 2026-10-04, SCENARIO-16) replaces the earlier "transactions start" lines:
   - snapshot: `no account has a balance on <d>; the first balance is on <f>`
   - history: `no account has a balance at any month end from <s> to <u>; the first balance is on <f>`
   - no data, snapshot: `no account has a balance on <d>; no account in Quicken's reports has transactions or holdings`
   - no data, history: `no account has a balance at any month end from <s> to <u>; no account in Quicken's reports has transactions or holdings`
   - `<s>`/`<u>` are the first and last listed month ends (as the caption); `<f>` (`FirstBalance`) is the earliest of `transactions.date` and `holding_shares.from_date` over counted accounts (`reportedAccount`); zero → the no-data form. The `<f>` form prints only when `<f>` is after `<u>` (snapshot: after `<d>`); otherwise no empty-result line. Empty = at least one listed date and no row on any listed date; zero-balance rows are rows. Same line in native, CAD and USD.
3. Unpriced holding:
   - snapshot: `"Brokerage" holds 1 security with no price on or before 2026-10-04, so its balance leaves it out; enter a price in Quicken, then run quarry sync`
   - plural: `"Brokerage" holds N securities with no price on or before <d>, so its balance leaves them out; enter prices in Quicken, then run quarry sync`
   - history: `"Brokerage" holds a security with no price on 3 of the month ends listed, so its balance leaves it out on those days; enter prices in Quicken, then run quarry sync`
4. No currency: `"<security>" has no currency in Quicken, so quarry leaves its value out of "<account>"'s balance; set its currency in Quicken, then run quarry sync`
5. Other currency: `"<security>" is priced in EUR, which quarry does not convert, so its value is left out of "<account>"'s balance`
6. Rates (not in native mode):
   - before the first rate: `USD balances on 2012-12-31, before 2013-01-02, the first exchange rate in the store, are not converted to CAD and are left out of the CAD total; pass --currency native to list them`
   - history: `USD balances on 12 month ends before 2013-01-02, the first exchange rate in the store, are not converted to CAD and are left out of the CAD total; pass --currency native to list them`
   - no rates: `the store has no exchange rates, so USD balances are not converted to CAD and are left out of the CAD total; pass --currency native to list them, or run quarry sync to fetch rates`

   - Copy ruling (product-vision 2026-10-04, SCENARIO-14b): reporting in USD uses the symmetric form through `money.NativeOf`:
     - snapshot: `CAD balances on <d>, before <first>, the first exchange rate in the store, are not converted to USD and are left out of the USD total; pass --currency native to list them`
     - history: `CAD balances on <N> month ends before <first>, the first exchange rate in the store, are not converted to USD and are left out of the USD total; pass --currency native to list them`
     - no rates: `the store has no exchange rates, so CAD balances are not converted to USD and are left out of the USD total; pass --currency native to list them, or run quarry sync to fetch rates`
   - The history count uses `humanize.Count(n, "month end", "month ends")` ("on 1 month end before …" accepted). A history run on a store with no rates uses the no-rates variant.
   - History cells: `no rate` only when the type has a row needing a rate and none of its rows convert; a mixed type shows the sum of rows that convert (no marker). A date whose rows all need a rate shows Total `no rate`; blank still means no rows.

Within one kind, warnings follow account name ignoring case. Lines 3–5 are also emitted by `quarry accounts`, from the same composer, with as-of = today.

**Refusals:**

| Input | Line | Exit |
| --- | --- | --- |
| Bad `--as-of` | `quarry: --as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` | 2 |
| `--as-of` after today | `quarry: --as-of 2027-01-01 is after today; net worth is valued up to today only, so pass an earlier --as-of` (`AsOfError` takes a noun; holdings bytes stay identical) | 2 |
| `--since` after today | `quarry: --since 2027-01 is after today; net worth is valued up to today only, so pass an earlier --since` (new `WindowError` kind) | 2 |
| `--as-of` with `--since`/`--until` | `quarry: --as-of cannot be combined with --since or --until; pass --as-of for one day, or --since and --until for month ends` | 2 |
| `--since` after `--until`; not-a-date; `--until` alone before Jan 1 | Existing `WindowError` lines; following "pass --since too" ends in success | 2 |
| `--until` after today | Clamped to today, no line | 0 |
| Bad `--currency`, positional argument | Existing | 2 |
| No store, older format (7) | Existing lines; re-sync rebuilds | 1 |

### `quarry accounts`

- Balance cell for brokerage and retirement accounts: cash plus holdings value today, via `formatMoney`. `notValuedBalance` (`render_accounts.go:13,79-85`) and its now-unreachable nil arm are deleted.
- JSON adds `cash` (never null) and `holdings_value` (null for non-investment accounts). `balance` is never null for investment accounts.
- Long, replace the paragraph at `accounts.go:17-19` with: `Brokerage and retirement accounts' balance is the cash in them plus the value of their holdings today, each at the latest price Quicken recorded (quarry holdings lists them).`
- Long, replace `accounts.go:24-25` with: `quarry does not add balances together here; quarry networth does.`

### Balances line (`render.go:223`; sync, status and the DIFFER line alike)

- `1 investment account's cash not checked` (singular), `N investment accounts' cash not checked` (plural).
- JSON key `investment_accounts` (`json.go:57`) and the `import_runs` column keep their meaning.

### MCP `net_worth`

- Parameters: `as_of`, `since`, `until`, `currency` (reuses `currencySchema`). Result: the same document as `--json`.
- Description: `Net worth on one day (as_of, default today) or at each month end from since to until, by account type and currency; brokerage and retirement accounts count their cash plus their holdings' value.`
- Refusals:
  - `as_of 2027-01-01 is after today; net worth is valued up to today only, so pass an earlier as_of`
  - `as_of cannot be combined with since or until; pass as_of for one day, or since and until for month ends`
  - stderr: `quarry: mcp: net_worth: refused the call's <param>; details went to the client only`
- Cap: 500 `dates`, through `capList`: `net_worth lists the first 500 month ends of 501; pass a later since, or query v_net_worth for the rest`.

### Changes to existing surfaces

| Where | New |
| --- | --- |
| `sql_conventions.go:19-21` ("Investment transactions are in investment_transactions, not in transactions…") | `Each investment transaction that moves cash also has a row in transactions (investment_transaction_id names it; NULL for a register entry), one split per Quicken entry, so an account's cash is the sum of its transactions. In v_cash_flow dividends, interest and capital-gain distributions are income; buys, sells and share moves are neither. investment_transactions holds each one's action, security and shares:` (rest unchanged). Append the `v_balances_daily` and `v_net_worth` sentences (N-5, N-6 comments). `v_account_balances` gets "(v_balances_daily for today)". |
| Hand copies (`cli/sql_test.go:213`, `run_shared_documents_test.go:354`), `schema.md` | Re-pin and regenerate |
| cashflow Long, after "…left out here too." | `In brokerage and retirement accounts, dividends, interest and capital-gain distributions count as income; buying, selling and moving shares count as neither.` (N-3 (b): "reinvested dividends included" dropped) |
| spend Long | Add: `Margin interest and other investment expenses Quicken puts in an expense category count as spending.` |
| findings Long, after the list (`findings.go:39`) | `duplicate and unlinked-transfer compare register entries only, not buys, sells, dividends or other investment transactions.` |
| holdings Long, last paragraph | `The total is the value of the securities only, without the cash held in investment accounts; quarry accounts shows each account's balance, cash included.` |
| `store.go:38,50-52` docs; `json_accounts.go:17` | Drop "cannot compute" and "not valued" |
| SKILL.md description | Add `net worth today or over time;`; the exclusion becomes `…for gains or ACB beyond…` ("net worth" and "dividend totals" removed) |
| SKILL.md §4 | Add row `\| Net worth today, on a day, or by month \| quarry networth [--as-of <d> \| --since <d>] --json \|` |
| SKILL.md:73 | Delete |
| SKILL.md:74 | `- **Realized gains, ACB:** "quarry counts dividends, interest and capital-gain distributions as income (query v_cash_flow by category for their totals), but does not compute gains or ACB yet."` |
| SKILL.md §9, `mcp --help` Tools line | Add `net_worth` after `holdings` |
| PRD L169 | `quarry networth` row: `Net worth on one day (--as-of, default today) or at each month end (--since/--until), by account type and currency` |
| PRD L125 | Drop "(from Phase 4c)" |
| Pins | Delete `phase4ViewPattern` and its cases (`run_skill_references_test.go:21,57,100-101`), `run_skill_schema_reference_test.go:182`, `run_skill_drift_names_test.go:222-224` |

What dies: `notValuedBalance`, the accounts "not valued" paragraph, the investment CASE in `v_account_balances`, `phase4ViewPattern`, SKILL.md:73. Nothing in `--json` is removed.

### Edge rows

| Input | Outcome |
| --- | --- |
| As-of before the first transaction, or no transactions | Caption and header, no Total, empty warning, exit 0 |
| History window entirely before the data | Same, history empty line |
| A type×currency row that is 0.00 | Omitted from text, kept in JSON |
| Closed account, non-zero balance | Counted, no warning |
| Not-in-reports or linked-tracking account | Not in networth; listed in `accounts` with its balance |
| Credit card, liability, home equity | Stored sign; negative reduces the total |
| Investment account, no holdings | balance = cash |
| Investment cash goes negative | Shown as recorded |
| Unpriced holding (one day, or some month ends) | Left out of the balance, warning |
| Security currency NULL or EUR | Left out, warning (EUR in every mode) |
| USD security in a CAD account | Converted to CAD at the date's rate, silently |
| Rate gap, or past the last stored rate | Latest prior rate, silently |
| Before the first rate, or no rates | Warning 6, `no rate` cell, separate `Total USD` row (snapshot) |
| `--until` in the future | Clamped to today |
| `--since` mid-month | First row is the end of that month |
| Since and until both in this month | One row, today |
| Native mode | Rows per currency, never summed across currencies |
| Excluded-from-reports transaction | In the balance, not in cash flow |
| Future-dated transaction | Not in a balance before its date |
| Old store (format 7) | Existing re-sync refusal, exit 1 |
| Month end with no balance row (history) | Listed; cells blank (native: one line, date only); exit 0 |
| History type × date summing to zero | `0.00` cell |

---

## Scenarios (Gherkin)

```gherkin
Feature: Net worth

  Scenario: SCENARIO-01a Investment cash joins transactions
    Given a Quicken file with an investment transaction that moves cash and has one entry
    When quarry sync runs
    Then transactions has a row naming it in investment_transaction_id with one split per entry
    And the account's cash is the sum of its transactions
    And share-only actions (add, remove, split) get no row
    And the store format is 8

  Scenario: SCENARIO-01b Investment cash rows pair transfers and keep entry-less transactions
    Given an investment transaction whose entry names a transfer account, and one with no entry
    When quarry sync runs
    Then the first pairs with its other side like any transfer and leaves cash flow
    And the second has one uncategorized split and raises the uncategorized finding

  Scenario: SCENARIO-02 Reinvested dividend moves no cash
    Given a reinvested dividend whose entry amount is zero, as Quicken records it
    When quarry sync runs
    Then it has no row in transactions and adds nothing to income

  Scenario: SCENARIO-03 Investment income counts in cash flow
    Given dividend, interest, capital-gain, buy and sell rows in a brokerage account
    When quarry cashflow runs
    Then dividends, interest and capital-gain distributions count as income
    And buys and sells count as neither

  Scenario: SCENARIO-04 Margin interest counts as spending
    Given a margin-interest row in an expense category
    When quarry spend runs
    Then it counts as spending

  Scenario: SCENARIO-05 Findings ignore investment cash rows
    Given two same-amount dividends and a sell with no transfer partner
    When quarry findings runs
    Then no duplicate or unlinked-transfer finding names them

  Scenario: SCENARIO-06 Daily balances combine cash and holdings
    Given a brokerage account with cash and priced, unpriced and other-currency holdings
    When v_balances_daily is queried for a date
    Then balance is cash plus the valued holdings in the account's currency
    And holdings_unvalued counts the holdings left out

  Scenario: SCENARIO-07 Accounts shows investment balances
    Given a brokerage account with cash and holdings
    When quarry accounts runs
    Then its Balance is cash plus holdings value today
    And --json carries cash and holdings_value

  Scenario: SCENARIO-08 Balances line says what is not checked
    Given a synced store with investment accounts
    When quarry status prints the Balances line
    Then it reads "N investment accounts' cash not checked"

  Scenario: SCENARIO-09 Net-worth view covers reported accounts
    Given counted, closed, not-in-reports and linked-tracking accounts
    When v_net_worth is queried for a date
    Then rows are by type and currency over the accounts Quicken's reports count, closed ones included

  Scenario: SCENARIO-10 Net worth today
    Given a synced store
    When quarry networth runs with no flags
    Then it prints today's balances by type and currency with a total in the reporting currency

  Scenario: SCENARIO-11 Net worth on a past day
    Given a synced store
    When quarry networth runs with --as-of 2025-12
    Then it values every counted account on 2025-12-31

  Scenario: SCENARIO-12 Net worth month by month
    Given a synced store
    When quarry networth runs with --since and an --until after today
    Then it lists one row per month end with a column per type, ending with today

  Scenario: SCENARIO-13 Net worth in native currencies
    Given CAD and USD accounts
    When quarry networth runs with --currency native
    Then CAD and USD are listed separately with one total each

  Scenario: SCENARIO-14a Net worth warns about unpriced and unconvertible holdings
    Given an unpriced holding and securities with no currency or another currency
    When quarry networth runs
    Then each left-out value gets its ruled warning, quarry accounts gives the same lines, and the exit code is 0

  Scenario: SCENARIO-14b Net worth warns about USD balances it cannot convert
    Given a USD balance before the first exchange rate, or a store with no rates
    When quarry networth runs
    Then the rate warning is printed, the row shows no rate, a separate Total USD row follows, and the exit code is 0

  Scenario: SCENARIO-15 Net worth refuses impossible dates
    Given a synced store
    When quarry networth runs with a future --as-of, a future --since, or --as-of with --since
    Then it prints the ruled refusal and exits 2

  Scenario: SCENARIO-16 Net worth before any data
    Given a store whose first balance is after the as-of date
    When quarry networth runs
    Then it prints the caption, no total, and the "first balance" warning, and exits 0

  Scenario: SCENARIO-17 MCP net_worth
    Given the MCP server over a synced store
    When a client calls net_worth
    Then it gets the same document as quarry networth --json, capped at 500 month ends

  Scenario: SCENARIO-18 Docs and pins follow the new surface
    Given the conventions text, help texts, SKILL.md, PRD and schema.md
    When the doc drift tests run
    Then they carry the ruled copy and the Phase-4 view pin is gone

  Scenario: SCENARIO-19 Reference check on the real Quicken file
    Given copies of the real snapshot and store in a scratch HOME
    When quarry sync and the read commands run
    Then v_balances_daily cash on each FI statement date matches ZAVAILCASH or the difference is explained
    And every action's entry category kind is as N-2 expects
    And the networth total equals the v_net_worth sum and the accounts sum
    And the cashflow and spend rises equal the new investment rows
    And the user confirms the 9 investment accounts' cash against Quicken
```

---

## Sizing

Architect sizing pass, 2026-10-04. The S01 and S14 splits were approved by the user on 2026-10-04.

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01a | OWNS A RUN (opus), 4 batches, importer. v9fixture entries; `investmentsQuery` reads entries, reconcile status, excluded and memo; the row/split builder for single-entry, non-transfer transactions; share-only actions get no row; `investment_transaction_id`; FormatVersion 7→8 with re-pins and schema.md. **Runs the N-3 probe first** (reinvest entry amounts, system categories on buy entries, entries with a transfer target, transactions with no entry). |
| SCENARIO-01b | OWNS A RUN (sonnet), 4 batches, importer + report; absorbs 02 and 05. Transfer-target entries go through pairing and the splits-sum check; a no-entry transaction gets a NULL-category split, which raises `uncategorized`. Includes the N-4 predicates and the findings Long. |
| SCENARIO-02 | FOLD into 01a (N-3 (b), user decision 10): the reinvest is a zero-amount action, covered by 01a's no-row arm; its acceptance assertion rides in 01a's test. Gherkin reworded to the (b) outcome. |
| SCENARIO-03 | LIGHT; absorbs 04. Coverage of 01's rows; cashflow Long (depends on N-3), spend Long, SKILL:74. Orchestrator ruling 2026-10-04: the SKILL description edit (dropping "dividend totals") moves to SCENARIO-10, which owns the rest of that line, so no unruled intermediate string ships and 03 stays at 3 ruled lines; the spend sentence goes after the paragraph ending "Closed accounts are included." |
| SCENARIO-04 | FOLD into 03. |
| SCENARIO-05 | FOLD into 01b, so the findings goldens are re-pinned once. |
| SCENARIO-06 | OWNS A RUN (opus), 4 batches, report. `v_balances_daily`: day grain, cash arms, holdings_value arms and `holdings_unvalued`, converted columns, COMMENT, conventions sentence, schema.md, and the `v_balances_daily` arm of `run_skill_schema_reference_test.go:182`. **Runs the N-6 precondition.** |
| SCENARIO-07 | OWNS A RUN (sonnet); absorbs 08; 4 batches, report. `v_account_balances` on `v_balances_daily`; the accounts cell and JSON; `notValuedBalance` deleted; accounts and holdings Long; store.go and json_accounts.go docs; the Balances line with re-pins. |
| SCENARIO-08 | FOLD into 07. |
| SCENARIO-09 | OWNS A RUN (sonnet); absorbs 18; 3 batches, report. `v_net_worth` arms, COMMENT and conventions; every remaining Phase-4 view pin retired (the drift `unknown_view` example goes red once the view exists). Re-confirms N-6. |
| SCENARIO-18 | FOLD into 09 (acceptance test = the drift test where the last view pin dies). Copy lines go to their owners: 01a conventions investment sentence, hand copies, PRD L125; 01b findings Long; 03 cashflow and spend Long, SKILL:74; 06 `v_balances_daily` sentence; 07 accounts and holdings Long, store/json docs; 09 `v_net_worth` sentence and pins; 10 SKILL description, §4, :73, PRD L169; 17 SKILL §9 and the `--help` Tools line. |
| SCENARIO-10 | OWNS A RUN (opus), 4 batches, report. Port plus `Server.NetWorth` snapshot; command copy; text renderer; `document.NetWorth` `--json` for both modes; root registration and the all-commands tables; SKILL description, :73, PRD L169. Absorbs SCENARIO-13 (snapshot native) — orchestrator ruling 2026-10-04: `reporting.currency = native` reaches native mode with no flag, so S10 must render it. SKILL §4 row moves to SCENARIO-15 (the drift test checks its `--as-of`/`--since` flags exist). Interim until 14b/16: no-rate converted cell blank and left out of the total; empty result prints caption and header only. |
| SCENARIO-12 | OWNS A RUN, 3–4 batches, report. N-9 month-end list (empty, inverted, mid-month since, both dates in this month, until clamp); the history pivot renderer; existing `WindowError` lines. Also history in native mode (`Month end  Currency  <types…>  Total`), reachable via config (S10 ruling). |
| SCENARIO-15 | OWNS A RUN (sonnet); absorbs 11; 3 batches. The as-of flag; `AsOfError` takes a noun (holdings bytes pinned identical); a new `WindowError` kind for a future since; the as-of × since/until conflict. Also SKILL §4 row (moved from S10). |
| SCENARIO-11 | FOLD into 15. |
| SCENARIO-13 | FOLD into SCENARIO-10 (snapshot native; acceptance test `Test_run_networth_lists_cad_and_usd_separately_with_one_total_each_in_native_mode`); history native goes to SCENARIO-12. |
| SCENARIO-14a | OWNS A RUN (sonnet), 3–4 batches. Warnings 3–5 in snapshot and history; the shared composer; both sites (networth and accounts, after the existing `accountsFXWarnings` at `internal/cli/accounts.go:47`). |
| SCENARIO-14b | OWNS A RUN (sonnet), 3 batches. Warning 6 (three variants), the `no rate` cell, the `Total <CUR>` row. |
| SCENARIO-16 | OWNS A RUN (sonnet), 2–3 batches. First-transaction fact in the same port call; three empty-result warnings; no Total row. |
| SCENARIO-17 | OWNS A RUN (sonnet), 2 batches, mcp. 6 ruled lines exceed the light lane's limit of 3; no assertion change. |
| SCENARIO-19 | No architect or developer. Orchestrator reference check; times `v_net_worth WHERE date IN (...)` on the real store; the user's manual cash comparison. |

**Traps:** existing v9fixture investment transactions have no entries, so 01b's NULL-category split raises `uncategorized` and changes Rows counts in `cmd/quarry` goldens. Rule gap: a file whose buys carry more than one system category, or a reinvest entry with no category, has no ruled outcome. If the 01a probe finds either, product-vision rules it before 01b builds.

## BDD Acceptance Progress

- [x] SCENARIO-01a: Investment cash joins transactions — `cmd/quarry/run_investment_cash_test.go` `Test_run_sync_gives_each_investment_transaction_that_moves_cash_a_row_in_transactions`
- [x] SCENARIO-01b: Investment cash rows pair transfers and keep entry-less transactions — `cmd/quarry/run_investment_cash_test.go` `Test_run_sync_pairs_an_investment_transfer_entry_and_gives_an_entry_less_investment_one_uncategorized_split`
- [x] SCENARIO-02: Reinvested dividend moves no cash — delivered by SCENARIO-01a — `cmd/quarry/run_investment_cash_test.go` `Test_run_sync_gives_a_reinvested_dividend_no_row_and_no_income`
- [x] SCENARIO-05: Findings ignore investment cash rows — delivered by SCENARIO-01b — `cmd/quarry/run_findings_investment_cash_test.go` `Test_run_findings_leaves_investment_cash_rows_out_of_duplicate_and_unlinked_transfer`
- [x] SCENARIO-03: Investment income counts in cash flow — `cmd/quarry/run_investment_cashflow_test.go` `Test_run_cashflow_counts_investment_dividends_interest_and_capital_gains_as_income_and_buys_and_sells_as_neither`
- [x] SCENARIO-04: Margin interest counts as spending — delivered by SCENARIO-03 — `cmd/quarry/run_investment_cashflow_test.go` `Test_run_spend_counts_investment_margin_interest_as_spending`
- [x] SCENARIO-06: Daily balances combine cash and holdings — `cmd/quarry/run_balances_daily_view_test.go` `Test_run_sql_combines_cash_and_valued_holdings_from_v_balances_daily`
- [x] SCENARIO-07: Accounts shows investment balances — `cmd/quarry/run_accounts_investment_balance_test.go` `Test_run_accounts_shows_an_investment_balance_as_cash_plus_holdings_value`
- [x] SCENARIO-08: Balances line says what is not checked — delivered by SCENARIO-07, `cmd/quarry/run_status_test.go` `Test_run_status_says_investment_accounts_cash_is_not_checked`
- [x] SCENARIO-09: Net-worth view covers reported accounts — `cmd/quarry/run_net_worth_view_test.go` `Test_run_sql_sums_net_worth_by_type_and_currency_over_the_accounts_quickens_reports_count`
- [x] SCENARIO-18: Docs and pins follow the new surface — delivered by SCENARIO-09, `cmd/quarry/run_skill_schema_reference_test.go` `Test_skill_schema_reference_carries_each_view_comment`
- [x] SCENARIO-10: Net worth today — `cmd/quarry/run_networth_test.go` `Test_run_networth_prints_todays_balances_by_type_and_currency_with_a_total_in_the_reporting_currency`
- [x] SCENARIO-12: Net worth month by month — `cmd/quarry/run_networth_history_test.go` `Test_run_networth_lists_each_month_end_with_a_column_per_type_ending_with_today`
- [x] SCENARIO-15: Net worth refuses impossible dates — `cmd/quarry/run_networth_as_of_test.go` `Test_run_networth_refuses_a_future_as_of_a_future_since_and_as_of_with_since`
- [x] SCENARIO-11: Net worth on a past day — delivered by SCENARIO-15, `cmd/quarry/run_networth_as_of_test.go` `Test_run_networth_values_every_counted_account_on_the_as_of_day`
- [x] SCENARIO-13: Net worth in native currencies — delivered by SCENARIO-10, `cmd/quarry/run_networth_test.go` `Test_run_networth_lists_cad_and_usd_separately_with_one_total_each_in_native_mode`
- [x] SCENARIO-14a: Net worth warns about unpriced and unconvertible holdings — `cmd/quarry/run_networth_holdings_warnings_test.go` `Test_run_networth_warns_about_each_holding_it_leaves_out_and_accounts_gives_the_same_lines`
- [x] SCENARIO-14b: Net worth warns about USD balances it cannot convert — `cmd/quarry/run_networth_rates_test.go` `Test_run_networth_warns_when_a_usd_balance_has_no_exchange_rate_and_totals_it_apart`
- [ ] SCENARIO-16: Net worth before any data
- [ ] SCENARIO-17: MCP net_worth
- [ ] SCENARIO-19: Reference check on the real Quicken file
