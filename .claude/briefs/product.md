# Brief: quarry's product, for product-vision

The quarry half of the plugin's `pipeline:product-vision` agent: what the product is, the
invariants it defends, its surface and consumers, and the copy a scoping pass owes. The agent
reaches this file from `.claude/briefs/repo.md` → *For product-vision*. Its verdict format and
evaluation questions are the plugin agent's own and are not repeated here.

Specs written before the plugin live under `docs/specifications/<slug>/`; new ones under
`docs/specs/<slug>.md`. Read both when judging a surface.

## What quarry is

Quicken Classic for Mac (v9) hold decades of categorized transactions, balances, investment activity — locked in undocumented Core Data SQLite store. Quicken reports fixed. Hand-exported CSV flatten splits, transfers, lots badly. `quarry` make that data first-class local source: **snapshot open Quicken file, rebuild normalized DuckDB store from it, reconcile against Quicken own balances, serve it** via CLI, local MCP server, Claude skill. Data-quality findings go back to user as cleanup worklist applied *in Quicken*, so every later snapshot cleaner.

Normalized store is product. Quicken quirks resolved once, in importer. Core library own every analysis rule. CLI, MCP server, skill stay thin.

**`docs/initial-prd.md` is the authority**, plus any later PRD in `docs/` (later one wins on conflict) — goals/non-goals, data model, conventions, CLI + MCP surface, cleanup findings, reporting currency, ACB, security, validation, milestones, risks, decisions. Feature specs (`docs/specs/<slug>.md`; older ones `docs/specifications/<slug>/specification.md`) refine it per feature. Read sections bearing on what you judge, not this summary. Its `Decisions` section record what settled — check there before re-opening anything.

## Invariants you defend

Rejection criteria, not suggestions. Proposal violating one = DON'T BUILD or RETHINK. Name the rule.

- **Q1 — read-only against Quicken.** quarry never writes to Quicken. The live database is touched only by SQLite's read-only backup API during `quarry sync`; every query runs on a snapshot, and snapshots are never written. Quicken closed (encrypted) is reported, not read. Fixes flow back as a worklist the user applies; Quicken stays the system of record.
- **Q2 — correct means matches Quicken.** Every account balance equals Quicken's to the cent, at the latest date and each reconciled statement date, or the sync fails. Splits sum to their transaction; transfers pair or are flagged. A failed build leaves the previous store untouched (new file, atomic swap). Never a partial or silently wrong store.
- **Q3 — only importer know Quicken.** Quicken schema live in one versioned package, schema fingerprint per import. Fingerprint change reported, never silently mis-imported. CLI, MCP server, skill know only quarry own tables and views.
- **Q4 — one owner per analysis rule.** Transfer exclusion, split allocation, sign convention, FX conversion, ACB live in core library. CLI and MCP never disagree. Proposal re-deriving rule in front end (or in skill SQL) = defect. Transfers excluded from spending and income by default.
- **Q5 — money exact and native.** `DECIMAL(18,2)`, never floats. Amounts stored in account native currency (CAD or USD); conversion only in views, at Bank of Canada rate for transaction date (valuation date for balances). Cross-currency transfers keep both legs. CAD default reporting currency.
- **Q6 — stable identity, no second truth.** IDs derive deterministically from Quicken source IDs, so re-imports keep them and finding tracked open → fixed across snapshots. quarry store *decisions about* findings (ignored, not an issue), never corrected data — no divergent copy of ledger.
- **Q7 — local and bounded.** Data stays on the Mac; only results a user or Claude explicitly asks for leave it. MCP is stdio only, SQL is read-only by connection, results are row-capped (default 500). Account numbers and card-like patterns are masked on import. Files are 0600 under `~/Library/Application Support/quarry/`. No telemetry; the only outbound call is `sync` fetching exchange rates, and it carries no user data.
- **Q8 — every number come from quarry.** Skill and MCP answer from quarry output or SQL, never estimation. ACB and realized gains = worksheet for review, not a filing. Superficial losses and positions with no purchase history flagged, not adjusted.

## Out of scope — the standing DON'T BUILD list

- **Writing to Quicken** in any form, or querying live database outside `sync`.
- **Bank aggregation / live account syncing.** Quicken remains ingestion point.
- **Quicken for Windows (.QDF)** in v1. Ingest layer stays pluggable so CSV/QIF importer can land later — keep that seam, don't build through it.
- **Dashboards or any UI** in v1.
- **Financial advice or automated decisions** — trades, payments, tax filing.
- **Silent cleanup** — merging payee variants, recategorizing, deduplicating inside quarry. Those are findings with suggested Quicken fix.
- **External price feeds** in v1; security prices are what Quicken recorded.
- **An MCP server that changes data.** Never runs `sync`; freshness reported, not changed.

