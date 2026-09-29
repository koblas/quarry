# Specification: Phase 2a — read foundation (`status`, `accounts`, `sql`)

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: Make quarry's store readable. `quarry status`, `quarry accounts` and `quarry sql` read the
store sync builds, through one locked-down read-only open, and set the `--json`, refusal and exit-code
contract every later Phase 2 reporting command (`spend`, `cashflow`, `findings`, `recurring`, `anomalies`)
reuses.

**Out of Scope**: findings, config file, FX / `--currency` (2f), `import_runs` history across rebuilds and
snapshot listing/pruning (2c), CSV output (2d), `spend`/`cashflow` (2b), `recurring`/`anomalies` (2e),
`networth` (moved to Phase 4), `export` (dropped — `sql --csv` in 2d covers it). Snapshots keep
accumulating (~200 MB each) until 2c — a known gap.

**Business Rules**: read commands never touch Quicken; the store is opened read-only with external access,
extension loading and configuration changes turned off; amounts are reported in each account's native
currency, never converted or summed across currencies.

## Business Rules & Invariants
- P2a-1: Every read command opens the store through one shared open function that (a) refuses a missing
  store (R1), (b) checks `store_info.format_version` once (R2), (c) classifies open/read faults (R3), and (d)
  applies the read lockdown: `access_mode=READ_ONLY`, `enable_external_access=false`,
  `autoload_known_extensions=false`, `autoinstall_known_extensions=false`, then `lock_configuration=true` last.
- P2a-2: One shared store-refusal builder produces R1–R3 for `status`, `accounts` and `sql`.
- P2a-3: Analysis rules live in DuckDB views created in the store's DDL at build time, owned by one Go
  package; front ends call Go functions that only add parameters. 2a adds one view, `v_account_balances`.
  `quarry sql` is plain passthrough.
- P2a-4: `store_info(format_version INTEGER NOT NULL, quarry_version VARCHAR NOT NULL, built_at TIMESTAMP NOT
  NULL)`, one row, `format_version` = 2. A store without the table counts as format 1.
- P2a-5: `import_runs` gains `snapshot_taken_at TIMESTAMP`, `source_path VARCHAR` (the manifest's recorded
  source), `balances_never_reconciled BIGINT`, `investment_accounts BIGINT`, `transfers_paired BIGINT`,
  `transfers_cross_currency BIGINT`. Invariant: `transfers_rows = transfers_paired + transfers_one_sided`;
  the importer asserts it before writing. (Copy ruling round 2.)
- P2a-6: Account balance = sum of the account's transactions dated on or before today (local date);
  future-dated transactions excluded. Brokerage and retirement accounts have no balance (`not imported` /
  `null`) because investment transactions are not imported.
- P2a-7: `categories.kind` stays `income / expense / system`; transfers are structural (`transfers`,
  `splits.transfer_account_id`). The PRD data-model row is corrected accordingly.
- P2a-8: Paths are `~`-abbreviated in human output and stderr, absolute in `--json`. Counts go through
  `internal/platform/humanize`. Every refusal is one line on stderr with stdout empty. TTY and piped output
  are the same bytes.
- P2a-10: `store_info.quarry_version` = `debug.ReadBuildInfo().Main.Version`, or the literal `(devel)` when
  there is no build info or the version is empty — never an empty string. Shown only in `status --json`.
- P2a-11: `quarry sql`'s error classifier checks `ctx.Err()` first (Q4); Q1 matches DuckDB
  `ErrorTypeInvalidInput` **and** a message containing `read-only mode` — any other InvalidInput (e.g. `SET`
  after the configuration lock) is Q3. `CREATE TEMP TABLE`/`VIEW` succeed (in-memory) and are acceptable.
- P2a-9: A usage error's hint names the matched command path (`Run 'quarry sql --help' for usage.`); sync's
  existing U4 text stays byte-identical.

---

## Triage Brief

- Only command today is `sync` (`internal/cli/root.go:9-27`); persistent `--json` on root; exit codes via
  `UsageError`/runtime error (`internal/cli/sync.go:14-20`, `internal/cli/errors.go`, `internal/cli/run.go`
  `Execute`). The 0/1/2 plumbing and `cli.ServerFactory` wiring (`cmd/quarry/run.go:39-76`) are prior art to
  reuse.
