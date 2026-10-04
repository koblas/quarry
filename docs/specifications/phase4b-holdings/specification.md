# Specification: Phase 4b — holdings and their value

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: the user can ask "what do I hold in my investment accounts, and what is it worth on a day?" and get an answer from quarry. The answer comes from `quarry holdings`, MCP `holdings` and a `v_holdings` view. All three are built on the same share counts that `quarry sync` already checks against Quicken (Phase 4a).

**Secondary Goals**:
- One owner for each rule. The 4a walk owns share counts over time, through the new `holding_shares` table. `v_holdings` is the only owner of price choice, value rounding and FX for holdings. The CLI and MCP read the view.
- Staleness is visible, not hidden: each row shows its price and that price's date.
- Two Phase 4a debts close here:
  - The SQL conventions list the 13 action values.
  - Table widths are measured in runes, not bytes.

**Out of Scope**:
- Cash in investment accounts, investment account balances, and the `accounts` "not valued" cell. All move to 4c, user decision 2026-10-04. 4c imports investment cash and reconciles it, and then values accounts as holdings plus cash.
- The Balances clause "N investment accounts not checked". This stays until 4c.
- Security `type`, deferred because Quicken's type codes are unlabelled: only code 6 has a label, "Other".
- Staleness thresholds.
- Quicken statement values (`ZFIPOSITION`/`ZFISTATEMENT`): 55 of 77 are zero.
- `quarry networth` (4c), ACB (4e), MCP `net_worth`/`acb` (4f), the monthly summary (4g).
- A manual comparison against Quicken's Portfolio view, which the user declined (see *Reference check*).

**User decisions (2026-10-04)**:
1. An investment account's value is holdings (shares × last price on or before the date) plus cash, computed by quarry. 4b delivers the holdings half. Cash and account balance are 4c (product-vision C1, accepted).
2. Surface: view plus command. That is `v_holdings`, `quarry holdings` and MCP `holdings`.
3. A stale or missing price uses the last known price and shows its date. The value is NULL only when there has never been a price on or before the date. No threshold.
4. The reference check is automated only. The user declined the manual Portfolio-view comparison.
5. Scenarios approved 2026-10-04.

## Business Rules & Invariants

- **H-1 `holding_shares` table**, Go-built at Replace by the 4a walk; it is not a second owner of the share count.
  - Columns: `account_id`, `security_id`, `from_date` DATE, `to_date` DATE (NULL while still held), `shares` DECIMAL(18,6).
  - One row per span of days over which the end-of-day count is unchanged and non-zero.
  - The walk (`internal/store/duckstore/shares.go` `holdingShares`) is extended to emit the running count after each date:
    - order is date, then `source_id`;
    - the split multiply is applied as in 4a;
    - same-day rows are netted;
    - a zero span is not stored, and no row means 0 shares;
    - negative counts are stored as recorded.
  - The 4a gate's final count per holding equals that holding's last span (one function).
  - `FormatVersion` 6 → 7, because 6 has shipped. An older store gets the existing format refusal, and a re-sync rebuilds it.
- **H-2 `v_holdings` view**, daily grain.
  - One row per holding per calendar day held, from `from_date` through `least(coalesce(to_date, today), today)`.
  - Columns: `date`, `account_id`, `security_id`, `security`, `ticker`, `shares`, `price`, `price_date`, `currency`, `value`, `value_cad`, `value_usd`, `usd_cad`.
  - `price` and `price_date` come from the latest `prices` row on or before `date` (ASOF). Zero and placeholder (1899-12-29) prices are used as recorded.
  - `currency` is `securities.currency` as stored. NULL stays NULL; there is no fallback to the account's currency.
  - `value` = `CAST(shares*price AS DECIMAL(38,2))` (orchestrator ruling 2026-10-04, SCENARIO-02: `value`, `value_cad`, `value_usd` are DECIMAL(38,2) so no holding can overflow; operands cast to DECIMAL(19,6), product exact at DECIMAL(38,12)); the cast rounds half away from zero, as `convertedTo` does. `value` is NULL when there is no price.
  - `value_cad` and `value_usd` = `convertedTo(value, currency, rate)` at the ASOF rate on `date` (PRD L243). They are NULL when `currency` is NULL, when it is not CAD/USD, or when there is no rate.
  - The CLI and MCP read this view.
  - `COMMENT ON VIEW v_holdings` (R2): `one row per holding per day it is held, through today, so filter by date; value is shares times price rounded to the cent, value_cad and value_usd convert it at the rate for date as quarry holdings does; cash in investment accounts is not included.` (schema.md view-comment pin 2 → 3)
  - **Architect precondition:** `WHERE date = ?` must run in under 1 s on the real store. If it does not, the fallback is a DuckDB table macro or a Go reader over `holding_shares` with an ASOF price and FX join, under the same column names. That fallback needs a scoped product re-ruling first.
