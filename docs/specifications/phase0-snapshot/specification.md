# Specification: Phase 0 — Verified Quicken snapshot and ported prior art

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry sync` takes a safe, read-only snapshot of the open Quicken Classic for Mac v9 database with SQLite's online backup API and verifies its schema against an embedded reference ported from dweekly/quicken-mac-mcp, so Phase 1's importer builds on verified ground. The phase gate is David running `quarry sync` on his real v9 file and getting exit 0 (after correcting the embedded reference if the first run reports drift).

**Secondary Goals**:
- CLI skeleton and the binary-wide exit-code contract (0 success incl. warnings, 1 failure, 2 usage) with one-line stderr errors.
- Snapshot files are private (`0600` files, `0700` dirs) under `~/Library/Application Support/quarry/`.
- Prior art ported as frozen reference docs with MIT notices preserved.

**Out of Scope**: importer, DuckDB store, FX fetch, findings, store swap, `quarry sync --from`, `quarry status`, snapshot retention, config file / env var, process-list check for Quicken, column-type comparison, wiring SQL recipes or the skill to any command.

**Business Rules**: see below.

## Business Rules & Invariants

- **BR-1 Live file is read-only.** The live `<bundle>/data` is opened with `mode=ro` only. Never `immutable=1`, never a `journal_mode` change, never a checkpoint, never any pragma write. The live file gets one probe read (encryption detection) plus the backup, nothing else. Integrity check, account count, hash and schema read all run on the snapshot.
- **BR-2 Backup API, not a byte copy.** The snapshot is taken with SQLite's online backup API so pages still in the live `-wal` are included.
- **BR-3 Fixture models an open Quicken file.** The synthetic live-file fixture is a WAL-mode database held open by a second connection with uncheckpointed writes (`-wal` and `-shm` present), matching Quicken's state while the file is open. At least one row exists only in the WAL; SCENARIO-01a asserts the snapshot contains it.
- **BR-4 Fixture schema comes from the reference.** The fixture's schema is generated from the embedded `reference.sql`, so the SCENARIO-01a Given holds by construction. hardkoded's `fixture-schema.sql` / `fixture-data.sql` supply row data where useful and serve as the port-time cross-check; its differences from dweekly's DDL are recorded in `docs/prior-art/README.md` and in *Reference reconciliation* below.
- **BR-5 One reference truth.** Only the embedded `reference.sql` (in the Quicken v9 package, pinned to an upstream dweekly commit) is ever corrected. `docs/prior-art/**` stays frozen upstream text.
- **BR-6 Schema scope.** Every table whose name starts with `Z`, including `Z_PRIMARYKEY` and `Z_<n>` join tables, is compared. Excluded: `Z_METADATA`, `Z_MODELCACHE`, `sqlite_*`, non-`Z` tables. Names only; types are not compared.
- **BR-7 Fingerprint.** `sha256:<hex>` over the UTF-8 bytes of `TABLE.COLUMN\n` for every in-scope table and column, names exactly as `pragma table_info` reports them, sorted bytewise. The same algorithm runs over the reference; exact match ⇔ equal fingerprints.
- **BR-8 Order of work.** 1 resolve path → 2 open read-only and probe → 3 back up to a partial file → 4 integrity check → 5 `ZACCOUNT` exists with ≥1 row → 6 SHA-256 of file → 7 schema diff → 8 write manifest → 9 rename snapshot. Nothing is created in the snapshots directory before the probe succeeds. A snapshot is accepted only if its manifest exists.
- **BR-9 Outcome policy.** Rejected snapshot (closed, busy, integrity, no accounts, not Quicken, I/O) → nothing left on disk. Missing tables/columns → exit 1, snapshot and manifest **kept**. Extras only → exit 0 with a warning.
- **BR-10 Manifest = `--json`.** The manifest file is byte-for-byte the `--json` document.
- **BR-11 R14 at unit level.** Disk-full / write error during backup or manifest write (R14) is tested at unit level through a fake on the snapshot-destination port, not in an acceptance test. The backup writes through SQLite's file layer, so the fault is injected via the destination port, not an `io.Writer`.
- **BR-12 Home injection.** `~/Documents` discovery, `~/` expansion and `~/Library/Application Support/quarry` all derive from the home directory (`os.UserHomeDir()` in production), injected so tests point `HOME` at a temp dir. No data-directory override flag.
- **BR-13 Fold fallback for SCENARIO-03.** SCENARIO-03 folds into SCENARIO-01a only if, with the BR-3 fixture, `mode=ro` leaves the bundle's file set, data bytes and mtime unchanged *and* removing `mode=ro` makes the test fail. If the test cannot pass against correct `mode=ro` code, the 01a developer stops and reports; SCENARIO-03 then gets its own run immediately after 01a.

### Reference reconciliation

_To be filled by the prior-art pre-step before SCENARIO-01a's architect runs: upstream commit SHAs, which source wins per disagreement, and the full dweekly-vs-hardkoded Z-table/column diff._

### Phase 1 obligations (deferred, recorded here)

- Snapshot retention (keep last N, default 12) with N as a config key, not a constant.
- `quarry sync --from <snapshot>`.
- `quarry status`.
- Config-file key for the Quicken path (named when the config file is introduced).
- Busy timeout as config (a named constant in Phase 0).
- Schema fingerprint recorded in `import_runs`.
- Possibly comparing column types.
- Rewrite `sync` help Long when sync gains import/validation.

---

## Triage Brief

- Repo empty of product code: `go.mod` (module `github.com/koblas/quarry`, `go 1.27.1`, deps cobra, pflag, testify, yaml.v3), `devenv.nix` (Go 1.27.1, golangci-lint), `docs/initial-prd.md`. No `cmd/`, no `internal/`. `go build ./...` matches no packages.
- **Caller table:** empty by construction — `cmd/**` and `internal/**` do not exist.
- cgo works in devenv (clang wrapper on PATH, `CGO_ENABLED=1`). Driver: `mattn/go-sqlite3` (direct Backup API wrapper; cgo already mandatory later for duckdb-go).
- **Already exists — do not re-plan:** cobra is the CLI framework (in `go.mod`).
- Testable synthetically: discovery, backup copy, file modes, integrity check, hash, no-accounts reject, closed detection (non-SQLite bytes fixture), schema diff. Real-file verification is a manual, user-only gate — no agent may mark it done.

## Product Verdict

**SHIP WITH CHANGES** (scoping pass). Accepted changes, all folded into this spec:
1. Split missing from unexpected: missing → exit 1, snapshot kept; extras only → exit 0 + warning.
2. Fingerprint algorithm pinned now (BR-7).
3. One reference truth (BR-5); manifest byte-identical to `--json` (BR-10).
4. `--quicken <path>`, not `--file`. No config file or env var in Phase 0.
5. Live-file open constraints as invariant (BR-1).
6. TCC rows R3, R7.
7. Deferred items recorded as Phase 1 obligations.
8. hardkoded `fixture-schema.sql` as port-time cross-check (BR-4).

Prior-art deliverables (pre-step, outside the pipeline, data/docs only): `docs/prior-art/dweekly/` (schema.md, recipes, skill layout) and `docs/prior-art/hardkoded/` (fixture-schema.sql, hygiene list) verbatim and frozen, each file headed with source URL, commit SHA and "MIT, see THIRD_PARTY_NOTICES"; `THIRD_PARTY_NOTICES` at repo root with both copyrights and MIT text; README credit line; `docs/prior-art/README.md` recording dweekly-vs-hardkoded disagreements; embedded `reference.sql` extracted from dweekly `schema.md` `CREATE TABLE` blocks.

---

## Surface & Copy

Implemented verbatim. Error vocabulary for the whole binary: `quarry: <what failed>; <next action>` and `quarry: warning: <what>; <consequence>`. Cobra's default usage dump and error printing are silenced so exactly one line is written.

### Scope rulings

| Item | Ruling |
|---|---|
| Command | `quarry sync` only. No `quarry snapshot`. |
| Configured path | One flag, `--quicken <path>`. quarry expands a leading `~/` itself. |
| Global flags | `--json` only, persistent on root. No `--verbose`, `--version`, colour, progress. |
| Data dir override | None (BR-12). |

### Files on disk

| Item | Value |
|---|---|
| Directories | `~/Library/Application Support/quarry/` and `…/quarry/snapshots/`, created `0700` |
| Snapshot | `snapshots/20260927T143005Z.sqlite` — UTC time the sync started, ISO 8601 basic, no colons, `0600`. Never opened for writing after rename. |
| Same-second collision | Exclusive create; on collision `…Z_2.sqlite`, `…Z_3.sqlite`. `.sqlite`/`.json` names reserved as a pair (same suffix). |
| Partial | `snapshots/.20260927T143005Z.sqlite.partial` (and `.json.partial`). Removed on any failure. Leftovers matching quarry's own `.partial` pattern removed at start of next sync without comment. |
| Manifest | `snapshots/20260927T143005Z.json`, `0600`, byte-for-byte the `--json` document. |
| Hash | SHA-256 of the snapshot file's bytes. |

### Help text

Root:
- Use: `quarry`
- Short: `Snapshot and query Quicken Classic for Mac data locally`
- Long:
```
quarry copies the Quicken Classic for Mac file you have open into a local,
read-only snapshot and checks it against quarry's schema reference. quarry
never writes to the Quicken file.
```

`sync`:
- Short: `Snapshot the open Quicken file and verify its schema`
- Long:
```
Copy the Quicken file's database with SQLite's backup API into
~/Library/Application Support/quarry/snapshots/, check its integrity, and
compare its tables and columns with quarry's reference for Quicken Classic
for Mac v9. A JSON manifest is written next to each snapshot.

Quicken must be running with the file open: it encrypts the database when
the file is closed. quarry only reads the Quicken file; it never writes to it.

Without --quicken, quarry uses the only .quicken file in ~/Documents.
```
- Example: `  quarry sync --quicken ~/Documents/Home.quicken`

Flags (backticks are pflag's placeholder):
- `--quicken`: ``"`path` to the .quicken file to snapshot (default: the only one in ~/Documents)"`` — renders as `--quicken path`.
- `--json` (persistent): `"print the result as JSON on stdout"`

### Success (exit 0)

Human, exact. All to stdout; stderr empty. Paths `~`-abbreviated; counts computed from the reference at runtime.
```
Snapshot  ~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite
Manifest  ~/Library/Application Support/quarry/snapshots/20260927T143005Z.json
Source    ~/Documents/Home.quicken
Size      212.4 MB, 42 accounts
SHA-256   9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
Schema    matches reference dweekly/quicken-mac-mcp@1a2b3c4 (71 tables, 1,042 columns)
```
Formatting: size in decimal MB, one decimal place; `1 account` / `N accounts`; thousands grouped with commas.

**Extras only (exit 0 + warning).** stdout: same block, `Schema` line and rows:
```
Schema    matches reference dweekly/quicken-mac-mcp@1a2b3c4 (71 tables, 1,042 columns), plus 1 table and 2 columns not in it
  + table   ZNEWENTITY
  + column  ZACCOUNT.ZNEWFLAG
  + column  ZTAG.ZCOLOR
```
stderr (W1):
```
quarry: warning: Home.quicken has 1 table and 2 columns that are not in the schema reference; quarry ignores them (listed in 20260927T143005Z.json)
```

### `--json`

Same document to stdout and manifest. `snapshot`, `schema`, `warnings` are permanent top-level keys; Phase 1 adds keys, never removes these.
```json
{
  "snapshot": {
    "path": "/Users/david/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite",
    "manifest": "/Users/david/Library/Application Support/quarry/snapshots/20260927T143005Z.json",
    "source": "/Users/david/Documents/Home.quicken",
    "taken_at": "2026-09-27T14:30:05Z",
    "bytes": 212400000,
    "sha256": "9f86…0a08",
    "accounts": 42
  },
  "schema": {
    "reference": "dweekly/quicken-mac-mcp@1a2b3c4",
    "verified": true,
    "fingerprint": "sha256:…",
    "reference_fingerprint": "sha256:…",
    "missing_tables": [],
    "missing_columns": [],
    "unexpected_tables": [],
    "unexpected_columns": []
  },
  "warnings": []
}
```
Field rules:
- Paths absolute.
- Column entries `{"table":"ZACCOUNT","column":"ZNEWFLAG"}`; table entries bare strings.
- All four lists always present, sorted bytewise (table, then column).
- A missing or unexpected table does not repeat its columns in the column lists.
- `verified` true exactly when `missing_tables` and `missing_columns` are both empty.
- `warnings` holds the stderr warning text without the `quarry: warning: ` prefix.

### Schema mismatch (exit 1)

stdout carries the complete diff: never truncated, sorted deterministically, `-` rows before `+` rows, a missing table listed once without its columns.
```
Snapshot  …/20260927T143005Z.sqlite
Manifest  …/20260927T143005Z.json
Source    ~/Documents/Home.quicken
Size      212.4 MB, 42 accounts
SHA-256   9f86…0a08
Schema    DIFFERS from reference dweekly/quicken-mac-mcp@1a2b3c4: 1 table and 2 columns missing, 1 column not in reference
  - table   ZLOT
  - column  ZCASHFLOWTRANSACTIONENTRY.ZMEMO
  - column  ZSECURITY.ZCUSIP
  + column  ZACCOUNT.ZNEWFLAG
```
stderr (M1):
```
quarry: schema check failed: Home.quicken is missing 1 table and 2 columns that the schema reference expects; the snapshot is kept at ~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite and the diff is in its .json manifest; quarry cannot import this file until its schema reference is updated
```
With `--json`: stdout gets the full document with `"verified": false`; manifest written. The one failure with data on stdout.

### Refusals

All to stderr as one line prefixed `quarry: `. stdout empty in both modes; nothing left on disk.

| # | Condition | Exact stderr | Exit |
|---|---|---|---|
| R1 | No `--quicken`, and no `*.quicken` directory at top level of `~/Documents` (dotfiles ignored) | `quarry: no .quicken file found in ~/Documents; pass one with --quicken <path>` | 1 |
| R2 | More than one | `quarry: found 3 .quicken files in ~/Documents (Business.quicken, Home.quicken, Old.quicken); choose one with --quicken <path>` (all names, sorted) | 1 |
| R3 | `~/Documents` unreadable (TCC or permissions) | `quarry: cannot read ~/Documents: operation not permitted; allow your terminal to access the Documents folder in System Settings > Privacy & Security > Files and Folders, or pass --quicken <path>` | 1 |
| R4 | `--quicken` path does not exist | `quarry: ~/Documents/Hom.quicken does not exist; check the path passed to --quicken` | 1 |
| R5 | Path ends `.qdf`/`.QDF` | `quarry: ~/Documents/Home.QDF is a Quicken for Windows file; quarry reads only Quicken Classic for Mac .quicken files` | 1 |
| R6 | Not a directory, or directory with no regular file named `data` (missing, directory, device). Symlinks followed. | `quarry: ~/Documents/Home.quicken is not a Quicken for Mac file (expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>` | 1 |
| R7 | `data` unreadable (permissions/TCC) | `quarry: cannot read ~/Documents/Home.quicken/data: permission denied; allow your terminal to access the folder in System Settings > Privacy & Security, or check the file's permissions` (OS reason verbatim) | 1 |
| R8 | Probe returns SQLITE_NOTADB (encrypted) | `quarry: ~/Documents/Home.quicken is encrypted, so Quicken does not have it open; open it in Quicken, then run quarry sync again` | 1 |
| R9 | SQLITE_BUSY/LOCKED persists through busy timeout (named constant) | `quarry: Quicken is busy writing ~/Documents/Home.quicken; run quarry sync again in a moment` | 1 |
| R10 | `integrity_check` not `ok` | `quarry: the snapshot of ~/Documents/Home.quicken failed SQLite's integrity check (<first result line>); nothing was kept; quit and reopen the file in Quicken, then run quarry sync again` | 1 |
| R11 | No `ZACCOUNT` table (incl. 0-byte `data`) | `quarry: ~/Documents/Home.quicken is not a Quicken Classic for Mac database (no ZACCOUNT table); pass the right file with --quicken <path>` | 1 |
| R12 | `ZACCOUNT` has 0 rows | `quarry: ~/Documents/Home.quicken has no accounts; nothing was kept; check you have the right file open, or pass it with --quicken <path>` | 1 |
| R13 | Cannot create/write app-support or snapshots dir | `quarry: cannot write to ~/Library/Application Support/quarry/snapshots: permission denied; make the directory writable by your user` (OS reason verbatim) | 1 |
| R14 | Disk full / other write error during backup or manifest | `quarry: cannot write snapshot to ~/Library/Application Support/quarry/snapshots: no space left on device; free disk space, then run quarry sync again` | 1 |
| U1 | Positional argument | `quarry: sync takes no arguments; pass the file with --quicken <path>` | 2 |
| U2 | Unknown flag, `--quicken` without value, unknown command | cobra's message with `quarry: ` prefix, plus `Run 'quarry sync --help' for usage.` No usage dump. | 2 |

### Edge-case rows by output

| Output | Input class | Text / behaviour |
|---|---|---|
| `Source` line | discovered vs `--quicken` | Identical: resolved bundle path, `~`-abbreviated. No "(auto-detected)". |
| `Size` line | 1 / N accounts | `1 account` / `N accounts` |
| `Schema` line | exact match | `matches reference … (T tables, C columns)`. No rows, no warning. |
| | extras only | adds `, plus X table(s) and Y column(s) not in it` (zero clause omitted, e.g. `plus 2 columns not in it`), then `+` rows. W1, exit 0. |
| | missing columns only | `DIFFERS … : Y column(s) missing` plus any `, N not in reference`, then rows. Exit 1. |
| | missing tables | as above, `- table` rows only for those tables. Exit 1. |
| | missing plus extras | both counts, `-` rows then `+` rows. Exit 1. |
| | non-`Z`, `sqlite_*`, `Z_METADATA`, `Z_MODELCACHE` | no row |
| | column type changed, name same | no row |
| Diff rows | any count | always complete, no cap |
| Warning line | extras only | the one warning in Phase 0 |
| stdout | R1–R14, U1–U2 | empty (human and `--json`) |
| stdout | mismatch | block + diff, or full document |
| Snapshot name | second run same second | `…Z_2.sqlite` / `…Z_2.json`; no dedupe by hash |
| Snapshot name | concurrent syncs | exclusive create; no lock file |
| Partial leftovers | earlier crash | removed silently at start |
| TTY vs pipe | either | same bytes |
| Quicken open on A, `--quicken` names closed B | | R8 ("encrypted") |
| First run on real file | likely mismatch | M1 path; fix `reference.sql`, re-run, exit 0 = gate |

---

## Scenarios (Gherkin)

Architect sizing: SCENARIO-01 split into 01a (snapshot core) and 01b (command surface). Folds: 03 → 01a (conditional, BR-13), 02 → 01b, 14 → 01b, 07 → 06, 09 → 10, 12 → 11. Order: 05 before 04 (05 establishes refusal type and bundle validation that 04 reuses).

```gherkin
Scenario: SCENARIO-01a — Snapshot of an open file matching the reference
  Given an open Quicken bundle (BR-3 fixture) whose schema equals the embedded reference, with 2 accounts and one account row present only in the WAL
  When the snapshot service syncs that bundle into a snapshots directory
  Then a 0600 <UTC>.sqlite and a 0600 <UTC>.json exist in a 0700 snapshots directory
  And the snapshot contains the WAL-only row
  And the manifest records the snapshot's SHA-256, 2 accounts, verified true, and fingerprint equal to reference_fingerprint
```

```gherkin
Scenario: SCENARIO-03 — Live Quicken file is never modified
  Given an open bundle (BR-3 fixture) held by a second connection
  When sync completes
  Then the bundle's data file bytes and mtime are unchanged and no file was added to or removed from the bundle
```

```gherkin
Scenario: SCENARIO-01b — quarry sync reports a verified snapshot
  Given an open bundle whose schema equals the embedded reference, with 2 accounts, and HOME pointed at a temp dir
  When I run `quarry sync --quicken <bundle>`
  Then snapshot and manifest land in ~/Library/Application Support/quarry/snapshots/
  And stdout is the six-line success block, stderr is empty, and exit is 0
```

```gherkin
Scenario: SCENARIO-02 — Machine-readable result
  Given the same bundle
  When I run `quarry sync --quicken <bundle> --json`
  Then stdout is the JSON document with snapshot, schema and warnings keys, byte-identical to the manifest, and exit is 0
```

```gherkin
Scenario Outline: SCENARIO-14 — Usage errors
  When I run `quarry <args>`
  Then stderr is one quarry:-prefixed line <line>, no usage dump, stdout empty, and exit is 2

  Examples:
    | args                     | line |
    | sync ~/x.quicken         | U1   |
    | sync --bogus             | U2   |
    | sync --quicken           | U2   |
    | frob                     | U2   |
```

```gherkin
Scenario Outline: SCENARIO-05 — Bad --quicken path refused
  Given HOME pointed at a temp dir
  When I run `quarry sync --quicken <path>` where the path is <kind>
  Then stderr is <line>, stdout is empty, exit is <code>, and nothing is written under the snapshots dir

  Examples:
    | kind                              | line    | code |
    | missing                           | R4      | 1    |
    | ending .QDF                       | R5      | 1    |
    | a plain file                      | R6      | 1    |
    | a bundle without data             | R6      | 1    |
    | a bundle whose data is unreadable | R7      | 1    |
    | a valid bundle given as ~/…       | success | 0    |
```

```gherkin
Scenario Outline: SCENARIO-04 — Bundle discovery in ~/Documents
  Given ~/Documents contains <bundles>
  When I run `quarry sync` without --quicken
  Then exit is <code> and stderr is <line>

  Examples:
    | bundles                                     | line                           | code |
    | no .quicken bundle                          | R1                             | 1    |
    | exactly one valid bundle                    | empty (Source = that bundle)   | 0    |
    | three bundles                               | R2 (names sorted)              | 1    |
    | an unreadable Documents directory           | R3                             | 1    |
```

```gherkin
Scenario: SCENARIO-06 — Encrypted file means Quicken does not have it open
  Given a bundle whose data file is not a SQLite database
  When I run `quarry sync --quicken <bundle>`
  Then stderr is R8, stdout is empty, exit is 1, and no snapshot, manifest or partial is left
```

```gherkin
Scenario: SCENARIO-07 — Quicken busy writing
  Given another connection holds an exclusive lock on the data file past the busy timeout
  When I run `quarry sync --quicken <bundle>`
  Then stderr is R9, stdout is empty, exit is 1, and nothing is kept
```

```gherkin
Scenario Outline: SCENARIO-08 — Snapshot content rejected
  Given a readable bundle that is <case>
  When I run `quarry sync --quicken <bundle>`
  Then stderr is <line>, stdout is empty, exit is 1, and nothing is kept

  Examples:
    | case                                                  | line |
    | damaged so only integrity_check fails                 | R10  |
    | missing ZACCOUNT (including a 0-byte data file)       | R11  |
    | ZACCOUNT with no rows                                 | R12  |
```

```gherkin
Scenario: SCENARIO-10 — Missing tables or columns fail the schema check
  Given a bundle missing 1 table and 2 columns the reference names, plus 1 column not in the reference
  When I run `quarry sync --quicken <bundle>`
  Then snapshot and manifest are kept with verified false
  And stdout is the DIFFERS block with - rows before + rows and the missing table's columns not repeated
  And stderr is M1 and exit is 1
```

```gherkin
Scenario: SCENARIO-09 — Extra tables or columns only
  Given a bundle with everything in the reference plus 1 table and 2 columns
  When I run `quarry sync --quicken <bundle>`
  Then snapshot is kept, the Schema line has the "plus …" clause and + rows, stderr is W1, warnings holds W1's text, and exit is 0
```

```gherkin
Scenario: SCENARIO-11 — Same-second runs do not collide
  Given a snapshot pair already named for the current second
  When I run sync again in that second
  Then the new files are <UTC>_2.sqlite and <UTC>_2.json and the first pair is untouched
```

```gherkin
Scenario: SCENARIO-12 — Crash leftovers cleaned silently
  Given quarry .partial files in the snapshots dir from an earlier crash
  When I run sync
  Then they are gone and output does not mention them
```

```gherkin
Scenario: SCENARIO-13 — Snapshots directory not writable
  Given the snapshots directory is not writable by the user
  When I run `quarry sync --quicken <bundle>`
  Then stderr is R13, stdout is empty, exit is 1, and no partial is left
```

---

## BDD Acceptance Progress
- [ ] SCENARIO-01a: Snapshot of an open file matching the reference
- [ ] SCENARIO-03: Live Quicken file is never modified
- [ ] SCENARIO-01b: quarry sync reports a verified snapshot
- [ ] SCENARIO-02: Machine-readable result
- [ ] SCENARIO-14: Usage errors
- [ ] SCENARIO-05: Bad --quicken path refused
- [ ] SCENARIO-04: Bundle discovery in ~/Documents
- [ ] SCENARIO-06: Encrypted file means Quicken does not have it open
- [ ] SCENARIO-07: Quicken busy writing
- [ ] SCENARIO-08: Snapshot content rejected
- [ ] SCENARIO-10: Missing tables or columns fail the schema check
- [ ] SCENARIO-09: Extra tables or columns only
- [ ] SCENARIO-11: Same-second runs do not collide
- [ ] SCENARIO-12: Crash leftovers cleaned silently
- [ ] SCENARIO-13: Snapshots directory not writable