- Store DDL `internal/store/duckstore/schema.go:6-89`: accounts (type, currency, institution, closed, active
  — no `hidden`), categories (parent_id, full_path, kind ∈ {income, expense, system} per
  `internal/importer/categories.go:14`), payees, tags, transactions (status uncleared/cleared/reconciled,
  cheque), splits (transfer_account_id), transfers (cross_currency), split_tags, import_runs. No views, no
  `store_info`.
- `import_runs` written once per build, id hardcoded 1 (`internal/importer/importer.go` `newImportRun`,
  `importRunID`); appended last in `duckstore.build`.
- No production read of `quarry.duckdb`. `internal/platform/duckdb/duckdb.go:70-87` `OpenReadOnly(ctx, path)`
  exists (`?access_mode=READ_ONLY`, `SetMaxOpenConns(1)`), unused outside its tests — the smallest seam.
- `duckstore.Replace` builds under a unique partial name and renames over `quarry.duckdb`; it never opens the
  final path, so a concurrent reader sees the old inode. STATE.md trap: DuckDB `InstanceCache` refuses a
  second connection to one path with a different config while the first is open (in-process only).
- Phase 1 phrase functions for Rows/Balances/Splits/Transfers live in `internal/cli/render.go`
  (`rowsPhrase`, `balancesPhrase`, `splitsPhrase`, …); `formatMoney`, `humanize`.
- `v9fixture` can express accounts of every type and currency, multi-year and future dates, closed/inactive
  accounts; `Builder.WriteBundle` feeds `cmd/quarry` end-to-end tests.
- **Already exists — do not re-plan:** exit-code plumbing, `--json` root flag, `formatMoney`, `humanize`,
  Phase 1 phrase functions, `duckdb.OpenReadOnly`, `homepath` `~` abbreviation, `runWith` test seam.

## Product Verdict

**SHIP WITH CHANGES** (Phase 2 as a whole). Accepted by the user 2026-09-29:
1. Phase 2 splits into ordered slices, each its own spec: 2a read-foundation (this), 2b spending (carries
   the "matches Quicken reports, 2 years" gate), 2c snapshots + config + carry-forward, 2d findings, 2e
   recurring + anomalies, 2f fx.
2. `networth` (and `v_balances_daily`, `v_net_worth`) moves to Phase 4.
3. `export` dropped; `sql --csv` (2d) covers it.
4. Duplicates are owned by `findings` only; `anomalies` = unusual amounts.
5. PRD `categories.kind` row corrected to `income / expense / system`.
6. Native currency only until 2f; `--currency` does not exist in 2a–2e.
7. Analysis rules = DuckDB views built with the store (read-only connections cannot create persistent views,
   so read-time views would be invisible to `quarry sql`).

**2a manual check**: the user compares `quarry accounts --all` with Quicken's account-list balances for every
non-investment account, to the cent. Differences are 2a defects or a changed `v_account_balances` rule.

## Surface & Copy

Implement verbatim.

### Root Long
```
quarry copies the Quicken Classic for Mac file you have open into a local,
read-only snapshot, rebuilds its own store from that snapshot, and checks the
store against Quicken's balances. Every other command reads that store;
quarry never writes to the Quicken file.
```

### `status`
- Short: `Show which snapshot the store was built from and what it holds`
- Long:
```
Show the store quarry's commands read: the snapshot it was built from, when
that snapshot was taken, the Quicken file it came from, the dates its
transactions cover, and the checks sync ran when it built the store.

status reads only quarry's store; it never looks at Quicken. Run quarry sync
to bring the store up to date.
```
- stdout (exit 0):
```
Store     ~/Library/Application Support/quarry/quarry.duckdb
Snapshot  20260927T143005Z, taken 2026-09-27 10:30 EDT (2 days ago)
Source    ~/Documents/Home.quicken
Dates     2003-01-04 to 2026-09-26
Rows      18,204 transactions, 21,977 splits, 3,141 transfers, 1,873 payees, 312 categories, 14 tags
Balances  35 accounts match Quicken's last reconciled balance; 3 never reconciled and 4 investment accounts not checked
Splits    all 18,204 transactions equal the sum of their splits
Transfers 3,112 paired, 29 one-sided
```
- Rows, Balances, Splits and Transfers reuse Phase 1's phrase functions and every singular/zero form, values
  read from `import_runs`. Rows' transfers count is all transfer rows (paired + one-sided, `Counts.Transfers`);
  the human Transfers line omits cross-currency, like sync's. Rows gets the `; N investment transactions not imported` clause exactly as in sync.
