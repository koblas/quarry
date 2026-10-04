# Reference check: phase4a-investments (SCENARIO-12)

Method: the binary built from this branch runs `quarry sync --from 20260930T072052Z` with `HOME` set to a scratch directory that holds a copy of the user's snapshot (`.sqlite` + `.json`). The user's real store is never touched. Only the Rows / Shares / stderr lines are recorded here, never holdings.

## Run 1 — 2026-10-04, HEAD after SCENARIO-11

- `sync --from 20260930T072052Z`: exit 1.
- stderr: `quarry: cannot import snapshot 20260930T072052Z: an investment transaction on 2017-01-12 in "Fidelity Investments" has a commission of 8.4998, which has more than 2 decimal places (and 17 more); ~/Library/Application Support/quarry/quarry.duckdb was not changed; run quarry sync --from 20260930T072052Z once quarry supports it`
- Rule gap: 18 `sell` commissions with 3–4 decimals (exact at 4). Ruled by product-vision 2026-10-04 → commission DECIMAL(18,4); appended as SCENARIO-13.

## Run 2 — 2026-10-04, HEAD e48059f (after SCENARIO-13)

- `sync --from 20260930T072052Z`: exit 0, stderr empty.
  ```
  Rows      14,061 transactions, 14,080 splits, 688 transfers, 3,383 payees, 233 categories, 13 tags; 1,605 investment transactions, 84 securities, 99,352 prices
  Balances  6 accounts match Quicken's last reconciled balance; 10 never reconciled and 9 investment accounts not checked
  Splits    all 14,061 transactions equal the sum of their splits
  Shares    145 holdings match Quicken's share counts
  Transfers 659 paired, 29 one-sided
  ```
- `sync --from … --json`: exit 0; `store.rows` investment_transactions 1605, securities 84, prices 99352; `store.shares` `{"checked":145,"mismatched":[]}`.
- `status`: exit 0; same Rows and Shares lines.
- `quarry sql`: 57 investment rows carry a commission, 18 of them below a cent (stored exactly), 1 `split` row.
- Cash side: 14,061 transactions equals the Phase 1 probe's CashFlowTransaction count (investment rows stay out of `transactions`); `spend` runs, exit 0. Byte-equality of spend/cashflow with and without investment rows is pinned by `Test_run_spend_and_cashflow_are_unchanged_by_investment_transactions`, not re-measured here.

PRD Phase 4 gate "Share counts match Quicken": met for the real file (145/145 holdings, tolerance 0.000001).

## Run 3 — 2026-10-04, HEAD 6737fe3: v5 → v6 upgrade on a copy of the real store

Scratch HOME holding a copy of the user's real v5 `quarry.duckdb` (2 `import_runs`, ids 1–2, `format_version` 5) plus the snapshot copy.
- `sync --from 20260930T072052Z` (1st): exit 0, stderr empty; `Shares    145 holdings match Quicken's share counts`.
- `sync --from 20260930T072052Z` (2nd): exit 0, stderr empty; same Shares line.
- After: `import_runs` 4 rows (ids 1–4), the 2 pre-4a rows carried with `shares_checked` NULL; `format_version` 6; `status` exit 0.
