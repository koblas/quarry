# Specification: Phase 4a — investment import and share-count gate

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry sync` imports the user's securities, their price history and every investment transaction from Quicken, and swaps the new store in only when, for every holding (account × security), the share count quarry derives from those transactions equals Quicken's own. This is the PRD Phase 4 gate ("Share counts match Quicken") and the base every later Phase 4 slice trusts.

**Secondary Goals**:
- Retire "not imported" from every surface (sync, status, accounts, `--json`, MCP `sync_status`, `import_runs`) once nothing is skipped.
- Keep spending and cash-flow totals unchanged: investment cash stays out of `transactions`, `splits`, `transfers`, `v_cash_flow` and `v_spending` in 4a, so the Phase 2 gate still holds.
- The share count is computed by one owner from the built store, so 4b's holdings reuse it rather than re-derive it.

**Out of Scope** (Phase 4 roadmap, user decision 2026-10-03; each slice its own pipeline run):
- 4b — holdings over time (`v_holdings`), valuation, price staleness, security `type`, the fallback for NULL/unsupported security currency, FX span extension to trade/price dates (`internal/store/duckstore/rates.go:26-27`).
- 4c — `v_balances_daily`, `v_net_worth`, `quarry networth`; investment cash joins `transactions`/cash flow (PRD "Cash side also appears in transactions"); investment accounts valued in `accounts`; retire the Phase-4 view pin in `cmd/quarry/run_skill_references_test.go`.
- 4d — account classification config (registered vs non-registered; account numbers for redaction); unclassified-account finding.
- 4e — `quarry acb` (CAD, pooled non-registered, trade-date FX), superficial-loss / no-purchase findings, manual adjustments file; meaning of zero-unit `add_shares`/`remove_shares` for cost base.
- 4f — MCP `net_worth`/`acb` tools, skill `net-worth.md`/`investments.md`.
- 4g — monthly scheduled summary: a skill recipe plus scheduled task over existing commands, no new Go.
- `ZSECURITYSPLIT` (price-history split table — not reflected in transaction units), `ZFIPOSITION`/`ZFISTATEMENT`, `ZLOTASSIGNMENT`, a stored positions or lots table, investment findings, tax lines.

**User decisions (2026-10-03)**:
1. Phase 4 sliced 4a–4g as above; only 4a specced now.
2. A share-count mismatch **fails the sync** like a cash balance mismatch: store not swapped, exit 1.
3. Monthly summary = skill recipe (4g), no Go.
4. A read-only counts-only probe of the real file was run (results under *Triage Brief*).
5. Scenarios approved 2026-10-03.

## Business Rules & Invariants

- **I4-1 Tables.** Three new tables, visible through `quarry sql`, MCP `query`/`describe_schema` and the generated `plugin/skills/quarry/references/schema.md`:
  - `securities`: `id` (`sec-<Z_PK>`), `source_id`, `name`, `ticker` (NULL when NULL or empty), `currency` (`'CAD'`, `'USD'` or NULL, as Quicken records `ZSECURITY.ZCURRENCY`; no refusal for other values in 4a — nothing converts with it yet).
  - `prices`: `security_id`, `source_id`, `date` (`ZQUOTEDATE` as a UTC day, as P1-5c), `price` DECIMAL(18,6). One row per security-day; a duplicate (security, day) keeps the highest `Z_PK`, silently (real file: 0 duplicates). NULL `ZCLOSINGPRICE` → not stored, not counted, silent. Zero → stored as recorded. A price with more than 6 decimals is **rounded half-even to 6, silently** (real file: 17 quotes over 12 securities; deviation ≤ 5e-7).
  - `investment_transactions`: `id` (`itxn-<Z_PK>`), `source_id`, `account_id`, `security_id` (via `ZPOSITION`→`ZSECURITY`; NULL for cash-only rows), `date` (`ZPOSTEDDATE`, else `ZENTEREDDATE`, per P1-5c), `action`, `shares` DECIMAL(18,6) (Quicken's sign: negative when shares leave; 0 when `ZUNITS` is 0; NULL when NULL), `amount` DECIMAL(18,2) native currency (negative when cash leaves the account), `commission` DECIMAL(18,2) (NULL when NULL, stored 0, or snapping to 0.00 under P1-7), `currency` (the account's), `memo` (`ZNOTE`), `split_new_shares`, `split_old_shares` (= `ZNUMERATOR`, `ZDENOMINATOR`; non-NULL only on `action = 'split'`; "split_new_shares new shares for every split_old_shares old"). No `price` column (derived; one owner later).
- **I4-2 Action vocabulary** — closed set, chosen from `ZTYPE` alone (never from category; `ZACTION` is NULL on every real row). Code numbers are never exposed.

  | `ZTYPE` | `action` | | `ZTYPE` | `action` |
  | --- | --- | --- | --- | --- |
  | 2 | `add_shares` | | 11 | `interest` |
  | 3 | `buy` | | 12 | `misc_income` |
  | 6 | `margin_interest` | | 15 | `reinvest_dividend` |
  | 7 | `misc_expense` | | 17 | `remove_shares` |
  | 8 | `capital_gain_long` | | 19 | `sell` |
  | 9 | `capital_gain_short` | | 23 | `split` |
  | 10 | `dividend` | | | |

  Any other code (including hardkoded's 5 and 21, absent from the real file) refuses the import (S4). No `other` bucket.
- **I4-3 Share precision.** Shares are DECIMAL(18,6). REAL residue snaps to 6 decimals under a tolerance the architect sets (≤ 1e-9), computed exactly on the decimal text as P1-7 does for money; any other value beyond 6 decimals refuses (S4). Real file: max 6 decimals, 0 residue. Amount and commission follow P1-7 unchanged.
- **I4-4 Investment cash stays separate.** Investment transactions and their `ZCASHFLOWTRANSACTIONENTRY` children never enter `transactions`, `splits`, `transfers`, `v_spending` or `v_cash_flow` in 4a. Spend and cashflow output is byte-identical before and after 4a on the same snapshot.
- **I4-5 Share-count gate (V1 member, single owner).** quarry's share count per holding is computed **from the newly built store** by one function or view that 4b reuses: walk the holding's `investment_transactions` in date order (ties by `source_id`), summing `shares`, and at a `split` row multiply the running count by `split_new_shares / split_old_shares`. It is compared with Σ non-deleted `ZLOT.ZLATESTUNITS` over the holding's non-deleted `ZPOSITION`s (Quicken's reference, read in the importer only — no lots/positions table in the store). A holding is checked when it has ≥ 1 imported investment transaction with a security or ≥ 1 non-deleted lot. It matches when |quarry − Quicken| ≤ 0.000001 shares (named constant, not config). All dates count; closed and inactive accounts are checked; brokerage and retirement alike. A mismatch fails the sync (store not swapped, exit 1). `Validation.Failed()` includes it.
- **I4-6 Deletion.** Deleted transactions, lots, positions and securities are excluded silently (P1-5d); a transaction in a skipped or deleted account is skipped silently.
- **I4-7 not_imported retired.** `store.NotImported`, the Rows clause `; N investment transactions not imported`, `--json` `store.not_imported` (sync) and top-level `not_imported` (status and MCP `sync_status`) are deleted. `import_runs.investment_transactions_not_imported` is not in the v6 schema; history carry treats it as read-and-discard (moved out of `requiredRunColumns`, `internal/store/duckstore/history.go:34-38`) so a sync over a v5 store's history, and the sync after that, succeed.
- **I4-8 import_runs.** New columns `securities_rows`, `prices_rows`, `investment_transactions_rows`, `shares_checked`. No `shares_mismatched` (a failed build is never stored).
- **I4-9 Format.** `FormatVersion` 5 → 6. An older store gets the existing other-format refusal; re-sync rebuilds.
- **I4-10 Reference check.** `quarry sync --from 20260930T072052Z` on the user's real snapshot imports 1,605 investment transactions, 84 securities, 99,352 prices, and 145 holdings match, exit 0 (145 = distinct account×security with ≥1 non-deleted investment transaction; the 80 with lots are among them; 173 is the position count).

---

## Triage Brief

**Exists today (do not re-plan):** `store.IsInvestmentAccount` (`internal/store/store.go:52`) and `accountTypeMap` (`internal/importer/accounts.go:20-23`); fx_rates + ASOF; `Source` port (`internal/importer/ports.go:11`), `Store.Replace` port (`ports.go:24`); format-version mechanism (`internal/store/duckstore/duckstore.go:25,235-260`); `v9fixture` builders for accounts/transactions/entries (`internal/quicken/v9/v9fixture/builder.go`); P1-7 exact money parse (`internal/importer/money.go`); offender/refusal classes (`internal/importer/offenders.go`, `reasons.go`); validation (`internal/importer/validate.go:105-135` balances skip investment accounts; `store.Validation` Balances/Splits/Transfers + `Failed()`); generated `schema.md` (`cmd/quarry/run_skill_schema_reference_test.go:25-28`, `-update`).

**Importer today:** InvestmentTransaction only counted — `surveyTransactions` (`internal/importer/transactions.go:51-74`, sole caller `importer.go:85`), `entities.go:13-27` (`investmentEntity` hard-wired; Security/Position/Lot Z_ENT not resolved), count → `store.NotImported` (`importer.go:126,133,154`). Twin for new mappers: `mapTransactions` (`transactions.go:85-197`). Validation twin: `validate.go:105-135`.

**Store today:** no securities/prices/investment tables (`internal/store/duckstore/schema.go:12-132`); `v_account_balances` NULL for investment types (`schema.go:136-160`, unchanged in 4a); import_runs columns mirrored in `schema.go:79-108`, `history.go:37-42,317-339`, `status.go:22-67`.

**Caller table** (`.claude/briefs/navigation.md`):

| Symbol / string | Callers | Via |
| --- | --- | --- |
| `store.NotImported` | `store/store.go:239,317`; `cli/render.go:266`; `cli/render_status.go:27`; `importer/importer.go:126,149`; tests `cli/render_internal_test.go:182,204,209,286,618`, `importer/not_imported_test.go` | LSP |
| `duckstore.FormatVersion` | `duckstore.go:25,252`; `rates.go:47`; tests `cmd/quarry/run_status_json_test.go:112`, `run_store_info_test.go:73`, `duckstore_test.go:182`, `open_test.go:180,181,186,338`, `status_test.go:36` | LSP |
| `store.Rows` | production `importer.go:117`, `ports.go:25`, `validate.go:14,33,106,154`, `duckstore.go:293,416,431`, `rates.go:26` (+134 test refs) | LSP |
| `investment_transactions_not_imported` | `schema.go:98`, `history.go:37`, `status.go:22`, `duckstore.go:589`, `references/schema.md:109` | grep |
| `document.NotImported` / `not_imported` | `report/document/common.go:35-37`, `report/document/status.go:21,142` → `internal/mcp/sync_status.go:25`; tests `cmd/quarry/run_shared_documents_test.go`, `run_import_runs_test.go`, `run_json_test.go`, `run_status_json_test.go` | grep |
| `InvestmentAccounts` / `investment_accounts` | `store.go:181,354`; `cli/render.go:95,203-221,442`; `cli/json.go:57,208`; `report/document/status.go:53,130`; `cli/render_status.go:28`; `schema.go:102`; `history.go:42,317-339` (stay — balance-check count) | grep |
| "not imported" copy | `cli/render_accounts.go:13`, `cli/render.go:276`, `cli/accounts.go:17-24`, `cli/json_accounts.go:17` | grep |
| `surveyTransactions` / `investmentEntity` | `importer.go:84-85`, `transactions.go:53-74`, `entities.go:16,21,27` (dead after 4a) | grep |

**Real-file probe** (read-only, counts only, snapshot `20260930T072052Z`):
- Z_ENT: FIPosition 33, FIStatement 34, Lot 44, LotAssignment 45, LotMod 46, Position 49, Security 66, SecurityQuote 68, SecuritySplit 70, CashFlowTransaction 79, InvestmentTransaction 81.
- 84 securities (all quoted; currency CAD 6 / USD 67 / NULL 11; 0 without name or ticker; 0 deleted). 99,352 quotes (integer 3,634 / real 95,718; 0 NULL; 25 zero; 0 duplicate days; 17 beyond 6 decimals). 33 ZSECURITYSPLIT rows over 16 securities. 173 positions (73 securities; 0 deleted; 28 with no transactions). 444 lots over 80 positions (max 3 decimals). 838 lotmods; 0 lot assignments; 77 FIPositions.
- 1,605 investment txns: 0 deleted; ZACTION NULL on all; ZTYPE per I4-2 (counts: 2→240, 3→272, 6→34, 7→2, 8→7, 9→5, 10→925, 11→1, 12→3, 15→5, 17→39, 19→71, 23→1); every one has entries and entry sum == ZAMOUNT (1,605/1,605); 0 entries carry ZTRANSFER and 0 cash entries link into them; ZPOSTEDDATE on 1,209, ZENTEREDDATE on all; ZUNITS integer 1,195 / real 410, max 6 decimals, 0 residue; ZNUMERATOR/ZDENOMINATOR non-NULL on all (1/1 on 1,359); zero-unit `add_shares` 73, `remove_shares` 12. Accounts: BROKERAGENORMAL 4, BROKERAGEOTHER 1, DEFERREDCOMPRETIREMENT401K 2, RETIREMENTIRA 2.
- Gate: signed Σ ZUNITS == Σ ZLOT.ZLATESTUNITS for 172/173 positions; the 173rd holds a `split` (1:12, new = old × NUM/DEN, reproduced by ZLOTMOD before/after) and closes to 0 lot units once the ratio is applied. Positions held across a ZSECURITYSPLIT date reconcile by plain sum (ZSECURITYSPLIT not applied).
- Store today: 29 one-sided transfers, 0 into brokerage/retirement.

## Product Verdict

**SHIP WITH CHANGES** (product-vision, 2026-10-03, two passes: Phase 1 + scoped C1–C3 ruling). All changes folded into the rules above and *Surface & Copy* below:
1. Preconditions C1 (action codes from evidence), C2 (precision), C3 (one-sided transfers into investment accounts) — met by the probe; C3 found nothing.
2. One owner for the gate rule, computed from the built store (I4-5).
3. Delete `not_imported` everywhere, history carry tolerant (I4-7); re-pin the four `cmd/quarry` tests in the same scenario that removes the field.
4. Share-specific stderr tail (S.3).
5. Accounts, skill and conventions copy (S.5).
6. Table named `investment_transactions` (not the PRD's `investment_txns`).
7. Prices rounded half-even to 6, no refusal; shares keep snap-or-refuse.
8. `securities.currency` enters 4a as recorded.
9. Scoped copy ruling (2026-10-03, after sizing): NULL action code, non-number/too-large shares/commission/amount/price, NULL quote date, undated transaction, deleted security under shares, positions/lots in skipped accounts, optional entities, zero commission → S.5/S.6. Security named by name in every refusal. Checked holdings on the real file = 145.

## Surface & Copy

### S.1 Sync success (stdout, Rows extended; Shares line after Splits, before Transfers)

```
Rows      18,204 transactions, 21,977 splits, 3,112 transfers, 1,873 payees, 312 categories, 14 tags; 1,605 investment transactions, 84 securities, 99,352 prices
Shares    145 holdings match Quicken's share counts
```
- The investment clause is always present. Each noun inflects on its own count (`1 investment transaction`, `1 security`, `1 price`; `0 …` plural); counts comma-grouped.
- Shares variants: `1 holding matches Quicken's share count`; `no holdings to check`.
- `quarry status` prints the same Shares line, from `shares_checked`.

