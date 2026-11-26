# quarry — Decisions

Part of the quarry product spec; index in [README.md](README.md). Every design step reads this
first. When a decision changes, amend its entry here in the same pull request that changes the code.

## Product decisions

Carried unchanged from the PRD's Decisions list (docs/initial-prd.md, Sep 27, 2026, rev 47).


- Source: Quicken Classic for Mac, version 9.
- Accounts: CAD and USD accounts both in scope.
- History: everything, including closed accounts.
- Input: `quarry sync` takes its own snapshot with SQLite's backup API while Quicken is open; no hand-made snapshot or import step.
- Interfaces: CLI, MCP server and a Claude skill, the skill as Claude's primary path.
- Claude install: quarry claude install / uninstall act on Claude Code and Claude Desktop, whichever is on the Mac. Claude Code gets the plugin by running the claude command (user scope); quarry never edits Claude Code's files, and the download is Claude Code's, not quarry's. Claude Desktop gets only the MCP server, as the mcpServers.quarry entry in its config file with quarry's full path; quarry changes only that entry, keeps a backup of the file it replaces, and refuses a file it cannot parse or an entry it does not own.
- Reuse: build on dweekly/quicken-mac-mcp (and hardkoded/quicken-skills), both MIT, with notices preserved.
- Reporting currency: selectable, CAD, USD or native (each account's own currency, unconverted); default from config `reporting.currency`, else CAD. CLI and MCP accept the same values; MCP's are exact-case.
- Search: defaults to all dates; --min/--max compare absolute native amounts; no --currency; search is a lookup, not a report.
- ACB: computed when reporting in CAD, per security across non-registered accounts, with trade-date FX.
- FX history: daily series from the Bank of Canada, fetched incrementally by `quarry sync`, since Quicken stores only the current rate. A failed fetch warns but does not fail the sync.
- Exit codes: `0` success (warnings included), `1` failure, `2` usage error, for every command.
- Feedback: findings go back into Quicken as a cleanup worklist so later snapshots and exports are clean.
- Command names: the worklist is `quarry findings`, snapshot housekeeping is `quarry snapshots` / `quarry snapshots prune`; no command is called `cleanup`, since it reads as either.
- Monthly summary: quarry summary reads the store only; "last month" is the calendar month before today in local time; a recurring charge is new in the first month quarry recurring can list it, unless it starts again after an off-schedule charge before its earlier series had ended; recurring is judged as of the month's last day; the job is quarry sync; quarry summary from launchd; no stored state.
- Snapshot retention: a count cap (`snapshots.keep`, default 12), applied after every successful sync and by `snapshots prune`; the store's own snapshot is always kept. No size or age caps in v1.
- Redaction: only account numbers held in `quarry`'s config are masked (last four digits). Free text is passed through as Quicken holds it, since institutions and Quicken already mask card and account numbers there.
- Dashboards: out of scope for v1.
- Language: Go, shipped as a single binary (duckdb-go, official MCP Go SDK).
- Store engine: DuckDB, for exact `DECIMAL` money, ASOF joins against FX rates and prices, and analytical SQL. Tables use standard types only, so a move to SQLite stays a port. Syncs write a new file and swap it in atomically, since DuckDB allows one writer.

## Rulings

Rulings made while building a feature, folded here from the feature-spec folders under
`docs/specifications/` (removed; kept at tag `pre-product-spec`). Each is dated and names the folder it came from; where a ruling quotes
copy, the copy is the contract.

### 2026-10-05 · ACB refusals and currency (phase4de-acb, RULING-S17)

- Refusal precedence for `quarry acb`: positional argument (exit 2), then `--currency` (2), then
  `--year` (2), then a config refusal (1), then a store open or read refusal (1), then unclassified
  accounts (1), then an unknown `--security` (1). In `--json` mode every refusal leaves stdout
  empty and prints the same stderr line.
- Unclassified accounts (exit 1): `quarry: acb needs every brokerage and retirement account
  classified; N accounts are in neither accounts.registered nor accounts.non-registered in
  ~/Library/Application Support/quarry/config.toml; quarry findings --type unclassified-account
  --status all lists them` (singular form for one account: `1 account is … lists it`). Open and
  ignored unclassified accounts count, closed ones included; zero never prints the line.
- `acb` is CAD only. Any other `--currency` value, in any case, is refused with exit 2 and
  `quarry: acb is in CAD only, as the CRA requires; run it without --currency`; the value is not
  echoed and the generic `--currency must be CAD, USD or native` line is not used. `CAD` in any
  case is accepted and ignored.
- The skill writes quarry's config file only when the user asks: to record an account
  classification the user just gave, or to add `acb.adjustment` lines for amounts the user reads
  from a T3 slip. It shows the exact lines first and writes them only after the user says yes.
  quarry itself never writes the file.
- The classification question lists `(RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or IRA, or
  similar)`, and never guesses from the account's name or Quicken's account type.

### 2026-10-05 · ACB on the MCP server (phase4de-acb, RULING-S19)

- MCP rewording rule: a CLI sentence stays the same on MCP; only a CLI command phrase becomes the
  tool phrase (`quarry findings --type <t>` → `data_quality with type <t>`; `… --status all` →
  `… and status all`; `quarry acb --security <id>` → `acb with security <id>`; bare
  `quarry acb --json` → `acb with no arguments`). `run quarry sync …` stays verbatim, since only
  the user runs it.
- Unknown security: `acb covers no security named "XYZ"; acb with no arguments lists every
  security it covers` (Go-quoted; the first unmatched selector in argument order).
- `year` is an integer from 1 to 9999; a year after this one is refused with `year 2027 is after
  this year; pass this year or an earlier one`.
- The `acb` tool returns at most 500 events in document order, keeps every `years[]` and
  `securities[]` header, and ends its warnings with `acb lists the first 500 events of N; pass
  security to narrow`.
- The unclassified-account fix sentence names RRSP, RRIF, TFSA, RESP, FHSA, LIRA, a US 401(k) or
  IRA, or a similar registered plan.

### 2026-10-05 · ACB warnings and the skill (phase4de-acb, FINAL-PV)

- Every name taken from the Quicken file (security, account, ticker) is Go-quoted (`%q`) in a
  warning, so a quote, newline or escape character in a name cannot split or disguise the line.
  Values echoed from config keep TOML basic-string quoting.
- The skill's section 7 is "Gains and tax": `quarry acb` is a worksheet to review with an
  accountant, not a filing; the skill relays every warning line with its numbers and never calls
  a loss deductible or denied.

### 2026-10-05 · Pooled oversell and short cover (phase4de-acb, RULING-S22)

- A sale or share removal of more units than the non-registered pool holds removes all of the
  pool's ACB and leaves the pool short by the excess, at ACB 0.00. A sale counts the excess at no
  cost, is marked `unknown_cost`, and its gain is not adjusted later (overstating the gain is the
  safe side).
- An acquisition while short covers the short first; covered units add no ACB. Units beyond the
  short enter the pool at a pro-rata share of the cost, rounded to the cent half away from zero,
  after converting USD at the acquisition's rate.
- An unknown-cost span ends when pool shares next reach 0 or below. A split while short
  multiplies the negative shares. An adjustment while shares are at or below 0 gets the existing
  not-held warning and counts no gain.
- Text positions list only shares above 0; JSON shows negative `shares`, `acb` "0.00",
  `acb_per_share` null and `incomplete` true while short.
- Warning 10a, per oversold sale: `"<security>": the sale on <date> in "<account>" sold <x> more
  shares than the non-registered accounts held; quarry counts them at no cost, so the sale's gain
  is too high by what they cost, and the next <x> shares acquired only bring the holding back to
  0; correct the shares in Quicken if they are wrong`. Warning 10b, per oversold removal:
  `"<security>": <x> more shares left "<account>" on <date> than the non-registered accounts held;
  the next <x> shares acquired only bring the holding back to 0; correct the shares in Quicken if
  they are wrong`. `<x>` is the short after the disposition.

### 2026-10-01 to 2026-10-06 · Reference checks (REFERENCE-CHECK files)

- A phase that reports on real money ships only after a reference check: the branch's binary runs
  on copies of the user's snapshot and store in a scratch `HOME`, never on the user's own store,
  snapshots or config. It records counts and outcomes only, never names or amounts from the
  user's file. A difference from Quicken it cannot explain becomes an issue, not a note.
- The verdicts those checks recorded are RC-1 to RC-6 below. The rule gaps they found are rulings
  of their own (the commission scale in `architecture.md`, RULING-S22 above, the recurring "new" rule
  in the Monthly summary decision). A check left to the user by hand is open work for an issue,
  not a decision.

### 2026-10-01 · Findings and recurring verdicts (phase2d-findings, phase2e-recurring-anomalies)

- RC-1. The findings heuristics were accepted on a sample of up to 20 per type. A reimbursement
  deposit that mirrors an expense's amount is an accepted false-positive shape: the user ignores it
  with `findings.ignore`, and the heuristic is not narrowed for it.
  (`pre-product-spec:docs/specifications/phase2d-findings/REFERENCE-CHECK.md`)
- RC-2. Anomaly volume (about 9% of charges since 2024 on the real file) and recurring series with
  many price changes (6 and 8 on the real file) are accepted as shipped; neither is a reason to
  tighten the rules. (`pre-product-spec:docs/specifications/phase2e-recurring-anomalies/REFERENCE-CHECK.md`)

### 2026-10-04 · Past holdings across a split, unverified (phase4b-holdings, phase4c-networth)

- RC-3. Whether past holdings values match Quicken's Portfolio view on dates that cross a
  `ZSECURITYSPLIT` date was not checked: it depends on whether Quicken's price history is
  split-adjusted, and the owner declined the manual comparison. It is unknown, not verified; 4c
  left it as 4b did. (`pre-product-spec:docs/specifications/phase4b-holdings/REFERENCE-CHECK.md`,
  `pre-product-spec:docs/specifications/phase4c-networth/REFERENCE-CHECK.md`)

### 2026-10-05 · Investment cash and ACB against Quicken (phase4c-networth, phase4de-acb)

- RC-4. An FI statement's cash can differ from quarry's cash on its as-of day by settlement
  timing, since the broker's figure already includes the next day's cash moves. That is accepted,
  not a rule gap. (`pre-product-spec:docs/specifications/phase4c-networth/REFERENCE-CHECK.md`)
- RC-5. Per-sale ACB removed differs from Quicken's lot cost by design: quarry pools and averages,
  as the CRA requires, where Quicken removes specific lots. Only each security's total CAD cost is
  conserved, so a per-sale difference from Quicken is not a bug.
  (`pre-product-spec:docs/specifications/phase4de-acb/REFERENCE-CHECK.md`)

### 2026-10-06 · Summary and today's recurring (phase4f-summary)

- RC-6. `quarry summary` can report as new a series that today's `quarry recurring` does not list:
  one steady at its month end that broke later. That is accepted, since summary judges as of the
  month's last day and plain `recurring` judges only the latest run as of today.
  (`pre-product-spec:docs/specifications/phase4f-summary/REFERENCE-CHECK.md`)
