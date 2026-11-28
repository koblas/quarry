# Why quarry

quarry has one goal: **answers from your Quicken data that you can check.** Quicken Classic for Mac holds years of categorized transactions, balances and investment activity, and offers a fixed set of reports to read them with. Two open-source projects opened that file to AI agents. quarry builds on both, and exists because of what they leave open. This page makes that case.

## The problem: the data is there, the answers are not

Quicken keeps its data in an undocumented Core Data SQLite database inside the `.quicken` package. Three things stand between that file and a question such as "how did our grocery spending change since 2022?":

1. **The file is locked.** Quicken encrypts the database when it closes, so the data is readable only while Quicken has the file open.
2. **The structure is Quicken's, not yours.** Dates count seconds from 2001. Categories sit on split lines, not on transactions. A transfer between two of your accounts is two transactions that look like a withdrawal and a deposit. Quicken keeps one current exchange rate per currency pair, not a history. Column names change between Quicken versions.
3. **A wrong total looks like a right one.** Count a transfer as spending, miss a refund, add US dollars to Canadian dollars, or sum a transaction's amount once per split, and the query still returns a number. Nothing marks it as wrong.

Exporting CSV files by hand for each question is slow and loses structure: splits, transfers and investment lots flatten badly.

## What already exists

Two MIT-licensed projects read the Quicken for Mac database for AI agents. quarry reuses their schema knowledge and SQL, and credits both.