### S.2 Gate failure (stdout, both modes; `--json` `"built": false`)

```
Shares    DIFFER for 1 of 145 holdings
  ! RRSP (CAD, closed)  iShares Core Equity ETF (XEQT)  quarry 120.5  Quicken 110.5  difference 10
```
- Rows: two spaces then `!`. Columns padded to the widest value in the block. Share figures right-aligned, comma-grouped, trailing fractional zeros trimmed (`0.000001`, `1,200`).
- Account label `Name (CUR[, closed][, inactive])` as in Balances. Security label: name, plus ` (TICKER)` when ticker non-NULL and differs from name. Both through `escapeCell`.
- `difference` = quarry − Quicken. Sort: account name, account source_id, security name, security source_id.
- When Shares passes but another check fails, it prints its pass line.

### S.3 Gate failure (stderr, exit 1)

- Shares alone: `quarry: validation failed: 1 of 145 holdings does not match Quicken's share count; ~/Library/Application Support/quarry/quarry.duckdb was not changed; each difference is listed on stdout; quarry read the holding's transactions differently from Quicken, so run quarry sync --from 20260930T072052Z after updating quarry`
  - Plural: `N of M holdings do not match Quicken's share counts`.
  - Plural tail (X mismatched > 1; ruled 2026-10-04, SCENARIO-06): `…; each difference is listed on stdout; quarry read those holdings' transactions differently from Quicken, so run quarry sync --from <id> after updating quarry`. X = 1 keeps "the holding's" (also for `1 of 3 holdings does not match`).
