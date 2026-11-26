# quarry — Testing and validation

Part of the quarry product spec; index in [README.md](README.md).

## Testing and validation

Correctness is defined as matching Quicken: every account's balance in `quarry` must equal Quicken's to the cent, or the sync fails.

**Validation on every import**

- Per-account balance reconciliation against the balances Quicken itself stores, at the latest date and at each reconciled statement date. (not built: #52)
- Splits sum to their transaction; every transfer has a matching counterpart (or is flagged as one-sided).
- Row counts and date ranges compared with the previous import; a large unexplained drop fails the sync. (not built: #53)
- Investment accounts: share counts per security match Quicken's holdings.

**Development tests**

- A synthetic Quicken file built in Quicken with known accounts and edge cases (splits, transfers, split-with-transfer, voided, foreign currency, stock split, reinvested dividend) used as a golden fixture.
- Unit tests for each view against hand-computed expectations.
- A one-time manual check: `quarry` reports for two recent years compared with the same reports run in Quicken.

**Non-functional**

| Requirement | Target |
| --- | --- |
| Full sync time | Under 60 s for 20+ years |
| Typical CLI / MCP query | Under 1 s |
| Runtime | macOS 14+, single Go binary (cgo, DuckDB linked in), installed with `go install` or a Homebrew tap (not built: #55) |
| Quicken upgrade | Schema fingerprint change is detected and reported, never silently mis-imported |