- **H-3 Totals** are the sum of the rounded row values (text and `--json`). They are never added across currencies.
- **H-4 FX span** (`internal/store/duckstore/rates.go:130` `needSpan`) runs from the earliest of the cash and investment transaction dates to today. Price dates never extend it.
- **H-5 Currency.** A NULL or non-CAD/USD security currency is not converted. It gets a warning and is left out of the converted total.
- **H-6 As-of.**
  - `--as-of` accepts YYYY, YYYY-MM or YYYY-MM-DD. A year or month means its last day; the current year or month resolves to today.
  - The default is today.
  - A date after today is refused (exit 2).
  - A future-dated investment transaction is not counted before its date.

---

## Triage Brief

Scoping notes, including the triage summary and the probe: triage agent report (2026-10-04) plus the orchestrator probe below.

**Builds on (do not re-plan):**
- The 4a walk in `internal/store/duckstore/shares.go`: `holdingShares`, `holdingKey`, `splitRatio`, `parseDecimal` and `millionthsOf`, with exact `big.Rat` and tolerance 0.000001. It is date-blind today, with a single caller, `deriveShares`, run on the scratch DB.
- `prices` holds one row per security-day, rounded half-even to 6 decimals.
- `securities.currency` is stored as recorded.
- Formatters `formatShares` and `document.Shares`.
- The 4a mismatch label rule: name plus ` (TICKER)` when the ticker differs.
- `convertedTo` / `convertedColumn` (`internal/store/duckstore/convert_sql.go:11,31`), and the ASOF FX template in `v_cash_flow` (`schema.go:212,228`).
- `accountsSchema` / `currencySchema` in `internal/mcp/tools.go:243-246`.
- `renderTable`, `windowCaption`, `escapeCell` and `formatMoney` in `internal/cli`.
- The `--currency` / `reporting.currency` resolution (`internal/cli/currency.go`).
- The "no account named" refusal used by spend.

**Must be built:**
- The span emitter from the walk, the `holding_shares` table and its loader, and the format bump.
- The `v_holdings` view.
- `quarry holdings` (cobra command, text and JSON renderers) and the `document.Holdings` document.
- The MCP `holdings` tool.
- The `needSpan` extension.
- Copy changes (S.7).
- Rune-width `widestLen`.

**Caller table** (`.claude/briefs/navigation.md`):

| Symbol | Callers | Via |
| --- | --- | --- |
| `holdingShares` | `duckstore/shares.go:74` (`deriveShares`) only | LSP |
| `needSpan` | `duckstore/rates.go:57` (`refreshRates`) | LSP |
| `accountBalance` | `cli/render_accounts.go:33` | LSP |
| `FormatVersion` | `duckstore.go:25,252`, `rates.go:47`; tests `run_status_json_test.go:112`, `run_store_info_test.go:73`, `run_shared_documents_test.go` (literal), `duckstore_test.go:182`, `open_test.go:180-186,338`, `status_test.go:36` | LSP (from 4a; the architect re-runs it) |
| `v_holdings` / `phase4ViewPattern` | `cmd/quarry/run_skill_references_test.go:21,100-101`; `run_skill_schema_reference_test.go:182` | grep |
| `does not compute holdings` | `plugin/skills/quarry/SKILL.md:72-73`; `cmd/quarry/run_skill_text_test.go:165,218-219` | grep |
| `widestLen` | `internal/cli/render*.go` (the architect runs findReferences) | gap |

