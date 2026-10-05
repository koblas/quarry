# Reference check: phase4c-networth (SCENARIO-19)

Method: the binary built from this branch (HEAD fd3d795) runs with `HOME` set to a scratch directory that holds copies of the user's store and snapshot. The user's real store is never touched. Only counts, pass/fail and timings are recorded here. Account figures were shown to the user once, in the conversation, for the manual check (user decision 7).

## Run — 2026-10-05, snapshot 20261004T184923Z

- **Sync**, `sync --from 20261004T184923Z`: exit 0 in 1.7 s.
  - Rows: 15,593 transactions; 1,611 investment transactions.
  - Balances line: `9 investment accounts' cash not checked` (SCENARIO-08 copy).
  - `Shares    145 holdings match Quicken's share counts`.
  - `store_info.format_version` = 8.
- **FI statement cash** (6 `ZFISTATEMENT` rows, each compared on its own `ZDATEASOF` day):
  - 5 match `v_balances_daily.cash` exactly.
  - 1 differs by ≥ 1.00. The difference is explained by settlement timing: the broker's figure already includes the next day's cash moves, and quarry's cash on the next day equals `ZAVAILCASH`.
  - No unexplained difference.
- **Category kind per action** (investment cash rows):
  - buy and sell are system; margin_interest is expense.
  - dividend, interest, capital_gain_long and capital_gain_short are income.
  - misc_expense: 1 expense, 1 with no category. misc_income: 2 income, 1 with no category.
  - The two rows with no category carry Quicken's built-in "Uncategorized" system category. The importer maps that category to no category (`internal/importer/categories.go:87`), so N-2's "no category counts by sign and raises `uncategorized`" applies. Not a rule gap.
  - These actions have no cash row: add_shares, remove_shares, split, and reinvest_dividend (all amount 0; N-3 outcome (b)).
- **Totals agree** (today, CAD):
  - The `networth --json` total equals `sum(balance_cad)` from `v_net_worth` for today, with 0 NULL rows.
  - It also equals the sum of `accounts --json` and of `accounts --all --json` `converted_balance` over counted accounts.
  - `networth` printed no warnings.
- **Cash-flow change** (rows in `v_cash_flow` from investment cash):
  - 944 income rows: dividends, interest, capital gains, misc_income, and 1 uncategorized.
  - 36 expense rows: 34 margin interest, 1 misc_expense, 1 uncategorized.
  - `v_spending` carries the same 36. Buy and sell rows are absent (system kind).
- **Timing** (warm, best of 3, process start included):

  | Command | Time |
  | --- | --- |
  | `networth` | 0.09 s |
  | `networth --since 2013 --json` | 0.10 s |
  | `accounts` | 0.07 s |
  | `holdings` | 0.04 s |
  | `v_net_worth WHERE date IN (200 month ends)` | 0.09 s |

  The 45 s synthetic-fixture cliff that SCENARIO-14a observed does not appear on the real file.
- **Manual cash check** (user decision 7):
  - The user compared quarry's cash for the 9 investment accounts with Quicken.
  - On the earlier snapshot (20260930T072052Z), two Fidelity accounts differed. Rebuilt from the Oct 4 snapshot, all 9 match to the cent, including both Fidelity accounts.
  - The earlier difference was cash activity Quicken recorded after Sep 30, not a quarry rule.

**Unverified:** none for this slice. Past values across a split date remain as 4b left them (user declined the Portfolio comparison).