- Time local, `2006-01-02 15:04 MST`. Age: `just now` < 1 min; `N minutes ago` < 60 min; `N hours ago` < 48 h;
  `N days ago` after. Singular `1 minute ago`, `1 hour ago`.
- Zero transactions: `Dates     no transactions`; JSON `"dates":{"first":null,"last":null}`.
- One-sided = 0: `Transfers 3,112 paired`; none paired: Phase 1's `none` form.
- Built with `--from`: Source is the manifest's recorded source; Snapshot is that snapshot's `taken_at`.
- Stale store: age only, no warning.
- `snapshot_taken_at` NULL: `Snapshot  20260927T143005Z, time taken not recorded in its manifest` (no age, never
  parsed from the ID, nothing on stderr); JSON `"taken_at":null` (key kept, never `""`).
- `source_path` NULL or `""` (whitespace-only prints as-is): `Source    not recorded in the snapshot's manifest`;
  JSON `"source":null`. Both NULL: both lines print.
- Negative age (taken_at ahead of the clock): `just now`.
- `--json`:
```json
{"store":{"path":"…","format_version":2,"quarry_version":"(devel)","built_at":"2026-09-27T14:31:02Z","rows":{…the same 8 keys as sync's store.rows, "transfers":3141…}},
 "snapshot":{"id":"20260927T143005Z","path":"…","taken_at":"2026-09-27T14:30:05Z","source":"/Users/…/Home.quicken","sha256":"…"},
 "dates":{"first":"2003-01-04","last":"2026-09-26"},
 "balances":{"checked":35,"never_reconciled":3,"investment_accounts":4},
 "splits":{"checked":18204},
 "transfers":{"paired":3112,"cross_currency":41,"one_sided":29},
 "not_imported":{"investment_transactions":1605},
 "warnings":[]}
```

### `accounts`
- Short: `List accounts with their current balances`
- Long:
```
List the accounts in quarry's store with each one's balance in its own
currency: the sum of its transactions dated today or earlier. Closed
accounts are left out unless --all is given.

Brokerage and retirement accounts show "not imported": quarry does not
import investment transactions yet, so it cannot compute their balance.
```
- Flag `--all`, help `include closed accounts`.
- stdout (exit 0): header row, rows sorted by name ignoring case, then name, then source id (`ORDER BY lower(name), name, source_id`), columns two spaces apart, Balance
  right-aligned via `formatMoney`, no totals row:
```
Account        Type         Currency       Balance  Status
Chequing       chequing     CAD          12,345.67
RRSP           retirement   CAD       not imported
US Chequing    chequing     USD           8,310.00
Visa Infinite  credit_card  CAD          -1,204.17  closed
```
- Status: empty for open+active; `inactive` for open, not active; `closed` for closed, whether active or not (only with `--all`).
- A non-investment account with no transactions dated today or earlier: Balance `0.00`, JSON `"0.00"` (`null` means only brokerage/retirement).
- All accounts closed, no `--all` (exit 0, stdout header only, or `accounts:[]` under `--json`); the stderr line
  prints in both modes and the same text without `quarry: ` goes in `warnings[]`:
  - N ≥ 2: `quarry: all 3 accounts are closed; pass --all to list them`
  - exactly 1: `quarry: the only account is closed; pass --all to list it`
  - zero accounts in the store: no line, `accounts:[]`, `warnings:[]`. `--all` given: no note.
- `--json`: `{"as_of":"2026-09-29","accounts":[{"id","name","type","currency","institution","closed","active","balance"}],"warnings":[]}` —
  `balance` 2-decimal string or `null` (brokerage/retirement); `institution` may be `null` (SQL NULL or `""`); `as_of` today's local
  date, read from DuckDB's `current_date` in the same query as the balances; `warnings` always present.