**Real-file probe** (read-only, counts only, snapshot `20260930T072052Z`):
- `ZSECURITY.ZTYPE` counts: 3→11, 4→33, 6→8, 7→32. The only label is `ZFIPOSITION` type 6 = "Other".
- Holdings today: 21 holdings over 20 securities. All 21 have a quote at most 7 days old (as of 2026-09-30). None is held at a zero price; 24 securities have some zero-price quote.
- 11 securities have no quote on or before their first trade.
- Security currency always equals account currency: CAD/CAD 6 securities, USD/USD 67; held now CAD 4, USD 17. The 11 NULL-currency securities have no positions.
- Investment transactions run 2013-02-07 to 2026-09-23. Cash transactions start 2013-02-07. Quotes run 1899-12-29 (placeholder) to 2026-09-29.
- `ZFISTATEMENT`: 6 rows, 2018-09-07 to 2026-09-23. `ZFIPOSITION`: 77 rows, with `ZMARKETVALUE` 0 on 55 of them.

## Product Verdict

**SHIP WITH CHANGES** (product-vision, 2026-10-04). All changes are folded into this spec.
1. **C1:** cash and the `accounts` cell move to 4c. The user accepted this.
2. `holding_shares` is Go-built from the 4a walk. `v_holdings` is the only SQL owner of price, rounding and FX. The daily grain carries the under-1 s precondition and its fallback.
3. NULL and non-CAD/USD currency is not converted: it gets a warning and is left out of the total. There is no silent fallback.
4. **C2:** MCP `holdings` is in. **C3:** security `type` is out.
5. **C4:** the action vocabulary sentence and rune widths fold in.
7. Copy rulings R1–R3 (2026-10-04): native totals per stored currency (CAD, USD, others alphabetical); NULL currency never totalled; one NULL-currency warning wording; EUR warning only in converted modes; `COMMENT ON VIEW v_holdings`; warning order; architect span / to_date / no-rate / --account defaults confirmed.
6. The reference check is the last scenario. The manual Portfolio comparison was declined by the user, so the split-adjustment question for past values is recorded as unverified.

## Surface & Copy

### S.1 Command

`Short: "List the securities held in each account and their value"`

Long (verbatim, wrapped as shown):
```
List each security held in a brokerage or retirement account on one day
(--as-of, default today): its share count, the latest price Quicken
recorded on or before that day with that price's date, and its value,
shares times price rounded to the cent. Share counts are the ones quarry
sync checks against Quicken. A holding with no price on or before that day
is listed with no value and left out of the total.

Values are in each security's own currency. A column shows each value in
the reporting currency (--currency, else reporting.currency in the config
file, else CAD) at the Bank of Canada rate for the --as-of day, or the
latest earlier one; --currency native leaves it out and totals each
currency separately.

The total is the value of the securities only. Cash held in investment
accounts is not included, so it is not those accounts' balance.
```
Example:
```
  quarry holdings
  quarry holdings --as-of 2025-12-31
  quarry holdings --account RRSP --currency native --json
```
Flags (the backticked word is the placeholder):
- `--as-of`: `value holdings on `date` (YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day; default today)`
- `--account`: `list only the account with this `name` or id; repeat for more`
- `--currency`: `add a column with each value in currency `code`: CAD or USD; native adds none (default reporting.currency in the config file, else CAD)`
- `--json` (the existing global flag).

There is no `--all` flag.

### S.2 Text output

```
Holdings on 2026-10-04 in all accounts, amounts in CAD; cash not included

Account    Security                        Shares   Price  Priced on   Currency      Value     In CAD
Brokerage  iShares Core Equity ETF (XEQT)   1,200   31.42  2026-09-29  CAD       37,704.00  37,704.00
IRA        Vanguard Total Stock (VTI)          85  290.11  2026-09-29  USD       24,659.35  33,540.12
Total                                                                                       71,244.12
```
- **Total row.** In converted mode the `Total` row has only `Total` and the In-currency sum.
- **Native mode.**
  - There is no In column.
  - There is one `Total` row per stored currency code, with the Currency cell and the Value sum filled, sorted CAD, USD, then other codes alphabetically (R1).
  - A NULL-currency row is never totalled, in any mode (R1).
  - The caption drops ", amounts in X".
