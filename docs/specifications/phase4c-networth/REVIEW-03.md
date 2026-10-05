# Review Report — final product-vision pass (step 10)

### Target
Finished surface at 01fe2c0: help for networth, accounts, cashflow, spend, findings, holdings and mcp; the MCP `net_worth` tool; SKILL.md, schema.md and the SQL conventions; the PRD rows.

### Ratified with no change (rulings on STATE open debts)
- `asOfRefusedLog` for the as_of × since/until conflict: kept.
- MCP `as_of`, `since` and `until` parameter descriptions (`internal/mcp/tools.go:138-140`): ruled as written; recorded in Surface & Copy.
- "on 1 of the month ends listed" and "on 1 month end before …": accepted.
- Empty-history caption `Net worth at each month end, amounts in CAD`: ratified (unreachable, and true).
- Mixed history cell shows the converting sum: already ruled.
- Holding in an account that is neither CAD nor USD: silent per N-7; the importer refuses such accounts.

### MAJOR (SHIP WITH CHANGES), ranked
1. **MCP rate-warning advice names a CLI flag.** `internal/report/document/networth_rate_warnings.go:21-22,28-29`:
   - On MCP, `pass --currency native to list them` becomes `pass currency native to list them`. This applies to the snapshot, history and no-rates forms.
   - The no-rates tail `…, or run quarry sync to fetch rates` is kept.
   - CLI copy is unchanged.
   - Implement it with a parameter or hint passed by each surface, not by `strings.Replace` on the finished line.
   - Pin it in the MCP `net_worth` tests. The CLI↔MCP parity tests compare warnings, so adjust them where a rate line appears.
2. **Conventions sentence** (`internal/report/sql_conventions.go:24-25`, with mirrors in `internal/cli/sql_test.go` and `cmd/quarry/run_shared_documents_test.go`; regenerate schema.md):
   - From: `investment_transactions holds each one's action, security and shares: Their amount is DECIMAL(18,2)`
   - To: `investment_transactions holds each one's action, security and shares; its amount is DECIMAL(18,2)`
3. **v_net_worth conventions sentence** (`sql_conventions.go:50-53`, same mirrors):
   - From: `v_net_worth has net worth by day, account type and currency over the accounts Quicken's reports count, as quarry networth does;`
   - To: `v_net_worth has one row per day, account type and currency, adding up the balances of the accounts Quicken's reports count, as quarry networth does;`
   - The tail is unchanged. The N-6 view COMMENT is unchanged.
4. **v_balances_daily grain wording**, in `balancesDailyViewComment` (`internal/store/duckstore/schema.go:298`) and `sql_conventions.go:46-47`:
   - `from its first transaction through today` becomes `from its first transaction or holding through today`.
   - Update the spec N-5 COMMENT line and regenerate schema.md.
5. **Skill findings reference** (`plugin/skills/quarry/references/findings.md`): after the `unlinked-transfer` bullet (line 16), add
   `- \`duplicate\` and \`unlinked-transfer\` compare register entries only, not buys, sells, dividends or other investment transactions.`

### MINOR
- A history run with since and until in the current month captions `… 2026-10-04 to 2026-10-04`. This is left alone.

### Verdict: SHIP WITH CHANGES