**[dweekly/quicken-mac-mcp](https://github.com/dweekly/quicken-mac-mcp)** is a Claude skill, an MCP server with eight tools, and a `qmac` command line. The skill teaches the agent Quicken's schema and has it write SQL against the live database, read-only. Its schema reference is the most complete public description of the file.

**[hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills)** is six agent skills for Claude Code, Cursor, GitHub Copilot and Codex. A shell script opens the live database read-only, defines normalized views for that one session, and runs SQL recipes for net worth, spending, investments and data hygiene. It downloads daily exchange rates, so history converts at the rate of each transaction's date.

Both prove the approach works, and both are a good choice for what they set out to do. Both also share one design: every question is SQL against Quicken's live database, and the result is reported as it comes back.

## The gaps quarry fills

### 1. Nothing checks the totals

Neither project compares what it computes with what Quicken itself says. quicken-mac-mcp's skill tells the agent to treat each balance as a dated observation and to state its source. quicken-skills warns when Quicken's lots disagree with summed transactions. Neither stops when the numbers are off.

quarry rebuilds its store on every `quarry sync` and runs three checks on it, to the cent. In every reconciled account, the reconciled transactions must add up to the balance of its last reconciled statement in Quicken. Every transaction must equal the sum of its splits. In every brokerage and retirement account, each security's share count must equal Quicken's. If a check fails, the sync fails and the previous store stays in place. An answer from quarry comes from a store that passed.

### 2. Every question reads the live file

Both projects read the database Quicken has open, so Quicken must be running, with the file open, for every question. Yesterday's answer cannot be reproduced once the file has changed.

quarry reads Quicken's file only during `quarry sync`, to copy it. The copy is a read-only snapshot, and the store is built from it. Quicken can be closed for every command after that. quarry keeps the newest 12 snapshots, and `quarry sync --from` rebuilds the store from an older one, so a number can be traced to the file it came from. A scheduled job needs Quicken open for one command, not for a conversation.

### 3. The rules live in the prompt

When a skill teaches an agent the schema and the agent writes the SQL, the rule for what counts as spending is re-derived for each question. quicken-mac-mcp's skill warns the agent about this in so many words: do not claim transfers are excluded because account types were restricted. Its packaged spending tools sum negative splits only. quicken-skills moves the rules into SQL recipes, which is firmer, and any question outside a recipe goes back to free-form SQL.

quarry's rules are compiled into its commands. `quarry spend`, `quarry cashflow`, `quarry recurring` and `quarry anomalies` leave out transfers between your own accounts, Quicken's system categories, transactions marked "exclude from reports" and accounts Quicken leaves out of reports, and net refunds against their category. The MCP tools call the same code as the command line, so the two return the same numbers. Free-form SQL reads views that already apply the rules. [How quarry counts money](counting-rules.md) lists them.

### 4. Hygiene checks start over on every run

quicken-skills has nine hygiene checks, each a count for a period. quicken-mac-mcp has none. A count says how many problems there are today. It does not say which ones you already looked at, or whether last week's fix worked.

quarry records ten types of finding on every sync. Each finding has an id built from the ids of the records it names, which stay the same from one sync to the next. A finding is open until a later sync no longer finds it, and then it is fixed. One you have checked and want to keep goes in `findings.ignore` and stays off the list. Each finding names the exact transactions and the change to make in Quicken. [Findings and the cleanup loop](findings.md) explains the cycle.

### 5. The analysis stops short

| Question | quicken-mac-mcp | quicken-skills | quarry |
|---|---|---|---|
| Recurring charges | None | Monthly only: a 25 to 35 day cadence, amounts within 15% | Weekly, monthly, quarterly and annual series, with the first charge, whether the series is active or ended, and the cost per year |
| Price changes | None | The latest charge is above the previous one | Every step of more than 5% in a series, with its date and both amounts; an exchange-rate change is never a price change |
| Unusually large charges | None | The largest expenses in a period | Charges above 2 times the median of the payee's earlier charges, or 5 times the category's |
| Realized capital gains | Estimated gains from tax lots | None: its investments skill says Quicken does not store the cost of shares sold | Adjusted cost base per security, pooled across non-registered accounts, and realized gains per tax year, in CAD as the CRA defines them |
| Duplicates | None | Same account, date, payee and amount | Same account and amount within 3 days, unless both are reconciled |
| Transfers entered as two unrelated transactions | None | None | Opposite amounts in two accounts within 3 days, not linked as one transfer |

### 6. A toolchain to assemble

quicken-mac-mcp runs on Node through `npx`, or on Python 3 for the skill. quicken-skills is shell scripts over `sqlite3` and `curl`. Each ships a skill and one or two other ways in.

quarry is one Go binary with DuckDB linked in. The command line, the Claude Code skill and the MCP server for Claude Desktop are the same program reading the same store. `quarry claude install` sets up both Claude apps.

## How it differs

| | quicken-mac-mcp | quicken-skills | quarry |
|---|---|---|---|
| What a question reads | Quicken's live database, read-only | Quicken's live database, read-only, through views defined for that session | quarry's own store, built from a snapshot |
| Quicken must be open | For every question | For every question | For `quarry sync` only |
| Totals checked against Quicken | No | No | On every sync; a failed check keeps the previous store |
| Who applies the transfer and refund rules | The agent, in SQL it writes; eight tools for common cases | SQL recipes; the agent for the rest | quarry's commands and views |
| Data problems | Not reported | Nine checks, counted on each run | Ten finding types, each tracked as open, fixed or ignored |
| Currencies | Kept separate | Any currency, at daily rates from the ECB | CAD and USD, at daily Bank of Canada rates |
| Free-form SQL | SELECT only, 500 rows | Over the session's views | Read-only, 500 rows unless you ask for more, over a schema that does not change with Quicken |
| What you install | Node or Python 3 | Nothing beyond macOS | One binary; building it needs Go and the Xcode command-line tools |
| Ways in | Skill, MCP server, command line | Skills for four coding agents | Command line, skill, MCP server |

## Where the others are the better choice

quarry is narrower than both in several ways, on purpose or for now.

- **Currencies other than CAD and USD.** quarry converts between Canadian and US dollars only. quicken-skills handles any currency pair.
- **No sync step.** Both projects read the open file, so an answer always matches what Quicken shows at that moment. quarry answers from the last `quarry sync` and says how old that is.
- **Other agents.** quicken-skills ships plugins for Cursor, GitHub Copilot and Codex. quarry ships a plugin for Claude Code; other clients use its MCP server.
- **Nothing to build.** quicken-skills needs only what macOS includes. quarry is installed with `go install` and compiles DuckDB through cgo.
- **Investment detail.** quicken-skills reports allocation by asset class, unrealized gains per lot and dividends by year. quarry reports holdings, their value on a day, and realized gains.
- **Budgets and QuickFill rules.** quicken-mac-mcp documents how to query both. quarry's store does not hold them.
- **Quicken versions.** quarry reads Quicken Classic for Mac v9 and refuses a file whose tables and columns differ from its reference, where reading the live schema adapts.
- **Tax rules outside Canada.** quarry's capital-gains worksheet follows the CRA's adjusted cost base. It has no US lot-matching.

## Principles

- **Quicken is the system of record.** quarry never writes to the Quicken file. Fixes are made in Quicken, and the next sync picks them up.
- **A number that cannot be checked is not an answer.** The store reconciles to Quicken or it is not used. Every amount Claude reports comes from quarry's output, never from estimation.
- **Each rule is decided once.** What counts as spending is decided in quarry's code, so the command line, the skill and the MCP server cannot disagree.
- **One direction.** Quicken to snapshot to store to answer. quarry keeps no second copy of the truth: the store is rebuilt from the snapshot on every sync, and the only decisions it keeps are the ones you write in its config file.
- **Local by default.** The only network request fetches exchange rates and carries nothing but dates. The MCP server opens no network port.
- **Report, never act.** quarry gives no financial advice and makes no trades or payments. Its capital-gains output is a worksheet to review with an accountant, not a filing.

## What it won't do

- Write to Quicken, or fix a finding for you.
- Query Quicken's live database. Every question runs on the store.
- Download from banks. Quicken remains the place data arrives.
- Read Quicken for Windows files.
- Show dashboards or any interface beyond the terminal and Claude.
- Mask or rewrite payee names, memos or account names. They reach Claude as Quicken holds them.

## Who it's for

| Reader | What they get |
|---|---|
| You, at a terminal | Commands with readable tables, `--json` on every one, and read-only SQL |
| You, asking Claude | Answers in plain words, each number from quarry's output, with the date of the data stated first |
| A scheduled job | `quarry sync` and `quarry summary` once a month, with exit codes a script can branch on |
| Your accountant | A per-security cost base history and realized gains per tax year to review |

## Where to go next

- [Your first question](../tutorials/first-question.md): install quarry, take a snapshot, ask something.
- [How quarry works](how-quarry-works.md): snapshots, the store, and what leaves your Mac.
- [How quarry counts money](counting-rules.md): the rules behind every total.
- [How to use quarry with Claude](../guides/use-with-claude.md).

## Credits

quarry builds on the schema knowledge and SQL of [dweekly/quicken-mac-mcp](https://github.com/dweekly/quicken-mac-mcp) and [hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills), both MIT-licensed. The comparison on this page describes the copies of each that quarry started from, kept in the repository under [`docs/prior-art`](https://github.com/koblas/quarry/tree/main/docs/prior-art); both projects have kept moving. The license text is in [`THIRD_PARTY_NOTICES`](https://github.com/koblas/quarry/blob/main/THIRD_PARTY_NOTICES).