- Shares together with Balances and/or Splits: clauses join with ` and ` in order balances, splits, shares, then the **existing** V1 tail.
- First run (no previous store): store clause uses V1's existing first-run form.

### S.4 `--json`

- `store.rows` (sync and status) gains `investment_transactions`, `securities`, `prices`.
- Sync `store.shares` = `{"checked":145,"mismatched":[…]}`; `mismatched` always present.
- Mismatch entry: `{"account_id","account","currency","closed","active","security_id","security","ticker","quarry":"120.500000","quicken":"110.500000","difference":"10.000000"}` — shares strings with exactly 6 decimals; `ticker` null when none; text raw.
- Status (and MCP `sync_status`): `"shares":{"checked":145}`.
- Removed: sync `store.not_imported`, status/MCP top-level `not_imported`.

### S.5 S4 reasons (frame unchanged: `quarry: cannot import snapshot <id>: <reason>; <store> was not changed; run quarry sync --from <id> once quarry supports it`, exit 1)

| Input | Reason |
| --- | --- |
| Unmapped `ZTYPE` | `an investment transaction on 2024-03-02 in "RRSP" has action code 14, which quarry does not map yet` |
| Shares beyond 6 decimals (not residue) | `an investment transaction on 2024-03-02 in "RRSP" has 1.23456789 shares, which has more than 6 decimal places` |
| Split ratio with a zero or NULL side | `a stock split on 2019-05-01 in "RRSP" of "iShares Core Equity ETF" has a ratio quarry cannot read (1:0)` — each side prints as Quicken's raw column text (NULL → `none`, blob → `blob`), e.g. `(1:none)`, `(n/a:12)`, `(blob:12)`; NULL, zero, negative (e.g. `(-1:12)`, orchestrator rule 2026-10-04, no new copy), non-number, beyond-6-decimals and too-large sides all use this one line; the security is named by its name in double quotes when one resolves, else the clause is dropped: `a stock split on <date> in "<account>" has a ratio quarry cannot read (1:0)` (ruled 2026-10-04, SCENARIO-03) |
| NULL `ZTYPE` | `an investment transaction on 2024-03-02 in "RRSP" has no action code` |
| Shares not a number (text/blob) | `an investment transaction on <date> in "<account>" has a share count that is not a number` |
| Shares out of DECIMAL(18,6) range | `an investment transaction on <date> in "<account>" has <v> shares, which is too large for quarry's share counts` |
| Commission not a number / too large / > 2 decimals after P1-7 snap | `… has a commission that is not a number` / `… has a commission of <v>, which is too large for quarry's amounts` / `… has a commission of <v>, which has more than 2 decimal places` (prefix `an investment transaction on <date> in "<account>"`) |
| Lot `ZLATESTUNITS` NULL / not a number / > 6 decimals / too large, on a counting lot (not deleted, non-deleted position, imported account) (ruled 2026-10-04, SCENARIO-04) | `a lot of "<security name>" in "<account>" has no share count` / `… has a share count that is not a number` / `… has <v> shares, which has more than 6 decimal places` / `… has <v> shares, which is too large for quarry's share counts` (NULL never counts as 0) |
| Lot entity missing while investment transactions import (ruled 2026-10-04, SCENARIO-04) | `the snapshot has investment transactions but no Quicken lots to check their share counts against` |
| NULL `ZAMOUNT` (ruled 2026-10-03, SCENARIO-02) | `an investment transaction on <date> in "<account>" has no amount` (refuse; share-only actions store 0, not NULL) |
| Amount, same three cases | as commission with "an amount" in place of "a commission" (existing cash copy with "investment " added) |
| Price not a number / too large | `a price of "<security name>" on <date> is not a number` / `a price of "<security name>" on <date> is <v>, which is too large for quarry's prices` (more than 6 decimals is rounded, never refused) |
| Neither posted nor entered date | `an investment transaction in "<account>" (source id <N>) has no date` |
| Non-zero shares on a position whose security is deleted or missing | existing `… has shares but no security` (zero/NULL shares → imports with NULL security, like cash-only) |
| Non-zero units, no position | `an investment transaction on 2024-03-02 in "RRSP" has shares but no security` |
| Security with no name | `a security (source id 12) has no name` — NULL or empty ZNAME (whitespace follows the account rule); a deleted security and its quotes are ignored; a nameless security's quotes are skipped so no `a price of ""` line appears |

