# Specification: Phase 2f — exchange rates and reporting currency

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: one answer in one currency. About two-thirds of the household's accounts are USD, so today every report splits CAD and USD totals that cannot be added together. 2f converts amounts at Bank of Canada daily USD/CAD rates for each transaction's date. `--currency CAD|USD|native` is available on spend, cashflow, recurring, anomalies and accounts. The default comes from `reporting.currency` in config, and is CAD out of the box.

**Secondary goals**:
- `sync` keeps the rates current, and a failed fetch never fails a sync.
- `status` shows rate coverage.
- Conversion is exact and the same everywhere: the CLI and `quarry sql` read the same view columns.

**Out of Scope**: ACB / `quarry acb`, `networth`, investment holdings (all Phase 4); MCP `currency` parameter (Phase 3); currencies other than CAD and USD; a balance total in `accounts`; converting `findings` amounts; a configurable fetch timeout or an offline flag.

**Business Rules** (user decisions at scoping, 2026-10-01):
- D1: `--currency native` keeps today's per-currency output exactly; the default converts.
- D2: recurring and anomalies detect in each account's own currency and convert only for display.
- D3: an amount with no rate (no rates stored, or dated before the first stored rate) gives a warning, exit 0, and is shown in its own currency on rows of its own. A total never mixes currencies.
- D4: `fx_rates` lives in quarry.duckdb, carried across rebuilds like findings and import_runs. The store format goes 4→5, so users re-sync once.

## Business Rules & Invariants
- R1 — Conversion lives in the store views (`v_cash_flow`, `v_spending`, `v_account_balances`).
  - Go never repeats the ASOF lookup. The one exception is anomalies' Usual, through a single `money.Convert`, with a parity test against the view column.
  - Same-currency conversion is the identity and needs no rate.
  - A converted column is NULL only for a cross-currency amount dated before the first stored rate (or when there are no rates at all).
- R2 — Exact money.
  - Each split converts and rounds half away from zero to the cent before it is summed. No DOUBLE anywhere in the path.
  - spend Total = cashflow Spent per currency, in every mode.
  - The converted rows of a currency sum to its Total (except `--by tag`, as today).
- R3 — Detection stays native.
  - Recurring series are keyed by (payee key, native currency), and the 5% price-change rule is judged on native amounts.
  - Anomaly baselines, the 100.00 floor and Times are native.
  - FX movement never creates, breaks or flags anything.
- R4 — Rates are never shown in reports, only via `quarry sql`.
  - `fx_rates.usd_cad` is CAD per 1 USD.
  - The views also carry the `usd_cad` they applied.
- R5 — The fetch runs only after build, validation and findings pass, and before the swap.
  - A sync that fails earlier makes no network request.
  - A cancel during the fetch is the existing interrupted outcome (exit 1, previous store untouched).
  - A fetch failure still swaps the new store in, with exit 0.
- R6 — Sync fetches the part of [earliest transaction date, today] not covered by [first, last] stored, at both ends.
  - *Amended at gate round 1:* fx_rates is always one interval. The forward span starts the day after the last stored rate, even when the earliest transaction is later, so no gap ever opens after `last`. Likewise the backward span ends the day before the first stored rate, even when the earliest transaction's whole range ends sooner, so no gap opens before `first` either (gate round 2). A Need whose first date is after its last (every transaction future-dated) is empty: no request, floor unchanged. After a rates fault, the floor carried by earlier runs is forgotten in the store too, not only in memory.
  - FXUSDCAD wins where it has a date; IEXE0101 is used only before it. `fx_rates.series` records which.
  - Carried rates are never deleted.
  - A permanent gap before the legacy series' first date is neither a failure nor refetched every sync. The SCENARIO-01 plan confirms IEXE0101's real start date with one fetch.
- R7 — The fetch is quarry's only outbound network call. The request carries only the series and date range, and each request has a fixed 30 s timeout.
- R8 — Config is read by the five `--currency` commands only when `--currency` is absent. `sql` keeps ignoring config.

---

## Triage Brief

None of 2f exists yet. There is no `--currency`, no `fx_rates`, no config key and no network code (zero `net/http` imports).

## PRD promises (docs/initial-prd.md)
- :176 says every reporting command takes `--currency CAD|USD`, default from config, CAD out of the box. The command list is not enumerated. The CLI table at :160-172 has accounts, spend, cashflow, recurring, anomalies, findings and sql. networth and acb are Phase 4 and not registered.
- :162 says sync refreshes Bank of Canada rates. :164 says status shows FX rate coverage.
- :152 sets the fetch rules:
  - Fetch only the dates missing since the last stored rate.
  - The first sync back-fills from the earliest transaction date.
  - Dates before the current daily series use the legacy noon-rate series.
  - The request carries only the series and the date range.
  - The fetch runs on `sync --from` too.
- :153 covers a failed fetch:
  - It does not fail sync, and the new store is still swapped in.
  - A one-line warning goes to stderr and to `warnings` in `--json`.
  - FX coverage is recorded in `import_runs`.
  - Dates past the last stored rate use the latest prior rate.
  - status shows how far coverage lags.
- :242 sets the rate date:
  - Each amount converts at the BoC rate for its transaction date.
  - Weekends and holidays use the latest prior business day (ASOF join).
  - Balances convert at the valuation-date rate.