### `sql`
- Use: `sql <query>`; Short: `Run a read-only SQL query against quarry's store`
- Long:
```
Run one SQL query against quarry's store and print the result. The store is
opened read-only: a query cannot change it, read or write other files, or
load extensions.

Pass the query as one quoted argument, or - to read it from stdin. Amounts
are DECIMAL(18,2) in each account's own currency; negative is money leaving
the account. Transfers between your own accounts are in the transfers table
and splits.transfer_account_id, never in a category kind. List the tables
and views with: quarry sql "SHOW TABLES"

At most --limit rows are printed (500 unless set); when there are more,
quarry says so on stderr. --limit 0 prints every row.
```
- Example:
```
  quarry sql "SELECT name, currency FROM accounts WHERE NOT closed"
  quarry sql --limit 0 --json - < monthly.sql
```
- Flag `--limit`, help ``print at most `n` rows (0 prints every row)``, default 500 (renders `--limit n ... (default 500)`).
- Table output: header of column names, then rows. Each value prints as DuckDB's own text for it (what
  `CAST(x AS VARCHAR)` gives), no digit grouping — every scalar, ±infinity, LIST, ARRAY, MAP and UNION. STRUCT
  prints `{'a': 1, 'b': x}` with fields in the order the column type declares (parsed from the type name,
  quoted identifiers and nesting handled); if the type name cannot be parsed, fields sorted by name — the only
  declared divergence (see Two declared divergences below). NULL → `NULL`. Newline, tab and CR inside a value → `\n`, `\t`, `\r`; nothing else is
  escaped. Numeric columns (and their headers) right-aligned; other headers left-aligned; no padding after the last column;
  a value's own spaces, leading or trailing, print as they are.
  Zero rows → header only, exit 0.
- The table is for reading: a NULL and the string `NULL` print the same, and so do an escaped newline and a
  literal `\n`. `--json` is the exact form.
- Two declared divergences from DuckDB's text: the STRUCT sorted-fields fallback, and `TIME '24:00:00'`, which
  prints `00:00:00` (the driver collapses it) in both table and `--json` output.
- TIMESTAMPTZ prints in this Mac's local time zone, as DuckDB's text does by default (e.g.
  `2026-09-29 10:00:00-04`). `SET TimeZone` cannot change it (the configuration is locked — Q3); for another
  zone, convert in the query, e.g. `timezone('UTC', ts)`. `--json` stays RFC3339Nano UTC.
- quarry's own tables and views only use types both renderers print, so `SELECT *` over any of them never gets Q5.
- `--json`: `{"columns":[{"name","type"}],"rows":[[…]],"row_count":N,"limit":500,"truncated":false,"warnings":[]}`.
  DECIMAL and HUGEINT → strings; other integers, DOUBLE, BOOLEAN native; FLOAT native encoded from float32;
  DOUBLE/FLOAT NaN/±Inf → `"nan"`, `"inf"`, `"-inf"` (never `null`); `-0.0` native; DATE `YYYY-MM-DD`;
  TIMESTAMP, TIMESTAMP_S/_MS/_NS, TIMESTAMPTZ → RFC3339Nano UTC with `Z`; DATE/TIMESTAMP ±infinity →
  `"infinity"`/`"-infinity"`; NULL `null`; everything else (TIME, TIMETZ, INTERVAL, UUID, BLOB, BIT, ENUM, LIST,
  ARRAY, MAP, STRUCT, UNION) DuckDB's text as a string — not nested JSON in 2a.
- Truncation: fetch `limit+1`. More rows → stderr
  `quarry: warning: showing the first 500 rows; the query returned more; pass --limit 0 to print every row`;
  same text without prefix in `warnings[]`; `truncated: true`; exit 0. Exactly `limit` rows → not truncated.
  `--limit 0` never truncates.
- Multiple statements: DuckDB's own behaviour, not a documented promise.

