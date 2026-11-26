# quarry — Users and use cases

Part of the quarry product spec; index in [README.md](README.md).

## Users and use cases

Three consumers share one database: David at the terminal, Claude through MCP or a shell, and unattended scheduled jobs.

| Consumer | Interface | Example |
| --- | --- | --- |
| David | CLI, SQL | `quarry spend --by category --since 2024`; ad-hoc DuckDB queries; export to a spreadsheet |
| Claude (desktop / chat) | MCP server | "How did our grocery spend change since 2022?" "Which subscriptions started this year?" |
| Claude (Claude Code) | Skill + CLI | Scripted analyses, building dashboards, one-off notebooks |
| Scheduled job | CLI | Monthly sync + summary of anomalies and new recurring charges |

Representative questions the system must answer correctly:

- Spending by category and payee over any period, excluding transfers between own accounts.
- Recurring charges: what they are, when they started, how their price changed.
- Anomalies: unusually large transactions, duplicates, uncategorized items.
- Cash flow and savings rate by month and year.
- Net worth history, by account and account type.
- Investment activity: contributions, dividends, realized gains, holdings over time.
- Tax-relevant totals by category for a given year.