- :241 says amounts are stored native and conversion happens in views only.
- :243 says cross-currency transfers keep both legs and are never a gain, loss or expense. cashflow and spend already exclude transfers.
- :124 defines the `fx_rates` table as CAD/USD rate by date. :265 says the rate fetch is v1's only outbound network call.
- :330-331 records the decisions: CAD default, and a failed fetch warns.
- The series ID and Valet API are not named in the PRD. FXUSDCAD on Valet (https://www.bankofcanada.ca/valet/observations/FXUSDCAD/json?start_date=…&end_date=…) is external knowledge. The legacy noon series is IEXE0101 (USD/CAD noon, ended 2017-04-28).
- ACB, networth and investments stay in Phase 4.
- 2a spec :81-90 verdict 7: read-only connections cannot create views, so analysis rules are DuckDB views built with the store, so `quarry sql` can see them.

## Command surface (internal/cli)
| Command | Flags | Port | Currency today |
|---|---|---|---|
| spend spend.go:18 | `--by`, plus reportFlags `--since/--until/--account` (window.go:34-36) | srv.Spend | Rows and totals grouped by currency, one Total per currency, CAD before USD |
| cashflow cashflow.go:41 | `--by` plus reportFlags | srv.CashFlow | Grouped by (period, currency) |
| recurring recurring.go:22 | reportFlags | srv.Recurring → Store.Charges | Group key (payee key, currency). Totals per currency |
| anomalies anomalies.go:22 | reportFlags | srv.Anomalies → Store.Charges | Baselines per (payee, currency). AnomalyMinAmount floor 100.00 (report/anomalies.go:19) |
| accounts accounts.go:6 | `--all` | srv.Accounts | Native balance per account, Currency column, no total |
| findings findings.go | `--status/--type/--csv` | srv.Findings | Item amount in its account's currency |
| sql sql.go:36 | `--csv/--limit` | srv.Query | Raw. Help says "each account's own currency" |
| status status.go:12 | none | srv.Status | Counts only |
| sync sync.go | `--quicken/--from` | snapshot.Server | n/a |

- Only spend, cashflow, recurring and anomalies share reportFlags.
- Help Long for spend, cashflow and recurring says "CAD and USD are listed separately, never added together". A `--currency` default makes that false. It is pinned byte-exact in internal/cli/report_test.go.
- Money is int64 cents. `formatMoney` (render.go:250) is used for text and `jsonMoney` (json.go:297, 2-decimal string) for JSON. No money-rounding helper exists.

## Store
- schema.go `schemaDDL`. The store is rebuilt on every sync via Replace (duckstore.go:280-334): build a new partial file, then rename.
- `readHistory` (history.go:55-84) carries import_runs and findings from the previous store across the rebuild. That is the precedent for carrying fx_rates.
- FormatVersion = 4 (duckstore.go:25). Any other version is refused as other-format, which means re-sync.
- accounts.currency is CAD/USD only. transactions.currency always equals the account currency. transfers.cross_currency exists.

## Config
- config.toml. Keys are snapshots.keep, quicken.path and findings.ignore.
- Unknown keys get a warning. A bad value is a refusal with per-key copy.
- Only sync, status, findings and snapshots load config. Read commands (spend, cashflow, recurring, anomalies, accounts, sql) do not, and they ignore a malformed config (pinned at cmd/quarry/run_config_test.go:209).

## Sync / status
- Sync warnings print as `quarry: warning: <text>` on stderr (sync.go:137-139). JSON carries them in `warnings`.
- exitCode: nil → 0, UsageError → 2, otherwise 1.
- Status text is `%-10s%s` lines: Store, Snapshot, Source, Dates, Rows, Balances, Splits, Transfers, Findings (render_status.go:25-37).
- Status JSON (json_status.go:13-23) has store, snapshot, dates, balances, splits, transfers, findings, not_imported and warnings.

## All-commands test tables (every reporting command gains a flag)
- cmd/quarry/run_read_refusals_test.go:46-60 and :~174
- run_usage_test.go:~200-225
- run_read_usage_test.go:30-60
- internal/cli/report_test.go
- json_*_internal_test.go key-set pins
- run_cashflow_invariant_test.go:76 (spend Total = cashflow Spent per currency)

## Real data
The latest snapshot has 8 CAD and 17 USD accounts, with 6,791 CAD and 8,875 USD raw transactions. USD is the majority.

## Product Verdict


The rows stay as they are: each row is in one currency, the Currency column stays, and there is one Total per currency. A converted report usually has one currency. Amounts with no rate stay on native rows, so a mixed total cannot be built. `native` mode reproduces today's output byte for byte.

## Rules
- **R1, one owner for conversion.** Conversion lives in the store views.
  - `v_cash_flow` gains `amount_cad`, `amount_usd` and `usd_cad` (the rate it applied).
  - `v_spending` gains `spent_cad`, `spent_usd` and `usd_cad`.
  - `v_account_balances` gains `balance_cad` and `balance_usd` at the current_date rate.
  - A converted column is NULL only for a cross-currency conversion with no rate on or before its date.
  - Same-currency conversion is the identity and needs no rate, so an all-CAD file in CAD never warns, even with an empty fx_rates.
  - Store.Charges reads v_spending, so recurring and anomalies inherit the conversion. Go never repeats the ASOF lookup.
  - Anomalies' Usual is the one conversion done in Go: the median times the charge's usd_cad, through a single core convert function. A parity test asserts that function equals the view column for every v_spending row in a fixture with half-cent cases.
- **R2, exact money.** Convert each split, round half away from zero to the cent, then sum. No DOUBLE anywhere in the path.
  - DuckDB DECIMAL/DECIMAL division returns DOUBLE, so the USD path must use exact integer arithmetic.
  - The test asserts `typeof(amount_usd) = 'DECIMAL(18,2)'` and pins ±0.125 cases in both directions.
  - spend Total = cashflow Spent per currency in every mode. run_cashflow_invariant_test.go:76 gains a USD-data/CAD-report case and a before-first-rate case.
  - The converted rows of each currency sum exactly to that currency's Total (`--by tag` is the exception, as today).
- **R3, detection stays native.**
  - Recurring series are keyed by (payee key, native currency).
  - The 5% price change is judged on native amounts.
  - Anomaly baselines are per (payee or category, native currency).
  - The 100.00 floor and Times are judged on native amounts, so a USD 90.00 charge is never listed.
  - FX movement never creates a series, breaks one, or makes an anomaly.
- **R4.** Rates are never shown in reports. They are visible only through sql, as `fx_rates.usd_cad` (CAD per 1 USD) and the views' `usd_cad` column.
- **R5, sync ordering.** The fetch runs only after build, validation and findings pass, and before the swap.
  - Quicken closed, reconciliation failed, fingerprint changed or a build error means no fetch, no Rates line, and existing copy and exit codes.
  - Ctrl-C during the fetch is the existing interrupted-sync outcome: exit 1, previous store untouched. It is not treated as a fetch failure.
- **R6, what sync fetches.** It fetches whatever part of [earliest transaction date, today] is not in fx_rates, at both ends, so a failed legacy back-fill retries.
  - FXUSDCAD wins wherever it has a date; IEXE0101 is used only for earlier dates. The `series` column records which.
  - The architect confirms IEXE0101's real start date with one fetch before planning.
  - Carried rates are never deleted. A `sync --from` of an older snapshot keeps them.
- **R7, store format 4→5.**
  - fx_rates and the new import_runs columns arrive with v5.
  - The existing other-version refusal (report/refusal.go:53) is unchanged.
  - A v4 store carries findings and import_runs (readHistory is column-tolerant); its absent fx_rates means nothing is carried and no warning is given.
  - An unreadable fx_rates gives this sync warning, then a full fetch: `cannot carry exchange rates forward from the previous store (<reason>); fetching them all again`
    *(Mid-feature ruling, SCENARIO-04.)* The `<reason>` phrases are:
    - `its fx_rates table repeats a date`
    - `its fx_rates table is incomplete` (a NULL cell or missing column; also the fallback for any other read failure)
    - `its fx_rates table holds an impossible rate` (a rate ≤ 0 or out of range)
    - `its fx_rates table names an unknown series` (a series that is not FXUSDCAD or IEXE0101)
  - A wholly unreadable previous store gives no separate rates line; the combined line covers it (see Changes to existing surfaces).
  - A v4 store is silent.
  - Order on stderr and in `warnings[]`: manifest, history, findings, rates, prune. Exit 0.
  - `status` says nothing about a carry fault.

## Which commands
| Command | Ruling |
|---|---|
| spend, cashflow, recurring, anomalies | `--currency`; convert by default |
| accounts | `--currency`; an `In CAD`/`In USD` column; NO Total row (DON'T BUILD — wrong net worth before Phase 4) |
| findings | no flag; native (evidence for a fix in Quicken) |
| sql | no flag; gets fx_rates and the R1 view columns |
| status, sync | no flag; a `Rates` line |

## Config
- **Key:** `reporting.currency`. Values are CAD, USD and native, case-insensitive on input; output always shows the canonical form. Unset means CAD.
- **Bad value** (exit 1, stderr; reuses badValue; `got` is the value as written, so `got ""`, `got 12`, `got true`):
  `quarry: ~/Library/Application Support/quarry/config.toml: reporting.currency must be CAD, USD or native, got "EUR"; fix the file and run the command again`
- **When it is read.** The five `--currency` commands read config ONLY when `--currency` is absent.
  - When read, a malformed file gets the existing `cannot read <path>: <detail>; fix the file and run the command again` refusal, exit 1.
  - Unknown-key warnings go to stderr in `~` form and to `warnings[]` in absolute form, as findings does (findings.go:77-96).
- **sql** keeps ignoring config. run_config_test.go:209 is split: the five commands refuse, sql still ignores.
- **Mid-feature ruling (SCENARIO-16):**
  - **One parser.** The flag and the key share one parser: case-insensitive on input, no trimming. `usd`, `Usd` and `NATIVE` are accepted; `" CAD"` is refused. Output is always canonical `CAD`/`USD`/`native`, in captions and JSON. The bad-flag line never echoes the input.
  - **Table-shape refusal.** Exit 1, `got` as written:
    `quarry: ~/Library/Application Support/quarry/config.toml: reporting must be a table, such as reporting.currency = "CAD", got "CAD"; fix the file and run the command again`
  - **status never refuses.** A bad reporting.currency gets the existing P2d-10 status warning (exit 0, `"ignored": null`). Use the existing prefix:
    `cannot tell which findings you ignored: ~/Library/Application Support/quarry/config.toml: reporting.currency must be CAD, USD or native, got "EUR"; findings you ignored are counted as open`
  - **These refuse with the Bad value line** (exit 1, stdout empty, absolute path in `--json`), because the whole file is validated: sync (including `--from`), snapshots, snapshots prune and findings.
  - **Error order.** A usage error (exit 2) comes before a config refusal, which comes before the no-store refusal.
  - **P2d-10 amended.** spend, cashflow, recurring, anomalies and accounts load and validate the whole file when `--currency` is absent. Known keys gain `reporting.currency`.


## Surface & Copy

### Literal surface
- **Flag on spend, cashflow, recurring and anomalies** (the backticked word is the placeholder):
  `show amounts in currency `code`: CAD, USD, or native for each account's own (default reporting.currency in the config file, else CAD)`
- **Flag on accounts:**
  `add a column with each balance in currency `code`: CAD or USD; native adds none (default reporting.currency in the config file, else CAD)`
- **Bad flag value** (exit 2, checked with the other flag usage errors, before config or store):
  `quarry: --currency must be CAD, USD or native`
  A bare `--currency` gets cobra's `flag needs an argument: --currency`, exit 2.
- **Caption.** In a converted report, windowCaption gains `, amounts in CAD` (or USD). The same applies to the Cash flow, Recurring charges and Unusually large charges captions. Native mode leaves the caption unchanged.
  e.g. `Spending 2026-01-01 to 2026-10-01 in all accounts, amounts in CAD`
- **Columns** are unchanged for spend, cashflow, recurring and anomalies. `Currency` is the currency of that row's amounts: the reporting currency, except on unconverted rows. Totals are one per currency, CAD first.
- **recurring:**
  - When the series' native currency differs from the row's, the Currency cell is `CAD (USD)`.
  - Price changes stay native, with the code prefixed when native differs: `1: USD 9.99 -> USD 12.99 (+30.0%)`. A same-currency series keeps `1: 9.99 -> 12.99 (+30.0%)`.
  - Amount converts at the latest charge's date. Per year = converted Amount × charges per year, so the Per year cells sum to Total.
- **anomalies:**
  - Amount and Usual are converted at the charge-date rate. Times and the footer are native.
  - An unconverted charge shows `USD 250.00` in its Amount and Usual cells.
- **accounts:**
  - Header: `Account  Type  Currency  Balance  In CAD  Status`.
  - The cell is blank for a not-imported account and reads `no rate` when no rate is available.
  - Native mode has no column, which is exactly today's output.

### --json (additive; no field disappears)
- **Every report** gets a top-level `"currency": "CAD"|"USD"|"native"`, right after `until` (after `by` where present). A report is complete when every row's and total's `currency` == `.currency`; there is no count field.
- **spend and cashflow** rows and totals keep `currency`, and their money is in it.
- **recurring:**
  - series `currency`, `amount`, `first_amount` (converted at the first charge's date) and `per_year` are in `currency`.
  - New series fields: `native_currency`, `native_amount`, `native_first_amount`.
  - `price_changes[]` gains `currency` (= native_currency), with from and to in it.
  - `totals[]` is unchanged.
- **anomalies:** `currency`, `amount` and `usual` are in `currency`. New: `native_currency`, `native_amount`, `native_usual`. `times` is native.
- **accounts:**
  - Top-level `currency`.
  - Rows keep `currency` and `balance` native.
  - New `converted_balance` is in the top-level currency. It is null in native mode, for a not-imported account, or when there is no rate.
- In native mode `currency == native_currency` and `amount == native_amount`, so the key set is the same in both modes.

#### Recurring (mid-feature ruling, SCENARIO-17)
- **JSON key order.**
  - Series: `payee, payee_key, payees, currency, cadence, amount, first_amount, per_year, native_currency, native_amount, native_first_amount, first_charge, last_charge, charge_count, state, new, accounts, price_changes`.
  - Price change: `date, currency, from, to, change_pct`.
  - Top level: `since, until, currency, account_filter, series, totals, warnings`.
  - Native mode has the same key set.
- **Currency cell and prefix.** The Currency cell is `<row> (<native>)`, and the price-change prefix is `<native> `, only when native differs from the row. In USD mode a CAD series reads `USD (CAD)` and `1: CAD 9.99 -> CAD 12.99 (+30.0%)`. An unconverted row is plain, with no prefix.
- **Sort.** A last tier follows the group key: the row-currency series comes before a converted one. The tiers in order are currency, state, standing, lower(payee), group key, then this new tier.
- **Recurring Long, wrapped at ≤74 columns.**
  - Paragraph inserted after paragraph 1:
    ```
    Series are found in each account's own currency, so a change in the
    exchange rate is never a price change, and a payee that charges in both
    CAD and USD has two series. Amount and Per year are converted to the
    reporting currency (--currency, else reporting.currency in the config
    file, else CAD) at the rate on the latest charge's date; price changes
    stay in the series' own currency. With --currency native nothing is
    converted.
    ```
  - Paragraph 3's last two lines become:
    ```
    the next, in the series' own currency. Per year is the latest amount
    times the charges in a year, for active series only.
    ```

### Anomalies (mid-feature ruling, SCENARIO-18)
- **Long** (anomalies.go:27), wrapped greedily at ≤74 columns. Paragraph 1 becomes:
  ```
  List charges that are unusually large: more than 2 times the median of the
  payee's earlier charges, when there are at least 3, or else more than 5
  times the median of the category's earlier charges, when there are at
  least 10. Charges under 100.00 in their account's own currency are never
  listed. Charges follow the rules of quarry spend, and a transaction counts
  once, with all its splits; an uncategorized or split charge from a payee
  with little history cannot be judged. Possible duplicates are listed by
  quarry findings, not here. Charges dated after today are left out, even
  with a later --until.
  ```
  A new paragraph goes after paragraph 1 and before the `--since`/`--until` paragraph, which stays unchanged. No line starts with `--currency`:
  ```
  Charges are judged in their account's own currency, so a change in the
  exchange rate never makes a charge unusual. Amount and Usual are then
  shown in the reporting currency (--currency, else reporting.currency in
  the config file, else CAD) at the rate on the charge's date.
  With --currency native nothing is converted.
  ```
- **JSON key order.**
  - Entry: `transaction_id, date, account_id, account, currency, payee, category, amount, baseline, usual, native_currency, native_amount, native_usual, earlier, times`.
  - Top level: `since, until, currency, account_filter, anomalies, checked, not_judged, warnings`.
  - Native mode has the same keys, with the native_* fields equal to their twins.
- **Prefix.** When an Amount or Usual cell is left native and native differs from the target, the cell reads `<native> 250.00` (`USD 250.00` in CAD mode, `CAD 250.00` in USD mode). Both cells always carry the prefix together. Converted cells are plain. Native mode never adds a prefix. The Account label keeps `(USD)`. Times and the footer stay native.
- **FX warning noun:** `charge`/`charges`, with `is`/`are`.

### Accounts (mid-feature ruling, SCENARIO-19)
- **JSON key order.** Top level: `as_of, currency, accounts, warnings`. Row: `id, name, type, currency, institution, closed, active, in_reports, linked_tracking, balance, converted_balance`. Native mode has the same keys, with `converted_balance: null` on every row.
- **Alignment.** The `In CAD` / `In USD` cell is right-aligned to the column width, including `no rate` and the blank cell. Status follows after the usual two-space gap. Trailing spaces are trimmed on every line.
- **Header.** USD mode: `Account  Type  Currency  Balance  In USD  Status`. With every account closed, the header-only output keeps the column, and only the all-closed note is printed (no FX line).
- **Warnings.** These print on stderr with `quarry: warning: `, the same text goes into `warnings[]`, exit 0, slot 3 of the ordering. Each fires only when at least one listed row shows `no rate`.
  - **No rates.** CAD mode: `the store has no exchange rates, so USD balances show no rate in the In CAD column; run quarry sync to fetch them`. USD mode swaps the currencies.
  - **All rates dated after today.** CAD mode: `the first exchange rate in the store, <first>, is dated after today, so USD balances show no rate in the In CAD column; check the Mac's date and time`. USD mode swaps the currencies.
  - **Silent cases:** all-CAD in CAD mode, a not-imported cross-currency account (blank cell), and header-only output.
- **Long append.** A new paragraph after the "not imported" paragraph, wrapped at 72 columns:
  ```
  A column shows each balance in the reporting currency (--currency, else
  reporting.currency in the config file, else CAD) at today's Bank of
  Canada rate, or the latest earlier one; --currency native leaves it
  out. quarry does not add balances together: a total that leaves out
  investment accounts would not be your net worth.
  ```

## Report warnings (stderr with `quarry: warning: `, the same text in warnings[], exit 0)
- **No rates and a conversion is needed:**
  `the store has no exchange rates, so amounts are listed in each account's own currency; run quarry sync to fetch them`
- **Before the first rate** (N via humanize.Count; `is` for 1, `are` for more):
  - spend and cashflow: `3 transactions dated before 1990-01-02, the first exchange rate in the store, are listed in USD, not converted to CAD`
  - anomalies: the same sentence with `charges`.
  - recurring: `2 series with a charge dated before 1990-01-02, the first exchange rate in the store, are listed in USD, not converted to CAD`. A series with its first or latest charge uncovered is shown entirely native.
- **accounts with no rates:** see "### Accounts (mid-feature ruling, SCENARIO-19)". Accounts have their own lines, not the shared "no rates" line.
- **Ordering** *(mid-feature ruling, SCENARIO-12)*: stderr and `warnings[]` share one order:
  1. config warnings
  2. left-out account(s)
  3. FX ("no rates" or "before")
  4. spend `--by tag` multi-tag note
  5. empty-window note

  An empty window gets no FX line.
- **Scope of the FX lines.** Third-currency splits (unreachable: the importer accepts only CAD/USD) get no FX warning, stay native, and are not counted. N counts distinct transactions in the report's own view, so spend and cashflow may differ on one store.
- **Zero fill** *(mid-feature ruling, SCENARIO-12)*:
  - In CAD or USD mode, `fillSeries` fills zero rows for the target currency only.
  - A row in another currency appears only in a period where the store returned one.
  - Within a period, the target row comes first, then the other currencies in `currencyList(Totals)` order.
  - Native mode is unchanged: every currency is filled in every period.
  - The same applies to text and `--json` rows.
  - Totals, grouping and keys are unchanged.
  - *(Final product-vision pass)* An empty window (the store returned no rows; the empty-window note fires) gets no rows in any mode, as before 2f: text prints the header and the note, --json has "periods": [] (cashflow) or "rows": [] (spend), and "totals": [].

### status (%-10s, after Findings)
- **Covered:** `Rates     USD/CAD from the Bank of Canada, 1990-01-02 to 2026-09-30 (1 day ago)` (age: `today`, `1 day ago`, `N days ago`).
- **First rate after the first transaction:** append `; transactions before 1990-01-02 are not converted`
- **Last fetch failed:** append `; the last sync could not fetch new rates: <reason>`
- **None:** `Rates     none, so amounts are not converted; run quarry sync to fetch them from the Bank of Canada`, plus the failure clause if one is recorded.
- **JSON:** `"rates": {"first": "1990-01-02"|null, "last": "2026-09-30"|null, "fetch_error": null|"<reason>"}`
- **Store:** import_runs gains rates_checked_from DATE (the earliest date quarry has asked the Bank of Canada for and got an answer; dates before it have no rate, and quarry does not ask again), rates_last DATE (the latest rate in fx_rates after this run), rates_fetch_error VARCHAR (this run's <reason>, NULL when the fetch succeeded or nothing was asked). *(Mid-feature ruling, SCENARIO-01: renamed from rates_first.)* rates_checked_from is cumulative: min(previous run's value, Need.First when this run asked that span with no FetchError), otherwise the previous value carried; NULL only if no fetch ever succeeded. Status, the sync Rates line and Replaced.Rates take first/last from min/max(fx_rates.date).

### sync Rates line (after Findings)
- `Rates     USD/CAD 1990-01-02 to 2026-09-30 (12 new)`
- `Rates     USD/CAD 1990-01-02 to 2026-09-30 (up to date)`
- `Rates     USD/CAD 1990-01-02 to 2026-09-24 (not refreshed; see warning)`
- `Rates     USD/CAD <first> to <last> (<n> new, not all fetched; see warning)`: a partial fetch (FetchError set, n ≥ 1 kept). `--json` gives `added: n` and `fetch_error` set. *(Mid-feature ruling, SCENARIO-03.)*
- `Rates     none (not fetched; see warning)`
- `Rates     none (no transactions to convert)`: no rates and no transactions.
- `Rates     none (the Bank of Canada has no rates for your transaction dates)`: no rates, transactions exist, the source answered empty.
- The Rates line is never omitted when the store was built. Its absence is reserved for a sync that failed before the swap (SCENARIO-05). An empty answer is not a FetchError and gives no warning.
- **JSON:** store.rates {"first","last","added": int,"fetch_error": null|"<reason>"}, the last key of store (after not_imported); null when the store was not built. *(Mid-feature ruling, SCENARIO-01.)*

### sync fetch warnings (exit 0; the store is still swapped in)
*(Mid-feature ruling, SCENARIO-03.)*
- **Order** on stderr and in `warnings[]`: manifest, history, findings, rates-carry, rates-fetch, prune. A sync prints at most one fetch warning.
- **Carry fault followed by a failed full fetch**: the carry line, then the nothing-stored fetch line, then `Rates     none (not fetched; see warning)`.
- **Stop rule**: after a timeout or a cannot-reach failure, later spans are not asked, so the worst case is 30 s. After a 503 or a not-a-list answer, later spans are still asked. FetchError is the first failure's reason.
- **Body cap**: 16 MiB. An answer over the cap counts as "not a list of exchange rates".
- **Unrecognised error**: falls back to `cannot reach www.bankofcanada.ca`.
- **Nothing new, some rates stored:** `could not fetch exchange rates from the Bank of Canada: <reason>; the store has rates from <first> to <last>, and later dates convert at the <last> rate; run quarry sync again to retry`
- **Nothing stored:** `could not fetch exchange rates from the Bank of Canada: <reason>; the store has no rates, so reports list amounts in each account's own currency; run quarry sync again to retry`
- **Partial range:** `could not fetch every exchange rate from the Bank of Canada: <reason>; the store has rates from <first> to <last>, and later dates convert at the <last> rate; run quarry sync again to fetch the rest`
- **`<reason>`**:
  - `cannot reach www.bankofcanada.ca`
  - `no answer from www.bankofcanada.ca within 30 seconds` (a fixed 30 s timeout per request)
  - `www.bankofcanada.ca answered 503 Service Unavailable`
  - `www.bankofcanada.ca sent an answer that is not a list of exchange rates`

### Edge rows (only cells that differ from the base)
Mid-feature ruling, SCENARIO-01 — nothing fetched, nothing failed (no warning, exit 0):

| Case | Text line | store.rates |
|---|---|---|
| Rates stored, nothing to ask (Need covered, or no transactions but rates carried) | `Rates     USD/CAD <first> to <last> (up to date)` | `{first,last,added:0,fetch_error:null}` |
| Rates stored, source answered empty for the asked span | `Rates     USD/CAD <first> to <last> (up to date)` | same; rates_checked_from advances |
| No rates, no transactions | `Rates     none (no transactions to convert)` | `{first:null,last:null,added:0,fetch_error:null}` |
| No rates, transactions exist, source answered empty | `Rates     none (the Bank of Canada has no rates for your transaction dates)` | same; later reports get the "no exchange rates" warning |

| Input | Text | json | --account | Totals |
|---|---|---|---|---|
| All-CAD, CAD | caption `amounts in CAD`; same numbers | currency:"CAD" | base | base |
| USD, USD | identity | base | base | base |
| Weekend or holiday | silent (ASOF) | base | base | base |
| After the last rate | silent; status shows the age | base | base | base |
| Before the first rate | a native row only in periods holding such an amount + the "before" warning | row currency ≠ .currency | base | extra native Total |
| Empty fx_rates, conversion needed | native rows + the "no rates" warning | same | base | per currency |
| Empty fx_rates, all CAD in CAD | silent | base | base | base |
| `--currency native` | exactly today | currency:"native" + additive fields | today | today |
| Config USD | caption `amounts in USD` | currency:"USD" | base | base |
| Bad config or malformed file | refusal exit 1, stdout empty; not reached when the flag is given | n/a | | |
| Bad flag | usage exit 2 | | | |
| Closed account | converts | | | |
| Future-dated | latest prior rate, silent | | | |
| Cross-currency transfer | still excluded | | | |
| Same store, spend vs cashflow | N may differ: each counts the transactions in its own report | | | |
| Empty window | existing empty note only; no FX warning | currency still set | | none |
| One USD `--account`, default CAD | converted; caption names the account and `amounts in CAD` | base | it | CAD Total |
| Recurring USD series in CAD | `CAD (USD)`; price changes `USD ...` | native fields | | included in the CAD Total |
| Same payee with CAD and USD series | two rows: `CAD` and `CAD (USD)` | native_currency differs | | |
| Anomaly in a USD account | label `(USD)`; Amount and Usual converted | native fields | | footer unchanged |
| findings with a USD account | unchanged | unchanged | | |
| accounts, not-imported | `In CAD` blank | converted_balance:null | | no Total |
| Sync: fetch fails mid-range | `(<n> new, not all fetched; see warning)` + the partial warning | rates.fetch_error, added n | | |
| Sync: zero new | `(up to date)` | added:0 | | |
| Sync `--from` an old snapshot | carried rates kept; fetch the missing ones | | | |
| First sync after the 4→5 upgrade | full back-fill; offline → "no rates" warning, native reports | | | |
| Quicken closed, recon failed, fingerprint changed | existing copy; no fetch; no Rates line | | | |

### Changes to existing surfaces
- **Combined carry warning** *(mid-feature ruling, SCENARIO-04)*.
  - Old (internal/snapshot/import.go:78): `cannot carry import history and findings forward from the previous store (<reason>); both start again with this sync`
  - New: `cannot carry import history, findings or exchange rates forward from the previous store (<reason>); all three start again with this sync`
  - Repoint its pins.
  - The `Warnings()` doc reads "the import-history restart, findings restart, exchange-rates restart and auto-prune warnings, in that order".
- **spend.go:20-23 Long.** The first paragraph becomes `Show how much you spent, grouped by category, payee, tag or month.` + P.
- **cashflow.go:45-47 Long.** It opens `Show income, spending and what was left over for each month or year.` + P. `…equals quarry spend's total for the same period and accounts.` becomes `…for the same period, accounts and currency.`
- **P (shared; pinned in report_help_test.go):**
```
Amounts are in CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
Each split converts at the Bank of Canada rate for its date, or the latest
earlier rate on weekends, holidays and dates after the last stored rate,
and is rounded to the cent before it is added. With --currency native, CAD
and USD are listed separately, never added together. Amounts dated before
the first stored rate stay in their own currency, on rows of their own,
with a warning.
```
- **recurring Long.** Insert after paragraph 1:
  `Series are found in each account's own currency, so a change in the exchange rate is never a price change, and a payee that charges in both CAD and USD has two series. Amount and Per year are converted to the reporting currency (--currency, else reporting.currency in the config file, else CAD) at the rate on the latest charge's date; price changes stay in the series' own currency. With --currency native nothing is converted.`
  Also `…from one charge to the next.` becomes `…from one charge to the next, in the series' own currency.`
- **anomalies Long.** `Charges under 100.00 are never listed.` becomes `Charges under 100.00 in their account's own currency are never listed.` Add:
  `Charges are judged in their account's own currency, so a change in the exchange rate never makes a charge unusual. Amount and Usual are then shown in the reporting currency (--currency, else reporting.currency in the config file, else CAD) at the rate on the charge's date. With --currency native nothing is converted.`
- **accounts Long.** Append:
  `A column shows each balance in the reporting currency (--currency, else reporting.currency in the config file, else CAD) at today's Bank of Canada rate, or the latest earlier one; --currency native leaves it out. quarry does not add balances together: a total that leaves out investment accounts would not be your net worth.`
- **sql.go:39 Long.** After the Amounts sentence:
  `v_cash_flow and v_spending also carry each amount in CAD and in USD (amount_cad and amount_usd; spent_cad and spent_usd), converted per split at the Bank of Canada rate for its date and rounded to the cent, as quarry spend and quarry cashflow convert; they are NULL for a date before the first rate. v_account_balances has balance_cad and balance_usd at today's rate. fx_rates holds one rate per business day: usd_cad is the Canadian dollars in one US dollar.`
- **status Long.** Append:
  *(Final pass: replaced, wrapped)*
  ```
  Rates shows the span of Bank of Canada USD/CAD rates the store holds and,
  when the last sync could not fetch new ones, why.
  ```
- **sync Long.** Insert before the snapshots paragraph:
  `sync then fetches the Bank of Canada's daily USD/CAD exchange rates for any dates the store does not have, back to your earliest transaction. This is quarry's only use of the network, and the request carries nothing but the dates. If the fetch fails, sync still succeeds, warns, and reports convert with the rates the store already has.`
- **PRD.**
  - :176 becomes `--currency CAD|USD|native` (default `reporting.currency`, CAD; native = each account's own, never added).
  - :244: the MCP currency takes the same three values.
  - Decisions gains `reporting.currency`.
- **Read commands** now emit unknown-key config warnings when `--currency` is absent.

### Every-X sites
- **run_read_usage_test.go:30-60:** bad `--currency` × spend, cashflow, recurring, anomalies and accounts (exit 2).
- **run_read_refusals_test.go:46-60 and :~174:** bad reporting.currency × the same five (exit 1).
- **run_config_test.go:209:** split.
- **report_help_test.go:** each Long, plus `--currency code` in the help.
- **json_*_internal_test key-set pins:** spend, cashflow, recurring, anomalies, accounts, status and sync.
- **run_cashflow_invariant_test.go:76:** see R2.

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — First sync back-fills exchange rates from the earliest transaction
  Given a store with no exchange rates and transactions back to 2005
  When I run quarry sync
  Then fx_rates holds Bank of Canada USD/CAD rates from the earliest transaction to the latest published day (FXUSDCAD where it has the date, IEXE0101 before it)
  And the output shows "Rates     USD/CAD <first> to <last> (<n> new)" and --json "rates" with first, last, added, fetch_error null

Scenario: SCENARIO-02 — Later sync fetches only the missing dates
  Given a store whose rates end before today
  When I run quarry sync
  Then the request asks only for the dates after the last stored rate, carried rates are kept, and the Rates line shows "(<n> new)", or "(up to date)" with added 0

Scenario Outline: SCENARIO-03 — Failed rate fetch warns and the sync still succeeds
  Given the Bank of Canada fetch fails with <failure>
  When I run quarry sync
  Then exit is 0, the new store is swapped in, stderr has the ruled one-line warning with reason "<reason>", --json warnings[] and rates.fetch_error carry it, and the Rates line says "(not refreshed; see warning)", "(<n> new, not all fetched; see warning)" or "none (not fetched; see warning)"
  Examples:
    | failure        | reason                                                                   |
    | unreachable    | cannot reach www.bankofcanada.ca                                         |
    | 30 s timeout   | no answer from www.bankofcanada.ca within 30 seconds                     |
    | HTTP 503       | www.bankofcanada.ca answered 503 Service Unavailable                     |
    | unparseable    | www.bankofcanada.ca sent an answer that is not a list of exchange rates |
    | partial range  | (partial-range warning copy)                                             |

Scenario: SCENARIO-04 — Rates survive the rebuild, including sync --from an older snapshot
  Given a store with rates and an older snapshot
  When I run quarry sync --from that snapshot
  Then every carried rate is still in fx_rates and only missing dates are fetched (a v4 store carries none, silently; an unreadable fx_rates warns "cannot carry exchange rates forward from the previous store (<reason>); fetching them all again")

Scenario: SCENARIO-05 — A sync that fails before the swap never fetches rates
  Given Quicken is open or the snapshot fails validation
  When I run quarry sync
  Then no network request is made, the existing copy and exit code are unchanged, and no Rates line is printed

Scenario Outline: SCENARIO-06 — Status shows rate coverage
  Given a store whose rates are <state>
  When I run quarry status
  Then the Rates line reads <text> and --json "rates" carries first, last and fetch_error
  Examples:
    | state                                  |
    | covered, age today / 1 day ago / N days ago |
    | first rate after first transaction     |
    | last fetch failed                      |
    | none                                   |

Scenario: SCENARIO-07 — Store views carry exact converted amounts
  Given rates including a weekend gap and amounts that convert to half a cent
  When I query v_cash_flow, v_spending and v_account_balances with quarry sql
  Then amount_cad/amount_usd (spent_*, balance_*) are DECIMAL, rounded half away from zero, use the latest earlier rate on weekends and holidays, equal the native amount for same-currency rows even without rates, and are NULL only before the first rate

Scenario: SCENARIO-08 — Spend converts to the reporting currency by default
  Given CAD and USD spending and full rate coverage
  When I run quarry spend
  Then the caption ends ", amounts in CAD", every row and the single Total are CAD, the CAD figure equals the sum of the converted splits, and --json has "currency": "CAD"

Scenario: SCENARIO-09 — Spend in USD and cashflow agree in every currency
  Given CAD and USD activity and full rate coverage
  When I run quarry spend and quarry cashflow with --currency USD
  Then spend's Total equals cashflow's Spent for the same period, accounts and currency

Scenario: SCENARIO-10 — Cashflow converts each period
  Given CAD and USD income and spending across months
  When I run quarry cashflow
  Then each month has one CAD row whose income, spent and net are the converted sums, and --json has "currency": "CAD"

Scenario Outline: SCENARIO-11 — --currency native reproduces today's output
  Given CAD and USD data
  When I run quarry <command> --currency native
  Then text is byte-identical to today's per-currency output, and --json has "currency": "native" plus the additive native_* fields equal to their converted twins
  Examples:
    | command   |
    | spend     |
    | cashflow  |
    | recurring |
    | anomalies |
    | accounts  |

Scenario: SCENARIO-12 — Amounts dated before the first rate stay native with a warning
  Given USD transactions dated before the first stored rate
  When I run quarry spend
  Then those amounts sit on USD rows with their own USD Total, stderr and warnings[] carry "<n> transactions dated before <date>, the first exchange rate in the store, are listed in USD, not converted to CAD", and exit is 0

Scenario: SCENARIO-13 — With no rates stored, reports fall back to native with a warning
  Given a store with no exchange rates
  When I run quarry spend on CAD and USD data
  Then rows are per native currency and the ruled "the store has no exchange rates…" warning prints (an all-CAD store reported in CAD prints no warning)

Scenario Outline: SCENARIO-14 — reporting.currency in config sets the default, the flag wins
  Given reporting.currency is <config> in config.toml
  When I run quarry spend <flag>
  Then amounts are in <result>, and the config is not read at all when --currency is given
  Examples:
    | config            | flag           | result |
    | unset             |                | CAD    |
    | USD               |                | USD    |
    | native            |                | native |
    | usd               |                | USD    |
    | USD               | --currency CAD | CAD    |
    | malformed file    | --currency CAD | CAD    |

Scenario Outline: SCENARIO-15 — A bad --currency value is a usage error
  When I run quarry <command> --currency EUR
  Then stderr is "quarry: --currency must be CAD, USD or native", exit 2, stdout empty
  Examples:
    | command   |
    | spend     |
    | cashflow  |
    | recurring |
    | anomalies |
    | accounts  |

Scenario Outline: SCENARIO-16 — A bad reporting.currency value refuses the read commands
  Given reporting.currency = <value> in config.toml
  When I run quarry <command>
  Then stderr is the ruled "reporting.currency must be CAD, USD or native, got <value>; fix the file and run the command again" refusal and exit 1 (quarry sql still ignores config)
  Examples:
    | value | command                                         |
    | "EUR" | spend, cashflow, recurring, anomalies, accounts |
    | ""    | spend                                           |
    | 12    | spend                                           |
    | true  | spend                                           |

Scenario: SCENARIO-17 — Recurring detects in native currency and shows converted amounts
  Given a USD subscription whose USD price is steady while the USD/CAD rate moves, and a payee charging in both CAD and USD
  When I run quarry recurring
  Then the USD series shows no price change, its Currency cell reads "CAD (USD)", Amount and Per year are converted at the latest charge's rate, the payee has two rows, and --json carries native_currency, native_amount and price_changes[].currency

Scenario: SCENARIO-18 — Anomalies are judged in native currency and shown converted
  Given a USD 90.00 charge (CAD ~123) and a USD charge far above its payee's USD baseline
  When I run quarry anomalies
  Then the 90.00 charge is not listed, the large one is, with Amount and Usual converted at its date's rate, and --json carries native_amount and native_usual

Scenario: SCENARIO-19 — Accounts show each balance in the reporting currency, never a total
  Given CAD and USD accounts including one not imported
  When I run quarry accounts
  Then an "In CAD" column shows each balance at today's rate (blank for not imported, "no rate" without rates), no total row exists, and --json rows keep native balance with converted_balance beside it

Scenario: SCENARIO-20 — Reference check against the real file and Bank of Canada
  Given a scratch copy of the latest real snapshot
  When I run sync --from it, then spend, cashflow, recurring, anomalies and accounts in CAD and USD
  Then the rate coverage, a sampled USD charge's conversion against the Bank of Canada's published rate for its date, and the converted totals look right to the user
```

---

## Sizing
Ruled (sizing pass 2026-10-01, architect):
- **Packages.**
  - New feature package `internal/fx` holds the `Source` port plus the Valet adapter beside it, with `WithHTTPClient`/`WithSource`. `Server.Refresh` plans the spans, chooses the series and classifies failures.
  - New leaf `internal/platform/money` holds the exact `Convert` (cents × rate, half away from zero) and the CAD/USD/native enum, with its behaviour classes pinned in its own package.
  - duckstore declares the consumer rates port, which SCENARIO-07 freezes. `*fx.Server` satisfies it directly. cmd/quarry wires `duckstore.New(dir, WithRates(fx.NewServer(...)))`.
- **`--currency` is resolved once in cli.** The flag is validated in Args (exit 2) and config is not read. With no flag, `loadConfig` → `cfg.Currency` → CAD. The value rides the request structs into `store.*Params`, and report never reads config. All five constructors gain `env.LoadConfig` (root.go:30-34).
- **Swap order** (duckstore.go:297-326): build → fetch → insert rates and the rates columns of import_runs → CheckpointClose → ctx check → rename. The 30 s limit is a child context per request; a parent cancel maps to the interrupted outcome, never to the timeout reason. Tests use synctest plus a fake RoundTripper.
- **Exact USD path.** DECIMAL/DECIMAL and integer `/` both give DOUBLE in DuckDB, so it uses integer arithmetic with an explicit half-away formula. Negative amounts and `typeof = DECIMAL(18,2)` are pinned.
- **No network in tests.** The scenario that wires the real client (01) makes the cmd/quarry test env default to a fake fetcher.
- **Interim state:** 16 binds `--currency` on all five commands. recurring, anomalies and accounts accept and resolve it but ignore it until 17–19.
- The overall order below is the build order.

| Scenario | Verdict — numbers |
| --- | --- |
| 07 | OWNS A RUN — 4 batches, duckstore + `platform/money`; test-first (rates hook in Replace's temp-then-rename): Convert + rounding; schema v5 (`fx_rates`, import_runs rates columns, FormatVersion 5); view columns + typeof + parity; rates port + `WithRates` in Replace |
| 01 | OWNS A RUN, 5 batches — orchestrator overruled SPLIT (no new Gherkin): B1 = `internal/fx` (Valet adapter, span planner, series choice), B2 = Replace integration, sync Rates line, `--json` rates, sync Long, cmd/quarry wiring + fake fetcher default; test-first |
| 02 | FOLD into 04 — the `(up to date)` / `added 0` arm of the span planner + carry |
| 04 | OWNS A RUN — 3 batches, duckstore: readHistory carries fx_rates (v4 → none, silent); unreadable fx_rates warning + full fetch; `--from` an older snapshot keeps rates (+02 arms) |
| 03 | OWNS A RUN — 4 batches, `fx` + duckstore + cli; test-first (failed fetch still swaps): reason per failure (30 s bound both sides); `rates_fetch_error` recorded, swap still happens, cancel = interrupted; 3 warnings + 2 Rates-line arms + JSON; folded 05 |
| 05 | FOLD into 03 — fake fetcher with 0 calls for Quicken open / validation failed / fingerprint changed |
| 06 | LIGHT — 3 steps, duckstore Status + cli: rates fields from the same read; 4 Rates-line arms (age 0/1/2 days); JSON `rates`, status Long |
| 16 | OWNS A RUN — 4 batches, config + cli + root: [15] shared `--currency` binder ×5 + help; `reporting.currency` parse; resolver (config only when the flag is absent, malformed refused, unknown-key warnings); all-commands rows + split run_config_test.go:209 |
| 15 | FOLD into 16 — flag binding and validation |
| 08 | OWNS A RUN — 3 batches, report + duckstore Spending + cli: SQL arms CAD/USD/native; caption, `currency`, spend Long P, JSON; folded 14 rows |
| 14 | FOLD into 08 — its Then needs spend to convert |
| 10 | OWNS A RUN — 3 batches, duckstore CashFlow + cli: converted income/spent/net; caption, JSON, cashflow Long; folded 09 invariant cases |
| 09 | FOLD into 10 — invariant coverage |
| 12 | OWNS A RUN — 3 batches, duckstore + report + cli: one read returns the unconverted count, first rate and whether any rates exist; warning choice + wording + order; `--json`/`--account`/extra native Total + invariant before-first case |
| 13 | FOLD into 12 — the no-rates arm |
| 17 | OWNS A RUN — 4 batches (B1 = 1–2, B2 = 3–4), duckstore Charges + report + cli: Charge gains converted amount + usd_cad; native detection with converted display (uncovered first/latest → native + series warning); `CAD (USD)` cell, price-change prefix, caption, totals; JSON native fields + recurring Long |
| 18 | OWNS A RUN — 3 batches, report + cli: Usual via money.Convert, floor/Times native; `USD 250.00` cells + charges warning; JSON native fields + anomalies Long |
| 19 | OWNS A RUN — 4 batches, duckstore + report + cli: balance_cad/usd read; In CAD column (blank / `no rate`), no total, no-rates warning; JSON converted_balance + accounts Long; folded 11 outline |
| 11 | FOLD into 19 — native arms are built and pinned by 08/10/17/18/19; 19 carries the 5-row outline |
| 20 | Reference check (below) — no production code; any gap becomes a new scenario before the gate |

## BDD Acceptance Progress
- [x] SCENARIO-07: Store views carry exact converted amounts — `cmd/quarry/run_sql_test.go` `Test_run_sql_views_carry_each_amount_converted_at_its_dates_rate`
- [x] SCENARIO-01: First sync back-fills exchange rates from the earliest transaction — `cmd/quarry/run_sync_rates_test.go` `Test_run_sync_back_fills_rates_from_the_earliest_transaction`
- [x] SCENARIO-04: Rates survive the rebuild, including sync --from an older snapshot — `cmd/quarry/run_sync_rates_carry_test.go` `Test_run_sync_from_an_older_snapshot_keeps_every_carried_rate`
- [x] SCENARIO-02: Later sync fetches only the missing dates — delivered by SCENARIO-04 — `cmd/quarry/run_sync_rates_carry_test.go` `Test_run_sync_asks_only_for_the_dates_after_the_last_stored_rate`
- [x] SCENARIO-03: Failed rate fetch warns and the sync still succeeds — `cmd/quarry/run_sync_rates_fetch_test.go` `Test_run_sync_warns_and_swaps_the_store_in_when_the_rate_fetch_fails`
- [x] SCENARIO-05: A sync that fails before the swap never fetches rates — delivered by SCENARIO-03 — `cmd/quarry/run_sync_rates_prefetch_test.go` `Test_run_sync_makes_no_rate_request_when_it_fails_before_the_swap`
- [x] SCENARIO-06: Status shows rate coverage — `internal/cli/status_test.go` `Test_status_prints_the_rates_line_for_each_coverage_state`
- [x] SCENARIO-16: A bad reporting.currency value refuses the read commands — `cmd/quarry/run_read_refusals_test.go` `Test_run_read_commands_refuse_a_bad_reporting_currency`
- [x] SCENARIO-15: A bad --currency value is a usage error — delivered by SCENARIO-16 — `cmd/quarry/run_usage_test.go` `Test_run_read_commands_refuse_a_bad_currency_flag`
- [x] SCENARIO-08: Spend converts to the reporting currency by default — `cmd/quarry/run_spend_fx_test.go` `Test_run_spend_converts_every_split_to_cad_by_default`
- [x] SCENARIO-14: reporting.currency in config sets the default, the flag wins — delivered by SCENARIO-08 — `cmd/quarry/run_spend_fx_test.go` `Test_run_spend_takes_its_currency_from_the_config_unless_the_flag_names_one`
- [x] SCENARIO-10: Cashflow converts each period — `cmd/quarry/run_cashflow_fx_test.go` `Test_run_cashflow_converts_each_period_to_cad_by_default`
- [x] SCENARIO-09: Spend in USD and cashflow agree in every currency — delivered by SCENARIO-10 — `cmd/quarry/run_cashflow_invariant_test.go` `Test_run_cashflow_spent_equals_spend_total_in_every_reporting_currency`
- [x] SCENARIO-12: Amounts dated before the first rate stay native with a warning — `cmd/quarry/run_spend_unconverted_test.go` `Test_run_spend_lists_a_split_before_the_first_rate_in_its_own_currency_and_warns`
- [x] SCENARIO-13: With no rates stored, reports fall back to native with a warning — delivered by SCENARIO-12 — `cmd/quarry/run_spend_unconverted_test.go` `Test_run_spend_without_rates_lists_each_currency_natively_and_warns_only_when_a_conversion_is_needed`
- [x] SCENARIO-17: Recurring detects in native currency and shows converted amounts — `cmd/quarry/run_recurring_fx_test.go` `Test_run_recurring_detects_in_native_currency_and_converts_at_the_latest_charges_rate`
- [x] SCENARIO-18: Anomalies are judged in native currency and shown converted — `cmd/quarry/run_anomalies_fx_test.go` `Test_run_anomalies_judges_in_native_currency_and_shows_converted_amounts`
- [x] SCENARIO-19: Accounts show each balance in the reporting currency, never a total — `cmd/quarry/run_accounts_fx_test.go` `Test_run_accounts_shows_each_balance_in_the_reporting_currency`
- [x] SCENARIO-11: --currency native reproduces today's output — delivered by SCENARIO-19 — `cmd/quarry/run_currency_native_test.go` `Test_run_currency_native_reproduces_the_pre_fx_output`

## Reference check
This is SCENARIO-20, the last step before the gate round. The outcome is recorded in `REFERENCE-CHECK.md` beside this file.
- [x] Rates and conversions on the real Quicken file hold up to review (see `REFERENCE-CHECK.md`, 2026-10-01). Steps:
  - Use a scratch HOME from the latest snapshot and run `sync --from`.
  - Run spend, cashflow, recurring, anomalies and accounts with `--currency CAD` and `--currency USD`.
  - Compare a sampled USD charge's converted amount against the Bank of Canada's published rate for its date.
  - The user reviews the rate coverage and the converted totals. Any gap becomes a new scenario before the gate.