### S.6 Edge-case rows

| Input | Outcome |
| --- | --- |
| Holding with transactions, no lots | Checked; Quicken = 0; non-zero quarry count → `!` row, exit 1 |
| Holding with lots, no transactions | Checked; quarry = 0; non-zero lots → `!` row, exit 1 (real file: none) |
| Zero transactions, zero lots | Not checked; no row; not counted |
| Unmapped or NULL action code | S4, exit 1 |
| Quote with NULL date | Not stored, not counted, silent |
| Positions and lots in skipped or deleted accounts | Excluded silently (P1-5d); no gate row |
| Security, SecurityQuote, Position or Lot entity missing from Z_PRIMARYKEY | Optional when no investment transactions import: no investment data — `Shares    no holdings to check`, zero Rows counts. Lot missing while investment transactions import → S4 (S.5). Missing security/position under transactions → their existing reasons. `no holdings to check` only when there are no imported investment transactions and no counting lots. Schema drift still caught by M1/M1b |
| Commission NULL, stored 0, or snapping to 0.00 | Stored NULL |
| Share residue within tolerance | Snapped silently; no field |
| Shares beyond scale | S4, exit 1 |
| Split with NULL/zero numerator or denominator | S4, exit 1 |
| Cash-only transaction (no position, NULL or zero units) | Imported, `security_id` NULL; outside the gate; silent |
| Zero-unit `add_shares`/`remove_shares` | Imported, `shares` 0; adds 0 to the gate; silent |
| Units non-zero, no position | S4 |
| Deleted transaction, lot, position or security | Excluded silently |
| Transaction in a skipped or deleted account | Skipped silently |
| Mismatch in a closed account | Same failure; label `(CAD, closed)` |
| Retirement vs brokerage | Identical |
| Quote with NULL price | Not stored, not counted, silent |
| Quote with zero price | Stored |
| Price beyond 6 decimals | Rounded half-even, silent |
| Duplicate quote day | Highest `Z_PK` kept, silent |
| Security never held | Stored with its prices; no gate row |
| No investment data in file | `Shares    no holdings to check`; Rows clause zeros |
| CAD vs USD | Shares currency-free; `amount` native; no conversion |
| Store older than v6 | Existing format refusal; re-sync rebuilds |
| Schema fingerprint lacks new tables/columns | Existing M1/M1b mismatch |