- **Cells.**
  - **Account:** the name through `escapeCell`, plus ` (closed)` for a closed account.
  - **Security:** the 4a label, the name plus ` (TICKER)` when the ticker is non-NULL and differs from the name.
  - **Shares:** `formatShares`.
  - **Price:** trailing zeros trimmed, at least 2 decimals.
  - **Priced on:** the price date, blank when there is no price.
  - **Currency:** `none` when NULL.
  - **Value:** `formatMoney`.
- **Sort:** account name, account source_id, security name, security source_id.
- **Caption:** names the accounts the way `windowCaption` does.

| Case | Price | Value | In column | In total? |
| --- | --- | --- | --- | --- |
| No price ever on/before as-of | `no price` | blank | blank | No |
| No rate for as-of (before first rate, or none) | as recorded | as recorded | `no rate` | Left out of the In total; each unconverted currency gets its own `Total` row with its Value sum, below it, e.g. `Total` … `USD` … `24,659.35` |
| Currency NULL or not CAD/USD | as recorded | as recorded | `not converted` | No |

### S.3 Warnings

Stderr lines take the prefix `quarry: warning: `. They exit 0, and each is also added to `warnings[]`.
- **No price:** `1 holding has no price on or before 2026-10-04, so it has no value and is left out of the total; enter a price for it in Quicken, then run quarry sync`. Plural (ruled 2026-10-04): `N holdings have no price on or before <d>, so they have no value and are left out of the total; enter a price for each in Quicken, then run quarry sync`.
- **Before the first rate:** `1 holding valued on 2012-12-31, before 2013-01-02, the first exchange rate in the store, is not converted to CAD and is totalled in USD`. Plural (ruled 2026-10-04): `N holdings valued on <d>, before <first>, the first exchange rate in the store, are not converted to <CAD|USD> and are totalled in <USD|CAD>` — one warning always names one currency (only CAD/USD rows reach it; EUR/NULL have their own).
- **No rates at all:** `the store has no exchange rates, so values are listed in each security's own currency; run quarry sync to fetch them`
- **Currency NULL** (every mode, R1): `"<security>" has no currency in Quicken, so quarry leaves its value out of the total; set its currency in Quicken, then run quarry sync`
- **Other currency** (CAD/USD modes only; native mode totals it, so no warning — R1): `"<security>" is priced in EUR, which quarry does not convert, so its value is left out of the total`
- **Non-investment `--account`:** `account "Chequing" is not a brokerage or retirement account, so it has no holdings`
- **Empty result.** Stdout still prints the caption and header, with no Total row. Stderr shows one of:
  - `no holdings on 2012-01-01; the store's investment transactions run 2013-02-07 to 2026-09-23`
  - with `--account`: `no holdings on <d> in the named accounts; their investment transactions run <a> to <b>`, or `…; they have no investment transactions`
  - no investment data: `no holdings on <d>; the store has no investment transactions`

- **Order (R3)**, stderr and `warnings[]` alike: (1) config warnings, (2) non-investment `--account`, (3) empty result, (4) no price, (5) no exchange rates / before first rate, (6) no currency, (7) other currency. Several of one kind follow the table sort.

### S.4 Refusals

| Input | Line | Exit |
| --- | --- | --- |
| Bad `--as-of` | `quarry: --as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` | 2 |
| `--as-of` after today | `quarry: --as-of 2027-01-01 is after today; holdings are valued up to today only, so pass an earlier --as-of` | 2 |
| Current year or month | Resolves to today; the caption shows the day | 0 |
| Unknown `--account` | Existing `no account named "X"; run quarry accounts --all to list them` | as in spend |
| No store, or an older-format store | Existing lines | 1 |
| Bad `--currency`, or a positional argument | Existing lines | 2 |

### S.5 `--json` and MCP

```json
{"as_of":"2026-10-04","currency":"CAD","account_filter":[],
 "holdings":[{"account_id","account","account_closed","security_id","security","ticker",
   "shares":"1200.000000","price":"31.420000"|null,"price_date":"2026-09-29"|null,
   "currency":"CAD"|null,"value":"37704.00"|null,"converted_value":"37704.00"|null}],
 "totals":[{"currency":"CAD","value":"71244.12"}],"warnings":[]}
```
- `converted_value` is null in native mode.
- `totals` has one entry per non-NULL currency actually totalled, in Total-row order (spend precedent; R1).
- `holdings`, `totals` and `account_filter` are `[]` when empty, never null.
- Text is raw.
- `shares` and `price` carry exactly 6 decimals (`document.Shares`).