### Refusals and outcomes
| # | Condition | stderr | Exit |
|---|---|---|---|
| R1 | no store | `quarry: no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it` | 1 |
| R2 | `store_info` missing, not exactly one row, or `format_version` ≠ this binary's | `quarry: the store at ~/Library/Application Support/quarry/quarry.duckdb was built by another version of quarry; run quarry sync --from 20260927T143005Z to rebuild it` (ID = basename without extension of `import_runs.snapshot_path`; if unreadable: `…; run quarry sync to rebuild it`) | 1 |
| I1 | status/accounts interrupted (SIGINT/SIGTERM); `ctx.Err()` checked before any R1–R3 classification (sql keeps Q4 byte-identical, incl. a cancel during the open) | `quarry: <cmd> interrupted` | 1 |
| R3a | store is not a DuckDB file / corrupt (`not a valid DuckDB database file`) | `quarry: cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: the file is not a DuckDB database; run quarry sync to rebuild it` | 1 |
| R3b | permission (`fs.ErrPermission` / `Permission denied`) | `quarry: cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: permission denied; run quarry sync to rebuild it` | 1 |
| R3c | lock (`Could not set lock on file`) | `quarry: cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: another program has it open for writing; close that program and run the command again` | 1 |
| R3 | any other open/read fault (G1: with a `*fs.PathError` in the tree, `<reason>` = `PathError.Err.Error()` only; with no `*duckdb.Error`, first line of `err.Error()` with the same strip and `~` replacement; empty after stripping → `unknown error`) | `quarry: cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb: <reason>; run quarry sync to rebuild it` — `<reason>` = first line of `(*duckdb.Error).Msg`, DuckDB type prefix (`^[A-Za-z ]+ Error: `) removed, every occurrence of the store path (as given and after `filepath.EvalSymlinks`) replaced by its `~` form | 1 |
| Q1 | write statement (DuckDB read-only mode error) | `quarry: quarry sql only reads the store; change the data in Quicken and run quarry sync` | 1 |
| Q2 | external access or extension refused (DuckDB Permission Error) | `quarry: quarry sql reads only quarry's store; other files, databases and extensions are turned off` | 1 |
| Q3 | any other query error, incl. `SET` after the lock (`quarry: query failed: Invalid Input Error: Cannot change configuration option "enable_external_access" - the configuration has been locked`) and `SELECT * FROM '/etc/hosts'` (Catalog Error) | `quarry: query failed: <first line of DuckDB's message>` — verbatim, type prefix kept (`Binder Error: …`); a path in it is the user's own query text and is not `~`-abbreviated (P2a-8 covers paths quarry prints) | 1 |
| Q4 | sql interrupted (SIGINT/SIGTERM) | `quarry: query interrupted` | 1 |
| Q5 | a result value quarry cannot print: a Go type the renderer does not know (never `%v`); a JSON column (the driver returns it decoded, so DuckDB's text is lost); or a type the driver refuses (`unsupported data type: <T>: index: <i>` → name from the column list at `<i>`, type `<T>`; unmatched message falls back to Q3; owner SCENARIO-12's classifier, checked before Q3) | `quarry: cannot print column "<name>" of type <DuckDB type>; cast it in the query, e.g. CAST(<name> AS VARCHAR)` | 1 |
| O2 | stdout write fails (EPIPE keeps Go's default death) | `quarry: cannot write the result to stdout: <OS reason>` | 1 |
| H1 | `$HOME` unset or unresolvable (status/accounts/sql/sync) | `quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry <cmd> again` — one builder taking the command name; sync's text stays byte-identical; help and usage errors never need `$HOME` | 1 |
| U5 | sql: no argument, empty/whitespace query, `-` with empty stdin, or a query DuckDB answers with its empty-query error (`;`, `-- note`) — classified after the `ctx.Err()` check; with no store, `sql ";"` gets R1 because only DuckDB can tell it is empty | `quarry: sql needs a query; pass it as one quoted argument, or - to read it from stdin` | 2 |
| U6 | sql: more than one argument | `quarry: sql takes one query; quote it as one argument` | 2 |
| U7 | `--limit` < 0 | `quarry: --limit must be 0 or more; 0 prints every row` | 2 |
| U8 | status/accounts given positional arguments | `quarry: <cmd> takes no arguments` | 2 |
| U9 | cobra-native usage error | `<cobra text>; Run 'quarry <matched command path> --help' for usage.` e.g. `unknown flag: --bogus; Run 'quarry sql --help' for usage.`; root: `unknown command "spend" for "quarry"; Run 'quarry --help' for usage.` sync's U4 stays byte-identical | 2 |

Shared open classifier order (one decision point): (1) `ctx.Err()` → I1/Q4; (2) stat not-exist → R1; (3) open fault then re-stat not-exist → R1 (store vanished between stat and open; never match the driver's "does not exist" text); (4) R3a/R3b/R3c; (5) R2 (catalog, row count, format_version); (6) R3 fallback.

`--json` plus any refusal: stdout empty. A sync running concurrently: readers see the old store, no message.

---

## Sizing (architect pass, 2026-09-29)

21 scenarios → 10 runs, 0 splits. Execution order (top to bottom):

| Run | Absorbs | Size | Introduces |
|---|---|---|---|
| SCENARIO-01 | — | M | `store_info` + format const 2, 6 `import_runs` columns, `SnapshotRef{TakenAt, Source}`, quarry-version wiring |
| SCENARIO-02 | — | L | `internal/report` feature package (all Phase 2 read commands), reader port, duckstore read side, driver-free read types in `internal/store`, cli/`runWith` widening (report factory + stdin, once), ADR for duckstore owning the read side |
| SCENARIO-03 | — | S | status JSON renderer |
| SCENARIO-04 | 05 | L | `v_account_balances` view in store DDL, Accounts query, `accounts` command + table |
| SCENARIO-07 | 06 | M | accounts JSON, all-closed note + `warnings[]` |
| SCENARIO-08 | — | L | `sql` passthrough, driver-free query-result shape, table renderer |
| SCENARIO-12 | 13, 14, 20 | M | read lockdown in `platform/duckdb.OpenReadOnly` (one DSN for every read-only open), Q1–Q4 classifier; **test-first** (write-safety guard) |
| SCENARIO-15 | 16, 17 | M | shared open: `os.Stat` (R1) → catalog check of `store_info` (R2) → tagged open fault (R3a–c) |
| SCENARIO-09 | 10 | M | typed JSON encoding; 10's outline rows run through `--json` (`truncated` only observable there) |
| SCENARIO-18 | 11, 19, 21 | M | Args validators U5–U8, stdin `-`, U9 matched-path hint via `ExecuteC`, O2 shared stdout-write check |

Probe-verified design facts (DuckDB v1.5.5): InstanceCache refuses a differently-configured open of a path
already open in-process — fixtures writing at the final path must Close first; DSN settings apply before open
so `lock_configuration` ordering holds by construction; `current_date` follows process TZ; a cancelled ctx
interrupts a running query (`errors.Is(err, ctx.Err())`), so SCENARIO-20 is testable via ctx or
`syscall.Kill(getpid, SIGINT)`; a missing `store_info` must be detected through the catalog, not a bare
SELECT.

---

## Scenarios (Gherkin)

```gherkin
Scenario: SCENARIO-01 — sync stamps the store's format and records what status needs
  Given a Quicken bundle that imports cleanly
  When I run quarry sync
  Then the store has one store_info row with format_version 2, quarry's version and the build time
  And its import_runs row records snapshot_taken_at, source_path, balances_never_reconciled, investment_accounts, transfers_paired and transfers_cross_currency
```

```gherkin
Scenario: SCENARIO-02 — status describes the store and the checks sync ran
  Given a store built by quarry sync from a known snapshot
  When I run quarry status
  Then stdout shows the Store, Snapshot (with local time and age), Source, Dates, Rows, Balances, Splits and Transfers lines
  And the exit code is 0
```

```gherkin
Scenario: SCENARIO-03 — status --json returns the store description as a document
  Given a store built by quarry sync from a known snapshot
  When I run quarry status --json
  Then stdout is one JSON document with store, snapshot, dates, balances, splits, transfers, not_imported and warnings
  And paths are absolute and times are RFC3339 UTC
```

```gherkin
Scenario: SCENARIO-04 — accounts lists open accounts with native-currency balances
  Given a store with open CAD and USD accounts, an inactive account, a retirement account, a closed account and a future-dated transaction
  When I run quarry accounts
  Then stdout lists the open accounts by name with each balance in its own currency
  And the retirement account shows "not imported"
  And the future-dated transaction is not in any balance
  And the closed account is not listed
```

```gherkin
Scenario: SCENARIO-05 — accounts --all includes closed accounts
  Given a store with open and closed accounts
  When I run quarry accounts --all
  Then the closed accounts are listed with Status "closed"
```

```gherkin
Scenario: SCENARIO-06 — accounts says how to see accounts when every one is closed
  Given a store whose accounts are all closed
  When I run quarry accounts
  Then stdout is the header only
  And stderr says "quarry: all 3 accounts are closed; pass --all to list them"
  And the exit code is 0
```

```gherkin
Scenario: SCENARIO-07 — accounts --json returns accounts as a document
  Given a store with a checking account, a retirement account and an account with no institution
  When I run quarry accounts --json
  Then stdout has as_of and an accounts list with balance as a 2-decimal string, null for the retirement account, and institution null where absent
```

```gherkin
Scenario: SCENARIO-08 — sql prints a query result as a table
  Given a built store
  When I run quarry sql with a query returning text, numbers, NULLs and values containing newlines and tabs
  Then stdout is a header and rows with NULL printed as NULL, control characters escaped and numeric columns right-aligned
```

```gherkin
Scenario: SCENARIO-09 — sql --json returns typed values
  Given a built store
  When I run quarry sql --json with a query returning DECIMAL, HUGEINT, INTEGER, DOUBLE, BOOLEAN, DATE, TIMESTAMP and NULL values
  Then stdout has columns with names and types and rows with each value encoded per the Surface & Copy rules
```

```gherkin
Scenario Outline: SCENARIO-10 — sql caps the rows it prints
  Given a built store and a query returning <returned> rows
  When I run quarry sql --limit <limit>
  Then <printed> rows are printed, truncated is <truncated>, and the warning is <warning>

  Examples:
    | returned | limit | printed | truncated | warning |
    | 3        | 2     | 2       | true      | printed |
    | 2        | 2     | 2       | false     | absent  |
    | 3        | 0     | 3       | false     | absent  |
```

```gherkin
Scenario: SCENARIO-11 — sql reads its query from stdin
  Given a built store
  When I run quarry sql - with the query on stdin
  Then the query's result is printed
```

```gherkin
Scenario: SCENARIO-12 — sql refuses to change the store
  Given a built store
  When I run quarry sql with a CREATE TABLE statement
  Then stderr is Q1, stdout is empty, the exit code is 1 and the store file's bytes are unchanged
```

```gherkin
Scenario Outline: SCENARIO-13 — sql cannot reach other files, databases, extensions or settings
  Given a built store
  When I run quarry sql "<statement>"
  Then the statement is refused with exit code 1 and stdout empty

  Examples:
    | statement                          |
    | SELECT * FROM read_csv('/etc/hosts') |
    | COPY (SELECT 1) TO '<tmp file>'     |
    | ATTACH '<tmp db>' AS other          |
    | INSTALL httpfs                      |
    | SET enable_external_access=true     |
```

```gherkin
Scenario: SCENARIO-14 — sql reports a bad query
  Given a built store
  When I run quarry sql with a query naming a column that does not exist
  Then stderr is "quarry: query failed: <first line of DuckDB's message>" and the exit code is 1
```

```gherkin
Scenario Outline: SCENARIO-15 — read commands refuse when there is no store
  Given quarry has never synced
  When I run quarry <command>
  Then stderr is R1, stdout is empty and the exit code is 1

  Examples:
    | command  |
    | status   |
    | accounts |
    | sql "SELECT 1" |
```

```gherkin
Scenario: SCENARIO-16 — read commands refuse a store built by another version
  Given a store without store_info, whose import_runs names snapshot 20260927T143005Z
  When I run quarry status
  Then stderr is R2 naming "quarry sync --from 20260927T143005Z" and the exit code is 1
```

```gherkin
Scenario: SCENARIO-17 — read commands refuse a store they cannot read
  Given a store file that is not a DuckDB database
  When I run quarry accounts
  Then stderr is R3a naming the store, and the exit code is 1
```

```gherkin
Scenario Outline: SCENARIO-18 — read commands reject bad usage
  Given any state
  When I run quarry <args>
  Then stderr is <refusal> and the exit code is 2

  Examples:
    | args                 | refusal |
    | sql                  | U5      |
    | sql "   "            | U5      |
    | sql - (empty stdin)  | U5      |
    | sql ";"              | U5      |
    | sql "-- note"        | U5      |
    | sql "SELECT 1" extra | U6      |
    | sql --limit -1 "SELECT 1" | U7 |
    | status extra         | U8      |
    | accounts extra       | U8      |
```

```gherkin
Scenario Outline: SCENARIO-19 — a usage error's hint names the command it came from
  Given any state
  When I run quarry <args>
  Then stderr ends with "Run '<path> --help' for usage." and the exit code is 2

  Examples:
    | args            | path       |
    | sql --bogus     | quarry sql |
    | status --bogus  | quarry status |
    | spend           | quarry     |
```

```gherkin
Scenario: SCENARIO-20 — interrupting sql stops the query
  Given a built store and a long-running query
  When quarry sql receives SIGINT
  Then stderr is "quarry: query interrupted" and the exit code is 1
```

```gherkin
Scenario: SCENARIO-21 — a read command reports a failed stdout write
  Given a built store
  When quarry accounts cannot write to stdout
  Then stderr is "quarry: cannot write the result to stdout: <OS reason>" and the exit code is 1
```

---

## BDD Acceptance Progress
- [x] SCENARIO-01: sync stamps the store's format and records what status needs — `cmd/quarry/run_store_info_test.go` `Test_run_stamps_the_store_with_its_format_and_the_build`
- [x] SCENARIO-02: status describes the store and the checks sync ran — `cmd/quarry/run_status_test.go` `Test_run_status_describes_the_store_sync_built`
- [x] SCENARIO-03: status --json returns the store description as a document — `cmd/quarry/run_status_json_test.go` `Test_run_status_json_describes_the_store_sync_built`
- [x] SCENARIO-04: accounts lists open accounts with native-currency balances — `cmd/quarry/run_accounts_test.go` `Test_run_accounts_lists_open_accounts_with_their_balances`
- [x] SCENARIO-05: accounts --all includes closed accounts — delivered by SCENARIO-04 — `cmd/quarry/run_accounts_test.go` `Test_run_accounts_all_lists_closed_accounts`
- [x] SCENARIO-06: accounts says how to see accounts when every one is closed — delivered by SCENARIO-07 — `cmd/quarry/run_accounts_test.go` `Test_run_accounts_says_how_to_list_them_when_every_account_is_closed`
- [x] SCENARIO-07: accounts --json returns accounts as a document — `cmd/quarry/run_accounts_json_test.go` `Test_run_accounts_json_returns_accounts_as_a_document`
- [x] SCENARIO-08: sql prints a query result as a table — `cmd/quarry/run_sql_test.go` `Test_run_sql_prints_the_query_result_as_a_table`
- [ ] SCENARIO-09: sql --json returns typed values
- [ ] SCENARIO-10: sql caps the rows it prints
- [ ] SCENARIO-11: sql reads its query from stdin
- [x] SCENARIO-12: sql refuses to change the store — `cmd/quarry/run_sql_test.go` `Test_run_sql_refuses_to_change_the_store`
- [x] SCENARIO-13: sql cannot reach other files, databases, extensions or settings — delivered by SCENARIO-12 — `cmd/quarry/run_sql_test.go` `Test_run_sql_refuses_to_write_another_file`
- [x] SCENARIO-14: sql reports a bad query — delivered by SCENARIO-12 — `cmd/quarry/run_sql_test.go` `Test_run_sql_reports_a_bad_query`
- [x] SCENARIO-15: read commands refuse when there is no store — `cmd/quarry/run_read_refusals_test.go` `Test_run_read_commands_refuse_when_there_is_no_store`
- [x] SCENARIO-16: read commands refuse a store built by another version — delivered by SCENARIO-15 — `cmd/quarry/run_read_refusals_test.go` `Test_run_status_refuses_a_store_built_by_another_version`
- [x] SCENARIO-17: read commands refuse a store they cannot read — delivered by SCENARIO-15 — `cmd/quarry/run_read_refusals_test.go` `Test_run_accounts_refuses_a_store_that_is_not_a_duckdb_file`
- [ ] SCENARIO-18: read commands reject bad usage
- [ ] SCENARIO-19: a usage error's hint names the command it came from
- [x] SCENARIO-20: interrupting sql stops the query — delivered by SCENARIO-12 — `cmd/quarry/run_sql_test.go` `Test_run_sql_reports_a_query_interrupted_by_sigint`
- [ ] SCENARIO-21: a read command reports a failed stdout write