### S.7 Changes to existing surfaces

| Where | Old | New |
| --- | --- | --- |
| `internal/cli/accounts.go:17-18` (Long) | investment accounts "not imported" sentence | `Brokerage and retirement accounts show "not valued": quarry imports their transactions and checks their share counts against Quicken, but does not value holdings yet, so it cannot compute their balance.` |
| `internal/cli/render_accounts.go:13` cell; `internal/cli/json_accounts.go:17` doc comment | `not imported` | `not valued` |
| `internal/cli/render.go:265-276`, `render_status.go:27` | `; N investment transactions not imported` | per S.1 |
| `internal/cli/sync.go:46-50` (Long) | "…sum of its splits; if a check fails…" | "…sum of its splits, and in every brokerage and retirement account each security's share count must equal Quicken's; if a check fails, the previous store is left unchanged." |
| `plugin/skills/quarry/SKILL.md` frontmatter `description` | "for net worth or investment questions beyond saying quarry does not cover them yet" | "for net worth, holdings, gains, dividend totals or ACB beyond saying quarry does not cover them yet" |
| `SKILL.md:72` | Net worth not-covered line | `- **Net worth:** "quarry does not compute net worth yet: it does not value investment holdings, so a total of the balances it has would leave them out." \`quarry accounts\` can list the other accounts' balances; don't add them up.` |
| `SKILL.md:73` | Investments not-covered line | `- **Investments, holdings, dividends, realized gains, ACB:** "quarry imports investment transactions but does not compute holdings, dividends, gains or ACB yet."` |
| `internal/report/sql_conventions.go` (single source for `describe_schema` and `schema.md`; regenerate `schema.md`) | — | add: `Investment transactions are in investment_transactions, not in transactions, v_cash_flow or v_spending, so dividends, interest and trades are not counted as income or spending there. Their amount is DECIMAL(18,2) in the account's own currency, negative when cash leaves the account; shares is DECIMAL(18,6) as Quicken recorded each transaction, negative when shares leave. A split row carries split_new_shares and split_old_shares instead, so a sum of shares is not a holding. prices holds each security's closing price per day as Quicken recorded it, rounded to 6 decimals, in the security's currency (securities.currency, NULL when Quicken records none); quarry does not convert prices yet.` |
| `docs/initial-prd.md` L123 (`securities, prices` row) | — | add "type from Phase 4b; currency as recorded, NULL when Quicken has none" |
| `docs/initial-prd.md` L125 | `investment_txns`; "Cash side also appears in `transactions`" | `investment_transactions`; "Cash side also appears in `transactions` (from Phase 4c)" |
| `docs/specifications/phase1-import-store/specification.md:216-228` | `not_imported` object | one-line note: superseded by phase4a-investments I4-7 |
| No new MCP tool; `cmd/quarry/run_skill_references_test.go` Phase-4 view pin | — | unchanged (4b/4c) |

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — Sync imports securities and their prices
  Given a Quicken file with securities carrying a name, a ticker or none, and currency CAD, USD or none, and daily quotes including a NULL price, a zero price and a price with more than 6 decimals
  When the user runs quarry sync
  Then securities holds each security with its ticker (NULL when empty) and currency as recorded, and prices holds one row per security-day with the zero kept, the NULL skipped and the long decimal rounded half-even to 6

