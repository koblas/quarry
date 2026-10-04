# Reference check: phase4b-holdings (SCENARIO-16)

Method: the binary built from this branch (HEAD a5b3b78) runs with `HOME` set to a scratch directory holding copies of the user's snapshot `20260930T072052Z` and the user's real store (format 6, built by the shipped 4a). The real store is never touched. Only counts and pass/fail are recorded, never holdings or amounts.

## Run 1 — 2026-10-04

- **Before the sync**, a read on the copied v6 store gives the existing format refusal: `the store … was built by another version of quarry; run quarry sync --from 20260930T072052Z to rebuild it`. This is I4-9 working as intended.
- **The sync**, `sync --from 20260930T072052Z`, exits 0 with stderr empty and prints `Shares    145 holdings match Quicken's share counts`. `store_info.format_version` becomes 7.
- **The command checks**, each exit 0:
  - `holdings --as-of 2026-09-30 --json`
  - `holdings --as-of 2026-09-30` (text)
  - `--currency native`
  - `holdings` with no flags (today)
- **On 2026-09-30, the command and the view agree**:
  - 21 holdings over 20 securities.
  - Every holding has a price, and the oldest price is dated on or after 2026-09-23.
  - `totals` has one CAD total, and `warnings` is empty, as it should be with no warnings expected.
  - The CLI `totals[0].value` equals `quarry sql "select sum(value_cad) from v_holdings where date = '2026-09-30'"`, compared exactly.
- **Share counts** (`holding_shares`, 414 span rows):
  - 21 spans are open (`to_date IS NULL`).
  - 0 `v_holdings` rows on 2026-09-30 have shares that differ from their open span.
  - 0 held rows lack an open span.
  - Since the 4a gate passed 145/145 and no investment transaction is dated after 2026-09-23, the shares on 2026-09-30 equal the share-check counts.
- **H-2 precondition:** `quarry sql --csv "select * from v_holdings where date = '2026-09-30'"` takes 0.047–0.049 s wall time per run, process start included.

**Unverified by user decision:** whether past values match Quicken's Portfolio view for dates that cross a `ZSECURITYSPLIT` date, which depends on whether Quicken's price history is split-adjusted. The user declined that manual comparison.