## The surface as proposed

CLI (every command: readable table by default, `--json` for machines; every reporting command takes `--currency CAD|USD`): `sync` (`--from <snapshot>`), `status`, `accounts`, `spend`, `cashflow`, `networth`, `recurring`, `anomalies`, `acb`, `cleanup` (`--csv`), `sql`, `export`, `mcp`. `sync` is the only command that touches Quicken or writes the store.

MCP tools: `describe_schema` (including sign, transfer and currency conventions), `query` (read-only, row cap, timeout), `spending`, `cash_flow`, `net_worth`, `recurring_charges`, `anomalies`, `acb`, `search_transactions`, `sync_status`, `data_quality`. Named tools exist so common questions don't depend on Claude re-deriving transfer rules; `query` is escape hatch.

Skill: Claude primary path (MCP for clients that can't load skills). Check freshness with `quarry status` first, call `quarry … --json` and `quarry sql`, carry no Quicken schema knowledge, ask in session for facts only user has (registered-account classification, return-of-capital adjustments), store answers in quarry config.

## Prior art — reused, not rejected

quarry build on MIT-licensed work instead of rediscovering Quicken schema. Notices go in `THIRD_PARTY_NOTICES`; ported files name their source.

- **dweekly/quicken-mac-mcp** — schema reference (84 entities, Core Data epoch dates), SQL recipes, skill layout, database auto-detection, CSV exporter. Starting code.
- **hardkoded/quicken-skills** — backup-API snapshot, daily FX import, hygiene checks, v9 test schema.
- **HarryDolan/qquery** — confirms approach hold across Quicken versions.

What none of them do, why quarry exist: reconcile against Quicken on every import, keep stable IDs so findings tracked to fixed, ship as one binary. Proposal dropping one of those three has re-proposed existing tool.

Rejected substrates: hand-exported CSV/QIF (lossy on splits, transfers, lots), querying live database (encrypted when closed, never safe to hold open), floats for money. DuckDB settled (exact DECIMAL, ASOF joins against FX and prices); tables use standard types so move to SQLite stays a port.

Settled (`docs/initial-prd.md` → `Decisions`): Quicken Classic for Mac v9; CAD and USD accounts; full history including closed accounts; `sync` takes own snapshot; CLI + MCP + skill, skill primary; CAD/USD reporting, CAD default; ACB in CAD per security across non-registered accounts at trade-date FX; Bank of Canada daily FX series; findings as Quicken worklist; Go, one binary, duckdb-go and official MCP Go SDK; `sync` fetches Bank of Canada rates incrementally (only outbound call, no user data; failed fetch warns, never fails the sync); exit codes `0` success (warnings included), `1` failure, `2` usage.

## The consumers

Every feature ship to three consumers. Serve only one = part-built.

1. **Person at terminal** — readable output, `--help` that answers question without web search, errors that say what to do next, no silent success, no wall of text where line will do.
2. **Script or scheduled job** — `quarry` in pipeline or monthly cron. Means: stable exit codes, machine-readable output on demand (`--json`), diagnostics on **stderr** and data on **stdout**, no interactive prompt without non-interactive path, no ANSI colour when output not a TTY.
3. **Claude, via skill or MCP server** — tool and field names that read right without PRD, conventions stated in `describe_schema` so ad-hoc SQL get them right, bounded results, freshness visible, same numbers CLI prints for same question.

Tell that these out of sync: user piping output into `grep`/`awk`/`jq` and having to strip banner, progress line, or colour escape to reach value; or MCP tool and CLI command giving different totals for same period. Not formatting nit — name the specific output that must move or the rule that must be shared.

## Diagnosability

Raise **during design**, not at review — "how does user find out why" change what data design must carry.

Every feature needs answers to:

- **"What went wrong and what do I do next?"** Error names resource, reason, concrete next action. One error vocabulary across binary; feature inventing own error shape has fragmented contract.
- **"Is failure distinguishable?"** Empty result and broken query must not look identical. Integrity failures return errors, never empty output with exit 0.
- **"Does exit code say which?"** Exit codes are contract a script branches on, PRD fixed them: `0` success (warnings included), `1` failure (reconciliation, Quicken closed, not found, query error), `2` usage. Judge whether new command maps cleanly onto those three; if genuinely cannot, that is PRD change to argue explicitly, not fourth code added in passing.
- **"Can operator answer it after the fact?"** What lands in logs, at what verbosity, keyed by what id. `--verbose` should be useful, not firehose.

## The scoping pass owes the literal copy

Two full passes not same job. **Final** pass judge surface that already exist, so every change it asks costs failing test, production edit, re-gate, reviewer round. **Scoping** pass costs one edit to spec section nobody implemented yet.

So scoping pass does not stop at "shape is right". Write out, literally:

- every command and flag name, and **flag help string** as user will see it rendered (including placeholder — backticked word in pflag usage string *becomes* the placeholder);
- success line, each refusal line, each fix line;
- exit code for each outcome;
- `--json` field names, and where document differs between modes of same command;
- **every failure outcome of a destructive or config-parsing command** — refusal, partial failure, interrupt, unreadable input, each value shape the parser can meet — each with line and exit code;
- **existing copy the feature makes false** — grep help Longs, error lines and skill text for statements of any rule the feature defines or changes (what counts as a transfer, which accounts count); list each with its replacement line under `## Surface & Copy` → *Changes to existing surfaces*;
- **reused copy re-derived** — refusal, hint or fix line reused from sibling command: perform its advice literally on the new command (run the suggested flag) and confirm it ends in success; if not, rule new copy here;
- **cross-surface rules, sites enumerated** — rule saying "every X" (every text cell, every path in `--json` / `warnings[]`, every `--csv` output) lists its concrete sites, and names existing precedent it reuses or deliberately differs from (e.g. `sql` table escaper, earlier phase's warning copy);
- **data-bearing text from user's file** (payees, memos, account, category and security names) — control characters, embedded commas and quotes, empty string versus NULL — ruled for each of text, `--json`, `--csv` **and stderr warnings / `warnings[]`** (a name inside a warning goes through `%q`, as `internal/report/document/holdings.go` warnings do, so a quote or newline cannot split the line); and **path form** per channel (`~` on stderr, absolute in `--json`);
- **every variant of each warning and refusal** — one ruled line per cause, not one line plus "…": which inputs pick which variant (cause only A, only B, both; rates exist vs none; sell vs remove), granularity (one per security / per item / per year) and order within its slot;
- **MCP counterpart of every CLI string** the feature adds — rule once, at scoping, the rewording rule for refusals, warnings and fix lines that name a CLI command (`quarry findings --type X` → `data_quality with type X`), which lines stay verbatim, and each new tool param's type, bounds and description;
- **behaviour claims cited** — every ruled sentence that states what quarry does or never does ("only", "never", "nothing", "always", what a null or empty value renders as) names the `file:line` that makes it true, beside the line in `## Surface & Copy`. No citation found → rule different copy. Byte pins prove a string is present, never that it is true;
- **business rules ruled as intent** — a ruling on what a value is ("a day with nothing needing a rate counts 0") states the intent first; any data-shape wording ("when `Totals` lacks the reporting entry") follows as an illustration, never as the rule. Code built to the shape misses the inputs where shape and intent part, and the gate finds them;
- **fallbacks for invalid input justified and staged** — a rule mapping invalid or ambiguous input (duplicate, missing, unreadable) to a fallback or default says why it is not an integrity failure (`correctness-reviewer` → bullet "Never downgrade an integrity failure to an empty result"), and names every stage that can meet that input first (schema check, importer, renderer) with the outcome at each — one ruling for all stages, not one per stage found later;
- **edge-case row table** for every output block, row kind, hint, suffix: each input class reaching it (no snapshot yet, Quicken closed, stale store, reconciliation failed, schema fingerprint changed, empty period, CAD vs USD, closed account, flag given vs not) with exact text it gets — or that it gets no row, and why. Hint only ruled once you said which rows it true for.

These land in specification `## Surface & Copy` section; developer implements verbatim. Anything left unwritten gets invented at keyboard and comes back to you in final pass, at ~10x cost.

At final pass, copy already ruled is settled — re-open only if implementation proved it wrong. Spend that pass on what only built surface can show: fix that cannot clear own finding, success line claiming more than happened, help string rendering differently than it reads in source.

**Mid-feature copy ruling (third, scoped pass).** When scenario, fix pass or reviewer find failure mode spec has no copy for (new refusal, new exit code, new hint), you called on that one outcome only: rule its literal line(s) and exit code into `## Surface & Copy`, same way as scoping pass; a business-rule ruling states its intent, as above. Ruling that overrides or narrows an earlier business rule or edge-case row names it, gives its replacement text, and lists the earlier pins it affects (`.claude/briefs/repo.md` → *For the orchestrator*). Do not re-review rest of surface. Verdict SHIP / SHIP WITH CHANGES auto-continues; RETHINK or DON'T BUILD goes back to user.

## Reading the surface

Read the actual surface before judging: `docs/initial-prd.md`, the specs and their
`SCENARIO-XX.md` plans, the commands under `internal/cli`, feature packages under `internal/`,
wiring in `cmd/quarry`, and the skill under `plugin/skills/quarry/`. quarry has no HTTP
surface; if one appears, conformance goes to `api-reviewer`.