MCP tool `holdings`:
- Parameters: `as_of` (string, same forms), `accounts`, and `currency`. `accounts` and `currency` reuse `accountsSchema` / `currencySchema`.
- Result: the same document.
- Description: `Securities held on one day with share count, latest price and its date, and value; cash in investment accounts is not included.`
- Refusals use the sibling tools' error shape.

### S.6 Edge-case rows

| Input | Outcome |
| --- | --- |
| No holdings / no investment data / as-of before the first trade | Caption + header; empty warning (S.3); exit 0 |
| Never priced on or before as-of | `no price` row; warning; not in total |
| Old price | Price + `Priced on` as recorded; no hint |
| Price 1899-12-29 placeholder | As recorded |
| Zero price | Value `0.00`; in total; no hint |
| Split mid-history | Shares change on the split date via the walk; price as recorded that day; quarry adjusts nothing |
| Closed account with shares | Listed; ` (closed)`; `account_closed: true` |
| Not-in-reports / linked-tracking account | Listed; no left-out warning |
| NULL / other security currency | S.2 / S.3 |
| As-of in the future | Refusal, exit 2; current YYYY or YYYY-MM resolves to today |
| As-of on a rate gap (weekend, holiday) | Latest prior rate, silent |
| As-of after the last stored rate | Latest prior rate, silent (`status` shows the lag) |
| As-of before the first rate, or no rates | `no rate`; per-currency Total row; warning |
| Negative shares | Listed; negative value; in total |
| Future-dated investment transactions | Not counted before their date |
| CAD vs USD | Native value plus In column; never summed across currencies |

### S.7 Changes to existing surfaces

| Where | New |
| --- | --- |
| `internal/cli/accounts.go:17-19` (Long) | `Brokerage and retirement accounts show "not valued": quarry values their holdings (quarry holdings) but not yet the cash in them, so it cannot compute their balance.` |
| `internal/report/sql_conventions.go`: replace `quarry does not convert prices yet.` | `holding_shares holds each account's count of each security, one row per span of days it is unchanged and not zero (from_date through to_date, NULL while still held), splits applied; these are the counts quarry sync checks against Quicken. v_holdings has one row per holding per day held, through today: price is the latest on or before date and price_date its day (NULL when none), value is shares times price rounded to the cent, value_cad and value_usd convert it at the rate for date, as quarry holdings does; filter it by date. Neither includes cash in investment accounts.` |
| `sql_conventions.go`: add (generated from `store.Action*`) | `action is one of add_shares, buy, capital_gain_long, capital_gain_short, dividend, interest, margin_interest, misc_expense, misc_income, reinvest_dividend, remove_shares, sell, split.` |
| Hand copies of conventions (`internal/cli/sql_test.go`, `cmd/quarry/run_shared_documents_test.go`) + `schema.md` | Re-pin + regenerate |
| SKILL.md frontmatter | Add use: `what they hold in investment accounts and its value on a day;`. The exclusion becomes `…for net worth, gains, dividend totals or ACB…` ("holdings" removed) |
| SKILL.md §4 | Add row `\| Holdings and their value on a day \| quarry holdings --as-of <date> --json \|` |
| SKILL.md §5 | Add `For holdings over time query v_holdings; never sum investment_transactions.shares.` |
| SKILL.md:72 | `- **Net worth:** "quarry does not compute net worth yet: it values investment holdings but not the cash in investment accounts." quarry accounts lists the other balances and quarry holdings the holdings; don't add them up.` |
| SKILL.md:73 | `- **Dividends, realized gains, ACB:** "quarry imports investment transactions and values holdings, but does not compute dividends, gains or ACB yet."` |
| Pins | Remove `holdings` from `phase4ViewPattern` (`cmd/quarry/run_skill_references_test.go:21`) and its case `:100-101`; remove `v_holdings` from `run_skill_schema_reference_test.go:182`; re-pin `run_skill_text_test.go:165,218-219` |
| `docs/initial-prd.md` | L123 `(type deferred: Quicken's type codes are unlabelled; …)`; derived views `v_holdings (shares and value by day, from holding_shares; Phase 4b)`; CLI table add `quarry holdings`; MCP table add `holdings` |
| `internal/cli` `widestLen` | Measure runes (4a debt) |
| MCP `instructions` const | Unchanged (grep for "investment" to confirm) |

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — Sync records each holding's share count over time
  Given investment transactions that buy, sell, and split a security across several dates, including two on the same day and one in the future
  When the user runs quarry sync
  Then holding_shares holds one row per unchanged non-zero span with from_date and to_date, the last span's shares equal the share-check count, and the store is format 7