Scenario: SCENARIO-02 — Sync imports investment transactions with named actions
  Given investment transactions for each of the 13 mapped action codes, a cash-only dividend with no position, zero-unit add and remove rows, and a 1:12 split
  When the user runs quarry sync
  Then investment_transactions holds each with its action name, shares in Quicken's sign, amount and commission as DECIMAL(18,2), security_id NULL for the cash-only row, and split_new_shares/split_old_shares only on the split, while transactions, v_spending and v_cash_flow are unchanged

Scenario Outline: SCENARIO-03 — Sync refuses an investment record quarry cannot read
  Given a Quicken file with <record>
  When the user runs quarry sync
  Then stderr is the S4 line with "<reason>", the store is not changed, and the exit code is 1

  Examples:
    | record                                   | reason                                                                                      |
    | an action code 14                        | has action code 14, which quarry does not map yet                                           |
    | 1.23456789 shares                        | has 1.23456789 shares, which has more than 6 decimal places                                 |
    | a split ratio 1:0                        | has a ratio quarry cannot read (1:0)                                                        |
    | a split ratio with a NULL denominator    | has a ratio quarry cannot read (1:none)                                                     |
    | non-zero units and no position           | has shares but no security                                                                  |
    | a security with no name                  | has no name                                                                                 |

