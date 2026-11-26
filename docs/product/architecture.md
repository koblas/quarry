# quarry — Architecture and data model

Part of the quarry product spec; index in [README.md](README.md).

## Architecture

A one-way pipeline copies the Quicken file, maps it into a normalized DuckDB store, and serves that store through one library through a CLI and an MCP server, with a skill that teaches Claude to use them.

```
Quicken for Mac  ──►  Snapshot            ──►  Importer             ──►  quarry.duckdb
live .quicken file    SQLite backup API        maps v9 schema            normalized schema
never written         while Quicken open       seeded from dweekly       stable, documented
                                                                              │
                                                                              ▼
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ Core library — transfer-aware queries: spending, recurring charges, anomalies,       │
│ cash flow, net worth, investments                                                    │
└──────────────────────────────────────────────────────────────────────────────────────┘
        │                                                          │
        ▼                                                          ▼
  quarry CLI  ◄── Claude skill                              MCP server (stdio)
  sync, spend, findings, sql    SKILL.md + references/      schema, query, analysis tools
  David, scheduled jobs         drives the CLI; Claude Code Claude desktop and chat
```

One normalized store feeds every interface; Quicken is only ever copied.

The normalized store is the product: Quicken's quirks are resolved once in the importer, so the CLI and MCP server stay simple. Everything is Go, shipped as one binary: the duckdb-go driver (cgo, DuckDB linked in) and the official MCP Go SDK.

- **Snapshot** isolates `quarry` from Quicken: the live database is copied with SQLite's online backup API while Quicken is open (it is encrypted when Quicken is closed), then checksummed and kept, so every import is reproducible.
- **Importer** is the only code that knows Quicken's schema. Each run rebuilds the store from a snapshot, so imports are idempotent and a Quicken schema change breaks one module.
- **Core library** owns every analysis rule (transfer handling, split allocation, sign conventions) so the CLI and MCP never disagree.

## Data model

The store has a small set of normalized tables plus derived views; every analysis reads the views, never Quicken's raw structure.

| Table | Holds | Key rules |
| --- | --- | --- |
| `accounts` | Name, type (chequing, savings, credit card, investment, retirement, loan, asset), currency (CAD or USD), institution, open/closed, hidden | Closed accounts included; type drives net-worth sign and grouping |
| `categories` | Hierarchy (`parent_id`, `full_path`), kind (income / expense / system), tax line | Full history kept, including retired categories |
| `payees` | Payee names as recorded in Quicken | Variants of one merchant are flagged for cleanup, not silently merged |
| `transactions` | Account, date, payee, memo, amount, currency, cleared/reconciled status, cheque number | Negative = money leaving the account |
| `splits` | Transaction, category, amount, memo, transfer target account | Every transaction has at least one split; splits sum to the transaction amount |
| `transfers` | Pairs of splits that move money between own accounts, including CAD to USD | Excluded from spending and income by default; cross-currency pairs keep both amounts |
| `tags`, `split_tags` | Quicken tags | Many-to-many on splits |
| `securities`, `prices` | Symbol, name, type, currency; historical prices | Prices as recorded in Quicken, no external feed in v1 (type deferred: Quicken's type codes are unlabelled; currency as recorded, NULL when Quicken has none) |
| `fx_rates` | CAD/USD rate by date | Quicken keeps only the current rate per pair, so history is a daily series (Bank of Canada) fetched by `quarry sync` |
| `investment_transactions` (commission DECIMAL(18,4)) | Action (buy, sell, dividend, reinvest, share transfer, split), security, shares, price, fees, amount | Cash side also appears in `transactions` |
| `findings`, `finding_items` | What each sync found to clean up: id, type, when first found, when fixed; `finding_items` names the transactions, splits, payees or categories | Status (open, fixed, ignored) and the suggested fix come from `quarry findings`; ignore decisions live in the config file |
| `import_runs` | Snapshot hash, row counts, validation results | One row per successful build, kept across rebuilds; audit trail |

**Derived views:** `v_spending` (expense splits, transfers removed, refunds netted), `v_cash_flow` (monthly income vs expense), `v_balances_daily` and `v_net_worth` (per account and total), `v_holdings` (shares and value by day, from `holding_shares`; Phase 4b). Recurring series and anomalies are computed by the core library; `quarry recurring --json` / `anomalies --json` are the shapes the Phase 3 `recurring_charges` / `anomalies` tools return.

**Conventions**

- Money is `DECIMAL(18,2)` in the account's native currency; no floats. Reporting-currency views convert with the rate in effect on the transaction date.
- Every row keeps `source_id` (Quicken's primary key) for traceability back to the file.
- `quarry` IDs are derived deterministically from source IDs, so a re-import keeps IDs stable and a finding can be tracked from open to fixed across snapshots.
- Real fixes happen in Quicken. `quarry` stores only decisions about findings: the ids the user lists under `findings.ignore` in `config.toml`, which quarry reads and never writes, so no second, divergent copy of the truth builds up and rebuilding the store loses no decision.