Scenario: SCENARIO-02 — v_holdings values each holding on each day held
  Given a store with holding spans, prices on some days, and CAD/USD exchange rates
  When the user queries v_holdings for a date
  Then each held holding has the latest price on or before that date with its price_date, value is shares times price rounded to the cent, and value_cad and value_usd use that date's rate

Scenario: SCENARIO-03 — quarry holdings lists today's holdings in the reporting currency
  Given a store with holdings in CAD and USD accounts, one in a closed account
  When the user runs quarry holdings
  Then stdout shows the caption ending "; cash not included", one row per holding with Shares, Price, Priced on, Currency, Value and In CAD, a Total row, and the exit code is 0

Scenario: SCENARIO-04 — Native currency lists each currency's own total
  Given holdings in CAD and USD
  When the user runs quarry holdings --currency native
  Then there is no In column and one Total row per currency, CAD then USD

Scenario: SCENARIO-05 — Holdings on a past date
  Given a holding whose shares changed through a split before 2025-12-31
  When the user runs quarry holdings --as-of 2025
  Then the caption shows 2025-12-31 and the row shows the shares held that day after the split, with the price recorded on or before it

Scenario: SCENARIO-06 — A holding with no price is listed without value
  Given a holding whose security has no price on or before the as-of day
  When the user runs quarry holdings
  Then its row shows "no price" and blank value, it is left out of the total, and stderr warns "1 holding has no price on or before <d>, so it has no value and is left out of the total; enter a price for it in Quicken, then run quarry sync"

Scenario: SCENARIO-07 — A security quarry cannot convert is left out of the total
  Given one held security with no currency and one priced in EUR
  When the user runs quarry holdings
  Then both rows show "not converted" in the In column, neither is in the total, and stderr carries the ruled no-currency and other-currency warnings

Scenario: SCENARIO-08 — A day with no exchange rate totals each currency separately
  Given a USD holding valued on a day before the store's first exchange rate
  When the user runs quarry holdings --as-of that day
  Then the In column shows "no rate", a separate USD Total row carries its value, and stderr carries the ruled before-first-rate warning

Scenario: SCENARIO-09 — Nothing held on the day
  Given a store whose investment transactions start after the as-of day
  When the user runs quarry holdings --as-of an earlier day
  Then stdout shows the caption and header with no Total row, stderr warns "no holdings on <d>; the store's investment transactions run <a> to <b>", and the exit code is 0

Scenario: SCENARIO-10 — Filtering by account
  Given a brokerage account, a retirement account and a chequing account
  When the user runs quarry holdings --account <brokerage> --account Chequing
  Then only the brokerage holdings are listed, the caption names the accounts, and stderr warns that Chequing "is not a brokerage or retirement account, so it has no holdings"

Scenario Outline: SCENARIO-11 — quarry holdings refuses a date it cannot use
  Given a store with holdings
  When the user runs quarry holdings --as-of <value>
  Then stderr is "<line>" and the exit code is 2

  Examples:
    | value      | line                                                                                                         |
    | 2024-13    | quarry: --as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD                                      |
    | 2099-01-01 | quarry: --as-of 2099-01-01 is after today; holdings are valued up to today only, so pass an earlier --as-of   |

Scenario: SCENARIO-12 — Holdings as JSON
  Given a store with holdings, one without a price
  When the user runs quarry holdings --json
  Then stdout is the ruled document with holdings, totals and account_filter always present, shares and price with 6 decimals, null price, price_date and value for the unpriced holding, and warnings listing it