Scenario: SCENARIO-04 — Sync reports holdings that match Quicken's share counts
  Given every holding's derived share count equals Quicken's lot units
  When the user runs quarry sync
  Then stdout shows the Rows investment clause and "Shares    N holdings match Quicken's share counts", --json store.shares is {checked: N, mismatched: []} and store.rows has investment_transactions, securities and prices, and the exit code is 0

Scenario: SCENARIO-05 — Share count applies a stock split in date order
  Given a holding bought before a 1:12 reverse split and sold after it, with Quicken's lots at 0 units
  When the user runs quarry sync
  Then the holding matches Quicken and the sync succeeds

Scenario: SCENARIO-06 — Sync fails when holdings' share counts differ from Quicken
  Given a holding in a closed account whose derived count differs from its lot units, and a holding with transactions but no lots
  When the user runs quarry sync
  Then stdout shows "Shares    DIFFER for 2 of N holdings" with one "!" row each, stderr is the shares validation line, the store is not changed, the exit code is 1, and --json store.shares.mismatched carries the ruled fields

Scenario: SCENARIO-07 — A share failure joins a balance failure in one line
  Given a cash account whose balance mismatches and a holding whose share count mismatches
  When the user runs quarry sync
  Then stderr joins the balances and shares clauses with " and " followed by the existing tail, and the exit code is 1

Scenario: SCENARIO-08 — A file with no investment data
  Given a Quicken file with no securities and no investment transactions
  When the user runs quarry sync
  Then Rows shows "0 investment transactions, 0 securities, 0 prices", Shares shows "no holdings to check", and the exit code is 0

Scenario: SCENARIO-09 — Status reports the share check without "not imported"
  Given a store built by this version
  When the user runs quarry status
  Then the Shares line appears, --json and MCP sync_status carry "shares": {"checked": N} and the new store.rows counts, and not_imported appears on no surface

Scenario: SCENARIO-10 — Sync over a pre-4a store carries import history forward
  Given a version-5 store whose import_runs rows have investment_transactions_not_imported
  When the user runs quarry sync twice
  Then both syncs build a version-6 store, the earlier import_runs rows are carried with that column discarded, and both exit 0

Scenario: SCENARIO-11 — Existing surfaces stop saying investments are not imported
  Given a store built by this version holding brokerage and retirement accounts
  When the user runs quarry accounts
  Then their balance cell reads "not valued", and the accounts and sync help, SKILL.md not-covered lines, SQL conventions and generated schema.md carry the ruled copy

Scenario: SCENARIO-12 — Reference check against the real Quicken file
  Given the user's snapshot 20260930T072052Z
  When the user runs quarry sync --from 20260930T072052Z
  Then 1,605 investment transactions, 84 securities and 99,352 prices import, 145 holdings match Quicken's share counts, and the exit code is 0
