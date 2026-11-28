# quarry

quarry turns the Quicken Classic for Mac file you have open into a clean, local, queryable financial database. It copies the file into a read-only snapshot, rebuilds its own store from that snapshot, and checks the store against Quicken's balances. Every other command reads that store; quarry never writes to the Quicken file.

Ask it how much you spent on groceries last year, which subscriptions started this year, what your net worth was at each month end, or what to clean up in Quicken, from a terminal or from Claude.

| You want to… | Read |
|---|---|
| Install quarry and answer a first question | [Your first question](tutorials/first-question.md) |
| Understand snapshots, the store and what leaves your Mac | [How quarry works](concepts/how-quarry-works.md) |
| Ask Claude Code or Claude Desktop about your data | [How to use quarry with Claude](guides/use-with-claude.md) |
| Change what quarry keeps, reads or reports | [How to configure quarry](guides/configure.md) |
| Look up a command or flag | [The CLI](reference/cli.md) |
| Look up a config key | [config.toml](reference/config.md) |

## Credits

quarry builds on the schema knowledge and SQL of two MIT-licensed projects, [dweekly/quicken-mac-mcp](https://github.com/dweekly/quicken-mac-mcp) and [hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills). The license text is in [`THIRD_PARTY_NOTICES`](https://github.com/koblas/quarry/blob/main/THIRD_PARTY_NOTICES).
