# How to query the store with SQL

`quarry sql` runs a read-only query, in DuckDB's SQL dialect, against quarry's store. Use it for a question no command answers. When a command does answer it, use the command: it carries the rules in [How quarry counts money](../concepts/counting-rules.md).

## 1. Read the schema

The store's tables, views and conventions are listed in the [store schema](https://github.com/koblas/quarry/blob/main/plugin/skills/quarry/references/schema.md), which is generated from the store. For the views alone:

```bash
quarry sql "SELECT table_name FROM information_schema.tables WHERE table_type = 'VIEW' ORDER BY 1"
```

## 2. Query the views for spending and income

`v_spending` and `v_cash_flow` already leave out transfers between your own accounts, Quicken's system categories, transactions excluded from reports and accounts Quicken leaves out of reports, so their totals match `quarry spend` and `quarry cashflow`. Do not rebuild them from `transactions` and `splits`.

```bash
quarry sql "SELECT category, sum(spent_cad) AS spent FROM v_spending WHERE date >= DATE '2026-01-01' GROUP BY 1 ORDER BY 2 DESC"
```

Each row of `v_spending` carries its amount three ways: `spent` in the account's own currency, and `spent_cad` and `spent_usd` converted at the Bank of Canada rate for its date. Sum one of the converted columns; never add `spent` across accounts in different currencies. The converted columns are NULL for a date before the first stored rate.

## 3. Pass a longer query on stdin

`-` reads the query from stdin. A quoted heredoc keeps the shell from changing it:

```bash
quarry sql - <<'SQL'
SELECT date_trunc('year', date) AS year, sum(spent_cad) AS spent
FROM v_spending
WHERE category LIKE 'Food:Groceries%'
GROUP BY 1
ORDER BY 1
SQL
```

## 4. Get every row, or a file

quarry prints at most 500 rows and says on stderr when there were more. Aggregate in SQL where a total is what you need. For every row:

```bash
quarry sql --limit 0 "SELECT * FROM v_spending"
```

For a spreadsheet, `--csv` prints every row with a header line:

```bash
quarry sql --csv "SELECT * FROM v_spending WHERE date >= DATE '2026-01-01'" > spending-2026.csv
```

`--json` prints the result as JSON.

## What SQL cannot answer

- **Recurring charges and unusually large charges** have no SQL form. Use `quarry recurring` and `quarry anomalies`.
- **Adjusted cost base and capital gains** are in no table or view. Use `quarry acb`.
- **Holdings over time** come from `v_holdings`, filtered by date. A sum of `investment_transactions.shares` is not a holding, because a stock split is a row of its own.

The store cannot be changed through `quarry sql`.