```

---

## Sizing

Architect sizing pass 2026-10-03 (one pass over all 12; folds change no scenario's assertions, every folded scenario keeps its own acceptance test).

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN (opus) — 4 batches, importer; owns FormatVersion 5→6 and re-pins `cmd/quarry/run_shared_documents_test.go:132` (seam A); `securities_rows`/`prices_rows` across schema.go/history.go/status.go |
| SCENARIO-02 | OWNS A RUN (opus) — 4 batches, importer; v9fixture Position + investment fields on TransactionRow (existing InvestmentTransaction fixture sites get a mapped ZTYPE); owns the unmapped-code and beyond-6-shares refusals |
| SCENARIO-03 | LIGHT — 3 remaining refusals (split ratio, shares without security, security without name); the 2 rows S02 delivered are green on arrival |
| SCENARIO-04 | OWNS A RUN (opus), absorbs SCENARIO-05 and SCENARIO-08 — 5 batches (ceiling), test-first (write-safety guard); owns the I4-5 share-count function (unexported duckstore query, not a view — seam C), Validation member, `shares_checked`, Rows clause, Shares variants, `--json` store.rows keys + `store.shares`. Over 5 batches → un-fold SCENARIO-05 as LIGHT after it |
| SCENARIO-05 | FOLD into SCENARIO-04 — split multiply lives inside the I4-5 function |
| SCENARIO-06 | OWNS A RUN (opus), absorbs SCENARIO-07 — 3–4 batches; DIFFER block, stderr shares clause, `--json` mismatched; batch 1 = cash-failure protocol if SCENARIO-04 did not settle it (seam C) |
| SCENARIO-07 | FOLD into SCENARIO-06 — same stderr clause builder; conditional on seam C |
| SCENARIO-08 | FOLD into SCENARIO-04 — the "no holdings to check" variant and zero Rows clause are S04's renderer |
| SCENARIO-09 | OWNS A RUN (opus), absorbs SCENARIO-10 — 3 batches, code-first; deletes `store.NotImported`, `document.NotImported`, `surveyTransactions`, `investmentEntity`, `not_imported_test.go`; re-pins the four `cmd/quarry` tests (Product Verdict 3) |
| SCENARIO-10 | FOLD into SCENARIO-09 — dropping the column and tolerant carry land with deleting the field |
| SCENARIO-11 | OWNS A RUN (sonnet) — 3 batches, cli + plugin + report; S.7 copy |
| SCENARIO-12 | No architect/developer — orchestrator manual reference run (see `## Reference check`) |

**Seams.** A: FormatVersion bump in S01; every schema-changing scenario (S01, S02, S04, S09) regenerates schema.md with `-update`. B: each `*_rows` column lands with its table. C: S04 owns the gate function and port shape; it runs after `loadRows`, before findings/rates/`finishBuild`; S04's Handoff states how a cash failure with shares still computed works — (i) separate port method on a scratch build keeping P1-2 "Import never calls Replace when a check fails", or (ii) Replace runs and never swaps (rewrites P1-2's test, test-first). D: S04 Mutation checks need a direct mismatch test (no rename, previous store byte-identical). E: S04 adds store.rows keys (shared `document.NewRows` → sync, status, MCP) and re-pins status/MCP goldens; `store.not_imported` keeps rendering until S09. F: `surveyTransactions` stays until S09.

## BDD Acceptance Progress

- [x] SCENARIO-01: Sync imports securities and their prices — `cmd/quarry/run_investments_test.go` `Test_run_sync_imports_securities_and_their_prices`
- [x] SCENARIO-02: Sync imports investment transactions with named actions — `cmd/quarry/run_investments_test.go` `Test_run_sync_imports_investment_transactions_with_named_actions`
- [x] SCENARIO-03: Sync refuses an investment record quarry cannot read — `cmd/quarry/run_investments_test.go` `Test_run_sync_refuses_an_investment_record_quarry_cannot_read`
- [x] SCENARIO-04: Sync reports holdings that match Quicken's share counts — `cmd/quarry/run_investments_test.go` `Test_run_sync_reports_holdings_that_match_quickens_share_counts`
- [x] SCENARIO-05: Share count applies a stock split in date order — delivered by SCENARIO-04 — `cmd/quarry/run_investments_test.go` `Test_run_sync_applies_a_stock_split_in_date_order`
- [x] SCENARIO-08: A file with no investment data — delivered by SCENARIO-04 — `cmd/quarry/run_investments_test.go` `Test_run_sync_reports_a_file_with_no_investment_data`
- [x] SCENARIO-06: Sync fails when holdings' share counts differ from Quicken — `cmd/quarry/run_share_gate_test.go` `Test_run_sync_fails_when_holdings_share_counts_differ_from_quicken`
- [x] SCENARIO-07: A share failure joins a balance failure in one line — delivered by SCENARIO-06 — `cmd/quarry/run_share_gate_test.go` `Test_run_sync_joins_a_share_failure_to_a_balance_failure_in_one_line`
- [ ] SCENARIO-09: Status reports the share check without "not imported"
- [ ] SCENARIO-10: Sync over a pre-4a store carries import history forward
- [ ] SCENARIO-11: Existing surfaces stop saying investments are not imported

## Reference check

SCENARIO-12 (phase2f precedent): run by the orchestrator after SCENARIO-11, before the gate, with a scratch HOME holding a copy of snapshot `20260930T072052Z` (the real path is sandbox read-denied; user grants access). Command, date and the observed Rows/Shares lines are recorded in `REFERENCE-CHECK.md`. A rule gap it finds becomes a new scenario appended above.

- [ ] SCENARIO-12: Reference check against the real Quicken file