Scenario: SCENARIO-13 — MCP holdings tool
  Given a store with holdings
  When Claude calls the MCP tool holdings with as_of and currency
  Then the result is the same document quarry holdings --json prints for those arguments

Scenario: SCENARIO-14 — Sync fetches exchange rates back to the earliest investment transaction
  Given a store whose earliest investment transaction is before its earliest cash transaction
  When the user runs quarry sync
  Then the exchange-rate fetch starts at the investment transaction's date

Scenario: SCENARIO-15 — Existing surfaces describe holdings
  Given a store built by this version
  When the user reads quarry accounts help, the SQL conventions and SKILL.md
  Then they carry the ruled holdings copy, the action vocabulary sentence, schema.md lists holding_shares and v_holdings, and table columns align for non-ASCII names

Scenario: SCENARIO-16 — Reference check against the real Quicken file
  Given the user's snapshot 20260930T072052Z
  When the orchestrator runs quarry holdings --as-of 2026-09-30 --json on a store built from it
  Then 21 holdings over 20 securities are listed, each holding's shares equal the share-check count, every holding has a price dated on or after 2026-09-23, and the CLI total equals sum(value_cad) from v_holdings for that date
```

---

## Sizing

Architect sizing pass 2026-10-04 (one pass over all 16; folds change no scenario's assertions; every folded scenario keeps its own acceptance test). Package count per phase4a precedent (store/duckstore/report/document count with `report`; cli/cmd/plugin do not; `mcp` own).

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN (opus), absorbs SCENARIO-14 — 4 batches, duckstore; span emitter from the 4a walk (gate final count = last span, one function), `holding_shares` DDL + load inside Replace, **FormatVersion 6→7 (only bump)** + re-pins + schema.md, `needSpan` extension; expect test-first (new fallible load before rename; mutation: failed load leaves previous store byte-identical) |
| SCENARIO-02 | OWNS A RUN (opus) — 3 batches; `v_holdings` (CAST generate_series to DATE; ASOF price; no window/DISTINCT above the day expansion), value/FX arms, typed reader `(*Store).Holdings`; **retires the `v_holdings` pins** (`run_skill_references_test.go:21,100-101`, `run_skill_schema_reference_test.go:182`) because schema.md regen forces it |
| SCENARIO-03 | OWNS A RUN (opus) — 4 batches; `report` port + `Server.Holdings`, command (Short/Long/Example, `--currency`), text renderer, `document.Holdings` + `--json`, root registration + all-commands tables, warnings plumbing; fixes stderr warning order (R3) |
| SCENARIO-04 | LIGHT — native totals per currency + renderer arm |
| SCENARIO-05 | OWNS A RUN (sonnet), absorbs SCENARIO-11 — 2 batches; exported `--as-of` parser in `report` (reused by MCP), flag, refusals, caption |
| SCENARIO-06 | OWNS A RUN (sonnet), absorbs SCENARIO-12 — 2–3 batches; no-price row + warning (1/2 bound) + JSON nulls |
| SCENARIO-07 | OWNS A RUN (sonnet) — 2 batches; `not converted` + two warnings (needs R1) |
| SCENARIO-08 | OWNS A RUN (sonnet) — 3 batches; first-rate/no-rates facts in the same port call, two new warning composers (existing `beforeFirstRateWarning` wording differs), `no rate` cell + unconverted Total rows |
| SCENARIO-10 | OWNS A RUN (sonnet) — 2–3 batches; repeatable `--account`, non-investment warning, caption, `account_filter` (built before S09) |
| SCENARIO-09 | OWNS A RUN (sonnet) — 3 batches; investment-transaction span in the port call, three empty-result warnings, no Total row when empty |
| SCENARIO-13 | LIGHT — MCP tool with shared schemas, `as_of` via S05 parser |
| SCENARIO-15 | OWNS A RUN (sonnet) — 3 batches; conventions (holdings + action sentence from a new exported `store` action list), hand copies, schema.md; accounts Long, SKILL.md, PRD, skill-text re-pins; `widestLen` runes (`render.go:495`) |
| SCENARIO-11 | FOLD into SCENARIO-05 |
| SCENARIO-12 | FOLD into SCENARIO-06 |
| SCENARIO-14 | FOLD into SCENARIO-01 |
| SCENARIO-16 | No architect/developer — orchestrator reference check; also times `v_holdings WHERE date = '2026-09-30'` on the real store (H-2 evidence) |

**H-2 precondition (measured by the architect, synthetic worst case):** 723,260 view rows (145 holdings × 40 spans, 2013→today, 520k prices, 3.6k rates): full `count(*)` 157 ms; `WHERE date = ?` 26–28 ms warm with the filter pushed below both ASOF joins. No fallback planned.

**Confirmed by product-vision (2026-10-04, R1–R3 ruling):** span boundaries and zero test use the half-even millionths count; `to_date` inclusive (day before the change); the 4a gate stays an exact compare of the walk's final `big.Rat` count (unchanged; orchestrator ruling 2026-10-04) and the same walk emits the spans, so the open span's shares = that count rounded half-even to millionths, no open span ⇔ it rounds to 0; "no exchange rates" warning only when a row shows `no rate`; `--account` naming only non-investment accounts prints both warnings. The view's `today` is DuckDB `current_date` (test clocks must not be later than the real date).

## BDD Acceptance Progress

- [x] SCENARIO-01: Sync records each holding's share count over time — `cmd/quarry/run_holding_shares_test.go` `Test_run_sync_records_each_holdings_share_count_over_time`
- [x] SCENARIO-14: Sync fetches exchange rates back to the earliest investment transaction — delivered by SCENARIO-01 — `internal/store/duckstore/rates_test.go` `Test_replace_asks_for_rates_from_the_earliest_investment_transaction`
- [x] SCENARIO-02: v_holdings values each holding on each day held — `cmd/quarry/run_holdings_view_test.go` `Test_run_sql_values_each_holding_on_a_date_from_v_holdings`
- [x] SCENARIO-03: quarry holdings lists today's holdings in the reporting currency — `cmd/quarry/run_holdings_test.go` `Test_run_holdings_lists_todays_holdings_in_the_reporting_currency`
- [x] SCENARIO-04: Native currency lists each currency's own total — `cmd/quarry/run_holdings_test.go` `Test_run_holdings_native_lists_each_currencys_own_total`
- [x] SCENARIO-05: Holdings on a past date — `cmd/quarry/run_holdings_as_of_test.go` `Test_run_holdings_as_of_a_year_lists_the_shares_after_a_split_before_it`
- [x] SCENARIO-11: quarry holdings refuses a date it cannot use — delivered by SCENARIO-05 — `cmd/quarry/run_holdings_as_of_test.go` `Test_run_holdings_refuses_a_date_it_cannot_use`
- [x] SCENARIO-06: A holding with no price is listed without value — `cmd/quarry/run_holdings_no_price_test.go` `Test_run_holdings_lists_a_holding_with_no_price_without_value`
- [x] SCENARIO-12: Holdings as JSON — delivered by SCENARIO-06 — `cmd/quarry/run_holdings_no_price_test.go` `Test_run_holdings_json_lists_the_unpriced_holding_with_nulls_and_a_warning`
- [ ] SCENARIO-07: A security quarry cannot convert is left out of the total
- [ ] SCENARIO-08: A day with no exchange rate totals each currency separately
- [ ] SCENARIO-10: Filtering by account
- [ ] SCENARIO-09: Nothing held on the day
- [ ] SCENARIO-13: MCP holdings tool
- [ ] SCENARIO-15: Existing surfaces describe holdings

## Reference check

SCENARIO-16 is run by the orchestrator after SCENARIO-15 and before the gate, the same way as in phase4a: a scratch HOME holding copies of the snapshot and the store. Results go in `REFERENCE-CHECK.md`. If it finds a rule gap, that gap becomes a new scenario appended above.

**Unverified by user decision:** whether Quicken's price history is split-adjusted for past dates that cross a `ZSECURITYSPLIT` date, i.e. whether a past `value` would match Quicken's Portfolio view. The manual comparison was declined, and the conventions do not claim that past values are verified.

- [ ] SCENARIO-16: Reference check against the real Quicken file
