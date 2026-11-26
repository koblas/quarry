# quarry — Security and privacy

Part of the quarry product spec; index in [README.md](README.md).

## Security and privacy

All data stays on the Mac; the only data that leaves is the result of a query David or Claude explicitly runs.

- **Read-only against Quicken.** `quarry` touches the live database only through SQLite's read-only backup API during sync; every query runs on a snapshot, and neither is ever written.
- **Encrypted at rest.** Snapshots and `quarry.duckdb` live under `~/Library/Application Support/quarry/` on a FileVault volume; files are `0600`. Optional DuckDB encryption with the key in the macOS Keychain. (not built: #54)
- **Account numbers in configuration.** `quarry` imports no account-number field from Quicken. Where the user writes an account number into `quarry`'s config (for example to name an account in the account-classification setting), `quarry` shows it masked to the last four digits in everything it prints or serves, CLI and MCP alike. Free text (payees, memos, notes) is not scanned: card and account numbers there are already masked by the institution's download or by Quicken, so `quarry` passes it through as Quicken holds it.
- **MCP boundary.** The MCP server is local stdio only (no network listener), SQL is read-only (enforced by a read-only connection), and results are row-capped so a stray `SELECT *` can't dump the whole history into a conversation.
- **Claude Desktop config.** quarry claude install and uninstall change only the quarry entry in Claude Desktop's config file, never another key, and save the file they replace beside it.
- **Snapshot retention.** Snapshots are capped by count (default 12) and the oldest are deleted after each successful sync; the rules live under Sync.
- **No telemetry.** The only outbound network call in v1 is `quarry sync` fetching exchange rates from the Bank of Canada; it sends no user data.
